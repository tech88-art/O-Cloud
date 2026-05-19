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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	imsv1alpha1 "github.com/example/ocloud-edge/operators/pool-operator/api/v1alpha1"
)

const (
	// aiCoreTotalAscend910B is the AI Core count per physical NPU device.
	// Phase 3: hardcoded for Ascend910B (32 cores). Phase 4+ will source this
	// from the Ascend DRA driver's per-device capacity report (see arch §1.3
	// / ADR-0001).
	aiCoreTotalAscend910B int32 = 32

	// npuCapacityResource is the K8s extended resource exposed by the Ascend
	// device plugin on each Node.Status.Capacity / Allocatable.
	npuCapacityResource corev1.ResourceName = "huawei.com/Ascend910"

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
//   - Sums `huawei.com/Ascend910` capacity across matched Nodes to derive
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

	// 6. Write status (totalSlices / availableSlices / Ready=True).
	pool.Status.TotalSlices = totalSlices
	avail := totalSlices - pool.Status.AllocatedSlices
	if avail < 0 {
		avail = 0
	}
	pool.Status.AvailableSlices = avail
	SetCondition(&pool.Status.Conditions, metav1.Condition{
		Type:               npuSlicePoolReadyConditionType,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: pool.Generation,
		Reason:             "Reconciled",
		Message:            fmt.Sprintf("Pool capacity computed: totalSlices=%d, npuCount=%d", totalSlices, npuCount),
	})
	if err := r.Status().Update(ctx, &pool); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
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
// cluster Node list, and sums `huawei.com/Ascend910` capacity. Returns a
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
// primary resource only. Secondary watches (parent NPUPool changes, Node
// label changes) trigger a re-list on the next periodic resync; explicit
// Watches/Owns are deferred to P3-T-003 once NPUPool Reconcile lands.
func (r *NPUSlicePoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&imsv1alpha1.NPUSlicePool{}).
		Named("npuslicepool").
		Complete(r)
}
