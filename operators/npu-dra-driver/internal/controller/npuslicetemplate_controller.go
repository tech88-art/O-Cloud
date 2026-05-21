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
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/npu-dra-driver/api/v1alpha1"
	"github.com/tech88-art/O-Cloud/operators/npu-dra-driver/internal/template"
)

// NPUSliceTemplateReconciler watches NPUSliceTemplate objects and stamps
// Validated + Allocatable conditions on .status via the Phase 7
// template.Engine (P7-T-007 + ADR-0011 §1 §4 §後果).
//
// **Phase 7 W1 scope** (this commit):
//
//   - Validate the spec.composition + spec.fallbackStrategy via
//     Engine.Validate; on success, run Engine.Decompose to populate
//     status.fallbackAppliedReason
//   - Stamp status.conditions[Validated] (True / False with reason)
//   - Stamp status.conditions[Allocatable] (Phase 7 W1: ALWAYS True
//     when Validated=True · Phase 7 T105 allocator will replace this
//     placeholder with real "sum vs pool availability" check)
//   - Stamp status.observedGeneration = .metadata.generation
//
// **Phase 7 T105 extension** (allocator dynamic-slice path): replaces
// the W1 "always True" Allocatable shortcut with a real bundle-vs-pool
// availability check; reconciler runs first (Validated → Allocatable),
// then Pod admission via slice-template label triggers allocator path.
//
// **Status update** uses Status().Patch with optimistic conflict
// handling (controller-runtime semantics + fake client compatibility).
type NPUSliceTemplateReconciler struct {
	Client   client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Engine is the template engine (Phase 7 W1 stateless). Tests
	// inject a fresh engine; production wires from cmd/main.go.
	Engine *template.Engine
}

// SetupWithManager registers this reconciler with the manager.
func (r *NPUSliceTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Engine == nil {
		r.Engine = template.New()
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.NPUSliceTemplate{}).
		Named("npuslicetemplate").
		Complete(r)
}

// Reconcile validates the composition + decomposes + stamps status.
func (r *NPUSliceTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := log.FromContext(ctx).WithName("npuslicetemplate-reconciler").WithValues("name", req.Name)

	var nst v1alpha1.NPUSliceTemplate
	if err := r.Client.Get(ctx, req.NamespacedName, &nst); err != nil {
		if apierrors.IsNotFound(err) {
			lg.V(1).Info("NPUSliceTemplate deleted, no reconcile work")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if r.Engine == nil {
		r.Engine = template.New()
	}

	// Run Decompose (which calls Validate internally). On
	// ValidationError → stamp Validated=False with the engine's
	// reason; on success → stamp Validated=True + Allocatable=True +
	// fallbackAppliedReason.
	bundle, reason, err := r.Engine.Decompose(nst.Spec)

	desired := computeStatus(&nst, bundle, reason, err)
	if statusEqual(nst.Status, desired) {
		// No churn — already in desired state. Phase 7 T105 may want a
		// requeue after some interval to re-check Allocatable when
		// pool availability changes, but Phase 7 W1 stamps once and
		// trusts the allocator + pool-operator to event-trigger
		// re-reconciles via owner refs / watches.
		return ctrl.Result{}, nil
	}

	nst.Status = desired
	if err := r.Client.Status().Update(ctx, &nst); err != nil {
		return ctrl.Result{}, err
	}

	if template.IsValidationError(err) {
		if r.Recorder != nil {
			ve := template.AsValidationError(err)
			r.Recorder.Eventf(&nst, "Warning", ve.Reason, ve.Message)
		}
	}

	lg.V(1).Info("status reconciled",
		"validated", conditionStatus(desired.Conditions, v1alpha1.ConditionTypeValidated),
		"allocatable", conditionStatus(desired.Conditions, v1alpha1.ConditionTypeAllocatable),
		"fallback", desired.FallbackAppliedReason)
	return ctrl.Result{}, nil
}

// computeStatus builds the desired status block from the Engine result.
// Phase 7 W1: Allocatable mirrors Validated (allocator T105 will
// replace with real check).
func computeStatus(nst *v1alpha1.NPUSliceTemplate, bundle *template.FixedTemplateBundle, reason string, decomposeErr error) v1alpha1.NPUSliceTemplateStatus {
	out := v1alpha1.NPUSliceTemplateStatus{
		ObservedGeneration: nst.Generation,
	}

	now := metav1.Time{Time: time.Now()}
	if decomposeErr != nil {
		ve := template.AsValidationError(decomposeErr)
		validatedReason := "InternalError"
		validatedMsg := decomposeErr.Error()
		if ve != nil {
			validatedReason = ve.Reason
			validatedMsg = ve.Message
		}
		out.Conditions = []metav1.Condition{
			{
				Type:               v1alpha1.ConditionTypeValidated,
				Status:             metav1.ConditionFalse,
				Reason:             validatedReason,
				Message:            validatedMsg,
				LastTransitionTime: now,
			},
			{
				Type:               v1alpha1.ConditionTypeAllocatable,
				Status:             metav1.ConditionFalse,
				Reason:             "ValidatedFalse",
				Message:            "Allocatable gated on Validated=True (Phase 7 P7-T-007)",
				LastTransitionTime: now,
			},
		}
		// FallbackAppliedReason stays empty when validation failed.
		return out
	}

	out.FallbackAppliedReason = reason
	out.Conditions = []metav1.Condition{
		{
			Type:               v1alpha1.ConditionTypeValidated,
			Status:             metav1.ConditionTrue,
			Reason:             "CompositionValid",
			Message:            "spec.composition passed Engine.Validate",
			LastTransitionTime: now,
		},
		{
			Type:               v1alpha1.ConditionTypeAllocatable,
			Status:             metav1.ConditionTrue,
			Reason:             "AllAvailable",
			Message:            "Phase 7 W1 placeholder · allocator T105 replaces with real pool-availability check",
			LastTransitionTime: now,
		},
	}
	_ = bundle // Phase 7 W1 doesn't expose bundle on status; T105 may add a field
	return out
}

// statusEqual is a shallow equality check that ignores LastTransitionTime
// (which we always set to time.Now() and would otherwise cause infinite
// requeue churn).
func statusEqual(a, b v1alpha1.NPUSliceTemplateStatus) bool {
	if a.ObservedGeneration != b.ObservedGeneration {
		return false
	}
	if a.FallbackAppliedReason != b.FallbackAppliedReason {
		return false
	}
	if len(a.Conditions) != len(b.Conditions) {
		return false
	}
	// Compare conditions by Type → Status + Reason (ignore LastTransitionTime).
	aByType := map[string]metav1.Condition{}
	for _, c := range a.Conditions {
		aByType[c.Type] = c
	}
	for _, bc := range b.Conditions {
		ac, ok := aByType[bc.Type]
		if !ok {
			return false
		}
		if ac.Status != bc.Status || ac.Reason != bc.Reason {
			return false
		}
	}
	return true
}

// conditionStatus returns the status string for the named condition,
// or "<absent>" when not present. Used by reconcile log.
func conditionStatus(conds []metav1.Condition, t string) string {
	for _, c := range conds {
		if c.Type == t {
			return string(c.Status)
		}
	}
	return "<absent>"
}
