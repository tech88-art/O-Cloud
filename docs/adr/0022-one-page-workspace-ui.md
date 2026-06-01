# ADR-0022: one-page workspace UI 信息架构(吸收 workloads/deploy/metrics/logs 四页 · ReactFlow 续用 · 右栏 selection-dispatch)

- **状态**:Accepted(Phase 12 entry · per ADR-0019 §2 Decision A Track C · Phase 12 plan P12-T-004 · 2026-06-01)
- **日期**:2026-06-01
- **决策者**:协调者(用户 · 2026-06-01 chat entry meeting 决策 2 操作台一体化 one-page workspace spec)
- **相关**:ADR-0019 §2 Decision A Track C(本 ADR 是 UI IA codification)+ §4 (b)(c) Open questions(本 ADR §2 Decision D + §4 close-out)/ ADR-0021 拓扑全保真(network 绿边 + edge hover 带宽 + PCIE/HCCS + runs-on · 本 ADR 拓扑画布渲染契约)/ `docs/demo-runbook.html` Step 1 拓扑 SVG(视觉北极星)/ `frontend/docs/one-page-workspace.md`(本 ADR sibling · 模块 DESIGN per root CLAUDE.md §14.2)/ `docs/phase12-plan.md` §5 P12-T-201..T205(本 ADR §3 component map 与其 Allowed Paths 一致)/ frontend/CLAUDE.md §4.8(三状态)+ §7(TopologyGraph 约定)

---

## §1 Context

### §1.1 现状:5 路由 + AntSider nav + Overview 3 栏 grid

`frontend/src/App.tsx` 现有 5 路由(overview / workloads / deploy / metrics / logs)+ `/poc/topology` dev 路由。`AppLayout`(Layout/index.tsx)= AntD Sider(nav 菜单 5 项)+ Header(brand + version + 语言 + sider 折叠)+ Content(Outlet)。`Overview/index.tsx` = 固定 CSS grid 3 栏(`treePane` AntD Tree / `centerPane` TopologyView ReactFlow / `detailPane` DetailPanel)· fabric/workloads toggle 在 tree header。

各页独立:Workloads(WorkloadTable + WorkloadDetailDrawer)· Deploy(PresetGrid + DeployWizard)· Metrics(DashboardTabs + VarSelectors · Grafana iframe)· Logs(LogViewer + useLogsWS)。

### §1.2 用户 one-page workspace spec(2026-06-01)

用户决策 2(操作台一体化):把 5 路由收敛为**单页工作台**,Overview 升级为吸收 workloads/deploy/metrics/logs 四页的一体化操作台:
- 删左栏 nav 菜单 · 资源树 + 右信息栏**可隐藏 + 可拖拽**(AntD `Splitter`)
- 拓扑:绿色互通连线 + edge hover 带宽 + PCIE/HCCS 渲染 + workload→node 连线 + focus/isolate 过滤(消费 ADR-0021 全保真数据)
- 右栏吸收 workload/pod 信息 + 指标 section(资源=硬件 / 负载=业务 grafana iframe)+ 日志 section(仅负载 · 可选容器)+ 显示开关
- 顶栏预置应用 bar(hover 详情 + deploy)
- 继续用 ReactFlow(G6 5.x 重写不在本 phase scope)

### §1.3 本 ADR 范围 = UI IA 决策 · 实现详 DESIGN + T201-T205

本 ADR codify IA region + 渲染器选型 + 右栏 dispatch + 退役策略。详细架构(右栏 section 状态机 / focus 算法 / edge 渲染契约 / Splitter 持久化)在 sibling `frontend/docs/one-page-workspace.md`(模块 DESIGN)· 各 task 实现详 plan §5 T201-T205。

---

## §2 Decision

### §2.1 Decision A:IA = 单页工作台 4 region(顶栏 preset bar · 左资源树 · 中拓扑画布 · 右信息栏)

```
┌──────────────────────────────────────────────────────────────┐
│ Header: brand+version · 语言 · [左树 toggle][右栏 toggle]      │
│ Preset bar: [预置应用卡 ×N · hover 详情 · click → deploy]      │  ← T204
├────────────┬─────────────────────────────────┬───────────────┤
│ 左 资源树   │  中 拓扑画布(ReactFlow)         │ 右 信息栏      │
│ (AntD Tree)│  network 绿边 + edge hover 带宽   │ DetailPanel   │
│ 可隐藏      │  + PCIE/HCCS + runs-on           │ + 指标 section│
│ 可拖拽      │  + focus/isolate 过滤            │ + 日志 section│
│            │                                  │ 可隐藏可拖拽   │
└────────────┴─────────────────────────────────┴───────────────┘
   ↑ AntD Splitter (左|中|右 · 可拖拽改宽 · 左右 panel collapsible)
```

- **顶栏**:Header(保留 brand + version + 语言 + 新增 Splitter panel toggle 按钮)+ Preset bar(预置应用 · T204)
- **左 资源树侧栏**:AntD `<Tree>`(现 LeftTree)· AntD `Splitter` panel · 可隐藏(toggle)可拖拽改宽
- **中 拓扑画布**:`<TopologyView>` ReactFlow · Splitter 中 panel(不可隐藏 · 主舞台)
- **右 信息栏**:`<DetailPanel>` · Splitter panel · 可隐藏可拖拽 · 吸收 workload/pod + 指标 + 日志 section
- **布局原语**:AntD `Splitter`(antd ^5.21 具备 · 替换固定 CSS grid · T201 实现时核查可用性)· 删 AntSider nav

### §2.2 Decision B:渲染器保持 ReactFlow(非 G6 · F01 charter 留 Phase 13+)

- 拓扑画布继续用 `@xyflow/react`(ReactFlow · TopologyGraph.tsx 现状)· 自定义 edge 组件加 network 绿色 + hover 带宽 tooltip + hccs/runs-on 样式(T202)
- **不**启用 G6 5.x(`@antv/g6 ^5.1` dep 在 package.json · 但不用)· G6 重写 = Frontend UX Track-2 F01 charter · 留 Phase 13+(per ADR-0019 out-of-scope)
- 理由:Phase 12 scope 聚焦 IA 收敛 + 拓扑数据全保真渲染 · ReactFlow 现状已支持自定义 edge/node · G6 重写是正交的渲染器迁移 · 同 phase 双线 risk 高

### §2.3 Decision C:右栏 dispatch = selection type(资源→硬件指标无日志 / 负载→业务指标+日志)

右栏 `<DetailPanel>` 按选中节点 type 分流(详 DESIGN §状态机):

| 选中 type | 信息区 | 指标 section | 日志 section |
|---|---|---|---|
| **资源**(cluster/nodepool/node/npu/slice) | 硬件属性(含 ADR-0021 pcieBandwidthGBps 等) | **硬件** Grafana(node-detail / npu-detail dashboard) | **无**(资源无业务日志) |
| **负载**(workload/pod) | workload/pod 信息(并入 WorkloadDetailDrawer 内容 + 3 indicator) | **业务** Grafana(workload-business / workload-resource dashboard) | **有**(容器选择 · 同现 Logs 逻辑) |

- 指标 / 日志**不单独成页** · 右栏内 section + **显示开关**(toggle · 避免干扰主视图)
- Grafana iframe 按 selection type 切 dashboard(GrafanaPanel urlBuilder 复用)
- 日志走 LogViewer + useLogsWS(右栏内嵌 · 仅负载 · 容器选择)

### §2.4 Decision D:4 页退役策略 = 路由删除 + catch-all redirect 保 deep-link(close ADR-0019 §4 (b))

**ADR-0019 §4 (b) 退役 4 页 deep-link close-out**:
- T205 **删** `/workloads /deploy /metrics /logs` 4 个显式 Route · 逻辑 fold 入 panel/bar
- **deep-link 安全由现有 catch-all 保证**:App.tsx 已有 `<Route path="*" element={<Navigate to="/overview" replace />} />` —— 删 4 路由后,旧 deep-link(/workloads 等)fall through 到 catch-all → redirect 到 `/`(= one-page workspace)· **无 404**(grep-verified App.tsx:30 catch-all 存在)
- `/` index 现 redirect `/overview` → T201 改为直达 one-page workspace(`/overview` 即 workspace · 或 `/` 直挂 · T201 定)
- **复用组件保留**:WorkloadTable / WorkloadDetailDrawer / PresetGrid / DeployWizard / DashboardTabs / VarSelectors / LogViewer 若被 panel/bar 引用则保留 · 仅删页壳(pages/*/index.tsx 路由入口)· T205 横向扫无死引用
- `/poc/topology` dev 路由:T205 一并清理(dev-only · POCPage / ReactFlowPOC / G6POC 是 P1-T-009 选型遗留 · ReactFlow 已选定)

---

## §3 Component map(改 / 新 / 退役 · 与 plan §5 T201-T205 Allowed Paths 一致)

**改(modify)**:
| 文件 | task | 改动 |
|---|---|---|
| `App.tsx` | T201 / T205 | 路由收敛 · 删 4 页路由(catch-all 保 deep-link)|
| `components/Layout/index.tsx` | T201 / T204 | 删 Sider nav · Header 折叠按钮控制左树/右栏 · 加 preset bar 区 |
| `components/Layout/Sider.tsx` | T201 | 删 5 项 nav 菜单(退化 brand-only 或删)|
| `pages/Overview/index.tsx` | T201 | 固定 grid → AntD Splitter(左树+中拓扑+右栏 可拖拽可隐藏)|
| `pages/Overview/styles.module.css` | T201 | grid → Splitter 容器全高样式 |
| `pages/Overview/DetailPanel.tsx` | T203 | 加 workload/pod dispatch 分支(并入 WorkloadDetailDrawer 内容)|
| `pages/Overview/TopologyView.tsx` | T202 | 传 focus 选中 + 过滤 |
| `components/TopologyGraph/TopologyGraph.tsx` | T202 | network 绿边 + hccs/runs-on 渲染 + 自定义 edge hover 带宽 |
| `store/topologyStore.ts` | T201/T202/T203 | panel 折叠/宽度 · focus/isolate · section toggle state |
| `store/index.ts` | T201 | panel 持久化 state(若放 cross-page)|
| `components/GrafanaPanel/*` | T203 | 右栏内嵌 · 硬件/业务 dashboard 映射 |
| `components/LogViewer/*` + `hooks/useLogsWS.ts` | T203 | 右栏内嵌 · 仅负载 · 容器选择 |
| `pages/Deploy/{PresetGrid,DeployWizard}.tsx` | T204 | 抽复用 fold 入 PresetBar |
| `services/cluster.ts` | T202 | 取新边(network/hccs/runs-on)|
| `i18n/{zh-CN,en-US}.json` | T201-T205 | panel toggle / edge tooltip / section / preset key · 删 menu/退役页 key |

**新(new)**:
| 文件 | task | 用途 |
|---|---|---|
| `components/TopologyGraph/BandwidthEdge.tsx` | T202 | 自定义 ReactFlow edge · hover 显 bandwidthGBps/medium/utilization |
| `pages/Overview/MetricsSection.tsx` | T203 | 右栏指标 section(硬件/业务 grafana toggle)|
| `pages/Overview/LogsSection.tsx` | T203 | 右栏日志 section(负载 · 容器选择 toggle)|
| `components/PresetBar/*` | T204 | 顶栏预置应用 bar(hover 详情 + deploy)|
| `frontend/docs/one-page-workspace.md` | T004(本 task)| 模块 DESIGN |

**退役(retire · T205)**:
| 文件 | 处置 |
|---|---|
| `pages/Workloads/index.tsx` | 删页壳 · WorkloadTable/WorkloadDetailDrawer 保留(DetailPanel 复用)|
| `pages/Deploy/index.tsx` | 删页壳 · PresetGrid/DeployWizard 保留(PresetBar 复用)|
| `pages/Metrics/index.tsx` | 删页壳 · DashboardTabs/VarSelectors 保留(MetricsSection 复用)|
| `pages/Logs/index.tsx` | 删页壳 · LogViewer 保留(LogsSection 复用)|
| `/poc/topology` + POCPage/ReactFlowPOC/G6POC/poc-fixture | dev 路由清理(ReactFlow 已选定)|

---

## §4 Open questions

### (a) Splitter 折叠态默认(close ADR-0019 §4 (c))

- **左资源树**:默认**展开**(主导航入口 · 先选资源才有右栏内容)
- **右信息栏**:默认**展开**(未选时显 empty/提示态 · 维持三状态 frontend CLAUDE §4.8)· 用户可 toggle 收起
- 宽度 / 折叠态 **Zustand 持久化**(localStorage · 刷新保留 · 详 DESIGN §Splitter 持久化)
- **当前倾向**:左树展开 + 右栏展开(empty 态)· T201 实现 confirm(若右栏空态视觉干扰 → 改默认收起)

### (b) focus/isolate 交互手势

- focus 触发:单击资源 = 选中(右栏)· 但 isolate(隐藏无关)用什么手势?
  - 选项 a:双击资源 = focus/isolate(单击=选中 · 双击=聚焦过滤)· 但双击 NPU 现已是展开切片(expandedNPUs)
  - 选项 b:右键菜单 "聚焦此资源" / 工具栏 focus 按钮 + 选中联动
- **当前倾向**:选项 b(显式 focus 按钮/菜单 · 避免与双击展开切片手势冲突)· 详 DESIGN §focus 算法 · T202 实现 confirm

### (c) 移动端 / 窄屏 fallback

- Splitter 三栏在窄屏(<768px)可能过挤 · 是否需要 responsive collapse(单栏 + 抽屉)?
- **当前倾向**:Phase 12 POC 聚焦桌面 demo(投屏 / 大屏)· 窄屏 fallback 留后续 · 不阻塞(demo 场景桌面为主)

---

## §5 引用

### 上游
- ADR-0019 §2 Decision A Track C + §4 (b)(c)(本 ADR codify + close-out)
- ADR-0021 拓扑全保真(network/hccs/runs-on 边 + PCIE + 带宽属性 · 本 ADR 拓扑画布渲染契约)
- `docs/demo-runbook.html` Step 1 拓扑 SVG(视觉北极星)
- frontend/CLAUDE.md §4.8 三状态 + §7 TopologyGraph 约定

### 下游
- `frontend/docs/one-page-workspace.md`(sibling 模块 DESIGN · 6 section)
- P12-T-201 layout shell(§2 Decision A region + Splitter · §4 (a) 折叠默认)
- P12-T-202 拓扑增强(§2 Decision B ReactFlow · ADR-0021 边 · §4 (b) focus 手势)
- P12-T-203 右栏(§2 Decision C dispatch · 指标/日志 section)
- P12-T-204 顶栏 preset bar(§2 Decision A 顶栏)
- P12-T-205 退役 4 页(§2 Decision D 路由删 + catch-all)

---

**END of ADR-0022**
