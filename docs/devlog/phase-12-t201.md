# P12-T-201 · Layout shell — 删 nav + AntD Splitter(可隐藏可拖拽 + 持久化)

- **Commit**: (this commit)
- **Date**: 2026-06-01
- **Duration**: 2d plan / ~0.5d actual

## Intent

per ADR-0022 把 5 路由 + AntSider nav 收敛为单页工作台 shell:
- 删 nav 菜单(Sider.tsx 整删 · brand 已在 Header)
- Overview 固定 CSS grid → AntD `<Splitter>`(左树 + 中拓扑 + 右栏)· 左右 panel 可隐藏(Header toggle)+ 可拖拽改宽 + 宽度/折叠态 localStorage 持久化
- Header 2 个 pane-toggle 按钮(PicLeft/PicRight)替换原 siderCollapse 单 toggle
- 路由暂保留(T205 退役)· `/`→`/overview`=workspace

## Path adaptations(plan literal vs codebase reality)

1. **panel 布局 state 放 `store/index.ts`(app store)非 topologyStore**:Header(Layout)写 + Overview 读 = cross-page · app store 是单一真实源。用 zustand `persist` middleware(partialize 只存 4 个 layout 字段 · 不碰 locale 的独立持久化)+ `merge` 做 corruption 守卫(坏 localStorage → fallback 默认布局 · DESIGN §4)。
2. **Splitter 用条件渲染 panel(非 controlled size=0)实现"隐藏"**:`{!leftPaneHidden && <Splitter.Panel>}`。好处:① 隐藏即 unmount(无残留 split bar)② `onResizeEnd` sizes 数组只含可见 panel · 按同一 visibility gate 回映射宽度(hidden pane 保留存储宽度不被覆盖)。
3. **测试集中在 `frontend/tests/`**(非 plan 写的 `components/Layout/*.test.tsx`)· 改 `tests/Layout.test.tsx`(删 5 项 nav 菜单断言 → 改断言 2 toggle + store 翻转 + 无 menu 文案)。
4. **setup.ts 加 ResizeObserver stub**:jsdom 无 ResizeObserver · AntD Splitter 用它测量 · 无 stub 则 Splitter mount 抛错 → Overview/Layout 全测试挂。no-op stub 让结构渲染(panel 0px · 结构断言够用)。

## ⚠️ Render-verify 抓到的回归(最关键 trail · 跨文件 fix)

**首次 render-verify after-overview.png = 拓扑画布全空**(只剩 ReactFlow 点阵背景 · 无 node/Controls/MiniMap)· tree 有数据(拓扑已加载)。**Phase 11 跳过 render-verify · 这种回归就漏网了 —— plan §8 强制 render-verify 的正当性实证。**

逐步定位(用 Playwright DOM probe · 非截图臆测 · P6 换方法):
1. 第一假设 = ReactFlow `fitView` 一次性 prop 在 Splitter 异步测量(ResizeObserver post-mount)前对 0-size viewport 跑 → node 出界。加 `useReactFlow` + ResizeObserver re-fit。**仍空。**
2. **DOM probe 揭真相**:`topology-graph` / `.react-flow` 高度 = **4321px**(非 ~810)· `fitView` 正确 fit 到 4321 → viewport transform `translateY(2068px)` 把 28 个 node 居中到 4321 的竖直中点(屏幕外)· Controls 在 y=4305。
3. **根因 = 高度 balloon**:`.workspace { height:100% }` 挂在 Layout 根的 `minHeight:100vh` 上 —— minHeight 允许增长 · 高资源树(24 NPU + 96 slice ≈ 4321px)把整个 flex 列撑到内容高 · pane 的 `overflow:auto` 永不触发(height:100% of 已撑大的父 = 无约束)。旧 grid 用 **固定** `calc(100vh - 96px)` 才 clip 正确。
4. **Fix**:`.workspace { height: calc(100vh - 88px) }`(固定 = Header 64 + Content margin 12×2)+ Content `overflow:hidden`。probe 复验:centerH 4321→810 · node 入屏 · 折叠后 re-fit(ResizeObserver)正常。

## Key decisions

- **保留 ResizeObserver re-fit(即便不是空画布根因)**:它解决"拖/折叠 pane → 画布宽高变 → 图自动 re-center"(T201 可拖拽/可隐藏的内在需求)· `FIT_VIEW_OPTIONS` 提为常量 = init prop 与 re-fit 单一真实源。
- **TopologyGraph.tsx 跨 task-Allowed-Path 改动(本属 T202)· justified**:回归由 T201 layout swap 引入 · "图 fit 其容器"的单一正确修复点就在 TopologyGraph(P4 单一真实源)· 与 T202 的 edge 工作正交且会被其 build-on · 同 owner 同 session 顺序执行。Overview.test mock 补 `useReactFlow`(in-scope · tests/* 在 Allowed Paths)。
- **88px offset 硬编码**:同旧 grid 96px 思路(AntD Header token = 64 固定)· demo 可接受。

## Verification(strict per-task · P3 三项)

- **typecheck** `tsc -b --noEmit` ✓ · **lint** `eslint --max-warnings 0` ✓
- **test** `vitest run` → **13 files / 111 passed**(改写的 Layout.test 3 例 + Overview.test 21 例含 Splitter 渲染 · 全绿)
- **render-verify**(Playwright · 非 Preview MCP · plan §8):
  - `after-overview.png` = 3 栏 workspace · 拓扑 node/Controls/MiniMap 全渲染(对齐 before 内容 · 新 shell)
  - `after-overview-left-collapsed.png` = 左树折叠 · 拓扑占据左+中并 re-fit · 右栏存
  - DOM probe 实证:panel 3→2(折叠左)→1(折叠右)· tree pane 0(unmount)· centerH=810(viewport-bound · 不再 4321)
- **build** 不在 T201 acceptance(T205 跑)

## Carry-forward

- **T202**(拓扑增强)将编辑 TopologyGraph.tsx:本 task 已加 ResizeObserver re-fit + `FIT_VIEW_OPTIONS` 常量 · BandwidthEdge / network 绿边 / focus 过滤 在其上叠加 · `useReactFlow` import 已就位。
- **T203**(右栏 section)用 app store `rightPaneHidden` · 但 MetricsSection/LogsSection 自带 section toggle(metricsSectionOpen/logsSectionOpen · DESIGN §2.1)与 pane 隐藏正交。
- **ResizeObserver re-fit 会在每次 pane drag/折叠时 re-center 图**:overview 场景可接受(用户手动 zoom 后再 drag splitter 会重置 zoom · Controls fit 按钮可恢复)· 若 T202 focus 模式不希望被 re-center → 那时再评估(加 user-interacted 守卫)。
- **after-workloads/deploy/metrics/logs.png 未在本 task 截**:那 4 页 T205 退役 · 其 after-* 由 T301 退役后统一截(路由 redirect 到 workspace)。

## §0a.11 compliance

- 单 task 串行 · strict verify(typecheck+lint+test+render-verify 全过)· commit · **不 push**(累积到 phase-12-complete tag · memory `feedback_push_at_phase_tag_only`)
- 无 subagent(前端 layout 连续性 · 无共享契约改动)
- rhythm:T201 done → 续 T202(拓扑增强 · 严格按序 · T201 gate 已立)· 不停
