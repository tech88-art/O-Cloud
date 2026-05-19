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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	inferencev1alpha1 "github.com/tech88-art/O-Cloud/operators/inference-operator/api/v1alpha1"
)

func newModelService(name, ns, poolName string) *inferencev1alpha1.ModelService {
	return &inferencev1alpha1.ModelService{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  ns,
			Generation: 1,
			UID:        types.UID(name + "-uid"),
		},
		Spec: inferencev1alpha1.ModelServiceSpec{
			Model: inferencev1alpha1.ModelSpec{
				Image:     "vllm-ascend:test",
				ModelPath: "/models/llama-7b",
			},
			PDPair: inferencev1alpha1.PDPairSpec{
				Prefill:     inferencev1alpha1.PDReplicaSpec{Replicas: 1},
				Decode:      inferencev1alpha1.PDReplicaSpec{Replicas: 1},
				RouterLabel: "inference.ocloud.edge.example.com/pd-role",
			},
			NPUSlicePoolRef: corev1.LocalObjectReference{Name: poolName},
		},
	}
}

func reconcile(t *testing.T, r *ModelServiceReconciler, key client.ObjectKey) ctrl.Result {
	t.Helper()
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	if err != nil {
		t.Fatalf("Reconcile(%s): %v", key, err)
	}
	return res
}

func getModelService(t *testing.T, c client.Client, key client.ObjectKey) *inferencev1alpha1.ModelService {
	t.Helper()
	var ms inferencev1alpha1.ModelService
	if err := c.Get(context.Background(), key, &ms); err != nil {
		t.Fatalf("Get(%s): %v", key, err)
	}
	return &ms
}

// TestReconcile_FinalizerAdded verifies the first reconcile adds the
// finalizer + Requeue, no allocation work done yet.
func TestReconcile_FinalizerAdded(t *testing.T) {
	ms := newModelService("ms-1", "ns-a", "pool-1")
	cli := newFakeClient(t, ms)
	r := &ModelServiceReconciler{Client: cli, Scheme: newTestScheme(t), Recorder: newFakeRecorder(4)}

	res := reconcile(t, r, client.ObjectKey{Namespace: "ns-a", Name: "ms-1"})
	if !res.Requeue {
		t.Errorf("first reconcile should Requeue after finalizer add; got %+v", res)
	}

	got := getModelService(t, cli, client.ObjectKey{Namespace: "ns-a", Name: "ms-1"})
	found := false
	for _, f := range got.Finalizers {
		if f == FinalizerName {
			found = true
		}
	}
	if !found {
		t.Errorf("finalizer %s not added; got %+v", FinalizerName, got.Finalizers)
	}
}

// TestReconcile_HappyPath_Provisioning verifies that with a finalizer
// already present + NPUSlicePool found with totalSlices>0, the
// reconcile lifts Status.Phase to Provisioning.
func TestReconcile_HappyPath_Provisioning(t *testing.T) {
	ms := newModelService("ms-2", "ns-b", "pool-2")
	ms.Finalizers = []string{FinalizerName}
	pool := newPoolFixture("ns-b", "pool-2", 3)
	cli := newFakeClient(t, ms, pool)
	r := &ModelServiceReconciler{Client: cli, Scheme: newTestScheme(t), Recorder: newFakeRecorder(4)}

	reconcile(t, r, client.ObjectKey{Namespace: "ns-b", Name: "ms-2"})

	got := getModelService(t, cli, client.ObjectKey{Namespace: "ns-b", Name: "ms-2"})
	if got.Status.Phase != inferencev1alpha1.PhaseProvisioning {
		t.Errorf("phase: want Provisioning, got %q", got.Status.Phase)
	}
	if c, ok := FindCondition(got.Status.Conditions, ConditionPoolUnresolved); !ok {
		t.Errorf("missing PoolUnresolved condition (should be True/found)")
	} else if c.Status != metav1.ConditionTrue {
		t.Errorf("PoolUnresolved status: want True (found), got %v", c.Status)
	}
	if c, ok := FindCondition(got.Status.Conditions, ConditionAllocationReady); !ok || c.Status != metav1.ConditionFalse {
		t.Errorf("expected AllocationReady=False (scaffold); got %+v", c)
	}
}

// TestReconcile_PoolNotFound_PhaseFailed verifies that a non-existent
// pool yields phase=Failed + PoolUnresolved=False/NPUSlicePoolNotFound.
func TestReconcile_PoolNotFound_PhaseFailed(t *testing.T) {
	ms := newModelService("ms-3", "ns-c", "nonexistent")
	ms.Finalizers = []string{FinalizerName}
	cli := newFakeClient(t, ms)
	rec := newFakeRecorder(4)
	r := &ModelServiceReconciler{Client: cli, Scheme: newTestScheme(t), Recorder: rec}

	reconcile(t, r, client.ObjectKey{Namespace: "ns-c", Name: "ms-3"})

	got := getModelService(t, cli, client.ObjectKey{Namespace: "ns-c", Name: "ms-3"})
	if got.Status.Phase != inferencev1alpha1.PhaseFailed {
		t.Errorf("phase: want Failed, got %q", got.Status.Phase)
	}
	if c, ok := FindCondition(got.Status.Conditions, ConditionPoolUnresolved); !ok || c.Status != metav1.ConditionFalse {
		t.Errorf("expected PoolUnresolved=False; got %+v", c)
	} else if c.Reason != reasonNPUSlicePoolNotFound {
		t.Errorf("PoolUnresolved reason: want %s, got %s", reasonNPUSlicePoolNotFound, c.Reason)
	}
	select {
	case ev := <-rec.Events:
		// good — pool-not-found event emitted
		_ = ev
	default:
		t.Errorf("expected NPUSlicePoolNotFound event; recorder empty")
	}
}

// TestReconcile_PoolFoundButEmpty_WaitingForPool verifies that a pool
// existing but with status.totalSlices=0 leaves phase=Provisioning
// and writes the WaitingForPool reason on AllocationReady.
func TestReconcile_PoolFoundButEmpty_WaitingForPool(t *testing.T) {
	ms := newModelService("ms-4", "ns-d", "pool-empty")
	ms.Finalizers = []string{FinalizerName}
	pool := newPoolFixture("ns-d", "pool-empty", 0)
	cli := newFakeClient(t, ms, pool)
	r := &ModelServiceReconciler{Client: cli, Scheme: newTestScheme(t), Recorder: newFakeRecorder(4)}

	reconcile(t, r, client.ObjectKey{Namespace: "ns-d", Name: "ms-4"})

	got := getModelService(t, cli, client.ObjectKey{Namespace: "ns-d", Name: "ms-4"})
	if got.Status.Phase != inferencev1alpha1.PhaseProvisioning {
		t.Errorf("phase: want Provisioning while waiting; got %q", got.Status.Phase)
	}
	if c, ok := FindCondition(got.Status.Conditions, ConditionAllocationReady); !ok || c.Status != metav1.ConditionFalse {
		t.Errorf("expected AllocationReady=False; got %+v", c)
	} else if c.Reason != reasonWaitingForPool {
		t.Errorf("AllocationReady reason: want %s, got %s", reasonWaitingForPool, c.Reason)
	}
}

// TestReconcile_Deletion_FinalizerRemoved verifies that on delete the
// finalizer is removed (T006 scaffold: no owned resources to drain;
// T008 expands this with Deployment + ResourceClaim drain).
func TestReconcile_Deletion_FinalizerRemoved(t *testing.T) {
	ms := newModelService("ms-5", "ns-e", "pool-x")
	ms.Finalizers = []string{FinalizerName}
	now := metav1.Now()
	ms.DeletionTimestamp = &now
	cli := newFakeClient(t, ms)
	r := &ModelServiceReconciler{Client: cli, Scheme: newTestScheme(t), Recorder: newFakeRecorder(4)}

	reconcile(t, r, client.ObjectKey{Namespace: "ns-e", Name: "ms-5"})

	// After the controller's RemoveFinalizer + Update, the fake client
	// fully removes the object (no other finalizers blocking GC).
	var ms2 inferencev1alpha1.ModelService
	err := cli.Get(context.Background(), client.ObjectKey{Namespace: "ns-e", Name: "ms-5"}, &ms2)
	if err == nil {
		// Object still present — check finalizer was removed.
		for _, f := range ms2.Finalizers {
			if f == FinalizerName {
				t.Errorf("finalizer %s should have been removed; got %+v", FinalizerName, ms2.Finalizers)
			}
		}
	}
	// NotFound is also acceptable (fake client GC kicked in).
}

func TestSetCondition_SmokeFromInferenceOperator(t *testing.T) {
	// Verify the helper compiles + behaves consistently with the
	// npu-dra-driver mirror (duplicated text, same semantics).
	var conds []metav1.Condition
	SetCondition(&conds, metav1.Condition{Type: "X", Status: metav1.ConditionTrue, Reason: "r1"})
	SetCondition(&conds, metav1.Condition{Type: "X", Status: metav1.ConditionFalse, Reason: "r2"})
	if len(conds) != 1 {
		t.Errorf("SetCondition must dedupe by Type; got %+v", conds)
	}
	if conds[0].Status != metav1.ConditionFalse || conds[0].Reason != "r2" {
		t.Errorf("SetCondition update wrong: %+v", conds[0])
	}
	if !RemoveCondition(&conds, "X") {
		t.Errorf("RemoveCondition should return true on match")
	}
	if len(conds) != 0 {
		t.Errorf("RemoveCondition should empty the slice; got %+v", conds)
	}
}
