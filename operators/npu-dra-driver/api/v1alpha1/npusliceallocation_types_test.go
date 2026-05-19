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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/runtime"
)

// TestNPUSliceAllocation_JSONRoundTrip exercises Marshal/Unmarshal
// equivalence across three representative cases — the pattern P4-T-004
// established in types_test.go (empty / single / list).
func TestNPUSliceAllocation_JSONRoundTrip(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		in := &NPUSliceAllocation{}
		buf, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("marshal empty: %v", err)
		}
		var out NPUSliceAllocation
		if err := json.Unmarshal(buf, &out); err != nil {
			t.Fatalf("unmarshal empty: %v", err)
		}
		if !reflect.DeepEqual(in.Spec, out.Spec) {
			t.Errorf("empty Spec drift: in=%+v out=%+v", in.Spec, out.Spec)
		}
		if !reflect.DeepEqual(in.Status, out.Status) {
			t.Errorf("empty Status drift: in=%+v out=%+v", in.Status, out.Status)
		}
	})

	t.Run("populated", func(t *testing.T) {
		now := metav1.Now()
		in := &NPUSliceAllocation{
			TypeMeta: metav1.TypeMeta{
				Kind:       "NPUSliceAllocation",
				APIVersion: GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "alloc-c1-node-a-npu-0",
				UID:  types.UID("alloc-uid-1"),
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: "resource.k8s.io/v1beta1",
						Kind:       "ResourceClaim",
						Name:       "c1",
						UID:        "c1-uid",
					},
				},
			},
			Spec: NPUSliceAllocationSpec{
				ClaimRef: corev1.ObjectReference{
					APIVersion: "resource.k8s.io/v1beta1",
					Kind:       "ResourceClaim",
					Namespace:  "ns-a",
					Name:       "c1",
					UID:        "c1-uid",
				},
				SliceRef: SliceReference{
					Driver: DriverName,
					Pool:   "nodeA",
					Device: "nodeA-npu-0",
				},
				NodeName:        "nodeA",
				AICores:         32,
				ModelServiceRef: "ns-a/llama",
			},
			Status: NPUSliceAllocationStatus{
				Phase: NPUSliceAllocationPhaseAllocated,
				Conditions: []metav1.Condition{
					{
						Type:               ConditionAvailable,
						Status:             metav1.ConditionTrue,
						Reason:             "Allocated",
						Message:            "device bound",
						LastTransitionTime: now,
					},
				},
				AllocatedAt: &now,
			},
		}
		buf, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("marshal populated: %v", err)
		}
		var out NPUSliceAllocation
		if err := json.Unmarshal(buf, &out); err != nil {
			t.Fatalf("unmarshal populated: %v", err)
		}
		if !reflect.DeepEqual(in.Spec, out.Spec) {
			t.Errorf("Spec drift: in=%+v out=%+v", in.Spec, out.Spec)
		}
		if out.Status.Phase != NPUSliceAllocationPhaseAllocated {
			t.Errorf("status.phase: want Allocated, got %q", out.Status.Phase)
		}
		if len(out.Status.Conditions) != 1 || out.Status.Conditions[0].Type != ConditionAvailable {
			t.Errorf("status.conditions drift: %+v", out.Status.Conditions)
		}
		if out.Status.AllocatedAt == nil {
			t.Errorf("status.allocatedAt should round-trip; got nil")
		}
	})

	t.Run("list-with-items", func(t *testing.T) {
		in := &NPUSliceAllocationList{
			TypeMeta: metav1.TypeMeta{
				Kind:       "NPUSliceAllocationList",
				APIVersion: GroupVersion.String(),
			},
			Items: []NPUSliceAllocation{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "alloc-1"},
					Spec: NPUSliceAllocationSpec{
						ClaimRef: corev1.ObjectReference{Namespace: "ns", Name: "c1"},
						SliceRef: SliceReference{Driver: DriverName, Pool: "nodeA", Device: "nodeA-npu-0"},
						NodeName: "nodeA",
						AICores:  32,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "alloc-2"},
					Spec: NPUSliceAllocationSpec{
						ClaimRef: corev1.ObjectReference{Namespace: "ns", Name: "c2"},
						SliceRef: SliceReference{Driver: DriverName, Pool: "nodeB", Device: "nodeB-npu-0"},
						NodeName: "nodeB",
						AICores:  8,
					},
				},
			},
		}
		buf, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("marshal list: %v", err)
		}
		var out NPUSliceAllocationList
		if err := json.Unmarshal(buf, &out); err != nil {
			t.Fatalf("unmarshal list: %v", err)
		}
		if len(out.Items) != 2 {
			t.Fatalf("list items count drift: want 2, got %d", len(out.Items))
		}
		if !reflect.DeepEqual(in.Items, out.Items) {
			t.Errorf("list items drift: in=%+v out=%+v", in.Items, out.Items)
		}
	})
}

// TestNPUSliceAllocation_DeepCopy verifies the controller-gen-emitted
// DeepCopy methods produce an independent clone. Mutating the original
// must not affect the copy.
func TestNPUSliceAllocation_DeepCopy(t *testing.T) {
	original := &NPUSliceAllocation{
		ObjectMeta: metav1.ObjectMeta{Name: "orig"},
		Spec: NPUSliceAllocationSpec{
			ClaimRef:        corev1.ObjectReference{Name: "c1"},
			SliceRef:        SliceReference{Driver: DriverName, Pool: "nodeA", Device: "nodeA-npu-0"},
			NodeName:        "nodeA",
			AICores:         32,
			ModelServiceRef: "ns/llama",
		},
		Status: NPUSliceAllocationStatus{
			Phase: NPUSliceAllocationPhaseAllocated,
			Conditions: []metav1.Condition{
				{Type: ConditionAvailable, Status: metav1.ConditionTrue},
			},
		},
	}
	clone := original.DeepCopy()
	clone.Spec.SliceRef.Device = "MUTATED"
	clone.Status.Conditions[0].Status = metav1.ConditionFalse
	if original.Spec.SliceRef.Device == "MUTATED" {
		t.Error("DeepCopy must NOT share Spec.SliceRef with original")
	}
	if original.Status.Conditions[0].Status == metav1.ConditionFalse {
		t.Error("DeepCopy must NOT share Status.Conditions with original")
	}
}

// TestNPUSliceAllocation_DeepCopyObject verifies the runtime.Object
// interface conformance (controller-gen emits DeepCopyObject which
// returns runtime.Object). Required for scheme-registered types.
func TestNPUSliceAllocation_DeepCopyObject(t *testing.T) {
	in := &NPUSliceAllocation{ObjectMeta: metav1.ObjectMeta{Name: "x"}}
	var ri runtime.Object = in.DeepCopyObject()
	out, ok := ri.(*NPUSliceAllocation)
	if !ok {
		t.Fatalf("DeepCopyObject must return *NPUSliceAllocation; got %T", ri)
	}
	if out.Name != "x" {
		t.Errorf("DeepCopyObject lost Name field; got %q", out.Name)
	}
}

// TestSchemeRegistration verifies AddToScheme registers both
// NPUSliceAllocation and NPUSliceAllocationList kinds against the
// supplied scheme. A failing AddToScheme would mean the manager cannot
// read/write NPUSliceAllocation through controller-runtime.
func TestSchemeRegistration(t *testing.T) {
	s := runtime.NewScheme()
	if err := AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	gvk := GroupVersion.WithKind("NPUSliceAllocation")
	if _, err := s.New(gvk); err != nil {
		t.Errorf("scheme.New(%s) error: %v", gvk, err)
	}
	gvkList := GroupVersion.WithKind("NPUSliceAllocationList")
	if _, err := s.New(gvkList); err != nil {
		t.Errorf("scheme.New(%s) error: %v", gvkList, err)
	}
}
