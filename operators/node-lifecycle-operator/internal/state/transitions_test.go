/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

package state

import (
	"testing"

	"github.com/tech88-art/O-Cloud/operators/node-lifecycle-operator/api/v1alpha1"
)

func TestIsPermittedHappyPath(t *testing.T) {
	cases := []struct {
		from, to v1alpha1.NodeLifecycleState
		want     bool
	}{
		{v1alpha1.NodeStateProvisioning, v1alpha1.NodeStateBootstrap, true},
		{v1alpha1.NodeStateBootstrap, v1alpha1.NodeStateAvailable, true},
		{v1alpha1.NodeStateAvailable, v1alpha1.NodeStateDegradedAvailable, true},
		{v1alpha1.NodeStateDegradedAvailable, v1alpha1.NodeStateAvailable, true},
		{v1alpha1.NodeStateAvailable, v1alpha1.NodeStateLocked, true},
		{v1alpha1.NodeStateLocked, v1alpha1.NodeStateUnlocked, true},
		{v1alpha1.NodeStateAvailable, v1alpha1.NodeStateRebootRequired, true},
		{v1alpha1.NodeStateRebootRequired, v1alpha1.NodeStateBootstrap, true},
	}
	for _, c := range cases {
		if got := IsPermitted(c.from, c.to); got != c.want {
			t.Errorf("IsPermitted(%s, %s) = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}

func TestIsPermittedRejectsArbitraryJump(t *testing.T) {
	// Provisioning → Available skipping Bootstrap should NOT be permitted.
	if IsPermitted(v1alpha1.NodeStateProvisioning, v1alpha1.NodeStateAvailable) {
		t.Fatal("Provisioning → Available should NOT be permitted (must pass through Bootstrap)")
	}
	// Locked → Available skipping Unlocked should NOT be permitted.
	if IsPermitted(v1alpha1.NodeStateLocked, v1alpha1.NodeStateAvailable) {
		t.Fatal("Locked → Available should NOT be permitted (must pass through Unlocked)")
	}
}

func TestIsPermittedIdentity(t *testing.T) {
	for _, s := range []v1alpha1.NodeLifecycleState{
		v1alpha1.NodeStateAvailable,
		v1alpha1.NodeStateLocked,
		v1alpha1.NodeStateRebootRequired,
	} {
		if !IsPermitted(s, s) {
			t.Errorf("IsPermitted(%s, %s) identity should be true", s, s)
		}
	}
}

func TestNextStateMultiHop(t *testing.T) {
	// Provisioning → Available requires Provisioning → Bootstrap → Available
	got := NextState(v1alpha1.NodeStateProvisioning, v1alpha1.NodeStateAvailable)
	if got != v1alpha1.NodeStateBootstrap {
		t.Fatalf("NextState(Provisioning, Available) = %s, want Bootstrap (intermediate)", got)
	}
}

func TestNextStateDirect(t *testing.T) {
	got := NextState(v1alpha1.NodeStateBootstrap, v1alpha1.NodeStateAvailable)
	if got != v1alpha1.NodeStateAvailable {
		t.Fatalf("NextState(Bootstrap, Available) = %s, want Available (direct)", got)
	}
}

func TestNextStateImpossible(t *testing.T) {
	// Locked → Provisioning is not in the matrix and has no intermediate.
	got := NextState(v1alpha1.NodeStateLocked, v1alpha1.NodeStateProvisioning)
	if got != "" {
		t.Fatalf("NextState(Locked, Provisioning) = %q, want \"\" (no path)", got)
	}
}

func TestMatrixCoverage(t *testing.T) {
	// 12 transitions per ADR-0003 v2 + this package doc comment.
	if got := len(Matrix); got != 12 {
		t.Fatalf("Matrix len = %d, want 12 (ADR-0003 v2 IMS-1 transition count)", got)
	}
}
