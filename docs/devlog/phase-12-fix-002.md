# P12-fix-002 · 拓扑连线降噪 — contextual edges(focus+context)

- **Commit**: (this commit · post `phase-12-complete`)
- **Date**: 2026-06-01
- **Trigger**: 用户反馈 —— "拓扑图很复杂,显示连线后显得凌乱"。
- **方向**: 用户选定 = **上下文连线(focus+context)**(2 选项问询)。

## 业界依据(本轮 web 调研)

毛球(hairball)问题的主流解法是 **focus+context / 聚焦邻居**:默认不全画,选中/hover 某节点才显示它的连线 —— Weave Scope(层间下钻)· Datadog Device Topology(点设备才显依赖)· Cambridge Intelligence("为用户工作流设计,而非照搬原始数据模型")· semantic-zoom("所有信息可达但不同时可见")。

## 改动(`TopologyGraph.tsx` + e2e)

- **contains 骨架常显**(cluster→worker / npu→slice)· 跨节点 fabric(network/hccs/fabric-link/runs-on/binds-to/pd-pair)**默认 hidden**,仅当其端点 = active 节点(hover 优先,否则 selected)才显示。
- `activeEdgeNodeId = hoveredId ?? selectedNodeId` · `displayEdges` 按 incident 给非骨架边打 `hidden` flag(ReactFlow 原生 hidden · 不在 DOM 画)· `onNodeMouseEnter/Leave` 维护 hoveredId。
- toggle 语义不变(仍控制数据是否含 fabric/workload)· 但**渲染上下文化** → 即使 Fabric+Workloads 全开,idle 也只剩骨架,无毛球;hover/选节点才显该节点连线。
- e2e fabric 测试改为:fabric on + idle → bandwidth edge = 0 · 选 worker → > 0(验证上下文行为)。

## Verification(strict)

- typecheck + lint(`--max-warnings 0`)✓ · vitest 10 files / **80 passed**(stub 忽略 hidden · edge 分类单测仍过)· e2e **9/9**(含改写的 contextual 测试)。
- **render-verify**(Playwright DOM 实证):Fabric+Workloads **全开**:
  - `after-ctx-idle.png`:idle → 可见边 = **3**(仅 cluster→worker 骨架)· 无毛球(对比改前 33+ 边全画)。
  - `after-ctx-selected.png`:选 worker-site-a-01 → 可见边 = **9**(骨架 + 该节点 network/fabric/runs-on)· 只显选中节点连线。

## Carry

- 下一步(P12-fix-003 · 用户同选):**点击下钻 + 面包屑**(站点 → 双击 worker 进节点详情 → 双击 NPU 进切片 · 复用 focus filter + 面包屑导航)。
- 可选增强:active 节点连线加粗高亮 + 非骨架"淡化"替代"隐藏"(若用户想保留上下文余晖)。
