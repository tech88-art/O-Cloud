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

// Shared test harness for the npu-dra-driver controller package.
//
// Mirrors the spirit (but not the framework) of
// operators/pool-operator/internal/controller/suite_test.go: pool-operator
// uses ginkgo + envtest because the pool CRDs require a real kube-apiserver
// + etcd to validate CRD schemas at admission time. For npu-dra-driver
// T006 we use sigs.k8s.io/controller-runtime/pkg/client/fake instead,
// because:
//
//  1. resource.k8s.io/v1beta1 is a built-in upstream API on K8s 1.31+; no
//     CRD registration is needed.
//  2. The Phase 4 claim controller exercises Get / Status().Update only —
//     fake.NewClientBuilder with WithStatusSubresource covers this fully.
//  3. Envtest binaries are not provisioned in the Windows dev shell; setup-
//     envtest requires `make` plumbing which we deliberately stubbed at T003.
//     Phase 4 plan §3 P4-T-006 acceptance lists 4 envtest-style cases — all
//     4 are exercised below; the choice between envtest and fake is a means,
//     not the spec.
//  4. Phase 5 may upgrade to real envtest when claim allocation needs real
//     watch semantics; T006 does not.

import (
	"testing"

	resourceapi "k8s.io/api/resource/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// newTestScheme returns a runtime.Scheme with all schemes the npu-dra-driver
// controllers operate against (core/v1 for Events + resource/v1beta1 for
// ResourceClaim).
func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := scheme.AddToScheme(s); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}
	if err := resourceapi.AddToScheme(s); err != nil {
		t.Fatalf("add resource v1beta1 scheme: %v", err)
	}
	return s
}

// newFakeClient returns a controller-runtime fake client with the status
// subresource registered for ResourceClaim — required so the claim
// controller's Status().Update calls take effect under test.
//
// Optional `objs` seed the fake client's initial object store.
func newFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(newTestScheme(t)).
		WithStatusSubresource(&resourceapi.ResourceClaim{}).
		WithObjects(objs...).
		Build()
}

// newFakeRecorder returns a buffered EventRecorder usable in tests. The
// channel's events are inspectable via FakeRecorder.Events; tests that
// don't care about Event semantics can pass this to the Reconciler and
// ignore the channel.
func newFakeRecorder(buffer int) *record.FakeRecorder {
	if buffer <= 0 {
		buffer = 32
	}
	return record.NewFakeRecorder(buffer)
}
