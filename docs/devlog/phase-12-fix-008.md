# P12-fix-008 · workloads 按归属 worker 分组多列 + 状态点

- **Commit**: (this commit · post `phase-12-complete`)
- **Date**: 2026-06-02
- **Trigger**: 人工检查反馈 #3 —— "工作负载开启后各节点平铺、杂乱;按归属工作站规则排布、两/多列、running/pending 用绿/黄/红点标识,更清晰省空间"。
- **方向**(2 选项问询定):**画布底部分组多列**(保留画布内呈现,不挪到详情面板)。

## verify-before-propose(数据核查)

`curl topology?includeWorkloads=true`:pod 节点直接带 `attributes.nodeName` = 所属 worker(最可靠映射,优于 runs-on/binds-to 链);pod 状态 Running/Pending/Succeeded/Failed;workload 是**跨 worker 聚合**(qwen-8b-pd:prefill@worker-01 + decode@worker-02),无单一 owner;4 个 pending pod 无 nodeName(未调度)。

## 改动(`TopologyGraph.tsx` 单文件)

- **`WorkloadDotNode`** 新渲染器(node type `wldot`):紧凑"状态点 + 名称"行(替代 workload/pod 的大 `TopoNode` 卡)· workload 加 ⚙ 前缀区分聚合 · 仍是真 ReactFlow 节点(click→DetailPanel + pd-pair/binds-to/runs-on 边照常)。`rendererTypeFor` 把 workload/pod → `wldot`。
- **`workloadStatusColor`**:running/succeeded/healthy/... → 绿 `#52c41a` · pending → 黄 `#faad14` · failed/error/... → 红 `#ff4d4f` · 其他 → 灰。
- **layout 分组**(`layoutSiteTopology` 底部区):pod 按 `attributes.nodeName` 分组,每组一列 x 对齐到该 worker 卡下方(卡片即列头);跨 worker 的 workload 聚合 + 未调度(无 nodeName)pod → 右侧额外列。`WL_ROW_H=22` 行距 · `WL_DOT_W=168` 行宽。替代原 `rowSpread(workloads/pods)` 平铺。
- slices/misc 仍走 rowSpread(仅 drilled/expanded 态出现),下移到底部列之下避免重叠。

## Verification(strict · per-task)

- typecheck ✓ · lint `--max-warnings 0` ✓ · vitest **84 passed**(无回归 · workloads toggle 渲染测试仍命中 rf-node-workload/pod)· e2e **10/10**(覆盖 #1/#2/#3)。
- **render-verify**(Playwright probe · DOM + 截图):workloads ON → 37 个 `wldot`(25 pod + 12 workload)分布在 **5 列**(columnX `-326/-94/138` = worker-01/02/03 卡下方 + `370` workloads 列 + `602` 未调度 pod 列)· 三种状态色齐全(绿/黄/红)。证据 `docs/screenshots/phase12/after-workloads-grouped.png`(worker 列 pod 绿点 + worker-01 红点 oom-test · workloads ⚙ 列 · pending 黄点列)。

## Notes / carry

- workloads 列 + 未调度 pod 列暂无文字列头(⚙ 前缀标聚合 · pending 黄点可辨);若要文字 caption 后续加(ReactFlow 手工 layout 加 caption 需额外伪节点/Panel)。
- workload→worker 不强行映射(聚合本就跨 worker);只 pod 按 nodeName 落到 worker 列。
- 人工检查反馈三连收官(#1 折叠/拉伸 · #2 开关上下文化 · #3 workloads 分组多列)· 本地 commit · **未 push**(`feedback_push_at_phase_tag_only`)。
