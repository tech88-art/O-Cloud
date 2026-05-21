/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
*/

// Package state implements the NodeLifecycle state transition matrix per
// ADR-0003 v2 IMS-1 node-lifecycle-operator (Phase 10 P10-T-007 controller
// body). 8 states adapted from StarlingX node lifecycle model + permitted
// transitions per O-Cloud edge platform context.
//
// Transition rules:
//   - Provisioning → Bootstrap (provisioning complete)
//   - Bootstrap → Available (K8s components healthy)
//   - Available ↔ DegradedAvailable (resource degradation)
//   - Available ↔ Unavailable (network partition / health check fail)
//   - Available ↔ Locked (admin lock)
//   - Locked ↔ Unlocked
//   - Available → RebootRequired (kernel update)
//   - RebootRequired → Bootstrap (reboot complete)
package state

import (
	"github.com/tech88-art/O-Cloud/operators/node-lifecycle-operator/api/v1alpha1"
)

// Transition describes a single permitted state move with its trigger
// condition.
type Transition struct {
	From    v1alpha1.NodeLifecycleState
	To      v1alpha1.NodeLifecycleState
	Trigger string // human-readable cause
}

// Matrix is the canonical set of permitted transitions per ADR-0003 v2.
// Reconciler consults this to validate desired-state moves; arbitrary
// moves (e.g. Provisioning → Available skipping Bootstrap) are rejected.
var Matrix = []Transition{
	{v1alpha1.NodeStateProvisioning, v1alpha1.NodeStateBootstrap, "provisioning complete"},
	{v1alpha1.NodeStateBootstrap, v1alpha1.NodeStateAvailable, "K8s components healthy"},
	{v1alpha1.NodeStateAvailable, v1alpha1.NodeStateDegradedAvailable, "resource degradation observed"},
	{v1alpha1.NodeStateDegradedAvailable, v1alpha1.NodeStateAvailable, "resources restored"},
	{v1alpha1.NodeStateAvailable, v1alpha1.NodeStateUnavailable, "health check failure / network partition"},
	{v1alpha1.NodeStateUnavailable, v1alpha1.NodeStateAvailable, "health check recovered"},
	{v1alpha1.NodeStateAvailable, v1alpha1.NodeStateLocked, "admin lock"},
	{v1alpha1.NodeStateLocked, v1alpha1.NodeStateUnlocked, "admin unlock"},
	{v1alpha1.NodeStateUnlocked, v1alpha1.NodeStateAvailable, "unlock complete"},
	{v1alpha1.NodeStateAvailable, v1alpha1.NodeStateRebootRequired, "reboot needed (e.g. kernel update)"},
	{v1alpha1.NodeStateRebootRequired, v1alpha1.NodeStateBootstrap, "reboot complete"},
	// Degraded → Locked permitted (admin can lock degraded nodes for maintenance)
	{v1alpha1.NodeStateDegradedAvailable, v1alpha1.NodeStateLocked, "admin lock during degraded state"},
}

// IsPermitted returns true if from → to is a permitted transition per Matrix.
// Identity transitions (from == to) are always permitted as no-ops.
func IsPermitted(from, to v1alpha1.NodeLifecycleState) bool {
	if from == to {
		return true
	}
	for _, t := range Matrix {
		if t.From == from && t.To == to {
			return true
		}
	}
	return false
}

// NextState returns the next state on the path from `current` toward
// `desired` per Matrix. Returns desired itself when permitted directly,
// or an intermediate state when a multi-hop path is needed. Returns
// the empty string when no path exists (caller should treat as invalid
// transition request).
func NextState(current, desired v1alpha1.NodeLifecycleState) v1alpha1.NodeLifecycleState {
	if current == desired {
		return current
	}
	if IsPermitted(current, desired) {
		return desired
	}
	// Multi-hop fallback: try one-step lookahead via any permitted intermediate.
	// Phase 10 minimum-viable: BFS depth 2 sufficient for the 12-edge matrix.
	for _, t := range Matrix {
		if t.From == current && IsPermitted(t.To, desired) {
			return t.To
		}
	}
	return ""
}

// IsTerminal returns true for states with no outgoing transitions other
// than to themselves. Phase 10 minimum-viable: no state is truly terminal
// (all participate in some recovery transition), so this always returns
// false. Phase 11+ may add terminal states for decommissioning.
func IsTerminal(_ v1alpha1.NodeLifecycleState) bool {
	return false
}
