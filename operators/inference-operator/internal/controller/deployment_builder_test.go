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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

// TestSliceTemplateLabelPropagation exercises P9-T-004 — the annotation
// `npu.huawei.com/slice-template` (written onto ModelService by
// NPUVerticalScaler controller per ADR-0012 §5 mutation model) MUST be
// propagated to Pod template labels with the same key so claim_controller
// (npu-dra-driver · P8-T-008 wiring) can look up the NPUSliceTemplate ref
// via the Pod label chain naturally. Removes phase8/install.sh
// `cmd_demo_bundle_path` direct annotate ResourceClaim workaround.
//
// 3 sub-cases per plan §3 T004 acceptance:
//   1. ms has annotation set → Pod label present with same value
//   2. ms has no annotation → Pod label absent (no empty key entry)
//   3. ms has explicit empty annotation → Pod label absent (treated as absent)
func TestSliceTemplateLabelPropagation(t *testing.T) {
	t.Run("ms annotation set → Pod label present with same value", func(t *testing.T) {
		ms := makeTestModelService()
		ms.Annotations = map[string]string{
			SliceTemplateAnnotation: "qwen-pd-idle",
		}
		dep := buildDeployment(ms, PDSidePrefill)
		got, ok := dep.Spec.Template.Labels[SliceTemplateAnnotation]
		if !ok {
			t.Fatalf("Pod template label %q missing; want %q", SliceTemplateAnnotation, "qwen-pd-idle")
		}
		if got != "qwen-pd-idle" {
			t.Fatalf("Pod template label %q = %q; want %q", SliceTemplateAnnotation, got, "qwen-pd-idle")
		}
	})

	t.Run("ms no annotations → Pod label absent", func(t *testing.T) {
		ms := makeTestModelService()
		// Annotations nil by default in makeTestModelService.
		dep := buildDeployment(ms, PDSidePrefill)
		if _, ok := dep.Spec.Template.Labels[SliceTemplateAnnotation]; ok {
			t.Fatalf("Pod template label %q unexpectedly present; want absent", SliceTemplateAnnotation)
		}
	})

	t.Run("ms annotation empty string → Pod label absent (empty treated as unset)", func(t *testing.T) {
		ms := makeTestModelService()
		ms.Annotations = map[string]string{
			SliceTemplateAnnotation: "",
		}
		dep := buildDeployment(ms, PDSidePrefill)
		if _, ok := dep.Spec.Template.Labels[SliceTemplateAnnotation]; ok {
			t.Fatalf("Pod template label %q unexpectedly present for empty annotation; want absent", SliceTemplateAnnotation)
		}
	})

	t.Run("propagation works for decode side too", func(t *testing.T) {
		ms := makeTestModelService()
		ms.Annotations = map[string]string{
			SliceTemplateAnnotation: "qwen-pd-busy",
		}
		dep := buildDeployment(ms, PDSideDecode)
		if got := dep.Spec.Template.Labels[SliceTemplateAnnotation]; got != "qwen-pd-busy" {
			t.Fatalf("decode-side Pod template label %q = %q; want %q", SliceTemplateAnnotation, got, "qwen-pd-busy")
		}
	})

	// P9-T-004 ALSO propagates the annotation onto ResourceClaimTemplate.Spec.ObjectMeta.Annotations
	// (via claim_builder.go) so K8s auto-copies the annotation onto every
	// materialised ResourceClaim. This closes the chain to claim_controller's
	// AnnotationSliceTemplate read (npu-dra-driver · P8-T-008 wiring).
	t.Run("ms annotation set → ResourceClaimTemplate claim spec carries annotation", func(t *testing.T) {
		ms := makeTestModelService()
		ms.Spec.NPUSlicePoolRef.Name = "qwen-pool"
		ms.Annotations = map[string]string{
			SliceTemplateAnnotation: "qwen-pd-idle",
		}
		tmpl := buildResourceClaimTemplate(ms, PDSidePrefill)
		got, ok := tmpl.Spec.ObjectMeta.Annotations[SliceTemplateAnnotation]
		if !ok {
			t.Fatalf("ResourceClaim spec annotation %q missing; want %q", SliceTemplateAnnotation, "qwen-pd-idle")
		}
		if got != "qwen-pd-idle" {
			t.Fatalf("ResourceClaim spec annotation %q = %q; want %q", SliceTemplateAnnotation, got, "qwen-pd-idle")
		}
		// Pre-existing annotations (model-service-ref + preferred-pool) MUST also
		// remain — P9-T-004 ADDS slice-template, does not replace the map.
		if _, ok := tmpl.Spec.ObjectMeta.Annotations[AnnotationModelServiceRef]; !ok {
			t.Fatalf("AnnotationModelServiceRef missing on claim template spec")
		}
		if _, ok := tmpl.Spec.ObjectMeta.Annotations[AnnotationPreferredPool]; !ok {
			t.Fatalf("AnnotationPreferredPool missing on claim template spec")
		}
	})

	t.Run("ms no annotation → ResourceClaimTemplate claim spec has no slice-template annotation", func(t *testing.T) {
		ms := makeTestModelService()
		ms.Spec.NPUSlicePoolRef.Name = "qwen-pool"
		tmpl := buildResourceClaimTemplate(ms, PDSidePrefill)
		if _, ok := tmpl.Spec.ObjectMeta.Annotations[SliceTemplateAnnotation]; ok {
			t.Fatalf("ResourceClaim spec annotation %q unexpectedly present; want absent", SliceTemplateAnnotation)
		}
	})
}

// findContainer returns the named container from a Deployment Pod
// template, or nil.
func findContainer(dep *appsv1.Deployment, name string) *corev1.Container {
	for i := range dep.Spec.Template.Spec.Containers {
		if dep.Spec.Template.Spec.Containers[i].Name == name {
			return &dep.Spec.Template.Spec.Containers[i]
		}
	}
	return nil
}

// envValue returns the value of the named env var in a container, plus
// whether it was found.
func envValue(c *corev1.Container, name string) (string, bool) {
	for _, e := range c.Env {
		if e.Name == name {
			return e.Value, true
		}
	}
	return "", false
}

// TestBuildDeployment_RealInferenceShape exercises P13-T-105 (ADR-0024
// §2 Decision F): the PD-pair Deployments carry a REAL vllm-ascend
// serving shape — real image, model-weights volume + mount, CANN env,
// and a huawei.com/Ascend910B device-plugin request+limit — for BOTH
// Prefill and Decode sides. This is the offline-layer functional
// assertion; the real-machine "Pod Ready on 910B + inference responds"
// stamp is lab-gated (plan §8 · devlog).
func TestBuildDeployment_RealInferenceShape(t *testing.T) {
	ms := makeTestModelService()
	// Real Qwen 8B image + weights path (sourced from spec — no
	// hardcoded registry in Go).
	ms.Spec.Model.Image = "quay.io/ascend/vllm-ascend:v0.11.0"
	ms.Spec.Model.ModelPath = "/models/qwen-8b"

	// Both sides must materialise (the controller calls buildDeployment
	// for PDSidePrefill + PDSideDecode → 2 Deployments).
	for _, tc := range []struct {
		side PDSide
		name string
	}{
		{PDSidePrefill, "llama-7b-prefill"},
		{PDSideDecode, "llama-7b-decode"},
	} {
		dep := buildDeployment(ms, tc.side)
		if dep.Name != tc.name {
			t.Fatalf("side %s: Deployment name = %q, want %q", tc.side, dep.Name, tc.name)
		}

		// (1) Real image on the vllm-ascend container.
		c := findContainer(dep, "vllm-ascend")
		if c == nil {
			t.Fatalf("side %s: vllm-ascend container missing", tc.side)
		}
		if c.Image != "quay.io/ascend/vllm-ascend:v0.11.0" {
			t.Fatalf("side %s: container image = %q, want the spec image", tc.side, c.Image)
		}

		// (2) Model-weights volume present on the Pod + mounted at
		// ModelPath inside the container.
		var volFound bool
		for _, v := range dep.Spec.Template.Spec.Volumes {
			if v.Name == modelWeightsVolumeName {
				volFound = true
				if v.HostPath == nil {
					t.Fatalf("side %s: model-weights volume is not a hostPath", tc.side)
				}
				if v.HostPath.Path != "/models/qwen-8b" {
					t.Fatalf("side %s: model hostPath = %q, want /models/qwen-8b (empty root)", tc.side, v.HostPath.Path)
				}
			}
		}
		if !volFound {
			t.Fatalf("side %s: model-weights Pod volume missing", tc.side)
		}
		var mountFound bool
		for _, m := range c.VolumeMounts {
			if m.Name == modelWeightsVolumeName {
				mountFound = true
				if m.MountPath != "/models/qwen-8b" {
					t.Fatalf("side %s: model mount path = %q, want /models/qwen-8b", tc.side, m.MountPath)
				}
			}
		}
		if !mountFound {
			t.Fatalf("side %s: model-weights volumeMount missing on vllm-ascend container", tc.side)
		}

		// (3) CANN env present — ASCEND_RT_VISIBLE_DEVICES is the
		// load-bearing var the昇腾 runtime reads. ASCEND_TOOLKIT_HOME
		// must default to a non-empty CANN toolkit path.
		if _, ok := envValue(c, "ASCEND_RT_VISIBLE_DEVICES"); !ok {
			t.Fatalf("side %s: ASCEND_RT_VISIBLE_DEVICES env missing", tc.side)
		}
		if v, ok := envValue(c, "ASCEND_TOOLKIT_HOME"); !ok || v == "" {
			t.Fatalf("side %s: ASCEND_TOOLKIT_HOME env missing/empty (got %q)", tc.side, v)
		}
		if v, _ := envValue(c, "VLLM_PD_ROLE"); v != string(tc.side) {
			t.Fatalf("side %s: VLLM_PD_ROLE = %q, want %q", tc.side, v, tc.side)
		}

		// (4) huawei.com/Ascend910B device-plugin request+limit present
		// (alongside the DRA slice claim).
		req, okReq := c.Resources.Requests[ascend910BResource]
		lim, okLim := c.Resources.Limits[ascend910BResource]
		if !okReq || !okLim {
			t.Fatalf("side %s: huawei.com/Ascend910B request/limit missing (req=%v lim=%v)", tc.side, okReq, okLim)
		}
		if req.Value() != 1 || lim.Value() != 1 {
			t.Fatalf("side %s: Ascend910B count req=%d lim=%d, want 1/1", tc.side, req.Value(), lim.Value())
		}
		// DRA slice claim must STILL be present (slice-granular HCCS
		// binding coexists with the whole-device gate).
		if len(c.Resources.Claims) != 1 || c.Resources.Claims[0].Name != claimRefNameInPod {
			t.Fatalf("side %s: DRA slice claim missing/altered: %+v", tc.side, c.Resources.Claims)
		}
	}
}

// TestModelWeightsVolume_HostPathRootPrefix proves the configurable
// MODEL_HOSTPATH_ROOT path: when ModelHostPathRoot is set (real
// profile), the node hostPath gets the root prefix while the in-
// container mount path stays ModelPath (so --model-path is stable
// across profiles — decoupling-seam: config VALUE differs, SHAPE
// invariant).
func TestModelWeightsVolume_HostPathRootPrefix(t *testing.T) {
	prev := ModelHostPathRoot
	ModelHostPathRoot = "/data"
	defer func() { ModelHostPathRoot = prev }()

	ms := makeTestModelService()
	ms.Spec.Model.ModelPath = "/models/qwen-8b"
	dep := buildDeployment(ms, PDSidePrefill)

	var vol *corev1.Volume
	for i := range dep.Spec.Template.Spec.Volumes {
		if dep.Spec.Template.Spec.Volumes[i].Name == modelWeightsVolumeName {
			vol = &dep.Spec.Template.Spec.Volumes[i]
		}
	}
	if vol == nil || vol.HostPath == nil {
		t.Fatalf("model-weights hostPath volume missing")
	}
	if vol.HostPath.Path != "/data/models/qwen-8b" {
		t.Fatalf("hostPath = %q, want /data/models/qwen-8b (root prefixed)", vol.HostPath.Path)
	}
	// Container mount path stays ModelPath regardless of host root.
	c := findContainer(dep, "vllm-ascend")
	for _, m := range c.VolumeMounts {
		if m.Name == modelWeightsVolumeName && m.MountPath != "/models/qwen-8b" {
			t.Fatalf("mount path = %q, want /models/qwen-8b (invariant)", m.MountPath)
		}
	}
}

// TestNPUDeviceCount_Configurable proves NPU_DEVICE_COUNT_PER_REPLICA
// flows into the Ascend910B request+limit (real profile may raise it for
// tensor-parallel prefill) without a code branch.
func TestNPUDeviceCount_Configurable(t *testing.T) {
	prev := NPUDeviceCountPerReplica
	NPUDeviceCountPerReplica = 4
	defer func() { NPUDeviceCountPerReplica = prev }()

	ms := makeTestModelService()
	dep := buildDeployment(ms, PDSideDecode)
	c := findContainer(dep, "vllm-ascend")
	if c == nil {
		t.Fatal("vllm-ascend container missing")
	}
	if got := c.Resources.Requests[ascend910BResource]; got.Value() != 4 {
		t.Fatalf("Ascend910B request = %d, want 4", got.Value())
	}
	if got := c.Resources.Limits[ascend910BResource]; got.Value() != 4 {
		t.Fatalf("Ascend910B limit = %d, want 4", got.Value())
	}
}
