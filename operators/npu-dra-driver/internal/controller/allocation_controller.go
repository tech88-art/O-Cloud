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
	"time"

	corev1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/npu-dra-driver/api/v1alpha1"
)

// OrphanGracePeriod is how long an NPUSliceAllocation may have a
// dangling owner-ref (claim deleted but cascade GC has not yet removed
// the audit entry) before the controller marks it phase=Orphaned.
//
// Plan T005 acceptance: 30s grace. CI kind clusters may need a longer
// value if GC is slow — values.yaml override + DESIGN.md §"Phase 5
// orphan timing" documents the knob.
const OrphanGracePeriod = 30 * time.Second

// AllocationReconciler observes Ocloud NPUSliceAllocation objects and
// drives Status.Phase through Allocated → Released → Orphaned per
// ADR-0009 §5 step 3.
//
// Lifecycle rules:
//
//   - Allocated: the owning ResourceClaim exists AND has a populated
//     Status.Allocation matching this audit entry's SliceRef.
//   - Released: owning ResourceClaim is gone (Get returns NotFound)
//     BUT the audit was created less than OrphanGracePeriod ago. K8s
//     garbage collection is expected to delete the audit shortly.
//   - Orphaned: owning ResourceClaim is gone AND audit is older than
//     OrphanGracePeriod. Surfaces a Warning event so operators can
//     audit / clean up dangling entries.
//
// Cascade-delete works via owner-ref: claim_controller.createAllocationAudit
// sets metadata.ownerReferences pointing at the ResourceClaim. K8s
// GC processes the cascade; this reconciler does NOT delete the audit
// itself (it observes the GC outcome).
type AllocationReconciler struct {
	Client   client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// GracePeriod overrides OrphanGracePeriod. Zero means use the
	// package default. Tests inject a tiny value (e.g. 100ms) so the
	// orphan path is exercisable without real wall-clock waits.
	GracePeriod time.Duration

	// Now is overridable for tests so the orphan grace period is
	// exercisable without manipulating system time. Zero means
	// time.Now.
	Now func() time.Time
}

func (r *AllocationReconciler) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *AllocationReconciler) gracePeriod() time.Duration {
	if r.GracePeriod > 0 {
		return r.GracePeriod
	}
	return OrphanGracePeriod
}

// Reconcile drives the lifecycle described in the package doc.
func (r *AllocationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := log.FromContext(ctx).WithName("allocation-controller").WithValues(
		"npua", req.NamespacedName, "task", "P5-T-005",
	)

	var audit v1alpha1.NPUSliceAllocation
	if err := r.Client.Get(ctx, req.NamespacedName, &audit); err != nil {
		if apierrors.IsNotFound(err) {
			lg.V(1).Info("NPUSliceAllocation deleted (likely owner-ref cascade)")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Look up the owning ResourceClaim. ClaimRef carries
	// Namespace+Name; the underlying owner-ref carries UID for K8s GC.
	var claim resourceapi.ResourceClaim
	claimKey := client.ObjectKey{
		Namespace: audit.Spec.ClaimRef.Namespace,
		Name:      audit.Spec.ClaimRef.Name,
	}
	claimErr := r.Client.Get(ctx, claimKey, &claim)

	switch {
	case claimErr == nil:
		// Claim exists. Make sure phase reflects Allocated.
		return r.markAllocated(ctx, &audit)

	case apierrors.IsNotFound(claimErr):
		// Claim deleted. Decide Released vs Orphaned based on
		// AllocatedAt + grace.
		age := r.now().Sub(allocatedAtOrCreation(&audit))
		if age < r.gracePeriod() {
			return r.markReleased(ctx, &audit, age)
		}
		return r.markOrphaned(ctx, &audit, age)

	default:
		return ctrl.Result{}, fmt.Errorf("get owning claim %s: %w", claimKey, claimErr)
	}
}

func (r *AllocationReconciler) markAllocated(ctx context.Context, audit *v1alpha1.NPUSliceAllocation) (ctrl.Result, error) {
	base := audit.DeepCopy()
	changed := false
	if audit.Status.Phase != v1alpha1.NPUSliceAllocationPhaseAllocated {
		audit.Status.Phase = v1alpha1.NPUSliceAllocationPhaseAllocated
		changed = true
	}
	if audit.Status.AllocatedAt == nil {
		now := metav1.Now()
		audit.Status.AllocatedAt = &now
		changed = true
	}
	if before := len(audit.Status.Conditions); true {
		SetCondition(&audit.Status.Conditions, metav1.Condition{
			Type:    v1alpha1.ConditionAvailable,
			Status:  metav1.ConditionTrue,
			Reason:  "Allocated",
			Message: "Owning ResourceClaim observed; allocation is live",
		})
		if len(audit.Status.Conditions) != before ||
			!conditionEqual(audit.Status.Conditions[0], base.Status.Conditions, v1alpha1.ConditionAvailable) {
			changed = true
		}
	}
	if !changed {
		return ctrl.Result{}, nil
	}
	if err := r.Client.Status().Patch(ctx, audit, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, fmt.Errorf("patch allocated status: %w", err)
	}
	return ctrl.Result{}, nil
}

func (r *AllocationReconciler) markReleased(ctx context.Context, audit *v1alpha1.NPUSliceAllocation, age time.Duration) (ctrl.Result, error) {
	base := audit.DeepCopy()
	if audit.Status.Phase == v1alpha1.NPUSliceAllocationPhaseReleased {
		// Re-requeue so we can promote to Orphaned once grace expires.
		return RequeueAfter(r.gracePeriod() - age), nil
	}
	audit.Status.Phase = v1alpha1.NPUSliceAllocationPhaseReleased
	SetCondition(&audit.Status.Conditions, metav1.Condition{
		Type:    v1alpha1.ConditionAvailable,
		Status:  metav1.ConditionFalse,
		Reason:  "ClaimDeleted",
		Message: "Owning ResourceClaim was deleted; awaiting GC cascade",
	})
	if err := r.Client.Status().Patch(ctx, audit, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, fmt.Errorf("patch released status: %w", err)
	}
	return RequeueAfter(r.gracePeriod() - age), nil
}

func (r *AllocationReconciler) markOrphaned(ctx context.Context, audit *v1alpha1.NPUSliceAllocation, age time.Duration) (ctrl.Result, error) {
	base := audit.DeepCopy()
	if audit.Status.Phase == v1alpha1.NPUSliceAllocationPhaseOrphaned {
		return ctrl.Result{}, nil
	}
	audit.Status.Phase = v1alpha1.NPUSliceAllocationPhaseOrphaned
	SetCondition(&audit.Status.Conditions, metav1.Condition{
		Type:    v1alpha1.ConditionAvailable,
		Status:  metav1.ConditionFalse,
		Reason:  "Orphaned",
		Message: fmt.Sprintf("Owning ResourceClaim absent for >%s; GC cascade did not run", age.Round(time.Second)),
	})
	if err := r.Client.Status().Patch(ctx, audit, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, fmt.Errorf("patch orphaned status: %w", err)
	}
	if r.Recorder != nil {
		r.Recorder.Event(audit, corev1.EventTypeWarning, "Orphaned",
			fmt.Sprintf("NPUSliceAllocation %s has dangling owner-ref to claim %s/%s for %s",
				audit.Name, audit.Spec.ClaimRef.Namespace, audit.Spec.ClaimRef.Name, age.Round(time.Second)))
	}
	return ctrl.Result{}, nil
}

// allocatedAtOrCreation returns the timestamp the controller treats
// as "when did this audit entry start counting toward orphan grace".
// Prefer Status.AllocatedAt; fall back to CreationTimestamp for entries
// created before T005 (e.g. T002 commits ran but the status pass
// errored).
func allocatedAtOrCreation(audit *v1alpha1.NPUSliceAllocation) time.Time {
	if audit.Status.AllocatedAt != nil {
		return audit.Status.AllocatedAt.Time
	}
	return audit.CreationTimestamp.Time
}

func conditionEqual(want metav1.Condition, in []metav1.Condition, t string) bool {
	for _, c := range in {
		if c.Type != t {
			continue
		}
		return c.Status == want.Status && c.Reason == want.Reason && c.Message == want.Message
	}
	return false
}

// SetupWithManager registers this Reconciler with the manager. Watches
// NPUSliceAllocation; the owning ResourceClaim watch happens implicitly
// via the GC controller — when the claim is deleted, K8s GC fires an
// update on this audit object's owner-ref status which surfaces via
// the watch.
func (r *AllocationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.NPUSliceAllocation{}).
		Named("npu-dra-allocation").
		Complete(r)
}
