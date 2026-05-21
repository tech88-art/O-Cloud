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

package template

import (
	"testing"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/npu-dra-driver/api/v1alpha1"
)

// TestDecomposeEmptyCompositionRejects covers Phase 7 P7-T-007
// acceptance case 1/6: empty composition surfaces a ValidationError
// with Reason=EmptyComposition.
func TestDecomposeEmptyCompositionRejects(t *testing.T) {
	eng := New()
	_, _, err := eng.Decompose(v1alpha1.NPUSliceTemplateSpec{
		Composition: nil,
	})
	if err == nil {
		t.Fatal("expected ValidationError, got nil")
	}
	ve := AsValidationError(err)
	if ve == nil {
		t.Fatalf("expected *ValidationError, got %T (%v)", err, err)
	}
	if ve.Reason != ReasonEmptyComposition {
		t.Fatalf("Reason = %q, want %q", ve.Reason, ReasonEmptyComposition)
	}
}

// TestDecomposeWholeOnly covers Phase 7 P7-T-007 acceptance case 2/6:
// a composition of 1× whole decomposes into a 1-item bundle.
func TestDecomposeWholeOnly(t *testing.T) {
	eng := New()
	bundle, reason, err := eng.Decompose(v1alpha1.NPUSliceTemplateSpec{
		Composition: []v1alpha1.TemplatePart{
			{Type: v1alpha1.PartTypeWhole, Count: 1},
		},
		FallbackStrategy: v1alpha1.FallbackStrategyRefuse,
	})
	if err != nil {
		t.Fatalf("Decompose: %v", err)
	}
	if len(bundle.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(bundle.Items))
	}
	if bundle.Items[0].Template != "whole" || bundle.Items[0].Count != 1 {
		t.Fatalf("Items[0] = %+v, want {whole, 1}", bundle.Items[0])
	}
	if bundle.TotalSlices() != 1 {
		t.Fatalf("TotalSlices() = %d, want 1", bundle.TotalSlices())
	}
	if reason == "" || reason[:len("decomposed into")] != "decomposed into" {
		t.Fatalf("reason = %q, want \"decomposed into ...\" prefix", reason)
	}
}

// TestDecomposeSingleVir04 covers Phase 7 P7-T-007 acceptance case 3/6:
// a composition of 1× vir04 decomposes into a 1-item bundle.
func TestDecomposeSingleVir04(t *testing.T) {
	eng := New()
	bundle, _, err := eng.Decompose(v1alpha1.NPUSliceTemplateSpec{
		Composition: []v1alpha1.TemplatePart{
			{Type: v1alpha1.PartTypeVir04, Count: 1},
		},
		FallbackStrategy: v1alpha1.FallbackStrategyFixedTemplateCombination,
	})
	if err != nil {
		t.Fatalf("Decompose: %v", err)
	}
	if len(bundle.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(bundle.Items))
	}
	if bundle.Items[0].Template != "vir04" || bundle.Items[0].Count != 1 {
		t.Fatalf("Items[0] = %+v, want {vir04, 1}", bundle.Items[0])
	}
}

// TestDecomposeVir04PlusVir08 covers Phase 7 P7-T-007 acceptance case
// 4/6: a Qwen-PD-style composition (1× vir04 + 1× vir08) decomposes
// into a 2-item bundle preserving Composition order.
func TestDecomposeVir04PlusVir08(t *testing.T) {
	eng := New()
	bundle, reason, err := eng.Decompose(v1alpha1.NPUSliceTemplateSpec{
		Composition: []v1alpha1.TemplatePart{
			{Type: v1alpha1.PartTypeVir04, Count: 1},
			{Type: v1alpha1.PartTypeVir08, Count: 1},
		},
		FallbackStrategy: v1alpha1.FallbackStrategyFixedTemplateCombination,
	})
	if err != nil {
		t.Fatalf("Decompose: %v", err)
	}
	if len(bundle.Items) != 2 {
		t.Fatalf("len(Items) = %d, want 2 (Qwen PD-pair)", len(bundle.Items))
	}
	// First item (per Composition order) should be vir04.
	if bundle.Items[0].Template != "vir04" {
		t.Fatalf("Items[0].Template = %q, want \"vir04\"", bundle.Items[0].Template)
	}
	if bundle.Items[1].Template != "vir08" {
		t.Fatalf("Items[1].Template = %q, want \"vir08\"", bundle.Items[1].Template)
	}
	if bundle.TotalSlices() != 2 {
		t.Fatalf("TotalSlices() = %d, want 2", bundle.TotalSlices())
	}
	// FallbackAppliedReason should mention BOTH templates in sorted
	// order (vir04 then vir08 alphabetically).
	wantReason := "decomposed into 1×vir04 + 1×vir08"
	if reason != wantReason {
		t.Fatalf("reason = %q, want %q", reason, wantReason)
	}
}

// TestDecomposeRefuseRejectsDynamicShard covers Phase 7 P7-T-007
// acceptance case 5/6: fallbackStrategy=refuse + composition containing
// dynamic-shard surfaces DynamicShardNotSupported (same as
// fixed-template-combination because Phase 7 W1 has no driver hook
// for dynamic-shard regardless of strategy).
func TestDecomposeRefuseRejectsDynamicShard(t *testing.T) {
	eng := New()
	_, _, err := eng.Decompose(v1alpha1.NPUSliceTemplateSpec{
		Composition: []v1alpha1.TemplatePart{
			{Type: v1alpha1.PartTypeDynamicShard, Count: 1, AICoreRequest: 4},
		},
		FallbackStrategy: v1alpha1.FallbackStrategyRefuse,
	})
	if err == nil {
		t.Fatal("expected ValidationError, got nil")
	}
	ve := AsValidationError(err)
	if ve == nil {
		t.Fatalf("expected *ValidationError, got %T (%v)", err, err)
	}
	if ve.Reason != ReasonDynamicShardNotSupported {
		t.Fatalf("Reason = %q, want %q", ve.Reason, ReasonDynamicShardNotSupported)
	}
}

// TestDecomposeFixedTemplateCombination covers Phase 7 P7-T-007
// acceptance case 6/6: fallbackStrategy=fixed-template-combination
// with valid composition decomposes (duplicate-Type merging works).
func TestDecomposeFixedTemplateCombination(t *testing.T) {
	eng := New()
	bundle, reason, err := eng.Decompose(v1alpha1.NPUSliceTemplateSpec{
		Composition: []v1alpha1.TemplatePart{
			{Type: v1alpha1.PartTypeVir04, Count: 2},
			{Type: v1alpha1.PartTypeVir04, Count: 1}, // dup type
			{Type: v1alpha1.PartTypeWhole, Count: 1},
		},
		FallbackStrategy: v1alpha1.FallbackStrategyFixedTemplateCombination,
	})
	if err != nil {
		t.Fatalf("Decompose: %v", err)
	}
	if len(bundle.Items) != 2 {
		t.Fatalf("len(Items) = %d, want 2 (vir04 merged + whole)", len(bundle.Items))
	}
	// vir04 should be merged: Count = 2 + 1 = 3
	var vir04Count, wholeCount int32
	for _, it := range bundle.Items {
		switch it.Template {
		case "vir04":
			vir04Count = it.Count
		case "whole":
			wholeCount = it.Count
		}
	}
	if vir04Count != 3 {
		t.Fatalf("vir04 Count after merge = %d, want 3", vir04Count)
	}
	if wholeCount != 1 {
		t.Fatalf("whole Count = %d, want 1", wholeCount)
	}
	if bundle.TotalSlices() != 4 {
		t.Fatalf("TotalSlices() = %d, want 4", bundle.TotalSlices())
	}
	wantReason := "decomposed into 3×vir04 + 1×whole"
	if reason != wantReason {
		t.Fatalf("reason = %q, want %q", reason, wantReason)
	}
}
