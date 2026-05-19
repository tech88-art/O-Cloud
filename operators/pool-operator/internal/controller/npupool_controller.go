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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	imsv1alpha1 "github.com/example/ocloud-edge/operators/pool-operator/api/v1alpha1"
)

// Ascend Device Plugin conventions — see docs/research/ascend-device-plugin.md.
// huawei.com/Ascend910 is the extended-resource name advertised on Node
// capacity/allocatable. huawei.com/Ascend910-Health=Healthy is the per-node
// health label written by the device plugin's health probe.
const (
	npuCapacityResource   corev1.ResourceName = "huawei.com/Ascend910"
	npuHealthLabel                            = "huawei.com/Ascend910-Health"
	npuHealthLabelHealthy                     = "Healthy"
)

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

// Reconcile aggregates NPU availability for the pool:
//  1. Resolve spec.selector → Node list.
//  2. Sum huawei.com/Ascend910 capacity into status.totalNPUs; subset of
//     nodes that are Ready AND carry huawei.com/Ascend910-Health=Healthy
//     contribute to status.healthyNPUs.
//  3. Sum huawei.com/Ascend910 requests across all non-terminal Pods on
//     matched nodes into status.allocatedNPUs.
//  4. Emit Ready / HCCSDiscovered conditions. HCCSTopology is a
//     placeholder until the Phase 6 scheduler-plugin lands real discovery.
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

	// 4. Write status.
	pool.Status.TotalNPUs = int32(totalNPUs)
	pool.Status.HealthyNPUs = int32(healthyNPUs)
	pool.Status.AllocatedNPUs = int32(allocated)
	// HCCS topology discovery is a Phase 6 scheduler-plugin responsibility;
	// emit an empty placeholder so consumers can rely on the field's shape.
	pool.Status.HCCSTopology = &imsv1alpha1.HCCSTopologyInfo{}

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
	SetCondition(&pool.Status.Conditions, metav1.Condition{
		Type:               "HCCSDiscovered",
		Status:             metav1.ConditionFalse,
		Reason:             "PendingPhase6",
		Message:            "HCCS topology discovery is a Phase 6 scheduler-plugin responsibility",
		LastTransitionTime: metav1.Now(),
	})

	if err := r.Status().Update(ctx, &pool); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
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
