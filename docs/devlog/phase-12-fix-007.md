# P12-fix-007 · Fabric/Workloads 开关上下文化(左树 header → 详情面板)

- **Commit**: (this commit · post `phase-12-complete`)
- **Date**: 2026-06-02
- **Trigger**: 人工检查反馈 #2 —— "显示网络/工作负载按钮集成到工作站、展开节点看详细信息时显示;核心简化、信息获取更便捷"。
- **方向**(2 选项问询定):**开关移到右侧详情面板**(保留手动控制 · 左树 header 简化)。

## 改动

- **`DetailPanel.tsx`**:新增 `ViewOptions` 卡(AntD `ResourceCard` · `detail-panel-view-options`),内含 `ViewToggle` ×2(Fabric / Workloads)· 渲染在面板**最顶部 · 每个状态都显示**(空/加载/失效/选中),所以"看详情面板就能切换",而总览本身保持干净。
  - 重构控制流:原 3 个早返回(empty/loading/stale)+ switch 改为 `viewOptions + body`;选中态的 type-dispatch switch 抽到新 `DetailBody` 组件(避免 switch 嵌进 else)。
  - testid 不变:`fabric-toggle` / `fabric-toggle-switch` / `workloads-toggle` / `workloads-toggle-switch`(现有 toggle 行为单测继续 pin)。
- **`Overview/index.tsx`**:左树 header 去掉两个 `<Switch>`(只剩标题 + WS 状态 chip);删 `FabricToggle`/`WorkloadsToggle` 组件 + 接口;去掉 `setShowFabric`/`setShowWorkloads` 读取(仍 READ `showFabric`/`showWorkloads` 喂 `useClusterTopology`,保持树/图/面板查询同 key);去掉无用 `Switch`/`Tooltip` 导入。
- i18n:`detailPanel.viewOptions`(zh "视图选项" / en "View options")。复用既有 `overview.includeFabric/...Hint` 标签键。

## Verification(strict · per-task)

- typecheck ✓ · lint `--max-warnings 0` ✓ · vitest **84 passed**(无回归 · fabric/workloads toggle 行为单测 + DetailPanel 分支测试全过 —— toggle 按 testid 定位,迁移到面板后仍命中)。
- **render-verify**(Playwright probe · DOM + 截图):空态 `toggles-in-tree=0` · `detail-panel-view-options=1` · fabric/workloads switch 各 1(在 detail pane);选中 worker → `view-options=1` + `detail-panel-node=1`(开关常驻顶部 + 节点详情在下)。证据 `docs/screenshots/phase12/after-detail-toggles-empty.png` + `after-detail-toggles-selected.png`(左树 header 仅"资源树"+连接状态 · 右侧"视图选项"卡 + 节点详情)。

## Notes / carry

- 边界:右侧详情 pane 若被折叠(rightPaneHidden),则暂时无法切换 Fabric/Workloads —— 需先展开右栏。用户选定"移到详情面板"已接受此取舍;默认右栏展开。
- 这是人工检查反馈三连第 2 项(#1 折叠/拉伸 done · #2 开关上下文化 done · #3 workloads 分组多列 待做)。
- 本地 commit · **未 push**(`feedback_push_at_phase_tag_only`)。
