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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/npu-dra-driver/api/v1alpha1"
	"github.com/tech88-art/O-Cloud/operators/npu-dra-driver/internal/template"
)

// nstFakeClient returns a fake client with NPUSliceTemplate status
// subresource registered. The suite_test.go newFakeClient seeds with
// ResourceClaim + NPUSliceAllocation status subresources; this helper
// adds NPUSliceTemplate to the WithStatusSubresource list so
// Status().Update calls take effect.
func nstFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := newTestScheme(t)
	builder := newStatusSubresourceClientBuilder(scheme, &v1alpha1.NPUSliceTemplate{})
	return builder.WithObjects(objs...).Build()
}

// TestNPUSliceTemplateReconciler_EmptyCompositionStampsValidatedFalse
// covers Phase 7 P7-T-007 acceptance case 1/3 (controller test): a
// NPUSliceTemplate with empty Spec.Composition gets
// Validated=False + reason=EmptyComposition after reconcile.
func TestNPUSliceTemplateReconciler_EmptyCompositionStampsValidatedFalse(t *testing.T) {
	ctx := context.Background()
	nst := &v1alpha1.NPUSliceTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "empty-comp", Generation: 1},
		Spec:       v1alpha1.NPUSliceTemplateSpec{}, // no Composition
	}

	c := nstFakeClient(t, nst)
	r := &NPUSliceTemplateReconciler{
		Client:   c,
		Scheme:   newTestScheme(t),
		Recorder: newFakeRecorder(8),
		Engine:   template.New(),
	}

	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: nst.Name}})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	var got v1alpha1.NPUSliceTemplate
	if err := c.Get(ctx, types.NamespacedName{Name: nst.Name}, &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cs := findCondition(got.Status.Conditions, v1alpha1.ConditionTypeValidated); cs == nil {
		t.Fatal("Validated condition missing after reconcile")
	} else {
		if cs.Status != metav1.ConditionFalse {
			t.Fatalf("Validated.Status = %q, want False", cs.Status)
		}
		if cs.Reason != template.ReasonEmptyComposition {
			t.Fatalf("Validated.Reason = %q, want %q", cs.Reason, template.ReasonEmptyComposition)
		}
	}
	if cs := findCondition(got.Status.Conditions, v1alpha1.ConditionTypeAllocatable); cs == nil {
		t.Fatal("Allocatable condition missing after reconcile")
	} else if cs.Status != metav1.ConditionFalse {
		t.Fatalf("Allocatable.Status = %q, want False (gated on Validated)", cs.Status)
	}
}

// TestNPUSliceTemplateReconciler_ValidCompositionStampsValidatedAndAllocatable
// covers Phase 7 P7-T-007 acceptance case 2/3: a valid composition
// (Qwen 8B PD-pair) gets Validated=True + Allocatable=True +
// FallbackAppliedReason populated.
func TestNPUSliceTemplateReconciler_ValidCompositionStampsValidatedAndAllocatable(t *testing.T) {
	ctx := context.Background()
	nst := &v1alpha1.NPUSliceTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "qwen-8b-pd-pair", Generation: 2},
		Spec: v1alpha1.NPUSliceTemplateSpec{
			Composition: []v1alpha1.TemplatePart{
				{Type: v1alpha1.PartTypeVir04, Count: 1},
				{Type: v1alpha1.PartTypeVir08, Count: 1},
			},
			FallbackStrategy: v1alpha1.FallbackStrategyFixedTemplateCombination,
		},
	}

	c := nstFakeClient(t, nst)
	r := &NPUSliceTemplateReconciler{Client: c, Scheme: newTestScheme(t), Recorder: newFakeRecorder(8), Engine: template.New()}
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: nst.Name}}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	var got v1alpha1.NPUSliceTemplate
	if err := c.Get(ctx, types.NamespacedName{Name: nst.Name}, &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cs := findCondition(got.Status.Conditions, v1alpha1.ConditionTypeValidated); cs == nil || cs.Status != metav1.ConditionTrue {
		t.Fatalf("Validated = %+v, want True", cs)
	}
	if cs := findCondition(got.Status.Conditions, v1alpha1.ConditionTypeAllocatable); cs == nil || cs.Status != metav1.ConditionTrue {
		t.Fatalf("Allocatable = %+v, want True (Phase 7 W1 placeholder)", cs)
	}
	if got.Status.FallbackAppliedReason != "decomposed into 1×vir04 + 1×vir08" {
		t.Fatalf("FallbackAppliedReason = %q, want \"decomposed into 1×vir04 + 1×vir08\"",
			got.Status.FallbackAppliedReason)
	}
	if got.Status.ObservedGeneration != 2 {
		t.Fatalf("ObservedGeneration = %d, want 2", got.Status.ObservedGeneration)
	}
}

// TestNPUSliceTemplateReconciler_DynamicShardRejected covers Phase 7
// P7-T-007 acceptance case 3/3: refuse strategy with dynamic-shard
// composition surfaces DynamicShardNotSupported (Phase 7 W1 has no
// driver-layer hook; gated on Phase 8+ per ADR-0011 §後果).
func TestNPUSliceTemplateReconciler_DynamicShardRejected(t *testing.T) {
	ctx := context.Background()
	nst := &v1alpha1.NPUSliceTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "dyn-shard-strict", Generation: 1},
		Spec: v1alpha1.NPUSliceTemplateSpec{
			Composition: []v1alpha1.TemplatePart{
				{Type: v1alpha1.PartTypeDynamicShard, Count: 1, AICoreRequest: 4},
			},
			FallbackStrategy: v1alpha1.FallbackStrategyRefuse,
		},
	}

	c := nstFakeClient(t, nst)
	r := &NPUSliceTemplateReconciler{Client: c, Scheme: newTestScheme(t), Recorder: newFakeRecorder(8), Engine: template.New()}
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: nst.Name}}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	var got v1alpha1.NPUSliceTemplate
	if err := c.Get(ctx, types.NamespacedName{Name: nst.Name}, &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cs := findCondition(got.Status.Conditions, v1alpha1.ConditionTypeValidated); cs == nil || cs.Status != metav1.ConditionFalse {
		t.Fatalf("Validated = %+v, want False", cs)
	} else if cs.Reason != template.ReasonDynamicShardNotSupported {
		t.Fatalf("Validated.Reason = %q, want %q", cs.Reason, template.ReasonDynamicShardNotSupported)
	}
	if got.Status.FallbackAppliedReason != "" {
		t.Fatalf("FallbackAppliedReason = %q, want \"\" (validation failed)",
			got.Status.FallbackAppliedReason)
	}
}

// findCondition returns the condition with the given type, or nil if
// absent. (Local helper for test assertions.)
func findCondition(conds []metav1.Condition, t string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == t {
			return &conds[i]
		}
	}
	return nil
}
