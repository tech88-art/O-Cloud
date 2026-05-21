/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tech88-art/O-Cloud/operators/node-lifecycle-operator/api/v1alpha1"
)

func newInput(currentState v1alpha1.NodeLifecycleState, desiredState v1alpha1.NodeLifecycleState, nodeReady bool) ReconcileInput {
	return ReconcileInput{
		Current: &v1alpha1.NodeLifecycle{
			Spec: v1alpha1.NodeLifecycleSpec{
				NodeName:     "worker-a",
				DesiredState: desiredState,
			},
			Status: v1alpha1.NodeLifecycleStatus{
				State: currentState,
			},
		},
		NodeReady: nodeReady,
		Now:       time.Now(),
	}
}

func TestReconcileAdvancesDesiredState(t *testing.T) {
	in := newInput(v1alpha1.NodeStateBootstrap, v1alpha1.NodeStateAvailable, true)
	out := ReconcileOnce(in)
	if out.NextState != v1alpha1.NodeStateAvailable {
		t.Fatalf("Bootstrap + DesiredState=Available → NextState = %s, want Available", out.NextState)
	}
}

func TestReconcileAutoTransitionsToUnavailable(t *testing.T) {
	in := newInput(v1alpha1.NodeStateAvailable, v1alpha1.NodeStateAvailable, false) // Node not ready
	out := ReconcileOnce(in)
	if out.NextState != v1alpha1.NodeStateUnavailable {
		t.Fatalf("Available + Node not Ready → NextState = %s, want Unavailable", out.NextState)
	}
}

func TestReconcileAutoRecoversToAvailable(t *testing.T) {
	in := newInput(v1alpha1.NodeStateUnavailable, v1alpha1.NodeStateAvailable, true) // Node restored
	out := ReconcileOnce(in)
	if out.NextState != v1alpha1.NodeStateAvailable {
		t.Fatalf("Unavailable + Node Ready → NextState = %s, want Available", out.NextState)
	}
}

func TestReconcileMultiHopUsesIntermediate(t *testing.T) {
	in := newInput(v1alpha1.NodeStateProvisioning, v1alpha1.NodeStateAvailable, true)
	out := ReconcileOnce(in)
	// Provisioning → Available requires intermediate Bootstrap step.
	if out.NextState != v1alpha1.NodeStateBootstrap {
		t.Fatalf("Provisioning + DesiredState=Available → NextState = %s, want Bootstrap (intermediate)", out.NextState)
	}
}

func TestReconcileFirstPassDefaultsToProvisioning(t *testing.T) {
	in := newInput("", v1alpha1.NodeStateAvailable, true) // empty current state
	out := ReconcileOnce(in)
	// First pass advances from default Provisioning → Bootstrap
	if out.NextState != v1alpha1.NodeStateBootstrap {
		t.Fatalf("empty State + DesiredState=Available → NextState = %s, want Bootstrap", out.NextState)
	}
}

func TestReconcileConditionsEmit(t *testing.T) {
	in := newInput(v1alpha1.NodeStateAvailable, v1alpha1.NodeStateAvailable, true)
	in.NodeDiskPressure = true
	out := ReconcileOnce(in)
	if len(out.Conditions) != 7 {
		t.Fatalf("Conditions count = %d, want 7 (per ADR-0003 v2)", len(out.Conditions))
	}
	// Find DiskPressure condition.
	var disk *metav1.Condition
	for i := range out.Conditions {
		if out.Conditions[i].Type == ConditionDiskPressure {
			disk = &out.Conditions[i]
			break
		}
	}
	if disk == nil {
		t.Fatal("DiskPressure condition not emitted")
	}
	if disk.Status != metav1.ConditionTrue {
		t.Fatalf("DiskPressure status = %s, want True (input NodeDiskPressure=true)", disk.Status)
	}
}

func TestReconcileRequeueCadenceFasterDuringTransition(t *testing.T) {
	transit := newInput(v1alpha1.NodeStateBootstrap, v1alpha1.NodeStateAvailable, true)
	steady := newInput(v1alpha1.NodeStateAvailable, v1alpha1.NodeStateAvailable, true)

	outT := ReconcileOnce(transit)
	outS := ReconcileOnce(steady)
	if outT.RequeueAfter >= outS.RequeueAfter {
		t.Fatalf("transit RequeueAfter = %v should be < steady RequeueAfter = %v", outT.RequeueAfter, outS.RequeueAfter)
	}
}

func TestReconcileMaintenanceWindowSurfacesCondition(t *testing.T) {
	in := newInput(v1alpha1.NodeStateAvailable, v1alpha1.NodeStateAvailable, true)
	mw := &v1alpha1.MaintenanceWindow{
		Start:           metav1.Time{Time: time.Now().Add(1 * time.Hour)},
		DurationSeconds: 600,
	}
	in.Current.Spec.MaintenanceWindow = mw

	out := ReconcileOnce(in)
	// Find Maintenance condition.
	for _, c := range out.Conditions {
		if c.Type == ConditionMaintenance && c.Status == metav1.ConditionTrue {
			return // pass
		}
	}
	t.Fatal("Maintenance condition not emitted as True when MaintenanceWindow set")
}
