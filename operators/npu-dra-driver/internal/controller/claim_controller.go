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
	"errors"
	"fmt"
	"strings"

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
	"github.com/tech88-art/O-Cloud/operators/npu-dra-driver/internal/allocator"
	"github.com/tech88-art/O-Cloud/operators/npu-dra-driver/internal/publisher"
)

// Phase 4 "AllocationDeferred" annotations.
//
// Deprecated for Phase 5 (T002): the claim controller no longer SETS
// these annotations. They are retained as named constants because the
// Phase 5 controller STRIPS them off pre-existing claims on first
// reconcile (migration path — a cluster that ran Phase 4 may have
// annotated claims still waiting for allocation; the Phase 5 controller
// drops the stale annotation and allocates normally).
//
// Phase 4 design rationale (kept for archaeological reference): upstream
// resource.k8s.io/v1beta1.ResourceClaimStatus has NO top-level Conditions
// field — conditions only live on AllocatedDeviceStatus, which presumes
// an allocation actually happened. Phase 4 had no real allocation so it
// expressed deferred state via annotations + Events. Phase 5 has real
// allocation; conditions move into AllocatedDeviceStatus where they
// belong, and these annotation keys exist only for migration cleanup.
const (
	AnnotationAllocationDeferred            = "ocloud.edge.example.com/allocation-deferred"
	AnnotationAllocationDeferredReason      = "ocloud.edge.example.com/allocation-deferred-reason"
	AnnotationAllocationDeferredMessage     = "ocloud.edge.example.com/allocation-deferred-message"
	AnnotationAllocationDeferredObservedGen = "ocloud.edge.example.com/allocation-deferred-observed-generation"
)

// ConditionAllocationDeferred is the logical condition Type recorded by
// Phase 4's skeleton — preserved as a string constant for cross-
// referencing from old logs / ADR-0009.
//
// Deprecated: Phase 5 uses Type=Ready on AllocatedDeviceStatus.Conditions
// to express "allocated and bound" instead. The constant remains for
// migration-test fixtures only.
const ConditionAllocationDeferred = "AllocationDeferred"

// ReasonPhase4Skeleton is the machine-readable reason value the Phase 4
// controller wrote.
//
// Deprecated: Phase 5 (T002) no longer emits this reason.
const ReasonPhase4Skeleton = "Phase4Skeleton"

// MessagePhase4Skeleton is the human-readable message the Phase 4
// controller wrote.
//
// Deprecated: Phase 5 (T002) no longer emits this message.
const MessagePhase4Skeleton = "ResourceClaim received by npu-dra-driver; " +
	"allocation logic is a Phase 5 deliverable per ADR-0001 v3 §7. " +
	"See docs/phase4-plan.md §3 P4-T-006 and docs/adr/0009-npu-dra-driver.md for the design."

// Phase 5 events.
const (
	reasonAllocated        = "Allocated"
	reasonAllocationFailed = "AllocationFailed"
	reasonNoAvailable      = "NoAvailableDevice"
	reasonMigrated         = "Phase4AnnotationStripped"
)

// ClaimReconciler observes upstream resource.k8s.io/v1beta1.ResourceClaim
// objects and, for those referencing the npu-dra-driver DeviceClass
// family, runs the allocator and writes the picked device into
// Status.Allocation + Status.Devices per ADR-0009 §6.
//
// Phase 5 swap-in (T002): the Phase 4 annotation-only skeleton is gone.
// The controller now performs real allocation. If a pre-existing claim
// still carries the Phase 4 AllocationDeferred annotations, they are
// stripped on first reconcile and a normal allocation pass proceeds.
type ClaimReconciler struct {
	Client   client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Allocator picks a device for the claim. When nil, a zero-value
	// Greedy is used — this keeps main.go and the helm chart Phase 4
	// shape working without a wiring change. Tests can inject custom
	// allocators (e.g. table-driven mocks in T003).
	Allocator allocator.Allocator
}

// Reconcile implements the controller-runtime Reconciler contract.
//
// Phase 5 behaviour matrix:
//   - Claim deleted (NotFound on Get): no-op, no finalizer to drain.
//   - Claim references a foreign driver class: ignore.
//   - Claim already allocated (Status.Allocation != nil): no-op (avoid
//     re-allocating; another reconcile triggered by a status update we
//     just wrote ourselves).
//   - Phase 4 annotations present on a not-yet-allocated claim: strip
//     them in a metadata patch, requeue immediately to run the real
//     allocator on the cleaned state.
//   - Otherwise: list owned slices + existing allocations, run the
//     allocator, write Status.Allocation + Status.Devices via the
//     status subresource. On ErrNoAvailableDevice, emit a Warning event
//     and return Requeue=true (next tick may have new slices / freed
//     devices).
func (r *ClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := log.FromContext(ctx).WithName("claim-controller").WithValues(
		"claim", req.NamespacedName, "task", "P5-T-002",
	)

	var claim resourceapi.ResourceClaim
	if err := r.Client.Get(ctx, req.NamespacedName, &claim); err != nil {
		if apierrors.IsNotFound(err) {
			lg.V(1).Info("ResourceClaim deleted; no finalizer to drain")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !claimRequestsOurDriver(claim) {
		lg.V(1).Info("Ignoring claim — not requesting npu-dra-driver class",
			"requests", deviceClassNames(claim))
		return ctrl.Result{}, nil
	}

	if claim.Status.Allocation != nil {
		// Even on the "already allocated" early-return path, make sure
		// the audit-log NPUSliceAllocation entry exists. This catches
		// the case where status.allocation was written previously but
		// the audit-log Create errored — next reconcile reconciles it.
		if err := r.ensureAllocationAudits(ctx, &claim); err != nil {
			return ctrl.Result{}, fmt.Errorf("ensure audit (already allocated): %w", err)
		}
		lg.V(1).Info("Claim already allocated; audit reconciled",
			"results", len(claim.Status.Allocation.Devices.Results))
		return ctrl.Result{}, nil
	}

	if hasPhase4Annotations(&claim) {
		if err := r.stripPhase4Annotations(ctx, &claim); err != nil {
			return ctrl.Result{}, fmt.Errorf("strip phase-4 annotations: %w", err)
		}
		if r.Recorder != nil {
			r.Recorder.Event(&claim, corev1.EventTypeNormal, reasonMigrated,
				"Stripped Phase 4 AllocationDeferred annotations; running Phase 5 allocator on next reconcile")
		}
		// Re-fetch happens implicitly via the requeue; not strictly
		// required to bounce here because the patch we just issued
		// touched only metadata. But returning Requeue=true keeps the
		// "one concern per reconcile" invariant clean.
		return ctrl.Result{Requeue: true}, nil
	}

	var slices resourceapi.ResourceSliceList
	if err := r.Client.List(ctx, &slices, client.MatchingLabels{
		publisher.SliceLabelManagedBy: publisher.SliceLabelManagedByValue,
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("list resource slices: %w", err)
	}

	var allClaims resourceapi.ResourceClaimList
	if err := r.Client.List(ctx, &allClaims); err != nil {
		return ctrl.Result{}, fmt.Errorf("list resource claims: %w", err)
	}
	allocated := allocator.ComputeAllocatedFromClaims(allClaims.Items, string(claim.UID))

	alloc := r.allocatorOrDefault()
	pick, err := alloc.Allocate(claim, slices.Items, allocated)
	if err != nil {
		if errors.Is(err, allocator.ErrNoAvailableDevice) {
			if r.Recorder != nil {
				r.Recorder.Event(&claim, corev1.EventTypeWarning, reasonNoAvailable,
					"No NPU device currently available; will retry")
			}
			return ctrl.Result{Requeue: true}, nil
		}
		if r.Recorder != nil {
			r.Recorder.Event(&claim, corev1.EventTypeWarning, reasonAllocationFailed, err.Error())
		}
		return ctrl.Result{}, fmt.Errorf("allocator: %w", err)
	}

	base := claim.DeepCopy()
	claim.Status.Allocation = &resourceapi.AllocationResult{
		Devices: resourceapi.DeviceAllocationResult{
			Results: []resourceapi.DeviceRequestAllocationResult{{
				Request: pick.Request,
				Driver:  pick.Driver,
				Pool:    pick.Pool,
				Device:  pick.Device,
			}},
		},
	}
	claim.Status.Devices = []resourceapi.AllocatedDeviceStatus{{
		Driver: pick.Driver,
		Pool:   pick.Pool,
		Device: pick.Device,
		Conditions: []metav1.Condition{{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "Allocated",
			Message:            fmt.Sprintf("Allocated by npu-dra-driver (strategy=%s, aiCores=%d)", pick.Strategy, pick.AICores),
			LastTransitionTime: metav1.Now(),
		}},
	}}

	if err := r.Client.Status().Patch(ctx, &claim, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, fmt.Errorf("write status.allocation: %w", err)
	}

	if err := r.createAllocationAudit(ctx, &claim, pick); err != nil {
		// Don't unwind the status patch — Kubernetes does not give
		// us a transaction across kinds. Surface the audit failure
		// and let the next reconcile finish the audit-log write via
		// ensureAllocationAudits().
		return ctrl.Result{}, fmt.Errorf("create allocation audit: %w", err)
	}

	if r.Recorder != nil {
		r.Recorder.Event(&claim, corev1.EventTypeNormal, reasonAllocated,
			fmt.Sprintf("Allocated NPU %s/%s/%s (strategy=%s, aiCores=%d, node=%s)",
				pick.Driver, pick.Pool, pick.Device, pick.Strategy, pick.AICores, pick.NodeName))
	}

	lg.Info("Allocated NPU device",
		"driver", pick.Driver,
		"pool", pick.Pool,
		"device", pick.Device,
		"strategy", pick.Strategy,
		"aiCores", pick.AICores,
		"node", pick.NodeName)

	return ctrl.Result{}, nil
}

func (r *ClaimReconciler) allocatorOrDefault() allocator.Allocator {
	if r.Allocator != nil {
		return r.Allocator
	}
	return &allocator.Greedy{}
}

// SetupWithManager registers this Reconciler with the manager. Watches
// upstream resource.k8s.io/v1beta1.ResourceClaim.
func (r *ClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&resourceapi.ResourceClaim{}).
		Named("npu-dra-claim").
		Complete(r)
}

// claimRequestsOurDriver returns true when the claim references the
// npu-dra-driver class family.
func claimRequestsOurDriver(claim resourceapi.ResourceClaim) bool {
	for _, req := range claim.Spec.Devices.Requests {
		if isOurClass(req.DeviceClassName) {
			return true
		}
	}
	return false
}

func isOurClass(name string) bool {
	if name == "" {
		return false
	}
	// Exact match or sub-class match — both supported so claims authored
	// against either the K8s-RFC-1123-legal `.suffix` form or the
	// forward-compat `/suffix` form bind to the npu-dra-driver class
	// family. T001 ships the `.suffix` DeviceClasses; the `/suffix`
	// branch is kept for outside-authored claims and migration tests.
	return name == v1alpha1.DriverName ||
		strings.HasPrefix(name, v1alpha1.DriverName+"/") ||
		strings.HasPrefix(name, v1alpha1.DriverName+".")
}

// deviceClassNames flattens the claim's per-request DeviceClassName
// fields to a slice for log emission.
func deviceClassNames(claim resourceapi.ResourceClaim) []string {
	out := make([]string, 0, len(claim.Spec.Devices.Requests))
	for _, req := range claim.Spec.Devices.Requests {
		out = append(out, req.DeviceClassName)
	}
	return out
}

// hasPhase4Annotations reports whether the claim still carries any of the
// four Phase 4 AllocationDeferred annotation keys. Used by the migration
// path: encounter → strip → requeue → allocate.
func hasPhase4Annotations(claim *resourceapi.ResourceClaim) bool {
	if claim.Annotations == nil {
		return false
	}
	for _, k := range []string{
		AnnotationAllocationDeferred,
		AnnotationAllocationDeferredReason,
		AnnotationAllocationDeferredMessage,
		AnnotationAllocationDeferredObservedGen,
	} {
		if _, ok := claim.Annotations[k]; ok {
			return true
		}
	}
	return false
}

// stripPhase4Annotations removes the Phase 4 AllocationDeferred annotation
// keys via a metadata MergeFrom patch — touching only the keys we own
// keeps concurrent edits to other annotations safe.
func (r *ClaimReconciler) stripPhase4Annotations(ctx context.Context, claim *resourceapi.ResourceClaim) error {
	base := claim.DeepCopy()
	if claim.Annotations == nil {
		return nil
	}
	delete(claim.Annotations, AnnotationAllocationDeferred)
	delete(claim.Annotations, AnnotationAllocationDeferredReason)
	delete(claim.Annotations, AnnotationAllocationDeferredMessage)
	delete(claim.Annotations, AnnotationAllocationDeferredObservedGen)
	return r.Client.Patch(ctx, claim, client.MergeFrom(base))
}

// allocationAuditName is the deterministic NPUSliceAllocation object
// name for one (claim, device) pair. Phase 5 ships single-device claims
// (one allocation result per claim), so namespace + claim name + device
// suffix produces a stable, readable name. RFC 1123 compliant — claim
// name and device name are both DNS labels by upstream contract.
func allocationAuditName(claimNS, claimName, device string) string {
	return claimNS + "-" + claimName + "-" + device
}

// createAllocationAudit creates the NPUSliceAllocation audit-log object
// for a freshly-allocated ResourceClaim. The owner-ref points back at
// the ResourceClaim so K8s garbage collection cascades the delete when
// the claim is removed.
//
// Idempotent: if an object with the same deterministic name already
// exists, AlreadyExists is treated as success (next reconcile will
// reach the up-to-date object via the watch).
func (r *ClaimReconciler) createAllocationAudit(ctx context.Context, claim *resourceapi.ResourceClaim, pick *allocator.Allocation) error {
	if claim == nil || pick == nil {
		return nil
	}
	name := allocationAuditName(claim.Namespace, claim.Name, pick.Device)

	ms := claim.Annotations[v1alpha1.AnnotationModelServiceRef]
	allocatedAt := metav1.Now()

	audit := &v1alpha1.NPUSliceAllocation{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "resource.k8s.io/v1beta1",
					Kind:               "ResourceClaim",
					Name:               claim.Name,
					UID:                claim.UID,
					Controller:         pointerToBool(true),
					BlockOwnerDeletion: pointerToBool(true),
				},
			},
			Labels: map[string]string{
				"ocloud.edge.example.com/claim-namespace": claim.Namespace,
				"ocloud.edge.example.com/claim-name":      claim.Name,
				"ocloud.edge.example.com/node":            pick.NodeName,
			},
		},
		Spec: v1alpha1.NPUSliceAllocationSpec{
			ClaimRef: corev1.ObjectReference{
				APIVersion: "resource.k8s.io/v1beta1",
				Kind:       "ResourceClaim",
				Namespace:  claim.Namespace,
				Name:       claim.Name,
				UID:        claim.UID,
			},
			SliceRef: v1alpha1.SliceReference{
				Driver: pick.Driver,
				Pool:   pick.Pool,
				Device: pick.Device,
			},
			NodeName:        pick.NodeName,
			AICores:         int32(pick.AICores),
			ModelServiceRef: ms,
		},
	}

	if err := r.Client.Create(ctx, audit); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return err
	}

	// Set status.phase=Allocated + AllocatedAt + Available=True. Use
	// the status subresource so the Spec patch above doesn't have to
	// race with the AllocationReconciler's own status updates (T005
	// reconciler will also write here on its own watch).
	base := audit.DeepCopy()
	audit.Status.Phase = v1alpha1.NPUSliceAllocationPhaseAllocated
	audit.Status.AllocatedAt = &allocatedAt
	SetCondition(&audit.Status.Conditions, metav1.Condition{
		Type:    v1alpha1.ConditionAvailable,
		Status:  metav1.ConditionTrue,
		Reason:  "Allocated",
		Message: "Audit log entry created for claim allocation",
	})
	return r.Client.Status().Patch(ctx, audit, client.MergeFrom(base))
}

// ensureAllocationAudits re-checks that every device in claim.Status.
// Allocation.Devices.Results has a corresponding NPUSliceAllocation. If
// any are missing (because a prior Create errored), this method
// re-creates them. Idempotent — the "already allocated" early-return
// path in Reconcile invokes this on every pass.
func (r *ClaimReconciler) ensureAllocationAudits(ctx context.Context, claim *resourceapi.ResourceClaim) error {
	if claim == nil || claim.Status.Allocation == nil {
		return nil
	}
	for _, res := range claim.Status.Allocation.Devices.Results {
		if res.Driver != v1alpha1.DriverName {
			continue
		}
		name := allocationAuditName(claim.Namespace, claim.Name, res.Device)
		var existing v1alpha1.NPUSliceAllocation
		err := r.Client.Get(ctx, client.ObjectKey{Name: name}, &existing)
		if err == nil {
			continue
		}
		if !apierrors.IsNotFound(err) {
			return err
		}
		// Missing — recreate from the claim's stored allocation. We
		// don't have the rich Allocation struct (NodeName / AICores /
		// Strategy) here, so the audit-log entry is a degraded copy:
		// NodeName mirrors Pool by Phase 5 simulator convention, and
		// AICores comes from the claim's annotation if present, else
		// 0. Acceptable trade-off — the canonical source of truth is
		// the claim's status.allocation; the audit log is a side
		// index that consumers (Phase 9 quota) can rebuild from the
		// claim list at any time.
		audit := &v1alpha1.NPUSliceAllocation{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         "resource.k8s.io/v1beta1",
						Kind:               "ResourceClaim",
						Name:               claim.Name,
						UID:                claim.UID,
						Controller:         pointerToBool(true),
						BlockOwnerDeletion: pointerToBool(true),
					},
				},
				Labels: map[string]string{
					"ocloud.edge.example.com/claim-namespace": claim.Namespace,
					"ocloud.edge.example.com/claim-name":      claim.Name,
					"ocloud.edge.example.com/node":            res.Pool,
				},
			},
			Spec: v1alpha1.NPUSliceAllocationSpec{
				ClaimRef: corev1.ObjectReference{
					APIVersion: "resource.k8s.io/v1beta1",
					Kind:       "ResourceClaim",
					Namespace:  claim.Namespace,
					Name:       claim.Name,
					UID:        claim.UID,
				},
				SliceRef: v1alpha1.SliceReference{
					Driver: res.Driver,
					Pool:   res.Pool,
					Device: res.Device,
				},
				NodeName:        res.Pool,
				AICores:         0,
				ModelServiceRef: claim.Annotations[v1alpha1.AnnotationModelServiceRef],
			},
		}
		if cerr := r.Client.Create(ctx, audit); cerr != nil && !apierrors.IsAlreadyExists(cerr) {
			return cerr
		}
	}
	return nil
}

func pointerToBool(b bool) *bool { return &b }
