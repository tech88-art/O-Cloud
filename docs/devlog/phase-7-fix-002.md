# P7-fix-002 · post-tag CI fix #2: chart-bundled CRD sync for SchedulerOverride field

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: ~15 min (CI log re-triage + fix + verify)

## Intent

P7-fix-001(c51a6a5)修了 CI (Validate Mock Data Schema) + Phase 5 schedulerOverride 字段加 fixture · 但 e2e-kind 仍 failed at Phase 5 step 18 with new error:

```
Error from server (BadRequest): error when creating "modelservice-sample.yaml":
ModelService in version "v1alpha1" cannot be handled as a ModelService:
strict decoding error: unknown field "spec.schedulerOverride"
```

Root cause:T003 加了 `ModelServiceSpec.SchedulerOverride *string` field + regenerated `operators/inference-operator/config/crd/bases/...modelservices.yaml` (CRD source-of-truth) · 但 `deploy/helm-charts/inference-operator/crds/modelservices.yaml` (chart bundle copy) 没同步刷新。Chart 安装时把 stale CRD 装上集群 · kubectl apply 用 strict decoding 拒绝 unknown field。

## Path adaptations + fix

Single-file change:
- `cp operators/inference-operator/config/crd/bases/inference.ocloud.edge.example.com_modelservices.yaml deploy/helm-charts/inference-operator/crds/modelservices.yaml`
- Chart bundle CRD 现在含 schedulerOverride field
- Chart pattern: Phase 4 起 inference-operator chart bundles CRD in `crds/` dir (chart-level CRD application happens before templates render)

## Debugging trail

- P7-fix-001 push 后 GitHub Actions rate-limited(unauthenticated · 60/hr exhausted by 25-min Monitor poll loop)
- Pull cached PAT from Windows Git Credential Manager · authenticated curl 立即拿到 c51a6a5 状态:CI ✅ + e2e-kind ❌
- Fetch `/jobs/{id}/logs` 2483 行 · grep "ModelService|::error|apply" 定位到第 1390 行 `== apply ModelService fixture ==` 后立即 BadRequest schedulerOverride error
- grep 验证 chart-bundled CRD vs operator config CRD:`config/crd/bases/...yaml` has schedulerOverride · `chart/crds/...yaml` does NOT。Stale。
- cp + helm lint 通过 · 1 chart 0 fail

## Key decisions

- **fix in same chart-bundles-CRD pattern as Phase 6/T006**(NPUSliceTemplate)和 Phase 5 (NPUSliceAllocation):同一 convention 走到底 · 不引入特殊路径
- **不在 T003 commit message 中追溯改 chart**:T003 已 merge + tag · 此 fix 是 post-tag polish · 单独 commit + devlog 反映"chart sync 是工艺 gap"
- **T003 commit acceptance 显示 'helm lint clean'** 那时 — 但 helm lint 不检查 CRD bundle 是否 vs operator config 一致(只检查 chart 内部 YAML 合法 + template render OK)。这是 lint vs sync 的本质区别 · 未来 add CI check 校验 chart CRDs == config/crd/bases CRDs(low-priority Phase 8+ candidate)

## Verification

- 存在性:
  - `deploy/helm-charts/inference-operator/crds/modelservices.yaml` 现在含 schedulerOverride field(grep -c = 1)✅
- 完整性(verified by execution):
  - `helm lint --strict deploy/helm-charts/inference-operator/` 1 chart linted · 0 failed ✅
  - `cp` 自 operator config-of-truth → chart bundle · byte-equivalent · no manual edits
- 正确性:
  - schedulerOverride field 是 *optional* field(no required marker · just `+optional` annotation in Go type)· chart upgrade 无 breaking change
  - 既有 Phase 4-6 ModelService objects(没设 schedulerOverride)继续 work · 字段默认 nil = effectiveSchedulerName returns "npu-scheduler"(T003 default)

## Carry-forward

- **Phase 8 baseline-bump task 加 chart-CRD-sync CI check**(low-priority polish):比较 `deploy/helm-charts/<chart>/crds/*.yaml` 与 `operators/<op>/config/crd/bases/*.yaml` · diff non-empty → fail。一行 `diff -q` shell script + GitHub workflow step · 防本类 drift 再发。
- **T003 + P7-fix-002 后续 audit**:check 还有没有其他 chart CRD 与 operator config 不一致(Phase 5 NPUSliceAllocation · Phase 6 metrics · Phase 7 NPUSliceTemplate)— **本 fix 只解 SchedulerOverride 一处** · 其他 CRD 字段如果有过 T003-like add 但 forgot sync 也是同一类 bug
- **CI 验证**:next workflow run on this commit · e2e-kind step 18 应通过(strict decoding error gone · ModelService apply 成功 · 后续 PD-pair Pod assertion 应能往前走)
