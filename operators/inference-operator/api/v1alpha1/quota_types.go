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

// Quota is the NPU-aware namespace-scoped resource quota CRD per ADR-0014.
//
// Multi-tenant fair scaling: Quota caps the number of concurrent
// NPUSliceAllocation objects in the namespace and the rate of
// NPUVerticalScaler scale events within a sliding window. Optional whitelist
// pins which NPUSliceTemplate refs the NPUVerticalScaler.spec may reference.
//
// Quota is orthogonal to upstream `core/v1.ResourceQuota` (generic CPU/mem/pod
// count quota): both can coexist in the same namespace. Enforcement happens
// via TWO ValidatingAdmissionWebhooks colocated with the inference-operator
// binary per ADR-0014 §2 Decision C:
//
//   - Webhook A on NPUSliceAllocation CREATE (group npu.ocloud.edge.example.com)
//     checks `status.usage.currentSliceAllocations + 1 <= spec.enforcement.maxSliceAllocations`
//   - Webhook B on NPUVerticalScaler UPDATE (group inference.ocloud.edge.example.com)
//     checks `status.usage.scaleEventsInWindow + 1 <= spec.enforcement.maxScaleEventsPerWindow.count`
//
// The Quota controller reconciles `status.usage` on a 60s tick by listing
// NPUSliceAllocation + NPUVerticalScaler.status.scaleHistory in the namespace.
// The admission webhooks read `status.usage` via an in-memory cache with a
// 5s TTL fallback Get.
//
// API group lives under `inference.ocloud.edge.example.com/v1alpha1` (same
// scheme as ModelService + NPUVerticalScaler) per P9-T-002-fix-001 (the
// initial ADR-0014 v1 picked the bare `ocloud.edge.example.com/v1alpha1`
// group but no existing CRD uses that bare group — fix-001 unifies under
// the inference-operator binary scheme · simpler multi-group avoided).
//
// +kubebuilder:resource:scope=Namespaced,shortName=quota
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="MaxAllocations",type=integer,JSONPath=`.spec.enforcement.maxSliceAllocations`
// +kubebuilder:printcolumn:name="Used",type=integer,JSONPath=`.status.usage.currentSliceAllocations`
// +kubebuilder:printcolumn:name="MaxScaleEvents",type=integer,JSONPath=`.spec.enforcement.maxScaleEventsPerWindow.count`
// +kubebuilder:printcolumn:name="ScaleEventsWindow",type=integer,JSONPath=`.spec.enforcement.maxScaleEventsPerWindow.windowSeconds`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=='Active')].status`
type Quota struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   QuotaSpec   `json:"spec,omitempty"`
	Status QuotaStatus `json:"status,omitempty"`
}

// QuotaSpec describes the namespace-wide NPU-aware quota.
type QuotaSpec struct {
	// Enforcement carries the caps that the admission webhooks enforce.
	// +kubebuilder:validation:Required
	Enforcement QuotaEnforcement `json:"enforcement"`
}

// QuotaEnforcement carries the namespace-wide caps.
type QuotaEnforcement struct {
	// MaxSliceAllocations is the namespace-wide cap on concurrent
	// NPUSliceAllocation objects. 0 (default) means unbounded.
	// +kubebuilder:default=0
	// +kubebuilder:validation:Minimum=0
	MaxSliceAllocations int32 `json:"maxSliceAllocations,omitempty"`

	// MaxScaleEventsPerWindow caps the rate of NPUVerticalScaler.spec
	// scaleSlice mutations (scale events) within a sliding window.
	// Count=0 (default) means unbounded.
	MaxScaleEventsPerWindow ScaleEventRateCap `json:"maxScaleEventsPerWindow,omitempty"`

	// MaxNPUSliceTemplateRefs is an optional whitelist of
	// NPUSliceTemplate names that NPUVerticalScaler.spec.scaleSlice.{busy,idle}TemplateName
	// may reference. Empty (default) allows any template.
	// +optional
	MaxNPUSliceTemplateRefs []string `json:"maxNPUSliceTemplateRefs,omitempty"`
}

// ScaleEventRateCap caps the rate of NPUVerticalScaler scale events.
type ScaleEventRateCap struct {
	// Count is the maximum number of scale events allowed within
	// WindowSeconds. 0 (default) means unbounded.
	// +kubebuilder:default=0
	// +kubebuilder:validation:Minimum=0
	Count int32 `json:"count,omitempty"`

	// WindowSeconds is the sliding-window length in seconds.
	// +kubebuilder:default=3600
	// +kubebuilder:validation:Minimum=60
	WindowSeconds int32 `json:"windowSeconds,omitempty"`
}

// QuotaStatus carries the observed namespace usage + conditions.
type QuotaStatus struct {
	// Conditions tracks Active / EnforcementOK lifecycle per ADR-0014 §5.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Usage records current namespace consumption against the spec caps.
	// +optional
	Usage QuotaUsage `json:"usage,omitempty"`

	// LastSyncTime is the timestamp the controller last refreshed Usage.
	// The admission webhooks use this to detect stale cache snapshots.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`
}

// QuotaUsage records current namespace consumption against the spec caps.
type QuotaUsage struct {
	// CurrentSliceAllocations is the count of NPUSliceAllocation objects
	// currently in the namespace.
	CurrentSliceAllocations int32 `json:"currentSliceAllocations"`

	// ScaleEventsInWindow is the count of NPUVerticalScaler scale events
	// observed across all NPUVerticalScalers in the namespace within the
	// most recent WindowSeconds.
	ScaleEventsInWindow int32 `json:"scaleEventsInWindow"`
}

// +kubebuilder:object:root=true
// QuotaList is a list of Quota objects.
type QuotaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Quota `json:"items"`
}

const (
	// ConditionQuotaActive is True when the Quota controller is reconciling
	// status.usage on its 60s tick.
	ConditionQuotaActive = "Active"

	// ConditionQuotaEnforcementOK is True when the admission webhooks are
	// registered + healthy. False indicates webhook registration / cert
	// chain issues (operator-visible).
	ConditionQuotaEnforcementOK = "EnforcementOK"
)

func init() {
	SchemeBuilder.Register(&Quota{}, &QuotaList{})
}
