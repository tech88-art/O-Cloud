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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	inferencev1alpha1 "github.com/tech88-art/O-Cloud/operators/inference-operator/api/v1alpha1"
)

// PDSide enumerates the two halves of a Prefill+Decode pair.
type PDSide string

const (
	PDSidePrefill PDSide = "prefill"
	PDSideDecode  PDSide = "decode"
)

// Default label key the controller stamps onto Deployment Pods so the
// PD Router webhook (T103) can route requests by role. Per ModelService
// spec.pdPair.routerLabel — empty value falls back to this constant.
const DefaultRouterLabelKey = "inference.ocloud.edge.example.com/pd-role"

// LabelModelService is the label key carrying the NAME (not full
// namespace/name) of the owning ModelService on every Pod the controller
// creates. K8s label-value regex rejects `/`, so we encode just the
// name — namespace is implicit from the Pod's own namespace. The PD
// Router admission webhook (T103) reconstructs the qualified ref
// `<pod.Namespace>/<label>` to match
// NPUSliceAllocation.spec.modelServiceRef (which keeps the `<ns>/<name>`
// form on the annotation). T124 fix · 2026-05-20.
const LabelModelService = "inference.ocloud.edge.example.com/model-service"

// pdReplicaSpec returns the PDReplicaSpec for the named side.
func pdReplicaSpec(ms *inferencev1alpha1.ModelService, side PDSide) inferencev1alpha1.PDReplicaSpec {
	switch side {
	case PDSidePrefill:
		return ms.Spec.PDPair.Prefill
	case PDSideDecode:
		return ms.Spec.PDPair.Decode
	}
	return inferencev1alpha1.PDReplicaSpec{}
}

// routerLabelKey returns the routerLabel key for the ModelService, with
// fallback to the documented default.
func routerLabelKey(ms *inferencev1alpha1.ModelService) string {
	if ms.Spec.PDPair.RouterLabel != "" {
		return ms.Spec.PDPair.RouterLabel
	}
	return DefaultRouterLabelKey
}

// deploymentName is the deterministic Deployment name for a (ModelService, PDSide).
func deploymentName(ms *inferencev1alpha1.ModelService, side PDSide) string {
	return ms.Name + "-" + string(side)
}

// claimTemplateName is the deterministic ResourceClaimTemplate name
// for a (ModelService, PDSide). The Pod template references this name
// via Pod.Spec.ResourceClaims[].ResourceClaimTemplateName; K8s creates
// one ResourceClaim per Pod replica automatically.
func claimTemplateName(ms *inferencev1alpha1.ModelService, side PDSide) string {
	return ms.Name + "-" + string(side) + "-claim"
}

// claimRefNameInPod is the per-Pod alias used to look up the claim
// inside the Pod template (Pod.Spec.ResourceClaims[].Name). Containers
// reference this name via container.Resources.Claims[].
const claimRefNameInPod = "npu-slice"

// SchedulerNameDefault is the kube-scheduler profile name our
// scheduler-plugin (operators/scheduler-plugin/) registers under per
// ADR-0010 §1. Phase 7 P7-T-003 auto-stamps this value on
// spec.schedulerName for every PD-pair Pod the controller materialises
// (closes known-issues #11). Operators opt out via
// ms.Spec.SchedulerOverride per ADR-0011 §1.
const SchedulerNameDefault = "npu-scheduler"

// effectiveSchedulerName returns the scheduler name to stamp on PD-pair
// Pod specs. Defaults to SchedulerNameDefault ("npu-scheduler") so HCCS-
// aware scheduling is automatic; operators opt out via
// ms.Spec.SchedulerOverride.
//
// Resolution table:
//
//	ms.Spec.SchedulerOverride         → effective schedulerName
//	-----------------------------------+--------------------------
//	nil (default)                     | "npu-scheduler"
//	*ptr → ""                         | "npu-scheduler"  (empty == nil)
//	*ptr → "default-scheduler"        | "default-scheduler"
//	*ptr → "<custom>"                 | "<custom>"
//
// Test coverage: TestEffectiveSchedulerName in deployment_builder_test.go
// exercises all 4 rows.
func effectiveSchedulerName(ms *inferencev1alpha1.ModelService) string {
	if ms.Spec.SchedulerOverride == nil || *ms.Spec.SchedulerOverride == "" {
		return SchedulerNameDefault
	}
	return *ms.Spec.SchedulerOverride
}

// buildDeployment returns the desired Deployment for one side of the
// PD pair. Pure function — caller is responsible for Create/Update.
//
// The pod template:
//   * carries the pd-role label (routerLabelKey -> string(side))
//   * carries the model-service label (ms.Name only — namespace is
//     implicit from pod.Namespace; see LabelModelService doc)
//   * declares one PodResourceClaim referencing the claim template
//     (which K8s expands per-replica at Pod admission)
//   * containers reference the claim via container.Resources.Claims
//
// OwnerRef points back at the ModelService so cascade-delete removes
// the Deployment (and indirectly the per-replica claims via K8s GC)
// when the ModelService is deleted.
func buildDeployment(ms *inferencev1alpha1.ModelService, side PDSide) *appsv1.Deployment {
	rep := pdReplicaSpec(ms, side)
	replicas := rep.Replicas
	roleKey := routerLabelKey(ms)
	roleVal := string(side)
	// LabelModelService VALUE = ms.Name only (no namespace). See
	// LabelModelService doc for why. T124 fix · 2026-05-20.
	msRefLabel := ms.Name

	selectorLabels := map[string]string{
		"app.kubernetes.io/name":      "modelservice",
		"app.kubernetes.io/instance":  ms.Name,
		"app.kubernetes.io/component": roleVal,
		roleKey:                       roleVal,
	}
	podLabels := map[string]string{
		"app.kubernetes.io/name":      "modelservice",
		"app.kubernetes.io/instance":  ms.Name,
		"app.kubernetes.io/component": roleVal,
		"app.kubernetes.io/part-of":   "ocloud-edge",
		roleKey:                       roleVal,
		LabelModelService:             msRefLabel,
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName(ms, side),
			Namespace: ms.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "modelservice",
				"app.kubernetes.io/instance":  ms.Name,
				"app.kubernetes.io/component": roleVal,
				LabelModelService:             msRefLabel,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptrInt32(replicas),
			Selector: &metav1.LabelSelector{MatchLabels: selectorLabels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: podLabels,
				},
				Spec: corev1.PodSpec{
					// Phase 7 P7-T-003 auto-stamp: closes known-issues #11
					// (scheduler-plugin opt-in). Operators opt out via
					// ms.Spec.SchedulerOverride per ADR-0011 §1.
					SchedulerName: effectiveSchedulerName(ms),
					ResourceClaims: []corev1.PodResourceClaim{{
						Name:                      claimRefNameInPod,
						ResourceClaimTemplateName: ptrString(claimTemplateName(ms, side)),
					}},
					Containers: buildPDPairContainers(ms, side, roleVal, claimRefNameInPod),
				},
			},
		},
	}
	return dep
}

func ptrInt32(v int32) *int32   { return &v }
func ptrString(v string) *string { return &v }

// buildPDPairContainers materialises the PD-pair Pod container list per
// P6-T-105:
//
//   - Container "vllm-ascend" runs ms.Spec.Model.Image (or
//     ms.Spec.PDPair.FallbackImage when set) with the standard
//     --model-path / --pd-role flags.
//   - If ms.Spec.PDPair.ProxyImage is non-empty, a second container
//     "pd-proxy" is appended that runs the vllm-ascend
//     disaggregated_prefill_v1 proxy_server with PREFILL_HOST /
//     DECODE_HOST env-vars resolving to the per-side Service names
//     (Phase 6 simulator scope uses convention; Phase 7 real-cluster
//     wires explicit Service refs).
//
// FallbackImage takes precedence over Model.Image on the main
// container ONLY. The proxy sidecar always uses ProxyImage when set
// (it doesn't have a fallback because it's opt-in by design — operators
// don't enable the proxy sidecar without the proxy_server image being
// available).
func buildPDPairContainers(ms *inferencev1alpha1.ModelService, side PDSide, roleVal, claimRef string) []corev1.Container {
	mainImage := ms.Spec.Model.Image
	if ms.Spec.PDPair.FallbackImage != "" {
		mainImage = ms.Spec.PDPair.FallbackImage
	}

	containers := []corev1.Container{{
		Name:  "vllm-ascend",
		Image: mainImage,
		Args: []string{
			"--model-path=" + ms.Spec.Model.ModelPath,
			"--pd-role=" + roleVal,
		},
		Resources: corev1.ResourceRequirements{
			Claims: []corev1.ResourceClaim{{
				Name: claimRef,
			}},
		},
	}}

	if ms.Spec.PDPair.ProxyImage != "" {
		// Per-side sibling Service name convention: <ms.Name>-<other-side>.
		// Phase 5 deployment_builder doesn't yet create per-side Services;
		// Phase 7+ work will. T105 ships the env-var pattern so a future
		// Service-creation task wires through.
		other := "decode"
		if string(side) == "decode" {
			other = "prefill"
		}
		containers = append(containers, corev1.Container{
			Name:  "pd-proxy",
			Image: ms.Spec.PDPair.ProxyImage,
			Env: []corev1.EnvVar{
				{Name: "VLLM_PD_ROLE", Value: roleVal},
				{Name: "VLLM_PD_PREFILL_HOST", Value: ms.Name + "-prefill"},
				{Name: "VLLM_PD_DECODE_HOST", Value: ms.Name + "-decode"},
				// The proxy_server connects to its local sibling
				// container over localhost (same Pod). Phase 7 may
				// add port discovery via downward-API env.
				{Name: "VLLM_PD_SIBLING_LOCALHOST_PORT", Value: "8000"},
				// Identify the sibling Pod side this sidecar's own
				// upstream is — proxy_server uses this to skip self.
				{Name: "VLLM_PD_OTHER_SIDE", Value: other},
			},
		})
	}

	return containers
}
