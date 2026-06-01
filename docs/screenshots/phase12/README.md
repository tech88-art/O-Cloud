# Phase 12 render baseline — before / after 对比

Phase 12 把当前 **5 页独立路由** 重构为 **one-page workspace**(见 `docs/phase12-plan.md`
Track C)。这里存前后像素对比,供 T302 checkpoint 出对比表。

## 捕获方式

```bash
# 前置: 后端 :8080 (mock) + 前端 :3000 在跑
#   backend:  cd backend && ./bin/demo-backend.exe -c configs/config.dev.yaml
#   frontend: pnpm -C frontend build && pnpm -C frontend preview --port 3000
node tests/e2e/baseline-screenshots.mjs                 # 截 before-*.png
PHASE=after node tests/e2e/baseline-screenshots.mjs     # 截 after-*.png
```

**用 Playwright**(`tests/e2e/baseline-screenshots.mjs` · 自控 `domcontentloaded` +
selector + 固定 settle)**而非 Preview MCP 截图** —— Preview 的 network-idle 截图在本
app 不可用:Vite HMR WS + Overview 拓扑 WS 让页面常驻活跃连接,network-idle 永不达
(实测 Preview 5 次 1 成;Playwright 5 次 5 成,含 WS 的 Overview 页)。

## before-*.png(2026-06-01 · 当前 dev HEAD = phase-11-complete · mock set-a-small · 1440×900)

| 文件 | 页 | 内容 | Phase 12 后变成 |
|---|---|---|---|
| before-overview.png | /overview | 3 栏:资源树(●已连接 · cluster→3 节点→24 NPU→slices)/ ReactFlow 拓扑(#5 degraded)/ 详情空态 | 主工作台(吸收其余 4 页 · 可拖拽可隐藏栏 · 拓扑加绿线/hover带宽/PCIE/HCCS/focus) |
| before-workloads.png | /workloads | 独立整页表格 · 12 workloads | → 右栏 workload 信息 + 拓扑 workload→slice/node 连线 |
| before-deploy.png | /deploy | 独立整页 · 4 预置卡 + 向导 | → 顶栏 preset bar(hover 详情) |
| before-metrics.png | /metrics | 独立整页 · 5 dashboard tab · **Grafana 不可达 fallback** | → 右栏指标 section(资源=硬件 / 负载=业务) |
| before-logs.png | /logs | 独立整页 · 空态(需选 workload) | → 右栏日志 section(仅负载 · 可选容器) |

## Env caveats(影响截图内容 · 非 bug)

- **metrics**:路径 A(无 docker-compose)→ Grafana 不可达 fallback。完整 iframe 需
  路径 B(`docker compose -f deploy/dev/docker-compose.yaml up -d grafana`)。UI 重构
  不改 iframe 本身,故 fallback 态足以记录结构变化。
- **logs**:无 Loki · 选中 workload 后由 mock generator 出"live"流(见 demo-runbook)。
- **arch**:当前 mock `arch: amd64`;Phase 12 Track A 改 aarch64 鲲鹏 + openEuler 后,
  after-overview/详情面板的节点 arch/os 字段会变。

## after-*.png(2026-06-01 · phase-12-complete · one-page workspace)

Track C 各 task(T201-T205）+ T301 落地后用 Playwright 截(`PHASE=after node
tests/e2e/baseline-screenshots.mjs` 截 workspace 关键态;per-task 交互态由各 task
probe 截)。**5 页 → 1 单页操作台**:

| 文件 | 态 | 对应 before |
|---|---|---|
| after-overview.png | 单页工作台默认(preset bar + 资源树 + 拓扑 + 右栏空态) | before-overview.png |
| after-overview-fabric.png | fabric on(network 绿边 + HCCS 紫虚 + fabric 蓝 + bandwidth hover) | before-overview.png |
| after-overview-left-collapsed.png | 左树折叠(Splitter 可隐藏 · 拓扑 re-fit) | — |
| after-edge-bandwidth-tooltip.png | edge hover tooltip(25 GB/s · roce · utilization) | — |
| after-overview-focus.png | focus/isolate 选中 NPU(右栏 NPU 详情 + 退出聚焦) | — |
| after-detail-npu-metrics.png | 资源(NPU)→ 硬件指标 section + PCIe · 无日志 | before-metrics.png |
| after-detail-workload-sections.png | 负载 → 业务指标 + 日志 section(容器选择 + 真实 log) | before-workloads/metrics/logs.png |
| after-preset-bar.png | 顶栏 preset bar(4 预置应用 chip) | before-deploy.png |
| after-preset-hover.png | preset hover 详情 popover | before-deploy.png |
| after-preset-deploy-wizard.png | preset → DeployWizard(auto/manual) | before-deploy.png |
| after-retire-workspace.png | 退役 4 页后单页完整渲染(deep-link redirect) | — |

**DATA 对比**(T302 checkpoint):路由 5→1 · 前端测试 119→80 unit + 9 e2e(one-page flow)·
i18n key 0 orphan · bundle 1.45MB(G6 dep 未用 · Phase 13+ code-split)。

> Grafana iframe 在路径 A(无 docker-compose)显 unreachable fallback(指标 section
> 结构已验 · 完整 iframe 需路径 B)。arch:after-* mock 已是 arm64/openEuler。
