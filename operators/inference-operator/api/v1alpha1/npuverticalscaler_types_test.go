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
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestNPUVerticalScalerRoundTripJSONMarshal covers P8-T-005 acceptance
// case 1/4: NPUVerticalScaler serialises + deserialises preserving all
// fields (spec.target / spec.metric / spec.scaleSlice / status.conditions
// / status.scaleHistory).
func TestNPUVerticalScalerRoundTripJSONMarshal(t *testing.T) {
	now := metav1.NewTime(time.Date(2026, 5, 21, 14, 32, 17, 0, time.UTC))
	orig := &NPUVerticalScaler{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "inference.ocloud.edge.example.com/v1alpha1",
			Kind:       "NPUVerticalScaler",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       "qwen-pd-scaler",
			Namespace:  "ai-edge-demo",
			Generation: 2,
		},
		Spec: NPUVerticalScalerSpec{
			Target: TargetRef{
				APIVersion: "inference.ocloud.edge.example.com/v1alpha1",
				Kind:       "ModelService",
				Name:       "qwen-pd",
				Namespace:  "ai-edge-demo",
			},
			Metric: MetricSpec{
				Type:          MetricTypeNPUUtilization,
				BusyThreshold: 75,
				IdleThreshold: 20,
				WindowSeconds: 300,
			},
			ScaleSlice: ScaleSliceSpec{
				BusyTemplateName: "qwen-pd-busy",
				IdleTemplateName: "qwen-pd-idle",
			},
			CooldownSeconds: 300,
		},
		Status: NPUVerticalScalerStatus{
			Conditions: []metav1.Condition{
				{Type: ConditionActive, Status: metav1.ConditionTrue, Reason: "ScalerReady"},
				{Type: ConditionScalingInProgress, Status: metav1.ConditionFalse, Reason: "NoActiveTransition"},
				{Type: ConditionCooldownActive, Status: metav1.ConditionFalse, Reason: "CooldownExpired"},
			},
			ObservedTarget: &ObservedTargetStatus{
				Name:            "qwen-pd",
				Namespace:       "ai-edge-demo",
				CurrentTemplate: "qwen-pd-busy",
			},
			LastScaleTime: &now,
			ScaleHistory: []ScaleEvent{
				{Time: now, FromTemplate: "qwen-pd-idle", ToTemplate: "qwen-pd-busy", Reason: "metric 87 > busyThreshold 75"},
			},
		},
	}

	raw, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var rt NPUVerticalScaler
	if err := json.Unmarshal(raw, &rt); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if !reflect.DeepEqual(orig.Spec, rt.Spec) {
		t.Fatalf("Spec round-trip mismatch:\norig: %+v\nrt:   %+v", orig.Spec, rt.Spec)
	}
	if rt.Status.ObservedTarget == nil ||
		rt.Status.ObservedTarget.CurrentTemplate != "qwen-pd-busy" {
		t.Fatalf("Status.ObservedTarget round-trip mismatch: %+v", rt.Status.ObservedTarget)
	}
	if len(rt.Status.Conditions) != 3 {
		t.Fatalf("Status.Conditions round-trip len = %d, want 3", len(rt.Status.Conditions))
	}
	if len(rt.Status.ScaleHistory) != 1 ||
		rt.Status.ScaleHistory[0].ToTemplate != "qwen-pd-busy" {
		t.Fatalf("Status.ScaleHistory round-trip mismatch: %+v", rt.Status.ScaleHistory)
	}
	if rt.Spec.CooldownSeconds != 300 {
		t.Fatalf("Spec.CooldownSeconds round-trip mismatch: got %d, want 300",
			rt.Spec.CooldownSeconds)
	}
}

// TestNPUVerticalScalerDeepCopyDetaches covers P8-T-005 acceptance case
// 2/4: DeepCopy returns a fully detached object (mutating the copy must
// not affect the original).
func TestNPUVerticalScalerDeepCopyDetaches(t *testing.T) {
	orig := &NPUVerticalScaler{
		Spec: NPUVerticalScalerSpec{
			Target: TargetRef{Name: "qwen-pd", Namespace: "ai-edge-demo"},
			Metric: MetricSpec{
				Type:          MetricTypeNPUUtilization,
				BusyThreshold: 75,
				IdleThreshold: 20,
				WindowSeconds: 300,
			},
			ScaleSlice: ScaleSliceSpec{
				BusyTemplateName: "qwen-pd-busy",
				IdleTemplateName: "qwen-pd-idle",
			},
			CooldownSeconds: 600,
		},
		Status: NPUVerticalScalerStatus{
			Conditions: []metav1.Condition{
				{Type: ConditionActive, Status: metav1.ConditionTrue, Reason: "ScalerReady"},
			},
			ScaleHistory: []ScaleEvent{
				{ToTemplate: "qwen-pd-busy", Reason: "init"},
			},
		},
	}

	cp := orig.DeepCopy()
	if cp == nil {
		t.Fatal("DeepCopy returned nil")
	}
	// Mutate the copy; verify orig unchanged.
	cp.Spec.Metric.BusyThreshold = 99
	cp.Spec.ScaleSlice.BusyTemplateName = "mutated-busy"
	cp.Status.Conditions[0].Reason = "MutatedReason"
	cp.Status.ScaleHistory = append(cp.Status.ScaleHistory,
		ScaleEvent{ToTemplate: "qwen-pd-idle", Reason: "extra"})

	if orig.Spec.Metric.BusyThreshold != 75 {
		t.Fatalf("DeepCopy did not detach Metric.BusyThreshold: orig = %d, want 75",
			orig.Spec.Metric.BusyThreshold)
	}
	if orig.Spec.ScaleSlice.BusyTemplateName != "qwen-pd-busy" {
		t.Fatalf("DeepCopy did not detach ScaleSlice.BusyTemplateName: orig = %q, want qwen-pd-busy",
			orig.Spec.ScaleSlice.BusyTemplateName)
	}
	if orig.Status.Conditions[0].Reason != "ScalerReady" {
		t.Fatalf("DeepCopy did not detach Conditions[0].Reason: orig = %q, want ScalerReady",
			orig.Status.Conditions[0].Reason)
	}
	if len(orig.Status.ScaleHistory) != 1 {
		t.Fatalf("DeepCopy slice not detached: orig.ScaleHistory len = %d, want 1",
			len(orig.Status.ScaleHistory))
	}
}

// TestNPUVerticalScalerEmptyStatusOmitted covers P8-T-005 acceptance case
// 3/4: A NPUVerticalScaler with empty Status serialises WITHOUT a status
// object (omitempty everywhere). Mirrors NPUSliceTemplate round-trip
// pattern.
func TestNPUVerticalScalerEmptyStatusOmitted(t *testing.T) {
	orig := &NPUVerticalScaler{
		ObjectMeta: metav1.ObjectMeta{Name: "minimal", Namespace: "ai-edge-demo"},
		Spec: NPUVerticalScalerSpec{
			Target: TargetRef{Name: "qwen-pd", Namespace: "ai-edge-demo"},
			Metric: MetricSpec{
				BusyThreshold: 75,
				IdleThreshold: 20,
			},
			ScaleSlice: ScaleSliceSpec{
				BusyTemplateName: "qwen-pd-busy",
				IdleTemplateName: "qwen-pd-idle",
			},
		},
	}
	raw, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(raw)
	if strings.Contains(s, `"conditions"`) {
		t.Fatalf("empty conditions was serialised: %s", s)
	}
	if strings.Contains(s, `"scaleHistory"`) {
		t.Fatalf("empty scaleHistory was serialised: %s", s)
	}
	if strings.Contains(s, `"lastScaleTime"`) {
		t.Fatalf("empty lastScaleTime was serialised: %s", s)
	}
	if strings.Contains(s, `"observedTarget"`) {
		t.Fatalf("empty observedTarget was serialised: %s", s)
	}
}

// TestNPUVerticalScalerEnumValuesArePinned covers P8-T-005 acceptance
// case 4/4: MetricType + Condition constants + Annotation/Finalizer
// constants + defaults are pinned to the ADR-0012 §4 schema (drift here
// = silent migration break for operators with hard-coded literals).
func TestNPUVerticalScalerEnumValuesArePinned(t *testing.T) {
	if MetricTypeNPUUtilization != "NPUUtilization" {
		t.Fatalf("MetricTypeNPUUtilization = %q, want NPUUtilization",
			MetricTypeNPUUtilization)
	}
	if ConditionActive != "Active" {
		t.Fatalf("ConditionActive = %q, want Active", ConditionActive)
	}
	if ConditionScalingInProgress != "ScalingInProgress" {
		t.Fatalf("ConditionScalingInProgress = %q, want ScalingInProgress",
			ConditionScalingInProgress)
	}
	if ConditionCooldownActive != "CooldownActive" {
		t.Fatalf("ConditionCooldownActive = %q, want CooldownActive",
			ConditionCooldownActive)
	}
	if AnnotationManagedBy != "ocloud.edge.example.com/vertical-scaler-managed" {
		t.Fatalf("AnnotationManagedBy = %q, want ocloud.edge.example.com/vertical-scaler-managed",
			AnnotationManagedBy)
	}
	if FinalizerName != "npuverticalscaler.inference.ocloud.edge.example.com/finalizer" {
		t.Fatalf("FinalizerName = %q, want npuverticalscaler.inference.ocloud.edge.example.com/finalizer",
			FinalizerName)
	}
	if DefaultCooldownSeconds != 600 {
		t.Fatalf("DefaultCooldownSeconds = %d, want 600", DefaultCooldownSeconds)
	}
	if DefaultMetricWindowSeconds != 300 {
		t.Fatalf("DefaultMetricWindowSeconds = %d, want 300", DefaultMetricWindowSeconds)
	}
	if MaxScaleHistoryEntries != 10 {
		t.Fatalf("MaxScaleHistoryEntries = %d, want 10", MaxScaleHistoryEntries)
	}
	if MetricTypePrometheusQuery != "PrometheusQuery" {
		t.Fatalf("MetricTypePrometheusQuery = %q, want PrometheusQuery",
			MetricTypePrometheusQuery)
	}
}

// TestNPUVerticalScalerPrometheusQueryRoundTrip covers P9-T-007 acceptance
// case 1/2 (types): MetricSpec round-trips the new PrometheusQuery type +
// PrometheusQuery field. Phase 9 P9-T-007 · ADR-0012 §7 forward note.
func TestNPUVerticalScalerPrometheusQueryRoundTrip(t *testing.T) {
	const customQuery = `avg_over_time(custom_kv_cache_hit_rate{model_service="qwen-pd"}[5m])`
	orig := &NPUVerticalScaler{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "inference.ocloud.edge.example.com/v1alpha1",
			Kind:       "NPUVerticalScaler",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "qwen-promql", Namespace: "ai-edge-demo"},
		Spec: NPUVerticalScalerSpec{
			Target: TargetRef{Name: "qwen-pd", Namespace: "ai-edge-demo"},
			Metric: MetricSpec{
				Type:            MetricTypePrometheusQuery,
				PrometheusQuery: customQuery,
				BusyThreshold:   80,
				IdleThreshold:   30,
				WindowSeconds:   300,
			},
			ScaleSlice: ScaleSliceSpec{
				BusyTemplateName: "qwen-pd-busy",
				IdleTemplateName: "qwen-pd-idle",
			},
		},
	}
	encoded, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var rt NPUVerticalScaler
	if err := json.Unmarshal(encoded, &rt); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if rt.Spec.Metric.Type != MetricTypePrometheusQuery {
		t.Fatalf("Type round-trip = %q, want %q", rt.Spec.Metric.Type, MetricTypePrometheusQuery)
	}
	if rt.Spec.Metric.PrometheusQuery != customQuery {
		t.Fatalf("PrometheusQuery round-trip = %q, want %q", rt.Spec.Metric.PrometheusQuery, customQuery)
	}
}

// TestNPUVerticalScalerPrometheusQueryOmitted covers P9-T-007 acceptance
// case 2/2 (types): when Type=NPUUtilization (default), PrometheusQuery
// field is omitted from JSON output (omitempty respected).
func TestNPUVerticalScalerPrometheusQueryOmitted(t *testing.T) {
	scaler := &NPUVerticalScaler{
		Spec: NPUVerticalScalerSpec{
			Target: TargetRef{Name: "qwen-pd", Namespace: "ai-edge-demo"},
			Metric: MetricSpec{
				Type:          MetricTypeNPUUtilization,
				BusyThreshold: 75,
				IdleThreshold: 20,
				WindowSeconds: 300,
				// PrometheusQuery intentionally empty
			},
			ScaleSlice: ScaleSliceSpec{
				BusyTemplateName: "qwen-pd-busy",
				IdleTemplateName: "qwen-pd-idle",
			},
		},
	}
	encoded, err := json.Marshal(scaler)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(encoded), `"prometheusQuery"`) {
		t.Errorf("prometheusQuery JSON field should be omitted when empty; got: %s", string(encoded))
	}
}
