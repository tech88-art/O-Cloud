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
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestSetCondition_New(t *testing.T) {
	var conds []metav1.Condition
	SetCondition(&conds, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "Initialized",
		Message: "first time",
	})
	if got := len(conds); got != 1 {
		t.Fatalf("expected 1 condition appended, got %d", got)
	}
	if conds[0].Type != "Ready" || conds[0].Status != metav1.ConditionTrue {
		t.Fatalf("unexpected condition stored: %+v", conds[0])
	}
	if conds[0].LastTransitionTime.IsZero() {
		t.Fatalf("LastTransitionTime should default to now when unset, got zero")
	}
}

func TestSetCondition_ReplaceSameStatus(t *testing.T) {
	original := metav1.Time{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	conds := []metav1.Condition{{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Old",
		Message:            "old message",
		LastTransitionTime: original,
	}}

	SetCondition(&conds, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "New",
		Message: "new message",
	})

	if len(conds) != 1 {
		t.Fatalf("expected 1 condition after replace, got %d", len(conds))
	}
	if !conds[0].LastTransitionTime.Equal(&original) {
		t.Fatalf("LastTransitionTime must be preserved when Status unchanged; got %v, want %v",
			conds[0].LastTransitionTime, original)
	}
	if conds[0].Reason != "New" || conds[0].Message != "new message" {
		t.Fatalf("Reason/Message should be updated; got %+v", conds[0])
	}
}

func TestSetCondition_ReplaceDifferentStatus(t *testing.T) {
	original := metav1.Time{Time: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	conds := []metav1.Condition{{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "Pending",
		LastTransitionTime: original,
	}}

	before := time.Now().Add(-time.Second)
	SetCondition(&conds, metav1.Condition{
		Type:    "Ready",
		Status:  metav1.ConditionTrue,
		Reason:  "Ready",
		Message: "all good",
	})

	if len(conds) != 1 {
		t.Fatalf("expected 1 condition after replace, got %d", len(conds))
	}
	if conds[0].LastTransitionTime.Equal(&original) {
		t.Fatalf("LastTransitionTime must update when Status changes; got %v (unchanged)", conds[0].LastTransitionTime)
	}
	if conds[0].LastTransitionTime.Time.Before(before) {
		t.Fatalf("LastTransitionTime must be ~now; got %v, before=%v", conds[0].LastTransitionTime, before)
	}
}

func TestRemoveCondition_Found(t *testing.T) {
	conds := []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionTrue},
		{Type: "Synced", Status: metav1.ConditionFalse},
	}
	if !RemoveCondition(&conds, "Ready") {
		t.Fatalf("RemoveCondition returned false; expected true for present type")
	}
	if len(conds) != 1 {
		t.Fatalf("expected 1 condition after remove, got %d", len(conds))
	}
	if conds[0].Type != "Synced" {
		t.Fatalf("wrong condition remains: %+v", conds[0])
	}
}

func TestRemoveCondition_NotFound(t *testing.T) {
	conds := []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}
	if RemoveCondition(&conds, "Missing") {
		t.Fatalf("RemoveCondition returned true for absent type")
	}
	if len(conds) != 1 {
		t.Fatalf("conditions list mutated unexpectedly: %d entries", len(conds))
	}
}

type fakeObj struct {
	metav1.ObjectMeta
}

func (f *fakeObj) GetObjectMeta() metav1.Object { return &f.ObjectMeta }

func TestMakeOwnerRef(t *testing.T) {
	obj := &fakeObj{ObjectMeta: metav1.ObjectMeta{
		Name: "pool-1",
		UID:  "uid-1234",
	}}
	gvk := schema.GroupVersionKind{
		Group:   "ims.ocloud.edge.example.com",
		Version: "v1alpha1",
		Kind:    "NPUSlicePool",
	}
	ref := MakeOwnerRef(&obj.ObjectMeta, gvk)
	if ref.APIVersion != "ims.ocloud.edge.example.com/v1alpha1" {
		t.Fatalf("APIVersion mismatch: %q", ref.APIVersion)
	}
	if ref.Kind != "NPUSlicePool" {
		t.Fatalf("Kind mismatch: %q", ref.Kind)
	}
	if ref.Name != "pool-1" {
		t.Fatalf("Name mismatch: %q", ref.Name)
	}
	if string(ref.UID) != "uid-1234" {
		t.Fatalf("UID mismatch: %q", ref.UID)
	}
	if ref.Controller == nil || !*ref.Controller {
		t.Fatalf("Controller flag should be true")
	}
	if ref.BlockOwnerDeletion == nil || !*ref.BlockOwnerDeletion {
		t.Fatalf("BlockOwnerDeletion flag should be true")
	}
}

func TestRequeueAfter_Clamped(t *testing.T) {
	if got := RequeueAfter(500 * time.Millisecond); got.RequeueAfter != time.Second {
		t.Fatalf("RequeueAfter(500ms) = %v; want clamp to 1s", got.RequeueAfter)
	}
	if got := RequeueAfter(10 * time.Second); got.RequeueAfter != 10*time.Second {
		t.Fatalf("RequeueAfter(10s) = %v; want 10s", got.RequeueAfter)
	}
}
