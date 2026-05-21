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

// Package template implements the Phase 7 P7-T-007 template engine
// (composition decomposition) + the NPUSliceTemplate reconciler that
// stamps Validated + Allocatable status conditions on every
// NPUSliceTemplate object.
//
// Phase 7 W1 design (per ADR-0011 §1 §4):
//
//   - Engine.Validate enforces schema-beyond-kubebuilder rules
//     (dynamic-shard rejection in fallback mode + duplicate-type
//     merging)
//   - Engine.Decompose maps NPUSliceTemplateSpec.Composition → a
//     FixedTemplateBundle the allocator (P7-T-105) handles as
//     all-or-nothing
//   - FallbackAppliedReason describes the decomposition for operators
//     (e.g. "decomposed into 1×vir04 + 1×vir08")
//
// **References**: ADR-0011 §1 + §4 + §後果 row 4 + ADR-0009 §4 +
// phase7-plan.md §3 P7-T-007 + §4 P7-T-105.
package template

// FixedTemplateItem is one entry in a FixedTemplateBundle — a request
// for Count slices of the existing fixed-template named Template
// (whole / vir04 / vir08 / vir16).
//
// Phase 7 W1: Template values are the literal strings from
// api/v1alpha1.PartType (lowercased per the enum). Phase 7 T105
// allocator reads these to invoke per-template Allocate calls.
type FixedTemplateItem struct {
	// Template is the existing fixed-template name (one of "whole" /
	// "vir04" / "vir08" / "vir16"). dynamic-shard is rejected at
	// Engine.Validate so it never reaches the bundle.
	Template string

	// Count is the number of slices of this Template the allocator
	// must reserve. Guaranteed ≥ 1 (Engine.Validate enforces).
	Count int32
}

// FixedTemplateBundle is the output of Engine.Decompose — a flat list
// of FixedTemplateItem requests the allocator handles as a single
// all-or-nothing unit. Phase 7 T105 allocator allocates each item in
// turn; any failure rolls back the whole bundle (per ADR-0011 §後果 row 4).
type FixedTemplateBundle struct {
	// Items is the per-template-request list. Order is the same as the
	// NPUSliceTemplate.Spec.Composition (after Validate's
	// duplicate-merge pass, if any).
	Items []FixedTemplateItem
}

// IsEmpty returns true when the bundle has no items. Useful for
// Engine.Decompose callers + tests.
func (b *FixedTemplateBundle) IsEmpty() bool {
	return b == nil || len(b.Items) == 0
}

// TotalSlices returns the sum of Count across all items. Phase 7 T105
// allocator uses this for early "sum vs pool availability" check.
func (b *FixedTemplateBundle) TotalSlices() int32 {
	if b == nil {
		return 0
	}
	var total int32
	for _, it := range b.Items {
		total += it.Count
	}
	return total
}
