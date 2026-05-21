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

// TestSoftwareBundleRoundTripJSONMarshal covers P9-T-105 acceptance
// case 1/3: SoftwareBundle JSON round-trip preserves all fields.
func TestSoftwareBundleRoundTripJSONMarshal(t *testing.T) {
	orig := &SoftwareBundle{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "softwaremgmt.ocloud.edge.example.com/v1alpha1",
			Kind:       "SoftwareBundle",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "bundle-2026-05"},
		Spec: SoftwareBundleSpec{
			Version: "v1.2.0",
			Patches: []SoftwarePatch{
				{Name: "kernel-update", SourceURI: "oci://registry/patches/kernel:v1.2.0", Checksum: "abc123"},
				{Name: "cann-update", SourceURI: "oci://registry/patches/cann:8.1.0", Checksum: "def456"},
			},
			RolloutPolicy: RolloutPolicy{
				Strategy:       RolloutStrategyRollingUpdate,
				MaxUnavailable: 2,
			},
			NodeSelector: map[string]string{"huawei.com/Ascend910B": "true"},
		},
		Status: SoftwareBundleStatus{
			AppliedVersion:   "v1.2.0",
			TargetNodeCount:  10,
			AppliedNodeCount: 8,
			FailedNodeCount:  0,
		},
	}
	encoded, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var rt SoftwareBundle
	if err := json.Unmarshal(encoded, &rt); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(*orig, rt) {
		t.Errorf("round-trip diff:\n  orig=%#v\n  back=%#v", *orig, rt)
	}
}

// TestSoftwareBundleRolloutStrategyEnum covers P9-T-105 acceptance case 2/3.
func TestSoftwareBundleRolloutStrategyEnum(t *testing.T) {
	expected := map[RolloutStrategy]string{
		RolloutStrategyRollingUpdate: "RollingUpdate",
		RolloutStrategyParallel:      "Parallel",
		RolloutStrategySequential:    "Sequential",
	}
	for got, want := range expected {
		if string(got) != want {
			t.Errorf("strategy const = %q, want %q", string(got), want)
		}
	}
}

// TestSoftwareBundleGroupVersion covers P9-T-105 acceptance case 3/3.
func TestSoftwareBundleGroupVersion(t *testing.T) {
	if GroupVersion.Group != "softwaremgmt.ocloud.edge.example.com" {
		t.Errorf("Group = %q, want softwaremgmt.ocloud.edge.example.com", GroupVersion.Group)
	}
	if GroupVersion.Version != "v1alpha1" {
		t.Errorf("Version = %q, want v1alpha1", GroupVersion.Version)
	}
}
