/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tech88-art/O-Cloud/operators/bare-metal-provisioning-operator/api/v1alpha1"
)

func newInput(current v1alpha1.ProvisioningState, desired v1alpha1.ProvisioningState) ReconcileInput {
	return ReconcileInput{
		Current: &v1alpha1.BareMetalNode{
			Spec: v1alpha1.BareMetalNodeSpec{
				BMC: v1alpha1.BMCSpec{
					Address: "redfish+https://bmc.example.com",
					CredentialsRef: v1alpha1.CredentialsReference{
						Name:      "bmc-creds",
						Namespace: "ocloud-system",
					},
				},
				DesiredState: desired,
			},
			Status: v1alpha1.BareMetalNodeStatus{
				ProvisioningState: current,
			},
		},
		Now: time.Now(),
	}
}

func TestReconcileInspectingAdvancesWhenBMCReachable(t *testing.T) {
	in := newInput(v1alpha1.BMStateInspecting, v1alpha1.BMStateReady)
	in.BMCReachable = true
	in.InspectionDone = true
	out := ReconcileOnce(in)
	if out.NextState != v1alpha1.BMStateRegistering {
		t.Fatalf("NextState = %s, want Registering", out.NextState)
	}
}

func TestReconcileProvisioningAdvancesWhenImagePulled(t *testing.T) {
	in := newInput(v1alpha1.BMStateProvisioning, v1alpha1.BMStateReady)
	in.BMCReachable = true
	in.ImagePulled = true
	out := ReconcileOnce(in)
	if out.NextState != v1alpha1.BMStateProvisioned {
		t.Fatalf("NextState = %s, want Provisioned", out.NextState)
	}
}

func TestReconcileProvisionedAdvancesWhenNodeJoined(t *testing.T) {
	in := newInput(v1alpha1.BMStateProvisioned, v1alpha1.BMStateReady)
	in.BMCReachable = true
	in.NodeJoined = true
	out := ReconcileOnce(in)
	if out.NextState != v1alpha1.BMStateReady {
		t.Fatalf("NextState = %s, want Ready", out.NextState)
	}
}

func TestReconcileBMCErrorFlipsToError(t *testing.T) {
	in := newInput(v1alpha1.BMStateProvisioning, v1alpha1.BMStateReady)
	in.BMCError = "BMC connection timeout"
	out := ReconcileOnce(in)
	if out.NextState != v1alpha1.BMStateError {
		t.Fatalf("NextState = %s, want Error (BMCError set)", out.NextState)
	}
}

func TestReconcileErrorRecovery(t *testing.T) {
	in := newInput(v1alpha1.BMStateError, v1alpha1.BMStateReady)
	// BMCError cleared (admin retry).
	out := ReconcileOnce(in)
	if out.NextState != v1alpha1.BMStateInspecting {
		t.Fatalf("Error + cleared BMCError → NextState = %s, want Inspecting", out.NextState)
	}
}

func TestReconcileDeprovisionTrigger(t *testing.T) {
	in := newInput(v1alpha1.BMStateReady, v1alpha1.BMStateDeprovisioning)
	in.BMCReachable = true
	out := ReconcileOnce(in)
	if out.NextState != v1alpha1.BMStateDeprovisioning {
		t.Fatalf("Ready + DesiredState=Deprovisioning → NextState = %s, want Deprovisioning", out.NextState)
	}
}

func TestReconcileDeprovisionedCyclesToInspecting(t *testing.T) {
	in := newInput(v1alpha1.BMStateDeprovisioning, v1alpha1.BMStateInspecting)
	in.BMCReachable = true
	out := ReconcileOnce(in)
	if out.NextState != v1alpha1.BMStateInspecting {
		t.Fatalf("Deprovisioning → NextState = %s, want Inspecting (cycle)", out.NextState)
	}
}

func TestReconcileConditionsEmit(t *testing.T) {
	in := newInput(v1alpha1.BMStateReady, v1alpha1.BMStateReady)
	in.BMCReachable = true
	in.InspectionDone = true
	in.ImagePulled = true
	out := ReconcileOnce(in)
	if len(out.Conditions) != 5 {
		t.Fatalf("Conditions = %d, want 5 (BMCReachable + InspectionDone + ImagePulled + Ready + Error)", len(out.Conditions))
	}
	// Find Ready · expect True since NextState=Ready.
	for _, c := range out.Conditions {
		if c.Type == ConditionReady && c.Status != metav1.ConditionTrue {
			t.Fatalf("Ready condition status = %s, want True", c.Status)
		}
	}
}

func TestReconcileRequeueCadence(t *testing.T) {
	ready := newInput(v1alpha1.BMStateReady, v1alpha1.BMStateReady)
	provisioning := newInput(v1alpha1.BMStateProvisioning, v1alpha1.BMStateReady)
	provisioning.BMCReachable = true

	outR := ReconcileOnce(ready)
	outP := ReconcileOnce(provisioning)

	if outR.RequeueAfter != 60*time.Second {
		t.Fatalf("Ready RequeueAfter = %v, want 60s", outR.RequeueAfter)
	}
	if outP.RequeueAfter != 10*time.Second {
		t.Fatalf("Provisioning RequeueAfter = %v, want 10s", outP.RequeueAfter)
	}
}
