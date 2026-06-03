# P13-fix-002 · Ascend NPU 整卡资源 key 全仓库统一 → `huawei.com/Ascend910B`

- **Commit**: this commit (main agent · resolves the T105 devlog "Resource-key drift" carry-forward)
- **Date**: 2026-06-03

## Intent

T105 devlog flagged a carry-forward: backend `deploy.go` requested `huawei.com/Ascend910B`
while inference-operator + build-doc + kind requested `huawei.com/Ascend910` — the two
PD/deploy paths would gate against different resource names, and only one matched the kind
fake-capacity. T105 deferred reconciliation to T301 真机. This fix does it now.

## Investigation — the framing reversed (P3 verify-before-propose / P6 反证)

The task hypothesis (and most of the repo) leaned `huawei.com/Ascend910`. The authoritative
sources say the opposite for this project's **target 910B silicon**:

- `docs/research/ascend-device-plugin.md` §3/§4/§6 ([A] · HAMi README + 华为云文档): whole 910B
  card → **`huawei.com/Ascend910B`** (incl. `resourceName:` spec). `910` (no B) = first-gen 910.
- `deploy/helm-charts/ascend-npu-exporter-plus/values.yaml:34`: "device-plugin marks with
  `910B`. Override to `910` **on first-gen 910 silicon**." → key is silicon-generation-specific.
- ADR-0024 §4(b) + `deploy/profiles/real/README.md`: real node advertises `910B` 容量标签.
- root CLAUDE.md §2: 目标硬件 = 昇腾 **910B**.

So **backend `deploy.go` was already correct**; the pervasive `huawei.com/Ascend910` was a latent
bug, masked only because everything synthetic (kind/mock) self-consistently used `910` and 真机
was lab-gated 6 phases. ADR-0024 was also internally contradictory (§2 Decision F said `910`,
§4(b) said `910B`).

## Decision (用户: 全仓库统一 910B)

Scope is larger than the 2-file framing: `910` was used as the whole-card resource/capacity/request
key across pool-operator (npupool/npuslicepool), o2-dms-adapter, inference-operator, backend
`cluster.go`, ALL kind fixtures, build-doc, demo/known-issues, phase2/3-plan. Unified every LIVE
reference onto `huawei.com/Ascend910B` (cross-module: backend + operators + deploy + docs).

- **Health label migrated too**: `huawei.com/Ascend910-Health` → `…Ascend910B-Health` (same chip
  suffix the device plugin generates · kind-config already paired the `910B` presence label) —
  pool-operator npupool + kind-config + demo/phase3 doc.
- inference-operator const renamed `ascend910Resource` → `ascend910BResource` (matches backend).
- kind: `install.sh` now patches `huawei.com/Ascend910B=8` capacity+allocatable (JSON-pointer
  `~1` form too); `mock-workload.yaml` requests `910B`; `kind-config` health label → `910B-Health`.
- ADR-0024 §2 Decision F prose reconciled to `910B` (now consistent with its own §4(b)).

## Deliberately LEFT (not stragglers)

- `backend/pkg/datasource/k8s/npu.go` — reads by `huawei.com/Ascend` **prefix** (handles 910/910B);
  `modelDefaults["Ascend910"]` + `npu_test.go` first-gen/mixed-model cases kept (test robustness).
- exporter chart `values.yaml:35` override comment (documents the first-gen `910` escape hatch).
- Frozen append-only trails: `devlog/phase-{5-t126,13-t105}.md`, `research/volcano-gang-…spike.md`,
  `phase9-plan.md:610` (Volcano shorthand · fixture never built · consistent with the frozen spike).

## Verification (offline — functional)

- `go build ./... && go vet ./...` green for backend / inference-operator / pool-operator / o2-dms.
- `go test ./pkg/datasource/k8s/...` (backend, fake clientset) → PASS.
- `go test ./internal/controller/...` (inference-operator) → PASS (incl. RealInferenceShape +
  NPUDeviceCount_Configurable asserting the `910B` request/limit).
- pool-operator envtest: 17 pass / 6 fail — the 6 are a **pre-existing** env artifact (cached
  envtest = k8s 1.35.0, which does not serve `resource.k8s.io/v1beta1` ResourceSlice by default;
  CI uses the pinned version + `runtime-config`). Confirmed innocent by `git stash`-ing the 4
  pool-operator files and re-running: original code yields the **identical** 17/6 split. The 17
  passing include the capacity/health-counting tests this change actually affects.
- Whole-repo sweeps: `Ascend910BB`=0 (no `910B`→`910BB` corruption) · no non-`B` resource keys or
  `…910-Health` labels remain outside the deliberate-leave set · no `~1Ascend910` (non-B) left.

## Real-machine confirmation — LAB-GATED (per ADR-0024 §3 single-point fallback)

The resource-key **and** the `-Health` suffix both follow the Ascend device plugin's chip-type
detection; T301 真机 confirms what the lab plugin advertises. If a lab stack advertises bare
`910`/`910-Health` (first-gen) → flip via the exporter-chart override pattern + real-profile values
and mark that one verify point lab-driver-gated. Do NOT reopen a phase.
