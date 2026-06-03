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

package webhook

import (
	"context"
	"encoding/json"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	inferencev1alpha1 "github.com/tech88-art/O-Cloud/operators/inference-operator/api/v1alpha1"
)

func quotaWebhookTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := inferencev1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("add v1alpha1: %v", err)
	}
	return s
}

func makeQuota(ns string, maxAlloc, maxScale int32, usage inferencev1alpha1.QuotaUsage, whitelist []string) *inferencev1alpha1.Quota {
	return &inferencev1alpha1.Quota{
		ObjectMeta: metav1.ObjectMeta{Name: "demo-quota", Namespace: ns},
		Spec: inferencev1alpha1.QuotaSpec{
			Enforcement: inferencev1alpha1.QuotaEnforcement{
				MaxSliceAllocations: maxAlloc,
				MaxScaleEventsPerWindow: inferencev1alpha1.ScaleEventRateCap{
					Count:         maxScale,
					WindowSeconds: 3600,
				},
				MaxNPUSliceTemplateRefs: whitelist,
			},
		},
		Status: inferencev1alpha1.QuotaStatus{Usage: usage},
	}
}

func makeNPUSliceAllocCreateReq(ns string) admission.Request {
	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Namespace: ns,
			Name:      "alloc-1",
			Operation: admissionv1.Create,
		},
	}
}

func makeScalerUpdateReq(t *testing.T, ns string, oldBusy, oldIdle, newBusy, newIdle string) admission.Request {
	t.Helper()
	oldScaler := &inferencev1alpha1.NPUVerticalScaler{
		ObjectMeta: metav1.ObjectMeta{Name: "qwen-scaler", Namespace: ns},
		Spec: inferencev1alpha1.NPUVerticalScalerSpec{
			ScaleSlice: inferencev1alpha1.ScaleSliceSpec{
				BusyTemplateName: oldBusy, IdleTemplateName: oldIdle,
			},
		},
	}
	newScaler := oldScaler.DeepCopy()
	newScaler.Spec.ScaleSlice.BusyTemplateName = newBusy
	newScaler.Spec.ScaleSlice.IdleTemplateName = newIdle
	oldRaw, _ := json.Marshal(oldScaler)
	newRaw, _ := json.Marshal(newScaler)
	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Namespace: ns,
			Name:      "qwen-scaler",
			Operation: admissionv1.Update,
			Object:    runtime.RawExtension{Raw: newRaw},
			OldObject: runtime.RawExtension{Raw: oldRaw},
		},
	}
}

// TestWebhookA_UnderCapAllow covers acceptance case 1/6 (Webhook A · NPUSliceAllocation create).
func TestWebhookA_UnderCapAllow(t *testing.T) {
	scheme := quotaWebhookTestScheme(t)
	q := makeQuota("ai-edge-demo", 8, 5, inferencev1alpha1.QuotaUsage{CurrentSliceAllocations: 5}, nil)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(q).Build()
	cache := NewQuotaCache(c, 0)
	v := &QuotaSliceAllocationValidator{Cache: cache}
	resp := v.Handle(context.Background(), makeNPUSliceAllocCreateReq("ai-edge-demo"))
	if !resp.Allowed {
		t.Errorf("under-cap NPUSliceAllocation should be allowed; got %+v", resp)
	}
}

// TestWebhookA_AtCapReject covers acceptance case 2/6.
func TestWebhookA_AtCapReject(t *testing.T) {
	scheme := quotaWebhookTestScheme(t)
	q := makeQuota("ai-edge-demo", 8, 5, inferencev1alpha1.QuotaUsage{CurrentSliceAllocations: 8}, nil)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(q).Build()
	cache := NewQuotaCache(c, 0)
	v := &QuotaSliceAllocationValidator{Cache: cache}
	resp := v.Handle(context.Background(), makeNPUSliceAllocCreateReq("ai-edge-demo"))
	if resp.Allowed {
		t.Errorf("at-cap NPUSliceAllocation should be rejected; got %+v", resp)
	}
}

// TestWebhookA_NoQuotaUnbounded covers acceptance case 3/6 (fail-open).
func TestWebhookA_NoQuotaUnbounded(t *testing.T) {
	scheme := quotaWebhookTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	cache := NewQuotaCache(c, 0)
	v := &QuotaSliceAllocationValidator{Cache: cache}
	resp := v.Handle(context.Background(), makeNPUSliceAllocCreateReq("empty-ns"))
	if !resp.Allowed {
		t.Errorf("no Quota in namespace should fail-open; got %+v", resp)
	}
}

// TestWebhookB_RateExceedReject covers acceptance case 4/6 (Webhook B · scaler scale rate).
func TestWebhookB_RateExceedReject(t *testing.T) {
	scheme := quotaWebhookTestScheme(t)
	q := makeQuota("ai-edge-demo", 8, 5, inferencev1alpha1.QuotaUsage{ScaleEventsInWindow: 5}, nil)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(q).Build()
	cache := NewQuotaCache(c, 0)
	dec := admission.NewDecoder(scheme)
	v := &QuotaScalerValidator{Cache: cache, Decoder: dec}
	req := makeScalerUpdateReq(t, "ai-edge-demo", "qwen-pd-idle", "qwen-pd-idle", "qwen-pd-busy", "qwen-pd-idle")
	resp := v.Handle(context.Background(), req)
	if resp.Allowed {
		t.Errorf("at-rate-cap NPUVerticalScaler.spec patch should be rejected; got %+v", resp)
	}
}

// TestWebhookB_WhitelistAllow covers acceptance case 5/6 (Webhook B · whitelist allow).
func TestWebhookB_WhitelistAllow(t *testing.T) {
	scheme := quotaWebhookTestScheme(t)
	q := makeQuota("ai-edge-demo", 8, 5, inferencev1alpha1.QuotaUsage{ScaleEventsInWindow: 1},
		[]string{"qwen-pd-busy", "qwen-pd-idle"})
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(q).Build()
	cache := NewQuotaCache(c, 0)
	dec := admission.NewDecoder(scheme)
	v := &QuotaScalerValidator{Cache: cache, Decoder: dec}
	req := makeScalerUpdateReq(t, "ai-edge-demo", "qwen-pd-idle", "qwen-pd-idle", "qwen-pd-busy", "qwen-pd-idle")
	resp := v.Handle(context.Background(), req)
	if !resp.Allowed {
		t.Errorf("under-cap + whitelisted template should be allowed; got %+v", resp)
	}
}

// TestWebhookB_WhitelistDeny covers acceptance case 6/6 (Webhook B · whitelist deny).
func TestWebhookB_WhitelistDeny(t *testing.T) {
	scheme := quotaWebhookTestScheme(t)
	q := makeQuota("ai-edge-demo", 8, 5, inferencev1alpha1.QuotaUsage{ScaleEventsInWindow: 1},
		[]string{"qwen-pd-busy", "qwen-pd-idle"})
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(q).Build()
	cache := NewQuotaCache(c, 0)
	dec := admission.NewDecoder(scheme)
	v := &QuotaScalerValidator{Cache: cache, Decoder: dec}
	req := makeScalerUpdateReq(t, "ai-edge-demo", "qwen-pd-idle", "qwen-pd-idle", "rogue-template", "qwen-pd-idle")
	resp := v.Handle(context.Background(), req)
	if resp.Allowed {
		t.Errorf("non-whitelisted template should be rejected; got %+v", resp)
	}
}

// ---- P13-T-204: cluster-wide ClusterQuota enforcement (ADR-0025 §2 Decision C) ----

func makeClusterQuota(maxAlloc, maxScale int32, total inferencev1alpha1.QuotaUsage) *inferencev1alpha1.ClusterQuota {
	return &inferencev1alpha1.ClusterQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-quota"},
		Spec: inferencev1alpha1.ClusterQuotaSpec{
			Enforcement: inferencev1alpha1.QuotaEnforcement{
				MaxSliceAllocations: maxAlloc,
				MaxScaleEventsPerWindow: inferencev1alpha1.ScaleEventRateCap{
					Count: maxScale, WindowSeconds: 3600,
				},
			},
		},
		Status: inferencev1alpha1.ClusterQuotaStatus{
			Usage: inferencev1alpha1.ClusterQuotaUsage{Total: total},
		},
	}
}

// TestWebhookA_ClusterCapReject: namespace UNDER cap but cluster OVER cap →
// CREATE denied (must be under BOTH). This is the build-doc §5 acceptance:
// "apply ClusterQuota + over-cap NPUSliceAllocation → admission 拒".
func TestWebhookA_ClusterCapReject(t *testing.T) {
	scheme := quotaWebhookTestScheme(t)
	q := makeQuota("ai-edge-demo", 8, 5, inferencev1alpha1.QuotaUsage{CurrentSliceAllocations: 2}, nil) // ns under cap
	cq := makeClusterQuota(10, 20, inferencev1alpha1.QuotaUsage{CurrentSliceAllocations: 10})           // cluster AT cap
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(q, cq).Build()
	v := &QuotaSliceAllocationValidator{Cache: NewQuotaCache(c, 0), ClusterCache: NewClusterQuotaCache(c, 0)}
	resp := v.Handle(context.Background(), makeNPUSliceAllocCreateReq("ai-edge-demo"))
	if resp.Allowed {
		t.Errorf("cluster-over-cap NPUSliceAllocation should be rejected even when namespace under cap; got %+v", resp)
	}
}

// TestWebhookA_ClusterCapAllow: both namespace + cluster under cap → allowed.
func TestWebhookA_ClusterCapAllow(t *testing.T) {
	scheme := quotaWebhookTestScheme(t)
	q := makeQuota("ai-edge-demo", 8, 5, inferencev1alpha1.QuotaUsage{CurrentSliceAllocations: 2}, nil)
	cq := makeClusterQuota(10, 20, inferencev1alpha1.QuotaUsage{CurrentSliceAllocations: 4})
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(q, cq).Build()
	v := &QuotaSliceAllocationValidator{Cache: NewQuotaCache(c, 0), ClusterCache: NewClusterQuotaCache(c, 0)}
	resp := v.Handle(context.Background(), makeNPUSliceAllocCreateReq("ai-edge-demo"))
	if !resp.Allowed {
		t.Errorf("under both namespace + cluster cap should be allowed; got %+v", resp)
	}
}

// TestWebhookA_NilClusterCacheBackcompat: ClusterCache nil → namespace-only
// (Phase 9 backward-compatible · no cluster enforcement).
func TestWebhookA_NilClusterCacheBackcompat(t *testing.T) {
	scheme := quotaWebhookTestScheme(t)
	q := makeQuota("ai-edge-demo", 8, 5, inferencev1alpha1.QuotaUsage{CurrentSliceAllocations: 2}, nil)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(q).Build()
	v := &QuotaSliceAllocationValidator{Cache: NewQuotaCache(c, 0)} // ClusterCache nil
	resp := v.Handle(context.Background(), makeNPUSliceAllocCreateReq("ai-edge-demo"))
	if !resp.Allowed {
		t.Errorf("nil ClusterCache should be namespace-only (allow); got %+v", resp)
	}
}

// TestWebhookB_ClusterScaleRateReject: namespace scale-rate has headroom but
// cluster-wide scale-rate AT cap → scaler template change denied.
func TestWebhookB_ClusterScaleRateReject(t *testing.T) {
	scheme := quotaWebhookTestScheme(t)
	q := makeQuota("ai-edge-demo", 8, 5, inferencev1alpha1.QuotaUsage{ScaleEventsInWindow: 1}, nil) // ns under rate cap
	cq := makeClusterQuota(64, 3, inferencev1alpha1.QuotaUsage{ScaleEventsInWindow: 3})             // cluster AT rate cap
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(q, cq).Build()
	dec := admission.NewDecoder(scheme)
	v := &QuotaScalerValidator{Cache: NewQuotaCache(c, 0), ClusterCache: NewClusterQuotaCache(c, 0), Decoder: dec}
	req := makeScalerUpdateReq(t, "ai-edge-demo", "qwen-pd-idle", "qwen-pd-idle", "qwen-pd-busy", "qwen-pd-idle")
	resp := v.Handle(context.Background(), req)
	if resp.Allowed {
		t.Errorf("cluster-over-rate-cap scaler change should be rejected even when namespace under cap; got %+v", resp)
	}
}

// TestClusterQuotaCache_GetAndNotFound: cache returns the singleton, and
// (nil,false,nil) when none set (fail-open).
func TestClusterQuotaCache_GetAndNotFound(t *testing.T) {
	scheme := quotaWebhookTestScheme(t)
	// not-found path
	empty := fake.NewClientBuilder().WithScheme(scheme).Build()
	if cq, ok, err := NewClusterQuotaCache(empty, 0).Get(context.Background()); err != nil || ok || cq != nil {
		t.Errorf("empty cluster → (nil,false,nil); got (%v,%v,%v)", cq, ok, err)
	}
	// found path
	cq := makeClusterQuota(10, 5, inferencev1alpha1.QuotaUsage{CurrentSliceAllocations: 3})
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cq).Build()
	got, ok, err := NewClusterQuotaCache(c, 0).Get(context.Background())
	if err != nil || !ok || got == nil {
		t.Fatalf("present ClusterQuota → (obj,true,nil); got (%v,%v,%v)", got, ok, err)
	}
	if got.Status.Usage.Total.CurrentSliceAllocations != 3 {
		t.Errorf("cached Total = %d; want 3", got.Status.Usage.Total.CurrentSliceAllocations)
	}
}
