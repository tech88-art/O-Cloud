# ADR-0009: npu-dra-driver design — slice ↔ ResourceClaim semantic mapping + KubeEdge gap

- **状态**:Accepted (design + scaffold landed; allocation logic Phase 5)(2026-05-19 — Phase 4 P4-T-105)
- **日期**:2026-05-19
- **决策者**:协调者(用户)
- **相关**:ADR-0001 v3(双轨路径 · 2026-05-19)/ ADR-0007(fabric discovery)/ ADR-0008(PD Router webhook · Phase 5 consumer of claim allocation)/ phase4-plan.md §3 P4-T-105 / architecture.md §3.4 (NPU/AI 运行时) / §5.5 (npu-dra-driver 模块) / §6.7 (Multi-tenancy) / §13 (review table)

---

## 上下文

Phase 4 W1(P4-T-003..006)+ W2 P4-T-101 落地了一个 **simulator-first** 的自研 NPU DRA driver:

```
operators/npu-dra-driver/
├── api/v1alpha1/              AscendDevice + AscendClaimAnnotations 类型(T004)
├── internal/publisher/        SimulatorSource + Publisher reconcile loop(T005)
├── internal/controller/       ClaimReconciler skeleton(T006 · AllocationDeferred annotations)
└── deploy/helm-charts/npu-dra-driver/ (P4-T-101)
```

整体接 ADR-0001 v3 §5 双轨路径:
- **Edge 路径(KubeEdge)** → Ascend Device Plugin v1(Phase 3 不变);DRA 升级 gated on KubeEdge DRA readiness。
- **Standard-K8s small-cluster 路径** → 本 driver 是 Phase 4 spike → real allocation Phase 5+ + Partitionable Devices Phase 7。

本 ADR 锁定:Phase 4 已落地的 slice ↔ ResourceClaim 语义映射、KubeEdge gap 的应对、Partitionable Devices forward note、Phase 5 实施要点 — 让 Phase 5 入口直接照做不需再设计。

---

## 决策

### 1. 自研 driver(非 vendor 路径)

**承袭 ADR-0001 §7**:不引入 MindCluster / MindX DL(黑盒,不支持自研动态切分),自研 fork-spirit from `kubernetes-sigs/dra-example-driver v0.2.1`(2026-01-09)+ 适配 Ocloud Ascend 语义。

**代价**:开发量增加(Phase 4 / Phase 7 范围);**收益**:可控可演进,能表达 Ascend 910B 特有的 NPU 切分 / NUMA / HCCS 拓扑 / Phase 7 Partitionable Devices 升级路径。

### 2. slice ↔ ResourceClaim 语义映射(operative 表)

下表是 Phase 5+ inference-operator + scheduler-plugin 必读的核心契约。**Phase 5 allocator 实现就照这张表读 / 写。**

| Ocloud 概念 | K8s DRA(v1beta1) 对应 | 来源 | 备注 |
|---|---|---|---|
| **NPUSlicePool** (Phase 3 CRD `ims.ocloud.edge.example.com/v1alpha1`) | `resource.k8s.io/v1beta1.DeviceClass` | pool-operator + npu-dra-driver | NPUSlicePool 描述"哪些设备 + 切分策略"; DeviceClass 是 cluster-scoped admission selector(Phase 5 npu-dra-driver 注册一个 DeviceClass = `npu.ocloud.edge.example.com` + 可选 sub-classes `/whole`、`.dynamic`)|
| **NPU 物理设备**(Ascend 910B 一块芯片) | `ResourceSlice.spec.devices[i]` (一个 `Device` 条目) | npu-dra-driver publisher (T005) | 每个 Device.Basic.Attributes 携带 `npu.huawei.com/{index,health,slice-strategy,ai-cores,numa-node,hccs-ring}`;Capacity 上 `npu.huawei.com/slice-aicore` 表达"该设备可切多少 ai-core"|
| **NPU slice**(逻辑切片 · whole / vir04 / vir08 / dynamic) | Phase 4: 单个 Device 表示 whole NPU(粒度 = 设备级)<br>Phase 7: Partitionable Devices(KEP-4815)GA 后,一个 NPU = N 个 partition entries(粒度 = slice 级) | npu-dra-driver publisher + scheduler-plugin | Phase 4 simulator 不切;Phase 5 仍按 NPU 级别 publish,allocator 在 Reconcile 时分配整 NPU 给一个 claim 或 deny(简化版);Phase 7 改 Partitionable Devices,publish 每个 slice 单独 entry |
| **slice 分配请求**(inference-operator ModelService 创建 PD-pair 时) | `resource.k8s.io/v1beta1.ResourceClaim`(per Pod / per side of PD-pair) | inference-operator + npu-dra-driver | Phase 4: claim 创建后 npu-dra-driver 仅 `AllocationDeferred=Phase4Skeleton` annotation(T006);Phase 5: 实际算法选 device + 写 `status.devices[]` |
| **Ocloud 业务绑定语义**(model-service / preferred-pool) | `ResourceClaim.metadata.annotations[ocloud.edge.example.com/{model-service-ref,preferred-pool}]` | inference-operator(Phase 5) | T004 ships `AscendClaimAnnotations` 类型 + `ocloud.edge.example.com/model-service-ref` 与 `/preferred-pool` 两个 annotation key 常量 |
| **PD-pair 角色**(Prefill / Decode) | Pod label `inference.ocloud.edge.example.com/pd-role` = `prefill` / `decode` + ResourceClaim annotation `ocloud.edge.example.com/model-service-ref` | Phase 5 PD Router webhook(ADR-0008) | webhook 读 ModelService.spec.pdPair.routerLabel,把对应 label 注入 Pod;allocator 从 annotation 反查 ModelService 决定亲和组 |

**关键不变量**:driver name 全栈唯一固定为 `npu.ocloud.edge.example.com`(`operators/npu-dra-driver/api/v1alpha1.DriverName` const)。`ResourceSlice.spec.driver` + `DeviceClass` 名前缀 + claim 控制器 prefix 过滤都用这个串。Phase 4 T102 pool-operator NPUSlicePool Reconcile 也按这个串 filter ResourceSlices 计入 `status.resourceSlicesObserved`。

**Phase 9 Quota admission cross-ref(2026-05-21 · ADR-0014 / P9-T-002 + fix-001 group correction)**:`NPUSliceAllocation` create(Phase 5 P5-T-004/T005 落地 · 实际 group `npu.ocloud.edge.example.com/v1alpha1` per 本 §2 表 + types.go +groupName)在 Phase 9 起被 Quota Webhook A 拦截 — `inference.ocloud.edge.example.com/v1alpha1.Quota` namespace-scoped CRD(同 inference-operator binary scheme · P9-T-002-fix-001 group correction)`spec.enforcement.maxSliceAllocations int32` 限定该 namespace 同时存在 `NPUSliceAllocation` 上限 · Webhook A `apiGroups: ["npu.ocloud.edge.example.com"]` rule match NPUSliceAllocation create · failurePolicy=Fail · matchPolicy=Equivalent · admission 拒绝时 claim_controller 内部 reconcile 走 backoff 路径(不卡 reconcile · Pod scheduling backoff)· 详 ADR-0014 §2 Decision C Webhook A + §3 risk row "NPUSliceAllocation create 路径 是 claim_controller owner-ref 创建"。Phase 11+ 候选 claim_controller pre-check Quota(避免 admission round-trip)走 ADR-0014 §7 forward note。

### 3. KubeEdge gap(承袭 ADR-0001 v3 §5)

**事实**:KubeEdge v1.22 release notes(2026-04-12 latest)不提 `resource.k8s.io`,依赖 K8s 1.31.x — DRA v1beta1 在 K8s 1.31 已 alpha 可启,但 KubeEdge 上游**未发布 edgecore DRA 适配**。

**应对**(operative):
1. **Edge 节点(KubeEdge)**:继续用 Ascend Device Plugin v1。npu-dra-driver 不部署到 edge 节点;helm chart 的 nodeSelector 由 deploy-time 决定(Phase 5+ 用 `node-role.kubernetes.io/control-plane` 或显式标 `ocloud.edge.example.com/dra-eligible=true`)。
2. **Standard-K8s small-cluster(KubeEdge-free)节点**:npu-dra-driver 在此运行,负责 publish + allocate。
3. **回退判据**:KubeEdge 上游 6 个月内仍无 DRA 支持(2026-05 起算 → 2026-11)→ 边缘永久 stay Device Plugin v1;否则等 KubeEdge DRA 落地后再统一双轨。

**Phase 5 不允许的事**:不能在 KubeEdge 节点上启用本 driver。helm chart 的 `nodeSelector` 必须显式排除 KubeEdge edgecore 节点。

### 4. Partitionable Devices forward note(Phase 7)

> 🆕 **2026-05-20 update (P7-T-001 / ADR-0011)**:Phase 7 introduces a `Source` Go interface (`internal/source/source.go`) abstracting where the publisher gets device inventory + topology. MockJSONSource preserves Phase 4-6 behavior bit-for-bit; RealAscendSource stub ships W1 (P7-T-004), lit up by lab-conditional P7-T-101 on real 910B silicon. The `Source.QueryTopology()` method (referenced by ADR-0010 §7 forward note as `Source.RealAscend.queryTopology()`) lives here. Cross-reference: **ADR-0011 §2 Source interface**. Source abstraction does NOT supersede the Partitionable Devices forward note below — both layers coexist (Source = where inventory comes from; Partitionable Devices = how each device decomposes into partitions).

**事实**(refreshed 2026-05-20 P7-T-106):KEP-4815(Partitionable Devices)在 K8s 1.35 Alpha confirmed · K8s 1.36 Beta confirmed("Tracked for Docs Freeze" complete · `stage/beta` label set)· **GA timing unconfirmed**(ADR-0009 v1 "est. K8s 1.37 GA" 估计未在 upstream tracker 上 commit · 应作 "1.37 minimum, possibly 1.38+")。详 `docs/research/k8s-partitionable-devices-spike.md` §1。

**升级路径**(Phase 7):
1. **当前(Phase 4-6)**:每个 NPU 发布为一个 `Device` 条目,Capacity 上 `slice-aicore=<chip total>`;allocator 按设备级粒度分配(整 NPU 给一个 claim 或 deny)。
2. **Phase 7(K8s ≥ 1.37 GA)**:每个 NPU 发布为 N 个 partition entries(`Device` + parent reference),partition entries 表示真正的 slice(vir04 / vir08 / dynamic AI-core 配额);allocator 按 partition 级粒度分配,多个 claim 可以共享同一 NPU 的不同 partition。

切换条件:
- K8s upstream GA Partitionable Devices(Phase 7 入口 gate)
- Ascend 厂商 npu-smi / DCMI 暴露真切分能力(arch §13 Phase 7 risk + ADR-0001 §"Phase 7 动态切分若 fallback")

切换不破坏 API:`AscendDevice` 类型(T004)的字段不变;publisher 内部多 emit partition entry 即可。Claim 消费者(inference-operator)不感知。

> 🆕 **2026-05-20 update (P7-T-001 / ADR-0011 §1)**:在 Partitionable Devices GA(K8s 1.37 est.)之前,Phase 7 通过 **NPUSliceTemplate CRD + 多模板组合 fallback** 提供动态切分能力。`NPUSliceTemplate.spec.composition` 表达用户期望切分,template engine 把 composition 拆解为现存固定模板(vir04/vir08/vir16/whole)的 bundle,allocator 按 bundle 多次分配。Pod opt-in via label `npu.huawei.com/slice-template=<name>`;absent → 走本 ADR §5 + §6 既有 whole-NPU 路径(零回归)。详 **ADR-0011 §1 NPU 动态切分** + §4 NPUSliceTemplate CRD schema。Phase 7 fallback 路径与 Partitionable Devices(本节)长期共存:driver-layer 突破或 KEP-4815 GA 后,fallback 标 deprecated 但 backward-compat。

> 🆕 **2026-05-21 update (P8-T-001 / ADR-0012 · Phase 8 partition path)**:Phase 8 引入 **NPUVerticalScaler CRD**(详 **ADR-0012 §4 CRD schema**)消费本 ADR §4 NPUSliceTemplate ref + AllocateBundle 控制器 wiring(P8-T-008 · 本 ADR §6 Allocator 升级)。Phase 8 W2 BETA-GATED 路径(T101+T102)若 light up Partitionable Devices Beta(K8s 1.36),partition-aware allocator(本 ADR §4 升级路径第 2 行)与 NPUVerticalScaler 协同 — NPUVerticalScaler patch ModelService.spec.template.sliceTemplate → claim_controller 按 label 解析 → AllocateBundle 内部选 partition 或 whole-NPU candidate(per `docs/research/k8s-partitionable-devices-spike.md` §5 sibling preference)。Beta 路径 default = doc-only refresh(ADR-0011 §3 spirit · spike §1 GA timing unconfirmed)。详 **ADR-0012 §"Scaling decision flow"** + ADR-0012 §"7. Forward notes" Phase 10 候选行。

### 5. Phase 5 实施要点(immediate next-phase work)

按 Phase 5 入口顺序列出:

1. **DeviceClass 注册**(P5-T-001 候选):helm chart 增加 `templates/deviceclass.yaml`,创建 `npu.ocloud.edge.example.com` DeviceClass(以及可选 sub-classes `/whole` 与 `.dynamic`)。allocator 在 Reconcile 时按 sub-class 选 ResourceSlice 子集(whole class match 所有 NPU;dynamic class 只 match `slice-strategy=Dynamic` 的 NPU)。
2. **claim 控制器升级**:把 T006 的 annotation-only AllocationDeferred 路径换成真实分配。算法选项(详 §6):
   - **a. Greedy first-fit**:按 ResourceSlice 顺序扫,第一个有 `Available > 0` 的 NPU 给 claim。简单。
   - **b. Best-fit**:按 `Available` 升序选(最贴合 claim 大小)。减碎片。
   - **c. Topology-aware**:同 NUMA 优先 → 同 HCCS group 优先(Phase 6 scheduler-plugin 给 hint)。生产推荐。
   Phase 5 入口建议 **a (greedy)**;Phase 6 切到 c。
3. **NPUSliceAllocation CRD**(arch §6.8 占位):
   - 每个成功 claim 分配 → 创建一个 NPUSliceAllocation 对象,记录 `claimRef + sliceRef + nodeName + aiCores`。
   - 提供 audit log / Phase 9 quota 入口 / scheduler-plugin reverse lookup(Pod → slice)。
   - 设计 deferred 到 Phase 5 启动会议;Phase 4 不预定义 schema。
4. **HCCS-ring affinity hooks**:在 ResourceClaim allocation 决策时读 ResourceSlice 的 `npu.huawei.com/hccs-ring`,Phase 6 scheduler-plugin 用其做亲和打分。Phase 5 allocator 仅记 affinity hint 到 NPUSliceAllocation 的 annotation,真排序由 Phase 6 决定。
5. **inference-operator 控制器 body**(P5-T-101 候选):ADR-0008 PD Router webhook 实现 + ModelService Reconcile。两端 binary 同 release(避免单独 build / sign)。
6. **claim_controller.go 注释 vs Status Conditions**:Phase 4 因 v1beta1.ResourceClaimStatus 无顶层 Conditions 字段,用 metadata.annotations 表达 AllocationDeferred(详 `internal/controller/claim_controller.go` 内嵌"Schema-drift note")。Phase 5 当 `status.devices[]` 有真分配时,把分配状态写入 `AllocatedDeviceStatus.Conditions`,annotation-only 路径删除。

### 6. Allocator 算法详细(Phase 5 入口讨论稿)

Phase 4 ADR 仅留 outline;Phase 5 启动会议正式定。

**输入**:
- `claim *resourceapi.ResourceClaim`(待分配)
- `slices []resourceapi.ResourceSlice`(全 cluster,filtered by driver)
- `pool *imsv1alpha1.NPUSlicePool`(从 claim.Annotations[`ocloud.edge.example.com/preferred-pool`] 反查)
- `allocations []NPUSliceAllocation`(已分配,用于 Available 计算)

**Greedy first-fit 伪代码**(Phase 5 入口推荐):
```
allocator.Allocate(claim, slices, pool, allocations):
  required_cores = claim.spec.devices.requests[0].cores  # via DeviceClass selector
  available_by_slice = compute_available(slices, allocations)
  for slice in slices (sorted by slice.Name for determinism):
    for device in slice.spec.devices:
      if available_by_slice[slice][device] >= required_cores:
        return Allocation{slice: slice, device: device, cores: required_cores}
  return ErrNoAvailableDevice
```

**Best-fit 变体**:
```
allocator.Allocate(...):
  ...
  candidates = []
  for ... in slices/devices:
    if available >= required:
      candidates.append({slice, device, slack: available - required})
  if candidates is empty: return ErrNoAvailable
  return candidate with smallest slack (best fit)
```

**Topology-aware**(Phase 6 把 scheduler-plugin hint 合进来):
- 同一 ResourceClaim 内的多 request → 同 NUMA 优先(读 `npu.huawei.com/numa-node`)→ 同 HCCS group 优先(读 `npu.huawei.com/hccs-ring`)。
- 跨 claim:不同 PD-pair Prefill / Decode → 同 HCCS group 优先(scheduler-plugin Score / Bind 阶段读 ModelService annotation 决定)。

---

## 后果

### 正面

- Phase 4 simulator 落地 → CI 全闭环(P4-T-104),不需要真机就能验证 reconcile / RBAC / Helm chart 体系。
- 自研 driver path 与 K8s DRA v1beta1 / Partitionable Devices 升级路径对齐,长期演进无版本债。
- pool-operator + npu-dra-driver 跨控制器 awareness(T102)落地 → 上层 inference-operator 不需要 SDK 调 NPUSlicePool,通过标准 ResourceSlice/Claim 接口完成所有 NPU 资源协商。

### 负面 / 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| Partitionable Devices GA 推迟(K8s 1.37 → 1.38+) | Phase 7 切分粒度推迟 | Phase 5 allocator 用设备级粒度足够支持 PD-pair 业务;Phase 7 升级是 nice-to-have 不是阻塞 |
| KubeEdge 长期无 DRA | 边缘永久 stay Device Plugin v1 | ADR-0001 v3 §5 已记录;dual-path 设计本就允许 |
| Ascend 厂商 SDK / driver 演进破坏 Device.Attributes 语义 | T004 typed view 失效 | `AscendDeviceFromUpstream` 已实现 warn-not-error 兼容(T004 测试覆盖) |
| Phase 5 greedy allocator 在生产集群产生碎片 | NPUSlicePool Available 计数高但实际无 N-core 连续可用 | Phase 6 切到 best-fit / topology-aware;Phase 9 加 defrag job |
| inference-operator 与 npu-dra-driver allocator 之间 race(并发 claim 创建) | 同一 NPU 被双重分配 | Phase 5 allocator 必用 K8s patch-with-resourceVersion 乐观锁 + NPUSliceAllocation owner reference |

---

## 推翻条件

| 决策项 | 推翻条件 |
|---|---|
| 自研 npu-dra-driver | 上游(Huawei / Ascend Cloud / kubernetes-sigs)出现等价开源 driver 且质量过关(Phase 5+ 评审重新讨论) |
| slice ↔ ResourceClaim 映射表 | K8s DRA v1 GA(K8s 1.34)后语义出现 breaking change → 重新设计映射;Phase 5 之前 v1beta1 是 source of truth |
| Greedy 入口算法 | Phase 5 生产 trial 显示碎片 > 30% → 切 best-fit |
| KubeEdge 排除 | KubeEdge 上游发布 DRA edgecore 适配 → 重新规划双轨 |

---

## 引用

- `docs/adr/0001-phase0-key-decisions.md` §5 v3 + §7 — 双轨路径 + Phase 4 scaffold 落地表
- `docs/adr/0008-pd-router-webhook.md` — PD Router admission webhook(Phase 5 consumer)
- `docs/adr/0007-fabric-discovery.md` — fabric discovery 与 npu-dra-driver 互补
- `docs/architecture.md` §3.4(NPU/AI 运行时)/ §5.5(npu-dra-driver 模块)/ §6.7(multi-tenancy)/ §13(Phase 5 review row)
- `docs/cann-driver-matrix.md` — host kernel × driver × CANN 兼容矩阵(Phase 7 入口 gate)
- `docs/phase4-plan.md` §3 P4-T-003..006 / P4-T-101 / P4-T-105 — Phase 4 落地任务
- `operators/npu-dra-driver/` — 本 driver 源码(T003..006 + T101)
- `operators/npu-dra-driver/api/v1alpha1/` — AscendDevice + AscendClaimAnnotations 类型
- `operators/pool-operator/api/v1alpha1/npuslicepool_types.go` — NPUSlicePoolStatus.ResourceSlicesObserved(T102 cross-watch)
- `operators/inference-operator/api/v1alpha1/modelservice_types.go` — ModelService CRD(T103;Phase 5 controller body)
- upstream:`kubernetes-sigs/dra-example-driver` v0.2.1 — fork-spirit 起点
- upstream:KEP-4815(Partitionable Devices)— Phase 7 升级路径

---

**END of ADR-0009**
