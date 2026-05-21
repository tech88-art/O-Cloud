/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

package state

import (
	"testing"

	"github.com/tech88-art/O-Cloud/operators/bare-metal-provisioning-operator/api/v1alpha1"
)

func TestProvisioningHappyPath(t *testing.T) {
	cases := []struct{ from, to v1alpha1.ProvisioningState }{
		{v1alpha1.BMStateInspecting, v1alpha1.BMStateRegistering},
		{v1alpha1.BMStateRegistering, v1alpha1.BMStateProvisioning},
		{v1alpha1.BMStateProvisioning, v1alpha1.BMStateProvisioned},
		{v1alpha1.BMStateProvisioned, v1alpha1.BMStateReady},
	}
	for _, c := range cases {
		if !IsPermitted(c.from, c.to) {
			t.Errorf("happy %s → %s not permitted", c.from, c.to)
		}
	}
}

func TestProvisioningErrorFromAnyState(t *testing.T) {
	froms := []v1alpha1.ProvisioningState{
		v1alpha1.BMStateInspecting,
		v1alpha1.BMStateRegistering,
		v1alpha1.BMStateProvisioning,
		v1alpha1.BMStateProvisioned,
		v1alpha1.BMStateReady,
		v1alpha1.BMStateDeprovisioning,
	}
	for _, f := range froms {
		if !IsPermitted(f, v1alpha1.BMStateError) {
			t.Errorf("%s → Error must be permitted", f)
		}
	}
}

func TestProvisioningErrorRecovery(t *testing.T) {
	if !IsPermitted(v1alpha1.BMStateError, v1alpha1.BMStateInspecting) {
		t.Fatal("Error → Inspecting (retry) must be permitted")
	}
}

func TestProvisioningRejectsSkip(t *testing.T) {
	// Inspecting → Provisioning skipping Registering must NOT be permitted.
	if IsPermitted(v1alpha1.BMStateInspecting, v1alpha1.BMStateProvisioning) {
		t.Fatal("Inspecting → Provisioning should NOT skip Registering")
	}
}

func TestNextStateMultiHopOneStep(t *testing.T) {
	// Inspecting → Provisioning is 2-hop: requires Inspecting → Registering → Provisioning.
	// NextState 1-hop lookahead returns the intermediate state (Registering).
	got := NextState(v1alpha1.BMStateInspecting, v1alpha1.BMStateProvisioning)
	if got != v1alpha1.BMStateRegistering {
		t.Fatalf("Inspecting → Provisioning next (1-hop) = %s, want Registering", got)
	}
}

func TestNextStateBeyondLookaheadDepth(t *testing.T) {
	// Inspecting → Ready is 4-hop (Inspecting → Registering → Provisioning →
	// Provisioned → Ready). 1-hop lookahead can't find this path · returns "".
	// Reconciler will advance one step per pass · multi-pass progression.
	got := NextState(v1alpha1.BMStateInspecting, v1alpha1.BMStateReady)
	if got != "" {
		t.Fatalf("Inspecting → Ready (4-hop) NextState = %s, want \"\" (beyond 1-hop lookahead)", got)
	}
}

func TestNextStateDeprovision(t *testing.T) {
	got := NextState(v1alpha1.BMStateReady, v1alpha1.BMStateDeprovisioning)
	if got != v1alpha1.BMStateDeprovisioning {
		t.Fatalf("Ready → Deprovisioning next = %s, want Deprovisioning", got)
	}
}

func TestMatrixCoverage(t *testing.T) {
	// 13 transitions per package doc (6 forward + 6 error + 1 recovery).
	if got := len(Matrix); got != 13 {
		t.Fatalf("Matrix len = %d, want 13 (ADR-0003 v2 IMS-3 transition count)", got)
	}
}
