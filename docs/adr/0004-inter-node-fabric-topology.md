# ADR-0004: Topology 含节点间网络 fabric

- **状态**:Accepted (2026-05-18,用户决定)
- **日期**:2026-05-18
- **决策者**:协调者(用户)
- **相关**:spec/OR-requirements.md F1 ("节点、网络,节点内的 CPU、内存、存储、NPU 信息")

---

## 上下文

spec F1 模糊:"网络" 是顶层项,可能指
- (a) 节点内 NIC(NodeDetail.networkInterfaces 已覆盖)
- (b) **节点间网络 fabric**(switch / 带宽 / VLAN)

用户 2026-05-18 拍板:**(a) + (b)**,含节点间 fabric。

---

## 决策

Topology 加入 **network 节点类型 + 节点间 link 边**。

### Schema 扩展(docs/api-contract.yaml)

新增 `components.schemas`:

```yaml
NetworkSwitch:
  type: object
  required: [id, name, type]
  properties:
    id: { type: string, example: "switch-tor-a-01" }
    name: { type: string }
    type: { type: string, enum: [tor, leaf, spine, access] }
    location: { type: string }
    ports: { type: integer, description: "端口总数" }
    portsUsed: { type: integer }
    bandwidthGbps: { type: integer, description: "单端口带宽(Gbps)" }
    vlans: { type: array, items: { type: string } }
    status: { type: string, enum: [up, degraded, down] }

NetworkLink:
  type: object
  required: [id, from, to, bandwidthGbps]
  properties:
    id: { type: string, example: "link-tor01-worker01" }
    from: { type: string, description: "node id or switch id" }
    to:   { type: string }
    bandwidthGbps: { type: integer }
    medium: { type: string, enum: [copper, fiber, dac, optical] }
    utilization: { type: number, format: float, description: "0-100" }
    rttUs: { type: number, description: "RTT 微秒" }
```

### Topology API 扩展

`GET /api/v1/clusters/:id/topology?depth=...&includeFabric=true`

- 新增 `depth=fabric` 或 `?includeFabric=true` 显式开关(默认 false,保持向后兼容)
- 当开启:Topology.nodes 含 `type=switch` 节点;Topology.edges 含 `type=fabric-link` 边
- 拓扑层级:cluster → fabric(switch 集合)→ node → npu → slice

### Mock 数据扩展

- set-a-small 加 1 个 ToR switch + 3 条 node↔switch link
- set-b-multi-site 加 leaf-spine 2 层 fabric

### 前端

- TopologyGraph 支持 switch / fabric-link 节点类型
- Switch 颜色:up=blue / degraded=orange / down=red
- 左树加 "Network Fabric" 父节点

### Phase 入口

- **Phase 1 W3**:**Mock-only**(schema + mock 数据 + 前端展示),不接真实 fabric discovery
- **Phase 2**:LLDP / SNMP 节点间发现集成(真实 source)
- **Phase 6+**:fabric 拓扑供调度器使用(HCCS 跨 NUMA / 跨节点拓扑感知)

### Phase 1 入口任务(新增)

| 任务 | Module | Allowed Paths | 估时 |
|---|---|---|---|
| **P1-T-013(W1 补)** | configs | `configs/mock-data/schema.json` (RFC 改) + `configs/mock-data/generator/` | 0.5d |
| **P1-T-211(W3 backend)** | backend | `backend/pkg/aggregator/topology.go` (extend) | 0.5d |
| **P1-T-212(W3 frontend)** | frontend | `frontend/src/components/TopologyGraph/TopologyGraph.tsx` (extend) | 0.5d |

总增量 ~1.5d。

## 后果

### 正面
- 调度器 Phase 6 直接消费已有 fabric 拓扑(不必另起 schema)
- spec F1 真覆盖

### 负面 / 风险
- 增加 mock 维护成本(schema + generator + 数据)
- 视觉混乱风险(switch 节点穿插会让小集群 demo 杂乱)— 默认 includeFabric=false 缓解
- Phase 2 真实 fabric discovery 选型未定(LLDP vs SNMP vs SONiC API)

## 推翻条件

- 真实 fabric discovery 复杂度 Phase 2 评估后判定 Phase 1 mock 误导用户 → ADR 修订
- 调度器 Phase 6 改用其他 fabric model
