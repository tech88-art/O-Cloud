/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestNodeLifecycleRoundTripJSONMarshal covers P9-T-105 acceptance
// case 1/3: NodeLifecycle serialises + deserialises preserving all
// fields (spec.nodeName + spec.desiredState + spec.maintenanceWindow +
// status.state + status.conditions).
func TestNodeLifecycleRoundTripJSONMarshal(t *testing.T) {
	now := metav1.NewTime(time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC))
	orig := &NodeLifecycle{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "lifecycle.ocloud.edge.example.com/v1alpha1",
			Kind:       "NodeLifecycle",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
		Spec: NodeLifecycleSpec{
			NodeName:     "worker-1.ocloud.example.com",
			DesiredState: NodeStateUnlocked,
			MaintenanceWindow: &MaintenanceWindow{
				Start:           now,
				DurationSeconds: 3600,
			},
		},
		Status: NodeLifecycleStatus{
			State:              NodeStateAvailable,
			LastTransitionTime: &now,
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, LastTransitionTime: now, Reason: "NodeReady"},
			},
		},
	}
	encoded, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var rt NodeLifecycle
	if err := json.Unmarshal(encoded, &rt); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for i := range rt.Status.Conditions {
		rt.Status.Conditions[i].LastTransitionTime.Time = rt.Status.Conditions[i].LastTransitionTime.UTC()
	}
	if rt.Status.LastTransitionTime != nil {
		t := rt.Status.LastTransitionTime.UTC()
		rt.Status.LastTransitionTime = &metav1.Time{Time: t}
	}
	if rt.Spec.MaintenanceWindow != nil {
		rt.Spec.MaintenanceWindow.Start.Time = rt.Spec.MaintenanceWindow.Start.UTC()
	}
	if !reflect.DeepEqual(*orig, rt) {
		t.Errorf("round-trip diff:\n  orig=%#v\n  back=%#v", *orig, rt)
	}
}

// TestNodeLifecycleEnumValues covers P9-T-105 acceptance case 2/3:
// State enum value pinning · 8 states per ADR-0003 v2 + StarlingX adapted.
func TestNodeLifecycleEnumValues(t *testing.T) {
	expectedStates := map[NodeLifecycleState]string{
		NodeStateProvisioning:      "Provisioning",
		NodeStateBootstrap:         "Bootstrap",
		NodeStateAvailable:         "Available",
		NodeStateDegradedAvailable: "DegradedAvailable",
		NodeStateUnavailable:       "Unavailable",
		NodeStateLocked:            "Locked",
		NodeStateUnlocked:          "Unlocked",
		NodeStateRebootRequired:    "RebootRequired",
	}
	for got, want := range expectedStates {
		if string(got) != want {
			t.Errorf("state const = %q, want %q", string(got), want)
		}
	}
}

// TestNodeLifecycleGroupVersion covers P9-T-105 acceptance case 3/3:
// GroupVersion is lifecycle.ocloud.edge.example.com/v1alpha1 (per
// kubebuilder per-operator group convention).
func TestNodeLifecycleGroupVersion(t *testing.T) {
	if GroupVersion.Group != "lifecycle.ocloud.edge.example.com" {
		t.Errorf("Group = %q, want lifecycle.ocloud.edge.example.com", GroupVersion.Group)
	}
	if GroupVersion.Version != "v1alpha1" {
		t.Errorf("Version = %q, want v1alpha1", GroupVersion.Version)
	}
}
