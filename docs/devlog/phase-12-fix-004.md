# P12-fix-004 · 节点内 HCCS ring(下钻 worker 默认显示卡内 HCCS 环)

- **Commit**: (this commit · post `phase-12-complete`)
- **Date**: 2026-06-02
- **Trigger**: 新 session 拓扑视觉打磨 · backlog #1(交接 `topology-polish-handover.md` §4)· 用户 4 选 1 选定起手项 = HCCS ring(P1)。
- **方向**: 下钻进 worker 时,把它的 NPU 按 hccsGroup 默认画成**卡内环**(每组一圈紫色虚线包围 + "HCCS-n" 标签)· runbook D6 的"HCCS group 共址"是自研调度器(NumaAffinity+HCCSTopology+Binpack)核心卖点。

## 关键设计决策(verify-before-propose)

- **数据来源 = NPU 节点的 `attributes.hccsGroup`**(后端 `topology.go` 始终 stamp · 不受 `IncludeFabric` 影响),**不是**后端 `hccs` 边(那些仅 `IncludeFabric=true` 才生成)。⇒ 环**自洽**:下钻即显,无需 Fabric toggle、无 refetch。验证:`curl .../topology` 确认 NPU 节点带 hccsGroup+index;set-a-small 每 worker 8 NPU = 2 组 ×4(hccs-0 / hccs-1)。
- 与 contextual edges 不冲突:跨节点 fabric 仍按 hover/select 隐藏;HCCS 环是**下钻层的卡内装饰**(focusedNodeId === 该 worker 才画),不是 ReactFlow 边。

## 改动

- **新模块 `siteLayout.ts`**(React-free):site-view 几何常量(CARD_*/NPU_GRID_*)+ 纯函数 `layoutWorkerNpus(npus)` → 每 dot 卡内相对位置 + 每组 HCCS 环包围盒。抽出原因:① 组件文件只导出组件(`react-refresh/only-export-components` · `--max-warnings 0`)② 纯几何可单测、无 canvas mock。
- **`layoutWorkerNpus`**:NPU 按 hccsGroup 分组(组内按 index 排序)· 每组占一行(>4 卡换行)· 仅"具名组 + ≥2 成员"出环(对齐后端 `appendHCCS` 规则)。set-a-small 下与原 index-order 2×4 网格**逐像素一致** → 站点视图不变。
- **`WorkerCardNode`**:`showHccsRing`(= 该 worker 被下钻)时叠加 SVG —— 每组紫色虚线 stadium 包围 + fieldset 式 "HCCS-n" 标签(白 halo 断开虚线)· `pointer-events:none` · 与 dot 共用同一几何常量 → 任意 zoom 对齐。
- `layoutSiteTopology` 加 `focusedNodeId` 参 → stamp worker 节点 `hccsRings` + `showHccsRing`;`TopologyGraphInner` 传入。
- i18n:`topology.hccs.ringTitle`(zh/en · hover title)。
- `NPU_GRID_TOP` 50→58:给顶环虚线 + 标签留出 ~9px 间距,避开 "N× Ascend 910B" 副标题(render-verify 发现拥挤后微调)。

## Verification(strict · per-task)

- typecheck ✓ · lint `--max-warnings 0` ✓ · vitest **83 passed**(10 files · +3 新 `layoutWorkerNpus` 单测:8→2 环/组内 index 排序/无组或孤卡不出环)· e2e **10/10**(复用 :8080/:3000 · 无回归)。
- **render-verify**(Playwright probe · DOM 实证 + 截图):站点级 `hccs-ring-overlay=0`(回归:未下钻视图不变)→ 双击 worker-site-a-01 → 面包屑=1 / overlay=1 / `hccs-ring-...-hccs-0`=1 / `hccs-1`=1。证据:`docs/screenshots/phase12/after-hccs-site.png`(站点·无环)+ `after-hccs-drill.png`(下钻·两组环 HCCS-0/HCCS-1)。

## Notes / carry

- 环用**包围盒(stadium)**而非 npu↔npu 连线弧:更净、不与 dot 抢视觉;若要"流动"效果见 backlog #2(连线高亮/动画)。
- 仅在"具名组 ≥2"出环;arch 上若某 worker 单组 8 卡会换 2 行、包围盒跨两行(已处理,未在 set-a-small 触发)。
- 这是 post-tag polish 第 4 弹(fix-001 卡片化 / 002 降噪 / 003 下钻 / **004 HCCS 环**)· 本地 commit · **未 push**(待用户批次确认 · `feedback_push_at_phase_tag_only`)。
- backlog 剩:②连线高亮/动画 ③面包屑去重 ④同 rank 路由 ⑤workloads 密度 ⑥bandwidth 单位 ⑦降级 pulse。
