/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	imsv1alpha1 "github.com/example/ocloud-edge/operators/pool-operator/api/v1alpha1"
)

// Ascend Device Plugin conventions — see docs/research/ascend-device-plugin.md.
// huawei.com/Ascend910-Health=Healthy is the per-node health label written
// by the device plugin's health probe. The extended-resource name
// huawei.com/Ascend910 is shared with NPUSlicePool and is declared as
// npuCapacityResource in npuslicepool_controller.go.
const (
	npuHealthLabel        = "huawei.com/Ascend910-Health"
	npuHealthLabelHealthy = "Healthy"
)

// npu-dra-driver ResourceSlice contract — text-copied (no Go import) per
// operators/CLAUDE.md §1 "module path 不交叉依赖" rule. Sources:
//   - SliceLabelManagedBy / Value: operators/npu-dra-driver/internal/publisher/publisher.go
//   - AttrHCCSRing / AttrNPUHealth / HealthHealthy: operators/npu-dra-driver/api/v1alpha1/resourceslice_types.go
// Phase 6 ADR-0010 §5 documents the attribute schema as the cross-controller
// contract.
const (
	npuDriverSliceLabel        = "npu.ocloud.edge.example.com/managed-by"
	npuDriverSliceLabelValue   = "npu-dra-driver"
	attrNPUHealth              = "npu.huawei.com/health"
	attrHCCSRing               = "npu.huawei.com/hccs_ring"
	npuHealthValueHealthy      = "Healthy"
)

// resourceSliceListGVK pins the kind aggregateHCCSTopology lists. Hard-coded
// to avoid cross-module import of the npu-dra-driver / upstream resourceapi
// Go type — pool-operator stays module-isolated.
var resourceSliceListGVK = schema.GroupVersionKind{
	Group:   "resource.k8s.io",
	Version: "v1beta1",
	Kind:    "ResourceSliceList",
}

// NPUPoolReconciler reconciles NPUPool resources.
type NPUPoolReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=ims.ocloud.edge.example.com,resources=npupools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ims.ocloud.edge.example.com,resources=npupools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ims.ocloud.edge.example.com,resources=npupools/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=resource.k8s.io,resources=resourceslices,verbs=get;list;watch

// Reconcile aggregates NPU availability for the pool:
//  1. Resolve spec.selector → Node list.
//  2. Sum huawei.com/Ascend910 capacity into status.totalNPUs; subset of
//     nodes that are Ready AND carry huawei.com/Ascend910-Health=Healthy
//     contribute to status.healthyNPUs.
//  3. Sum huawei.com/Ascend910 requests across all non-terminal Pods on
//     matched nodes into status.allocatedNPUs.
//  4. Aggregate HCCS topology by listing ResourceSlices labelled
//     managed-by=npu-dra-driver, filtering to matched nodes, and grouping
//     healthy device entries by (nodeName, hccs_ring). Phase 6 T003 per
//     ADR-0010 §5.
//  5. Emit Ready / HCCSDiscovered conditions.
func (r *NPUPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var pool imsv1alpha1.NPUPool
	if err := r.Get(ctx, req.NamespacedName, &pool); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 1. Resolve selector → list matched Nodes.
	sel := labels.Everything()
	if pool.Spec.Selector != nil {
		s, err := metav1.LabelSelectorAsSelector(pool.Spec.Selector)
		if err != nil {
			SetCondition(&pool.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				Reason:             "InvalidSelector",
				Message:            err.Error(),
				LastTransitionTime: metav1.Now(),
			})
			if upErr := r.Status().Update(ctx, &pool); upErr != nil {
				return ctrl.Result{}, upErr
			}
			return ctrl.Result{}, nil
		}
		sel = s
	}

	var nodes corev1.NodeList
	if err := r.List(ctx, &nodes, &client.ListOptions{LabelSelector: sel}); err != nil {
		return ctrl.Result{}, err
	}
	matchedNodeNames := make(map[string]struct{}, len(nodes.Items))
	for _, n := range nodes.Items {
		matchedNodeNames[n.Name] = struct{}{}
	}

	// 2. Sum NPU capacity + health across matched nodes.
	var totalNPUs, healthyNPUs int64
	for _, n := range nodes.Items {
		q, ok := n.Status.Capacity[npuCapacityResource]
		if !ok {
			continue
		}
		cap := q.Value()
		totalNPUs += cap
		if isNodeNPUHealthy(&n) {
			healthyNPUs += cap
		}
	}

	// 3. Sum allocated NPUs from non-terminal Pods on matched nodes.
	var allocated int64
	if len(matchedNodeNames) > 0 {
		var pods corev1.PodList
		if err := r.List(ctx, &pods); err != nil {
			return ctrl.Result{}, err
		}
		for _, p := range pods.Items {
			if _, ok := matchedNodeNames[p.Spec.NodeName]; !ok {
				continue
			}
			if p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed {
				continue
			}
			for _, c := range p.Spec.Containers {
				if q, ok := c.Resources.Requests[npuCapacityResource]; ok {
					allocated += q.Value()
				}
			}
		}
	}

	// 4. Aggregate HCCS topology from ResourceSlices the npu-dra-driver
	//    published for matched nodes. Phase 6 T003 per ADR-0010 §5.
	hccs, err := r.aggregateHCCSTopology(ctx, matchedNodeNames)
	if err != nil {
		return ctrl.Result{}, err
	}

	// 5. Write status.
	pool.Status.TotalNPUs = int32(totalNPUs)
	pool.Status.HealthyNPUs = int32(healthyNPUs)
	pool.Status.AllocatedNPUs = int32(allocated)
	pool.Status.HCCSTopology = hccs

	if totalNPUs == 0 {
		SetCondition(&pool.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "NoNPUFound",
			Message:            "Selector matched no nodes with huawei.com/Ascend910 capacity",
			LastTransitionTime: metav1.Now(),
		})
	} else {
		SetCondition(&pool.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "Reconciled",
			Message:            fmt.Sprintf("%d/%d NPUs healthy, %d allocated", healthyNPUs, totalNPUs, allocated),
			LastTransitionTime: metav1.Now(),
		})
	}

	hccsCondition := metav1.Condition{
		Type:               "HCCSDiscovered",
		LastTransitionTime: metav1.Now(),
	}
	switch {
	case hccs == nil:
		hccsCondition.Status = metav1.ConditionFalse
		hccsCondition.Reason = "NoResourceSlicesObserved"
		hccsCondition.Message = "no managed-by=npu-dra-driver ResourceSlices found for pool nodes"
	case len(hccs.PeerGroups) == 0:
		hccsCondition.Status = metav1.ConditionFalse
		hccsCondition.Reason = "NoHealthyDevicesWithHCCS"
		hccsCondition.Message = "ResourceSlices observed but no healthy devices carry hccs_ring attribute"
	default:
		hccsCondition.Status = metav1.ConditionTrue
		hccsCondition.Reason = "Aggregated"
		hccsCondition.Message = fmt.Sprintf("%d HCCS peer groups observed across pool nodes", len(hccs.PeerGroups))
	}
	SetCondition(&pool.Status.Conditions, hccsCondition)

	if err := r.Status().Update(ctx, &pool); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// aggregateHCCSTopology lists ResourceSlices labelled
// `npu.ocloud.edge.example.com/managed-by=npu-dra-driver`, filters by
// membership in matchedNodeNames, and groups healthy devices by
// (nodeName, hccs_ring) into HCCSPeerGroup entries.
//
// PeerGroup convention (documented for downstream consumers + scheduler-
// plugin T005 sibling-Pod lookups):
//   - GroupID    = "<nodeName>/ring-<int>"     (e.g. "worker-a/ring-0")
//   - DeviceIDs  = ["<nodeName>/<deviceName>", ...] sorted asc
//   - Entries within PeerGroups sorted by (nodeName, ring) asc for
//     deterministic status writes (avoids spurious resourceVersion churn).
//
// Returns nil when no ResourceSlices match the label / belong to the pool —
// callers distinguish nil ("no data yet") from empty PeerGroups ("data but
// no rings recoverable"). Health filter: devices whose
// `npu.huawei.com/health` attribute is anything other than "Healthy" are
// skipped (absent attribute → treated as healthy for back-compat with
// pre-T002 publishers).
//
// Cross-module discipline: ResourceSlices are read via unstructured.
// UnstructuredList — operators/CLAUDE.md §1 forbids Go imports across
// operators sub-projects. The schema contract is fixed by ADR-0010 §5
// (text-copied attribute names + label name).
//
// Tolerance: if the cluster has no resource.k8s.io API installed (e.g.
// envtest without DRA feature gate), the meta.IsNoMatchError check returns
// nil so the controller doesn't crash — HCCSDiscovered just stays
// NoResourceSlicesObserved until the API surface arrives.
func (r *NPUPoolReconciler) aggregateHCCSTopology(
	ctx context.Context,
	matchedNodeNames map[string]struct{},
) (*imsv1alpha1.HCCSTopologyInfo, error) {
	if len(matchedNodeNames) == 0 {
		return nil, nil
	}

	var sliceList unstructured.UnstructuredList
	sliceList.SetGroupVersionKind(resourceSliceListGVK)
	if err := r.List(ctx, &sliceList, client.MatchingLabels{
		npuDriverSliceLabel: npuDriverSliceLabelValue,
	}); err != nil {
		if meta.IsNoMatchError(err) {
			// resource.k8s.io API not installed → behave as if no slices.
			return nil, nil
		}
		return nil, err
	}
	if len(sliceList.Items) == 0 {
		return nil, nil
	}

	// groupedDevices[nodeName][ringID] = []deviceName, accumulated then sorted.
	groupedDevices := map[string]map[int64][]string{}
	matchedSlice := false

	for _, slice := range sliceList.Items {
		nodeName, _, _ := unstructured.NestedString(slice.Object, "spec", "nodeName")
		if _, ok := matchedNodeNames[nodeName]; !ok {
			continue
		}
		matchedSlice = true
		devices, _, _ := unstructured.NestedSlice(slice.Object, "spec", "devices")
		for _, d := range devices {
			dm, ok := d.(map[string]interface{})
			if !ok {
				continue
			}
			deviceName, _, _ := unstructured.NestedString(dm, "name")
			attrs, _, _ := unstructured.NestedMap(dm, "basic", "attributes")
			if !healthAttributeOK(attrs) {
				continue
			}
			ring, ok := readIntAttribute(attrs, attrHCCSRing)
			if !ok {
				continue
			}
			if _, exists := groupedDevices[nodeName]; !exists {
				groupedDevices[nodeName] = map[int64][]string{}
			}
			groupedDevices[nodeName][ring] = append(groupedDevices[nodeName][ring], deviceName)
		}
	}

	if !matchedSlice {
		return nil, nil
	}
	if len(groupedDevices) == 0 {
		// Slices belong to pool nodes but no healthy device carries the
		// hccs_ring attribute. Return an empty (non-nil) topology so
		// callers see "discovered but empty" — HCCSDiscovered condition
		// flips to NoHealthyDevicesWithHCCS.
		return &imsv1alpha1.HCCSTopologyInfo{}, nil
	}

	nodes := make([]string, 0, len(groupedDevices))
	for n := range groupedDevices {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)

	var peers []imsv1alpha1.HCCSPeerGroup
	for _, node := range nodes {
		rings := make([]int64, 0, len(groupedDevices[node]))
		for r := range groupedDevices[node] {
			rings = append(rings, r)
		}
		sort.Slice(rings, func(i, j int) bool { return rings[i] < rings[j] })
		for _, ring := range rings {
			devs := append([]string(nil), groupedDevices[node][ring]...)
			sort.Strings(devs)
			qualified := make([]string, len(devs))
			for i, d := range devs {
				qualified[i] = fmt.Sprintf("%s/%s", node, d)
			}
			peers = append(peers, imsv1alpha1.HCCSPeerGroup{
				GroupID:   fmt.Sprintf("%s/ring-%d", node, ring),
				DeviceIDs: qualified,
			})
		}
	}

	return &imsv1alpha1.HCCSTopologyInfo{
		PeerGroups: peers,
	}, nil
}

// healthAttributeOK reports whether the device's npu.huawei.com/health
// attribute is "Healthy" or absent (back-compat: pre-T002 publishers may
// not emit this attribute). Anything else (Unhealthy / Unknown / unexpected
// payload shape) returns false.
func healthAttributeOK(attrs map[string]interface{}) bool {
	a, ok := attrs[attrNPUHealth]
	if !ok {
		return true
	}
	m, ok := a.(map[string]interface{})
	if !ok {
		return false
	}
	s, _, _ := unstructured.NestedString(m, "string")
	return s == npuHealthValueHealthy
}

// readIntAttribute extracts an int64 from a DeviceAttribute structured as
// {"int": <int64>}. Returns ok=false on missing key / wrong shape so
// callers can decide whether absence is fatal.
func readIntAttribute(attrs map[string]interface{}, key string) (int64, bool) {
	a, ok := attrs[key]
	if !ok {
		return 0, false
	}
	m, ok := a.(map[string]interface{})
	if !ok {
		return 0, false
	}
	v, found, err := unstructured.NestedInt64(m, "int")
	if err != nil || !found {
		return 0, false
	}
	return v, true
}

// isNodeNPUHealthy reports whether n is both kubelet-Ready and carries the
// Ascend Device Plugin's huawei.com/Ascend910-Health=Healthy label.
func isNodeNPUHealthy(n *corev1.Node) bool {
	var ready bool
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
			ready = true
			break
		}
	}
	if !ready {
		return false
	}
	return n.Labels[npuHealthLabel] == npuHealthLabelHealthy
}

// SetupWithManager wires the reconciler into mgr, watching NPUPool only.
// Node/Pod fan-in via secondary watches is deferred to a later task; the
// reconcile path runs on a periodic resync today, which is sufficient for
// the Phase 3 demo dataset (4 nodes, dozens of pods).
func (r *NPUPoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&imsv1alpha1.NPUPool{}).
		Named("npupool").
		Complete(r)
}
