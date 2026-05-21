# P7-T-003 · inference-operator deployment_builder schedulerName=npu-scheduler auto-stamp

- **Commit**: (this commit)
- **Date**: 2026-05-20
- **Duration**: plan 0.5d / actual ~0.5d (main agent direct per §0a.11 exception · small typed-field + builder hook + 4 sub-tests + CRD regen + DESIGN + known-issues update)

## Intent

Phase 6 T101 ships scheduler-plugin as a SECOND scheduler — Pods opt in via `spec.schedulerName=npu-scheduler`. Without the auto-stamp, ModelService Pods schedule via default-scheduler and bypass HCCSTopology Filter+Score / Binpack. Phase 6 known-issues #11 documented the gap; Phase 7 T003 closes it by wiring `inference-operator deployment_builder.buildDeployment` to stamp `SchedulerName` on every PD-pair Pod template, with `ModelServiceSpec.SchedulerOverride *string` opt-out.

## Path adaptations

- Plan §3-T003 referenced `buildPodSpec` function for the stamp site — actual code has the PodSpec inline inside `buildDeployment` (no separate buildPodSpec function in Phase 5+ deployment_builder.go). Stamped inline at `corev1.PodSpec.SchedulerName` per the actual structure.
- Plan listed `config/samples/modelservice_sample.yaml` for edit, but `config/samples/` directory didn't exist (Phase 4 scaffold P4-T-103 didn't create samples · only the api/types + CRD bases YAML). Created the directory + sample inline.
- Plan acceptance "`kubectl apply --dry-run=server` passes" required a real cluster context — local Docker Desktop kubectl is set up but has no cluster running (`couldn't get current server API group list: the server could not find the requested resource`). Substituted YAML lint via `python yaml.safe_load` (passed). kind smoke (T103) verifies server-side dry-run.

## Debugging trail

- **controller-gen path arg quirk on Windows (Go 1.25.7 + controller-gen v0.21.0)**: `paths=./...` and `paths=./api/...` both failed with confusing error `-: no Go files in <module-root>`. Workaround: use explicit subdir path `paths=./api/v1alpha1/`. Same trick works for `controller-gen object:headerFile=...`. Documented in this devlog because future agents WILL hit the same friction.
- **Empty-string override** edge case discovered while writing the test cases — if a chart template renders `schedulerOverride: ""` (accidental empty default), the field is non-nil but points to empty string. Per the safety-default impl, empty-string still falls back to `npu-scheduler`. Test `empty-string override pointer → default npu-scheduler` covers this.

## Key decisions

- **`effectiveSchedulerName(ms)` as a separate helper** (not inline in `buildDeployment`) — keeps the resolution logic testable without spinning up a full ModelService → Deployment build. Tests call `effectiveSchedulerName(ms)` directly for 3 of 4 cases; the 4th invokes `buildDeployment` for the round-trip integration.
- **`SchedulerOverride *string` (pointer)** rather than `string` (value) — distinguishes "absent" (nil) from "explicit empty string" (empty pointer). Both resolve to default per the resolution table, but the pointer-vs-value distinction matters for kubebuilder OpenAPI generation (pointer → `omitempty` correctly hides the field from kubectl get -o json when unset).
- **Constant `SchedulerNameDefault = "npu-scheduler"`** mirrors the chart `profileName` value (deploy/helm-charts/scheduler-plugin/values.yaml). Per the const godoc, "changing one without the other = silent HCCS bypass" — followup auditor for Phase 8 chart bumps must verify both move together. The chart already documents this gotcha in known-issues #11 (now RESOLVED).
- **known-issues #11 RESOLVED but historical narrative retained** — per CLAUDE.md P3 honesty: the original narrative is STILL ACCURATE for Pods created outside the inference-operator path (raw `kubectl apply` of a Deployment, raw Pods). Documenting this nuance in the RESOLVED line rather than wiping the entry.
- **DESIGN.md new §5.2 "Scheduler routing"** placed between §5.1 Metrics and §6 扩展点 — keeps the operative Phase 6 + Phase 7 features in §5 as one block (Metrics + Scheduler routing both ship "live behavior added to the controller body").

## Verification

- 存在性:
  - `api/v1alpha1/modelservice_types.go` — `SchedulerOverride *string` field present at line 154 ✅
  - `internal/controller/deployment_builder.go` — `SchedulerNameDefault` constant + `effectiveSchedulerName` function + PodSpec stamp present ✅
  - `internal/controller/deployment_builder_test.go` — `TestEffectiveSchedulerName` with 4 sub-tests present ✅
  - `config/crd/bases/inference.ocloud.edge.example.com_modelservices.yaml` — `schedulerOverride` field at line 167 (regenerated via `controller-gen crd paths=./api/v1alpha1/`) ✅
  - `config/samples/modelservice_sample.yaml` — new file with default behavior + commented opt-out instructions ✅
  - `DESIGN.md` §5.2 Scheduler routing — added with resolution table + impl + test coverage + Phase 7 T103 forward note ✅
  - `docs/known-issues.md` #11 → RESOLVED with cross-ref ✅
- 完整性(verified by execution):
  - `go build ./...` clean ✅
  - `go test ./...` clean — 3 packages PASS (controller / metrics / webhook); 4 new TestEffectiveSchedulerName sub-tests verified via `go test ./internal/controller/... -run TestEffective -v` showing all PASS ✅
  - `controller-gen crd paths=./api/v1alpha1/ output:crd:artifacts:config=config/crd/bases` regen — `grep -nE "schedulerOverride" config/crd/bases/*.yaml` confirms field present at line 167 ✅
  - `controller-gen object:headerFile=hack/boilerplate.go.txt paths=./api/v1alpha1/` regen — no manual edit to zz_generated.deepcopy.go needed (*string handled by primitive deepcopy) ✅
  - `python -c "import yaml; yaml.safe_load(open('config/samples/modelservice_sample.yaml','r',encoding='utf-8'))"` clean parse ✅
- 正确性(grep cross-ref):
  - `grep -nE "SchedulerOverride|schedulerOverride|SchedulerNameDefault|effectiveSchedulerName"` across 6 touchpoint files returns 22+ hits — all consistent with the impl design ✅
  - `grep -nE "P7-T-003|known-issues #11"` in known-issues.md returns 3 hits — #11 RESOLVED line + #12 cross-ref ✅
- 注释:`kubectl apply --dry-run=server` NOT executed locally (no cluster context available in dev env); deferred to Phase 7 T103 kind smoke for server-side validation per CLAUDE.md P3 local-env honesty

## Carry-forward

- **T103** (kind smoke E2E Phase 7 extension) — the new fixture must produce Pods carrying `spec.schedulerName=npu-scheduler`; assert_scheduler_name check upgrades from Phase 6 T106 warning to T103 hard FAIL. T103 subagent brief must reference this T003 commit for the contract.
- **Phase 6 T106 kind smoke** — currently emits a warning IF Pod's schedulerName != npu-scheduler. With T003 landed, fixture-generated Pods should now satisfy it. NO immediate change needed to T106 — that workflow step is forward-compat (assert_scheduler_name is a "if absent warn" check). T103 will introduce a strict version.
- **scheduler-plugin chart `profileName` parity** — `SchedulerNameDefault = "npu-scheduler"` is hard-coded; chart `deploy/helm-charts/scheduler-plugin/values.yaml::profileName` MUST stay `"npu-scheduler"` (verified Phase 6 T101). Phase 8+ chart bumps that touch profileName must coordinate with this constant.
- **Phase 8 vertical scaling / Volcano experiments** — operators wanting to route ModelService to Volcano during training-job experiments set `ms.Spec.SchedulerOverride = "volcano-scheduler"`. Phase 8 plan should document this opt-out path in the busy-idle controller design.
