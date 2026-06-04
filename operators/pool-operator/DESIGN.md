# pool-operator — module detailed design

> 4-level NPU pooling control plane (ADR-0001 §4 · IMS core). Kubebuilder v4
> project owning the `ims.ocloud.edge.example.com/v1alpha1` group: **ClusterPool
> → NodePool → NPUPool → NPUSlicePool**. Controllers landed Phase 3 (P3-T-002..
> T005 + T105); Phase 4 P4-T-102 added the ResourceSlice cross-watch; Phase 6
> P6-T-003 added `NPUPool.status.hccsTopology` aggregation (ADR-0010 §5). This
> DESIGN.md is a closer backfill (CLAUDE.md §14.2) — it documents the shipped code.

---

## 1. 架构概览

### 1.1 模块在系统中的位置

```
        ┌──────────────────────────────────────────────────────────┐
        │  pool-operator (controller-manager · leader-elected)        │
        │   4 reconcilers behind --enable-controllers bitmask         │
        │                                                             │
        │   ClusterPool ──(PhaseDeferred · Phase 9 Karmada)           │
        │   NodePool    ──selector+role→Node 聚合 (CPU/Mem)           │
        │   NPUPool     ──selector→Node 聚合 (NPU cap/health/alloc)   │
        │                 + HCCS topology ← ResourceSlice             │
        │   NPUSlicePool──npuPoolRef→NPUPool→Node→ Strategy→slices    │
        └───────┬───────────────────────────┬────────────────────────┘
                │ reads (controller-runtime) │ reads (cross-watch)
                ↓                            ↓
   ┌────────────────────────┐   ┌──────────────────────────────────┐
   │ K8s core: Nodes / Pods │   │ resource.k8s.io ResourceSlice     │
   │ (capacity / health     │   │ (npu-dra-driver published ·       │
   │  label / requests)     │   │  hccs_ring + health attrs)        │
   └────────────────────────┘   └──────────────────────────────────┘
                ▲                            ▲
                │ status consumed by         │ same contract consumed by
   ┌────────────┴───────────┐   ┌────────────┴───────────────────────┐
   │ backend crd.Source     │   │ scheduler-plugin HCCSTopology       │
   │ (/api/v1/pools · 拓扑) │   │ (PeerGroup sibling-Pod lookups)     │
   └────────────────────────┘   └─────────────────────────────────────┘
```

### 1.2 数据流

- **Down-resolution**: each CRD resolves *downward* to physical objects —
  NodePool/NPUPool resolve `spec.selector` against the cluster `Node` list;
  NPUSlicePool resolves `spec.npuPoolRef` → parent NPUPool → its selector →
  Node set. There is **no parent→child write**; coupling is by reference +
  shared selector, so each reconciler is independent and idempotent.
- **Up-aggregation**: capacity/health/allocation roll *up* into `.status`
  (NodePool totalCPU/Mem · NPUPool totalNPUs/healthyNPUs/allocatedNPUs ·
  NPUSlicePool totalSlices/availableSlices) for dashboards + backend.
- **Cross-controller contract**: NPUPool + NPUSlicePool both read
  npu-dra-driver `ResourceSlice`s (label `npu.ocloud.edge.example.com/
  managed-by=npu-dra-driver`); NPUPool extracts `hccs_ring`/health device
  attributes into `HCCSTopologyInfo`. **Read via `unstructured`/typed list, no
  cross-module Go import** (operators/CLAUDE.md §1 module isolation) — the
  attribute/label names are text-copied consts pinned by ADR-0010 §5.

### 1.3 Phase 演进

| Phase | Delivery |
|---|---|
| Phase 1 | CRD 类型定义 only (P1-T-003 · no controllers) |
| Phase 3 | 4 reconciler bodies (P3-T-002..T005 + T105) |
| Phase 4 | NPUSlicePool `.Watches(ResourceSlice)` cross-watch (P4-T-102) · `resourceSlicesObserved` |
| Phase 6 | NPUPool `status.hccsTopology` aggregation (P6-T-003 · ADR-0010 §5) |
| Phase 13 | resource-key 统一 `Ascend910→Ascend910B` (P13-fix-002 · single source of truth) |

---

## 2. 接口契约

### 2.1 4 级 CRD (group `ims.ocloud.edge.example.com/v1alpha1`)

| CRD | Scope | Spec 关键字段 | Status 关键字段 | Reconcile 状态 |
|---|---|---|---|---|
| **ClusterPool** | Cluster | `clusters[]`（多站点） | `PhaseDeferred` condition only | **Phase 9 deferred**（passive observer） |
| **NodePool** | Cluster | `selector`（required）· `role`（edge/core） | `nodes[]` · `totalCPU` · `totalMemory` · `Ready` | active（Phase 3） |
| **NPUPool** | Cluster | `selector` | `totalNPUs` · `healthyNPUs` · `allocatedNPUs` · `hccsTopology` · `Ready`+`HCCSDiscovered` | active（Phase 3 + Phase 6 HCCS） |
| **NPUSlicePool** | **Namespaced** | `npuPoolRef` · `strategy`（FixedTemplate/Dynamic）· `fixedTemplates[]{name,aiCoreCount}` · `dynamicSlicing{minAICore}` | `totalSlices` · `availableSlices` · `allocatedSlices` · `resourceSlicesObserved` · `Ready` | active（Phase 3） |

> ⚠️ **Namespaced-under-Cluster 已知隔离缺口**（phase0-review MUST-FIX #6）:
> NPUSlicePool 是 Namespaced 而三级父池是 Cluster-scoped → 多 namespace 可对同一
> 物理 NPU 定义切片,无 RBAC/Admission 强制隔离。Phase 1-2 **隔离 by convention**
> (全部落 `ocloud-system`);完整 multi-tenancy 走 ADR-0014 Quota + Karmada RBAC。

### 2.2 cross-controller ResourceSlice 契约（ADR-0010 §5 · text-copied）

NPUPool `aggregateHCCSTopology` + NPUSlicePool `countNPUDRAResourceSlices` 读
npu-dra-driver 发布的 ResourceSlice:

| 常量 | 值 | 用途 |
|---|---|---|
| label `npu.ocloud.edge.example.com/managed-by` | `npu-dra-driver` | 筛选本 driver 的 slice |
| driver name `spec.driver` | `npu.ocloud.edge.example.com` | NPUSlicePool 计数匹配 |
| attr `npu.huawei.com/hccs_ring` | int | HCCS 环 ID → `HCCSPeerGroup` 分组键 |
| attr `npu.huawei.com/health` | `Healthy`（缺省=健康 · 向后兼容 pre-T002） | 设备健康过滤 |

**PeerGroup 约定**（下游 scheduler-plugin T005 sibling-Pod 查找用）:
`GroupID = "<node>/ring-<int>"` · `DeviceIDs = ["<node>/<device>", ...]`（升序）·
entries 按 (node, ring) 升序写入（确定性 · 避免 resourceVersion churn）。

### 2.3 切片容量公式（NPUSlicePool `computeTotalSlices`）

每物理 910B = `aiCoreTotalAscend910B = 32` AI Core。
- **FixedTemplate**: `Σ_t floor(32 / template.AICoreCount) × npuCount`
- **Dynamic**: `floor(32 / dynamicSlicing.MinAICore) × npuCount`
- `npuCount == 0` → `totalSlices = 0`（非错误:父池暂无设备 / Node 未打标签）

`npuCount` = 父 NPUPool selector 匹配 Node 上 `huawei.com/Ascend910B` capacity 之和。

---

## 3. 生命周期

### 3.1 启动（cmd/main.go）

controller-manager 起 → leader election → 按 `--enable-controllers` bitmask
（`BitNPUSlicePool|BitNPUPool|BitNodePool|BitClusterPool`，default none）选择性
注册 4 reconciler → `SetupWithManager` wire watches → cache sync → 服务。

### 3.2 各 reconciler 触发 + 流程

- **NPUPool** (`For(NPUPool)`): selector→Node list → ① sum `Ascend910B`
  capacity→totalNPUs（Ready+`910B-Health=Healthy` 子集→healthyNPUs）② sum
  非终态 Pod 的 `Ascend910B` requests→allocatedNPUs ③ `aggregateHCCSTopology`
  → ④ 写 `Ready` + `HCCSDiscovered`（nil=NoResourceSlicesObserved / empty=
  NoHealthyDevicesWithHCCS / aggregated=N groups）。
- **NPUSlicePool** (`For(NPUSlicePool)` + `Watches(ResourceSlice)`): ① 加
  finalizer（requeue）② resolve `npuPoolRef`→NPUPool→Node→npuCount ③
  `computeTotalSlices` per Strategy ④ count ResourceSlices→`resourceSlicesObserved`
  ⑤ `availableSlices = totalSlices − allocatedSlices`（clamp ≥0）+ Ready=True。
- **NodePool** (`For(NodePool)`): selector（required，空=Ready=False）+role+排除
  arm64 → 聚合 nodes[]/totalCPU/totalMemory + Ready。
- **ClusterPool** (`For(ClusterPool)`): 仅 emit `PhaseDeferred=True`
  (`WaitingForKarmada`)，幂等（已 True 则 skip update 避免 churn）。Karmada
  member-sync body 留 Phase 9。

### 3.3 Finalizer + 关闭

仅 NPUSlicePool 持 `pool.ocloud.edge.example.com/npuslicepool-cleanup`:
创建即加（先加再做活，防 delete race orphan 子 slice）· 删除路径 drain
finalizer（Phase 3 cleanup 为 placeholder；Phase 5 NPUSliceAllocation owner-ref
释放在此）。其余 3 CRD 无 finalizer（纯 status 聚合，无外部副作用）。

---

## 4. 错误处理

- **`reconcileErr` sentinel**（NPUSlicePool）: 非瞬态业务违规（缺 ref / 父池
  NotFound / selector 非法 / 模板非法）→ `markNotReady` 写 `Ready=False` +
  返回 `(Result{}, nil)` **不重试**（用户改 spec → watch 重新入队）。
- **瞬态错误**（API list/update 失败）: 返回 `(Result{}, err)` → workqueue
  指数退避重试。
- **ResourceSlice API 不存在容错**: envtest / KubeEdge / K8s<1.31 无
  `resource.k8s.io` → NPUPool `meta.IsNoMatchError` 返回 nil（HCCSDiscovered
  停在 NoResourceSlicesObserved，不 crash manager）· NPUSlicePool list 失败
  记 `observed=0` 继续（fail-soft，cross-watch 非致命）。
- **确定性 status**: 所有聚合按 name/ring 排序后写，避免 envtest list 顺序
  导致 resourceVersion churn 反复触发下游 watch。
- **NodePool markNotReady 重置容量**: Ready=False 时清零 nodes/CPU/Mem，
  防陈旧聚合值误导消费者。

> 历史事故（P5-T-121）: P4-T-102 加了 `Watches(ResourceSlice)` 但漏 RBAC
> marker → forbidden watch 阻塞 cache-sync → reconciler worker 不启动 →
> totalSlices 卡 0。修复 = 补 `resource.k8s.io/resourceslices get;list;watch`
> marker。教训记此，warn 二次 watch 必带 RBAC。

---

## 5. 扩展点

- **ClusterPool Karmada body**（Phase 9 / 现 deferred）: member-cluster sync +
  PropagationPolicy；现仅 PhaseDeferred condition 占位（watch Karmada CRD 在
  Phase 3 集群会 crash manager，故不 watch）。
- **🔴 NodePool arm64 排除 = 待修 forward-fix**: `nodepool_controller.go` 仍按
  *architecture.md §1.2 旧 amd64-only 假设* 过滤掉 `kubernetes.io/arch=arm64`
  节点。**ADR-0020（Phase 12）已翻转目标平台为 aarch64 鲲鹏 920** → 真集群上
  NodePool 会错误排除真实目标节点。NPUPool/NPUSlicePool 不含此过滤（按 NPU
  capacity 聚合，arch-agnostic），故 real 版核心 NPU 路径不受影响；但 NodePool
  CPU/Mem 聚合在真 arm64 集群会空。**收口已知残留**（非 real NPU 路径 gating ·
  见本节末 + checkpoint-phase13 §残留候选）。
- **二级 watch 补全**: NPUPool/NodePool 现靠 periodic resync 感知 Node/Pod 变化
  （Phase 3 demo 4 节点足够）；大集群可加 Node/Pod secondary watch + 父池
  `Owns` 链。
- **NPUSliceAllocation 联动**（Phase 5）: `allocatedSlices` 现为占位读；
  NPUSliceAllocation CRD（npu-dra-driver）owner-ref 回填 + finalizer 真清理。
- **AI Core 容量来源**: `aiCoreTotalAscend910B=32` 现硬编码；Phase 4+ 可从
  Ascend DRA driver per-device capacity report 取（arch §1.3 / ADR-0001）。

---

## 6. 集成示例

### 6.1 backend crd.Source 消费 pool status

```bash
# backend datasource/crd 用 controller-runtime client 读 4 级池 status →
# /api/v1/pools 聚合（real profile）· 拓扑页消费 NPUPool.status.hccsTopology
kubectl get clusterpool,nodepool,npupool,npuslicepool -A
kubectl get npupool <p> -o jsonpath='{.status.hccsTopology.peerGroups}'
```

### 6.2 scheduler-plugin 消费 HCCS PeerGroup（ADR-0010 §5）

HCCSTopology Filter/Score（P6-T-004/005）读同一 `hccs_ring` 契约做 PD 共置
sibling-Pod 查找 — `GroupID="<node>/ring-<int>"` 是跨 controller 共享键。

### 6.3 典型 NPUSlicePool（FixedTemplate）

```yaml
apiVersion: ims.ocloud.edge.example.com/v1alpha1
kind: NPUSlicePool
metadata: { name: qwen-pool, namespace: ocloud-system }
spec:
  npuPoolRef: { name: ascend-910b-pool }   # cluster-scoped NPUPool
  strategy: FixedTemplate
  fixedTemplates:
    - { name: vir08, aiCoreCount: 8 }       # 32/8=4 slices/NPU
# status.totalSlices = 4 × (父池 NPU 数) · availableSlices = total − allocated
```

---

## 7. 参考

- **ADR-0001**（`docs/adr/0001-phase0-key-decisions.md`）§4 4 级池化决策 · §13 平台
- **ADR-0010**（scheduler-plugin）§5 ResourceSlice 属性 schema = 跨 controller 契约
- **ADR-0020**（aarch64 鲲鹏 target · supersede ADR-0001 §13）— NodePool arm64 过滤的 forward-fix 依据
- `operators/CLAUDE.md` §1 模块隔离（no cross-module import）· §4 4 级 CRD 关系 · §3.4 CRD 规范
- `docs/architecture.md` §5 模块划分 · §6 CRD Schema · §1.3 Phase 路线图
- `docs/phase3-plan.md`（P3-T-002..T005 + T105）· `docs/phase6-plan.md`（P6-T-003 HCCS）
- 代码: `internal/controller/{clusterpool,nodepool,npupool,npuslicepool}_controller.go` ·
  `utils.go`（`SetCondition`）· `controller.go`（`--enable-controllers` bitmask）·
  `api/v1alpha1/*_types.go`
- cross-module 契约源: `operators/npu-dra-driver/internal/publisher/publisher.go`（label）+
  `operators/npu-dra-driver/api/v1alpha1/resourceslice_types.go`（attrs）
- CLAUDE.md §14.2 module DESIGN.md 7-section convention

---

**END of pool-operator DESIGN.md**
