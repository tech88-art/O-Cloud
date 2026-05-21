/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

// Package controller implements the BareMetalNode Reconcile pattern per
// ADR-0003 v2 IMS-3 (Phase 10 P10-T-101).
package controller

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tech88-art/O-Cloud/operators/bare-metal-provisioning-operator/api/v1alpha1"
	"github.com/tech88-art/O-Cloud/operators/bare-metal-provisioning-operator/internal/state"
)

const (
	ConditionBMCReachable   = "BMCReachable"
	ConditionImagePulled    = "ImagePulled"
	ConditionInspectionDone = "InspectionDone"
	ConditionReady          = "Ready"
	ConditionError          = "Error"
)

type ReconcileInput struct {
	Current        *v1alpha1.BareMetalNode
	BMCReachable   bool
	ImagePulled    bool
	InspectionDone bool
	NodeJoined     bool
	BMCError       string // non-empty → flip to Error state
	Now            time.Time
}

type ReconcileOutput struct {
	NextState    v1alpha1.ProvisioningState
	Conditions   []metav1.Condition
	RequeueAfter time.Duration
}

// ReconcileOnce computes the next provisioning state + Conditions for a
// single reconcile pass per ADR-0003 v2 IMS-3 state machine.
func ReconcileOnce(in ReconcileInput) ReconcileOutput {
	out := ReconcileOutput{
		NextState: in.Current.Status.ProvisioningState,
	}

	current := in.Current.Status.ProvisioningState
	if current == "" {
		current = v1alpha1.BMStateInspecting
	}

	// BMC error path · short-circuit to Error from any state.
	if in.BMCError != "" && current != v1alpha1.BMStateError {
		out.NextState = v1alpha1.BMStateError
	} else if current == v1alpha1.BMStateError && in.BMCError == "" {
		// Error recovery · admin cleared error or BMC came back.
		out.NextState = v1alpha1.BMStateInspecting
	} else {
		// Normal progression per observations.
		switch current {
		case v1alpha1.BMStateInspecting:
			if in.BMCReachable && in.InspectionDone {
				out.NextState = v1alpha1.BMStateRegistering
			}
		case v1alpha1.BMStateRegistering:
			out.NextState = v1alpha1.BMStateProvisioning
		case v1alpha1.BMStateProvisioning:
			if in.ImagePulled {
				out.NextState = v1alpha1.BMStateProvisioned
			}
		case v1alpha1.BMStateProvisioned:
			if in.NodeJoined {
				out.NextState = v1alpha1.BMStateReady
			}
		case v1alpha1.BMStateReady:
			if in.Current.Spec.DesiredState == v1alpha1.BMStateDeprovisioning {
				out.NextState = v1alpha1.BMStateDeprovisioning
			}
		case v1alpha1.BMStateDeprovisioning:
			// Deprovisioning complete · cycle back to Inspecting
			// (operator removes finalizer + image artifact).
			out.NextState = v1alpha1.BMStateInspecting
		}
	}

	// If DesiredState is set and differs from observation-driven next,
	// try a state.NextState step toward it (per multi-hop transitions).
	if in.Current.Spec.DesiredState != "" && in.Current.Spec.DesiredState != out.NextState {
		if step := state.NextState(out.NextState, in.Current.Spec.DesiredState); step != "" {
			// Only override when observation hasn't already advanced.
			if out.NextState == current {
				out.NextState = step
			}
		}
	}

	// Validate transition · revert if illegal.
	if !state.IsPermitted(current, out.NextState) {
		out.NextState = current
	}

	now := metav1.Time{Time: in.Now}
	out.Conditions = []metav1.Condition{
		condition(ConditionBMCReachable, in.BMCReachable, now, "BMCContact", "BMC endpoint responsive"),
		condition(ConditionInspectionDone, in.InspectionDone, now, "InspectionCompleted", "hardware inventory obtained"),
		condition(ConditionImagePulled, in.ImagePulled, now, "ImagePulled", "OS image artifact pulled"),
		condition(ConditionReady, out.NextState == v1alpha1.BMStateReady, now, "NodeReady", "bare-metal node Ready"),
		condition(ConditionError, out.NextState == v1alpha1.BMStateError, now, "BMCFailure", in.BMCError),
	}

	// Requeue cadence:
	//   - Ready or Error · 60s (stable)
	//   - Otherwise · 10s (active provisioning)
	if out.NextState == v1alpha1.BMStateReady || out.NextState == v1alpha1.BMStateError {
		out.RequeueAfter = 60 * time.Second
	} else {
		out.RequeueAfter = 10 * time.Second
	}
	return out
}

func condition(condType string, ok bool, now metav1.Time, reason, msg string) metav1.Condition {
	st := metav1.ConditionFalse
	if ok {
		st = metav1.ConditionTrue
	}
	return metav1.Condition{
		Type:               condType,
		Status:             st,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            msg,
	}
}
