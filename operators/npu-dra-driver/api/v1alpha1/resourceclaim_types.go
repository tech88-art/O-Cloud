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

// Annotation keys set by Ocloud consumers on upstream
// resource.k8s.io/v1beta1.ResourceClaim objects. Schema documented in the
// package doc (types.go). Consumed by the claim controller skeleton landing
// at P4-T-006 (logs only in Phase 4; real allocation Phase 5+ per ADR-0009).
const (
	// AnnotationModelServiceRef binds this claim to a Phase 5
	// inference-operator ModelService. Phase 4 controller logs the value
	// but performs no binding. The Phase 5 PD Router webhook (ADR-0008)
	// reads this annotation to route Prefill / Decode pods to the right
	// claim allocation.
	AnnotationModelServiceRef = "ocloud.edge.example.com/model-service-ref"

	// AnnotationPreferredPool expresses a soft affinity to a named
	// NPUSlicePool. The Phase 5 allocator uses this as a tiebreaker when
	// multiple pools satisfy a claim's device requirements.
	AnnotationPreferredPool = "ocloud.edge.example.com/preferred-pool"
)

// AscendClaimAnnotations is the typed view of Ocloud-specific annotations
// carried on upstream ResourceClaim.metadata.annotations.
//
// Use ToMap / FromMap to convert between this typed view and the upstream
// annotations map. Unknown keys are tolerated on FromMap so future Ocloud
// (or third-party) annotations round-trip cleanly through the conversion.
//
// +kubebuilder:object:generate=true
type AscendClaimAnnotations struct {
	// ModelServiceRef is the namespace/name reference to the
	// inference-operator ModelService that owns this claim. Empty when the
	// claim is not part of a managed ModelService (Phase 4 manual claims
	// for testing the publisher leave this blank).
	ModelServiceRef string `json:"modelServiceRef,omitempty"`

	// PreferredPool is the soft affinity NPUSlicePool name. Empty for "no
	// preference, allocator picks any pool that satisfies the request".
	PreferredPool string `json:"preferredPool,omitempty"`
}

// ToMap renders this typed view as an annotations map suitable for
// merging into ResourceClaim.metadata.annotations. Empty fields are omitted
// so the resulting map only contains keys the caller actually set.
func (a AscendClaimAnnotations) ToMap() map[string]string {
	out := make(map[string]string)
	if a.ModelServiceRef != "" {
		out[AnnotationModelServiceRef] = a.ModelServiceRef
	}
	if a.PreferredPool != "" {
		out[AnnotationPreferredPool] = a.PreferredPool
	}
	return out
}

// AscendClaimAnnotationsFromMap parses Ocloud annotations from a
// ResourceClaim.metadata.annotations map. Unknown keys are silently ignored
// (warn-not-error contract identical to AscendDeviceFromUpstream).
func AscendClaimAnnotationsFromMap(m map[string]string) AscendClaimAnnotations {
	return AscendClaimAnnotations{
		ModelServiceRef: m[AnnotationModelServiceRef],
		PreferredPool:   m[AnnotationPreferredPool],
	}
}
