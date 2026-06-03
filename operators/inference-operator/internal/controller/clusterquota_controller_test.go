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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	inferencev1alpha1 "github.com/tech88-art/O-Cloud/operators/inference-operator/api/v1alpha1"
)

// makeTestClusterQuota constructs a minimal ClusterQuota for unit tests.
func makeTestClusterQuota() *inferencev1alpha1.ClusterQuota {
	return &inferencev1alpha1.ClusterQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-quota"},
		Spec: inferencev1alpha1.ClusterQuotaSpec{
			Enforcement: inferencev1alpha1.QuotaEnforcement{
				MaxSliceAllocations: 64,
				MaxScaleEventsPerWindow: inferencev1alpha1.ScaleEventRateCap{
					Count:         20,
					WindowSeconds: 3600,
				},
			},
		},
	}
}

// allocWith builds an unstructured NPUSliceAllocation in namespace ns with an
// optional Karmada source-cluster annotation (empty cluster = no annotation).
func allocWith(name, ns, cluster string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(NPUSliceAllocationGVK)
	obj.SetName(name)
	obj.SetNamespace(ns)
	if cluster != "" {
		obj.SetAnnotations(map[string]string{KarmadaCachedFromClusterAnnotation: cluster})
	}
	return obj
}

// TestClusterQuotaReconcileEmptyZero: no allocations / scalers → Total zero,
// LastSyncTime set, requeue at period.
func TestClusterQuotaReconcileEmptyZero(t *testing.T) {
	cq := makeTestClusterQuota()
	scheme := quotaTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cq).WithStatusSubresource(cq).Build()
	r := &ClusterQuotaReconciler{
		Client: c, Scheme: scheme,
		NowFn: func() time.Time { return time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC) },
	}
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: keyFor(cq)})
	if err != nil {
		t.Fatalf("Reconcile err: %v", err)
	}
	if res.RequeueAfter != ClusterQuotaReconcilePeriod {
		t.Errorf("RequeueAfter = %s; want %s", res.RequeueAfter, ClusterQuotaReconcilePeriod)
	}
	var after inferencev1alpha1.ClusterQuota
	if err := c.Get(context.Background(), keyFor(cq), &after); err != nil {
		t.Fatalf("Get after: %v", err)
	}
	if after.Status.Usage.Total.CurrentSliceAllocations != 0 {
		t.Errorf("Total.CurrentSliceAllocations = %d; want 0", after.Status.Usage.Total.CurrentSliceAllocations)
	}
	if after.Status.LastSyncTime == nil {
		t.Errorf("LastSyncTime not set")
	}
}

// TestClusterQuotaReconcileCrossClusterSum: allocations tagged member1 (×2) +
// member2 (×3) + one untagged (→ host) → PerCluster buckets + Total = 6.
// Covers the ADR-0018 §2 Decision D cross-cluster sum + RecomputeTotal path.
func TestClusterQuotaReconcileCrossClusterSum(t *testing.T) {
	cq := makeTestClusterQuota()
	scheme := quotaTestScheme(t)
	objs := []runtime.Object{
		allocWith("a1", "ns-a", "member1"),
		allocWith("a2", "ns-b", "member1"),
		allocWith("b1", "ns-a", "member2"),
		allocWith("b2", "ns-c", "member2"),
		allocWith("b3", "ns-c", "member2"),
		allocWith("h1", "ns-a", ""), // untagged → local "host"
	}
	builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cq).WithStatusSubresource(cq)
	for _, o := range objs {
		builder = builder.WithRuntimeObjects(o)
	}
	c := builder.Build()
	r := &ClusterQuotaReconciler{
		Client: c, Scheme: scheme,
		NowFn: func() time.Time { return time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC) },
	}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: keyFor(cq)}); err != nil {
		t.Fatalf("Reconcile err: %v", err)
	}
	var after inferencev1alpha1.ClusterQuota
	if err := c.Get(context.Background(), keyFor(cq), &after); err != nil {
		t.Fatalf("Get after: %v", err)
	}
	pc := after.Status.Usage.PerCluster
	if got := pc["member1"].CurrentSliceAllocations; got != 2 {
		t.Errorf("member1 = %d; want 2", got)
	}
	if got := pc["member2"].CurrentSliceAllocations; got != 3 {
		t.Errorf("member2 = %d; want 3", got)
	}
	if got := pc[DefaultLocalClusterName].CurrentSliceAllocations; got != 1 {
		t.Errorf("host = %d; want 1", got)
	}
	if got := after.Status.Usage.Total.CurrentSliceAllocations; got != 6 {
		t.Errorf("Total.CurrentSliceAllocations = %d; want 6 (RecomputeTotal sum)", got)
	}
}

// TestClusterQuotaReconcileScaleEventsBucketed: scaleHistory within-window
// bucketed per source cluster + older entries excluded; Total sums them.
func TestClusterQuotaReconcileScaleEventsBucketed(t *testing.T) {
	cq := makeTestClusterQuota()
	now := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	scheme := quotaTestScheme(t)

	mkScaler := func(name, ns, cluster string, evMinsAgo ...int) *inferencev1alpha1.NPUVerticalScaler {
		s := &inferencev1alpha1.NPUVerticalScaler{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		}
		if cluster != "" {
			s.SetAnnotations(map[string]string{KarmadaCachedFromClusterAnnotation: cluster})
		}
		for _, m := range evMinsAgo {
			s.Status.ScaleHistory = append(s.Status.ScaleHistory,
				inferencev1alpha1.ScaleEvent{Time: metav1.NewTime(now.Add(-time.Duration(m) * time.Minute))})
		}
		return s
	}
	// member1: 2 in-window (5m, 30m) + 1 out (120m); member2: 1 in-window (10m).
	s1 := mkScaler("s1", "ns-a", "member1", 5, 30, 120)
	s2 := mkScaler("s2", "ns-b", "member2", 10)

	c := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(cq, s1, s2).WithStatusSubresource(cq, s1, s2).Build()
	r := &ClusterQuotaReconciler{Client: c, Scheme: scheme, NowFn: func() time.Time { return now }}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: keyFor(cq)}); err != nil {
		t.Fatalf("Reconcile err: %v", err)
	}
	var after inferencev1alpha1.ClusterQuota
	_ = c.Get(context.Background(), keyFor(cq), &after)
	if got := after.Status.Usage.PerCluster["member1"].ScaleEventsInWindow; got != 2 {
		t.Errorf("member1 scaleEvents = %d; want 2 (in-window only)", got)
	}
	if got := after.Status.Usage.PerCluster["member2"].ScaleEventsInWindow; got != 1 {
		t.Errorf("member2 scaleEvents = %d; want 1", got)
	}
	if got := after.Status.Usage.Total.ScaleEventsInWindow; got != 3 {
		t.Errorf("Total.ScaleEventsInWindow = %d; want 3", got)
	}
}
