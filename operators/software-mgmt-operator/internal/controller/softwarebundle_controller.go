/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

// Package controller implements the SoftwareBundle Reconcile pattern per
// ADR-0003 v2 IMS-2 (Phase 10 P10-T-008).
//
// Same pure-Go Reconcile pattern as node-lifecycle-operator (P10-T-007):
// ReconcileOnce(in) → out decouples rollout strategy logic from controller-
// runtime wiring. Phase 11+ chart packaging adds ctrl.Reconciler that imports
// this package + internal/rollout.
package controller

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tech88-art/O-Cloud/operators/software-mgmt-operator/api/v1alpha1"
	"github.com/tech88-art/O-Cloud/operators/software-mgmt-operator/internal/rollout"
)

const (
	ConditionProgressing = "Progressing"
	ConditionCompleted   = "Completed"
	ConditionFailed      = "Failed"
	ConditionDegraded    = "Degraded"
)

// ReconcileInput captures the inputs for one SoftwareBundle reconcile pass.
type ReconcileInput struct {
	// Current is the SoftwareBundle's current Spec + Status.
	Current *v1alpha1.SoftwareBundle

	// AllTargetNodes is the resolved node list per Spec.NodeSelector.
	// Phase 11+ ctrl.Reconciler queries K8s Nodes; this pure function
	// takes the resolved list directly.
	AllTargetNodes []string

	// AppliedNodes is the set of nodes that completed rollout (from
	// Status or external observation).
	AppliedNodes []string

	// InProgressNodes is the set currently mid-rollout.
	InProgressNodes []string

	// FailedNodes is the set that failed rollout.
	FailedNodes []string

	// Now is the reconcile timestamp (testability).
	Now time.Time
}

// ReconcileOutput captures the desired post-reconcile Status update.
type ReconcileOutput struct {
	NextStatus   v1alpha1.SoftwareBundleStatus
	Plan         rollout.Plan
	RequeueAfter time.Duration
}

// ReconcileOnce computes the next Status + rollout batch per ADR-0003 v2
// IMS-2 state machine. Pure function: callers (ctrl.Reconciler or test)
// pass observation inputs, receive next-state + batch decisions.
func ReconcileOnce(in ReconcileInput) ReconcileOutput {
	out := ReconcileOutput{}

	// Compute next rollout batch per strategy.
	plan, err := rollout.NextBatch(
		in.Current.Spec.RolloutPolicy.Strategy,
		in.Current.Spec.RolloutPolicy.MaxUnavailable,
		in.AllTargetNodes,
		in.AppliedNodes,
		in.InProgressNodes,
	)
	out.Plan = plan

	// Compose Status.
	status := v1alpha1.SoftwareBundleStatus{
		TargetNodeCount:  int32(len(in.AllTargetNodes)),
		AppliedNodeCount: int32(len(in.AppliedNodes)),
		FailedNodeCount:  int32(len(in.FailedNodes)),
	}
	if rollout.IsComplete(in.AllTargetNodes, in.AppliedNodes, in.InProgressNodes) {
		status.AppliedVersion = in.Current.Spec.Version
	}

	// Conditions emission.
	now := metav1.Time{Time: in.Now}
	progressing := len(in.InProgressNodes) > 0 || (err == nil && len(plan.NodesToTarget) > 0)
	completed := rollout.IsComplete(in.AllTargetNodes, in.AppliedNodes, in.InProgressNodes) && len(in.FailedNodes) == 0
	failed := len(in.FailedNodes) > 0
	degraded := len(in.FailedNodes) > 0 && len(in.AppliedNodes) > 0

	status.Conditions = []metav1.Condition{
		conditionBool(ConditionProgressing, progressing, now, "RolloutInFlight", "rollout batch in progress or pending"),
		conditionBool(ConditionCompleted, completed, now, "RolloutComplete", "all target nodes applied successfully"),
		conditionBool(ConditionFailed, failed, now, "RolloutFailures", "one or more nodes failed rollout"),
		conditionBool(ConditionDegraded, degraded, now, "PartialRollout", "rollout partially succeeded with failures"),
	}
	out.NextStatus = status

	// Requeue cadence.
	if completed {
		out.RequeueAfter = 0 // no further reconcile needed
	} else if progressing {
		out.RequeueAfter = 5 * time.Second
	} else {
		out.RequeueAfter = 30 * time.Second
	}
	return out
}

func conditionBool(condType string, ok bool, now metav1.Time, reason, msg string) metav1.Condition {
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
