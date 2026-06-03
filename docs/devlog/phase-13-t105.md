# P13-T-105 · 真 CANN + vllm-ascend PD 分离推理(inference-operator)

- **Commit**: (pending — main agent verifies + commits)
- **Date**: 2026-06-03
- **Duration**: plan 3d vs actual ~0.4d (offline layer only · real-machine stamp lab-gated)

## Intent
Make inference-operator emit REAL vllm-ascend PD-disaggregated Deployments instead of a
placeholder shape: real image (already spec-sourced), a model-weights volume mounted at
`spec.model.modelPath`, CANN runtime env (`ASCEND_RT_VISIBLE_DEVICES` + toolkit), and a
`huawei.com/Ascend910` device-plugin request+limit on the Prefill/Decode containers. The PD
Router webhook additionally injects the real prefill→decode KV-cache endpoint alongside the
(unchanged) ADR-0008 slice-bindings annotation. Closes build-doc §4.4(4). ADR-0024 §2 Decision F.

## Path adaptations
- **Image was NOT a mock/placeholder already**: `deployment_builder.go` already sourced the main
  container image from `ms.Spec.Model.Image` (+ `FallbackImage` for CI, `DefaultProxyImage` for the
  proxy sidecar). The actual gaps were: no model mount, no CANN env, no NPU device-plugin resource.
  So the "replace mock image" task reduced to "add the real serving shape around the
  already-config-sourced image". No hardcoded registry existed or was introduced.
- **`cmd/main.go` + base helm chart are OUTSIDE T105 Allowed Paths.** Could not add a new env wire
  through main.go (the `DefaultProxyImage` pattern). Resolution: the real-inference config knobs
  (`MODEL_HOSTPATH_ROOT` / `NPU_DEVICE_COUNT_PER_REPLICA` / `CANN_TOOLKIT_HOME` /
  `ASCEND_RT_VISIBLE_DEVICES`) are package vars in `deployment_builder.go` that read the operator's
  env directly at package init. They reach the operator Pod via the base chart's EXISTING `extraEnv`
  knob, set in `deploy/profiles/real/inference-operator.values.yaml` — no base-chart edit.
- **Resource key = `huawei.com/Ascend910`** (matches ADR-0024 §2 Decision F text + build-doc §4.4(1)
  + the kind smoke fake-capacity allocatable key). Backend `pkg/datasource/k8s/deploy.go` (sibling
  T104) uses the `Ascend910B` variant — noted as carry-forward (cross-module, out of scope here).

## Debugging trail
- `Edit` of the `buildPDPairContainers` tail failed once on exact whitespace (tabs); re-matched on the
  real tab-indented closing block. No logic issue.
- Test helper first referenced a non-existent `appsv1Deployment` type → added `appsv1` import + used
  `*appsv1.Deployment`. Caught at compile, fixed before first test run.
- Confirmed the existing webhook happy-path tests still pass: they use a Pod with the model-service
  label but NO pd-role label, so `pdEndpointFor` returns "" → slice-bindings annotation injected as
  before, pd-endpoint skipped. The ADR-0008 contract is untouched (guarded by
  `TestAnnotationSliceBindings_FormatStable` + new `TestAnnotationPDEndpoint_KeyStable`).

## Key decisions
- **Decoupling-seam (ADR-0024 §2 Decision G) held strictly**: NO `if real {}` anywhere. The PD-pair
  Pod SHAPE is identical demo vs real; only VALUES differ via config (image from spec; host root /
  device count / visible-devices / toolkit from operator env set by the real profile). Demo/kind gets
  a benign `hostPath` (`DirectoryOrCreate`) + 1-device request; real gets the staged root + real
  count. Verified: `grep -n "if real" internal/` → 0 hits; demo unit tests green.
- **Model volume = hostPath `DirectoryOrCreate`** so demo/kind never fails on a missing `/models/...`
  dir, while real points at the staged weights root. Container mount path is invariant (= ModelPath)
  so `--model-path` is stable across profiles. Operators wanting a PVC override the pod volume in the
  real profile (carry-forward).
- **PD endpoint is ADDITIVE** to the webhook patch: new annotation
  `inference.ocloud.edge.example.com/pd-endpoint` = `<ms>-<otherSide>:8000` (sibling Service
  convention, matching the existing `VLLM_PD_PREFILL_HOST`/`DECODE_HOST` sidecar env). Injected only
  on the all-allocated happy path so the real endpoint travels with the real slice bindings.

## Verification (offline layer — functional)
From `operators/inference-operator/`:
- `go build ./...` → OK
- `go vet ./...` → OK
- `go test ./...` → PASS (controller + webhook + metrics + api; new T105 tests ran:
  `TestBuildDeployment_RealInferenceShape`, `TestModelWeightsVolume_HostPathRootPrefix`,
  `TestNPUDeviceCount_Configurable`, `TestHandle_PDEndpoint_InjectedForPDRolePod`,
  `TestPDEndpointFor`, `TestAnnotationPDEndpoint_KeyStable`)
- `CGO_ENABLED=0 GOARCH=arm64 GOOS=linux go build ./...` → OK (no cgo introduced; ADR-0020)
- `helm template ... -f deploy/profiles/real/inference-operator.values.yaml` → renders
  `DEFAULT_PROXY_IMAGE` + all 4 CANN config env vars into the operator container (plumbing proven via
  the existing `extraEnv` knob, no base-chart edit)
- P3 三维: 存在性 (env render + container fields present) · 完整性 (both Prefill+Decode sides asserted ·
  slice-bindings + pd-endpoint both patched) · 正确性 (host-root prefix + device count + sibling-side
  endpoint table all asserted, not just presence)

## Real-machine stamp — LAB-GATED (plan §8 真机层 · NOT verified here)
P3 honesty: real-machine verification is NOT done. To stamp on the lab arm64 + 910B cluster:
`kubectl apply -f config/samples/modelservice_qwen8b_pd.yaml` → Prefill/Decode Pods Ready on real
910B → inference request returns (build-doc §4.4(4) → 🟢 by T301). If the lab CANN/driver rejects the
`huawei.com/Ascend910` key (some stacks advertise `Ascend910B`) or the model hostPath staging differs
→ single-point fallback per ADR-0024 §3: adjust the real-profile env values, mark that one verify
point lab-driver-gated, do NOT reopen a phase.

## Carry-forward
- **Soft-dependency on T101** (real NPU allocation): the DRA ResourceClaim slices + the device-plugin
  `huawei.com/Ascend910` admission both need the real-Ascend publisher (T101) to advertise real
  capacity/attrs on the lab nodes. T105 is shape-complete against the expected schema; real Pods
  schedule once T101's publisher is live.
- **PVC alternative for model weights**: T105 ships hostPath (simplest staging). A
  ReadOnlyMany/CSI PVC is the production-grade alternative — would be a real-profile pod-volume
  override (base chart has no model-volume knob today → small base-chart follow-up if desired).
- **Resource-key drift**: backend `deploy.go` uses `huawei.com/Ascend910B`; inference-operator +
  build-doc + kind use `huawei.com/Ascend910`. T301 真机 should confirm which key the lab device
  plugin actually advertises and reconcile if they diverge.
- **Base-chart knob (optional)**: the CANN config rides the generic `extraEnv` knob. A future cleanup
  could promote first-class `realInference.{modelHostPathRoot,npuDeviceCount,...}` chart values — not
  required for T105 (extraEnv is sufficient + Allowed-Paths-clean).
