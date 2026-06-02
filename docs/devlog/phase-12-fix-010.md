# P12-fix-010 · 侧栏宽度封顶(展开/默认不再全屏宽)

- **Commit**: (this commit · post `phase-12-complete`)
- **Date**: 2026-06-02
- **Trigger**: 人工检查反馈(第 2 轮)#2 —— "侧边栏展开后默认宽度,不是全屏宽"。
- 修正 fix-006:fix-006 为满足"全屏空间拉伸"移除了 Splitter `max`,导致可把侧栏拖到近全屏、且持久化后下次 load 复现全屏宽。

## verify-before-fix(probe 实证根因)

probe 喂入持久化 `leftPaneSize=1200` 后 reload → center 被挤到 0、`topology-graph` 隐藏。说明 **AntD Splitter `max` 只约束 LIVE 拖拽,不 clamp 我们从 storage 喂的 `defaultSize`**。故"只加 max"不够,必须同时 clamp 持久化值。

## 改动(两处)

- **`pages/Overview/index.tsx`**:左/右 `<Splitter.Panel>` 重新加 `max="55%"`(比原 45% 宽松,但远非全屏)—— 约束 live 拖拽。
- **`store/index.ts`**:新增 `MAX_PANE_SIZE=600` + `clampPaneSize`(`[160,600]`),在 `setPaneSizes`(写入时)+ `merge`(读持久化时)都 clamp —— 约束 `defaultSize`,旧的拖宽脏值在 load 时被夹回 ≤600。两层一起:拖拽 ≤55% · 默认/恢复 ≤600px。

## Verification(strict · per-task)

- typecheck ✓ · lint ✓ · vitest **84 passed**(无回归)· e2e **10/10**。
- **render-verify**(probe):持久化 `leftPaneSize=1200` → 渲染左 panel **600px**(被 clamp,非 1200),center/graph 正常可见;默认态 [280,776,360] 不变;折叠→展开正确恢复 280。

## Notes / carry

- 600px 为"宽松但非全屏"取值;如需更宽/更窄改 `MAX_PANE_SIZE` 即可。
- 本地 commit · **未 push**。
