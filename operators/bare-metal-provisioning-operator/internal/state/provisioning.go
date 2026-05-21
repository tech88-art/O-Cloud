/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

// Package state implements the BareMetalNode provisioning state machine per
// ADR-0003 v2 IMS-3 (Phase 10 P10-T-101).
//
// 7 states adapted from Metal3 / cluster-api BareMetalHost pattern:
//   Inspecting → Registering → Provisioning → Provisioned → Ready
//   any state → Error (BMC unreachable / failure)
//   Ready → Deprovisioning → (back to Inspecting OR end)
package state

import (
	"github.com/tech88-art/O-Cloud/operators/bare-metal-provisioning-operator/api/v1alpha1"
)

type Transition struct {
	From    v1alpha1.ProvisioningState
	To      v1alpha1.ProvisioningState
	Trigger string
}

// Matrix lists the permitted transitions per ADR-0003 v2 IMS-3.
var Matrix = []Transition{
	{v1alpha1.BMStateInspecting, v1alpha1.BMStateRegistering, "BMC inspection complete · hardware inventory obtained"},
	{v1alpha1.BMStateRegistering, v1alpha1.BMStateProvisioning, "registered in inventory · image write start"},
	{v1alpha1.BMStateProvisioning, v1alpha1.BMStateProvisioned, "OS image written successfully"},
	{v1alpha1.BMStateProvisioned, v1alpha1.BMStateReady, "node booted + K8s joined"},
	{v1alpha1.BMStateReady, v1alpha1.BMStateDeprovisioning, "deprovision request"},
	{v1alpha1.BMStateDeprovisioning, v1alpha1.BMStateInspecting, "OS removed · cycle back to Inspecting"},
	// Error transitions — any non-error state can flip to Error.
	{v1alpha1.BMStateInspecting, v1alpha1.BMStateError, "BMC unreachable"},
	{v1alpha1.BMStateRegistering, v1alpha1.BMStateError, "registration failure"},
	{v1alpha1.BMStateProvisioning, v1alpha1.BMStateError, "image write failure"},
	{v1alpha1.BMStateProvisioned, v1alpha1.BMStateError, "boot failure"},
	{v1alpha1.BMStateReady, v1alpha1.BMStateError, "Node unhealthy"},
	{v1alpha1.BMStateDeprovisioning, v1alpha1.BMStateError, "deprovision failure"},
	// Error recovery → restart from Inspecting (operator clears error condition).
	{v1alpha1.BMStateError, v1alpha1.BMStateInspecting, "operator retry · clear error"},
}

func IsPermitted(from, to v1alpha1.ProvisioningState) bool {
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

func NextState(current, desired v1alpha1.ProvisioningState) v1alpha1.ProvisioningState {
	if current == desired {
		return current
	}
	if IsPermitted(current, desired) {
		return desired
	}
	// 1-hop lookahead.
	for _, t := range Matrix {
		if t.From == current && IsPermitted(t.To, desired) {
			return t.To
		}
	}
	return ""
}
