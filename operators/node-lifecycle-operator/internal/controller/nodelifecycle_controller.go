/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

// Package controller implements the NodeLifecycle Reconcile pattern per
// ADR-0003 v2 IMS-1 (Phase 10 P10-T-007).
//
// **Reconcile pattern**: ReconcileOnce(current, desired, observedNode) →
// next NodeLifecycleStatus + Conditions. Plain-Go function shape decouples
// state-machine logic from controller-runtime wiring. main.go controller-
// runtime manager integration deferred to Phase 11+ chart packaging when
// the node-lifecycle-operator deploys as a real K8s controller.
//
// This separation enables:
//   - State-machine unit tests without envtest / fake client setup
//   - Future ctrl.Reconciler implementation imports this package's
//     ReconcileOnce + state.NextState + condition helpers
//   - Cross-controller awareness (e.g. bare-metal-provisioning-operator
//     can call ReconcileOnce-style helpers to validate state transitions
//     it observes)
package controller

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tech88-art/O-Cloud/operators/node-lifecycle-operator/api/v1alpha1"
	"github.com/tech88-art/O-Cloud/operators/node-lifecycle-operator/internal/state"
)

// ConditionReady reports overall readiness · True when state == Available
// or Unlocked (workload-accepting).
const (
	ConditionReady              = "Ready"
	ConditionRebootRequired     = "RebootRequired"
	ConditionMaintenance        = "Maintenance"
	ConditionDegraded           = "Degraded"
	ConditionAdminLocked        = "AdminLocked"
	ConditionDiskPressure       = "DiskPressure"        // surfaced from Node.status
	ConditionNetworkUnavailable = "NetworkUnavailable"  // surfaced from Node.status
)

// ReconcileInput captures the inputs ReconcileOnce uses to compute the
// next observed state + condition set. Mirrors the data a ctrl.Reconciler
// would gather (spec.desiredState · status.state · linked Node observations)
// without taking a Kubernetes client dependency in this package.
type ReconcileInput struct {
	// Current is the NodeLifecycle object's current Spec + Status.
	Current *v1alpha1.NodeLifecycle

	// NodeReady mirrors `core/v1.Node.status.conditions[type=Ready].status`
	// — true if the linked K8s Node reports Ready=True.
	NodeReady bool

	// NodeDiskPressure mirrors `Node.status.conditions[type=DiskPressure]`.
	NodeDiskPressure bool

	// NodeNetworkUnavailable mirrors `Node.status.conditions[type=NetworkUnavailable]`.
	NodeNetworkUnavailable bool

	// Now is the reconcile timestamp · injected for testability.
	Now time.Time
}

// ReconcileOutput captures the desired post-reconcile Status. Caller (a
// ctrl.Reconciler or test) writes Current.Status = Output.Status and
// returns ctrl.Result.
type ReconcileOutput struct {
	NextState  v1alpha1.NodeLifecycleState
	Conditions []metav1.Condition
	// RequeueAfter mirrors ctrl.Result.RequeueAfter · zero means no requeue.
	RequeueAfter time.Duration
}

// ReconcileOnce computes the next state + Conditions for a single
// reconcile pass per the ADR-0003 v2 state machine. Pure function: no I/O,
// no controller-runtime imports. ctrl.Reconciler wiring (in main.go ·
// deferred to chart packaging) handles fetching Node objects + writing
// Status back to API server.
//
// Logic per ADR-0003 v2:
//  1. If Spec.DesiredState differs from Status.State and is reachable
//     via state.NextState, advance one step (multi-hop transitions take
//     multiple reconcile passes).
//  2. Observe linked Node conditions and surface them as NodeLifecycle
//     Conditions (Ready / DiskPressure / NetworkUnavailable).
//  3. If Node.Ready==false and current state is Available, auto-transition
//     to Unavailable (passive degradation observation).
//  4. Emit Maintenance + AdminLocked conditions per state.
func ReconcileOnce(in ReconcileInput) ReconcileOutput {
	out := ReconcileOutput{
		NextState: in.Current.Status.State,
	}

	currentState := in.Current.Status.State
	if currentState == "" {
		// First-pass · default to Provisioning entry state.
		currentState = v1alpha1.NodeStateProvisioning
	}

	// Rule (3): auto-transition to Unavailable when Node loses readiness.
	if currentState == v1alpha1.NodeStateAvailable && !in.NodeReady {
		out.NextState = v1alpha1.NodeStateUnavailable
	} else if currentState == v1alpha1.NodeStateUnavailable && in.NodeReady {
		// Auto-recovery: Unavailable → Available when Node Ready=True.
		out.NextState = v1alpha1.NodeStateAvailable
	} else if in.Current.Spec.DesiredState != "" && in.Current.Spec.DesiredState != currentState {
		// Rule (1): advance toward DesiredState.
		next := state.NextState(currentState, in.Current.Spec.DesiredState)
		if next != "" {
			out.NextState = next
		}
	}

	// Rule (2): surface Node conditions.
	out.Conditions = buildConditions(in, out.NextState)

	// Requeue cadence · 30s in steady state, faster (5s) when actively
	// transitioning or in degraded states.
	if out.NextState == in.Current.Status.State {
		out.RequeueAfter = 30 * time.Second
	} else {
		out.RequeueAfter = 5 * time.Second
	}
	return out
}

// buildConditions assembles the per-pass NodeLifecycle Status.Conditions
// slice. 5-7 condition types per ADR-0003 v2 scaffold acceptance.
func buildConditions(in ReconcileInput, nextState v1alpha1.NodeLifecycleState) []metav1.Condition {
	now := metav1.Time{Time: in.Now}
	conds := []metav1.Condition{
		condition(ConditionReady, in.NodeReady, now, "NodeReady", "linked K8s Node reports Ready=True"),
		condition(ConditionDiskPressure, in.NodeDiskPressure, now, "NodeDiskPressure", "linked Node reports DiskPressure"),
		condition(ConditionNetworkUnavailable, in.NodeNetworkUnavailable, now, "NodeNetworkUnavailable", "linked Node reports NetworkUnavailable"),
		condition(ConditionRebootRequired, nextState == v1alpha1.NodeStateRebootRequired, now, "RebootPending", "node lifecycle in RebootRequired state"),
		condition(ConditionAdminLocked, nextState == v1alpha1.NodeStateLocked, now, "AdminLocked", "administratively locked from workloads"),
		condition(ConditionDegraded, nextState == v1alpha1.NodeStateDegradedAvailable, now, "DegradedResources", "node operating with degraded resources"),
		condition(ConditionMaintenance, in.Current.Spec.MaintenanceWindow != nil, now, "MaintenanceWindowConfigured", "spec.maintenanceWindow set"),
	}
	return conds
}

// condition builds a metav1.Condition with True/False status from a boolean.
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
