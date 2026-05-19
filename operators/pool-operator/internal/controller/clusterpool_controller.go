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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	imsv1alpha1 "github.com/example/ocloud-edge/operators/pool-operator/api/v1alpha1"
)

const (
	// clusterPoolPhaseDeferredCondition is set on every ClusterPool
	// observed by this controller in Phase 3. The real Karmada-integrated
	// reconcile body lands in Phase 9 (see architecture.md sec 13).
	clusterPoolPhaseDeferredCondition = "PhaseDeferred"
)

// ClusterPoolReconciler reconciles ClusterPool resources.
//
// Phase 3 contract (P3-T-105): passive observer. Reconcile sets a single
// PhaseDeferred=True condition with Reason=WaitingForKarmada and otherwise
// leaves Status untouched. Real member-sync / Karmada propagation logic
// lands in Phase 9; we do not watch Karmada APIs here because those CRDs
// are not installed in a Phase 3 cluster and watching them would crash the
// manager on startup.
type ClusterPoolReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=ims.ocloud.edge.example.com,resources=clusterpools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ims.ocloud.edge.example.com,resources=clusterpools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ims.ocloud.edge.example.com,resources=clusterpools/finalizers,verbs=update

// Reconcile is the Phase 3 placeholder. It emits a PhaseDeferred=True
// condition so dashboards can render the deferred state, then returns. It
// is idempotent: once the condition is True it skips the Status update on
// subsequent passes so downstream watchers do not see LastTransitionTime
// churn.
func (r *ClusterPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var pool imsv1alpha1.ClusterPool
	if err := r.Get(ctx, req.NamespacedName, &pool); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Idempotent: if the PhaseDeferred condition is already present and True,
	// there is nothing to do — skip the Status update to avoid LastTransitionTime
	// churn that would re-queue downstream watchers.
	if existing := findExistingCondition(pool.Status.Conditions, clusterPoolPhaseDeferredCondition); existing != nil && existing.Status == metav1.ConditionTrue {
		return ctrl.Result{}, nil
	}

	SetCondition(&pool.Status.Conditions, metav1.Condition{
		Type:               clusterPoolPhaseDeferredCondition,
		Status:             metav1.ConditionTrue,
		Reason:             "WaitingForKarmada",
		Message:            "ClusterPool Reconcile is a Phase 9 deliverable; Phase 3 only verifies CRD acceptance and emits this condition so dashboards can render the state. See architecture.md sec 13 review-table Phase 9 entry.",
		LastTransitionTime: metav1.Now(),
	})

	if err := r.Status().Update(ctx, &pool); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// findExistingCondition is a local lookup used to make Reconcile idempotent
// without colliding with the test-only `findCondition` helper in
// npuslicepool_controller_test.go (which lives in package controller_test
// and is therefore invisible to production code).
func findExistingCondition(conds []metav1.Condition, condType string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == condType {
			return &conds[i]
		}
	}
	return nil
}

// SetupWithManager wires the reconciler into mgr, watching the ClusterPool
// primary resource only. Secondary watches against Karmada Cluster /
// PropagationPolicy resources land in Phase 9 once those CRDs exist in the
// hosting cluster; watching them in Phase 3 would crash the manager.
func (r *ClusterPoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&imsv1alpha1.ClusterPool{}).
		Named("clusterpool").
		Complete(r)
}
