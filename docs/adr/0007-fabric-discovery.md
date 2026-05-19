# ADR-0007: Fabric Discovery — Phase 2 ships static-config; LLDP / SONiC deferred

- **状态**:Accepted (2026-05-18 — Phase 2 P2-T-104)
- **日期**:2026-05-18
- **决策者**:协调者(用户)
- **相关**:ADR-0004(inter-node fabric topology)/ phase2-plan.md §P2-T-104 / architecture.md §13

---

## 上下文

ADR-0004 加了 `NetworkSwitch` + `NetworkLink` 节点/边到 topology schema,Phase 1 demo 用 hand-written / generator-produced fixture 喂数据。Phase 2 切到真实 K8s 后,fabric 数据从哪里来需要选型:

候选方案:

1. **Static config**(YAML/JSON file 或 ConfigMap):运维写死,backend 启动时读
2. **LLDP probe**(Link Layer Discovery Protocol):节点上跑 lldpd,backend 通过 Pod exec 或 sidecar 收集
3. **SONiC API**(开源网络 OS):if 交换机跑 SONiC,有 REST API 可查 LLDP / fabric topology
4. **SNMP**:传统网管协议,通用但需逐 switch 配置 community string

每个方案的成本/可行性维度:

| 方案 | 集成成本 | 实时性 | 需要交换机配合 | Phase 2 可行度 |
|---|---|---|---|---|
| Static config | 极低 | 重启生效 | 无 | ✓ 立即 |
| LLDP probe | 中高(节点 sidecar + RBAC) | 分钟级 | 节点上 lldpd | 需 0.5-1d 调研 |
| SONiC API | 中(假设 switch 支持) | 秒级 | switch 是 SONiC | 大多数客户不是 SONiC,落地受限 |
| SNMP | 中(community string 管理) | 秒级 | 每 switch 配置 | 安全合规摩擦大 |

---

## 决策

**Phase 2 ship static-config**;LLDP / SONiC API 留 Phase 3+。

理由:

1. **Phase 2 demo 主线是"真实 datasource"而非"fabric 自动发现"** — fabric 是 ADR-0004 加的可视层,从 ops 视角变更频率低(机柜布线变更才会动)。静态配置完全够用。
2. **决策可逆**:静态 source 输出的 `NetworkSwitch` / `NetworkLink` 结构与 LLDP / SONiC 产出相同。Phase 3 加 LLDP source 时,只需切换 mapping(`mapping.fabric: static` → `mapping.fabric: lldp`),wire shape 不变。
3. **避免对交换机栈做假设**:Phase 2 部署目标多样(K3s on 单台 / 多节点裸机 / 云 K8s),交换机栈不一致;LLDP 需要节点上 `lldpd` 服务,可能与客户安全策略冲突。
4. **测试可行性**:静态 source 可全 unit-test(读 YAML → 解析 → 输出 DTO),不需要真实 fabric。

---

## 实施

### Source 形式

`backend/pkg/datasource/fabric/` 包,提供 `*fabric.Source` 类型(并非 `datasource.Source` interface 实现 — fabric 数据由 aggregator 注入 topology,不通过 REST 暴露独立 endpoint):

```go
type Source struct { ... }

func NewSource(opts Options) (*Source, error)
func (s *Source) ListSwitches(ctx) ([]aggregator.NetworkSwitch, error)
func (s *Source) ListLinks(ctx) ([]aggregator.NetworkLink, error)
```

### Options

```go
type Options struct {
    Path string  // file path to fabric.yaml
}
```

YAML 形如(与 `configs/mock-data/set-a-small/networkSwitches.json` + `networkLinks.json` 结构同):

```yaml
switches:
  - id: switch-tor-01
    name: switch-tor-01
    type: tor
    location: site-a-shanghai
    portsTotal: 48
    portsUsed: 3
    bandwidthGbps: 25
    status: up
links:
  - id: link-tor-01-worker-site-a-01
    from: worker-site-a-01
    to: switch-tor-01
    bandwidthGbps: 25
    medium: dac
    utilization: 12.0
```

### 配置项

```yaml
# backend/configs/config.example.yaml
datasources:
  fabric:
    enabled: false
    path: "/etc/ocloud/fabric.yaml"
```

### Aggregator 集成

Phase 3+ aggregator 在构造 topology 时,如果 `fabric.Source` 可用 + 请求 `includeFabric=true`,从 fabric.Source 拿 switches/links 注入 `appendFabric()`(目前 mock fixtures 走的同一路径)。

Aggregator 拿 `*fabric.Source` 通过 main.go 显式注入(不是经 datasource.Source 接口) — fabric 是聚合层的依赖,不是 REST 资源。

### 不做(明示)

- Phase 2 不实现 LLDP / SONiC / SNMP — 复杂度收益比不划算
- 静态文件不支持 hot reload(运维改 yaml 必须重启 backend)— Phase 3 加 file watcher 若需要
- Switch 健康轮询不在 fabric.Source 职责内(Prometheus + custom exporter 走 metrics 路径)

---

## 后果

### 正面
- Phase 2 demo 真实 K8s 路径 + 真实 fabric 数据,无 mock 依赖
- 测试可全 unit-cover(无外部交换机依赖)
- 与现有 NetworkSwitch / NetworkLink schema 完全兼容

### 负面 / 风险
- 静态配置过时风险:机柜布线变更后,运维忘记更新 yaml → topology 渲染过时
- 缓解:Phase 3 加 file watcher 或定时重读(< 1d 工作);告警机制 Phase 5+
- 多站点(set-b-multi-site)需要 per-cluster fabric.yaml — Phase 9 Karmada 集中管理

## 推翻条件

- 客户 fabric 规模 > 100 switches 且变更频繁 → 静态文件维护成本不可接受 → 启动 LLDP / SONiC ADR(Phase 3+)
- 客户全栈 SONiC 部署 → 直接走 SONiC API 而非静态

---

**END of ADR-0007**
