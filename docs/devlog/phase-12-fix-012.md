# P12-fix-012 · 语言切换按钮改为纯图标

- **Commit**: (this commit · post `phase-12-complete`)
- **Date**: 2026-06-02
- **Trigger**: 人工检查反馈(第 3 轮)#1 —— "中英切换去掉'简体中文'文字描述,保留图标"。

## 改动(`components/Layout/index.tsx`)

- 语言下拉触发按钮:去掉 face 上的 flag emoji(Windows 上渲染成生硬的 "CN")+ `简体中文` 文案,只留 `GlobalOutlined` 地球图标 → 干净的纯图标语言切换器。`aria-label` 保留(可访问性);当前 locale + 全名仍在点开的下拉菜单里(selectedKeys 高亮)。
- 移除随之不再使用的 `Typography.Text` 解构。

## Verification(strict · per-task)

- typecheck ✓ · lint `--max-warnings 0` ✓ · vitest **84 passed**(无回归)· e2e **10/10**。
- **render-verify**:头部右上角语言按钮仅地球图标(无 "CN 简体中文")—— 见 `docs/screenshots/phase12/after-mig-slices.png` 右上角(本批 fix-013 截图同帧可见)。

## Notes / carry

- 人工检查第 3 轮:#1(本)/ #2 显示优化(fix-013 · MIG 切片分区 + 负载列头/PD 标)。
- 本地 commit · **未 push**(`feedback_push_at_phase_tag_only`)。
