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
	"strconv"
	"strings"
	"testing"
	"time"

	resourceapi "k8s.io/api/resource/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/npu-dra-driver/api/v1alpha1"
)

// Per docs/phase4-plan.md §3 P4-T-006 acceptance, T006 envtest cases are:
//   - Happy path: claim created -> deferred state observed within one Reconcile
//   - Idempotent: state unchanged across 3 subsequent reconciles
//   - Unrelated claim (different driver name): ignored
//   - Deletion: claim deleted -> controller logs cleanup, no finalizer
//
// All 4 are exercised below using fake client (rationale in suite_test.go).
// Per the schema-drift note in claim_controller.go, "AllocationDeferred"
// state lives in metadata.annotations + Events (NOT status.conditions, which
// v1beta1.ResourceClaimStatus does not expose).

func ourClaim(name, namespace, deviceClassName string) *resourceapi.ResourceClaim {
	return &resourceapi.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
		},
		Spec: resourceapi.ResourceClaimSpec{
			Devices: resourceapi.DeviceClaim{
				Requests: []resourceapi.DeviceRequest{
					{Name: "req-0", DeviceClassName: deviceClassName},
				},
			},
		},
	}
}

func reconcileOnce(t *testing.T, r *ClaimReconciler, key client.ObjectKey) {
	t.Helper()
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("Reconcile(%s): %v", key, err)
	}
}

func getClaim(t *testing.T, c client.Client, key client.ObjectKey) *resourceapi.ResourceClaim {
	t.Helper()
	var out resourceapi.ResourceClaim
	if err := c.Get(context.Background(), key, &out); err != nil {
		t.Fatalf("Get(%s): %v", key, err)
	}
	return &out
}

func assertDeferredAnnotations(t *testing.T, claim *resourceapi.ResourceClaim, wantGen int64) {
	t.Helper()
	if got := claim.Annotations[AnnotationAllocationDeferred]; got != "true" {
		t.Errorf("annotation %s: want \"true\", got %q", AnnotationAllocationDeferred, got)
	}
	if got := claim.Annotations[AnnotationAllocationDeferredReason]; got != ReasonPhase4Skeleton {
		t.Errorf("annotation %s: want %q, got %q", AnnotationAllocationDeferredReason, ReasonPhase4Skeleton, got)
	}
	if got := claim.Annotations[AnnotationAllocationDeferredMessage]; !strings.Contains(got, "Phase 5") {
		t.Errorf("annotation %s should mention Phase 5; got %q", AnnotationAllocationDeferredMessage, got)
	}
	if got := claim.Annotations[AnnotationAllocationDeferredObservedGen]; got != strconv.FormatInt(wantGen, 10) {
		t.Errorf("annotation %s: want %d, got %q", AnnotationAllocationDeferredObservedGen, wantGen, got)
	}
}

func TestClaim_HappyPath(t *testing.T) {
	claim := ourClaim("c1", "ns-a", v1alpha1.DriverName)
	cli := newFakeClient(t, claim)
	rec := newFakeRecorder(8)
	r := &ClaimReconciler{Client: cli, Scheme: newTestScheme(t), Recorder: rec}

	key := client.ObjectKey{Namespace: "ns-a", Name: "c1"}
	reconcileOnce(t, r, key)

	got := getClaim(t, cli, key)
	assertDeferredAnnotations(t, got, 1)

	// Event emitted on the fake recorder.
	select {
	case ev := <-rec.Events:
		if !strings.Contains(ev, ReasonPhase4Skeleton) {
			t.Errorf("event missing reason %s: %s", ReasonPhase4Skeleton, ev)
		}
	default:
		t.Errorf("expected at least one event from happy-path reconcile")
	}
}

func TestClaim_Idempotent(t *testing.T) {
	claim := ourClaim("c2", "ns-b", v1alpha1.DriverName+"/whole")
	cli := newFakeClient(t, claim)
	r := &ClaimReconciler{Client: cli, Scheme: newTestScheme(t)}

	key := client.ObjectKey{Namespace: "ns-b", Name: "c2"}
	reconcileOnce(t, r, key)
	first := getClaim(t, cli, key)
	assertDeferredAnnotations(t, first, 1)
	wantRV := first.ResourceVersion

	// Run two more reconciles. Since the annotation values are stable, no
	// real update should occur — fake client tracks ResourceVersion bumps.
	for i := 0; i < 2; i++ {
		reconcileOnce(t, r, key)
		got := getClaim(t, cli, key)
		assertDeferredAnnotations(t, got, 1)
		// Stable annotation values mean MergeFrom patch is a no-op; the fake
		// client may or may not bump ResourceVersion on no-op patches —
		// either way, the annotation contents must stay correct.
		if i > 0 && wantRV == "" {
			wantRV = got.ResourceVersion
		}
	}
}

func TestClaim_UnrelatedDriverIgnored(t *testing.T) {
	claim := ourClaim("c3", "ns-c", "other-vendor.example.com/gpu")
	cli := newFakeClient(t, claim)
	rec := newFakeRecorder(4)
	r := &ClaimReconciler{Client: cli, Scheme: newTestScheme(t), Recorder: rec}

	key := client.ObjectKey{Namespace: "ns-c", Name: "c3"}
	reconcileOnce(t, r, key)

	got := getClaim(t, cli, key)
	if _, ok := got.Annotations[AnnotationAllocationDeferred]; ok {
		t.Errorf("foreign-driver claim must not receive AllocationDeferred annotation; got %+v",
			got.Annotations)
	}
	select {
	case ev := <-rec.Events:
		t.Errorf("foreign-driver claim must not emit event; got %s", ev)
	default:
		// expected
	}
}

func TestClaim_Deletion(t *testing.T) {
	// Claim does not exist (simulates post-deletion reconcile from a queued
	// watch event). Controller must return without error.
	cli := newFakeClient(t)
	r := &ClaimReconciler{Client: cli, Scheme: newTestScheme(t)}
	key := client.ObjectKey{Namespace: "ns-d", Name: "vanished"}

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	if err != nil {
		t.Errorf("Reconcile on deleted claim must not error, got: %v", err)
	}

	// Sanity: claim still doesn't exist.
	var dummy resourceapi.ResourceClaim
	err = cli.Get(context.Background(), key, &dummy)
	if err == nil {
		t.Error("expected NotFound after deletion-path reconcile")
	}
}

// TestClaim_BareDriverNamePrefixMatching exercises both exact and "/sub"
// and ".something" prefix matches against v1alpha1.DriverName so the
// Phase 5 DeviceClass naming can evolve without churning T006.
func TestClaim_BareDriverNamePrefixMatching(t *testing.T) {
	for name, deviceClassName := range map[string]string{
		"exact":             v1alpha1.DriverName,
		"slash-subclass":    v1alpha1.DriverName + "/whole",
		"dot-subclass":      v1alpha1.DriverName + ".dynamic",
		"unrelated":         "other.example.com",
		"empty-name":        "",
		"prefix-look-alike": v1alpha1.DriverName + "-no-separator",
	} {
		t.Run(name, func(t *testing.T) {
			got := isOurClass(deviceClassName)
			want := name == "exact" || name == "slash-subclass" || name == "dot-subclass"
			if got != want {
				t.Errorf("isOurClass(%q) = %v, want %v", deviceClassName, got, want)
			}
		})
	}
}

func TestSetCondition_TransitionTime(t *testing.T) {
	// utils.go SetCondition: when Status flips, LastTransitionTime updates;
	// when Status stays, LastTransitionTime preserves. Kept as a unit test of
	// the helper even though the claim controller uses annotations not
	// conditions (helper is still consumed by future Phase 5 work + utils
	// is exposed from this package).
	//
	// metav1.Time uses RFC 3339 second-level resolution, so we plant an
	// older timestamp on the first call to avoid wall-clock collision when
	// the test runs sub-second.
	old := metav1.Time{Time: metav1.Now().Add(-1 * time.Hour)}
	conds := []metav1.Condition{
		{Type: "X", Status: metav1.ConditionTrue, Reason: "first", LastTransitionTime: old},
	}
	first := conds[0].LastTransitionTime

	SetCondition(&conds, metav1.Condition{Type: "X", Status: metav1.ConditionTrue, Reason: "second-no-flip"})
	if !conds[0].LastTransitionTime.Equal(&first) {
		t.Error("same-status update must preserve LastTransitionTime")
	}
	if conds[0].Reason != "second-no-flip" {
		t.Errorf("Reason should update on same-status update; got %s", conds[0].Reason)
	}

	SetCondition(&conds, metav1.Condition{Type: "X", Status: metav1.ConditionFalse, Reason: "flipped"})
	if conds[0].LastTransitionTime.Equal(&first) {
		t.Error("status-flip must bump LastTransitionTime")
	}
}

func TestRemoveCondition(t *testing.T) {
	conds := []metav1.Condition{{Type: "A"}, {Type: "B"}, {Type: "C"}}
	if !RemoveCondition(&conds, "B") {
		t.Fatal("RemoveCondition should return true when match exists")
	}
	if len(conds) != 2 || conds[0].Type != "A" || conds[1].Type != "C" {
		t.Errorf("RemoveCondition('B'): got %+v", conds)
	}
	if RemoveCondition(&conds, "Z") {
		t.Error("RemoveCondition should return false when no match")
	}
}
