# P12-fix-009 · 折叠图标改 Obsidian 式 panel 图标 + 分置两端

- **Commit**: (this commit · post `phase-12-complete`)
- **Date**: 2026-06-02
- **Trigger**: 人工检查反馈(第 2 轮)#1 —— "参考 Obsidian 左/右侧边栏展开折叠图标设计,分别在两端"。
- 修正 fix-006:fix-006 把两个折叠按钮(方向感知 chevron)并排堆在头部最左;Obsidian 是左栏开关在最左端、右栏开关在最右端。

## 改动(`components/Layout/index.tsx`)

- 图标:`DoubleLeft/DoubleRightOutlined`(chevron)→ 自绘 **`PanelLeftIcon` / `PanelRightIcon`**(lucide `panel-left`/`panel-right` 同形:圆角矩形 + 近左/右边的竖分隔线),即 Obsidian 侧栏开关图标。静态(不翻转),折叠时按钮置灰(`#8c8c8c`)+ tooltip 表意。
- 布局:头部 `justify: space-between`,**左栏开关移到最左端**(品牌左侧)、**右栏开关移到最右端**(语言下拉右侧)——各自落在所控制 pane 的一侧。
- testid 不变(`toggle-left-pane` / `toggle-right-pane`),Layout 单测继续命中。

## Verification(strict · per-task)

- typecheck ✓ · lint `--max-warnings 0` ✓ · vitest **84 passed**(无回归)· e2e **10/10**。
- **render-verify**(Playwright probe):left-toggle x=16(最左)· right-toggle x=1392(视口 1440,最右)· 分置两端 `split=true`。证据 `docs/screenshots/phase12/after-toggles-two-ends.png`。

## Notes / carry

- 人工检查第 2 轮 #1(本)/ #2 宽度 clamp(fix-010)/ #3 NPU 分层(fix-011)。
- 本地 commit · **未 push**(`feedback_push_at_phase_tag_only`)。
