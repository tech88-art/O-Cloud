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

	corev1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	imsv1alpha1 "github.com/example/ocloud-edge/operators/pool-operator/api/v1alpha1"
)

// npuDraDriverName is the resource.k8s.io DriverName the npu-dra-driver
// publishes slices under. Phase 4 P4-T-102: the NPUSlicePool Reconcile
// counts ResourceSlices carrying this driver name into
// status.resourceSlicesObserved. Kept as a const in this package so the
// cross-watch filter and the post-list match use the same string;
// operators/npu-dra-driver/api/v1alpha1.DriverName holds the same value
// but we deliberately do not cross-module import (per operators/CLAUDE.md
// §1 "module path 不交叉依赖").
const npuDraDriverName = "npu.ocloud.edge.example.com"

const (
	// aiCoreTotalAscend910B is the AI Core count per physical NPU device.
	// Phase 3: hardcoded for Ascend910B (32 cores). Phase 4+ will source this
	// from the Ascend DRA driver's per-device capacity report (see arch §1.3
	// / ADR-0001).
	aiCoreTotalAscend910B int32 = 32

	// npuCapacityResource is the K8s extended resource exposed by the Ascend
	// device plugin on each Node.Status.Capacity / Allocatable.
	npuCapacityResource corev1.ResourceName = "huawei.com/Ascend910B"

	// npuSlicePoolFinalizer is the finalizer added on creation; removed on
	// deletion after slice-instance cleanup completes. Phase 3 cleanup is a
	// no-op placeholder; Phase 5 NPUSliceAllocation will require it.
	npuSlicePoolFinalizer = "pool.ocloud.edge.example.com/npuslicepool-cleanup"

	// npuSlicePoolReadyConditionType is the canonical Ready condition type
	// reported on every NPUSlicePool status.
	npuSlicePoolReadyConditionType = "Ready"
)

// NPUSlicePoolReconciler reconciles NPUSlicePool resources.
//
// Reconcile contract (P3-T-002):
//   - Reads spec.npuPoolRef -> Get the cluster-scoped NPUPool
//   - Resolves NPUPool.Spec.Selector against the cluster Node list
//   - Sums `huawei.com/Ascend910B` capacity across matched Nodes to derive
//     the parent NPU count
//   - Computes status.totalSlices per Spec.Strategy
//     (FixedTemplate: sum_t(aiCoreTotalAscend910B/template.AICoreCount) * npuCount;
//      Dynamic: floor(aiCoreTotalAscend910B/DynamicSlicing.MinAICore) * npuCount)
//   - Maintains availableSlices = totalSlices - allocatedSlices (clamped >= 0;
//     Phase 5 NPUSliceAllocation populates AllocatedSlices)
//   - Manages the npuslicepool-cleanup finalizer; deletion-path body is a
//     Phase 5 placeholder.
type NPUSlicePoolReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// reconcileErr is an internal sentinel for a non-transient business-rule
// violation that should surface as a Ready=False condition rather than a
// requeue-on-error. Returning it does not retry the workqueue; the user must
// edit the spec to clear the condition.
type reconcileErr struct {
	reason, message string
}

// +kubebuilder:rbac:groups=ims.ocloud.edge.example.com,resources=npuslicepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ims.ocloud.edge.example.com,resources=npuslicepools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ims.ocloud.edge.example.com,resources=npuslicepools/finalizers,verbs=update
// +kubebuilder:rbac:groups=ims.ocloud.edge.example.com,resources=npupools,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// P4-T-102 added the .Watches(*v1beta1.ResourceSlice) secondary
// watch but missed the RBAC marker; P5-T-121 (2026-05-20) fixes
// the gap. The controller never listed resourceslices before this
// (manifests/CRDs hadn't been deployed in any test scenario), so
// the kind smoke ran for the first time on commit f00c808 and
// failed at NPUSlicePool reconcile with `Failed to watch
// *v1beta1.ResourceSlice: resourceslices.resource.k8s.io is
// forbidden`. The forbidden watch blocked the controller's cache-
// sync; reconciler workers never started → totalSlices stuck at 0.
// +kubebuilder:rbac:groups=resource.k8s.io,resources=resourceslices,verbs=get;list;watch

// Reconcile materialises NPUSlicePool status from the parent NPUPool's
// resolved Node set. See type doc on NPUSlicePoolReconciler for the contract.
func (r *NPUSlicePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// 1. Get the NPUSlicePool. NotFound = object deleted, nothing to do.
	var pool imsv1alpha1.NPUSlicePool
	if err := r.Get(ctx, req.NamespacedName, &pool); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 2. Deletion path: drain finalizer (placeholder cleanup) and return.
	if !pool.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(&pool, npuSlicePoolFinalizer) {
			// TODO(P3-T-105 / Phase 5): release child NPUSliceAllocation owner
			// references and any externally-tracked slice instances here.
			controllerutil.RemoveFinalizer(&pool, npuSlicePoolFinalizer)
			if err := r.Update(ctx, &pool); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// 3. Ensure finalizer is present before any further work, so a delete
	//    race after Step 1 cannot orphan child slices.
	if !controllerutil.ContainsFinalizer(&pool, npuSlicePoolFinalizer) {
		controllerutil.AddFinalizer(&pool, npuSlicePoolFinalizer)
		if err := r.Update(ctx, &pool); err != nil {
			return ctrl.Result{}, err
		}
		// Requeue to continue status reconciliation against the freshly
		// stored object (resourceVersion has advanced).
		return ctrl.Result{Requeue: true}, nil
	}

	// 4. Resolve parent NPUPool -> Node selector -> NPU device count.
	npuCount, rerr := r.resolveNPUCount(ctx, &pool)
	if rerr != nil {
		log.Info("npuslicepool: cannot resolve npu count", "reason", rerr.reason, "message", rerr.message)
		return r.markNotReady(ctx, &pool, rerr.reason, rerr.message)
	}

	// 5. Compute totalSlices per Strategy.
	totalSlices, rerr := computeTotalSlices(&pool.Spec, npuCount)
	if rerr != nil {
		log.Info("npuslicepool: cannot compute total slices", "reason", rerr.reason, "message", rerr.message)
		return r.markNotReady(ctx, &pool, rerr.reason, rerr.message)
	}

	// 6. Cross-controller observability (P4-T-102): count ResourceSlices
	//    whose driver matches npu-dra-driver. Filtered post-list because
	//    resource.k8s.io/v1beta1 does not currently support a server-side
	//    field selector on `spec.driver` (DRA-1.34 field-selector enablement
	//    is gated behind feature flags in some clusters); a client-side
	//    filter is portable and the slice population is bounded (one entry
	//    per node × driver) so the list is cheap.
	observed, rerr := r.countNPUDRAResourceSlices(ctx)
	if rerr != nil {
		log.Info("npuslicepool: cannot list ResourceSlices (cross-watch)",
			"reason", rerr.reason, "message", rerr.message)
		// Non-fatal — record 0 but keep going. ResourceSlice API may be
		// disabled in older kubelets (KubeEdge / K8s < 1.31); the rest of
		// the Reconcile is still useful.
		observed = 0
	}

	// 7. Write status (totalSlices / availableSlices / resourceSlicesObserved
	//    / Ready=True).
	pool.Status.TotalSlices = totalSlices
	avail := totalSlices - pool.Status.AllocatedSlices
	if avail < 0 {
		avail = 0
	}
	pool.Status.AvailableSlices = avail
	pool.Status.ResourceSlicesObserved = observed
	SetCondition(&pool.Status.Conditions, metav1.Condition{
		Type:               npuSlicePoolReadyConditionType,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: pool.Generation,
		Reason:             "Reconciled",
		Message: fmt.Sprintf(
			"Pool capacity computed: totalSlices=%d, npuCount=%d, resourceSlicesObserved=%d",
			totalSlices, npuCount, observed),
	})
	if err := r.Status().Update(ctx, &pool); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// countNPUDRAResourceSlices lists ResourceSlices cluster-wide and returns
// the count of those whose Spec.Driver matches npu-dra-driver's published
// name. The filter is post-list because v1beta1 lacks a portable
// server-side selector on Spec.Driver.
//
// Returns (count, nil) on success; (0, reconcileErr) on list error. The
// caller treats list errors as "ResourceSlice API unavailable" and records
// 0 without short-circuiting the rest of the reconcile.
func (r *NPUSlicePoolReconciler) countNPUDRAResourceSlices(ctx context.Context) (int32, *reconcileErr) {
	var slices resourceapi.ResourceSliceList
	if err := r.List(ctx, &slices); err != nil {
		return 0, &reconcileErr{reason: "ResourceSliceListFailed", message: err.Error()}
	}
	var n int32
	for _, sl := range slices.Items {
		if sl.Spec.Driver == npuDraDriverName {
			n++
		}
	}
	return n, nil
}

// markNotReady writes Ready=False with the given reason/message and returns
// (Result{}, nil) so the workqueue does not retry. Spec edits will re-trigger
// reconciliation through the watch.
func (r *NPUSlicePoolReconciler) markNotReady(ctx context.Context, pool *imsv1alpha1.NPUSlicePool, reason, message string) (ctrl.Result, error) {
	SetCondition(&pool.Status.Conditions, metav1.Condition{
		Type:               npuSlicePoolReadyConditionType,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: pool.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := r.Status().Update(ctx, pool); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// resolveNPUCount fetches the parent NPUPool, applies its Selector against the
// cluster Node list, and sums `huawei.com/Ascend910B` capacity. Returns a
// reconcileErr (non-nil) for any business-rule failure (missing ref, parent
// not found, malformed selector, list error).
func (r *NPUSlicePoolReconciler) resolveNPUCount(ctx context.Context, pool *imsv1alpha1.NPUSlicePool) (int32, *reconcileErr) {
	if pool.Spec.NPUPoolRef.Name == "" {
		return 0, &reconcileErr{reason: "MissingNPUPoolRef", message: "spec.npuPoolRef.name is empty"}
	}

	var npuPool imsv1alpha1.NPUPool
	if err := r.Get(ctx, client.ObjectKey{Name: pool.Spec.NPUPoolRef.Name}, &npuPool); err != nil {
		if apierrors.IsNotFound(err) {
			return 0, &reconcileErr{
				reason:  "NPUPoolNotFound",
				message: fmt.Sprintf("NPUPool %q (cluster-scoped) does not exist", pool.Spec.NPUPoolRef.Name),
			}
		}
		return 0, &reconcileErr{reason: "NPUPoolFetchError", message: err.Error()}
	}

	// nil Selector == match every Node in the cluster.
	sel := labels.Everything()
	if npuPool.Spec.Selector != nil {
		s, err := metav1.LabelSelectorAsSelector(npuPool.Spec.Selector)
		if err != nil {
			return 0, &reconcileErr{reason: "InvalidNPUPoolSelector", message: err.Error()}
		}
		sel = s
	}

	var nodeList corev1.NodeList
	if err := r.List(ctx, &nodeList, &client.ListOptions{LabelSelector: sel}); err != nil {
		return 0, &reconcileErr{reason: "NodeListError", message: err.Error()}
	}

	var total int64
	for _, node := range nodeList.Items {
		if q, ok := node.Status.Capacity[npuCapacityResource]; ok {
			total += q.Value()
		}
	}
	return int32(total), nil
}

// computeTotalSlices implements the Strategy -> totalSlices contract.
// For FixedTemplate: sum across templates of floor(32/template.AICoreCount) * npuCount.
// For Dynamic:      floor(32/DynamicSlicing.MinAICore) * npuCount.
// npuCount=0 returns 0 cleanly (not an error: the parent pool simply has no
// devices yet, e.g. Node not yet labelled).
func computeTotalSlices(spec *imsv1alpha1.NPUSlicePoolSpec, npuCount int32) (int32, *reconcileErr) {
	if npuCount == 0 {
		return 0, nil
	}
	switch spec.Strategy {
	case imsv1alpha1.SliceStrategyFixedTemplate:
		if len(spec.FixedTemplates) == 0 {
			return 0, &reconcileErr{
				reason:  "InvalidStrategy",
				message: "Strategy=FixedTemplate but fixedTemplates is empty",
			}
		}
		var total int64
		for _, t := range spec.FixedTemplates {
			if t.AICoreCount <= 0 {
				return 0, &reconcileErr{
					reason:  "InvalidTemplate",
					message: fmt.Sprintf("template %q has non-positive aiCoreCount", t.Name),
				}
			}
			slicesPerNPU := int64(aiCoreTotalAscend910B / t.AICoreCount)
			total += slicesPerNPU * int64(npuCount)
		}
		return int32(total), nil
	case imsv1alpha1.SliceStrategyDynamic:
		if spec.DynamicSlicing == nil {
			return 0, &reconcileErr{
				reason:  "InvalidStrategy",
				message: "Strategy=Dynamic but dynamicSlicing is nil",
			}
		}
		if spec.DynamicSlicing.MinAICore <= 0 {
			return 0, &reconcileErr{
				reason:  "InvalidDynamicSlicing",
				message: "dynamicSlicing.minAICore must be > 0",
			}
		}
		slicesPerNPU := int64(aiCoreTotalAscend910B / spec.DynamicSlicing.MinAICore)
		return int32(slicesPerNPU * int64(npuCount)), nil
	default:
		return 0, &reconcileErr{
			reason:  "UnknownStrategy",
			message: fmt.Sprintf("unknown Strategy %q", spec.Strategy),
		}
	}
}

// SetupWithManager wires the reconciler into mgr, watching the NPUSlicePool
// primary resource plus a secondary watch on resource.k8s.io/v1beta1
// ResourceSlice (P4-T-102 cross-controller awareness — when any matching
// slice changes, re-enqueue every NPUSlicePool so status.resourceSlicesObserved
// stays current).
//
// Parent NPUPool changes + Node label changes still rely on periodic resync;
// explicit Owns are deferred to P3-T-003 once NPUPool Reconcile lands.
func (r *NPUSlicePoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&imsv1alpha1.NPUSlicePool{}).
		Watches(
			&resourceapi.ResourceSlice{},
			handler.EnqueueRequestsFromMapFunc(r.mapResourceSliceToPools),
		).
		Named("npuslicepool").
		Complete(r)
}

// mapResourceSliceToPools is the EnqueueRequestsFromMapFunc that fans out
// a ResourceSlice change to every NPUSlicePool. Cheap to compute because
// pool count stays small (Phase 4 < 100 pools per cluster); skipping the
// fan-out for unrelated drivers is a Phase 5 optimisation once the slice
// volume scales.
func (r *NPUSlicePoolReconciler) mapResourceSliceToPools(ctx context.Context, obj client.Object) []reconcile.Request {
	slice, ok := obj.(*resourceapi.ResourceSlice)
	if !ok {
		return nil
	}
	// Filter: skip enqueue when the slice is from a foreign driver — keeps
	// the workqueue quiet when inference-operator (Phase 5) or third-party
	// drivers publish their own slices.
	if slice.Spec.Driver != npuDraDriverName {
		return nil
	}
	var pools imsv1alpha1.NPUSlicePoolList
	if err := r.List(ctx, &pools); err != nil {
		return nil
	}
	out := make([]reconcile.Request, 0, len(pools.Items))
	for _, p := range pools.Items {
		out = append(out, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      p.Name,
				Namespace: p.Namespace,
			},
		})
	}
	return out
}
