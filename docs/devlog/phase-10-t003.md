# P10-T-003 · K8s baseline bump 三件套 part 1(scheduler-plugin v0.32 → v0.34.7 + kindest/node v1.32 → v1.34.3 + option B per-module skew)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 1-2d plan / ~1d actual(option B 务实 scope · 比 plan literal "全 9 module lockstep" 收敛)

## Intent

Phase 10 三件套(K8s baseline bump + scheduler framework migration + NumaAffinity wrap)part 1 · bump runtime baseline 1.32 → 1.34 · 不 touch scheduler-plugin source(T004 part 2 owns framework migration)。Phase 9 P9-T-003 整任务尝试 bump 1.34 但 framework restructuring 9 files 超出 T003 Forbidden Paths → reverted → Phase 10 拆三件套 mitigation。本 task 严格执行 part 1 = "baseline + dep 升级 · 不动 source"。

## Path adaptations(plan literal vs reality)

**3 处重大偏离 plan 假定**(option B 重塑 deliverable scope):

1. **Per-module K8s version 已 drift · "lockstep no skew" 假定不成立**:
   - Plan §3 T003 acceptance "All go.mod files updated to target K8s minor in lockstep (no version skew)"
   - 现实 grep:scheduler-plugin v0.32 / o2-dms-adapter v0.36.1 / 3 modules v0.35.0 / backend v0.31.4 / 3 IMS scaffold 无直接 dep
   - 强制 lockstep 1.34 = downgrade 4 modules(o2-dms-adapter / 3 drift modules)+ upgrade 1 module(backend)· 破 KubeEdge lock
   - 解决:用户 chat 接受 option B(务实 lockstep runtime + 尊重既有 locks)· 真 bump 只 scheduler-plugin + kindest/node + workflow comment · 已 drift 模块留 · backend 留 KubeEdge lock
   - ADR-0001 v3 §5 加 explicit per-module skew policy(2026-05-21 P10-T-003 update)替换隐性 "不动"

2. **Upstream tracker 排除 1.35/1.36**:
   - Plan §3 T003 decision branches 列 4 选项(1.34/1.35/1.36/defer)
   - WebFetch 实测:sched-plugins v0.35+/v0.36+ 未发布 · kindest/node v1.36 image 不存在 · KEP-4815 仅 Beta in 1.36(T202 cascade 不 unlock)
   - 1.34 是唯一 feasible · 其他 3 选项被 upstream gap blocked
   - 推荐 deterministic outcome · 但 W1 entry meeting agenda item 1 是 explicit decision point · 仍走 chat 信号

3. **kind v0.25.0 → v0.31.0(不是 plan 暗示的 v0.30.0)**:
   - P9-T-003 devlog migration matrix 暗示 v0.30.0(因为 v1.34.0 在 v0.30.0)
   - kind v0.31.0 ships v1.34.3(latest 1.34 patch · 同时也支持 v1.35.0)+ v1.33.7 + v1.32.11 + v1.31.14
   - 选 v0.31.0 一次到位 · 未来 evaluate 1.35 时不需再 bump kind binary

## Debugging trail

- **Step 1 · WebFetch upstream 3 tracker**:
  - kubernetes-sigs/kind:v0.31.0 latest(2024-12-18)· v1.34.3 / v1.35.0 / v1.33.7 / v1.32.11 / v1.31.14 images · **v1.36 image 不存在**
  - kubernetes-sigs/scheduler-plugins:v0.34.7 latest GA(2024-04-20)· **v0.35.x / v0.36.x 未发布**
  - KEP-4815 Partitionable Devices:Beta in K8s 1.36 · GA not targeted · T202 cascade 不 unlock(详 ADR-0016 §3 Stream 6)

- **Step 2 · Per-module survey grep**:见 ADR-0001 v3 §5 2026-05-21 update 表 · 6 类 module 状态(active lockstep / drift-tolerant / transitively constrained / K8s-version-agnostic / frozen for KubeEdge)

- **Step 3 · 用户 chat 决策 = Option B**:
  - 提议 A(strict lockstep)/ B(务实 runtime lockstep + 尊重 locks)/ C(A + bump backend)三选项
  - 用户 "接受建议 B" — 务实路径

- **Step 4 · 实际 edits 收敛到 6 files**:
  - `operators/scheduler-plugin/go.mod`:v0.32.0 → v0.34.7(`replace_all`)· v1.32.0 → v1.34.7(`k8s.io/kubernetes`)· v0.31.8 → v0.34.7(`replace_all` for indirect block)· comment update P10-T-003 三件套 part 1
  - `tests/e2e/kind/kind-config.yaml`:v1.32.0 → v1.34.3(`replace_all` · 3 places · control-plane + 2 worker · K8s 1.34 DRA GA `resource.k8s.io/v1` 兼容 `v1beta1` chart serves)
  - `.github/workflows/e2e-kind.yml`:kind v0.25.0 → v0.31.0 + comment block update(DRA GA in 1.34 note + Partitionable Devices Beta in 1.36 ADR-0016 §3 Stream 6 cross-ref)
  - `docs/adr/0010-scheduler-plugin.md` §1:add 2026-05-21 P10-T-003 update segment(三件套 part 1 lands + per-module skew policy + execution trail)
  - `docs/adr/0001-phase0-key-decisions.md` §5:add 2026-05-21 P10-T-003 update(per-module K8s skew policy explicit · 6 类 module 表 + 4-point policy 解读 + Phase 11+ re-eval triggers)
  - `docs/devlog/phase-10-t003.md`(本文件)

- **Step 5 · `go mod tidy` clean exit 0**:scheduler-plugin downloaded v0.34.7 cohort + transitive deps · 30 indirect entries auto-bumped(`k8s.io/apiextensions-apiserver` v0.31.8 → v0.34.7 etc.)· 无 conflict

- **Step 6 · `go build ./...` per module**(verify · plan acceptance "scheduler-plugin module may show stale due to framework API drift · expected · T004 owns fix-up"):
  - scheduler-plugin:**rc=1 · 15 errors across 9 files**(同 P9-T-003 fail pattern):
    - `framework.Status` / `NewStatus` / `Error` / `StateKey` / `StateData` undefined
    - `framework.NodeInfo` interface vs `*framework.NodeInfo` struct pointer 不兼容(`cannot use nodeInfo (variable of interface type "k8s.io/kube-scheduler/framework".NodeInfo) as *"k8s.io/kubernetes/pkg/scheduler/framework".NodeInfo value in argument`)
    - 影响:hccs/{filter,score,plugin,filter_test,score_test}.go + binpack/{binpack,binpack_test}.go + numa/plugin.go + internal/integration/integration_test.go
    - **状态**:**expected per Phase 10 plan §3 T003 acceptance** · T004 owns fix-up
  - npu-dra-driver:**rc=0 ✓**(v0.35.0 drift · client-go backward compat to 1.34 runtime · 同 verify Option B "已 drift 留" 正确)
  - inference-operator:**rc=0 ✓**(v0.35.0 同 npu-dra-driver)
  - pool-operator:**rc=0 ✓**(v0.35.0 同)
  - o2-dms-adapter:**rc=0 ✓**(v0.36.1 drift up · 仍 backward compat)
  - backend:**rc=0 ✓**(v0.31.4 KubeEdge lock · 不动)

## Key decisions

- **Option B 务实 scope > plan literal "lockstep no skew"** — plan 假定 clean slate 与 reality drift 矛盾;P3 verify-before-claim 反对延续 stale 假定;option B 落 explicit policy(ADR-0001 v3 §5)替换 implicit "不动"
- **scheduler-plugin source NOT touched · T004 owns fix-up** — 严格遵 plan T003 Forbidden Paths · `internal/plugins/{hccs,binpack,numa}/*.go` + `internal/integration/*.go` 不动 · framework migration 是 T004 part 2 完整 task scope
- **kind v0.31.0(v1.34.3)** > v0.30.0(v1.34.0) · 一次到位 · 不阻塞未来 1.35 evaluate
- **backend KubeEdge lock 显式 codify in ADR-0001 v3 §5 表** — 之前是 implicit("不动" comment)· 现在是 explicit("frozen for KubeEdge primary edge-path constraint")
- **k8s.io/kubernetes v1.34.7** 是 sched-plugins v0.34.7 lockstep cohort 顶端 minor patch(P9-T-003 同选)· 不选 v1.34.0 GA initial · 选 latest patch 拿 backport bug fix

## Verification

P3 三项验证维度:

| 维度 | 方法 | 结果 |
|---|---|---|
| **存在性** | `git diff --stat` 看实际 6 files changed;`grep -n "v0.34.7\|v1.34.7\|v1.34.3\|v0.31.0\|P10-T-003"` 各 target file | scheduler-plugin go.mod / kind-config.yaml / e2e-kind.yml / ADR-0010 §1 / ADR-0001 §5 / devlog 全 6 files 命中 |
| **完整性** | Plan §3 T003 acceptance 9 项逐项核(go.mod 升级 · go mod tidy clean · scheduler-plugin build FAIL expected · 其他 modules build clean · helm lint TBD · Phase 5-9 kind smoke re-run TBD · ADR-0010 §1 update · devlog · Allowed Paths boundary)+ option B departure(per-module skew policy in ADR-0001 §5) | 7/9 acceptance items + option B addition;helm lint + Phase 5-9 kind smoke re-run 不在 main-agent direct verify 范围(在 CI gate per `feedback_post_tag_ci_gate` phase tag push 后) |
| **正确性** | `go mod tidy` exit 0;`go build` per module rc 实测;framework error pattern 与 P9-T-003 devlog 比对 1:1 | 全 verified;scheduler-plugin error 与 P9-T-003 同 9 files 同 error categories |

**Verify output 实证**:
```
=== go mod tidy scheduler-plugin ===
exit 0 · clean · v0.34.7 cohort + 30 indirect propagate

=== go build ./... per module ===
scheduler-plugin: rc=1 · 15 errors across 9 files
  (framework.Status / NewStatus / NodeInfo interface drift)
  PER PLAN T003 ACCEPTANCE · T004 OWNS FIX-UP
npu-dra-driver:    rc=0 ✓
inference-operator: rc=0 ✓
pool-operator:     rc=0 ✓
o2-dms-adapter:    rc=0 ✓
backend:           rc=0 ✓
```

## Carry-forward

- **T004 三件套 part 2 = scheduler framework migration**:9 files NodeInfo + CycleState struct→interface + Status/StateKey/StateData import path migrate · Phase 10 plan §3 T004 Allowed Paths(`internal/plugins/{hccs,binpack,numa}/**` + `cmd/main.go` + `internal/composition/composition.go` + `internal/integration/integration_test.go`)· 估 1-2d · 即开
- **T005 三件套 part 3 = NumaAffinity wrap body**:depend on T004 完成 · `noderesourcetopology.New(ctx, args, h)` wrap + 4 sanity tests + chart toggle default flip · 估 0.5-1d
- **T202 Partitionable Devices cascade unchanged**:KEP-4815 仍 Beta(per ADR-0016 §3 Stream 6)· 1.34 bump 不 unlock · Phase 11+ defer
- **CI gate post-tag**:phase-10-complete tag push 时 helm lint --strict + Phase 5-9 kind smoke E2E re-run 才实际跑 · 本 task verify 范围内 scheduler-plugin build FAIL 是 expected · 但 fix-001 series 可能在 tag push 后 CI 暴露其他 dep 不兼容 · 同 P7-fix-001..004 模式(per `feedback_post_tag_ci_gate`)

## §0a.10 / §0a.11 + memory adherence

- 本 task fresh in same execute session (T001 + T002 + 本 T003 same chat)· per `feedback_strict_per_task_verify` 默认按 plan 顺序连续推进
- §0a.11 main-agent serial(cross-module · 不派 subagent)· strict verify 各 module 真跑 `go build`
- per `feedback_push_at_phase_tag_only` commit 后停 · 不主动 push · 累积本地(dev ahead by 3 commits after this)
- Option B 用户决断 = `feedback_strict_per_task_verify` "仅必要交互时停" 实战 — bump target decision 不能默认推
