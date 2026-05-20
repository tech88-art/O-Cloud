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

**T006 落地状态(2026-05-20 update)**:placeholder · upstream wrap deferred。
直接 `nrt.New(...)` 委托在 P6-T-006 entry 时构建失败 — 上游
`sigs.k8s.io/scheduler-plugins/pkg/noderesourcetopology` v0.31.8 引用
`framework.GVK` 符号,K8s 1.31 `pkg/scheduler/framework` 包内有但 K8s
1.32(本仓库 go.mod replace block baseline · ADR §1)已移除。等 sched-
plugins v0.32.x 发布(或 v0.31.y backport)再补 wrap。详 DESIGN.md §5.2 +
phase-6-t006 devlog。

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

| 演进项 | 触发 | 切换路径 |
|---|---|---|
| 真硬件 HCCS ring 发现 | Phase 7 实机 + CANN driver ≥ 24.x | 替换 npu-dra-driver `Source.RealAscend.queryTopology()` · 走 `npu-smi info -t topo` 解析 HCCS group 拓扑;ResourceSlice attribute 写法不变 |
| Partitionable Devices(KEP-4815 GA · K8s 1.37 est.) | npu-dra-driver Phase 7 升级 · per ADR-0009 §4 | 每个 NPU emit N 个 partition entries · 各自带 `hccs_ring` 属性;Filter/Score 不变,只是粒度更细 |
| 同一 ModelService Pod 跨节点 HCCL 通信 | Phase 7 多机训练 / 推理需求 | 引入 Cilium + Mellanox CX-7 SR-IOV;scheduler-plugin Score 加入 "同 NIC 链路 / 同 leaf switch" 维度(读 ADR-0007 fabric discovery 产出的 Node label) |
| Per-Pod RDMA bandwidth quota | Phase 9 multi-tenancy | 引入 SR-IOV VF partitioning;quota controller 读 NPUSliceAllocation by namespace 累加 |
| Volcano gang-scheduling 整合 | Phase 8+ 训练大批量 job 时 | 评估 Volcano PodGroup CRD 集成本插件;**当前 Phase 6 不引入** — 推理场景 PD-pair 2-4 个 Pod 不需要 gang |

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
| HCCSTopology Score Adjacency map 经验数据缺失 | 默认空 map → Score 表现退化为"同 ring 100 · 其他 30"二分 | Phase 6 ship empty default · Phase 7 实机验证后 chart values 加默认 adjacency(如 910B 8 卡机的环形拓扑 0↔1↔2↔3↔0) |
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
