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
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestQuotaRoundTripJSONMarshal covers P9-T-005 acceptance case 1/4:
// Quota serialises + deserialises preserving all fields (spec.enforcement
// + status.usage + status.conditions + status.lastSyncTime).
func TestQuotaRoundTripJSONMarshal(t *testing.T) {
	now := metav1.NewTime(time.Date(2026, 5, 21, 14, 32, 17, 0, time.UTC))
	orig := &Quota{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "inference.ocloud.edge.example.com/v1alpha1",
			Kind:       "Quota",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       "ai-edge-demo-quota",
			Namespace:  "ai-edge-demo",
			Generation: 2,
		},
		Spec: QuotaSpec{
			Enforcement: QuotaEnforcement{
				MaxSliceAllocations: 8,
				MaxScaleEventsPerWindow: ScaleEventRateCap{
					Count:         5,
					WindowSeconds: 3600,
				},
				MaxNPUSliceTemplateRefs: []string{"qwen-pd-busy", "qwen-pd-idle"},
			},
		},
		Status: QuotaStatus{
			Conditions: []metav1.Condition{
				{
					Type:               ConditionQuotaActive,
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "ControllerReady",
					Message:            "Quota controller reconciling usage on 60s tick",
				},
				{
					Type:               ConditionQuotaEnforcementOK,
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "WebhooksRegistered",
				},
			},
			Usage: QuotaUsage{
				CurrentSliceAllocations: 3,
				ScaleEventsInWindow:     1,
			},
			LastSyncTime: &now,
		},
	}

	encoded, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded Quota
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// Normalize metav1.Time zones to UTC — round-trip JSON via RFC3339
	// preserves the instant but may convert to time.Local on the receiving
	// side. reflect.DeepEqual treats time.UTC ≠ time.Local even at the same
	// instant, so we normalize before comparing.
	for i := range decoded.Status.Conditions {
		decoded.Status.Conditions[i].LastTransitionTime.Time = decoded.Status.Conditions[i].LastTransitionTime.UTC()
	}
	if decoded.Status.LastSyncTime != nil {
		t := decoded.Status.LastSyncTime.UTC()
		decoded.Status.LastSyncTime = &metav1.Time{Time: t}
	}

	if !reflect.DeepEqual(*orig, decoded) {
		t.Errorf("round-trip diff:\n  orig = %#v\n  back = %#v", *orig, decoded)
	}
}

// TestQuotaDeepCopy covers P9-T-005 acceptance case 2/4: zz_generated
// DeepCopy method produces an independent clone (mutating clone does not
// affect orig).
func TestQuotaDeepCopy(t *testing.T) {
	orig := &Quota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo-quota",
			Namespace: "ai-edge-demo",
		},
		Spec: QuotaSpec{
			Enforcement: QuotaEnforcement{
				MaxSliceAllocations: 8,
				MaxScaleEventsPerWindow: ScaleEventRateCap{
					Count:         5,
					WindowSeconds: 3600,
				},
				MaxNPUSliceTemplateRefs: []string{"qwen-pd-busy", "qwen-pd-idle"},
			},
		},
	}

	clone := orig.DeepCopy()
	if clone == orig {
		t.Fatalf("DeepCopy returned same pointer")
	}
	if !reflect.DeepEqual(orig, clone) {
		t.Errorf("DeepCopy not equal:\n  orig  = %#v\n  clone = %#v", orig, clone)
	}

	// Mutate clone slices/strings — orig must remain untouched.
	clone.Spec.Enforcement.MaxSliceAllocations = 99
	clone.Spec.Enforcement.MaxNPUSliceTemplateRefs[0] = "mutated"

	if orig.Spec.Enforcement.MaxSliceAllocations == 99 {
		t.Errorf("clone mutation leaked into orig (MaxSliceAllocations)")
	}
	if orig.Spec.Enforcement.MaxNPUSliceTemplateRefs[0] == "mutated" {
		t.Errorf("clone slice mutation leaked into orig (MaxNPUSliceTemplateRefs)")
	}
}

// TestQuotaOmitEmpty covers P9-T-005 acceptance case 3/4: zero-value
// Spec/Status fields are omitted from JSON output (omitempty tag respected),
// while required fields (Enforcement) remain present even when zero-value.
func TestQuotaOmitEmpty(t *testing.T) {
	minimal := &Quota{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "inference.ocloud.edge.example.com/v1alpha1",
			Kind:       "Quota",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "minimal",
			Namespace: "default",
		},
		Spec: QuotaSpec{
			// Enforcement zero-value (all caps 0 = unbounded).
		},
	}

	encoded, err := json.Marshal(minimal)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	out := string(encoded)

	// Status is omitempty but holds non-pointer fields (Usage / Conditions)
	// — Conditions is omitempty (nil slice), Usage is value type with both
	// fields = 0, so Status itself isn't omitted (struct with zero-value
	// fields still marshals as `"status":{"usage":{...zero...}}`). This is
	// the K8s subresource convention: status is always present in YAML
	// after a controller writes it; tests below assert key presence not
	// status:{} absence.

	// Spec MUST be present (required + has enforcement subresource).
	if !strings.Contains(out, `"spec"`) {
		t.Errorf("spec missing from minimal Quota JSON: %s", out)
	}
	if !strings.Contains(out, `"enforcement"`) {
		t.Errorf("spec.enforcement missing from minimal Quota JSON: %s", out)
	}

	// status.conditions is omitempty []; absent when nil.
	if strings.Contains(out, `"conditions"`) {
		t.Errorf("status.conditions should be omitted when nil; got: %s", out)
	}
	// status.lastSyncTime is omitempty *metav1.Time; absent when nil.
	if strings.Contains(out, `"lastSyncTime"`) {
		t.Errorf("status.lastSyncTime should be omitted when nil; got: %s", out)
	}
}

// TestQuotaScaleEventRateCapWindowSecondsDefault covers P9-T-005 acceptance
// case 4/4: ScaleEventRateCap.WindowSeconds JSON tag is `windowSeconds`
// matching kubebuilder convention + ADR-0014 §2 Decision B documented
// schema (operative test pinning the field name across Go ↔ YAML).
func TestQuotaScaleEventRateCapWindowSecondsDefault(t *testing.T) {
	cap := ScaleEventRateCap{
		Count:         5,
		WindowSeconds: 3600,
	}
	encoded, err := json.Marshal(cap)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	out := string(encoded)

	if !strings.Contains(out, `"count":5`) {
		t.Errorf(`count field missing or wrong JSON tag (want "count":5): %s`, out)
	}
	if !strings.Contains(out, `"windowSeconds":3600`) {
		t.Errorf(`windowSeconds field missing or wrong JSON tag (want "windowSeconds":3600): %s`, out)
	}

	// MaxScaleEventsPerWindow JSON tag check on parent
	enf := QuotaEnforcement{
		MaxScaleEventsPerWindow: cap,
	}
	encEnf, err := json.Marshal(enf)
	if err != nil {
		t.Fatalf("enforcement Marshal failed: %v", err)
	}
	if !strings.Contains(string(encEnf), `"maxScaleEventsPerWindow"`) {
		t.Errorf("maxScaleEventsPerWindow JSON tag missing: %s", string(encEnf))
	}
}
