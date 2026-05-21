# ADR-0010: scheduler-plugin design — HCCSTopologyPlugin Filter+Score + NumaAffinity reuse + Binpack opt-in + Phase 6 CNI selection

- **状态**:Accepted (design freeze; implementation P6-T-002..T008)(2026-05-20 — Phase 6 P6-T-001)
- **日期**:2026-05-20
- **决策者**:协调者(用户)
- **相关**:ADR-0001 v3(双轨路径)/ ADR-0007(fabric discovery)/ ADR-0008(PD Router webhook · slice-bindings annotation source)/ ADR-0009(npu-dra-driver · ResourceSlice attribute schema + topology-aware allocator forward note)/ phase6-plan.md §3 P6-T-001..T008 / docs/cni-hccl-research.md §4(Phase 6 entry recommendation)/ docs/checkpoint-phase5.md §7(Phase 6 handoff brief)/ architecture.md §5.6(scheduler-plugin 模块)+ §6.3(HCCSTopologyInfo)+ §13(Phase 5+ NPU pod 网络 + Phase 6 HCCS 调度行)

---

## 上下文

Phase 5 完成了 ModelService → PD-pair Deployment → ResourceClaim → npu-dra-driver
greedy first-fit allocator → NPUSliceAllocation 审计 CRD → PD Router mutating
webhook 注入 `npu.huawei.com/slice-bindings` 注释的完整闭环。但分配决策**没有拓扑感知**:

- npu-dra-driver allocator(`operators/npu-dra-driver/internal/allocator/`)是 greedy /
  best-fit 设备级算法,不读 `npu.huawei.com/hccs_ring` / `npu.huawei.com/numa_node` 属性。
- ResourceSlice attributes(`hccs_ring` / `numa_node`)已 publish(P4-T-005 + P5-T-002)
  但消费侧空缺。
- npu-dra-driver/DESIGN.md §6.3.1 明确:"Phase 6 supersedes both with topology-aware
  scoring (NUMA + HCCS-ring affinity) inside `kube-scheduler` via a scheduler-plugin".
- NPUSliceAllocation reverse-lookup index(npu-dra-driver/DESIGN.md §6.3.4 §1)预留给
  scheduler-plugin 做"同 ModelService Pod 优先同 HCCS ring"协同决策。
- 同时 Phase 5 P5-T-105 落地的 `docs/cni-hccl-research.md` §4 推荐 Cilium + Multus +
  SR-IOV(top)/ Calico + Multus + SR-IOV(fallback),但选型 deferred 到 Phase 6 入口。
- arch §13 Phase 6 row 标注"HCCS 拓扑获取接口可能要走 Huawei SDK · 调研 npu-smi / DCMI 接口" —
  Phase 6 simulator 仍用 mock JSON(set-a-small `hccsGroup` + `hccsRing` 字段已就位),
  真硬件接口 deferred 到 Phase 7。

本 ADR 锁定 Phase 6 scheduler-plugin 的:框架选型 / 三个 plugin 的语义 / args schema /
ResourceSlice attribute 消费契约 / CNI 选型 / Phase 7+ forward notes。Phase 6
T002-T008 直接按本表实施,不再设计。

---

## 决策

### 1. 框架与部署形态

**框架**:`sigs.k8s.io/scheduler-plugins` v0.31.x(对齐 e2e-kind 当前 K8s 1.32 baseline,
P5-T-114 落地;具体 patch 版本在 P6-T-002 task entry 按当时 latest release notes 拍定)。
使用上游 plugin framework 的 **factory + KubeSchedulerConfiguration** 模式;不 fork,只 wrap。

> 🆕 **2026-05-21 update (P8-T-002 · K8s baseline stay at 1.32)**:Phase 8 W1 entry re-WebFetch findings:
> - **upstream sched-plugins latest = v0.34.7**(2026-04-20 GA);v0.35.x / v0.36.x **未发布**
> - **kind v0.31.0** 仍最新(2025-12-18)· `kindest/node:v1.36` 镜像**不存在** · 最高 prebuilt = v1.35.0
> - **K8s 1.36.1** itself GA(2026-05-12)but cluster-tooling lag(kind / sched-plugins)blocks 1.36 baseline
>
> **用户决策**(2026-05-21 chat):**Phase 8 stay at K8s 1.32 baseline**,不主动 upgrade(降低本期风险 · 不引入 baseline drift)。具体效果:
> - scheduler-plugin go.mod 维持 K8s v0.32.0 / sched-plugins replace block v0.32.0(本节 §1 baseline 不变)
> - 主模块(npu-dra-driver / inference-operator / pool-operator)go.mod 维持当前状态(K8s v0.35.0 · controller-runtime v0.23.3 — Phase 7 期间因日常 `go mod tidy` 微飘升 · 不主动 downgrade · 也不主动 upgrade)
> - kindest/node v1.32.0 不变;CI workflow / Helm chart kubeVersion 不变
>
> **Phase 8 影响**:
> - **T003**(NumaAffinity wrap upgrade)→ **doc-only carry to Phase 9**:sched-plugins v0.32.x 与 K8s 1.32 baseline 的 framework.GVK 不兼容(详 `operators/scheduler-plugin/internal/plugins/numa/plugin.go` API drift 表)· 不 bump baseline 即无法 light up
> - **T101 / T102**(Partitionable Devices Beta + partition-aware allocator)→ **doc-only refresh + carry to Phase 10**:KEP-4815 1.36 Beta 需要 K8s 1.36 cluster · 但 kindest/node v1.36 不在 kind v0.31 · 加上用户 stay 1.32 决策,Beta enable 路径完全 blocked
> - **T004**(vllm-ascend ProxyImage chart 默认 flip)、**T005-T008**(NPUVerticalScaler CRD + ingestor + controller + AllocateBundle wiring)、**T103/T104**(kind smoke ext + HCCS hard-fail upgrade)、**T106**(Volcano spike)、**T107**(checkpoint + tag)— 均**独立于 K8s baseline 升级** · 按 plan 继续
>
> **下一次 baseline bump 评估时机**:Phase 9 W1 entry re-WebFetch — 届时若 sched-plugins v0.35.x+ GA + kindest/node v1.36+ 在 kind release · 重新评估升级路径。known-issues #12(NumaAffinity)继续 OPEN 跟踪。

> 🆕 **2026-05-21 update (P9-T-003 · Bump 1.34 attempted · framework API drift surfaced · revert to v0.32.0)**:Phase 9 W1 entry re-WebFetch findings:
> - upstream sched-plugins **v0.34.7** GA(同 Phase 8 W1 latest 2026-04-20)— v0.35.x / v0.36.x 仍未发布
> - kind **v0.31.0** (2025-12-18) ships kindest/node:**v1.35.0** · kind **v0.30.0** (2025-08-27) ships kindest/node:**v1.34.0** · v1.36 镜像仍不存在 (与 Phase 8 W1 同)
> - Phase 9 plan §3 T003 default-policy 触发条件:`Doc-only refresh unless re-WebFetch shows BOTH (kindest/node ≥1.34 released stable AND sched-plugins ≥v0.34.x GA released)` — BOTH 条件 met → bump 1.34 plan-default 路径 activated
>
> **用户决策**(2026-05-21 chat · Phase 9 W1 entry T003 path choice):**A · Bump 1.34**(plan-recommended · 1-2d 跨模块 · CI may surface dep API drift)
>
> **执行过程 + 复盘**(2026-05-21 main agent 主动汇报):
> 1. 编辑 `kind-config.yaml` kindest/node:v1.32.0 → v1.34.0 · `scheduler-plugin/go.mod` v0.32.0 → v0.34.7 (require + replace block + indirect block 全部) · `e2e-kind.yml` kind version v0.25.0 → v0.30.0 + comment refresh
> 2. `go mod tidy` clean (exit 0 · 下载 K8s v0.34.7 cohort + transitive deps · 没有 conflict)
> 3. **`go build ./...` FAIL**:K8s 1.34 scheduler framework restructured · `framework.Status` / `NewStatus` / `Error` / `StateKey` / `StateData` / `NodeInfo` 从 `k8s.io/kubernetes/pkg/scheduler/framework` 迁移到 `k8s.io/kube-scheduler/framework` · 且 `NodeInfo` 从 struct-pointer (`*NodeInfo`) 改为 interface (`NodeInfo`) · `CycleState` 同样改为 interface · 影响 9 个文件 (`hccs/{filter.go,score.go,plugin.go,filter_test.go,score_test.go}` + `binpack/{binpack.go,binpack_test.go}` + `numa/plugin.go` + `internal/integration/integration_test.go`)
> 4. **Plan §3 T003 Forbidden Paths**: "Source code outside `go.mod` / `go.sum` (any source change must be SEPARATE post-bump task — T102 NumaAffinity wrap is the explicit post-bump task in W2)"
> 5. **路径决策**:framework restructuring fixes 不属于 T102 NumaAffinity wrap scope (T102 仅 wrap upstream `noderesourcetopology.New(ctx, args, h)` + 3 sanity tests · 不含 framework type signature migration) · 是更广的 K8s 1.34 scheduler framework migration · 超出 T003 Forbidden Paths boundary · per P3 conservative + plan §3 default fallback → revert + 应用 doc-only refresh 路径
> 6. `git checkout origin/dev -- operators/scheduler-plugin/go.mod operators/scheduler-plugin/go.sum` + revert `kind-config.yaml` + revert `e2e-kind.yml` + restore `hccs/filter.go` import · `go build ./...` clean exit 0 · `go vet ./...` clean exit 0(scheduler-plugin v0.32.0 baseline 恢复)
>
> **Phase 9 影响**:
> - **T003 实际 outcome** = **doc-only refresh**(本 update segment + known-issues #12+#13 refresh + `docs/devlog/phase-9-t003.md`)· K8s baseline 维持 1.32 · scheduler-plugin go.mod 维持 v0.32.0 · 主模块(npu-dra-driver / inference-operator / pool-operator)go.mod 维持 v0.35.0(Phase 7 期间 drift · 不主动 downgrade)· backend go.mod 维持 v0.31.4(KubeEdge compat lock per ADR-0001 v3 §5)
> - **T102**(NumaAffinity wrap upgrade)→ auto-deferred Phase 10 · known-issues #12 maintained OPEN with framework restructuring note · prerequisite expanded from "baseline bump" → "baseline bump + framework migration"
> - **Phase 10 carry**:full K8s 1.34/1.35 baseline bump + framework API migration(NodeInfo/CycleState interface conversion across 9 files)+ NumaAffinity wrap upgrade · coordinated task chain · 需独立 2-3d scope outside Phase 9 W1 boundary · 详 `docs/devlog/phase-9-t003.md` migration matrix
>
> **下一次 baseline bump 评估时机**:Phase 10 W1 entry · 或 sched-plugins v0.35+/v0.36+ 发布时(re-eval whether v0.35+ relaxes framework restructuring · 现 v0.34.x 是 K8s 1.34 framework GA baseline · v0.35+ 可能进一步演进)。

> 🆕 **2026-05-21 update (P10-T-003 · 三件套 part 1 lands · option B per-module skew accepted)**:Phase 10 W1 entry re-WebFetch findings + per-module state survey:
> - upstream **sched-plugins v0.34.7** GA(2026-04-20 · 不变 vs Phase 9)— v0.35.x / v0.36.x 仍未发布
> - **kind v0.31.0**(2024-12-18 latest)ships kindest/node:**v1.35.0 / v1.34.3 / v1.33.7 / v1.32.11 / v1.31.14** — v1.34.3 是 1.34 cohort 最新 patch
> - **KEP-4815 Partitionable Devices** Beta in K8s 1.36 · GA not targeted yet · T202 cascade Phase 11+ regardless of 1.36 bump(详 `docs/adr/0016-lab-onboarding-and-phase-11-outlook.md` §3 Stream 6)
> - **唯一 feasible bump target = 1.34**(1.35 sched-plugins blocked · 1.36 sched-plugins + kindest 双 blocked · defer 不必要)
>
> **Per-module current K8s state**(verified at T003 entry · grep `k8s.io/api` 每 module go.mod):
> - operators/scheduler-plugin · `v0.32.0`(P9-T-003 revert · 唯一 laggard)
> - operators/o2-dms-adapter · `v0.36.1`(Phase 9 P9-T-008 new module · 自然 picks latest)
> - operators/{npu-dra-driver, inference-operator, pool-operator} · `v0.35.0`(Phase 7 期间 `go mod tidy` drift up · per Phase 8 P8-T-002 policy "不主动 downgrade 也不主动 upgrade")
> - operators/{node-lifecycle-operator, software-mgmt-operator, bare-metal-provisioning-operator} · 无直接 `k8s.io/api` dep(Phase 9 P9-T-105 scaffold · 仅 controller-runtime indirect)
> - exporters/ascend-npu-exporter-plus · 无直接 `k8s.io/api` dep
> - backend · `v0.31.4`(**KubeEdge v1.22 compat lock per ADR-0001 v3 §5** · K8s 1.31.x 是 KubeEdge primary edge-path constraint · 不动)
> - tests/e2e/kind kindest/node · `v1.32.0`(必须移)
>
> **用户决策**(2026-05-21 chat · Phase 10 W1 entry T003 path choice):**选项 B · 务实 lockstep runtime + 尊重既有 locks**(plan literal "lockstep no skew" 与 reality skew 矛盾 · 务实修正而非假装 clean · P3 verify-before-claim):
> - 真 bump = `scheduler-plugin/go.mod v0.32.0 → v0.34.7 cohort`(sched-plugins lockstep 必须)+ `kindest/node v1.32.0 → v1.34.3`(3 places)+ `e2e-kind.yml kind v0.25.0 → v0.31.0` + comment update
> - 已 drift 模块(v0.35.0 / v0.36.1)留 — client-go is backward-compat(对 K8s 1.34 runtime 仍 work)
> - backend `v0.31.4` 留 — KubeEdge v1.22 compat 是 hard requirement per ADR-0001 v3 §5
> - 3 IMS scaffold + exporter 不动(无 direct k8s.io/api dep)
>
> **执行过程 + 复盘**(2026-05-21 main agent · 同 P9-T-003 main agent 主动汇报模式):
> 1. 编辑 `scheduler-plugin/go.mod` v0.32.0 → v0.34.7(require + replace block 30 lines + indirect block v0.31.8 → v0.34.7 + comment refresh)· `kind-config.yaml` v1.32.0 → v1.34.3(replace_all · 3 places · control-plane + 2 worker)· `e2e-kind.yml` kind v0.25.0 → v0.31.0 + comment block update(DRA `resource.k8s.io/v1` GA in 1.34 note · Partitionable Devices Beta in 1.36 ADR-0016 §3 Stream 6 cross-ref)
> 2. `go mod tidy` clean exit 0(下载 K8s v0.34.7 cohort + transitive deps · indirect block 全 propagate)
> 3. **`go build ./operators/scheduler-plugin/...` FAIL**:同 P9-T-003 框架 drift error pattern · 15 errors across 9 files(`framework.Status` / `NewStatus` / `Error` / `StateKey` / `StateData` / `NodeInfo` interface vs `*NodeInfo` struct pointer)· **Phase 10 plan §3 T003 acceptance 明示**:"scheduler-plugin module may show stale due to framework API drift · expected · T004 owns fix-up" · T003 part 1 deliver scope = baseline bump ONLY,framework migration 是 T004 part 2 owner
> 4. **其他 modules `go build ./...` clean**:npu-dra-driver / inference-operator / pool-operator / o2-dms-adapter / backend 各 rc=0(Phase 10 T003 option B "不动" 模块 build status 与 T003 前一致 · 验证 client-go backward compat)
>
> **Phase 10 影响**:
> - **T003 outcome** = **baseline bump landed**(本 update segment + ADR-0001 v3 §5 per-module skew policy explicit + `docs/devlog/phase-10-t003.md`)· scheduler-plugin v0.32.0 → v0.34.7 · kindest/node v1.32 → v1.34.3 · 已 drift 模块不动 · backend KubeEdge lock 不动
> - **T004**(K8s baseline bump 三件套 part 2 · scheduler framework migration · 9 files NodeInfo+CycleState interface conversion)→ T003 entry 条件 met · 主 work start(per Phase 10 plan §3 T004 Allowed Paths)
> - **T005**(三件套 part 3 · NumaAffinity wrap upgrade body)→ depend on T004 完成
> - **T202**(Partitionable Devices Beta + partition-aware allocator)→ cascade decision unchanged · KEP-4815 Beta only in 1.36 · 1.34 bump 不 unlock T202 · Phase 11+ defer per ADR-0016 §3 Stream 6
>
> **下一次 baseline bump 评估时机**:Phase 11+ entry · 或 sched-plugins v0.35+/v0.36+ 发布时(re-eval 1.35/1.36 cohort 可行性)。known-issues #12 prerequisite met(K8s 1.34 baseline landed)· closer 是 T004 + T005 完成。

**部署形态**:**独立 kube-scheduler 二进制**(`bin/kube-scheduler` from
`operators/scheduler-plugin/cmd/main.go`),作为**第二 scheduler** 运行,通过
KubeSchedulerConfiguration 注册 profile `npu-scheduler`。**不修改 default-scheduler。**

Pod 显式 opt-in:`spec.schedulerName = npu-scheduler` 才走本插件链。inference-operator
deployment_builder(Phase 5 T007)落 Phase 6 时由 P6-T-105 / 后续 polish 加上
`spec.schedulerName` 设置;未设置的 Pod 走默认 scheduler 不受影响。

**为什么不 patch default-scheduler**:
- 部署兼容性 — 多 scheduler 模式是 K8s 一等公民(kube-scheduler.config.k8s.io/v1
  KubeSchedulerConfiguration profiles[] 原生支持)。
- 灰度路径 — 失效只影响 opt-in 的 ModelService Pod,不影响集群其他工作负载。
- 升级路径 — 上游 sched-plugins 版本升级 = 升级本 binary,不动 default-scheduler。

### 2. HCCSTopologyPlugin Filter + Score 语义

**Filter**(P6-T-004):

| 输入 | 决策 |
|---|---|
| Pod 无 `npu.huawei.com/preferred-hccs-ring` 注释 + Args.FailIfMissing=false (default) | 返回 Success (permissive — 不过滤) |
| Pod 无该注释 + Args.FailIfMissing=true | 返回 UnschedulableAndUnresolvable reason="HCCS ring annotation required" |
| Pod 带注释 = "0,1"(逗号分隔 int list) + 节点至少一个 ResourceSlice 有 device 属性 `npu.huawei.com/hccs_ring ∈ {0,1}` AND device `Available >= request.cores` AND health ∉ {Unhealthy, Unknown} | 返回 Success |
| Pod 带注释 + 节点无任何 device 满足上述 | 返回 UnschedulableAndUnresolvable reason="no HCCS ring N on this node" |

数据来源:scheduler 框架的 `SharedInformerFactory` cache(标准 plugin 约定 — 不直接
call API server);ResourceSlice GVK `resource.k8s.io/v1beta1` 的 listers 由
scheduler-plugins 框架启动时注册。

**Score**(P6-T-005):

| 输入 | 评分 |
|---|---|
| Pod 无 `inference.ocloud.edge.example.com/model-service` label | 50(中性 — 不偏好) |
| Pod 有该 label · 兄弟 Pod(同 label value · 已 Allocated 的 NPUSliceAllocation)的 ring 集合 P + 当前节点至少一 device 在 P 内 | 100 |
| 同上 · 当前节点 device 在 adjacency map 定义的相邻 ring(Args 可配 · default 空 map = 无 adjacency) | 70 |
| 同上 · 当前节点 device 仅在 disjoint ring | 30 |
| 当前节点无候选 device | 0 |

`NormalizeScore` 显式不实现(`framework.ScoreExtensions` 接口不实现) — framework
自动 normalize 到 [0..100]。

**为什么 Filter 默认 permissive**:
- inference-operator Phase 5 不写 `preferred-hccs-ring` 注释;Phase 6 期间逐步 enable
  (P6-T-105 同时考虑 inference-operator 是否 stamp 这个 annotation;若不 stamp,
  permissive Filter 让 Pod 仍能落到任何节点,Score 仍可做亲和优化)。
- 集群运维可在 Args 切到 FailIfMissing=true 强制契约。

**为什么 Score 用兄弟 Pod 而非 ModelService Spec**:
- 兄弟 Pod 通过 NPUSliceAllocation 反查("已分配在哪")是 ground truth · 比读 spec("将来想分配")更稳。
- 第一个 Pod 落地后,后续兄弟自动向其聚拢;无需 ModelService 提前 declare ring 偏好。

### 3. NumaAffinityPlugin reuse(P6-T-006)

直接复用上游 `sigs.k8s.io/scheduler-plugins/pkg/noderesourcetopology` 的 Filter + Score
实现,本仓库只:
- 在 `operators/scheduler-plugin/internal/plugins/numa/` 提供 thin wrapper(import +
  re-register with local plugin name)。
- 重命名 plugin 注册名为 `NumaAffinity`(chart 可读性),不重命名底层包。

**为什么 wrap 不 fork**:
- 上游已成熟(NodeResourceTopology Cache + TopologyManager 策略集成均完整)。
- 本仓库 NUMA 决策无任何 Ascend 特有逻辑(NUMA 是 host-level 概念,与 NPU 厂商无关)。
- Fork 增加同步维护成本,wrap 不增加。

**默认 Weight**:2(per arch §5.6 + cni-hccl-research §4 间接 — NUMA 在 vllm-ascend
PD 场景下次于 HCCS,因 HCCL 通信带宽是主要瓶颈;NUMA cache locality 次之)。

**T006 落地状态(2026-05-20 P6 update)**:placeholder · upstream wrap deferred。
直接 `nrt.New(...)` 委托在 P6-T-006 entry 时构建失败 — 上游
`sigs.k8s.io/scheduler-plugins/pkg/noderesourcetopology` v0.31.8 引用
`framework.GVK` 符号,K8s 1.31 `pkg/scheduler/framework` 包内有但 K8s
1.32(本仓库 go.mod replace block baseline · ADR §1)已移除。等 sched-
plugins v0.32.x 发布(或 v0.31.y backport)再补 wrap。详 DESIGN.md §5.2 +
phase-6-t006 devlog。

**P7-T-002 升级尝试结果(2026-05-20)**:**doc-only fallback** · 重新 defer to
Phase 8 baseline bump · 详 DESIGN.md §5.2 + phase-7-t002 devlog + 新
known-issues #12。verified GA matrix:v0.32.7(2024-08-06) / v0.33.5
(2024-10-27) / v0.34.7(2025-04-20 latest)。直接 `go get
sigs.k8s.io/scheduler-plugins@v0.32.7` + 把 replace block uniformly bump
到 v0.32.7 后 → `go mod tidy` surface k8s.io/apimachinery v0.32.7 缺
`pkg/api/{safe,operation,validate}` packages 沿 import chain
`cmd/main.go → kube-scheduler/app → pkg/scheduler → pkg/apis/core/{validation,v1}`。
这些 apimachinery packages 是 post-v0.32 添加(估 v1.33+),本 ADR §1
K8s 1.32 baseline pin 无法吸收 transitive expectation。Revert clean(单
`git checkout go.mod go.sum`),无 commit 走升级路径。Phase 8 候选:bump
K8s baseline 1.32 → 1.33 / 1.34(同时升 kind smoke `kindest/node` 基线)
+ sched-plugins 同 minor + 补 wrap body + 3 sanity tests + chart toggle
flip default。

### 4. BinpackPlugin opt-in(P6-T-007)

**实现选择**:内部 thin impl(50 LOC)Score-only · 不引入 volcano dep。

**Score formula**:
```
score = sum over r in requested resources:
    resourceWeight[r] * requested[r] / allocatable[r]
```

normalized 到 [0..100] via framework。

**默认 ResourceWeights**:
```
{
  cpu: 1,
  memory: 1,
  npu.ocloud.edge.example.com/devices: 5,
}
```

NPU 权重高 — binpack 倾向把 Pod 集中到已 load NPU 的节点 · 释放完全空闲节点 · 利好
Phase 8 vertical scaling(缩节点更容易)。

**默认 Enabled=false**:
- Phase 6 入口先观察 NodeResourcesFit baseline + HCCS Score 共同作用下的分布。
- 集群运维通过 `KubeSchedulerConfiguration.profiles[*].plugins.score.enabled[]` +
  `pluginConfig` Args.Enabled=true 主动启用。

### 5. Plugin args schema 与 ResourceSlice attribute 消费契约

**Args schema**(`+k8s:deepcopy-gen` + `RegisterPluginArgs`):

```go
type HCCSTopologyArgs struct {
    metav1.TypeMeta  `json:",inline"`
    Weight           int32           `json:"weight,omitempty"`            // default 5 · validated [0..100]
    PreferAnnotation string          `json:"preferAnnotation,omitempty"`  // default "npu.huawei.com/preferred-hccs-ring"
    FailIfMissing    bool            `json:"failIfMissing,omitempty"`     // default false
    Adjacency        map[string][]int32 `json:"adjacency,omitempty"`      // ring-id -> adjacent ring-ids; default {} (no adjacency)
}

type NumaAffinityArgs struct {
    metav1.TypeMeta `json:",inline"`
    Weight          int32 `json:"weight,omitempty"`   // default 2
}

type BinpackArgs struct {
    metav1.TypeMeta `json:",inline"`
    Weight          int32             `json:"weight,omitempty"`           // default 1
    Enabled         bool              `json:"enabled,omitempty"`          // default false
    ResourceWeights map[string]int64  `json:"resourceWeights,omitempty"`  // default {cpu:1, memory:1, npu.ocloud.edge.example.com/devices:5}
}
```

**ResourceSlice attribute contract**(必须与 npu-dra-driver/api/v1alpha1
`AttrHCCSRing` / `AttrNUMANode` 常量同步,Filter/Score 读以下:

| Attribute QualifiedName | 类型 | 数据源 | 消费者 |
|---|---|---|---|
| `npu.huawei.com/hccs_ring` | int(`IntValue`) | npu-dra-driver publisher(P4-T-005 + P5-T-003 sim source); Phase 7 改 npu-smi 真硬件查询 | HCCSTopology Filter + Score |
| `npu.huawei.com/numa_node` | int(`IntValue`) | npu-dra-driver publisher | (passthrough — NodeResourceTopology CR 是主索引,这里冗余可观察) |
| `npu.huawei.com/health` | string(`StringValue` ∈ {Healthy, Degraded, Unhealthy, Unknown}) | npu-dra-driver publisher · mock JSON `status` 字段映射 | HCCSTopology Filter — Unhealthy/Unknown skip |
| `npu.huawei.com/ai-cores` | int (Capacity) | npu-dra-driver publisher | (passthrough — Filter 比 `Available >= request.cores`) |

**Pod 注释契约**:
- `npu.huawei.com/preferred-hccs-ring`:可选,逗号分隔的 int list(如 `"0,1"`)。
  HCCSTopology Filter 用作硬过滤(per Args.FailIfMissing);Score 不读此注释(只读
  NPUSliceAllocation 反查兄弟 Pod)。
- `inference.ocloud.edge.example.com/model-service`:Phase 5 PD Router objectSelector
  匹配键 · 同时是 Score 计算"兄弟 Pod"的分组键。Value 格式 `<ns>/<name>`(per
  inference-operator/DESIGN.md §4.2.2)。
- `npu.huawei.com/slice-bindings`:Phase 5 PD Router 写出 · 本 plugin 不读 · 仅作为
  调度结果观察(kind smoke + 前端 Workloads 页消费)。

### 6. Phase 6 CNI 选型

**结论**:**Cilium + Multus + SR-IOV** 为 Phase 6 + Phase 7 推荐底座 · Calico + Multus +
SR-IOV 为 production fallback。

**理据**(per `docs/cni-hccl-research.md` §4 + §2 matrix):

| 维度 | Cilium 1.16+ | Calico 3.28+ |
|---|---|---|
| 主网(Pod-to-Pod overlay) | eBPF native · sidesteps overlay 封装问题 | BGP / VXLAN — VXLAN 模式无法承载 RoCE PFC |
| HCCL 副网(secondary nic via Multus) | 支持 — eBPF dataplane bypass | 支持 — 标准 Multus 接入,battle-tested |
| Topology-aware scheduling 集成 | Cilium 1.16 publish NUMA topology metadata | 需自行实现 NUMA scoring |
| RoCE v2 dataplane bypass | 1.16 announcement(Confidence C · derived per cni-hccl-research §2.1)· GA target 1.18 | 不原生支持 RDMA |
| 生产成熟度 | 中(大规模 telco RDMA 部署仍少) | 高(Calico 是 K8s networking 行业标杆) |

**Phase 6 chart 默认值**:`schedulerPlugin.cniHints.preferredOverlay=cilium`(用作
documentation hint;chart 不强制 install CNI · 运维自带)。运维若用 Calico,只需在
README 标 fallback 链接。

**CNI-portable 设计**(关键):scheduler-plugin **不依赖任何 CNI 特有 API**。所有
topology 元数据来自:
- ResourceSlice attributes(npu-dra-driver publishes,与 CNI 无关)
- NodeResourceTopology CR(NodeResourceTopology Operator publishes,与 CNI 无关)
- Pod annotations(inference-operator 写,与 CNI 无关)

CNI 选型只影响 HCCL 数据面是否能跑通(secondary nic 路径)· 不影响 plugin
调度逻辑能不能跑。

### 7. Phase 7+ forward notes

> 🆕 **2026-05-20 update (P7-T-001 / ADR-0011)**:`Source.RealAscend.queryTopology()`(下表第 1 行)实际落地分两步:**P7-T-004** ship Source 接口 + RealAscendSource stub(返回 ErrNotImplemented)+ factory 选择;**P7-T-101 lab-conditional** 在 W2 entry meeting 用户 signal lab access available 时 light up 真实现(npu-smi 解析 + DCMI health poll)。缺省 = defer to Phase 10 — 详 **ADR-0011 §3 Lab gating 政策**。同时本节下表第 2 行(HCCS Adjacency map 经验数据缺失 · §256 risk row)对应的 Phase 7 fix 是 **P7-T-008**:chart values 默认 910B 8 卡 ring-of-rings adjacency,运行时通过 Args 可覆盖。

> 🆕 **2026-05-21 update (P8-T-001 / ADR-0012 · Phase 8 busy-idle reconcile loop)**:Phase 8 引入 **NPUVerticalScaler CRD**(co-located with ModelService in inference-operator binary · 详 **ADR-0012 §"NPUVerticalScaler CRD shape"**)。NPUVerticalScaler 不参与本 scheduler-plugin 的 Filter/Score 决策 — 它 patch `ModelService.spec.template.sliceTemplate` 字段触发 rolling restart;rebuilt Pod 仍走本 plugin profile(`schedulerName=npu-scheduler` per P7-T-003 auto-stamp)+ T008 wiring 标记 `npu.huawei.com/preferred-hccs-ring=<ringID>` annotation。Phase 8 T104-v2 把 Phase 7 T104 的 HCCS placement soft-warning(synthetic ring fixture)升级为 hard-fail(读 T008 wiring stamp 的 annotation)— 详 ADR-0012 §"Scaling mutation model"。本 plugin 端**无代码变化**;Phase 8 baseline bump(T002 1.32→1.36+)同时 unblocks NumaAffinity wrap upgrade(下表 §3 NumaAffinity row · known-issues #12 closer)。

| 演进项 | 触发 | 切换路径 |
|---|---|---|
| 真硬件 HCCS ring 发现 | Phase 7 实机 + CANN driver ≥ 24.x | 替换 npu-dra-driver `Source.RealAscend.queryTopology()` · 走 `npu-smi info -t topo` 解析 HCCS group 拓扑;ResourceSlice attribute 写法不变 |
| Partitionable Devices(KEP-4815 GA · K8s 1.37 est.) | npu-dra-driver Phase 7 升级 · per ADR-0009 §4 | 每个 NPU emit N 个 partition entries · 各自带 `hccs_ring` 属性;Filter/Score 不变,只是粒度更细 |
| 同一 ModelService Pod 跨节点 HCCL 通信 | Phase 7 多机训练 / 推理需求 | 引入 Cilium + Mellanox CX-7 SR-IOV;scheduler-plugin Score 加入 "同 NIC 链路 / 同 leaf switch" 维度(读 ADR-0007 fabric discovery 产出的 Node label) |
| Per-Pod RDMA bandwidth quota | Phase 9 multi-tenancy | 引入 SR-IOV VF partitioning;quota controller 读 NPUSliceAllocation by namespace 累加 |
| Volcano gang-scheduling 整合 | Phase 8+ 训练大批量 job 时 | 评估 Volcano PodGroup CRD 集成本插件;**当前 Phase 6 不引入** — 推理场景 PD-pair 2-4 个 Pod 不需要 gang。**2026-05-21 P8-T-106 spike landed**:`docs/research/volcano-gang-scheduling-spike.md` — 推荐 Phase 9 W1 entry 引入 Volcano binary(独立 helm · 1-2d 工作量 · 路径 A)· 训练 job 走 `schedulerName=volcano` + PodGroup atomicity · npu-scheduler 继续 own NPU device 级 Filter+Score · 解耦清晰。**2026-05-21 P9-T-101 deferred(doc-only path)**:Phase 9 W1 entry + W2 entry chat 均**无** training-job demo 正面 signal · 用户 "继续" 未明示 gang 需求 · per plan §4 P9-T-101 default policy "Deferred unless positive signal at W1 entry chat" · T101 走 doc-only deferred 路径 · Phase 10+ carry if training-job demo 实质化时 mid-phase 评估。**Re-WebFetch outcome 2026-05-21**:Volcano upstream stable v1.10.x line 保持 K8s 1.32 兼容(本 phase scheduler-plugin 维持 K8s 1.32 baseline per T003 doc-only refresh outcome)· `helm install` 路径 unchanged 若 mid-phase 重启 light up。 |

---

## 后果

### 正面

- **Phase 5 ResourceSlice attribute 公开数据立即变现** — `hccs_ring` / `numa_node`
  从"placeholder integer 字段"升级为"调度决策实际读取的契约"。
- **多 scheduler 模式不破坏现有工作负载** — 默认 scheduler 行为不变,opt-in 才走本插件;
  运维零风险 trial。
- **CNI-portable** — 不绑 Cilium,运维生态选择空间大;chart 默认值是 hint 不是硬依赖。
- **Phase 7 升级路径平滑** — Partitionable Devices GA 后 publisher 改 emit 粒度即可,
  plugin 逻辑不动。
- **inference-operator + scheduler-plugin 解耦** — inference-operator 不感知 plugin 存在,
  只 stamp 标准 K8s 注释;plugin 不感知 ModelService schema,只读 Pod label。

### 负面 / 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| sched-plugins 框架 API 在 v0.30→v0.31 已有 breaking change | Phase 6 后任何 plugin framework bump 都要 review wrap 层 | T002 pin specific release tag · Makefile 锁定;升级走专项 task 不混进其他 phase |
| 多 scheduler 运维复杂度 + leader election 配置错误 | 调度卡住 / 双调度抢资源 | T101 chart 默认 leader election 在 `kube-system` namespace · known-issues #11 documents schedulerName 必要 · 监控 metric `scheduler_pending_pods` 异常告警 |
| Pod 不显式设置 schedulerName 时 plugin 失活 | 集群运维忘了配置 → 拓扑感知功能不生效但无错误 | inference-operator deployment_builder Phase 6 polish 主动 stamp schedulerName(P6 后续 polish · 可能 P6-T-105 中);kind smoke T106 显式验证 schedulerName 字段 |
| HCCSTopology Score Adjacency map 经验数据缺失 | 默认空 map → Score 表现退化为"同 ring 100 · 其他 30"二分 | Phase 6 ship empty default · **Phase 7 P7-T-008 落 910B 8 卡机的 ring-of-rings `0↔1↔2↔3↔0` chart 默认** + `DefaultAdjacency910B8Card()` + `BuildAdjacency()` pre-flight validator(`internal/plugins/hccs/adjacency.go`) · 4 单元测试 + 2 score 集成测试覆盖 · Phase 10 实机验证后再 fine-tune chart 默认或文档化"如何按真硬件 override"|
| NPUSliceAllocation reverse-lookup 性能 (Score 每节点 list 一次) | 大集群 Score 阶段延迟 | Phase 6 单集群 ≤ 100 节点 × 8 NPU 量级,list 廉价;Phase 9 多集群引入 indexer cache · 单 list query 不超过 1ms |
| FailIfMissing=true 误配导致集群全卡 | 所有 Pod 都被 filter 掉 | Args 默认 false · README 强警告; e2e-kind smoke 含 strict + permissive 双路径 |
| Cilium 1.16+ RoCE 支持仍 experimental(per cni-hccl-research §2.1 confidence C) | Phase 7 实机部署可能 fallback Calico + Multus | ADR-0010 §6 已明记 fallback 路径 · Phase 7 入口实测后 ADR 增补 v2 if needed |

---

## 推翻条件

| 决策项 | 推翻条件 |
|---|---|
| sched-plugins 框架选型 | 上游(SIG-scheduling)弃用 framework v1 → 切上游替代品 |
| 独立 kube-scheduler 部署形态 | K8s 引入 plugin-as-extension(KEP-XXXX)替代多 scheduler 模式 → 单 scheduler + extension binary |
| HCCSTopology 默认 permissive Filter | 实机 trial 显示生产环境 ≥ 5% Pod 落错 ring 引起 HCCL 性能掉坑 → 切 FailIfMissing=true 默认 + inference-operator 强制 stamp 注释 |
| Cilium 优先 | Calico 在样机部署中证明 Multus + SR-IOV 链路 ≥ Cilium 性能且配置更稳 → 切 Calico 优先 |
| Binpack 默认 disabled | Phase 8 vertical scaling 需要 binpack 才能腾空闲节点 → 默认 enabled 同时上调 NPU 权重 |
| NumaAffinity wrap 上游 | 上游 noderesourcetopology Cache 实现出现 ≥ P0 bug 且 upstream 修复 lag > 3 月 → 临时 fork patch |

---

## 引用

- `docs/adr/0001-phase0-key-decisions.md` v3 §5 — 双轨路径 + Standard-K8s 上 DRA spike
- `docs/adr/0007-fabric-discovery.md` — fabric / leaf-switch topology discovery(Phase 7+ scheduler-plugin secondary input)
- `docs/adr/0008-pd-router-webhook.md` — `npu.huawei.com/slice-bindings` annotation source · model-service label objectSelector
- `docs/adr/0009-npu-dra-driver.md` §2(slice ↔ ResourceClaim 表)§3(KubeEdge gap)§4(Partitionable Devices forward)§5(Phase 5 实施)§6(Allocator 算法)§6.2 — topology-aware scoring forward note 是 Phase 6 入口
- `docs/architecture.md` §5.6(scheduler-plugin 模块)§6.3(HCCSTopologyInfo struct)§13(Phase 5+ NPU pod 网络 + Phase 6 HCCS 调度行)
- `docs/cni-hccl-research.md` §2 matrix + §4 entry recommendation + §5 known gaps
- `docs/checkpoint-phase5.md` §7 — Phase 6 handoff brief(scheduler-plugin scaffold + NUMA 推荐 first session)
- `docs/phase6-plan.md` §3 P6-T-001..T008 / §6 risks
- upstream:`sigs.k8s.io/scheduler-plugins` v0.31.x — plugin framework
- upstream:`sigs.k8s.io/scheduler-plugins/pkg/noderesourcetopology` — NUMA plugin to wrap
- upstream:KEP-4815 Partitionable Devices(K8s 1.37 est. GA · Phase 7 入口 gate)

---

**END of ADR-0010**
