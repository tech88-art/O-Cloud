# P12-T-205 · 退役 4 页路由 + POC 清理 + i18n cleanup + build gate

- **Commit**: (this commit)
- **Date**: 2026-06-01
- **Duration**: 1d plan / ~0.5d actual

## Intent

per ADR-0022 §2.4 Decision D 退役独立页(逻辑已 fold 入 workspace + preset bar):
- 删 `/workloads /deploy /metrics /logs` 4 路由 + `/poc/topology` dev 路由
- 删页壳 + orphan 组件 · 保留被新代码引用的复用组件
- catch-all 保 deep-link(旧链接 → /overview · 无 404)
- i18n 清孤儿 key · `pnpm build` 新 gate(无 unused/死码)

## 删 / 留 判定(P4 横向扫 importers · 非拍脑袋)

`git grep` 全量 importers 后:
- **删(orphan · 仅自身页用)**:pages/Workloads/{index,WorkloadTable,WorkloadDetailDrawer,styles} · pages/Metrics/{index,DashboardTabs,VarSelectors,styles} · pages/Logs/index · pages/Deploy/{index,PresetGrid} · POC(TopologyGraph/{G6POC,ReactFlowPOC,POCPage,poc-fixture,useFpsMeter})
- **留(被新代码引用)**:`DeployWizard`(PresetBar T204)· `LogViewer`(LogsSection T203)· `GrafanaPanel`(MetricsSection T203)· `TopologyGraph`(workspace)
- **关键**:DeployWizard 留在 `pages/Deploy/`(不 move · PresetBar import 路径不变)· Deploy/ 目录退化为只剩 DeployWizard.tsx + styles.module.css。
- **TopologyGraph/index.ts**:删 POC re-exports(G6POC/ReactFlowPOC/POCPage/poc-fixture)· 只留 production wrapper export。

## 测试迁移

- 删退役页测试:Workloads/Metrics/Logs/TopologyPOC.test.tsx。
- **Deploy.test.tsx → DeployWizard.test.tsx**:页 + PresetGrid 没了 · 但 DeployWizard 留(PresetBar 复用)· 把 4 个 wizard 用例(step 转换 / auto submit / manual submit / 409)从"点 preset card 开 wizard"改为**直接渲染 `<DeployWizard open>`** · 保住 wizard 全覆盖(没丢逻辑测试)。PresetGrid loading/error/card 测试随页删(PresetBar.test 覆盖 chip 渲染)。

## i18n 孤儿清理(P4 · 实证 0 孤儿)

`git grep` 确认 orphan 后删:
- 整块删:`workloads.*`(WorkloadTable)· `workloadDetail.*`(WorkloadDetailDrawer)· `metrics.*`(Metrics 页)· `page.{overview,workloads,deploy,metrics}`(占位)
- 部分裁:`page.logs.*` 12→4(只留 LogsSection 用的 containerAll/live/empty/errorTitle)· `deploy.subtitle`(仅 Deploy 页)· `deployMessages.deploying`(无引用)· `overview.{loading,detailPanePending}`(无引用)· `app.subtitle` + `common.{loading,language}`(pre-existing 孤儿)
- **保**:`deploy.*`(DeployWizard 用 title/autoMode/.../preset.* · PresetBar 用 preset.*)· `deployMessages.{successId,conflict,error}`
- **实证**:node 脚本 flat 两 locale → **122→119 key · zh/en parity true · orphan=0**(dyn 前缀 detailPanel.type./header.languageOption./topology.edge.typeLabel. 排除后逐 key grep src)

## Verification(strict per-task · P3 · T205 acceptance 全项)

- **typecheck** `tsc -b --noEmit` ✓(删后无 dangling import)· **lint** `--max-warnings 0` ✓
- **test** `vitest run` → **10 files / 80 passed**(119→80:删 4 退役页测试 + Deploy→DeployWizard · 退役代码的测试随之删 · DeployWizard 覆盖保留)
- **build** `tsc -b && vite build` → **✓ built**(3679 modules · 无 unused/死码报错 · >500kB chunk warning 是 pre-existing bundle 体积提醒 · 非本 task 引入)
- **i18n**:119 key · parity true · **0 orphan**(node 实证)
- **render-verify**(Playwright · `after-retire-workspace.png`):
  - **deep-link 重定向**:`/workloads` `/deploy` `/metrics` `/logs` `/poc/topology` 全 → `/overview`(workspace + preset bar 渲染 · 无 404 · catch-all 实证)
  - **退役后单页完整**:preset bar(4 chip)+ 资源树 + 拓扑(28 node + Controls + MiniMap)+ 右栏 全渲染

## Carry-forward

- **EmptyState JSDoc**:残留 `t('workloads.empty')` 示例注释(已删 key)· 纯文档示例非真引用 · 不影响 build/运行 · 低优先级(未改 · 避免无谓 churn)。
- **T301**(e2e + baseline)：baseline-screenshots.mjs 仍截 5 页(overview/workloads/.../logs)· 退役后 4 页 redirect 到 workspace · T301 改脚本只截 workspace 关键态(资源/负载/focus/preset)· 出 before/after 对比。
- **bundle 体积**(1.45MB · pre-existing)：G6 dep 仍在 package.json(Phase 13+ G6 重写才用)· 可 code-split / 移除未用 dep · 留后续 perf task。

## §0a.11 compliance

- 单 task 串行 · strict verify(typecheck+lint+test+**build**+render-verify+i18n orphan 全过)· commit · **不 push**(累积到 phase-12-complete tag)
- 无 subagent · 无共享契约改动
- rhythm:T205 done(Track C 5 task 全完)→ 续 T301(e2e one-page flow + baseline)· 不停
