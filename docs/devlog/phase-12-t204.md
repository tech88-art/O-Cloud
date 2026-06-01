# P12-T-204 · 顶栏预置应用 bar

- **Commit**: (this commit)
- **Date**: 2026-06-01
- **Duration**: 1.5d plan / ~0.5d actual

## Intent

per ADR-0022 §2.1 把 Deploy 页 catalog fold 进 Header 下方薄条:
- 预置应用 chip(基础信息:name + NPU 数)· hover 显详情 popover(模型规模/运行时/NPU/显存/描述)· click → DeployWizard
- 复用 `usePresets` + `<DeployWizard>`(不 fork · 与 /deploy 页同源)
- 挂在 Layout(Header 与 Content 之间)· 单页工作台顶栏

## ⚠️ 布局:preset bar 高度 → 必须把 workspace 从 fixed-calc 改 flex-fill(跨 T201 文件)

T201 给 `.workspace` 设 `height: calc(100vh - 88px)`(88 = Header 64 + Content margin 24)修高度 balloon。**加 preset bar(~40px)后该 calc 失准** → workspace 高出 ~40px → 底部(拓扑 Controls)被 Content `overflow:hidden` 裁掉。

**正解(非再加 magic number)**:把整个 shell 改成 proper full-height flex —
- Layout 根 AntLayout `minHeight:100vh` → **`height:100vh` + `overflow:hidden`**(固定 · flex column)
- Header + PresetBar 自然高(flex-shrink:0)· Content `flex:auto` 填剩余
- `.workspace` `height:calc(...)` → **`flex:1 1 auto; min-height:0`**(flex-fill · Splitter `height:100%` 对其 flex-resolved 高度生效)

→ preset bar 高度**自动**被计入(Content 填 Header+PresetBar 之后的剩余)· 无 magic number · 拓扑 canvas 实测 770px viewport-bound(不再 balloon · 28 node + Controls 全在)。

**跨文件说明**:`pages/Overview/styles.module.css`(`.workspace`)属 T201 Allowed Path · 本 task 改它 —— 因 preset bar(T204)直接迫使 workspace 高度机制重做 · 同 owner 同 session · 是更优架构(去 magic number)· 类比 T201 改 TopologyGraph。已 render-verify 确认 T201/T203 的拓扑/panes 行为零回归。

## Key decisions

- **PresetBar chip = AntD Button + Popover(hover)**:button 显 `name · N NPU` · Popover content 复用 /deploy grid card 同字段(modelSize/runtime/npuCount/vramMiBPerNPU/description · i18n `deploy.preset.*` 复用)。
- **DeployWizard 直接复用**(import from `pages/Deploy/DeployWizard`)· 不抽取不 fork · 避免改 PresetGrid(降风险)· 单一真实源 = /deploy 页 + preset bar 共用同 wizard + usePresets。
- **on success 不导航**(/deploy 页导航到 /workloads · 但 /workloads 退役中)· 改为 toast success + 留在 workspace(用户可开 workloads toggle 看落地)。
- **空/加载态**:bar 是 chrome · loading→小 Spin · 空→"暂无预置应用" 文案 · 始终渲染容器(高度稳定 · 不抖)。

## Verification(strict per-task · P3)

- **typecheck** ✓ · **lint**(`--max-warnings 0`)✓ · **test** `vitest run` → **14 files / 119 passed**(+2 PresetBar:chip 渲染含 NPU 数 · click 开 wizard)
- **render-verify**(Playwright):
  - `after-preset-bar.png`:顶栏 4 chip(Pi 3B·1NPU / Qwen 8B PD·2NPU / DeepSeek 20B·2NPU / Qwen 14B·1NPU)+ 下方 workspace 完整(拓扑 28 node + Controls + MiniMap + 右栏)
  - `after-preset-hover.png` + 文本实证:hover → popover "Kind inference · 模型规模 3B · 运行时 mindie · 需要 NPU 数 1 · 每卡显存 8192 MiB · 描述"
  - `after-preset-deploy-wizard.png`:click → DeployWizard "应用部署 · Pi 3B Inference"(选择模式 自动调度/手动指定 · 下一步)
  - flex 重构实证:canvasH=770(viewport-bound · 非 balloon)· nodes=28 · controls=true

## Carry-forward

- **T205**(退役):删 /deploy 页壳 · PresetGrid/DeployWizard **保留**(PresetBar import DeployWizard · 不可删)· 横向扫确认无死引用。Deploy 页 onSuccess 导航 /workloads —— 页删后该路径随之删。
- **preset bar 窄屏**:overflow-x:auto 横向滚(>4 preset 或窄屏)· responsive 留后续(ADR-0022 §4(c) Phase 12 桌面 demo 为主)。

## §0a.11 compliance

- 单 task 串行 · strict verify(typecheck+lint+test+render-verify 全过)· commit · **不 push**(累积到 phase-12-complete tag)
- 无 subagent · 无共享契约改动(消费既有 presets/deploy API · 不动 → 无 gen:types)
- rhythm:T204 done → 续 T205(退役 4 页 + i18n cleanup + build)· 不停
