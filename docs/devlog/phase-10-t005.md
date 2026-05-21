# P10-T-005 · 三件套 part 3 — NumaAffinity wrap body lands(4 prior phase carry RESOLVED)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 0.5-1d plan / ~0.5d actual(API discovery + wrap delegate + args defaulting + 4 sanity tests + ADR/known-issues/DESIGN.md + devlog)

## Intent

Phase 10 三件套 closer · NumaAffinity wrap body 4-phase carry(Phase 6 T006 → Phase 7 P7-T-002 → Phase 8 P8-T-003 → Phase 9 P9-T-102)收敛于本 task。upstream `sigs.k8s.io/scheduler-plugins/pkg/noderesourcetopology` v0.34.7 引入 + `nrt.New(ctx, args, h)` delegate · 不 fork。chart `numaAffinity.enabled` default flip `false → true` · 4 sanity tests · known-issues #12 RESOLVED。

## Path adaptations(plan literal vs reality)

**3 处偏离 plan**(verify-before-claim · 不延续过期假定):

1. **Plan T005 `pkg/plugins/numa/affinity.go` path · 现实 `internal/plugins/numa/plugin.go`**:
   - Plan Allowed Paths 写 `operators/scheduler-plugin/pkg/plugins/numa/affinity.go`
   - 实测 grep:文件实际位置 `operators/scheduler-plugin/internal/plugins/numa/plugin.go`(同 T004)
   - 解决:用 reality path · 不创建 plan-literal path 否则 duplication
   - 同 acceptance `affinity_test.go` 实际 `plugin_test.go` · 同 `affinity_integration_test.go` 不存(只在 plugin_test.go 加 4 tests · 不需 separate integration)

2. **`internal/composition/composition.go` 不存在**:
   - Plan Allowed Paths "small edit · NumaAffinity plugin registration"
   - 实测 `ls internal/composition/` → 目录不存在
   - 实际 plugin 注册在 `cmd/main.go` 已经 `app.WithPlugin(numa.Name, numa.New)`(自 P6-T-002 起)· 无需新 edit
   - 解决:skip composition.go edit · plugin 注册 already in place · T005 wrap body 直接覆盖 placeholder 即生效

3. **`configmap-scheduler-config.yaml` 不存在 · 实际 `configmap.yaml`**:
   - Plan Allowed Paths `deploy/helm-charts/scheduler-plugin/templates/configmap-scheduler-config.yaml`
   - 实测 chart `templates/` 内是 `configmap.yaml`(non-suffix)
   - 解决:edit 实际 `configmap.yaml`(comment 改 `T006 deferred` 字样 → `P10-T-005 lands`)· chart 模板逻辑 `{{- if .Values.numaAffinity.enabled }}` 不动 · default flip 由 values.yaml 触发

## Debugging trail

- **Step 1 · API discovery `nrt.New` signature**:
  - `~/go/pkg/mod/sigs.k8s.io/scheduler-plugins@v0.34.7/pkg/noderesourcetopology/plugin.go:92`
  - `func New(ctx context.Context, args runtime.Object, handle framework.Handle) (framework.Plugin, error)` — 完全同我们 placeholder 的 New 签名 · drop-in delegate
  - upstream 验证 `args.(*apiconfig.NodeResourceTopologyMatchArgs)` — nil args 会 fail 类型断言

- **Step 2 · args defaulting strategy**:
  - 避免要求 chart 必带 `pluginConfig` block(需 scheme 注册到 cmd/main.go · T005 Forbidden Path)
  - 在 wrap 加 `if args == nil { args = defaultArgs() }`
  - defaultArgs() 返回 `ScoringStrategy=LeastAllocated · Resources cpu+memory weight 1:1`

- **Step 3 · 实施 wrap + 4 sanity tests**:
  - `numa/plugin.go` 全 rewrite · placeholder Name()-only struct 删除 · 改 thin delegate
  - `numa/plugin_test.go` 4 tests(替换原 2 placeholder tests):
    - TestNameConstants(local "NumaAffinity" + upstream "NodeResourceTopologyMatch" log-correlation)
    - TestDefaultArgs(默认 args 合法性 · ScoringStrategy + Resources verify)
    - TestArgsPassthrough(non-nil 自定义 args 不被覆写 · 直接 struct identity check)
    - TestScoringStrategyTypes(upstream type constants import contract)

- **Step 4 · ResourceSpec import drift**:
  - 第一次 build 报 `undefined: apiconfig.ResourceSpec`
  - 实测 sched-plugins v0.34.7 source:`Resources []schedconfig.ResourceSpec` · `schedconfig = k8s.io/kubernetes/pkg/scheduler/apis/config`
  - 解决:加 `schedconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"` import · 改 `[]apiconfig.ResourceSpec` → `[]schedconfig.ResourceSpec`

- **Step 5 · chart `numaAffinity.enabled: false → true`** + comment refresh on values.yaml + configmap.yaml

- **Step 6 · ADR-0010 + known-issues + DESIGN.md update**:
  - ADR-0010 §1 加 2026-05-21 P10-T-005 update segment(三件套 part 3 lands · args defaulting + chart flip + 4 sanity tests + verify outcome)
  - ADR-0010 §3 NumaAffinity reuse status flip "P10-T-005 LANDED" · 保留旧 placeholder + P7-T-002 升级尝试段 as historical trail
  - known-issues #12 status `OPEN → RESOLVED` + 5-phase carry tally closer + verify outcome + Real upstream nrt behavior CI gate
  - DESIGN.md §5.2 add new "NumaAffinity (Phase 10 P10-T-005 wrap body LANDED)" section + 5.2-historical 保留 pre-P10 trail

## Key decisions

- **wrap delegate > fork**:复用 upstream 成熟实现(NodeResourceTopology Cache + TopologyManager 策略集成完整)· 0 重复维护
- **args=nil defaultArgs() fallback** > 强制 chart pluginConfig:避免 cmd/main.go 改动(T005 Forbidden Path)· chart 默认 profile 更简单
- **`numaAffinity.enabled: true` default flip**:Operators wanting NUMA-aware scheduling 默认得到 · 显式 opt out 仍可
- **Name() 内部返回 "NodeResourceTopologyMatch"**(upstream)· 注册名 "NumaAffinity"(local):mismatch 仅 surface in 调度器日志 · 不影响 framework 的 profile 查找(用注册 name key)· 接受 quirk · 通过 `UpstreamName` 常量文档化
- **kind smoke real behavior 推后 CI gate**:Test 2/3/4 plan acceptance(NRT present / multi-NUMA / annotation override)需要 NodeResourceTopology CRD instances + 真集群 path · 在 phase-10-complete tag push 时 CI 跑 · 不在 unit test 范围
- **三件套 part 3 closes 5-phase carry**:P6-T-006(1st)→ P7-T-002(2nd)→ P8-T-003(3rd)→ P9-T-102(4th)→ P10-T-005(RESOLVED)· known-issues #12 closer

## Verification

P3 三项验证维度:

| 维度 | 方法 | 结果 |
|---|---|---|
| **存在性** | `git diff --stat` files modified;`grep nrt\.\|sigs.k8s.io/scheduler-plugins` plugin.go;`grep numaAffinity.enabled: true` values.yaml | numa/plugin.go + numa/plugin_test.go 重写 · values.yaml 改 + configmap comment 改 · ADR-0010 §1 + §3 · known-issues #12 · DESIGN.md §5.2 · devlog 新 · go.mod + go.sum bumped(sched-plugins direct require + indirect deps) |
| **完整性** | Plan §3 T005 acceptance 6 项核(build clean / vet clean / test PASS 4 new + 既有 baseline preserved / helm lint clean / helm template renders NumaAffinity / kind smoke deferred to CI gate)+ 8 文件 Allowed Paths boundary | 5/6 acceptance items + kind smoke deferred per `feedback_post_tag_ci_gate` · 8 文件: numa/plugin.go + numa/plugin_test.go + go.mod + go.sum + values.yaml + configmap.yaml + ADR-0010 + known-issues + DESIGN.md + devlog |
| **正确性** | `go build` / `go vet` / `go test` / `helm lint` 真跑 · 4 packages 全 PASS · helm template render verify `- name: NumaAffinity` in filter + preScore + score sections | 全 clean · numa package 4 sanity tests PASS · helm template renders NumaAffinity 在 filter(`- name: NumaAffinity`)+ preScore(无)+ score(`- name: NumaAffinity weight: 2`) |

**Verify output 实证**:
```
=== go mod tidy scheduler-plugin ===
exit 0 · downloaded sigs.k8s.io/scheduler-plugins v0.34.7 + k8s.io/kubernetes/
pkg/scheduler/apis/config + transitive deps(noderesourcetopology-api +
podfingerprint + go-logr + topologyaware scheduling)

=== go build ./operators/scheduler-plugin/... ===
exit 0 · 无 framework drift error · clean

=== go vet ./operators/scheduler-plugin/... ===
exit 0

=== go test ./operators/scheduler-plugin/internal/... ===
ok internal/integration         (cached)
ok internal/plugins/binpack     (cached)
ok internal/plugins/hccs        (cached)
ok internal/plugins/numa        0.355s · 4 new tests PASS

=== helm lint --strict deploy/helm-charts/scheduler-plugin/ ===
1 chart(s) linted, 0 chart(s) failed

=== helm template (default · numaAffinity.enabled=true) ===
- name: NumaAffinity (in filter.enabled)
- name: NumaAffinity (in score.enabled · weight: 2)
```

## Carry-forward

- **三件套 全 land · known-issues #12 RESOLVED** · scheduler-plugin Phase 10 K8s 1.34 baseline + framework migration + NumaAffinity wrap closer
- **Phase 10 W1 progress**:T001 + T002 + T003 + T004 + T005 = 5/8 W1 tasks done · 接下来 T006 demo-backend cache impl · T007/T008 IMS-1/2 controller body
- **CI gate post-tag**:phase-10-complete tag push 时 kind smoke phase6/install.sh 真集群 + KubeSchedulerConfiguration NumaAffinity actually loads + Filter+Score 调用 path 真测 · NodeResourceTopology CRD 必须 deploy 才有真 behavior · 否则 nrt.Filter 走 fail-safe permissive path

## §0a.10 / §0a.11 + memory adherence

- 本 task 在同 execute session 起 T005 · 紧接 T004 commit `c833d6a` · per `feedback_strict_per_task_verify` 默认按 plan 顺序连续推进
- §0a.11 main-agent serial(scheduler-plugin module · framework wrap · 不派 subagent)· strict verify
- per `feedback_push_at_phase_tag_only` commit 后停 · 累积本地(dev ahead by 5 commits after this)
