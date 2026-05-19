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
	"strings"
	"testing"
	"time"

	resourceapi "k8s.io/api/resource/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/npu-dra-driver/api/v1alpha1"
	"github.com/tech88-art/O-Cloud/operators/npu-dra-driver/internal/publisher"
)

// Phase 5 T002 controller-package tests cover:
//   - foreign claim ignored
//   - happy path: claim with bare class allocates against a fixture
//     ResourceSlice and ends up with Status.Allocation + Status.Devices
//   - no-slice requeue: empty slice store → Requeue=true, no allocation
//   - migration: Phase 4 AllocationDeferred annotations stripped on
//     first reconcile
//
// T003 adds 5+ envtest cases (sub-class filtering, multi-claim
// determinism, etc.) — those live alongside the allocator unit tests in
// internal/allocator/ and a deeper envtest harness here.

func ourClaim(name, namespace, deviceClassName string) *resourceapi.ResourceClaim {
	return &resourceapi.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
			UID:        types.UID(name + "-uid"),
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

func fixtureSlice(name, node string, devices ...resourceapi.Device) *resourceapi.ResourceSlice {
	return &resourceapi.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				publisher.SliceLabelManagedBy: publisher.SliceLabelManagedByValue,
			},
		},
		Spec: resourceapi.ResourceSliceSpec{
			Driver: v1alpha1.DriverName,
			Pool: resourceapi.ResourcePool{
				Name:               node,
				Generation:         1,
				ResourceSliceCount: 1,
			},
			NodeName: node,
			Devices:  devices,
		},
	}
}

func fixtureHealthyWholeDevice(name string, index int64) resourceapi.Device {
	return v1alpha1.AscendDevice{
		Name:                name,
		Index:               index,
		Health:              v1alpha1.HealthHealthy,
		SliceStrategy:       v1alpha1.SliceStrategyFixedTemplate,
		AICores:             0,
		NUMANode:            0,
		HCCSRing:            0,
		SliceAICoreCapacity: resource.MustParse("32"),
	}.ToUpstream()
}

func fixtureHealthyDynamicDevice(name string, index, aiCores int64) resourceapi.Device {
	return v1alpha1.AscendDevice{
		Name:                name,
		Index:               index,
		Health:              v1alpha1.HealthHealthy,
		SliceStrategy:       v1alpha1.SliceStrategyDynamic,
		AICores:             aiCores,
		NUMANode:            0,
		HCCSRing:            0,
		SliceAICoreCapacity: resource.MustParse("32"),
	}.ToUpstream()
}

func reconcileOnce(t *testing.T, r *ClaimReconciler, key client.ObjectKey) ctrl.Result {
	t.Helper()
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("Reconcile(%s): %v", key, err)
	}
	return res
}

func getClaim(t *testing.T, c client.Client, key client.ObjectKey) *resourceapi.ResourceClaim {
	t.Helper()
	var out resourceapi.ResourceClaim
	if err := c.Get(context.Background(), key, &out); err != nil {
		t.Fatalf("Get(%s): %v", key, err)
	}
	return &out
}

func TestClaim_UnrelatedDriverIgnored(t *testing.T) {
	claim := ourClaim("c-foreign", "ns-c", "other-vendor.example.com/gpu")
	cli := newFakeClient(t, claim)
	rec := newFakeRecorder(4)
	r := &ClaimReconciler{Client: cli, Scheme: newTestScheme(t), Recorder: rec}

	key := client.ObjectKey{Namespace: "ns-c", Name: "c-foreign"}
	reconcileOnce(t, r, key)

	got := getClaim(t, cli, key)
	if got.Status.Allocation != nil {
		t.Errorf("foreign claim must not be allocated; got %+v", got.Status.Allocation)
	}
	select {
	case ev := <-rec.Events:
		t.Errorf("foreign claim must not emit event; got %s", ev)
	default:
		// expected
	}
}

func TestClaim_HappyPath_Allocates(t *testing.T) {
	claim := ourClaim("c-happy", "ns-a", v1alpha1.DriverName)
	dev := fixtureHealthyWholeDevice("nodeA-npu-0", 0)
	slice := fixtureSlice("npu-dra-nodeA", "nodeA", dev)

	cli := newFakeClient(t, claim, slice)
	rec := newFakeRecorder(8)
	r := &ClaimReconciler{Client: cli, Scheme: newTestScheme(t), Recorder: rec}

	key := client.ObjectKey{Namespace: "ns-a", Name: "c-happy"}
	res := reconcileOnce(t, r, key)
	if res.Requeue {
		t.Errorf("happy path must not requeue; got Requeue=%v", res.Requeue)
	}

	got := getClaim(t, cli, key)
	if got.Status.Allocation == nil {
		t.Fatalf("expected Status.Allocation to be set; got nil")
	}
	if got.Status.Allocation.Devices.Results == nil || len(got.Status.Allocation.Devices.Results) != 1 {
		t.Fatalf("expected one allocation result; got %+v", got.Status.Allocation.Devices.Results)
	}
	r0 := got.Status.Allocation.Devices.Results[0]
	if r0.Driver != v1alpha1.DriverName {
		t.Errorf("result driver: want %q, got %q", v1alpha1.DriverName, r0.Driver)
	}
	if r0.Pool != "nodeA" {
		t.Errorf("result pool: want nodeA, got %q", r0.Pool)
	}
	if r0.Device != "nodeA-npu-0" {
		t.Errorf("result device: want nodeA-npu-0, got %q", r0.Device)
	}
	if len(got.Status.Devices) != 1 {
		t.Fatalf("expected one AllocatedDeviceStatus; got %+v", got.Status.Devices)
	}
	devStatus := got.Status.Devices[0]
	if len(devStatus.Conditions) == 0 || devStatus.Conditions[0].Type != "Ready" || devStatus.Conditions[0].Status != metav1.ConditionTrue {
		t.Errorf("expected Ready=True condition; got %+v", devStatus.Conditions)
	}

	select {
	case ev := <-rec.Events:
		if !strings.Contains(ev, reasonAllocated) {
			t.Errorf("expected Allocated event; got %s", ev)
		}
	default:
		t.Errorf("expected Allocated event")
	}
}

func TestClaim_NoSliceRequeues(t *testing.T) {
	claim := ourClaim("c-stuck", "ns-x", v1alpha1.DriverName)
	cli := newFakeClient(t, claim)
	rec := newFakeRecorder(4)
	r := &ClaimReconciler{Client: cli, Scheme: newTestScheme(t), Recorder: rec}

	key := client.ObjectKey{Namespace: "ns-x", Name: "c-stuck"}
	res := reconcileOnce(t, r, key)
	if !res.Requeue {
		t.Errorf("no-slice path must Requeue=true; got %+v", res)
	}

	got := getClaim(t, cli, key)
	if got.Status.Allocation != nil {
		t.Errorf("no-slice path must not allocate; got %+v", got.Status.Allocation)
	}
	select {
	case ev := <-rec.Events:
		if !strings.Contains(ev, reasonNoAvailable) {
			t.Errorf("expected NoAvailableDevice event; got %s", ev)
		}
	default:
		t.Errorf("expected NoAvailableDevice event")
	}
}

func TestClaim_SubClassWhole_FiltersDynamic(t *testing.T) {
	// Two devices on the same node: one FixedTemplate (whole), one
	// Dynamic. A claim asking for the .whole sub-class must only pick
	// the FixedTemplate device.
	whole := fixtureHealthyWholeDevice("nodeS-npu-0", 0)
	dyn := fixtureHealthyDynamicDevice("nodeS-npu-1", 1, 8)
	slice := fixtureSlice("npu-dra-nodeS", "nodeS", dyn, whole)

	claim := ourClaim("c-whole", "ns-s", v1alpha1.DriverName+".whole")
	cli := newFakeClient(t, claim, slice)
	r := &ClaimReconciler{Client: cli, Scheme: newTestScheme(t), Recorder: newFakeRecorder(4)}

	key := client.ObjectKey{Namespace: "ns-s", Name: "c-whole"}
	reconcileOnce(t, r, key)

	got := getClaim(t, cli, key)
	if got.Status.Allocation == nil || len(got.Status.Allocation.Devices.Results) != 1 {
		t.Fatalf("expected one allocation result; got %+v", got.Status.Allocation)
	}
	r0 := got.Status.Allocation.Devices.Results[0]
	if r0.Device != "nodeS-npu-0" {
		t.Errorf(".whole must pick the FixedTemplate device nodeS-npu-0; got %s", r0.Device)
	}
}

func TestClaim_SubClassDynamic_FiltersWhole(t *testing.T) {
	// Same fixture as the .whole test but the claim asks for .dynamic
	// → must pick the Dynamic device.
	whole := fixtureHealthyWholeDevice("nodeD-npu-0", 0)
	dyn := fixtureHealthyDynamicDevice("nodeD-npu-1", 1, 8)
	slice := fixtureSlice("npu-dra-nodeD", "nodeD", whole, dyn)

	claim := ourClaim("c-dyn", "ns-d", v1alpha1.DriverName+".dynamic")
	cli := newFakeClient(t, claim, slice)
	r := &ClaimReconciler{Client: cli, Scheme: newTestScheme(t), Recorder: newFakeRecorder(4)}

	key := client.ObjectKey{Namespace: "ns-d", Name: "c-dyn"}
	reconcileOnce(t, r, key)

	got := getClaim(t, cli, key)
	if got.Status.Allocation == nil || len(got.Status.Allocation.Devices.Results) != 1 {
		t.Fatalf("expected one allocation result; got %+v", got.Status.Allocation)
	}
	r0 := got.Status.Allocation.Devices.Results[0]
	if r0.Device != "nodeD-npu-1" {
		t.Errorf(".dynamic must pick the Dynamic device nodeD-npu-1; got %s", r0.Device)
	}
}

func TestClaim_MultiClaim_DistinctDevices(t *testing.T) {
	// Two claims, two devices: claim-1 must allocate device-0,
	// claim-2 must allocate device-1 (deterministic by lex order).
	d0 := fixtureHealthyWholeDevice("nodeM-npu-0", 0)
	d1 := fixtureHealthyWholeDevice("nodeM-npu-1", 1)
	slice := fixtureSlice("npu-dra-nodeM", "nodeM", d0, d1)

	c1 := ourClaim("c-1", "ns", v1alpha1.DriverName)
	c2 := ourClaim("c-2", "ns", v1alpha1.DriverName)
	cli := newFakeClient(t, c1, c2, slice)
	r := &ClaimReconciler{Client: cli, Scheme: newTestScheme(t), Recorder: newFakeRecorder(8)}

	reconcileOnce(t, r, client.ObjectKey{Namespace: "ns", Name: "c-1"})
	reconcileOnce(t, r, client.ObjectKey{Namespace: "ns", Name: "c-2"})

	g1 := getClaim(t, cli, client.ObjectKey{Namespace: "ns", Name: "c-1"})
	g2 := getClaim(t, cli, client.ObjectKey{Namespace: "ns", Name: "c-2"})
	if g1.Status.Allocation == nil || g2.Status.Allocation == nil {
		t.Fatalf("expected both claims allocated; got c1=%+v c2=%+v", g1.Status.Allocation, g2.Status.Allocation)
	}
	d1Pick := g1.Status.Allocation.Devices.Results[0].Device
	d2Pick := g2.Status.Allocation.Devices.Results[0].Device
	if d1Pick == d2Pick {
		t.Errorf("multi-claim must pick distinct devices; both got %s", d1Pick)
	}
	if d1Pick != "nodeM-npu-0" {
		t.Errorf("c-1 should win nodeM-npu-0 (lex first); got %s", d1Pick)
	}
	if d2Pick != "nodeM-npu-1" {
		t.Errorf("c-2 should get nodeM-npu-1 after c-1 took 0; got %s", d2Pick)
	}
}

func TestClaim_AlreadyAllocated_NoChange(t *testing.T) {
	// Claim already has Status.Allocation set; controller must NOT
	// re-allocate (avoid clobbering scheduler/kubelet state).
	claim := ourClaim("c-already", "ns-a", v1alpha1.DriverName)
	claim.Status.Allocation = &resourceapi.AllocationResult{
		Devices: resourceapi.DeviceAllocationResult{
			Results: []resourceapi.DeviceRequestAllocationResult{
				{Driver: v1alpha1.DriverName, Pool: "preset", Device: "preset-npu-0", Request: "req-0"},
			},
		},
	}
	dev := fixtureHealthyWholeDevice("nodeA-npu-0", 0)
	slice := fixtureSlice("npu-dra-nodeA", "nodeA", dev)
	cli := newFakeClient(t, claim, slice)
	r := &ClaimReconciler{Client: cli, Scheme: newTestScheme(t), Recorder: newFakeRecorder(4)}

	reconcileOnce(t, r, client.ObjectKey{Namespace: "ns-a", Name: "c-already"})

	got := getClaim(t, cli, client.ObjectKey{Namespace: "ns-a", Name: "c-already"})
	if len(got.Status.Allocation.Devices.Results) != 1 ||
		got.Status.Allocation.Devices.Results[0].Pool != "preset" {
		t.Errorf("already-allocated claim must be unchanged; got %+v", got.Status.Allocation)
	}
}

func TestClaim_StripsPhase4Annotations(t *testing.T) {
	claim := ourClaim("c-mig", "ns-m", v1alpha1.DriverName)
	claim.Annotations = map[string]string{
		AnnotationAllocationDeferred:            "true",
		AnnotationAllocationDeferredReason:      ReasonPhase4Skeleton,
		AnnotationAllocationDeferredMessage:     MessagePhase4Skeleton,
		AnnotationAllocationDeferredObservedGen: "1",
	}
	dev := fixtureHealthyWholeDevice("nodeM-npu-0", 0)
	slice := fixtureSlice("npu-dra-nodeM", "nodeM", dev)

	cli := newFakeClient(t, claim, slice)
	rec := newFakeRecorder(8)
	r := &ClaimReconciler{Client: cli, Scheme: newTestScheme(t), Recorder: rec}

	key := client.ObjectKey{Namespace: "ns-m", Name: "c-mig"}

	// First reconcile: strip annotations + Requeue (no allocation yet).
	res := reconcileOnce(t, r, key)
	if !res.Requeue {
		t.Errorf("first reconcile on migration path should Requeue; got %+v", res)
	}
	got := getClaim(t, cli, key)
	for _, k := range []string{
		AnnotationAllocationDeferred,
		AnnotationAllocationDeferredReason,
		AnnotationAllocationDeferredMessage,
		AnnotationAllocationDeferredObservedGen,
	} {
		if _, ok := got.Annotations[k]; ok {
			t.Errorf("annotation %s should have been stripped; still present", k)
		}
	}
	if got.Status.Allocation != nil {
		t.Errorf("first reconcile (strip pass) must not allocate; got %+v", got.Status.Allocation)
	}

	// Second reconcile: real allocation now that annotations are gone.
	reconcileOnce(t, r, key)
	got2 := getClaim(t, cli, key)
	if got2.Status.Allocation == nil {
		t.Fatalf("expected allocation after second reconcile; got nil")
	}
}

// TestClaim_BareDriverNamePrefixMatching exercises both exact and "/sub"
// and ".something" prefix matches against v1alpha1.DriverName so the
// Phase 5 controller continues to accept claims authored against
// either DeviceClass naming shape.
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
