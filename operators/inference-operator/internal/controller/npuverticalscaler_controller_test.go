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

package controller

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	inferencev1alpha1 "github.com/tech88-art/O-Cloud/operators/inference-operator/api/v1alpha1"
	"github.com/tech88-art/O-Cloud/operators/inference-operator/internal/metrics"
)

// fixedNow returns a deterministic clock for cooldown / scaleHistory
// assertions. Tests pass `Now: fixedNow(t)` so the reconciler's now()
// method returns a known value.
func fixedNow(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// buildScalerFixture returns a ready-to-reconcile NPUVerticalScaler with
// sensible defaults; tests adjust fields as needed.
func buildScalerFixture(modifiers ...func(*inferencev1alpha1.NPUVerticalScaler)) *inferencev1alpha1.NPUVerticalScaler {
	s := &inferencev1alpha1.NPUVerticalScaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "qwen-pd-scaler",
			Namespace: "ai-edge-demo",
		},
		Spec: inferencev1alpha1.NPUVerticalScalerSpec{
			Target: inferencev1alpha1.TargetRef{
				APIVersion: "inference.ocloud.edge.example.com/v1alpha1",
				Kind:       "ModelService",
				Name:       "qwen-pd",
				Namespace:  "ai-edge-demo",
			},
			Metric: inferencev1alpha1.MetricSpec{
				Type:          inferencev1alpha1.MetricTypeNPUUtilization,
				BusyThreshold: 75,
				IdleThreshold: 20,
				WindowSeconds: 300,
			},
			ScaleSlice: inferencev1alpha1.ScaleSliceSpec{
				BusyTemplateName: "qwen-pd-busy",
				IdleTemplateName: "qwen-pd-idle",
			},
			CooldownSeconds: 300,
		},
	}
	for _, m := range modifiers {
		m(s)
	}
	return s
}

// buildTargetFixture returns a minimal ModelService matching the default
// target ref. modifiers can add annotations etc.
func buildTargetFixture(modifiers ...func(*inferencev1alpha1.ModelService)) *inferencev1alpha1.ModelService {
	m := &inferencev1alpha1.ModelService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "qwen-pd",
			Namespace: "ai-edge-demo",
		},
		Spec: inferencev1alpha1.ModelServiceSpec{
			Model:           inferencev1alpha1.ModelSpec{Image: "vllm:test", ModelPath: "/m"},
			PDPair:          inferencev1alpha1.PDPairSpec{Prefill: inferencev1alpha1.PDReplicaSpec{Replicas: 1}, Decode: inferencev1alpha1.PDReplicaSpec{Replicas: 1}},
			NPUSlicePoolRef: corev1.LocalObjectReference{Name: "ascend-910b-pool"},
		},
	}
	for _, mod := range modifiers {
		mod(m)
	}
	return m
}

// newScalerReconciler builds a reconciler with the supplied scaler +
// target objects pre-seeded into the fake client + canned ingestor
// results.
func newScalerReconciler(t *testing.T, now time.Time, canned []metrics.IngestorResult,
	scaler *inferencev1alpha1.NPUVerticalScaler,
	target *inferencev1alpha1.ModelService) (*NPUVerticalScalerReconciler, *metrics.FakeIngestor) {
	t.Helper()
	cli := newFakeClient(t, scaler, target)
	ing := metrics.NewFakeIngestor(canned)
	r := &NPUVerticalScalerReconciler{
		Client:   cli,
		Scheme:   newTestScheme(t),
		Recorder: newFakeRecorder(32),
		Ingestor: ing,
		Now:      fixedNow(now),
	}
	return r, ing
}

// TestNPUVerticalScalerReconcile_NoDataStays covers P8-T-007 case 1/6:
// Ingestor returns NoData=true → controller does NOT patch the target +
// requeues 30s + Condition Active=True reason NoDataThisTick.
func TestNPUVerticalScalerReconcile_NoDataStays(t *testing.T) {
	now := time.Date(2026, 5, 21, 14, 0, 0, 0, time.UTC)
	scaler := buildScalerFixture()
	target := buildTargetFixture()
	r, _ := newScalerReconciler(t, now, []metrics.IngestorResult{{NoData: true}}, scaler, target)

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{
		Name: scaler.Name, Namespace: scaler.Namespace,
	}})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.RequeueAfter != 30*time.Second {
		t.Fatalf("RequeueAfter = %v, want 30s on NoData", res.RequeueAfter)
	}

	// Target annotation must be unchanged.
	var refreshed inferencev1alpha1.ModelService
	if err := r.Client.Get(context.Background(), types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, &refreshed); err != nil {
		t.Fatalf("Get target: %v", err)
	}
	if v := refreshed.GetAnnotations()[annotationSliceTemplate]; v != "" {
		t.Fatalf("annotation %q = %q, want empty on NoData", annotationSliceTemplate, v)
	}
	// Scaler must not have appended ScaleEvent on NoData.
	var sr inferencev1alpha1.NPUVerticalScaler
	if err := r.Client.Get(context.Background(), types.NamespacedName{Name: scaler.Name, Namespace: scaler.Namespace}, &sr); err != nil {
		t.Fatalf("Get scaler: %v", err)
	}
	if len(sr.Status.ScaleHistory) != 0 {
		t.Fatalf("ScaleHistory len = %d, want 0 on NoData", len(sr.Status.ScaleHistory))
	}
}

// TestNPUVerticalScalerReconcile_BusyThresholdCross covers P8-T-007 case
// 2/6: metric value > busyThreshold → patch target annotation to
// busyTemplateName + append ScaleEvent + lastScaleTime set + requeue 60s.
func TestNPUVerticalScalerReconcile_BusyThresholdCross(t *testing.T) {
	now := time.Date(2026, 5, 21, 14, 0, 0, 0, time.UTC)
	scaler := buildScalerFixture()
	target := buildTargetFixture(func(m *inferencev1alpha1.ModelService) {
		m.SetAnnotations(map[string]string{annotationSliceTemplate: "qwen-pd-idle"})
	})
	r, _ := newScalerReconciler(t, now, []metrics.IngestorResult{{Value: 87}}, scaler, target)

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{
		Name: scaler.Name, Namespace: scaler.Namespace,
	}})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.RequeueAfter != 60*time.Second {
		t.Fatalf("RequeueAfter = %v, want 60s post-scale", res.RequeueAfter)
	}

	var refreshed inferencev1alpha1.ModelService
	if err := r.Client.Get(context.Background(), types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, &refreshed); err != nil {
		t.Fatalf("Get target: %v", err)
	}
	if v := refreshed.GetAnnotations()[annotationSliceTemplate]; v != "qwen-pd-busy" {
		t.Fatalf("annotation %q = %q, want qwen-pd-busy", annotationSliceTemplate, v)
	}
	if v := refreshed.GetAnnotations()[inferencev1alpha1.AnnotationManagedBy]; v != scaler.Name {
		t.Fatalf("annotation %q = %q, want %s", inferencev1alpha1.AnnotationManagedBy, v, scaler.Name)
	}

	var sr inferencev1alpha1.NPUVerticalScaler
	if err := r.Client.Get(context.Background(), types.NamespacedName{Name: scaler.Name, Namespace: scaler.Namespace}, &sr); err != nil {
		t.Fatalf("Get scaler: %v", err)
	}
	if len(sr.Status.ScaleHistory) != 1 {
		t.Fatalf("ScaleHistory len = %d, want 1 post-scale", len(sr.Status.ScaleHistory))
	}
	if sr.Status.ScaleHistory[0].ToTemplate != "qwen-pd-busy" {
		t.Fatalf("ScaleHistory[0].ToTemplate = %q, want qwen-pd-busy",
			sr.Status.ScaleHistory[0].ToTemplate)
	}
	if sr.Status.LastScaleTime == nil {
		t.Fatalf("LastScaleTime nil, want set")
	}
}

// TestNPUVerticalScalerReconcile_IdleThresholdCross covers P8-T-007 case
// 3/6: metric value < idleThreshold → patch to idleTemplateName.
func TestNPUVerticalScalerReconcile_IdleThresholdCross(t *testing.T) {
	now := time.Date(2026, 5, 21, 14, 0, 0, 0, time.UTC)
	scaler := buildScalerFixture()
	target := buildTargetFixture(func(m *inferencev1alpha1.ModelService) {
		m.SetAnnotations(map[string]string{annotationSliceTemplate: "qwen-pd-busy"})
	})
	r, _ := newScalerReconciler(t, now, []metrics.IngestorResult{{Value: 12}}, scaler, target)

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{
		Name: scaler.Name, Namespace: scaler.Namespace,
	}}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	var refreshed inferencev1alpha1.ModelService
	if err := r.Client.Get(context.Background(), types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, &refreshed); err != nil {
		t.Fatalf("Get target: %v", err)
	}
	if v := refreshed.GetAnnotations()[annotationSliceTemplate]; v != "qwen-pd-idle" {
		t.Fatalf("annotation %q = %q, want qwen-pd-idle on idle cross", annotationSliceTemplate, v)
	}
}

// TestNPUVerticalScalerReconcile_CooldownBlocks covers P8-T-007 case 4/6:
// scaler has LastScaleTime within cooldown window → controller does NOT
// patch + requeues with the remainder + CooldownActive=True condition.
func TestNPUVerticalScalerReconcile_CooldownBlocks(t *testing.T) {
	now := time.Date(2026, 5, 21, 14, 0, 0, 0, time.UTC)
	lastScale := metav1.NewTime(now.Add(-120 * time.Second)) // 2 min ago, cooldown 300s
	scaler := buildScalerFixture(func(s *inferencev1alpha1.NPUVerticalScaler) {
		s.Status.LastScaleTime = &lastScale
	})
	target := buildTargetFixture(func(m *inferencev1alpha1.ModelService) {
		m.SetAnnotations(map[string]string{annotationSliceTemplate: "qwen-pd-idle"})
	})
	r, _ := newScalerReconciler(t, now, []metrics.IngestorResult{{Value: 87}}, scaler, target)

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{
		Name: scaler.Name, Namespace: scaler.Namespace,
	}})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	// Cooldown remainder should be ~180s (300 - 120).
	if res.RequeueAfter < 170*time.Second || res.RequeueAfter > 190*time.Second {
		t.Fatalf("RequeueAfter = %v, want ~180s (cooldown remainder)", res.RequeueAfter)
	}

	var refreshed inferencev1alpha1.ModelService
	if err := r.Client.Get(context.Background(), types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, &refreshed); err != nil {
		t.Fatalf("Get target: %v", err)
	}
	// Annotation must NOT have flipped — cooldown blocks the patch.
	if v := refreshed.GetAnnotations()[annotationSliceTemplate]; v != "qwen-pd-idle" {
		t.Fatalf("annotation %q = %q, want unchanged (cooldown blocks)",
			annotationSliceTemplate, v)
	}

	var sr inferencev1alpha1.NPUVerticalScaler
	if err := r.Client.Get(context.Background(), types.NamespacedName{Name: scaler.Name, Namespace: scaler.Namespace}, &sr); err != nil {
		t.Fatalf("Get scaler: %v", err)
	}
	// CooldownActive condition must be True.
	var cd *metav1.Condition
	for i := range sr.Status.Conditions {
		if sr.Status.Conditions[i].Type == inferencev1alpha1.ConditionCooldownActive {
			cd = &sr.Status.Conditions[i]
			break
		}
	}
	if cd == nil || cd.Status != metav1.ConditionTrue {
		t.Fatalf("ConditionCooldownActive = %v, want True", cd)
	}
}

// TestNPUVerticalScalerReconcile_ScaleHistoryRotates covers P8-T-007 case
// 5/6: scaleHistory enforces MaxScaleHistoryEntries (10) FIFO eviction.
// Seeds 10 prior events + triggers one more → oldest evicted, total stays 10.
func TestNPUVerticalScalerReconcile_ScaleHistoryRotates(t *testing.T) {
	now := time.Date(2026, 5, 21, 14, 0, 0, 0, time.UTC)
	// Seed 10 prior events (no LastScaleTime so cooldown doesn't block).
	prior := make([]inferencev1alpha1.ScaleEvent, 10)
	for i := 0; i < 10; i++ {
		prior[i] = inferencev1alpha1.ScaleEvent{
			Time:         metav1.NewTime(now.Add(time.Duration(-(10 - i)) * time.Hour)),
			FromTemplate: "qwen-pd-idle",
			ToTemplate:   "qwen-pd-busy",
			Reason:       "seeded",
		}
	}
	scaler := buildScalerFixture(func(s *inferencev1alpha1.NPUVerticalScaler) {
		s.Status.ScaleHistory = prior
		// No LastScaleTime → no cooldown.
	})
	target := buildTargetFixture(func(m *inferencev1alpha1.ModelService) {
		m.SetAnnotations(map[string]string{annotationSliceTemplate: "qwen-pd-idle"})
	})
	r, _ := newScalerReconciler(t, now, []metrics.IngestorResult{{Value: 90}}, scaler, target)

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{
		Name: scaler.Name, Namespace: scaler.Namespace,
	}}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	var sr inferencev1alpha1.NPUVerticalScaler
	if err := r.Client.Get(context.Background(), types.NamespacedName{Name: scaler.Name, Namespace: scaler.Namespace}, &sr); err != nil {
		t.Fatalf("Get scaler: %v", err)
	}
	if len(sr.Status.ScaleHistory) != inferencev1alpha1.MaxScaleHistoryEntries {
		t.Fatalf("ScaleHistory len = %d, want %d (FIFO cap)",
			len(sr.Status.ScaleHistory), inferencev1alpha1.MaxScaleHistoryEntries)
	}
	// Newest entry should be the just-triggered one.
	tail := sr.Status.ScaleHistory[len(sr.Status.ScaleHistory)-1]
	if tail.Reason == "seeded" {
		t.Fatalf("ScaleHistory tail Reason = %q, want fresh event (FIFO eviction failed)",
			tail.Reason)
	}
	if tail.ToTemplate != "qwen-pd-busy" {
		t.Fatalf("ScaleHistory tail ToTemplate = %q, want qwen-pd-busy", tail.ToTemplate)
	}
}

// TestNPUVerticalScalerReconcile_TargetNotFound covers P8-T-007 case 6/6:
// target ModelService missing → Condition Active=False reason TargetNotFound
// + scaler does not requeue + no patch attempted.
func TestNPUVerticalScalerReconcile_TargetNotFound(t *testing.T) {
	now := time.Date(2026, 5, 21, 14, 0, 0, 0, time.UTC)
	scaler := buildScalerFixture()
	// NO target seeded → Get returns NotFound.
	cli := newFakeClient(t, scaler)
	r := &NPUVerticalScalerReconciler{
		Client:   cli,
		Scheme:   newTestScheme(t),
		Recorder: newFakeRecorder(32),
		Ingestor: metrics.NewFakeIngestor(nil),
		Now:      fixedNow(now),
	}

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{
		Name: scaler.Name, Namespace: scaler.Namespace,
	}})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter = %v, want 0 on TargetNotFound", res.RequeueAfter)
	}

	var sr inferencev1alpha1.NPUVerticalScaler
	if err := r.Client.Get(context.Background(), types.NamespacedName{Name: scaler.Name, Namespace: scaler.Namespace}, &sr); err != nil {
		t.Fatalf("Get scaler: %v", err)
	}
	var active *metav1.Condition
	for i := range sr.Status.Conditions {
		if sr.Status.Conditions[i].Type == inferencev1alpha1.ConditionActive {
			active = &sr.Status.Conditions[i]
			break
		}
	}
	if active == nil || active.Status != metav1.ConditionFalse {
		t.Fatalf("ConditionActive = %v, want False on TargetNotFound", active)
	}
	if active.Reason != "TargetNotFound" {
		t.Fatalf("ConditionActive.Reason = %q, want TargetNotFound", active.Reason)
	}
}

// TestNPUVerticalScalerReconcile_ClusterCapDefers covers P13-T-204: a scale
// that would cross busyThreshold is DEFERRED (target annotation unchanged · no
// ScaleEvent) when the cluster-wide ClusterQuota scale-rate cap is reached,
// even though the per-scaler cooldown + metric say "scale now".
func TestNPUVerticalScalerReconcile_ClusterCapDefers(t *testing.T) {
	now := time.Date(2026, 6, 3, 14, 0, 0, 0, time.UTC)
	scaler := buildScalerFixture()
	target := buildTargetFixture(func(m *inferencev1alpha1.ModelService) {
		m.SetAnnotations(map[string]string{annotationSliceTemplate: "qwen-pd-idle"})
	})
	r, _ := newScalerReconciler(t, now, []metrics.IngestorResult{{Value: 87}}, scaler, target)
	// Cluster at rate cap: used=3, cap=3 → used+1 > cap → defer.
	r.ClusterScaleCap = func(_ context.Context) (int32, int32, bool, error) {
		return 3, 3, true, nil
	}

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{
		Name: scaler.Name, Namespace: scaler.Namespace,
	}})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.RequeueAfter != requeueAfterScale {
		t.Fatalf("RequeueAfter = %v, want %v on cluster-cap defer", res.RequeueAfter, requeueAfterScale)
	}
	// Target annotation must be UNCHANGED (scale deferred).
	var refreshed inferencev1alpha1.ModelService
	_ = r.Client.Get(context.Background(), types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, &refreshed)
	if v := refreshed.GetAnnotations()[annotationSliceTemplate]; v != "qwen-pd-idle" {
		t.Fatalf("annotation %q = %q, want qwen-pd-idle (deferred)", annotationSliceTemplate, v)
	}
	var sr inferencev1alpha1.NPUVerticalScaler
	_ = r.Client.Get(context.Background(), types.NamespacedName{Name: scaler.Name, Namespace: scaler.Namespace}, &sr)
	if len(sr.Status.ScaleHistory) != 0 {
		t.Fatalf("ScaleHistory len = %d, want 0 on cluster-cap defer", len(sr.Status.ScaleHistory))
	}
	var active *metav1.Condition
	for i := range sr.Status.Conditions {
		if sr.Status.Conditions[i].Type == inferencev1alpha1.ConditionActive {
			active = &sr.Status.Conditions[i]
			break
		}
	}
	if active == nil || active.Reason != reasonClusterQuotaExceeded {
		t.Fatalf("ConditionActive.Reason = %v, want %s", active, reasonClusterQuotaExceeded)
	}
}

// TestNPUVerticalScalerReconcile_ClusterCapUnderProceeds: cluster under cap →
// scale proceeds normally (annotation patched to busy).
func TestNPUVerticalScalerReconcile_ClusterCapUnderProceeds(t *testing.T) {
	now := time.Date(2026, 6, 3, 14, 0, 0, 0, time.UTC)
	scaler := buildScalerFixture()
	target := buildTargetFixture(func(m *inferencev1alpha1.ModelService) {
		m.SetAnnotations(map[string]string{annotationSliceTemplate: "qwen-pd-idle"})
	})
	r, _ := newScalerReconciler(t, now, []metrics.IngestorResult{{Value: 87}}, scaler, target)
	r.ClusterScaleCap = func(_ context.Context) (int32, int32, bool, error) {
		return 1, 10, true, nil // headroom
	}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{
		Name: scaler.Name, Namespace: scaler.Namespace,
	}}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	var refreshed inferencev1alpha1.ModelService
	_ = r.Client.Get(context.Background(), types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, &refreshed)
	if v := refreshed.GetAnnotations()[annotationSliceTemplate]; v != "qwen-pd-busy" {
		t.Fatalf("annotation %q = %q, want qwen-pd-busy (cluster under cap → proceed)", annotationSliceTemplate, v)
	}
}
