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
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	inferencev1alpha1 "github.com/tech88-art/O-Cloud/operators/inference-operator/api/v1alpha1"
)

// DefaultProgressDeadline is the wall-clock window the controller
// gives a ModelService to leave the Provisioning phase before
// promoting Status to Failed with reason=ProgressDeadlineExceeded.
// Plan acceptance: "ProgressDeadline Status=False when stuck
// Provisioning > 10min".
const DefaultProgressDeadline = 10 * time.Minute

// Condition reason strings.
const (
	reasonAllReady                = "AllReady"
	reasonProvisioningInProgress  = "ProvisioningInProgress"
	reasonProgressDeadlineHit     = "ProgressDeadlineExceeded"
	reasonImagePullError          = "ImagePullError"
	reasonAllocationReady         = "AllocationReady"
	reasonClaimsNotYetAllocated   = "ClaimsNotYetAllocated"
)

// PhaseInputs is the immutable snapshot of child state aggregated for
// one phase computation. T008 collects this in modelservice_controller
// .go reconcileChildren tail, then hands it to computePhase().
type PhaseInputs struct {
	// PrefillDesired / DecodeDesired are the spec'd replica counts.
	PrefillDesired int32
	DecodeDesired  int32

	// PrefillReady / DecodeReady are the per-side Deployment Status
	// .ReadyReplicas values (0 if Deployment missing or not yet
	// reconciled).
	PrefillReady int32
	DecodeReady  int32

	// PrefillProgressing / DecodeProgressing capture the
	// Deployment.Status.Conditions[Type=Progressing] state. Used to
	// detect image-pull failures + permanent stalls.
	PrefillProgressing *appsv1.DeploymentCondition
	DecodeProgressing  *appsv1.DeploymentCondition

	// ClaimsTotal / ClaimsAllocated are the count of per-replica
	// ResourceClaims K8s materialised from the templates and the
	// subset whose Status.Allocation has been populated by
	// npu-dra-driver T002.
	ClaimsTotal     int
	ClaimsAllocated int

	// Now is the wall-clock reference for the progress deadline
	// computation. Injected for tests.
	Now time.Time
}

// PhaseDecision is the output of computePhase(). The Reconciler uses
// it to drive Status.Phase + Conditions via a single status patch.
type PhaseDecision struct {
	Phase      inferencev1alpha1.ModelServicePhase
	Conditions []metav1.Condition
}

// computePhase is the pure phase machine — given PhaseInputs and the
// last-observed status, return the target phase + condition set. The
// Reconciler is responsible for applying.
func computePhase(in PhaseInputs, last *inferencev1alpha1.ModelServiceStatus) PhaseDecision {
	out := PhaseDecision{}

	// Allocation readiness:
	allocReady := in.ClaimsTotal > 0 && in.ClaimsAllocated == in.ClaimsTotal
	allocCond := metav1.Condition{Type: ConditionAllocationReady}
	switch {
	case allocReady:
		allocCond.Status = metav1.ConditionTrue
		allocCond.Reason = reasonAllocationReady
		allocCond.Message = "All per-replica ResourceClaims allocated"
	case in.ClaimsTotal == 0:
		allocCond.Status = metav1.ConditionFalse
		allocCond.Reason = reasonClaimsNotYetAllocated
		allocCond.Message = "No ResourceClaims yet observed for this ModelService"
	default:
		allocCond.Status = metav1.ConditionFalse
		allocCond.Reason = reasonClaimsNotYetAllocated
		allocCond.Message = "Some ResourceClaims not yet allocated by npu-dra-driver"
	}

	// Image-pull / progress-stall detection on either side.
	imageErr := detectImagePullError(in.PrefillProgressing, in.DecodeProgressing)

	// Replica-level readiness.
	pdReady := in.PrefillReady >= in.PrefillDesired && in.DecodeReady >= in.DecodeDesired

	// Progress deadline: how long the controller has been in non-Ready
	// state. Use last-observed Available condition's LastTransitionTime
	// as the baseline; if absent, assume "just now" (no deadline yet).
	stuckFor := durationSinceAvailableFalse(last, in.Now)

	availCond := metav1.Condition{Type: ConditionAvailable}
	pdCond := metav1.Condition{Type: "ProgressDeadline"}

	switch {
	case imageErr != "":
		out.Phase = inferencev1alpha1.PhaseFailed
		availCond.Status = metav1.ConditionFalse
		availCond.Reason = reasonImagePullError
		availCond.Message = imageErr
		pdCond.Status = metav1.ConditionFalse
		pdCond.Reason = reasonImagePullError
		pdCond.Message = "Image pull failure prevents Pod startup"

	case pdReady && allocReady:
		out.Phase = inferencev1alpha1.PhaseReady
		availCond.Status = metav1.ConditionTrue
		availCond.Reason = reasonAllReady
		availCond.Message = "Prefill + Decode Deployments fully ready; all claims allocated"
		pdCond.Status = metav1.ConditionFalse
		pdCond.Reason = reasonAllReady
		pdCond.Message = "Progress complete"

	case stuckFor > DefaultProgressDeadline:
		out.Phase = inferencev1alpha1.PhaseFailed
		availCond.Status = metav1.ConditionFalse
		availCond.Reason = reasonProgressDeadlineHit
		availCond.Message = "ProgressDeadlineExceeded: stuck Provisioning >10min"
		pdCond.Status = metav1.ConditionTrue
		pdCond.Reason = reasonProgressDeadlineHit
		pdCond.Message = "Progress deadline expired"

	default:
		out.Phase = inferencev1alpha1.PhaseProvisioning
		availCond.Status = metav1.ConditionFalse
		availCond.Reason = reasonProvisioningInProgress
		availCond.Message = "Provisioning Prefill + Decode pair"
		pdCond.Status = metav1.ConditionFalse
		pdCond.Reason = reasonProvisioningInProgress
		pdCond.Message = "Progress within deadline"
	}

	out.Conditions = []metav1.Condition{availCond, pdCond, allocCond}
	return out
}

// detectImagePullError returns a non-empty message string when either
// Deployment's Progressing condition reports an image-pull failure.
// Reads upstream Reason strings ("ImagePullBackOff" / "ErrImagePull")
// from the Progressing condition. Empty string when no error detected.
func detectImagePullError(p, d *appsv1.DeploymentCondition) string {
	for _, c := range []*appsv1.DeploymentCondition{p, d} {
		if c == nil {
			continue
		}
		if c.Status == corev1.ConditionFalse &&
			(strings.Contains(c.Reason, "ImagePull") || strings.Contains(c.Reason, "ErrImagePull")) {
			return "Deployment Progressing False: " + c.Reason + " — " + c.Message
		}
	}
	return ""
}

// durationSinceAvailableFalse returns the wall-clock distance between
// the last-observed Available=False transition and Now. Used to drive
// the ProgressDeadline. Returns 0 when no Available condition exists
// yet (first reconcile — no deadline pressure).
func durationSinceAvailableFalse(last *inferencev1alpha1.ModelServiceStatus, now time.Time) time.Duration {
	if last == nil {
		return 0
	}
	for _, c := range last.Conditions {
		if c.Type == ConditionAvailable && c.Status == metav1.ConditionFalse {
			return now.Sub(c.LastTransitionTime.Time)
		}
	}
	return 0
}

// countClaimsAllocated scans the ResourceClaim list and returns
// (total, allocated). Only counts claims whose
// `inference.ocloud.edge.example.com/model-service` label matches the
// ModelService — claims for other ModelServices in the same namespace
// are skipped.
//
// `allocated` is the subset that have non-nil Status.Allocation AND
// at least one entry whose Driver == NPUDeviceClassName.
func countClaimsAllocated(ms *inferencev1alpha1.ModelService, claims []resourceapi.ResourceClaim) (total, allocated int) {
	msRef := ms.Namespace + "/" + ms.Name
	for i := range claims {
		c := &claims[i]
		if c.Labels[LabelModelService] != msRef {
			continue
		}
		total++
		if c.Status.Allocation == nil {
			continue
		}
		hasOurDriver := false
		for _, res := range c.Status.Allocation.Devices.Results {
			if res.Driver == NPUDeviceClassName {
				hasOurDriver = true
				break
			}
		}
		if hasOurDriver {
			allocated++
		}
	}
	return total, allocated
}

// progressingCondition returns the first Progressing condition on the
// Deployment (or nil if absent). Helper for PhaseInputs assembly.
func progressingCondition(dep *appsv1.Deployment) *appsv1.DeploymentCondition {
	if dep == nil {
		return nil
	}
	for i := range dep.Status.Conditions {
		c := &dep.Status.Conditions[i]
		if c.Type == appsv1.DeploymentProgressing {
			return c
		}
	}
	return nil
}
