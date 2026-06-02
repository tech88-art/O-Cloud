# P12-fix-006 · 折叠 chevron 箭头 + Splitter 全屏拉伸

- **Commit**: (this commit · post `phase-12-complete`)
- **Date**: 2026-06-02
- **Trigger**: 人工检查反馈 #1 —— "左/右栏折叠采用业界成熟的箭头,点击就折叠;左右拉伸条可全屏空间拉伸"。

## 改动

- **折叠按钮图标**(`components/Layout/index.tsx`):`PicLeftOutlined`/`PicRightOutlined`(语义模糊的"图片左右"图标)→ **方向感知 double-chevron**:
  - 左栏:显示态 `«`(DoubleLeft · 向左收起)· 隐藏态 `»`(DoubleRight · 向右展开)
  - 右栏:显示态 `»` · 隐藏态 `«`(镜像)
  - 即"箭头指向折叠方向",符合 VS Code / 仪表盘类业界惯例。点击逻辑不变(toggle 整 pane)。
- **Splitter 拉伸**(`pages/Overview/index.tsx`):左/右 `<Splitter.Panel>` 去掉 `max="45%"`,仅留 `min`(左 200 / 右 260),center `min={360}` 兜底 → 侧栏可一路拖到 center 触底,使用近全屏空间。

## Verification(strict · per-task)

- typecheck ✓ · lint `--max-warnings 0` ✓ · vitest **84 passed**(无回归 · Layout 测试按 data-testid 取按钮,不依赖具体图标)。
- **render-verify**(Playwright probe · DOM + 截图):默认态 `toggle-left-pane` icon=`anticon-double-left`、`toggle-right-pane`=`anticon-double-right`;点折叠左栏 → 左按钮 icon 翻转为 `anticon-double-right` + 树 pane 移除。证据 `docs/screenshots/phase12/after-chrome-default.png`(头部 `«` `»`)+ `after-chrome-left-collapsed.png`。

## Notes / carry

- 折叠按钮仍在 Header(未移到 pane 边缘);若后续要 VS Code 式"pane 边缘 collapse 把手"再议。
- 这是人工检查反馈三连的第 1 项(#1 折叠/拉伸 · #2 开关上下文化 · #3 workloads 分组)· #2/#3 涉交互模型重设计,待用户确认方向后做。
- 本地 commit · **未 push**(`feedback_push_at_phase_tag_only`)。
