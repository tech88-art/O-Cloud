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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NPUSlicePoolSpec is the level-4 (leaf) pool: a slicing policy applied to a
// parent NPUPool's devices. The pool-operator materialises slice instances
// according to Strategy and reports them in Status.Slices.
//
// See `docs/architecture.md` §6.2 (most detailed CRD in the hierarchy).
type NPUSlicePoolSpec struct {
	// NPUPoolRef references the parent NPUPool (Cluster-scoped). NPUSlicePool
	// itself is Namespaced (see scope warning on NPUSlicePool), but the
	// parent is Cluster-scoped, so a name-only reference is used.
	// +kubebuilder:validation:Required
	NPUPoolRef corev1.LocalObjectReference `json:"npuPoolRef"`

	// Strategy selects FixedTemplate or Dynamic. Exactly one of
	// FixedTemplates / DynamicSlicing must be populated to match.
	// +kubebuilder:validation:Required
	Strategy SliceStrategy `json:"strategy"`

	// FixedTemplates lists the discrete slice shapes when Strategy=FixedTemplate.
	// Must be non-empty for that strategy; ignored otherwise.
	// +optional
	// +listType=map
	// +listMapKey=name
	FixedTemplates []SliceTemplate `json:"fixedTemplates,omitempty"`

	// DynamicSlicing parameterises Strategy=Dynamic. Must be non-nil for
	// that strategy; ignored otherwise.
	// +optional
	DynamicSlicing *DynamicSlicingSpec `json:"dynamicSlicing,omitempty"`
}

// NPUSlicePoolStatus reports aggregate slice counts plus per-instance state.
type NPUSlicePoolStatus struct {
	// TotalSlices is the number of slice instances materialised in this pool.
	// +optional
	// +kubebuilder:validation:Minimum=0
	TotalSlices int32 `json:"totalSlices,omitempty"`

	// AllocatedSlices is the count currently bound to a ResourceClaim.
	// +optional
	// +kubebuilder:validation:Minimum=0
	AllocatedSlices int32 `json:"allocatedSlices,omitempty"`

	// AvailableSlices = TotalSlices - AllocatedSlices - (slices in error).
	// +optional
	// +kubebuilder:validation:Minimum=0
	AvailableSlices int32 `json:"availableSlices,omitempty"`

	// Slices is the per-instance live state of every slice in this pool.
	// +optional
	// +listType=map
	// +listMapKey=id
	Slices []SliceStatusEntry `json:"slices,omitempty"`

	// Conditions follow the standard Kubernetes condition convention.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=nsp
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Strategy",type=string,JSONPath=`.spec.strategy`
// +kubebuilder:printcolumn:name="Total",type=integer,JSONPath=`.status.totalSlices`
// +kubebuilder:printcolumn:name="Allocated",type=integer,JSONPath=`.status.allocatedSlices`
// +kubebuilder:printcolumn:name="Available",type=integer,JSONPath=`.status.availableSlices`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// NPUSlicePool is the level-4 (leaf) pool resource. Hierarchy:
// ClusterPool -> NodePool -> NPUPool -> NPUSlicePool.
//
// !!! MULTI-TENANCY / SCOPE WARNING (operators/CLAUDE.md §4) !!!
//
// NPUSlicePool is Namespaced while every ancestor pool
// (ClusterPool / NodePool / NPUPool) is Cluster-scoped. As a direct
// consequence multiple namespaces can declare overlapping slicing policies
// against the SAME physical NPU devices in the parent NPUPool. The CRD
// schema alone does NOT enforce tenant isolation — there is no RBAC or
// admission policy gating cross-namespace ownership of a slice.
//
// Mitigation roadmap (also tracked in docs/phase0-review.md MUST-FIX #6):
//   - Phase 1-2 (current): isolation by convention only. All NPUSlicePool
//     objects MUST be created in the `ocloud-system` namespace; deploy
//     manifests and the demo backend hard-code this. Out-of-namespace
//     objects are not actively rejected but will be ignored by the
//     pool-operator in Phase 3+.
//   - Phase 3: add a ValidatingAdmissionPolicy skeleton under
//     `operators/pool-operator/config/admission/` that rejects NPUSlicePool
//     objects outside `ocloud-system` and prevents two pools from declaring
//     overlapping slices against the same NPUPool.
//   - Phase 9: full multi-tenancy. RBAC ties NPUSlicePool ownership to a
//     tenant CRD federated by Karmada; admission policy enforces
//     per-tenant quota.
//
// Do NOT rely on the schema for isolation today. Treat any cross-namespace
// NPUSlicePool as a misconfiguration to be caught in review.
type NPUSlicePool struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the desired state of NPUSlicePool.
	// +kubebuilder:validation:Required
	Spec NPUSlicePoolSpec `json:"spec"`

	// status defines the observed state of NPUSlicePool.
	// +optional
	Status NPUSlicePoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NPUSlicePoolList contains a list of NPUSlicePool.
type NPUSlicePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NPUSlicePool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NPUSlicePool{}, &NPUSlicePoolList{})
}
