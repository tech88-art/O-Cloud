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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	imsv1alpha1 "github.com/example/ocloud-edge/operators/pool-operator/api/v1alpha1"
)

// NPUSlicePoolReconciler reconciles NPUSlicePool resources.
// Phase 3 scaffolding: empty Reconcile body. Business logic lands in P3-T-002.
type NPUSlicePoolReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=ims.ocloud.edge.example.com,resources=npuslicepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ims.ocloud.edge.example.com,resources=npuslicepools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ims.ocloud.edge.example.com,resources=npuslicepools/finalizers,verbs=update

// Reconcile is the no-op shell. P3-T-002 will replace this with the
// NPUSlicePool status + finalizer logic per docs/phase3-plan.md §3.
func (r *NPUSlicePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// TODO(P3-T-002): implement NPUSlicePool Reconcile (totalSlices / availableSlices / finalizer)
	_ = ctx
	_ = req
	return ctrl.Result{}, nil
}

// SetupWithManager wires the reconciler into mgr, watching the NPUSlicePool
// primary resource only. Secondary watches (e.g. owning NPUPool) are added
// in P3-T-002.
func (r *NPUSlicePoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&imsv1alpha1.NPUSlicePool{}).
		Named("npuslicepool").
		Complete(r)
}
