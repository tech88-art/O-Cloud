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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	inferencev1alpha1 "github.com/tech88-art/O-Cloud/operators/inference-operator/api/v1alpha1"
)

// makeTestQuota constructs a minimal Quota for unit tests.
func makeTestQuota() *inferencev1alpha1.Quota {
	return &inferencev1alpha1.Quota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo-quota",
			Namespace: "ai-edge-demo",
		},
		Spec: inferencev1alpha1.QuotaSpec{
			Enforcement: inferencev1alpha1.QuotaEnforcement{
				MaxSliceAllocations: 8,
				MaxScaleEventsPerWindow: inferencev1alpha1.ScaleEventRateCap{
					Count:         5,
					WindowSeconds: 3600,
				},
			},
		},
	}
}

// quotaTestScheme registers types needed by reconciler tests.
func quotaTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := inferencev1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("add v1alpha1 to scheme: %v", err)
	}
	return s
}

// TestQuotaReconcileEmptyNamespaceUsageZero covers P9-T-006 acceptance
// case 1/4: no NPUSliceAllocation + no NPUVerticalScaler scaleHistory →
// status.usage.currentSliceAllocations=0 + scaleEventsInWindow=0.
func TestQuotaReconcileEmptyNamespaceUsageZero(t *testing.T) {
	q := makeTestQuota()
	scheme := quotaTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(q).
		WithStatusSubresource(q).
		Build()
	r := &QuotaReconciler{
		Client: c,
		Scheme: scheme,
		NowFn:  func() time.Time { return time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC) },
	}
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: keyFor(q)})
	if err != nil {
		t.Fatalf("Reconcile err: %v", err)
	}
	if res.RequeueAfter != QuotaReconcilePeriod {
		t.Errorf("Reconcile RequeueAfter = %s; want %s", res.RequeueAfter, QuotaReconcilePeriod)
	}
	var after inferencev1alpha1.Quota
	if err := c.Get(context.Background(), keyFor(q), &after); err != nil {
		t.Fatalf("Get after Reconcile: %v", err)
	}
	if after.Status.Usage.CurrentSliceAllocations != 0 {
		t.Errorf("CurrentSliceAllocations = %d; want 0", after.Status.Usage.CurrentSliceAllocations)
	}
	if after.Status.Usage.ScaleEventsInWindow != 0 {
		t.Errorf("ScaleEventsInWindow = %d; want 0", after.Status.Usage.ScaleEventsInWindow)
	}
	if after.Status.LastSyncTime == nil {
		t.Errorf("LastSyncTime not set")
	}
}

// TestQuotaReconcileNPUSliceAllocationCount covers P9-T-006 acceptance
// case 2/4: N NPUSliceAllocation in namespace → status.usage.currentSliceAllocations=N.
func TestQuotaReconcileNPUSliceAllocationCount(t *testing.T) {
	q := makeTestQuota()
	scheme := quotaTestScheme(t)
	// Seed 3 unstructured NPUSliceAllocation objects in the namespace.
	allocs := make([]runtime.Object, 0, 3)
	for i := 0; i < 3; i++ {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(NPUSliceAllocationGVK)
		obj.SetName("alloc-" + string(rune('a'+i)))
		obj.SetNamespace(q.Namespace)
		allocs = append(allocs, obj)
	}
	builder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(q).
		WithStatusSubresource(q)
	for _, a := range allocs {
		builder = builder.WithRuntimeObjects(a)
	}
	c := builder.Build()
	r := &QuotaReconciler{
		Client: c,
		Scheme: scheme,
		NowFn:  func() time.Time { return time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC) },
	}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: keyFor(q)}); err != nil {
		t.Fatalf("Reconcile err: %v", err)
	}
	var after inferencev1alpha1.Quota
	if err := c.Get(context.Background(), keyFor(q), &after); err != nil {
		t.Fatalf("Get after Reconcile: %v", err)
	}
	if after.Status.Usage.CurrentSliceAllocations != 3 {
		t.Errorf("CurrentSliceAllocations = %d; want 3", after.Status.Usage.CurrentSliceAllocations)
	}
}

// TestQuotaReconcileScaleEventCountSync covers P9-T-006 acceptance case 3/4:
// NPUVerticalScaler scaleHistory entries within windowSeconds counted +
// older entries (outside window) excluded.
func TestQuotaReconcileScaleEventCountSync(t *testing.T) {
	q := makeTestQuota()
	q.Spec.Enforcement.MaxScaleEventsPerWindow.WindowSeconds = 3600 // 1 hour window
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	scheme := quotaTestScheme(t)

	scaler := &inferencev1alpha1.NPUVerticalScaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-scaler",
			Namespace: q.Namespace,
		},
		Status: inferencev1alpha1.NPUVerticalScalerStatus{
			ScaleHistory: []inferencev1alpha1.ScaleEvent{
				// Within window (5 min ago)
				{Time: metav1.NewTime(now.Add(-5 * time.Minute)), ToTemplate: "qwen-pd-busy"},
				// Within window (30 min ago)
				{Time: metav1.NewTime(now.Add(-30 * time.Minute)), ToTemplate: "qwen-pd-idle"},
				// Outside window (2 hours ago)
				{Time: metav1.NewTime(now.Add(-2 * time.Hour)), ToTemplate: "qwen-pd-busy"},
			},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(q, scaler).
		WithStatusSubresource(q, scaler).
		Build()
	r := &QuotaReconciler{
		Client: c,
		Scheme: scheme,
		NowFn:  func() time.Time { return now },
	}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: keyFor(q)}); err != nil {
		t.Fatalf("Reconcile err: %v", err)
	}
	var after inferencev1alpha1.Quota
	if err := c.Get(context.Background(), keyFor(q), &after); err != nil {
		t.Fatalf("Get after Reconcile: %v", err)
	}
	if after.Status.Usage.ScaleEventsInWindow != 2 {
		t.Errorf("ScaleEventsInWindow = %d; want 2 (within-window entries only)", after.Status.Usage.ScaleEventsInWindow)
	}
}

// TestQuotaReconcileLastSyncTimeMonotonic covers P9-T-006 acceptance
// case 4/4: lastSyncTime advances on subsequent ticks (controller writes
// new time each Reconcile invocation).
func TestQuotaReconcileLastSyncTimeMonotonic(t *testing.T) {
	q := makeTestQuota()
	scheme := quotaTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(q).
		WithStatusSubresource(q).
		Build()

	t0 := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(60 * time.Second)

	// Tick 1
	r := &QuotaReconciler{
		Client: c,
		Scheme: scheme,
		NowFn:  func() time.Time { return t0 },
	}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: keyFor(q)}); err != nil {
		t.Fatalf("tick 1 Reconcile err: %v", err)
	}
	var after1 inferencev1alpha1.Quota
	_ = c.Get(context.Background(), keyFor(q), &after1)
	if after1.Status.LastSyncTime == nil {
		t.Fatalf("LastSyncTime not set after tick 1")
	}
	got1 := after1.Status.LastSyncTime.Time

	// Tick 2 at t1
	r.NowFn = func() time.Time { return t1 }
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: keyFor(q)}); err != nil {
		t.Fatalf("tick 2 Reconcile err: %v", err)
	}
	var after2 inferencev1alpha1.Quota
	_ = c.Get(context.Background(), keyFor(q), &after2)
	if after2.Status.LastSyncTime == nil {
		t.Fatalf("LastSyncTime not set after tick 2")
	}
	got2 := after2.Status.LastSyncTime.Time

	if !got2.After(got1) {
		t.Errorf("LastSyncTime not monotonic: tick1=%s tick2=%s", got1, got2)
	}
}

// keyFor returns the NamespacedName for an object meta accessor.
func keyFor(obj interface {
	GetName() string
	GetNamespace() string
}) types.NamespacedName {
	return types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
}
