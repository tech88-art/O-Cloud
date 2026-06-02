# P12-fix-003 · 拓扑分层下钻 + 面包屑(click-to-drill + breadcrumb)

- **Commit**: (this commit · post `phase-12-complete`)
- **Date**: 2026-06-01
- **Trigger**: 用户 —— "能否做分层显示,一层一层展开详情"。
- **方向**: 用户选定 = **点击下钻 + 面包屑**(Weave Scope 式 · 2 选项问询)。

## 业界依据

Weave Scope 在拓扑层间下钻(containers→hosts→pods→services);semantic-zoom / 渐进式披露 —— "一层一层展开,所有信息可达但不同时可见"。

## 改动(复用现有 focus filter · 最小新增)

- **dbl-click = 下钻**(TopologyView):`onNodeDoubleClick(id)` → `setSelectedNode(id) + setFocusedNode(id)`(取代旧的 npu-only `toggleExpandedNPU`)。复用 `filterTopologyForFocus`(已实现:focus 节点 + contains 子树 + 落点负载),即"下钻进该层"。
- **面包屑**(TopologyGraph · 新 Panel top-left):由 focus 节点的 contains 祖先链派生 `站点 → cluster → worker → npu`。点 crumb 下钻到该层;点 **站点** 返回总览(clear focus)。`返回站点` 按钮(原 clear-focus)同义。
- **下钻 NPU 自动展开切片**:`focusedNodeId` 是 npu 时,`effectiveExpanded` 自动加它 → NPU 详情层显示切片(无需额外手势)。
- i18n:`topology.breadcrumb.site` + focus 标签改下钻语义(下钻选中 / 返回站点)。

与 P12-fix-002 contextual edges 叠加:站点总览干净(无毛球)→ 双击下钻进节点(只剩该节点 + NPU 网格 + 右栏详情)→ 双击 NPU 进切片 → 面包屑逐级返回。

## Verification(strict)

- typecheck + lint(`--max-warnings 0`)✓ · vitest 10 files / **80 passed**(改写 dbl-click 单测:断言 focusedNodeId 而非 expandedNPUs · 切片仍现)· e2e **10/10**(新增 drill+breadcrumb 测试:dbl-click → 面包屑现 + node 数降 → 站点 crumb 返回)。
- **render-verify**(Playwright DOM 实证):site 28 node / 无面包屑 → 双击 worker-site-a-01 → **9 node**(该 worker + 8 NPU · 其余隐藏)+ 面包屑 "站点/cluster-prod-a-01/worker-site-a-01" → 点站点 → 28 node / 面包屑消失。`after-drill-node.png`:仅该 worker 卡居中 + 面包屑 + 返回站点 + 右栏全详情。

## Notes / carry

- 面包屑含 站点 + cluster-prod-a-01 两级(单 cluster 时略冗余 · 二者点击效果近似 · 可后续合并)。
- focus 按钮(下钻选中)与 dbl-click 双触发并存 · 面包屑导航出 · 三者一致(都驱动 focusedNodeId)。
- 这是 post-tag polish 第 3 弹(fix-001 卡片化 · fix-002 连线降噪 · fix-003 分层下钻)· 均本地 commit · 未 push(待用户确认/批次)。
