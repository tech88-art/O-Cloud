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

// NPUVerticalScaler observes a busy-idle metric on a target ModelService and
// patches the ModelService.spec.template.sliceTemplate ref between two
// NPUSliceTemplate refs (busy / idle) to drive a "重启切片" (restart-slice)
// vertical scaling pattern.
//
// Per ADR-0012 §1 / §5 the scaler MUST NOT delete Pods directly; it only
// patches ModelService.spec.template.sliceTemplate and lets the existing
// inference-operator ModelService controller drive the rolling restart.
// Pod recreation invokes claim_controller (P8-T-008 wiring) to re-allocate
// per the new template.
//
// +kubebuilder:resource:scope=Namespaced,shortName=npuvs
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.spec.target.name`
// +kubebuilder:printcolumn:name="CurrentTemplate",type=string,JSONPath=`.status.observedTarget.currentTemplate`
// +kubebuilder:printcolumn:name="LastScaleTime",type=date,JSONPath=`.status.lastScaleTime`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=='Active')].status`
type NPUVerticalScaler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NPUVerticalScalerSpec   `json:"spec,omitempty"`
	Status NPUVerticalScalerStatus `json:"status,omitempty"`
}

// NPUVerticalScalerSpec describes the desired scaling behaviour.
type NPUVerticalScalerSpec struct {
	// Target is the ModelService this scaler manages.
	// +kubebuilder:validation:Required
	Target TargetRef `json:"target"`

	// Metric describes how scaling decisions are computed.
	// +kubebuilder:validation:Required
	Metric MetricSpec `json:"metric"`

	// ScaleSlice maps busy/idle states to NPUSliceTemplate refs.
	// +kubebuilder:validation:Required
	ScaleSlice ScaleSliceSpec `json:"scaleSlice"`

	// CooldownSeconds is the minimum gap between scaling events; prevents
	// flapping. Default 600s. Phase 8 sample uses 300s for demo录屏.
	// +kubebuilder:default=600
	// +kubebuilder:validation:Minimum=0
	CooldownSeconds int32 `json:"cooldownSeconds,omitempty"`
}

// TargetRef references the ModelService this scaler manages.
// Phase 8 single-tenant scope: Namespace must match the scaler's own
// namespace; cross-namespace targets are Phase 9 forward (Quota CRD).
type TargetRef struct {
	// APIVersion of the target object.
	// +kubebuilder:default="inference.ocloud.edge.example.com/v1alpha1"
	APIVersion string `json:"apiVersion,omitempty"`

	// Kind of the target object.
	// +kubebuilder:default=ModelService
	// +kubebuilder:validation:Enum=ModelService
	Kind string `json:"kind,omitempty"`

	// Name of the target object.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the target object. Phase 8 requires it to match the
	// scaler's own namespace (single-tenant scope per ADR-0012 §2).
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`
}

// MetricSpec describes the metric source + thresholds.
type MetricSpec struct {
	// Type selects the metric backend. Phase 8 shipped only NPUUtilization;
	// Phase 9 P9-T-007 adds PrometheusQuery per ADR-0012 §7 forward note.
	// +kubebuilder:default=NPUUtilization
	// +kubebuilder:validation:Enum=NPUUtilization;PrometheusQuery
	Type MetricType `json:"type,omitempty"`

	// PrometheusQuery is the custom PromQL expression used when
	// Type=PrometheusQuery. The ingestor passes the expression verbatim
	// to the Prometheus /api/v1/query endpoint and expects a single-vector
	// result (windowed average is the operator's responsibility — the
	// expression should already encode `avg_over_time(... [window])` if
	// desired). MUST be non-empty when Type=PrometheusQuery; ignored when
	// Type=NPUUtilization.
	//
	// Per ADR-0012 §7 + P9-T-007: namespace + model_service label injection
	// is the operator's responsibility (the controller does NOT mutate the
	// expression). NPUUtilization path remains the hardcoded
	// `avg_over_time(ascend_npu_utilization_percent{namespace,model_service}[window])`
	// wired by PrometheusIngestor.Query when CustomPromQL is empty.
	//
	// +optional
	PrometheusQuery string `json:"prometheusQuery,omitempty"`

	// BusyThreshold is the metric value (0-100) above which the scaler
	// switches to busyTemplateName.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	BusyThreshold int32 `json:"busyThreshold"`

	// IdleThreshold is the metric value (0-100) below which the scaler
	// switches to idleTemplateName.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	IdleThreshold int32 `json:"idleThreshold"`

	// WindowSeconds is the sliding-window length for averaging the metric.
	// Default 300s; minimum 30s.
	// +kubebuilder:default=300
	// +kubebuilder:validation:Minimum=30
	WindowSeconds int32 `json:"windowSeconds,omitempty"`
}

// MetricType selects the metric backend. Phase 8 shipped only NPUUtilization;
// Phase 9 P9-T-007 adds PrometheusQuery per ADR-0012 §7 forward note.
// +kubebuilder:validation:Enum=NPUUtilization;PrometheusQuery
type MetricType string

const (
	// MetricTypePrometheusQuery dispatches the ingestor to evaluate
	// MetricSpec.PrometheusQuery verbatim. The operator owns label scoping
	// (no namespace/model_service auto-injection). Phase 9 P9-T-007.
	MetricTypePrometheusQuery MetricType = "PrometheusQuery"
	// MetricTypeNPUUtilization reads ascend_npu_utilization_percent
	// exposed by ascend-npu-exporter-plus, scoped by namespace +
	// model_service label. PromQL shape:
	//   avg_over_time(ascend_npu_utilization_percent{namespace="$ns",
	//     model_service="$ms"}[$window])
	MetricTypeNPUUtilization MetricType = "NPUUtilization"
)

// ScaleSliceSpec maps busy/idle states to NPUSliceTemplate refs.
type ScaleSliceSpec struct {
	// BusyTemplateName references an NPUSliceTemplate (cluster-scoped,
	// ADR-0011 §4) used when metric is above busyThreshold.
	// +kubebuilder:validation:Required
	BusyTemplateName string `json:"busyTemplateName"`

	// IdleTemplateName references an NPUSliceTemplate used when metric is
	// below idleThreshold.
	// +kubebuilder:validation:Required
	IdleTemplateName string `json:"idleTemplateName"`
}

// NPUVerticalScalerStatus tracks observed state + scaling history.
type NPUVerticalScalerStatus struct {
	// Conditions tracks Active / ScalingInProgress / CooldownActive lifecycle.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedTarget records the last observed state of the target
	// ModelService (mainly currentTemplate for the printer column).
	ObservedTarget *ObservedTargetStatus `json:"observedTarget,omitempty"`

	// LastScaleTime is the timestamp of the most recent scaling event;
	// used by the cooldown gate.
	LastScaleTime *metav1.Time `json:"lastScaleTime,omitempty"`

	// ScaleHistory is a rolling window of recent scaling events (oldest
	// first, max 10 entries; FIFO eviction).
	// +kubebuilder:validation:MaxItems=10
	ScaleHistory []ScaleEvent `json:"scaleHistory,omitempty"`
}

// ObservedTargetStatus reflects the last observed state of the target
// ModelService.
type ObservedTargetStatus struct {
	// Name of the observed ModelService (echoes spec.target.name).
	Name string `json:"name"`

	// Namespace of the observed ModelService (echoes spec.target.namespace).
	Namespace string `json:"namespace"`

	// CurrentTemplate is the value of target.spec.template.sliceTemplate
	// as last observed by the scaler. Surfaced as the CurrentTemplate
	// printer column for at-a-glance state.
	CurrentTemplate string `json:"currentTemplate,omitempty"`
}

// ScaleEvent records one scaling transition for audit + UI surfacing.
type ScaleEvent struct {
	// Time of the scaling event.
	// +kubebuilder:validation:Required
	Time metav1.Time `json:"time"`

	// FromTemplate is the sliceTemplate ref before the patch. May be empty
	// for the first observed scaling event (no prior template recorded).
	FromTemplate string `json:"fromTemplate,omitempty"`

	// ToTemplate is the sliceTemplate ref after the patch.
	// +kubebuilder:validation:Required
	ToTemplate string `json:"toTemplate"`

	// Reason is a human-readable explanation
	// (e.g. "metric 87 > busyThreshold 75").
	// +kubebuilder:validation:Required
	Reason string `json:"reason"`
}

// +kubebuilder:object:root=true

// NPUVerticalScalerList contains a list of NPUVerticalScaler.
type NPUVerticalScalerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NPUVerticalScaler `json:"items"`
}

const (
	// ConditionActive indicates the scaler is reconciling its target.
	ConditionActive = "Active"
	// ConditionScalingInProgress is True between patching the target spec
	// and observing the rolling restart complete.
	ConditionScalingInProgress = "ScalingInProgress"
	// ConditionCooldownActive is True when cooldown blocks scaling.
	ConditionCooldownActive = "CooldownActive"

	// AnnotationManagedBy is stamped on the target ModelService when the
	// scaler first patches sliceTemplate. Operators must configure GitOps
	// tools (ArgoCD ignoreDifferences / Flux ignore paths) to ignore
	// .spec.template.sliceTemplate on annotated ModelServices.
	// Per ADR-0012 §5 mutation model.
	AnnotationManagedBy = "ocloud.edge.example.com/vertical-scaler-managed"

	// FinalizerName is set on NPUVerticalScaler at create; removed on
	// delete after stop-scaling cleanup completes. Phase 8 implementation
	// is simple no-op besides finalizer removal; Phase 9 Quota CRD will
	// dec a quota counter via this hook.
	FinalizerName = "npuverticalscaler.inference.ocloud.edge.example.com/finalizer"

	// DefaultCooldownSeconds is the default scaling cooldown.
	DefaultCooldownSeconds = 600
	// DefaultMetricWindowSeconds is the default sliding-window length.
	DefaultMetricWindowSeconds = 300
	// MaxScaleHistoryEntries is the rolling-window size for
	// Status.ScaleHistory (FIFO eviction).
	MaxScaleHistoryEntries = 10
)

func init() {
	SchemeBuilder.Register(&NPUVerticalScaler{}, &NPUVerticalScalerList{})
}
