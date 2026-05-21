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
	"errors"
	"fmt"
	"sort"
	"strings"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/npu-dra-driver/api/v1alpha1"
)

// ValidationError is the typed error Engine.Validate emits. The Reason
// field maps to NPUSliceTemplate.Status.Conditions[Validated].reason
// stamped by the template_controller (per ADR-0011 §後果 + DESIGN.md
// §3.8 validation rules table).
type ValidationError struct {
	Reason  string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation: %s: %s", e.Reason, e.Message)
}

// IsValidationError returns true when err is a *ValidationError.
// Callers (template_controller) discriminate validation failures from
// transient I/O errors to decide whether to stamp Validated=False or
// requeue with backoff.
func IsValidationError(err error) bool {
	var v *ValidationError
	return errors.As(err, &v)
}

// AsValidationError returns the underlying *ValidationError or nil
// when err is not one.
func AsValidationError(err error) *ValidationError {
	var v *ValidationError
	if errors.As(err, &v) {
		return v
	}
	return nil
}

// Common ValidationError reasons exposed for the template_controller
// status-stamping path + test assertion stability.
const (
	ReasonEmptyComposition       = "EmptyComposition"
	ReasonInvalidCount           = "InvalidCount"
	ReasonInvalidType            = "InvalidType"
	ReasonDynamicShardNotSupported = "DynamicShardNotSupported"
)

// Engine implements the Phase 7 W1 template validation + decomposition
// logic per ADR-0011 §1 §4. The struct is stateless — caller (the
// template_controller) instantiates one per process and shares it
// across reconcile passes.
type Engine struct{}

// New returns a fresh Engine. Phase 7 W1: no construction config (the
// engine is stateless). Phase 8+ may add a config for tuning (e.g.
// dynamic-shard allow-list once driver-layer breakthrough lands).
func New() *Engine {
	return &Engine{}
}

// Validate enforces schema rules beyond what kubebuilder admission
// covers — specifically:
//
//   - Composition must be non-empty (kubebuilder MinItems=1 also
//     enforces; engine double-checks defensively)
//   - Each part Count must be ≥ 1 (kubebuilder Minimum=1; defensive)
//   - PartType=dynamic-shard is rejected in Phase 7 (no decomposition
//     path; gated on driver-layer breakthrough OR KEP-4815 GA per
//     ADR-0009 §4 + ADR-0011 §後果)
//   - PartType enum membership is checked (kubebuilder validation
//     normally catches; engine defensively rejects unknown values
//     to surface clearer error messages in status.conditions)
//
// Returns a *ValidationError on rule violation; nil on success.
// Callers (template_controller) use IsValidationError to decide
// whether to stamp Validated=False (validation) or requeue (transient).
func (e *Engine) Validate(spec v1alpha1.NPUSliceTemplateSpec) error {
	if len(spec.Composition) == 0 {
		return &ValidationError{
			Reason:  ReasonEmptyComposition,
			Message: "spec.composition must contain at least 1 part (kubebuilder MinItems=1)",
		}
	}
	for i, part := range spec.Composition {
		if part.Count < 1 {
			return &ValidationError{
				Reason:  ReasonInvalidCount,
				Message: fmt.Sprintf("spec.composition[%d].count = %d, must be >= 1", i, part.Count),
			}
		}
		if !isKnownPartType(part.Type) {
			return &ValidationError{
				Reason:  ReasonInvalidType,
				Message: fmt.Sprintf("spec.composition[%d].type = %q is not a recognised PartType enum", i, part.Type),
			}
		}
		if part.Type == v1alpha1.PartTypeDynamicShard {
			// Phase 7 fallback path has no driver-layer hook for
			// dynamic sharding; reject regardless of fallback strategy.
			// Gated on Phase 8+ driver breakthrough OR KEP-4815 GA per
			// ADR-0011 §後果 row.
			return &ValidationError{
				Reason: ReasonDynamicShardNotSupported,
				Message: fmt.Sprintf(
					"spec.composition[%d].type = dynamic-shard requires driver-layer breakthrough OR KEP-4815 GA "+
						"(both deferred to Phase 8+ per ADR-0009 §4 + ADR-0011 §後果); set type to one of "+
						"whole/vir04/vir08/vir16 to use the Phase 7 fallback path",
					i),
			}
		}
	}
	return nil
}

// Decompose maps NPUSliceTemplateSpec.Composition into a
// FixedTemplateBundle the allocator (P7-T-105) handles as all-or-nothing.
//
// Phase 7 W1 decomposition is mechanical:
//
//   - For each Composition[i] of (Type=X, Count=N): emit
//     FixedTemplateItem{Template: X, Count: N}
//   - Same Type appearing twice → merge by summing Count
//   - FallbackStrategy=refuse + Composition uses only fixed templates
//     → succeed unchanged (refuse only rejects when decomposition is
//     needed beyond direct mapping; Phase 7 W1 always maps directly)
//   - FallbackStrategy=fixed-template-combination → succeed unchanged
//     (same direct mapping; the strategy name describes the BUDGET for
//     decomposition, not a different output shape)
//
// The Phase 7 W1 distinction between strategies surfaces only when
// dynamic-shard is in the composition (Validate rejects upstream for
// both strategies; future Phase 8+ may make fixed-template-combination
// attempt synthesis while refuse continues to reject).
//
// Returns (bundle, reason, error). reason is the human-readable
// description for NPUSliceTemplate.Status.FallbackAppliedReason; empty
// when no merging or special handling fired.
func (e *Engine) Decompose(spec v1alpha1.NPUSliceTemplateSpec) (*FixedTemplateBundle, string, error) {
	if err := e.Validate(spec); err != nil {
		return nil, "", err
	}

	// Merge duplicate Types by summing Counts (preserves order of
	// first occurrence).
	merged := []FixedTemplateItem{}
	indexOf := map[v1alpha1.PartType]int{}
	for _, part := range spec.Composition {
		if idx, ok := indexOf[part.Type]; ok {
			merged[idx].Count += part.Count
			continue
		}
		indexOf[part.Type] = len(merged)
		merged = append(merged, FixedTemplateItem{
			Template: string(part.Type),
			Count:    part.Count,
		})
	}

	bundle := &FixedTemplateBundle{Items: merged}

	// Build the FallbackAppliedReason summary string in stable order
	// (sort by Template name asc for diff idempotence — chart-rendered
	// status doesn't churn just because map iteration order shuffled).
	sorted := make([]FixedTemplateItem, len(merged))
	copy(sorted, merged)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Template < sorted[j].Template })
	parts := make([]string, 0, len(sorted))
	for _, it := range sorted {
		parts = append(parts, fmt.Sprintf("%d×%s", it.Count, it.Template))
	}
	reason := "decomposed into " + strings.Join(parts, " + ")

	return bundle, reason, nil
}

// isKnownPartType returns true when t is a recognised PartType enum
// (whole / vir04 / vir08 / vir16 / dynamic-shard). dynamic-shard is
// rejected elsewhere in Validate, but isKnownPartType accepts it so
// Validate can give the more-specific DynamicShardNotSupported error
// instead of InvalidType.
func isKnownPartType(t v1alpha1.PartType) bool {
	switch t {
	case v1alpha1.PartTypeWhole,
		v1alpha1.PartTypeVir04,
		v1alpha1.PartTypeVir08,
		v1alpha1.PartTypeVir16,
		v1alpha1.PartTypeDynamicShard:
		return true
	}
	return false
}
