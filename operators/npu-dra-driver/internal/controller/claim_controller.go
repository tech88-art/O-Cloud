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
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/npu-dra-driver/api/v1alpha1"
)

// Phase 4 "AllocationDeferred" annotations.
//
// Schema-drift note (P3 honesty): docs/phase4-plan.md §3 P4-T-006 acceptance
// asks for status.conditions[Type=AllocationDeferred]=True on the claim.
// Upstream resource.k8s.io/v1beta1.ResourceClaimStatus has NO top-level
// Conditions field — conditions only live on AllocatedDeviceStatus, which
// presumes an allocation actually happened (Phase 5+ deliverable). Faking
// status.devices entries to attach a condition would mislead the scheduler
// and pod admission ("we allocated nothing but pretended otherwise").
//
// We satisfy the plan intent — machine-observable deferred state with
// reason + message — via metadata.annotations on the claim, plus a
// Kubernetes Event for ops visibility. Phase 5 may revisit this when real
// allocation logic lands and AllocatedDeviceStatus conditions become
// semantically appropriate.
const (
	AnnotationAllocationDeferred           = "ocloud.edge.example.com/allocation-deferred"
	AnnotationAllocationDeferredReason     = "ocloud.edge.example.com/allocation-deferred-reason"
	AnnotationAllocationDeferredMessage    = "ocloud.edge.example.com/allocation-deferred-message"
	AnnotationAllocationDeferredObservedGen = "ocloud.edge.example.com/allocation-deferred-observed-generation"
)

// ConditionAllocationDeferred is the logical condition Type recorded by
// Phase 4's skeleton — preserved as a string constant for cross-referencing
// from logs, the Phase 5 controller, and ADR-0009.
const ConditionAllocationDeferred = "AllocationDeferred"

// ReasonPhase4Skeleton is the machine-readable reason value (per plan literal).
const ReasonPhase4Skeleton = "Phase4Skeleton"

// MessagePhase4Skeleton is the human-readable message (per plan literal).
const MessagePhase4Skeleton = "ResourceClaim received by npu-dra-driver; " +
	"allocation logic is a Phase 5 deliverable per ADR-0001 v3 §7. " +
	"See docs/phase4-plan.md §3 P4-T-006 and docs/adr/0009-npu-dra-driver.md for the design."

// ClaimReconciler observes upstream resource.k8s.io/v1beta1.ResourceClaim
// objects and, for those referencing the npu-dra-driver DeviceClass family,
// records the AllocationDeferred annotations + emits a Kubernetes Event.
//
// Phase 4 performs NO real allocation (no status.allocation writes, no pod
// binding) — Phase 5+ per ADR-0001 v3 §7.
type ClaimReconciler struct {
	Client   client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// Reconcile implements the controller-runtime Reconciler contract.
//
// Behaviour matrix:
//   - Claim deleted (NotFound on Get): log cleanup, return without error.
//     Phase 4 holds no finalizer — there is nothing to clean up.
//   - Claim references a foreign driver class: ignore (no annotations
//     added, no event emitted).
//   - Claim references the npu-dra-driver class: set the AllocationDeferred
//     annotations on the claim metadata (idempotent — re-applying same
//     values is a no-op), emit a Kubernetes Event when a Recorder is
//     configured, and return without requeue.
func (r *ClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := log.FromContext(ctx).WithName("claim-controller").WithValues(
		"claim", req.NamespacedName, "task", "P4-T-006",
	)

	var claim resourceapi.ResourceClaim
	if err := r.Client.Get(ctx, req.NamespacedName, &claim); err != nil {
		if apierrors.IsNotFound(err) {
			lg.V(1).Info("ResourceClaim deleted; nothing to clean up (Phase 4 holds no finalizer)")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !claimRequestsOurDriver(claim) {
		lg.V(1).Info("Ignoring claim — not requesting npu-dra-driver class",
			"requests", deviceClassNames(claim))
		return ctrl.Result{}, nil
	}

	// Patch claim metadata.annotations to set the AllocationDeferred state.
	// Use Patch (not Update) so concurrent mutations of other annotation
	// keys (e.g. by inference-operator, kubectl edit) don't get clobbered.
	base := claim.DeepCopy()
	if claim.Annotations == nil {
		claim.Annotations = map[string]string{}
	}
	claim.Annotations[AnnotationAllocationDeferred] = "true"
	claim.Annotations[AnnotationAllocationDeferredReason] = ReasonPhase4Skeleton
	claim.Annotations[AnnotationAllocationDeferredMessage] = MessagePhase4Skeleton
	claim.Annotations[AnnotationAllocationDeferredObservedGen] = strconv.FormatInt(claim.Generation, 10)

	if err := r.Client.Patch(ctx, &claim, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, err
	}

	if r.Recorder != nil {
		r.Recorder.Event(&claim, corev1.EventTypeNormal,
			ReasonPhase4Skeleton, MessagePhase4Skeleton)
	}

	lg.Info("Allocation deferred (Phase 4 skeleton)",
		"requests", deviceClassNames(claim),
		"generation", claim.Generation,
		"condition", ConditionAllocationDeferred)

	return ctrl.Result{}, nil
}

// SetupWithManager registers this Reconciler with the manager. Watches
// upstream resource.k8s.io/v1beta1.ResourceClaim — controller-runtime's
// builder constructs the informer + workqueue.
func (r *ClaimReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&resourceapi.ResourceClaim{}).
		Named("npu-dra-claim").
		Complete(r)
}

// claimRequestsOurDriver returns true when the claim references the
// npu-dra-driver class family. Phase 4 matches by DeviceClassName prefix
// because the DeviceClass registration itself lands Phase 5. The expected
// prefix is v1alpha1.DriverName.
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
	// Exact match or sub-class match — both supported so Phase 5 can
	// register either a single bare class ("npu.ocloud.edge.example.com")
	// or sub-classes (".../whole", ".dynamic", etc.).
	return name == v1alpha1.DriverName ||
		strings.HasPrefix(name, v1alpha1.DriverName+"/") ||
		strings.HasPrefix(name, v1alpha1.DriverName+".")
}

// deviceClassNames flattens the claim's per-request DeviceClassName fields
// to a slice for log emission.
func deviceClassNames(claim resourceapi.ResourceClaim) []string {
	out := make([]string, 0, len(claim.Spec.Devices.Requests))
	for _, req := range claim.Spec.Devices.Requests {
		out = append(out, req.DeviceClassName)
	}
	return out
}
