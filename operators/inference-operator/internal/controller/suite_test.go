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

// Shared test harness for the inference-operator controller package.
//
// Mirrors operators/npu-dra-driver/internal/controller/suite_test.go:
// fake client over envtest because (a) the Phase 5 controller body
// touches only Get / List / Update / Status().Patch, which
// fake.NewClientBuilder supports natively; (b) setup-envtest binaries
// are not provisioned in the Windows dev shell. Phase 6+ may upgrade
// to real envtest when the inference-operator needs real watch
// semantics for owned Deployments + ResourceClaims.

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	resourceapi "k8s.io/api/resource/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	inferencev1alpha1 "github.com/tech88-art/O-Cloud/operators/inference-operator/api/v1alpha1"
)

// newTestScheme returns a runtime.Scheme with corev1 + inference-
// operator's v1alpha1 registered. NPUSlicePool is read via
// unstructured client (cross-module Go imports forbidden); the
// fake client supports unstructured natively when callers call
// `SetGroupVersionKind(...)` on the object before Create.
func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := scheme.AddToScheme(s); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}
	if err := inferencev1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("add inference v1alpha1 scheme: %v", err)
	}
	if err := appsv1.AddToScheme(s); err != nil {
		t.Fatalf("add apps/v1 scheme: %v", err)
	}
	if err := resourceapi.AddToScheme(s); err != nil {
		t.Fatalf("add resource/v1beta1 scheme: %v", err)
	}
	return s
}

// newFakeClient returns a controller-runtime fake client with the
// status subresource registered for ModelService.
func newFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithStatusSubresource(&inferencev1alpha1.ModelService{}).
		WithObjects(objs...).
		Build()
}

// newPoolFixture returns an unstructured NPUSlicePool fixture suitable
// for seeding the fake client. totalSlices populates the readiness
// heuristic (>0 → markProvisioning, ==0 → markWaitingForPool).
func newPoolFixture(namespace, name string, totalSlices int64) *unstructured.Unstructured {
	pool := &unstructured.Unstructured{}
	pool.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "ims.ocloud.edge.example.com",
		Version: "v1alpha1",
		Kind:    "NPUSlicePool",
	})
	pool.SetNamespace(namespace)
	pool.SetName(name)
	if totalSlices > 0 {
		_ = unstructured.SetNestedField(pool.Object, totalSlices, "status", "totalSlices")
	}
	return pool
}

func newFakeRecorder(buffer int) *record.FakeRecorder {
	if buffer <= 0 {
		buffer = 32
	}
	return record.NewFakeRecorder(buffer)
}
