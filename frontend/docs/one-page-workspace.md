# one-page workspace — 模块 DESIGN

> Frontend one-page workspace 模块详细设计(per root CLAUDE.md §14.2)。
> 上游决策:ADR-0022(UI IA)+ ADR-0019 §2 Decision A Track C + ADR-0021(拓扑全保真)。
> 实现 task:P12-T-201..T205。Phase 12 把现有 5 路由收敛为单页操作台。

---

## 1. 架构概览

### 1.1 在系统中的位置

`frontend/` SPA 的主壳从 "5 路由多页" 收敛为 "单页工作台 + 退役页复用组件"。对应 arch §3.2(前端)+ §9 部署形态(前端 :3000)。后端契约不变(读 `docs/api-contract.yaml` · ADR-0021 扩拓扑数据)。

```
AppLayout (Layout/index.tsx)
├── Header: brand+version · 语言 · [左树 toggle][右栏 toggle]
├── PresetBar (T204): 预置应用卡 · hover 详情 · click → DeployWizard
└── Workspace (Overview/index.tsx · AntD Splitter)
    ├── 左 panel:  LeftTree (AntD Tree · 资源层级)         可隐藏可拖拽
    ├── 中 panel:  TopologyView → TopologyGraph (ReactFlow) 主舞台
    └── 右 panel:  DetailPanel                              可隐藏可拖拽
        ├── 资源/负载信息区 (selection dispatch)
        ├── MetricsSection (T203 · 硬件/业务 Grafana iframe · toggle)
        └── LogsSection    (T203 · 仅负载 · 容器选择 · toggle)
```

### 1.2 数据流(谁 read / 谁 write / 何时触发)

- **topology DTO**:`useClusterTopology(clusterId, depth, showFabric, showWorkloads)` react-query · LeftTree + TopologyView 共享同一 cache entry(同参数 de-dupe)。ADR-0021 后 DTO 含 network/hccs/runs-on 边 + pcieBandwidthGBps + edge 带宽属性。
- **selection**:LeftTree click + ReactFlow node click 都 write `topologyStore.selectedNodeId`。DetailPanel + TopologyGraph highlight read 同 key(单一真实源)。
- **focus/isolate**:`topologyStore.focusedNodeId`(新)· TopologyView 按它过滤可见 node/edge。
- **panel 布局**:`store` panel 折叠态 + 宽度(Zustand · localStorage 持久化)。
- **WS**:`useTopologyWS` 推送增量 · 更新 react-query cache + `lastEventAt`。
- **指标**:MetricsSection → GrafanaPanel `urlBuilder` 按 selection type 出 dashboard iframe URL。
- **日志**:LogsSection → `useLogsWS`(仅 workload/pod · 容器选择)。

---

## 2. 接口契约(组件 props + store + 渲染契约)

### 2.1 topologyStore 扩展(T201/T202/T203)

```ts
interface TopologyState {
  // 现有
  selectedClusterId: string | null;
  selectedNodeId: string | null;       // 选中(右栏 + highlight)
  expandedNPUs: Set<string>;           // 双击 NPU 展开切片
  lastEventAt: string | null;
  showFabric: boolean; showWorkloads: boolean;
  // T202 新增
  focusedNodeId: string | null;        // focus/isolate 锚点(null = 不过滤)
  setFocusedNode: (id: string | null) => void;
  // T203 新增
  metricsSectionOpen: boolean;         // 右栏指标 section 显隐
  logsSectionOpen: boolean;            // 右栏日志 section 显隐
  selectedContainer: string | null;    // 负载日志容器选择
  // ...setters
}
```

panel 布局态(T201 · 放 `store/index.ts` cross-page 或 topologyStore · 实现定):
```ts
leftPaneHidden: boolean; rightPaneHidden: boolean;
leftPaneWidth: number; rightPaneWidth: number;  // px or %, persisted
```

### 2.2 拓扑 edge 渲染契约(T202 · 消费 ADR-0021)

| edge type | 渲染 | hover tooltip |
|---|---|---|
| `contains` | 层级实线(现状) | — |
| `network`(node↔node) | **绿色**实线 | bandwidthGBps + medium(eth/roce/ib) + utilization |
| `hccs`(npu↔npu intra-node) | 高亮虚线(HCCS 色) | bandwidthGBps + utilization |
| `fabric-link`(node↔switch) | 现状(ADR-0004) | bandwidthGBps + medium + utilization |
| `binds-to`(pod↔slice) | 现状(ADR-0005) | — |
| `pd-pair` | 现状 | — |
| `runs-on`(非 NPU workload→node) | 负载落点连线(区别 binds-to) | — |
| PCIE(host↔NPU) | npu 节点 / contains 边呈现 | pcieBandwidthGBps |

- `BandwidthEdge.tsx`:自定义 ReactFlow edge · 读 `edge.attributes.bandwidthGBps/medium/utilization` · hover 显 tooltip。属性缺失则降级为无 tooltip(三状态精神 · 不崩)。

### 2.3 右栏 selection dispatch(T203 · ADR-0022 §2 Decision C)

```
selectedNodeId → node.type:
  资源 (cluster/nodepool/node/npu/slice):
    信息区 = 硬件属性 (含 pcieBandwidthGBps for npu)
    MetricsSection = 硬件 Grafana (node-detail / npu-detail)
    LogsSection = 隐藏 (资源无业务日志)
  负载 (workload/pod):
    信息区 = workload/pod 信息 (WorkloadDetailDrawer 内容 + 3 indicator)
    MetricsSection = 业务 Grafana (workload-business / workload-resource)
    LogsSection = 显示 (容器选择 · useLogsWS)
```

---

## 3. 生命周期

- **启动(mount)**:AppLayout mount → 自动 pick 首 cluster(`useClusters` 落地后)→ `useClusterTopology` 拉 DTO → LeftTree + TopologyView 渲染 → `useTopologyWS` 订阅。panel 折叠态从 localStorage 恢复。
- **运行(selection)**:用户 tree/graph click → `selectedNodeId` → DetailPanel dispatch + graph highlight。focus 按钮/菜单 → `focusedNodeId` → TopologyView 过滤。Splitter 拖拽 → panel 宽度持久化。section toggle → 指标/日志 iframe/WS 挂载或卸载。
- **关闭(unmount)**:WS hooks(useTopologyWS / useLogsWS)清理连接。退役页删除后无残留路由订阅。

---

## 4. 错误处理(三状态 · frontend CLAUDE §4.8)

- **LeftTree / TopologyView / DetailPanel** 各自 loading(Skeleton)/ error(ErrorState + retry)/ empty(EmptyState)。
- **右栏未选中**:empty 提示态("选择左侧资源查看详情")· 不是空白。
- **MetricsSection**:Grafana iframe 加载失败 → 提示 + 重试(GrafanaPanel 现有处理)。无对应 dashboard → 隐藏 section + 说明。
- **LogsSection**:WS 断 → 重连(useLogsWS)· 无容器 → empty。
- **edge 带宽属性缺失**:tooltip 降级(无带宽则不显)· 不崩。
- **focus 过滤后空**:若 focus 资源无关联节点 → 提示 + 一键清除 focus 恢复全图。
- **Splitter 持久化损坏**:localStorage 解析失败 → fallback 默认布局(左展开+右展开)。

---

## 5. 扩展点(Phase N+)

- **G6 5.x 拓扑重写**(Frontend UX Track-2 F01 charter · `@antv/g6 ^5.1` dep 已在 · Phase 13+):若 ReactFlow 在 set-c-stress 大基数渲染瓶颈 materialise → 切 G6。TopologyView/TopologyGraph 边界保持(props 契约不变)便于替换。
- **真实带宽 telemetry**:ADR-0021 §4 (c) utilization 现为 mock fixture · Phase 13+ 接 NPU-Exporter / 网络 telemetry。
- **窄屏 responsive**:ADR-0022 §4 (c) · Splitter → 单栏 + 抽屉 fallback。
- **deep-link 状态**:focus/selection 入 URL query(可分享视图)· 现仅 `?ffwd`(E2E)。
- **单位统一**:ADR-0021 §4 (b) switch `bandwidthGbps`(Gbps)vs 新 `bandwidthGBps`(GB/s)· 后续 cleanup。

---

## 6. 集成示例

### 6.1 focus/isolate 算法(T202)

```
focus(resourceId):
  visible = {resourceId}
  // 下属(contains 边 BFS · 复用 buildTreeData 的 contains 遍历)
  visible ∪= descendants(resourceId, edges where type=contains)
  // 关联负载(binds-to / pd-pair / runs-on · workload/pod 落在该资源)
  visible ∪= workloads bound to any node in visible
  // 相关硬件边(network/hccs/fabric-link 两端都在 visible)
  edges' = edges where source ∈ visible AND target ∈ visible
  隐藏 visible 外的 node/edge
clearFocus(): focusedNodeId = null → 全图恢复
```

复用现有 `expandedNPUs` 过滤机制(切片按 parent NPU 展开)· focus 是其超集(按 focus 锚点过滤整图)。

### 6.2 右栏 Grafana dashboard 映射(T203)

```ts
// GrafanaPanel urlBuilder 复用
selection.type === 'node'     → dashboard: 'node-detail',  vars: { node }
selection.type === 'npu'      → dashboard: 'npu-detail',   vars: { npu }
selection.type === 'workload' → dashboard: 'workload-business', vars: { namespace, workload }
selection.type === 'pod'      → dashboard: 'workload-resource',  vars: { namespace, pod }
```

### 6.3 Splitter 持久化(T201)

```tsx
<Splitter onResize={(sizes) => persistPaneSizes(sizes)}>
  <Splitter.Panel collapsible defaultSize={leftPaneWidth}>...</Splitter.Panel>
  <Splitter.Panel>...</Splitter.Panel>
  <Splitter.Panel collapsible defaultSize={rightPaneWidth}>...</Splitter.Panel>
</Splitter>
```
宽度/折叠态 → Zustand → localStorage(刷新恢复 · 损坏 fallback 默认)。

---

## 7. 参考

- ADR-0022(UI IA · 4 Decision + component map)· ADR-0019 §2 Decision A Track C
- ADR-0021(拓扑全保真 · edge 渲染契约数据源)· ADR-0004 / ADR-0005 / ADR-0006(拓扑 schema)
- `docs/phase12-plan.md` §5 P12-T-201..T205 + §8 render-verify
- frontend/CLAUDE.md §4.3(Zustand)§4.5(AntD/G6)§4.8(三状态)§7(TopologyGraph)§8(Grafana)
- `docs/demo-runbook.html` Step 1 拓扑(视觉北极星)
- 代码:`pages/Overview/{index,TopologyView,DetailPanel}.tsx` · `components/TopologyGraph/TopologyGraph.tsx` · `store/topologyStore.ts` · `components/{GrafanaPanel,LogViewer}/`
