# P10-T-004 · 三件套 part 2 — scheduler framework migration(NodeInfo/CycleState struct→interface · 9 files · build/vet/test/lint all clean)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 1-2d plan / ~1d actual(API drift mapping 一次 grep + 9 file 系统化 edit + verify · 比 P9-T-003 estimate matrix 5h 更紧凑)

## Intent

Phase 10 三件套 part 2 — K8s 1.34 scheduler framework restructuring 后 scheduler-plugin source build break(15 errors across 9 files per T003 expected outcome)的 systematic fix-up。把 plugin data types(NodeInfo / CycleState / Status / Code constants / StateKey / StateData)从 `k8s.io/kubernetes/pkg/scheduler/framework`(struct types)迁移到 `k8s.io/kube-scheduler/framework`(interface types · plan data contract)。plugin 接口契约(Plugin / FilterPlugin / ScorePlugin / Handle)留在 `k8s.io/kubernetes/pkg/scheduler/framework`。

## Path adaptations(plan literal vs reality)

**2 处偏离 plan 假定**(全 verify-before-claim grep · 不延续 P9-T-003 devlog 假定):

1. **Score 签名 v1.34 实际变化**(plan 未明示):
   - Plan T004 acceptance 列 9 files + "NodeInfo + CycleState struct→interface migration" · 暗示只是 type 改 not 签名改
   - 实测 v1.34 `ScorePlugin.Score(ctx, state, pod, nodeInfo fwk.NodeInfo)` — 最后 param 从 `nodeName string` 改为 `nodeInfo fwk.NodeInfo`!这是 method signature 实质改变
   - 影响:hccs/score.go Score 方法签名重写 · binpack/binpack.go 同 · 不再需 `p.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)` lookup(冗余消除)· 内部需要 nodeName 时用 `nodeInfo.Node().Name`
   - tests 同步:wiring helper(runPreScoreAndScore / runHCCSScore)build NodeInfo 再传给 Score

2. **cmd/main.go 不需改**(plan Allowed Paths 列 conservative):
   - Plan T004 Allowed Paths 含 `cmd/main.go(scheduler app.NewSchedulerCommand registration · API drift absorb)`
   - 实测:`app.NewSchedulerCommand` 来自 `k8s.io/kubernetes/cmd/kube-scheduler/app` — 不是 framework 数据契约包 · 不受 plugin 数据 type 迁移影响
   - `go build cmd/main.go` exit 0 不动 · plan Allowed Paths 标 cmd/main.go 是 conservative scope · 实际无需改
   - 结论:keep cmd/main.go untouched · 减少 commit scope · 同时验证 plan author 假定的"API drift" 仅限 plugin 数据契约 · 不波及 scheduler app entrypoint

## API drift mapping(P10-T-004 关键 finding)

K8s 1.34 把 plugin data 契约从 `k8s.io/kubernetes/pkg/scheduler/framework`(struct types) 拆出到 `k8s.io/kube-scheduler/framework`(interface types · plan contract):

| 类别 | 旧(`framework.X` struct types in k8s.io/kubernetes)| 新(`fwk.X` interface types in k8s.io/kube-scheduler) |
|---|---|---|
| **Data types** | `*framework.NodeInfo` / `*framework.CycleState` | `fwk.NodeInfo` / `fwk.CycleState` interface |
| **Status types** | `*framework.Status` / `framework.NewStatus()` | `*fwk.Status` / `fwk.NewStatus()` |
| **Code constants** | `framework.Error` / `framework.Success` / `framework.UnschedulableAndUnresolvable` / `framework.Skip` / `framework.Wait` / `framework.Pending` | `fwk.Error` / `fwk.Success` / `fwk.UnschedulableAndUnresolvable` / etc. |
| **CycleState helpers** | `framework.StateKey` / `framework.StateData` | `fwk.StateKey` / `fwk.StateData` |
| **NodeInfo accessor** | `ni.Allocatable.MilliCPU`(field)| `ni.GetAllocatable().GetMilliCPU()`(interface method) |

**保留位置**(`framework.X` 仍 in `k8s.io/kubernetes/pkg/scheduler/framework`):
- Plugin 契约接口:`Plugin` / `FilterPlugin` / `ScorePlugin` / `PreScorePlugin` / `Handle` / `ScoreExtensions` / `NodeScoreList` etc.
- 构造器:`NewNodeInfo()` / `NewCycleState()`(返回 struct pointer · 满足 `fwk.X` interface)
- Plugin 接口 method signatures 内部用 `fwk.X` 数据类型(`Filter(... fwk.NodeInfo) *fwk.Status`)

**Plugin method signature 实际改变**(v1.34 vs v1.32):
- `Filter(ctx, state fwk.CycleState, pod, nodeInfo fwk.NodeInfo) *fwk.Status` — nodeInfo 改 interface
- `PreScore(ctx, state fwk.CycleState, pod, nodes []fwk.NodeInfo) *fwk.Status` — slice 元素改 interface
- `Score(ctx, state fwk.CycleState, p, nodeInfo fwk.NodeInfo) (int64, *fwk.Status)` — **nodeName string → nodeInfo fwk.NodeInfo**

## Debugging trail

- **Step 1 · API discovery via go module cache grep**:
  - `~/go/pkg/mod/k8s.io/kube-scheduler@v0.34.7/framework/interface.go` line 246 `NodeInfo interface`,line 267 `GetAllocatable() Resource`
  - `~/go/pkg/mod/k8s.io/kube-scheduler@v0.34.7/framework/cycle_state.go` line 45 `CycleState interface`,line 39 `StateKey string`,line 31 `StateData interface`
  - `~/go/pkg/mod/k8s.io/kube-scheduler@v0.34.7/framework/interface.go` line 218 `func NewStatus(code Code, reasons ...string) *Status`,line 37 `Success Code = iota` 等 Code 常量
  - `~/go/pkg/mod/k8s.io/kubernetes@v1.34.7/pkg/scheduler/framework/interface.go` line 333 `FilterPlugin interface`,line 388 `PreScorePlugin interface`,line 408 `ScorePlugin interface` · 全用 `fwk.X` 数据类型在 method signatures · `ScorePlugin.Score` 用 `nodeInfo fwk.NodeInfo`
  - `~/go/pkg/mod/k8s.io/kubernetes@v1.34.7/pkg/scheduler/framework/types.go` line 1058 `NewNodeInfo(pods ...*v1.Pod) *NodeInfo`(返回 struct pointer · 仍 satisfy `fwk.NodeInfo` interface)
  - `~/go/pkg/mod/k8s.io/kubernetes@v1.34.7/pkg/scheduler/framework/cycle_state.go` line 42 `NewCycleState() *CycleState`

- **Step 2 · Migration strategy choice**:
  - 选 dual import:`framework "k8s.io/kubernetes/..."` + `fwk "k8s.io/kube-scheduler/..."`
  - 数据类型用 `fwk.X` · 接口契约用 `framework.X` · 构造器用 `framework.NewX`
  - 避免单一 import strategy(全 `fwk` 会丢 Plugin 接口 / 构造器;全 `framework` 不满足 v1.34 method signature interface types)

- **Step 3 · Per-file edits**(9 files · `git diff --stat` 实证 +112/-58 = 170 lines changed):
  1. `hccs/filter.go`(22 lines)— import + Filter 签名(`nodeInfo *framework.NodeInfo` → `nodeInfo fwk.NodeInfo`)+ 6 处 `framework.NewStatus(framework.X, ...)` → `fwk.NewStatus(fwk.X, ...)`
  2. `hccs/score.go`(31 lines)— **Score 签名 nodeName → nodeInfo**(plan acceptance 暗示但未明示的变化)+ PreScore 签名 + readHCCSState param + Score body 内 `nodeName := nodeInfo.Node().Name` + 6 处 `fwk.X` status + StateKey + StateData
  3. `hccs/plugin.go`(7 lines)— 加 note comment block · imports 不动(framework.Plugin / FilterPlugin / Handle 保留)
  4. `hccs/filter_test.go`(14 lines)— import + makeNodeInfo doc + `framework.UnschedulableAndUnresolvable` → `fwk.UnschedulableAndUnresolvable`(replace_all)
  5. `hccs/score_test.go`(12 lines)— import + runPreScoreAndScore 现在 build NodeInfo 再传给 Score
  6. `binpack/binpack.go`(54 lines)— import + Score 签名 + 移除 SnapshotSharedLister lookup(冗余 · 现 nodeInfo 直接 param)+ scoreNodeForPod nodeInfo 参数类型 + allocatableFor 重写(`ni.Allocatable.X` field → `ni.GetAllocatable().GetX()` method)
  7. `binpack/binpack_test.go`(7 lines)— import comment + Score wrapper test build NodeInfo
  8. `numa/plugin.go`(7 lines)— 加 note comment(placeholder body retained · T005 owns wrap)· imports 不动
  9. `integration/integration_test.go`(16 lines)— import + 2 wiring helpers(runHCCSFilter return type + runHCCSScore build NodeInfo)+ `framework.UnschedulableAndUnresolvable` → `fwk.X`

- **Step 4 · `go build ./...` exit 0**(首次 build 即 clean · 无 fix-up loop · API mapping 上下文准确)

## Key decisions

- **Dual import strategy** > single import — preserves 接口契约 + data types 分离的 v1.34 设计 · 不引入大量 alias
- **cmd/main.go 不动** — plan Allowed Paths conservative,实际 `app.NewSchedulerCommand` 不在 framework 数据契约迁移 scope · 减少 commit scope · per P3 verify-before-claim
- **Note comment blocks 加 plugin.go + numa/plugin.go** — 解释 import 策略 + T005 forward note · 防未来读者误以为 numa placeholder body 是 T004 漏改
- **Score 签名重写优于 keep nodeName via interface adapter** — 接受 v1.34 API change 实质 · 不引入 shim layer · 5 处 calling site update 比 shim 维护成本低
- **scoreNodeForPod nodeInfo param 改 interface**(不再 keep pointer fallback)— testable 路径同样改 · 因为 interface 与 struct pointer 共存测试不优雅
- **`ni.GetAllocatable()` 缓存到 `alloc` 局部变量** — 4 switch case 共享 · 避免多次 interface method call(微优化 · 但更可读)
- **NumaAffinity placeholder body 保持** — 严格按 plan T004 Forbidden Paths(`affinity.go body` placeholder retains · T005 part 3 owns body)· T005 即可 light up

## Verification

P3 三项验证维度 全过:

| 维度 | 方法 | 结果 |
|---|---|---|
| **存在性** | `git diff --stat`(9 files modified · +112/-58 = 170 lines)+ `grep -n "fwk\\.\|k8s.io/kube-scheduler"` 每个 target file | 全 9 files 命中 fwk.X usage · cmd/main.go 不动 verified |
| **完整性** | Plan §3 T004 acceptance 6 项逐项核(build clean / vet clean / test PASS / helm lint clean / kind smoke phase6 skipped · post-tag CI runs / ADR-0010 §1 update + §3 NumaAffinity cross-ref T005 + devlog) | 5/6 acceptance items + kind smoke deferred to phase-10-complete tag CI gate · ADR-0010 §1 P10-T-004 update + §3 P10-T-004 note · devlog · 全 5 done |
| **正确性** | `go build` / `go vet` / `go test` / `helm lint` 真跑 · 比对 build error 前后 from 15 errors → 0 · test 全 packages PASS | 4 包全 PASS:integration(5 sub-tests · 真 fixture)+ binpack(6 sub-tests + TestParseArgs)+ hccs(filter 6 + score 9 sub-tests + parsePreferredRings + parseArgs + buildAdjacency)+ numa(no tests · placeholder)· 全 P6-T-002..T008 + P7-T-003 + P8-T-005 baseline tests preserved |

**Verify output 实证**:
```
=== go build ./operators/scheduler-plugin/... ===
exit 0 (was 1 with 15 errors before T004)

=== go vet ./operators/scheduler-plugin/... ===
exit 0

=== go test ./operators/scheduler-plugin/... ===
ok  internal/integration                0.581s
ok  internal/plugins/binpack            0.494s
ok  internal/plugins/hccs               0.498s
ok  internal/plugins/numa               0.491s (no tests)

=== helm lint --strict deploy/helm-charts/scheduler-plugin/ ===
1 chart(s) linted, 0 chart(s) failed
```

## Carry-forward

- **T005 三件套 part 3 = NumaAffinity wrap body** prerequisites 全 met:K8s 1.34 baseline ✓ · framework migration ✓ · sched-plugins v0.34.7 lockstep ✓ · 即可 introduce `sigs.k8s.io/scheduler-plugins v0.34.7` direct require + `noderesourcetopology.New(ctx, args, h)` wrap + 4 sanity tests + chart toggle default flip · 估 0.5-1d
- **CI gate post-tag**:phase-10-complete tag push 时 kind smoke phase6/install.sh 真集群 install + assert.sh + Phase 5-9 phases re-run with K8s 1.34 baseline · 本 task verify 范围 = unit + helm lint(不跑 kind cluster · per `feedback_post_tag_ci_gate`)
- **K8s 1.34 DRA GA `resource.k8s.io/v1` 兼容性**:scheduler-plugin 走 `k8s.io/api/resource/v1beta1` 不变(npu-dra-driver / inference-operator / pool-operator 各 module 走 `v1beta1` ResourceSlice / DeviceClass · Phase 11+ 评估 `v1` migrate)· kind cluster K8s 1.34 同时 serve `v1` + `v1beta1` · 兼容
- **fwk vs framework 区分**:future T006-T008 IMS controller body 不涉及 scheduler-plugin · 不需 fwk import · plugin layer 自包含

## §0a.10 / §0a.11 + memory adherence

- 本 task 在同 execute session 起 T004 · 紧接 T003 commit `8d70efa` · per `feedback_strict_per_task_verify` 默认按 plan 顺序连续推进
- §0a.11 main-agent serial:single module(scheduler-plugin)· 不派 subagent · API migration 需 careful sequencing
- per `feedback_push_at_phase_tag_only` commit 后停 · 累积本地(dev ahead by 4 commits after this)· 不主动 push · 不问 push
- 9 file edit 量大但 systematic · 不算"事前预防大型交付物"分批阈值(每 file < 100 lines · 总 +112/-58)
