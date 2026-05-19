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

// NPUSliceAllocation phase enum values for Status.Phase.
//
// Lifecycle (per ADR-0009 §5 step 3 + arch §6.8):
//
//   - "Allocated": the owning ResourceClaim is allocated and the
//     NPUSliceAllocation entry mirrors that allocation. Status.Conditions
//     [Type=Available, Status=True] accompanies this phase.
//   - "Released": the owning ResourceClaim was deleted or its
//     status.allocation was cleared. NPUSliceAllocation lingers briefly
//     (informational; GC cascade removes it via owner-ref shortly).
//   - "Orphaned": the owning ResourceClaim is gone (owner-ref dangling)
//     for longer than the grace period (30s by default). The controller
//     (T005) drives this transition + emits a Warning event so audit
//     log readers spot stale entries.
const (
	NPUSliceAllocationPhaseAllocated = "Allocated"
	NPUSliceAllocationPhaseReleased  = "Released"
	NPUSliceAllocationPhaseOrphaned  = "Orphaned"
)

// Condition Type values written to NPUSliceAllocation.Status.Conditions.
const (
	// ConditionAvailable is True when the allocation is current and the
	// device is actually bound to the owning claim. False with reason
	// "ClaimDeleted" once the owning claim disappears (transient before
	// GC), or reason "Orphaned" once the grace period expires.
	ConditionAvailable = "Available"
)

// SliceReference identifies one NPU device within the DRA pool model.
//
// Field shape mirrors resource.k8s.io/v1beta1.DeviceRequestAllocationResult
// (driver / pool / device) so the T005 controller can copy from a
// ResourceClaim's status.allocation result directly. NodeName is
// duplicated from the slice's ResourceSliceSpec.NodeName for downstream
// consumers (Phase 6 scheduler-plugin reverse lookup, Phase 9 quota
// per-node accounting) so they do not have to re-join against the
// ResourceSlice.
type SliceReference struct {
	// Driver is the DRA driver name (always npu.ocloud.edge.example.com
	// in Phase 5).
	// +kubebuilder:validation:Required
	Driver string `json:"driver"`

	// Pool is the ResourceSlice ResourcePool.Name — by convention the
	// node hostname (Phase 5 simulator + real Ascend Phase 7 both follow
	// this convention).
	// +kubebuilder:validation:Required
	Pool string `json:"pool"`

	// Device is the per-slice Device.Name (DNS label, e.g.
	// `<node>-npu-<index>`).
	// +kubebuilder:validation:Required
	Device string `json:"device"`
}

// NPUSliceAllocationSpec captures the immutable allocation outcome.
//
// Phase 5 invariants:
//   - Spec is set by the npu-dra-driver claim controller (T002) at the
//     same moment status.allocation is written on the owning
//     ResourceClaim. It is never mutated thereafter — releases delete
//     the object, not update it.
//   - One NPUSliceAllocation per (claim, device) tuple. Multi-device
//     claims (Phase 6+) produce multiple NPUSliceAllocation entries
//     under the same owner-ref.
type NPUSliceAllocationSpec struct {
	// ClaimRef is an immutable reference back to the upstream
	// resource.k8s.io/v1beta1.ResourceClaim that owns this allocation.
	// The owning ResourceClaim is also set as the OwnerReference on
	// metadata.ownerReferences so K8s garbage collection cascades the
	// delete; ClaimRef carries the namespace + name + UID for ergonomic
	// reads (avoid round-tripping through ownerRefs).
	// +kubebuilder:validation:Required
	ClaimRef corev1.ObjectReference `json:"claimRef"`

	// SliceRef identifies the picked device within the DRA pool model.
	// +kubebuilder:validation:Required
	SliceRef SliceReference `json:"sliceRef"`

	// NodeName is the node hosting the device. Duplicated from
	// SliceRef.Pool by convention but tracked separately so a Phase 7
	// real-Ascend deployment where pool != node can express both.
	// +kubebuilder:validation:Required
	NodeName string `json:"nodeName"`

	// AICores is the allocated AI-core count. For SliceStrategyDynamic
	// devices this is the requested per-slice AI-core count; for
	// SliceStrategyFixedTemplate devices it is the device's full
	// AI-core capacity (e.g. 32 for an Ascend 910B whole NPU).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Required
	AICores int32 `json:"aiCores"`

	// ModelServiceRef carries the inference-operator ModelService that
	// triggered this allocation, when set. Mirrors the annotation
	// `ocloud.edge.example.com/model-service-ref` on the owning claim.
	// Empty for manually-created claims (Phase 5 tests, smoke runs).
	// +optional
	ModelServiceRef string `json:"modelServiceRef,omitempty"`
}

// NPUSliceAllocationStatus tracks the lifecycle of one allocation.
type NPUSliceAllocationStatus struct {
	// Phase mirrors the lifecycle described in the NPUSliceAllocationPhase*
	// constants.
	// +kubebuilder:validation:Enum=Allocated;Released;Orphaned
	// +optional
	Phase string `json:"phase,omitempty"`

	// Conditions surfaces the Available signal + future Phase 6
	// scheduler-plugin readiness signals.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// AllocatedAt records the wall-clock time of the allocation
	// transition (the moment Spec was written). Useful as a tiebreaker
	// for the Phase 9 quota controller when two NPUSliceAllocations
	// reference the same device through a race.
	// +optional
	AllocatedAt *metav1.Time `json:"allocatedAt,omitempty"`
}

// NPUSliceAllocation records one allocator decision for audit, reverse
// lookup, and Phase 9 quota purposes.
//
// Cluster-scoped: physical NPU devices are cluster-wide resources, so
// the audit log lives at the same scope. Namespace-scoped consumers
// (inference-operator ModelService, multi-tenant quota controllers)
// reach into this list via label selectors when they need per-namespace
// views.
//
// The npu-dra-driver claim controller (T005) creates one
// NPUSliceAllocation per ResourceClaim allocation it writes. The
// canonical source of truth for "is this device allocated" remains the
// claim's `Status.Allocation` — NPUSliceAllocation is a parallel index
// that Phase 6 scheduler-plugin + Phase 9 quota controller read but the
// allocator (T002 Greedy / BestFit) does not.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=npua
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Claim",type=string,JSONPath=`.spec.claimRef.name`
// +kubebuilder:printcolumn:name="Device",type=string,JSONPath=`.spec.sliceRef.device`
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeName`
// +kubebuilder:printcolumn:name="AICores",type=integer,JSONPath=`.spec.aiCores`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type NPUSliceAllocation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NPUSliceAllocationSpec   `json:"spec,omitempty"`
	Status NPUSliceAllocationStatus `json:"status,omitempty"`
}

// NPUSliceAllocationList is the list type for NPUSliceAllocation.
//
// +kubebuilder:object:root=true
type NPUSliceAllocationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []NPUSliceAllocation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NPUSliceAllocation{}, &NPUSliceAllocationList{})
}
