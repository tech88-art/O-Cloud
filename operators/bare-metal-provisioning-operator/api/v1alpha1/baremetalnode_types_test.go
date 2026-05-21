/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package v1alpha1

import (
	"encoding/json"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestBareMetalNodeRoundTripJSONMarshal covers P9-T-105 acceptance
// case 1/3: BareMetalNode JSON round-trip preserves all fields.
func TestBareMetalNodeRoundTripJSONMarshal(t *testing.T) {
	orig := &BareMetalNode{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "provisioning.ocloud.edge.example.com/v1alpha1",
			Kind:       "BareMetalNode",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "edge-rack-1-server-3"},
		Spec: BareMetalNodeSpec{
			BMC: BMCSpec{
				Address: "redfish+https://bmc-rack1-3.example.com",
				CredentialsRef: CredentialsReference{
					Name:      "bmc-rack1-3-creds",
					Namespace: "ocloud-system",
				},
			},
			Image: &ImageSpec{
				URL:      "oci://registry.example.com/os/ascend-host:v1.0.0",
				Checksum: "abcdef123456",
			},
			DesiredState: BMStateReady,
		},
		Status: BareMetalNodeStatus{
			ProvisioningState: BMStateProvisioned,
			MACAddress:        "aa:bb:cc:dd:ee:01",
			HardwareInfo: &HardwareInfo{
				CPUCount:    64,
				MemoryBytes: 512 * 1024 * 1024 * 1024,
				NPUCount:    8,
			},
		},
	}
	encoded, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var rt BareMetalNode
	if err := json.Unmarshal(encoded, &rt); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(*orig, rt) {
		t.Errorf("round-trip diff:\n  orig=%#v\n  back=%#v", *orig, rt)
	}
}

// TestBareMetalNodeStateEnum covers P9-T-105 acceptance case 2/3.
func TestBareMetalNodeStateEnum(t *testing.T) {
	expected := map[ProvisioningState]string{
		BMStateInspecting:    "Inspecting",
		BMStateRegistering:   "Registering",
		BMStateProvisioning:  "Provisioning",
		BMStateProvisioned:   "Provisioned",
		BMStateReady:         "Ready",
		BMStateDeprovisioning: "Deprovisioning",
		BMStateError:         "Error",
	}
	for got, want := range expected {
		if string(got) != want {
			t.Errorf("state const = %q, want %q", string(got), want)
		}
	}
}

// TestBareMetalNodeGroupVersion covers P9-T-105 acceptance case 3/3.
func TestBareMetalNodeGroupVersion(t *testing.T) {
	if GroupVersion.Group != "provisioning.ocloud.edge.example.com" {
		t.Errorf("Group = %q, want provisioning.ocloud.edge.example.com", GroupVersion.Group)
	}
	if GroupVersion.Version != "v1alpha1" {
		t.Errorf("Version = %q, want v1alpha1", GroupVersion.Version)
	}
}
