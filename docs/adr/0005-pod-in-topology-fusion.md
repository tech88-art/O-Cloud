# ADR-0005: POD 与 Workload 融合进主 Topology 图

- **状态**:Accepted (2026-05-18,用户决定)
- **日期**:2026-05-18
- **决策者**:协调者(用户)
- **相关**:spec/OR-requirements.md F2 ("POD 与 POD 间的关系"), ADR-0002 (PD Router)

---

## 上下文

spec F2: "支持查看集群中的业务信息,包括运行中的 POD、POD 与 POD 间的关系,具体内容包括启动命令等业务信息和占用的具体资源等信息"。

当前实现:
- Workloads 页是 AntD Table — 无图形展示 pod 关系
- WorkloadDetail.relations 字段(`prefill → decode`)在 OpenAPI 已建模,但前端未呈现
- 主 Topology 只有 cluster/node/npu/slice — 没有 workload/pod 维度

用户 2026-05-18 拍板:**融合进主 Topology**(不在 Workload Drawer 内单独 mini-graph)。

---

## 决策

主 Topology 增加 **workload 节点类型** + **pod 节点类型** + **pd-pair 边类型**,与 cluster/node/npu/slice 同图渲染。

### Schema 扩展(docs/api-contract.yaml)

`Topology.nodes[].type` enum 扩展:`cluster | node | npu | slice | switch | workload | pod`

新增边 type:`pd-pair`(prefill ↔ decode)、`sidecar`、`init`、`peer`(已在 WorkloadDetail.relations 定义,搬到 TopologyEdge.type)

### Topology API 扩展

`GET /api/v1/clusters/:id/topology?depth=...&includeWorkloads=true`

- 默认 `includeWorkloads=false`(保持向后兼容)
- 开启时:
  - Topology.nodes 含 workload 节点(label = workload name + namespace + status)
  - Topology.edges 把 pod 绑定到具体 slice(`type=binds-to`)
  - pd-pair 边显式展示

### 前端渲染

- workload 节点:AntD 灰 + workload icon
- pod 节点:小一号,挂在 workload 下
- pd-pair 边:虚线箭头 + 标签 "PD pair"
- 主 Overview 页 toggle:**"显示 Workloads"** 开关(默认关,避免初始视图过于密集)

### Phase 入口

- **Phase 1 W3**:Mock-only(schema + topology aggregator + 前端 toggle)
- **Phase 2**:真实数据(K8s Pod 资源 + scheduler binding annotation)

### Phase 1 入口任务(新增)

| 任务 | Module | Allowed Paths | 估时 |
|---|---|---|---|
| **P1-T-213(W3 backend)** | backend | `backend/pkg/aggregator/topology.go` (extend includeWorkloads) | 0.5d |
| **P1-T-214(W3 frontend)** | frontend | `frontend/src/components/TopologyGraph/TopologyGraph.tsx` (workload/pod renderers) + Overview toggle | 1d |

总增量 ~1.5d。

## 后果

### 正面
- spec F2 真覆盖
- PD pair 一目了然(对 D6 亲和对比演示有用)
- 与 ADR-0004 fabric 拓扑同图,呈现完整 "infrastructure ↔ workload" 关系

### 负面 / 风险
- **视觉密度爆炸**:1 集群 + 3 节点 + 24 NPU + 几十 slice + N workload + N×K pod + fabric → 拓扑图可能超过 200 节点,渲染性能压力
- 缓解:默认 `includeWorkloads=false` + UI toggle;**T009 POC 已选 ReactFlow** 可承载 ~500 节点
- 边数爆炸:slice-bound-by-pod 边数 = pod 数 × 1 ≈ 几十,可接受

## 推翻条件

- 实测 set-c-stress(800 NPU)开启 workload 后 FPS < 15 → 改 Workload Drawer mini-graph(降级方案)
- 用户反馈"图太杂",改为 Workloads 页内独立 mini-graph
