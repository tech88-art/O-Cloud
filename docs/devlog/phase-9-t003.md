# P9-T-003 · K8s baseline bump re-evaluate (DECISION-GATED · bump 1.34 attempted · doc-only refresh outcome)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 0.3d planned (doc-only fallback) / 1-2d planned (bump path) · ~0.8d actual (bump attempted + reverted + doc-only refresh)

## Intent

Phase 9 W1 entry decision-gated K8s baseline bump re-evaluate per P8-T-002 carry-forward + plan §3 T003 acceptance。Re-WebFetch upstream(kindest/node + scheduler-plugins)→ plan default-policy 触发条件 met → user chose A · Bump 1.34 → 执行发现 K8s 1.34 scheduler framework API restructuring 超出 T003 Forbidden Paths scope → revert + 应用 doc-only refresh fallback。

## Path adaptations

- **Plan §3 T003 default-policy interpretation**:Plan 写 "Default decision: **Doc-only refresh** unless re-WebFetch shows BOTH (kindest/node ≥1.34 released stable AND sched-plugins ≥v0.34.x GA released)"。re-WebFetch shows both conditions met → "unless" branch activates → bump 1.34 plan-recommended path。用户选 A · 准入此路径。
- **Bump 1.34 实际 scope vs T003 Forbidden Paths conflict**:Plan §3 Forbidden Paths 明示 "Source code outside `go.mod` / `go.sum` (any source change must be SEPARATE post-bump task — T102 NumaAffinity wrap is the explicit post-bump task in W2)"。但 K8s 1.34 framework restructuring 影响 9 个 plugin 文件 · 远超 T102 NumaAffinity wrap scope(T102 仅 `noderesourcetopology.New(ctx, args, h)` wrap + 3 sanity tests)· 是更广的 framework migration · 任何 inline source 改动都会违反 plan boundary · 故 revert + doc-only refresh fallback per plan §3 default(P3 conservative)。
- **Plan author 未预料**:Plan §3 写 T003 acceptance "go build clean" + Forbidden Paths "no source changes" 是 mutually exclusive 当 framework API drift surfaces · plan author 假定 minor bump 不会触发 framework restructuring · 现实 K8s 1.34 IS the major framework migration cohort。Phase 10 plan 需要把 framework migration 作为独立 task layer。

## Debugging trail

- **Step 1 · WebFetch upstream condition table**:
  - `kindest/node`:kind v0.31.0 → v1.35.0(2025-12-18 latest)· kind v0.30.0 → v1.34.0(2025-08-27)· v1.36 镜像不存在
  - `scheduler-plugins`:v0.34.7 latest GA(2026-04-20)· v0.35.x / v0.36.x 未发布
  - 结论:BOTH (kindest/node ≥1.34 released stable · sched-plugins ≥v0.34.x GA released) met → bump 1.34 plan-default path activated
- **Step 2 · 用户 chat 决策**:User chose "A · Bump 1.34"(plan-recommended)· "CI may surface dep API drift — handle via fix-001 series if needed"。
- **Step 3 · 编辑 + go mod tidy**:
  - `tests/e2e/kind/kind-config.yaml`:kindest/node:v1.32.0 → v1.34.0(3 places)
  - `operators/scheduler-plugin/go.mod`:k8s.io/* v0.32.0 → v0.34.7 · k8s.io/kubernetes v1.32.0 → v1.34.7 · k8s.io/* indirect v0.31.8 → v0.34.7 · replace block 26 entries v0.32.0 → v0.34.7 · 注释更新 P9-T-003 + v0.34.7 + K8s 1.34
  - `.github/workflows/e2e-kind.yml`:kind version v0.25.0 → v0.30.0 + 注释更新
  - `go mod tidy` clean exit 0(下载 K8s v0.34.7 cohort + transitive deps)
- **Step 4 · go build FAIL**:
  ```
  internal/plugins/hccs/filter.go:51:14: undefined: framework.Status
  internal/plugins/hccs/score.go:40:30: undefined: framework.StateKey
  internal/plugins/hccs/score.go:61:39: undefined: framework.StateData
  internal/plugins/binpack/binpack.go:107:38: cannot use nodeInfo (variable of interface type
    "k8s.io/kube-scheduler/framework".NodeInfo) as *"k8s.io/kubernetes/pkg/scheduler/framework".NodeInfo value
  ```
  - **诊断**:K8s 1.34 scheduler framework restructured · 关键 types (`Status` / `NewStatus` / `Error` / `StateKey` / `StateData` / `NodeInfo` / `CycleState`) 从 `k8s.io/kubernetes/pkg/scheduler/framework` 迁移到 `k8s.io/kube-scheduler/framework` · 且 `NodeInfo` + `CycleState` 从 struct-pointer 改为 interface
  - **影响范围**(grep `k8s.io/kubernetes/pkg/scheduler/framework` 全 scheduler-plugin 仓库):9 个文件
    - `operators/scheduler-plugin/internal/plugins/hccs/{filter.go, score.go, plugin.go, filter_test.go, score_test.go}`
    - `operators/scheduler-plugin/internal/plugins/binpack/{binpack.go, binpack_test.go}`
    - `operators/scheduler-plugin/internal/plugins/numa/plugin.go`
    - `operators/scheduler-plugin/internal/integration/integration_test.go`
- **Step 5 · 探索 minimum import-only fix**:试 `framework "k8s.io/kube-scheduler/framework"` import alias replace 在 filter.go 验证 — 仍多 errors:`Filter()` 签名要求 `*framework.NodeInfo`(old struct pointer)vs handle 期待 `framework.NodeInfo`(new interface)· 不是 import-only fix · 需 signature changes + interface method usage migration
- **Step 6 · 路径决策**:T003 Forbidden Paths "no source changes" + framework migration ≠ T102 NumaAffinity wrap scope → revert + doc-only refresh fallback per plan §3 default + P3 conservative
- **Step 7 · revert outcome**:
  - `git checkout origin/dev -- operators/scheduler-plugin/go.mod operators/scheduler-plugin/go.sum` (cleanest full restore)
  - Revert kind-config.yaml(replace_all v1.34.0 → v1.32.0)+ e2e-kind.yml comment revert + hccs/filter.go import revert
  - `go build ./...` clean exit 0(scheduler-plugin v0.32.0 baseline 恢复)+ `go vet ./...` clean exit 0
- **Step 8 · 应用 doc-only refresh**:ADR-0010 §1 加 2026-05-21 P9-T-003 update segment(re-WebFetch outcome + 执行复盘 + Phase 9 影响 + Phase 10 carry 路径)· known-issues #12 status header refresh + 2026-05-21 P9-T-003 update segment(prerequisite expanded 注解)· known-issues #13 status header refresh + 短 2026-05-21 P9-T-003 inherited 注解

## Key decisions

- **Revert > inline source migration**:严格按 plan §3 T003 Forbidden Paths · P3 conservative · framework migration 应 Phase 10 coordinated task chain · 不 Phase 9 W1 sneak。
- **T102 auto-deferred to Phase 10**:T102 NumaAffinity wrap upgrade gated on T003 outcome · T003 doc-only refresh → T102 auto-deferred · known-issues #12 maintains OPEN with expanded prerequisite。
- **Phase 10 prerequisite expanded**:从"baseline bump"(原 Phase 8/9 描述)扩展为"**baseline bump + scheduler framework API migration + NumaAffinity wrap upgrade**"三件套 coordinated task chain · estimate budget 2-3d (vs 原 1-2d)。
- **主模块 go.mod 状态不动**:npu-dra-driver / inference-operator / pool-operator 已在 v0.35.0(Phase 7 期间因日常 `go mod tidy` 微飘升 · 不主动 downgrade 也不主动 upgrade · per ADR-0010 §1 2026-05-21 P8-T-002 update segment policy 延续)。backend 在 v0.31.4(KubeEdge compat lock per ADR-0001 v3 §5)。

## K8s 1.34 scheduler framework migration matrix(Phase 10 蓝图)

下表是 Phase 10 全 bump 1.34 + framework migration 时需 touch 的 file scope · 每行 estimate effort:

| File | Lines affected | Migration type | Effort |
|---|---|---|---|
| `hccs/filter.go` | 11 occurrences | Import path + `*framework.NodeInfo` → `framework.NodeInfo` + `*framework.CycleState` → `framework.CycleState` (interface) | 30 min |
| `hccs/score.go` | 18 occurrences | Same as filter.go + `StateKey`/`StateData` import path | 45 min |
| `hccs/plugin.go` | 5 occurrences | factory function signature update | 20 min |
| `hccs/filter_test.go` | ~10 occurrences | Mock NodeInfo as interface | 30 min |
| `hccs/score_test.go` | ~15 occurrences | Same + CycleState interface mock | 45 min |
| `binpack/binpack.go` | 10 occurrences | Same as filter.go pattern | 30 min |
| `binpack/binpack_test.go` | ~8 occurrences | Mock pattern updates | 30 min |
| `numa/plugin.go` | 3 occurrences | Import path · wrap signature(post-T102 wrap) | 20 min |
| `internal/integration/integration_test.go` | ~10 occurrences | Integration test framework setup | 45 min |
| **Total** | 9 files | | **~5 hours** |

Plus T102 NumaAffinity wrap upstream body(原估 0.5d · 3 sanity tests)+ T003 bump path(go.mod + kind-config + helm chart + CI workflow + 0.3d ADR updates)= 2-3d coordinated task chain total。

## Verification

P3 三项验证维度:

| 维度 | 验证方法 | 结果 |
|---|---|---|
| **存在性** | `git diff --stat HEAD` + grep updated `2026-05-21 update (P9-T-003)` markers · git status 工作树 clean except Allowed Paths | 3 文件改:ADR-0010 §1 + known-issues #12 + #13 + 新 devlog · go.mod/go.sum/source files 全部 untouched(`git checkout origin/dev --` restored) |
| **完整性** | Plan §3 P9-T-003 Acceptance(doc-only refresh path) 4 项逐项核对(ADR-0010 §1 update segment landed · known-issues #12+#13 status header refresh · devlog records · CI no-op)+ Allowed Paths(doc-only) 3 文件全覆盖 | 4/4 acceptance items · Allowed Paths(doc-only)3/3 · zero code change · CI no-op |
| **正确性** | `go build ./operators/scheduler-plugin/...` clean exit 0 · `go vet ./operators/scheduler-plugin/...` clean exit 0(post-revert)· markdown 引用闭合(ADR-0010 §1 ↔ known-issues #12 ↔ devlog cross-ref + framework migration matrix self-contained) | 编译 clean · bi-directional cross-ref · matrix 9 files inventory 与 grep findings 1:1 对齐 |

## Carry-forward

- **T102 NumaAffinity wrap upgrade** Phase 10 plan W1 起手(原 Phase 9 W2 P9-T-102 auto-deferred 至 Phase 10):升级 prerequisite scope 包含 framework migration + NumaAffinity wrap 同 coordinated chain · Phase 10 plan 起草时拆为 "T0xx K8s baseline bump (v0.32.0 → v0.34.x/v0.35.x cohort · go.mod + kind config + helm + CI)" + "T0xx framework migration (9 files NodeInfo/CycleState interface)" + "T0xx NumaAffinity wrap upgrade (3 sanity tests + chart toggle)" 三段(2-3d budget)
- **Phase 10 W1 entry decision point**:re-WebFetch sched-plugins · 若 v0.35.x+ 已 GA · 评估是否直接跳到 v0.35.x/v0.36.x baseline(避免 v0.34.x → v0.35.x 双次 framework migration);若 v0.35.x 仍未发布 · default v0.34.7
- **known-issues #12** prerequisite refined:从"K8s baseline bump"扩展为"K8s baseline bump + scheduler framework API migration"
- **known-issues #13(ProxyImage chart flip)** inherits Phase 8 P8-T-004 conservative posture · re-eval Phase 10 demo polish window · 不再单独 Phase 9 评估
- **T004(deployment_builder annotation propagation polish)** Phase 9 W1 task chain 不受 T003 影响 · 独立 ship · 下一个 task
- **T005-T007(Quota CRD types + controller + PromQL extension)** Phase 9 W1 task chain 不受 T003 影响 · 独立 ship · 后续 tasks
- **T008(O2 DMS scaffold)** Phase 9 W1 task chain 不受 T003 影响 · 独立 ship · 后续 tasks

---

**END of P9-T-003 devlog**
