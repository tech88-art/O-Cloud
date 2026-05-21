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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestRoundTripJSONMarshal covers Phase 7 P7-T-006 acceptance case 1/4:
// NPUSliceTemplate serializes + deserializes preserving all fields
// (Spec.Composition order + FallbackStrategy + Status.Conditions).
func TestRoundTripJSONMarshal(t *testing.T) {
	orig := &NPUSliceTemplate{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "npu.ocloud.edge.example.com/v1alpha1",
			Kind:       "NPUSliceTemplate",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       "qwen-8b-pd-pair",
			Generation: 3,
		},
		Spec: NPUSliceTemplateSpec{
			Composition: []TemplatePart{
				{Type: PartTypeVir04, Count: 1},
				{Type: PartTypeVir08, Count: 1},
			},
			FallbackStrategy: FallbackStrategyFixedTemplateCombination,
		},
		Status: NPUSliceTemplateStatus{
			Conditions: []metav1.Condition{
				{Type: ConditionTypeValidated, Status: metav1.ConditionTrue, Reason: "CompositionValid"},
				{Type: ConditionTypeAllocatable, Status: metav1.ConditionTrue, Reason: "AllAvailable"},
			},
			FallbackAppliedReason: "decomposed into 1×vir04 + 1×vir08",
			ObservedGeneration:    3,
		},
	}

	raw, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var rt NPUSliceTemplate
	if err := json.Unmarshal(raw, &rt); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if !reflect.DeepEqual(orig.Spec, rt.Spec) {
		t.Fatalf("Spec round-trip mismatch:\norig: %+v\nrt:   %+v", orig.Spec, rt.Spec)
	}
	if orig.Status.FallbackAppliedReason != rt.Status.FallbackAppliedReason {
		t.Fatalf("Status.FallbackAppliedReason round-trip mismatch: %q vs %q",
			orig.Status.FallbackAppliedReason, rt.Status.FallbackAppliedReason)
	}
	if orig.Status.ObservedGeneration != rt.Status.ObservedGeneration {
		t.Fatalf("Status.ObservedGeneration round-trip mismatch: %d vs %d",
			orig.Status.ObservedGeneration, rt.Status.ObservedGeneration)
	}
	if len(rt.Status.Conditions) != 2 {
		t.Fatalf("Status.Conditions round-trip len = %d, want 2", len(rt.Status.Conditions))
	}
}

// TestDeepCopyPreservesComposition covers Phase 7 P7-T-006 acceptance
// case 2/4: DeepCopy returns a fully detached object (mutating the copy
// must not affect the original).
func TestDeepCopyPreservesComposition(t *testing.T) {
	orig := &NPUSliceTemplate{
		Spec: NPUSliceTemplateSpec{
			Composition: []TemplatePart{
				{Type: PartTypeVir04, Count: 2, AICoreRequest: 4},
			},
			FallbackStrategy: FallbackStrategyRefuse,
		},
	}

	cp := orig.DeepCopy()
	if cp == nil {
		t.Fatal("DeepCopy returned nil")
	}
	// Mutate the copy; verify orig unchanged.
	cp.Spec.Composition[0].Count = 99
	cp.Spec.FallbackStrategy = FallbackStrategyFixedTemplateCombination
	cp.Spec.Composition = append(cp.Spec.Composition, TemplatePart{Type: PartTypeWhole, Count: 1})

	if orig.Spec.Composition[0].Count != 2 {
		t.Fatalf("DeepCopy slice not detached: orig.Composition[0].Count = %d, want 2",
			orig.Spec.Composition[0].Count)
	}
	if orig.Spec.FallbackStrategy != FallbackStrategyRefuse {
		t.Fatalf("DeepCopy did not detach FallbackStrategy: orig = %q, want %q",
			orig.Spec.FallbackStrategy, FallbackStrategyRefuse)
	}
	if len(orig.Spec.Composition) != 1 {
		t.Fatalf("DeepCopy slice not detached: orig.Composition len = %d, want 1",
			len(orig.Spec.Composition))
	}
}

// TestEmptyStatusOmitted covers Phase 7 P7-T-006 acceptance case 3/4:
// A NPUSliceTemplate with empty Status serializes WITHOUT a status
// object (omitempty everywhere). Important for `kubectl get -o yaml`
// readability + status-subresource semantics.
func TestEmptyStatusOmitted(t *testing.T) {
	orig := &NPUSliceTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "minimal"},
		Spec: NPUSliceTemplateSpec{
			Composition: []TemplatePart{{Type: PartTypeWhole, Count: 1}},
		},
	}
	raw, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// The marshaled JSON should not include an explicit
	// `"fallbackAppliedReason":""` or `"conditions":[]` block because
	// every status field is +optional with omitempty.
	s := string(raw)
	if contains(s, "fallbackAppliedReason") {
		t.Fatalf("empty FallbackAppliedReason was serialized: %s", s)
	}
	if contains(s, "observedGeneration") {
		t.Fatalf("empty ObservedGeneration was serialized: %s", s)
	}
}

// TestEnumValuesArePinned covers Phase 7 P7-T-006 acceptance case 4/4:
// PartType + FallbackStrategy enum value strings are pinned to the
// ADR-0011 §4 schema (drift here = silent migration break for operators
// who hard-coded the literal strings in chart values / kubectl scripts).
func TestEnumValuesArePinned(t *testing.T) {
	if PartTypeWhole != "whole" {
		t.Fatalf("PartTypeWhole = %q, want \"whole\"", PartTypeWhole)
	}
	if PartTypeVir04 != "vir04" {
		t.Fatalf("PartTypeVir04 = %q, want \"vir04\"", PartTypeVir04)
	}
	if PartTypeVir08 != "vir08" {
		t.Fatalf("PartTypeVir08 = %q, want \"vir08\"", PartTypeVir08)
	}
	if PartTypeVir16 != "vir16" {
		t.Fatalf("PartTypeVir16 = %q, want \"vir16\"", PartTypeVir16)
	}
	if PartTypeDynamicShard != "dynamic-shard" {
		t.Fatalf("PartTypeDynamicShard = %q, want \"dynamic-shard\"", PartTypeDynamicShard)
	}
	if FallbackStrategyFixedTemplateCombination != "fixed-template-combination" {
		t.Fatalf("FallbackStrategyFixedTemplateCombination = %q, want \"fixed-template-combination\"",
			FallbackStrategyFixedTemplateCombination)
	}
	if FallbackStrategyRefuse != "refuse" {
		t.Fatalf("FallbackStrategyRefuse = %q, want \"refuse\"", FallbackStrategyRefuse)
	}
	if ConditionTypeValidated != "Validated" {
		t.Fatalf("ConditionTypeValidated = %q, want \"Validated\"", ConditionTypeValidated)
	}
	if ConditionTypeAllocatable != "Allocatable" {
		t.Fatalf("ConditionTypeAllocatable = %q, want \"Allocatable\"", ConditionTypeAllocatable)
	}
}

// contains is a tiny helper avoiding an explicit `strings` import (one
// of the conventions the existing api/v1alpha1 test files follow).
func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
