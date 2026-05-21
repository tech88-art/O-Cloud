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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PartType enumerates the supported slice composition part types per
// ADR-0011 §4. Phase 7 W1 ships 5 enum values; "dynamic-shard" is
// reserved for driver-layer breakthrough (Phase 7 fallback path
// rejects "dynamic-shard" when fallbackStrategy=fixed-template-combination
// — see ADR-0011 §後果 row for the validation rationale).
//
// +kubebuilder:validation:Enum=whole;vir04;vir08;vir16;dynamic-shard
type PartType string

const (
	// PartTypeWhole = the whole NPU (no slicing) — equivalent to
	// requesting a complete physical chip.
	PartTypeWhole PartType = "whole"

	// PartTypeVir04 = 4 AI cores per slice (Ascend 910B fixed template
	// per Huawei NPU template documentation).
	PartTypeVir04 PartType = "vir04"

	// PartTypeVir08 = 8 AI cores per slice.
	PartTypeVir08 PartType = "vir08"

	// PartTypeVir16 = 16 AI cores per slice.
	PartTypeVir16 PartType = "vir16"

	// PartTypeDynamicShard = driver-layer dynamic sharding (AICoreRequest
	// field consulted). Phase 7 fallback path REJECTS this with
	// validation error when fallbackStrategy=fixed-template-combination
	// — gated on driver-layer breakthrough OR KEP-4815 Partitionable
	// Devices GA per ADR-0011 §後果 row + ADR-0009 §4.
	PartTypeDynamicShard PartType = "dynamic-shard"
)

// FallbackStrategy enumerates the behavior when the template engine
// (P7-T-007) cannot directly serve a composition with existing fixed
// templates (vir04/vir08/vir16/whole). Phase 7 ships 2 enum values per
// ADR-0011 §4.
//
// +kubebuilder:validation:Enum=fixed-template-combination;refuse
type FallbackStrategy string

const (
	// FallbackStrategyFixedTemplateCombination = engine decomposes the
	// composition into the existing fixed-template set (default per
	// ADR-0011 §4). Allocator (P7-T-105) handles each item as a separate
	// NPU allocation; whole-bundle semantics enforced "all-or-nothing".
	FallbackStrategyFixedTemplateCombination FallbackStrategy = "fixed-template-combination"

	// FallbackStrategyRefuse = engine emits Validated=False + reason
	// when composition cannot be served by existing templates; no
	// decomposition attempted. Strict mode for production workloads
	// that must fail-fast on misconfigured templates.
	FallbackStrategyRefuse FallbackStrategy = "refuse"
)

// Standard ConditionType values stamped by the template_controller
// (P7-T-007) on NPUSliceTemplate.Status.Conditions. Phase 7 W1 defines
// the constants; T007 reconciler writes them.
const (
	// ConditionTypeValidated indicates the composition passed schema
	// validation (PartType enum / Count > 0 / fallbackStrategy compatible
	// with composition).
	ConditionTypeValidated = "Validated"

	// ConditionTypeAllocatable indicates the allocator (P7-T-105)
	// confirmed every bundle item can be served (sum of required NPUs
	// ≤ pool availability).
	ConditionTypeAllocatable = "Allocatable"
)

// TemplatePart describes one part of an NPUSliceTemplate composition
// per ADR-0011 §4. Each part requests Count slices of Type granularity.
type TemplatePart struct {
	// Type selects the slice granularity per PartType enum.
	// +kubebuilder:validation:Required
	Type PartType `json:"type"`

	// Count is the number of slices of this Type requested by the
	// template. Must be ≥ 1; 0 makes no sense (omit the part instead).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	Count int32 `json:"count"`

	// AICoreRequest is the per-slice AI core budget. Consulted ONLY
	// when Type=dynamic-shard; ignored otherwise (informational for
	// other types where the AI core count is implied by the template
	// name — vir04=4, vir08=8, vir16=16, whole=32 on 910B).
	// +kubebuilder:validation:Minimum=0
	// +optional
	AICoreRequest int32 `json:"aiCoreRequest,omitempty"`
}

// NPUSliceTemplateSpec is the desired-state block of an NPUSliceTemplate
// per ADR-0011 §4.
type NPUSliceTemplateSpec struct {
	// Composition lists the slice parts that together make up this
	// template. Order is significant only for documentation; the
	// allocator (P7-T-105) handles parts in slice (sum) form.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Composition []TemplatePart `json:"composition"`

	// FallbackStrategy controls what happens when the composition
	// cannot be directly served by existing fixed templates per
	// ADR-0011 §4.
	// +kubebuilder:default=fixed-template-combination
	// +optional
	FallbackStrategy FallbackStrategy `json:"fallbackStrategy,omitempty"`
}

// NPUSliceTemplateStatus is the observed-state block written by the
// template_controller (P7-T-007).
type NPUSliceTemplateStatus struct {
	// Conditions follows the standard Kubernetes condition convention.
	// Phase 7 W1 + T007 stamps:
	//   - Type="Validated"   — composition passed schema validation
	//   - Type="Allocatable" — allocator confirmed bundle can be served
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// FallbackAppliedReason describes the decomposition the template
	// engine applied (when FallbackStrategy=fixed-template-combination
	// actually fired). Format: "decomposed into 1×vir04 + 1×vir08".
	// Empty when no decomposition was needed (composition directly
	// matched existing templates) or when refuse strategy rejected
	// the composition.
	// +optional
	FallbackAppliedReason string `json:"fallbackAppliedReason,omitempty"`

	// ObservedGeneration is the most recent .metadata.generation
	// reconciled by the template_controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// NPUSliceTemplate captures a user-defined complex slice composition
// (e.g. "1× vir04 (prefill) + 1× vir08 (decode)") that the template
// engine + allocator (P7-T-007 + P7-T-105) decompose into existing
// fixed-template requests.
//
// Pod opt-in via label `npu.huawei.com/slice-template=<name>`. Absent
// label → Pod goes through Phase 5 existing whole-NPU allocator path
// (zero regression per ADR-0011 §後果 first bullet).
//
// Cluster-scoped — Phase 9 multi-tenancy may revisit to namespace
// scoping per ADR-0011 §推翻条件 row 4.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=npust
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Parts",type=integer,JSONPath=`.status.observedGeneration`,description="generation reconciled by template engine"
// +kubebuilder:printcolumn:name="Fallback",type=string,JSONPath=`.spec.fallbackStrategy`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type NPUSliceTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NPUSliceTemplateSpec   `json:"spec,omitempty"`
	Status NPUSliceTemplateStatus `json:"status,omitempty"`
}

// NPUSliceTemplateList is the list type for NPUSliceTemplate.
//
// +kubebuilder:object:root=true
type NPUSliceTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []NPUSliceTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NPUSliceTemplate{}, &NPUSliceTemplateList{})
}
