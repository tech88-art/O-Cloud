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

// ModelServicePhase enumerates the high-level lifecycle phase. Phase 4
// controller is absent so all ModelServices stay at PhasePending.
type ModelServicePhase string

const (
	// PhasePending means the ModelService has been created but the
	// controller has not yet started provisioning (Phase 4 default since
	// no controller exists; Phase 5 lifts to PhaseProvisioning once
	// admission lands).
	PhasePending ModelServicePhase = "Pending"

	// PhaseProvisioning means the controller is materialising the PD-pair
	// pods + ResourceClaims (Phase 5+).
	PhaseProvisioning ModelServicePhase = "Provisioning"

	// PhaseReady means the PD-pair is serving traffic with all replicas
	// healthy (Phase 5+).
	PhaseReady ModelServicePhase = "Ready"

	// PhaseFailed means provisioning failed and the controller has
	// surrendered (Phase 5+).
	PhaseFailed ModelServicePhase = "Failed"
)

// ModelSpec describes the model + image to serve.
type ModelSpec struct {
	// Image is the vllm-ascend container image reference
	// (e.g. registry.example.com/vllm-ascend:v0.11.0).
	// +kubebuilder:validation:Required
	Image string `json:"image"`

	// ModelPath is the model weights path inside the image or on a PVC
	// mounted by the pod (e.g. /models/llama-7b).
	// +kubebuilder:validation:Required
	ModelPath string `json:"modelPath"`
}

// PDReplicaSpec configures one side of the PD-pair.
type PDReplicaSpec struct {
	// Replicas is the number of pods on this side of the PD-pair.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	Replicas int32 `json:"replicas"`
}

// PDPairSpec configures the Prefill+Decode pair. The Phase 5 PD Router
// webhook (ADR-0008) reads `routerLabel` to learn which label key on a
// pod identifies its PD role; the controller stamps that label on the
// per-side Deployments it creates.
type PDPairSpec struct {
	// Prefill is the Prefill-side replicas configuration.
	// +kubebuilder:validation:Required
	Prefill PDReplicaSpec `json:"prefill"`

	// Decode is the Decode-side replicas configuration.
	// +kubebuilder:validation:Required
	Decode PDReplicaSpec `json:"decode"`

	// RouterLabel is the label key the Phase 5 PD Router webhook
	// reads to route traffic across the PD-pair. Conventionally
	// "inference.ocloud.edge.example.com/pd-role" with values
	// "prefill" / "decode". Empty disables PD routing (the controller
	// treats the ModelService as a single-pod inference deployment).
	// +kubebuilder:default="inference.ocloud.edge.example.com/pd-role"
	RouterLabel string `json:"routerLabel,omitempty"`

	// ProxyImage opts in to vllm-ascend `disaggregated_prefill_v1`
	// proxy_server sidecar (P6-T-105). When non-empty, the controller
	// adds a second container ("pd-proxy") to each PD-pair Pod that
	// runs the proxy_server process with PREFILL_HOST / DECODE_HOST
	// env-vars derived from the PD-pair sibling Service endpoints.
	// Empty (default) keeps the Phase 5 single-container behavior:
	// each Pod runs only Spec.Model.Image with --pd-role flag.
	//
	// **P7-T-102 gating (2026-05-20)**: vllm-ascend v0.12+ IS GA
	// (latest stable v0.18.0 · 2024-04-30 per release page). Chart
	// default for this field STAYS empty pending Phase 10 demo polish
	// verification of (a) CI image-pull access from GHA runners
	// (~5GB) and (b) `quay.io/vllm-project/vllm-ascend` exact tag
	// convention. Operators wanting the proxy sidecar today set
	// e.g. `proxyImage: "quay.io/vllm-project/vllm-ascend:v0.18.0"`
	// explicitly. See DESIGN.md §5.0.1.
	// +optional
	ProxyImage string `json:"proxyImage,omitempty"`

	// FallbackImage is used in CI / kind smoke environments where
	// pulling the real ~5GB vllm-ascend image is too expensive. When
	// non-empty, the controller substitutes it for Spec.Model.Image
	// on the PD-pair containers — operators can ship a busybox-style
	// stand-in that exits 0 immediately so the readiness probe
	// completes the smoke run without real inference.
	//
	// Production deployments leave this empty.
	// +optional
	FallbackImage string `json:"fallbackImage,omitempty"`
}

// ModelServiceSpec is the Spec block of a ModelService.
type ModelServiceSpec struct {
	// Model identifies the image + weights path.
	// +kubebuilder:validation:Required
	Model ModelSpec `json:"model"`

	// PDPair configures the prefill/decode split per ADR-0008.
	// +kubebuilder:validation:Required
	PDPair PDPairSpec `json:"pdPair"`

	// NPUSlicePoolRef binds this ModelService to a NPUSlicePool the
	// Phase 5 controller will allocate slices from. The reference is
	// namespace-local (ModelService is namespaced; NPUSlicePool also
	// namespaced post-Phase-3 — same-namespace convention via
	// operators/CLAUDE.md §4 "Phase 1-2 隔离 by convention only").
	// +kubebuilder:validation:Required
	NPUSlicePoolRef corev1.LocalObjectReference `json:"npuSlicePoolRef"`

	// SchedulerOverride opts out of the default Phase 7 auto-stamp of
	// spec.schedulerName on PD-pair Pods. Phase 7 P7-T-003 (ADR-0011 §1 +
	// closes known-issues #11) wires deployment_builder to stamp
	// spec.schedulerName="npu-scheduler" by default — putting PD-pair
	// Pods on our HCCS-aware scheduler-plugin (operators/scheduler-plugin/
	// per ADR-0010 §1). When this field is non-nil, the controller stamps
	// the override value instead; when nil OR empty-string-pointer, the
	// default "npu-scheduler" applies.
	//
	// Typical operator overrides:
	//   - "default-scheduler" — opt out of HCCS-aware placement (e.g. for
	//     a single-replica diagnostic ModelService that doesn't need ring
	//     affinity)
	//   - "<custom-scheduler-name>" — route to a third scheduler (e.g.
	//     Volcano during Phase 8 training-job experiments)
	//
	// Defaults to nil ("auto-stamp npu-scheduler"). Empty pointer
	// (`*string` to "") also resolves to the default for safety — see
	// internal/controller/deployment_builder.go effectiveSchedulerName.
	// +optional
	SchedulerOverride *string `json:"schedulerOverride,omitempty"`
}

// ModelServiceStatus is the Status block of a ModelService.
type ModelServiceStatus struct {
	// Phase is the high-level lifecycle phase. See ModelServicePhase
	// for the enumeration. Phase 4 controller is absent so phase stays
	// at PhasePending after creation.
	// +kubebuilder:validation:Enum=Pending;Provisioning;Ready;Failed
	// +kubebuilder:default="Pending"
	Phase ModelServicePhase `json:"phase,omitempty"`

	// Conditions follow the standard Kubernetes condition convention.
	// Phase 5 controller adds:
	//   - Type="Available"      — at least one Pod ready per side
	//   - Type="ProgressDeadline" — provisioning timed out
	//   - Type="AllocationReady" — npu-dra-driver ResourceClaims bound
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent .metadata.generation reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=ms
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.model.image`
// +kubebuilder:printcolumn:name="Prefill",type=integer,JSONPath=`.spec.pdPair.prefill.replicas`
// +kubebuilder:printcolumn:name="Decode",type=integer,JSONPath=`.spec.pdPair.decode.replicas`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ModelService is the Schema for the modelservices API. Phase 4 T103
// ships the type only; the controller body lands Phase 5 per ADR-0008
// (PD Router webhook impl) + ADR-0009 (ResourceClaim allocation via
// npu-dra-driver).
type ModelService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModelServiceSpec   `json:"spec,omitempty"`
	Status ModelServiceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ModelServiceList contains a list of ModelService.
type ModelServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ModelService `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ModelService{}, &ModelServiceList{})
}
