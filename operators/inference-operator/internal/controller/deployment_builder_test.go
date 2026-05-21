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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	inferencev1alpha1 "github.com/tech88-art/O-Cloud/operators/inference-operator/api/v1alpha1"
)

// makeTestModelService builds a minimal ModelService for unit tests.
// Caller mutates Spec.PDPair fields per case.
func makeTestModelService() *inferencev1alpha1.ModelService {
	return &inferencev1alpha1.ModelService{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "llama-7b",
			Namespace: "ocloud-system",
		},
		Spec: inferencev1alpha1.ModelServiceSpec{
			Model: inferencev1alpha1.ModelSpec{
				Image:     "registry.example.com/vllm-ascend:v0.11.0",
				ModelPath: "/models/llama-7b",
			},
			PDPair: inferencev1alpha1.PDPairSpec{
				Prefill: inferencev1alpha1.PDReplicaSpec{Replicas: 2},
				Decode:  inferencev1alpha1.PDReplicaSpec{Replicas: 4},
			},
		},
	}
}

// TestBuildPDPairContainers exercises P6-T-105 — the buildPDPairContainers
// helper that materialises the PD-pair Pod container list. 4 cases
// covering plan acceptance + a 5th clamp-against-fallback edge.
func TestBuildPDPairContainers(t *testing.T) {
	t.Run("proxy disabled (default) → single container, Model.Image", func(t *testing.T) {
		ms := makeTestModelService()
		containers := buildPDPairContainers(ms, PDSidePrefill, "prefill", "npu-slice")
		if len(containers) != 1 {
			t.Fatalf("expected 1 container (no proxy), got %d", len(containers))
		}
		if containers[0].Image != "registry.example.com/vllm-ascend:v0.11.0" {
			t.Fatalf("expected Model.Image, got %q", containers[0].Image)
		}
		if containers[0].Name != "vllm-ascend" {
			t.Fatalf("expected container name vllm-ascend, got %q", containers[0].Name)
		}
	})

	t.Run("proxy enabled → 2 containers (main + pd-proxy)", func(t *testing.T) {
		ms := makeTestModelService()
		ms.Spec.PDPair.ProxyImage = "registry.example.com/vllm-pd-proxy:v0.12.0"
		containers := buildPDPairContainers(ms, PDSidePrefill, "prefill", "npu-slice")
		if len(containers) != 2 {
			t.Fatalf("expected 2 containers (main + proxy), got %d", len(containers))
		}
		if containers[1].Name != "pd-proxy" {
			t.Fatalf("expected sidecar name pd-proxy, got %q", containers[1].Name)
		}
		if containers[1].Image != "registry.example.com/vllm-pd-proxy:v0.12.0" {
			t.Fatalf("expected ProxyImage on sidecar, got %q", containers[1].Image)
		}
		// VLLM_PD_OTHER_SIDE should resolve to decode when this Pod
		// is the prefill side.
		var otherSide string
		for _, e := range containers[1].Env {
			if e.Name == "VLLM_PD_OTHER_SIDE" {
				otherSide = e.Value
			}
		}
		if otherSide != "decode" {
			t.Fatalf("expected VLLM_PD_OTHER_SIDE=decode for prefill side, got %q", otherSide)
		}
	})

	t.Run("fallbackImage takes precedence over Model.Image on main container", func(t *testing.T) {
		ms := makeTestModelService()
		ms.Spec.PDPair.FallbackImage = "busybox:1.36"
		containers := buildPDPairContainers(ms, PDSidePrefill, "prefill", "npu-slice")
		if len(containers) != 1 {
			t.Fatalf("expected 1 container, got %d", len(containers))
		}
		if containers[0].Image != "busybox:1.36" {
			t.Fatalf("expected fallbackImage to override Model.Image, got %q", containers[0].Image)
		}
	})

	t.Run("both ProxyImage + FallbackImage set: fallback on main, proxy on sidecar", func(t *testing.T) {
		ms := makeTestModelService()
		ms.Spec.PDPair.FallbackImage = "busybox:1.36"
		ms.Spec.PDPair.ProxyImage = "registry.example.com/vllm-pd-proxy:v0.12.0"
		containers := buildPDPairContainers(ms, PDSidePrefill, "prefill", "npu-slice")
		if len(containers) != 2 {
			t.Fatalf("expected 2 containers, got %d", len(containers))
		}
		if containers[0].Image != "busybox:1.36" {
			t.Fatalf("expected fallback on main, got %q", containers[0].Image)
		}
		if containers[1].Image != "registry.example.com/vllm-pd-proxy:v0.12.0" {
			t.Fatalf("expected proxy on sidecar, got %q", containers[1].Image)
		}
	})

	t.Run("decode side: VLLM_PD_OTHER_SIDE=prefill", func(t *testing.T) {
		ms := makeTestModelService()
		ms.Spec.PDPair.ProxyImage = "registry.example.com/vllm-pd-proxy:v0.12.0"
		containers := buildPDPairContainers(ms, PDSideDecode, "decode", "npu-slice")
		var otherSide string
		for _, e := range containers[1].Env {
			if e.Name == "VLLM_PD_OTHER_SIDE" {
				otherSide = e.Value
			}
		}
		if otherSide != "prefill" {
			t.Fatalf("expected VLLM_PD_OTHER_SIDE=prefill for decode side, got %q", otherSide)
		}
	})
}

// TestEffectiveSchedulerName covers Phase 7 P7-T-003 auto-stamp logic
// per ADR-0011 §1 + closes known-issues #11. Three plan-listed cases
// (default / override set / empty override pointer) plus the
// buildDeployment round-trip assertion (4th case) that the Pod template
// actually carries SchedulerName=npu-scheduler.
func TestEffectiveSchedulerName(t *testing.T) {
	t.Run("nil override → default npu-scheduler", func(t *testing.T) {
		ms := makeTestModelService()
		// SchedulerOverride is nil by default in makeTestModelService.
		if got := effectiveSchedulerName(ms); got != SchedulerNameDefault {
			t.Fatalf("effectiveSchedulerName(nil-override) = %q, want %q", got, SchedulerNameDefault)
		}
	})

	t.Run("override = default-scheduler → uses override", func(t *testing.T) {
		ms := makeTestModelService()
		override := "default-scheduler"
		ms.Spec.SchedulerOverride = &override
		if got := effectiveSchedulerName(ms); got != "default-scheduler" {
			t.Fatalf("effectiveSchedulerName(override=default-scheduler) = %q, want \"default-scheduler\"", got)
		}
	})

	t.Run("empty-string override pointer → default npu-scheduler", func(t *testing.T) {
		ms := makeTestModelService()
		empty := ""
		ms.Spec.SchedulerOverride = &empty
		if got := effectiveSchedulerName(ms); got != SchedulerNameDefault {
			t.Fatalf("effectiveSchedulerName(empty-string-override) = %q, want %q (empty == nil fallback)", got, SchedulerNameDefault)
		}
	})

	t.Run("buildDeployment Pod template stamps SchedulerName", func(t *testing.T) {
		ms := makeTestModelService()
		dep := buildDeployment(ms, PDSidePrefill)
		got := dep.Spec.Template.Spec.SchedulerName
		if got != SchedulerNameDefault {
			t.Fatalf("buildDeployment(default-override).Pod.SchedulerName = %q, want %q", got, SchedulerNameDefault)
		}
		// And with override set, buildDeployment should respect it.
		custom := "volcano-scheduler"
		ms.Spec.SchedulerOverride = &custom
		dep2 := buildDeployment(ms, PDSideDecode)
		if got := dep2.Spec.Template.Spec.SchedulerName; got != "volcano-scheduler" {
			t.Fatalf("buildDeployment(custom-override).Pod.SchedulerName = %q, want \"volcano-scheduler\"", got)
		}
	})
}
