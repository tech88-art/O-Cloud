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
	"os"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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

// SliceTemplateAnnotation is the annotation key NPUVerticalScaler controller
// (ADR-0012 §5 mutation model · Phase 8 P8-T-007) writes onto ModelService.
// Phase 9 P9-T-004 propagates this annotation to the matching Pod label so
// claim_controller (npu-dra-driver · P8-T-008 wiring) can look up the
// NPUSliceTemplate ref from the Pod label without depending on the upstream
// ModelService annotation chain. Removes the phase8/install.sh
// `cmd_demo_bundle_path` direct `kubectl annotate resourceclaim` workaround.
const SliceTemplateAnnotation = "npu.huawei.com/slice-template"

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

// ------------------------------------------------------------------
// P13-T-105 · 真 CANN + vllm-ascend PD 分离推理 (ADR-0024 §2 Decision F)
// ------------------------------------------------------------------
//
// The PD-pair Deployments carry a REAL vllm-ascend serving shape:
//   * a model-weights volume mounted at ms.Spec.Model.ModelPath so the
//     vllm-ascend process loads the Qwen/LLaMA weights from disk,
//   * CANN runtime env (ASCEND_RT_VISIBLE_DEVICES + ASCEND_VISIBLE_
//     DEVICES + the CANN toolkit path) so the昇腾 driver exposes the
//     allocated 910B device(s) to the container,
//   * a huawei.com/Ascend910 device-plugin resource request+limit so
//     the kubelet admits the Pod only onto a node advertising real NPU
//     capacity.
//
// **Decoupling-seam invariant (ADR-0024 §2 Decision G)**: this is the
// SAME container SHAPE for demo and real. The demo-vs-real difference is
// the VALUES — the image (ms.Spec.Model.Image / FallbackImage / chart
// defaults), the model-mount host root, the device count, the visible-
// device list — all sourced from config (ModelService spec + the
// package vars below, which read operator-level env). There is NO
// `if real {}` branch: a kind/mock cluster gets a benign hostPath
// (DirectoryOrCreate) + a 1-device request; a real鲲鹏+910B node gets
// the production root + real device count via the real profile env.

// ascend910Resource is the Ascend Device Plugin extended-resource key
// requested per PD-pair Pod. Matches `docs/build-and-production-
// validation.md` §4.4(1) + the kind smoke fake-capacity key
// (`tests/e2e/kind/install.sh` patches `huawei.com/Ascend910=8`
// allocatable). The DRA ResourceClaim (claim_builder.go) remains the
// slice-granular binding; this device-plugin request is the whole-
// device admission gate the real昇腾 stack expects alongside CANN env.
//
// NB: backend `pkg/datasource/k8s/deploy.go` uses the `Ascend910B`
// variant for its standalone deploy path — that drift is noted as
// carry-forward in the T105 devlog (out of this task's Allowed Paths).
const ascend910Resource = corev1.ResourceName("huawei.com/Ascend910")

// modelWeightsVolumeName is the Pod volume + volumeMount name carrying
// the model weights directory.
const modelWeightsVolumeName = "model-weights"

// cannToolkitPathDefault is the conventional CANN toolkit install path
// inside the official昇腾 base image (Atlas 800 / openEuler). Exposed
// via ASCEND_TOOLKIT_HOME so the vllm-ascend launcher sources
// set_env.sh. Overridable via the CANN_TOOLKIT_HOME operator env.
const cannToolkitPathDefault = "/usr/local/Ascend/ascend-toolkit/latest"

// Real-inference config knobs. Read once at first build from operator
// env so the values flow from chart values → container env →
// deployment_builder, mirroring the DefaultProxyImage pattern WITHOUT
// touching cmd/main.go (which is outside T105 Allowed Paths). Tests
// override these package vars directly (see deployment_builder_test.go).
var (
	// ModelHostPathRoot is prepended to ms.Spec.Model.ModelPath to form
	// the node hostPath backing the model-weights volume. Empty (demo
	// default) → the hostPath IS ModelPath verbatim (e.g.
	// /models/qwen-8b). The real profile sets MODEL_HOSTPATH_ROOT to the
	// node mount root where weights are staged (e.g. /data). The volume
	// always mounts AT ModelPath inside the container regardless of root
	// so the --model-path flag is stable across profiles.
	ModelHostPathRoot = os.Getenv("MODEL_HOSTPATH_ROOT")

	// NPUDeviceCountPerReplica is the huawei.com/Ascend910 request+limit
	// stamped per PD-pair Pod. Default 1. Real profile may raise it via
	// NPU_DEVICE_COUNT_PER_REPLICA for tensor-parallel prefill.
	NPUDeviceCountPerReplica = getenvInt("NPU_DEVICE_COUNT_PER_REPLICA", 1)

	// AscendVisibleDevices seeds ASCEND_RT_VISIBLE_DEVICES. Empty (demo
	// default) lets the Ascend Device Plugin inject the allocated device
	// list at admission; an explicit value (e.g. "0,1") pins devices for
	// a static real deployment. Sourced from ASCEND_RT_VISIBLE_DEVICES.
	AscendVisibleDevices = os.Getenv("ASCEND_RT_VISIBLE_DEVICES")

	// CANNToolkitHome is exposed as ASCEND_TOOLKIT_HOME. Defaults to the
	// conventional install path; overridable via CANN_TOOLKIT_HOME.
	CANNToolkitHome = getenvOr("CANN_TOOLKIT_HOME", cannToolkitPathDefault)
)

// getenvOr returns os.Getenv(key) or def when unset/empty.
func getenvOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// getenvInt parses os.Getenv(key) as an int, falling back to def on
// unset / parse error / non-positive value.
func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

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
	// P9-T-004: propagate slice-template annotation (written by
	// NPUVerticalScaler controller per ADR-0012 §5) to Pod label so
	// claim_controller (npu-dra-driver · P8-T-008 wiring) can look up
	// the NPUSliceTemplate ref via the Pod label chain naturally.
	// Removes the phase8/install.sh `cmd_demo_bundle_path` direct
	// `kubectl annotate resourceclaim` workaround.
	if v, ok := ms.Annotations[SliceTemplateAnnotation]; ok && v != "" {
		podLabels[SliceTemplateAnnotation] = v
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
					// P13-T-105: model-weights volume so the vllm-ascend
					// container loads real Qwen/LLaMA weights from disk.
					Volumes:    []corev1.Volume{modelWeightsVolume(ms)},
					Containers: buildPDPairContainers(ms, side, roleVal, claimRefNameInPod),
				},
			},
		},
	}
	return dep
}

func ptrInt32(v int32) *int32   { return &v }
func ptrString(v string) *string { return &v }

// DefaultProxyImage is overridden at startup from the chart-injected
// env var `DEFAULT_PROXY_IMAGE` (per Phase 10 P10-T-106 ProxyImage chart
// default flip · closes known-issues #13)。Empty preserves Phase 7-8
// behavior (operator must specify ms.Spec.PDPair.ProxyImage per-CR).
//
// Read at cmd/main.go startup:
//
//	if v := os.Getenv("DEFAULT_PROXY_IMAGE"); v != "" {
//	    deployment_builder.DefaultProxyImage = v
//	}
//
// Phase 10 W2 wires this from chart values.proxyImage. Phase 11+ may
// add hot-reload via ConfigMap watch if image rotation cadence demands.
var DefaultProxyImage string

// EffectiveProxyImage returns the operative proxy image for a given
// ModelService · `ms.Spec.PDPair.ProxyImage` wins · `DefaultProxyImage`
// fallback when per-CR field is empty · "" preserves Phase 7-8 no-proxy-
// sidecar behavior (buildPDPairContainers omits sidecar when "").
//
// Per ADR-0010 §1 Phase 10 P10-T-106 + known-issues #13 closer:
// chart `defaults.proxyImage` is the operator-side global default · per-CR
// override is operator-side authoritative when set.
func EffectiveProxyImage(ms *inferencev1alpha1.ModelService) string {
	if ms != nil && ms.Spec.PDPair.ProxyImage != "" {
		return ms.Spec.PDPair.ProxyImage
	}
	return DefaultProxyImage
}

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
		// P13-T-105: CANN runtime env so the昇腾 driver/toolkit expose the
		// allocated 910B device(s) to the vllm-ascend process.
		Env: cannEnv(roleVal),
		// P13-T-105: mount the model weights at ModelPath so --model-path
		// resolves to a real on-disk directory.
		VolumeMounts: []corev1.VolumeMount{{
			Name:      modelWeightsVolumeName,
			MountPath: ms.Spec.Model.ModelPath,
			ReadOnly:  true,
		}},
		// P13-T-105: keep the DRA slice claim (slice-granular binding) AND
		// add the device-plugin whole-device request+limit the real昇腾
		// stack admits against (ADR-0024 §2 Decision F). Both coexist:
		// the claim drives HCCS-aware slice placement; the resource gate
		// keeps the Pod off NPU-less nodes.
		Resources: npuResourceRequirements(claimRef),
	}}

	proxyImage := EffectiveProxyImage(ms)
	if proxyImage != "" {
		// Per-side sibling Service name convention: <ms.Name>-<other-side>.
		// Phase 5 deployment_builder doesn't yet create per-side Services;
		// Phase 7+ work will. T105 ships the env-var pattern so a future
		// Service-creation task wires through. P10-T-106 adds chart-level
		// default (via DefaultProxyImage package var) so operators can flip
		// proxy-sidecar default on without editing every ModelService CR.
		other := "decode"
		if string(side) == "decode" {
			other = "prefill"
		}
		containers = append(containers, corev1.Container{
			Name:  "pd-proxy",
			Image: proxyImage,
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

// modelWeightsVolume returns the Pod volume backing the model weights.
// hostPath with DirectoryOrCreate keeps it safe on demo/kind nodes
// (the directory is created if absent) while pointing at the staged
// weights root on a real node (ModelHostPathRoot + ModelPath).
//
// Decoupling-seam (ADR-0024 §2 Decision G): SAME volume shape both
// profiles; only the host path root differs by config (MODEL_HOSTPATH_
// ROOT env). Operators wanting a PVC instead override the chart's pod
// volume in the real profile — the container-side mount (at ModelPath)
// is invariant so --model-path never changes.
func modelWeightsVolume(ms *inferencev1alpha1.ModelService) corev1.Volume {
	hostDir := ModelHostPathRoot + ms.Spec.Model.ModelPath
	hostType := corev1.HostPathDirectoryOrCreate
	return corev1.Volume{
		Name: modelWeightsVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: hostDir,
				Type: &hostType,
			},
		},
	}
}

// cannEnv returns the CANN runtime env-vars for a vllm-ascend container.
// ASCEND_RT_VISIBLE_DEVICES is ALWAYS present (the env-var the昇腾
// runtime + vllm-ascend read to pick devices); its value defaults to the
// AscendVisibleDevices config (empty → Ascend Device Plugin injects the
// allocated list at admission). ASCEND_VISIBLE_DEVICES mirrors it for
// older driver builds. ASCEND_TOOLKIT_HOME points the launcher at the
// CANN toolkit. VLLM_PD_ROLE carries the prefill/decode side so the
// vllm-ascend disaggregated_prefill_v1 entrypoint knows its half.
func cannEnv(roleVal string) []corev1.EnvVar {
	return []corev1.EnvVar{
		{Name: "ASCEND_RT_VISIBLE_DEVICES", Value: AscendVisibleDevices},
		{Name: "ASCEND_VISIBLE_DEVICES", Value: AscendVisibleDevices},
		{Name: "ASCEND_TOOLKIT_HOME", Value: CANNToolkitHome},
		{Name: "VLLM_PD_ROLE", Value: roleVal},
	}
}

// npuResourceRequirements builds the container ResourceRequirements
// carrying BOTH the DRA slice claim (slice-granular HCCS binding) and
// the huawei.com/Ascend910 device-plugin request+limit (whole-device
// admission gate). NPUDeviceCountPerReplica controls the count.
func npuResourceRequirements(claimRef string) corev1.ResourceRequirements {
	qty := *resource.NewQuantity(int64(NPUDeviceCountPerReplica), resource.DecimalSI)
	return corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{ascend910Resource: qty},
		Requests: corev1.ResourceList{ascend910Resource: qty},
		Claims: []corev1.ResourceClaim{{
			Name: claimRef,
		}},
	}
}
