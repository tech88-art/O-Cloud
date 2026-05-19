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
	resourceapi "k8s.io/api/resource/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/npu-dra-driver/api/v1alpha1"
)

func mkAllocatedClaim(name, ns string) *resourceapi.ResourceClaim {
	c := ourClaim(name, ns, v1alpha1.DriverName)
	c.Status.Allocation = &resourceapi.AllocationResult{
		Devices: resourceapi.DeviceAllocationResult{
			Results: []resourceapi.DeviceRequestAllocationResult{
				{Request: "req-0", Driver: v1alpha1.DriverName, Pool: "nodeA", Device: "nodeA-npu-0"},
			},
		},
	}
	return c
}

func mkAudit(name, claimNS, claimName string, allocatedAt time.Time) *v1alpha1.NPUSliceAllocation {
	at := metav1.NewTime(allocatedAt)
	return &v1alpha1.NPUSliceAllocation{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			UID:  types.UID(name + "-uid"),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "resource.k8s.io/v1beta1",
					Kind:       "ResourceClaim",
					Name:       claimName,
					UID:        types.UID(claimName + "-uid"),
				},
			},
		},
		Spec: v1alpha1.NPUSliceAllocationSpec{
			ClaimRef: corev1.ObjectReference{
				APIVersion: "resource.k8s.io/v1beta1",
				Kind:       "ResourceClaim",
				Namespace:  claimNS,
				Name:       claimName,
			},
			SliceRef: v1alpha1.SliceReference{
				Driver: v1alpha1.DriverName,
				Pool:   "nodeA",
				Device: "nodeA-npu-0",
			},
			NodeName: "nodeA",
			AICores:  32,
		},
		Status: v1alpha1.NPUSliceAllocationStatus{
			AllocatedAt: &at,
		},
	}
}

func TestAllocation_HappyPath_PhaseAllocated(t *testing.T) {
	// Claim exists and is allocated. Audit reconcile should set
	// phase=Allocated + Available=True.
	claim := mkAllocatedClaim("c1", "ns-a")
	audit := mkAudit("ns-a-c1-nodeA-npu-0", "ns-a", "c1", time.Now())
	cli := newFakeClient(t, claim, audit)
	r := &AllocationReconciler{Client: cli, Scheme: newTestScheme(t)}

	key := client.ObjectKey{Name: audit.Name}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var out v1alpha1.NPUSliceAllocation
	if err := cli.Get(context.Background(), key, &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.Status.Phase != v1alpha1.NPUSliceAllocationPhaseAllocated {
		t.Errorf("phase: want Allocated, got %q", out.Status.Phase)
	}
	if len(out.Status.Conditions) == 0 || out.Status.Conditions[0].Type != v1alpha1.ConditionAvailable {
		t.Errorf("Available condition missing; got %+v", out.Status.Conditions)
	}
	if out.Status.Conditions[0].Status != metav1.ConditionTrue {
		t.Errorf("Available status: want True, got %v", out.Status.Conditions[0].Status)
	}
}

func TestAllocation_CascadeDelete_OwnerRefIsSet(t *testing.T) {
	// fake client doesn't run real K8s GC, so "cascade delete" means
	// "verify the OwnerReference is set + Controller=true so upstream
	// GC processes the cascade in a real cluster". The integration test
	// in T106 kind smoke exercises real GC.
	claim := mkAllocatedClaim("c1", "ns-a")
	cli := newFakeClient(t, claim)

	// Drive claim_controller to create the audit (exercises the
	// claim_controller → allocation audit creation path).
	cr := &ClaimReconciler{Client: cli, Scheme: newTestScheme(t), Recorder: newFakeRecorder(8)}
	dev := fixtureHealthyWholeDevice("nodeA-npu-0", 0)
	slice := fixtureSlice("npu-dra-nodeA", "nodeA", dev)
	if err := cli.Create(context.Background(), slice); err != nil {
		t.Fatalf("create slice: %v", err)
	}
	// Replace claim with one that has no allocation yet so the
	// reconcile runs the full allocate path including audit create.
	if err := cli.Delete(context.Background(), claim); err != nil {
		t.Fatalf("delete claim: %v", err)
	}
	fresh := ourClaim("c1", "ns-a", v1alpha1.DriverName)
	if err := cli.Create(context.Background(), fresh); err != nil {
		t.Fatalf("recreate claim: %v", err)
	}

	key := client.ObjectKey{Namespace: "ns-a", Name: "c1"}
	reconcileOnce(t, cr, key)

	// Now look for the audit object — name is deterministic.
	auditKey := client.ObjectKey{Name: allocationAuditName("ns-a", "c1", "nodeA-npu-0")}
	var audit v1alpha1.NPUSliceAllocation
	if err := cli.Get(context.Background(), auditKey, &audit); err != nil {
		t.Fatalf("get audit: %v", err)
	}
	if len(audit.OwnerReferences) != 1 {
		t.Fatalf("audit must have one ownerRef; got %+v", audit.OwnerReferences)
	}
	or := audit.OwnerReferences[0]
	if or.Kind != "ResourceClaim" || or.Name != "c1" {
		t.Errorf("ownerRef kind/name wrong: %+v", or)
	}
	if or.Controller == nil || !*or.Controller {
		t.Errorf("ownerRef.Controller must be true; got %+v", or.Controller)
	}
	if or.BlockOwnerDeletion == nil || !*or.BlockOwnerDeletion {
		t.Errorf("ownerRef.BlockOwnerDeletion must be true; got %+v", or.BlockOwnerDeletion)
	}
}

func TestAllocation_MultiClaim_DistinctEntries(t *testing.T) {
	// Three claims allocated on three different devices → three
	// distinct NPUSliceAllocations, each with its own owner-ref.
	cli := newFakeClient(t)
	cr := &ClaimReconciler{Client: cli, Scheme: newTestScheme(t), Recorder: newFakeRecorder(16)}

	slice := fixtureSlice("npu-dra-nodeM", "nodeM",
		fixtureHealthyWholeDevice("nodeM-npu-0", 0),
		fixtureHealthyWholeDevice("nodeM-npu-1", 1),
		fixtureHealthyWholeDevice("nodeM-npu-2", 2),
	)
	if err := cli.Create(context.Background(), slice); err != nil {
		t.Fatalf("create slice: %v", err)
	}
	for _, name := range []string{"c-a", "c-b", "c-c"} {
		if err := cli.Create(context.Background(), ourClaim(name, "ns", v1alpha1.DriverName)); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		reconcileOnce(t, cr, client.ObjectKey{Namespace: "ns", Name: name})
	}

	var audits v1alpha1.NPUSliceAllocationList
	if err := cli.List(context.Background(), &audits); err != nil {
		t.Fatalf("list audits: %v", err)
	}
	if len(audits.Items) != 3 {
		t.Fatalf("expected 3 audit entries; got %d", len(audits.Items))
	}
	seen := map[string]bool{}
	for _, a := range audits.Items {
		seen[a.Spec.SliceRef.Device] = true
		if a.Spec.NodeName != "nodeM" {
			t.Errorf("nodeName: want nodeM, got %s", a.Spec.NodeName)
		}
	}
	for _, d := range []string{"nodeM-npu-0", "nodeM-npu-1", "nodeM-npu-2"} {
		if !seen[d] {
			t.Errorf("missing audit for device %s", d)
		}
	}
}

func TestAllocation_Orphan_AfterGracePeriod(t *testing.T) {
	// Audit exists but owning claim does NOT. AllocatedAt is "long
	// ago" relative to the configured grace; reconcile should mark
	// phase=Orphaned + emit a Warning event.
	now := time.Now()
	long := now.Add(-2 * time.Minute) // 2 min ago, > grace
	audit := mkAudit("ns-a-c-vanished-nodeA-npu-0", "ns-a", "c-vanished", long)
	cli := newFakeClient(t, audit)
	rec := newFakeRecorder(8)
	r := &AllocationReconciler{
		Client:      cli,
		Scheme:      newTestScheme(t),
		Recorder:    rec,
		GracePeriod: 30 * time.Second,
		Now:         func() time.Time { return now },
	}

	key := client.ObjectKey{Name: audit.Name}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	var out v1alpha1.NPUSliceAllocation
	if err := cli.Get(context.Background(), key, &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.Status.Phase != v1alpha1.NPUSliceAllocationPhaseOrphaned {
		t.Errorf("phase: want Orphaned, got %q", out.Status.Phase)
	}

	var foundEvent bool
	for {
		select {
		case ev := <-rec.Events:
			if contains(ev, "Orphaned") {
				foundEvent = true
			}
		default:
			goto done
		}
	}
done:
	if !foundEvent {
		t.Errorf("expected Orphaned event; recorder drained without one")
	}
}

func TestAllocation_Released_WithinGracePeriod(t *testing.T) {
	// Audit exists, owning claim absent, but AllocatedAt was a few
	// seconds ago — within grace. Reconcile sets phase=Released
	// (transient) and requeues for the orphan promotion.
	now := time.Now()
	recent := now.Add(-5 * time.Second)
	audit := mkAudit("ns-a-c-gone-nodeA-npu-0", "ns-a", "c-gone", recent)
	cli := newFakeClient(t, audit)
	r := &AllocationReconciler{
		Client:      cli,
		Scheme:      newTestScheme(t),
		GracePeriod: 30 * time.Second,
		Now:         func() time.Time { return now },
	}

	key := client.ObjectKey{Name: audit.Name}
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.RequeueAfter <= 0 {
		t.Errorf("released path should requeue; got %+v", res)
	}

	var out v1alpha1.NPUSliceAllocation
	if err := cli.Get(context.Background(), key, &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.Status.Phase != v1alpha1.NPUSliceAllocationPhaseReleased {
		t.Errorf("phase: want Released within grace, got %q", out.Status.Phase)
	}
}

// contains is a tiny helper to avoid importing strings in the test
// file (the receiver also reads cleaner this way).
func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
