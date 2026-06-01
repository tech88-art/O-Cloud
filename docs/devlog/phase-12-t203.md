# P12-T-203 · 右栏 — 吸收 workload/pod + 指标 section + 日志 section

- **Commit**: (this commit)
- **Date**: 2026-06-01
- **Duration**: 2.5d plan / ~1.5d actual

## Intent

per ADR-0022 §2.3 把指标/日志从独立页 fold 进右栏 DetailPanel · selection-type dispatch:
- 右栏吸收 workload/pod 信息(原 DetailPanel 只资源)
- MetricsSection:资源→硬件 Grafana(cluster/node/npu dashboard)· 负载→业务 Grafana(workload_business/workload_resource)· toggle 显隐
- LogsSection:仅负载 · 容器选择 + live · 复用 Logs 页机制(useWorkloadLogs REST seed + useLogsWS append + LogViewer)
- PCIE 补进 NPU DetailPanel(T202 仅 node hover title · 本 task prominent 显示)

## ⚠️ render-verify 抓到的集成 bug · DetailPanel 拓扑查询参数不对齐

选 workload graph node → 右栏显 **`detail-panel-stale`("选中节点已失效")**(截图实证)。根因:DetailPanel 用 `useClusterTopology(selectedClusterId)` **默认 flags**(showFabric/showWorkloads=false)· 但 graph + TopologyView 用 **带 showWorkloads** 的查询。workload/pod node 只在 showWorkloads=true 时存在 → DetailPanel 的拓扑(无 workloads)找不到选中 node → stale。

**Fix**:DetailPanel 读 store 的 showFabric/showWorkloads → 传给 useClusterTopology(与 graph 同参 → react-query 同 cache entry de-dupe)。修后 workloadCard/business metrics/logs section 全渲染(实证 metricsTitle "业务指标" · logLines 真实)。**单测没抓到**(mock 按 path 匹配 `/topology` · 忽略 params · 返回带 workloads 的 fixture)→ 必须 render-verify 才暴露(P7 · 真后端 respects params)。

## Key decisions

- **MetricsSection dashboard dispatch**(DESIGN §6.2):cluster/nodepool→cluster_overview · node→node_detail · npu→npu_detail · slice→npu_detail(parentNPU)· workload→workload_business · pod→workload_resource · 其余(network/switch)→ null(section 自隐)。GrafanaPanel urlBuilder 复用(POC_DASHBOARD_MAP)。
- **LogsSection 仅 workload/pod**(资源无业务日志 · ADR-0022 §2.3):DetailPanel 用 `logsWorkloadRef(node)` 判定 —— workload → 自身 ns/name · pod → 父 workload(attr `workload` · 日志端点是 per-workload)。WS 仅在 section open && live 时连(unmount-on-hide 省连接)。
- **WorkloadDetailView**:workload node fetch `useWorkloadDetail`(status/type/kind/replicas 就绪-期望/nodeNames/pods)· pod node 走 topology attributes(pod 名不是 workload 端点 · query 传 name=null 保持 disabled · 满足 hooks 规则不可条件调用)。
- **section toggle state 入 store**(metricsSectionOpen/logsSectionOpen/selectedContainer · DESIGN §2.1)· 默认 open(选中即显 dashboard · 用户可收)· 持久 across selection。
- **workloadRef.ts shared helper**(parseWorkloadRef + logsWorkloadRef · 单一真实源 P4)· MetricsSection/LogsSection/DetailPanel 共用 · `.ts`(非组件 · 不触 react-refresh lint)。
- **sections.module.css**(§6 CSS Modules · section 壳)· AntD widget 尺寸 inline(对齐被 fold 的 Logs/Metrics 页既有风格)。

## Verification(strict per-task · P3)

- **typecheck** ✓ · **lint**(`--max-warnings 0`)✓ · **test** `vitest run` → **13 files / 117 passed**(+3 T203:workload→info+business+logs · npu→hardware+pcie+no-logs · metrics toggle off 收起 body)
- **render-verify**(Playwright · path A · Grafana fallback 足够记录结构 per plan §8):
  - `after-detail-npu-metrics.png`:NPU 卡(PCIe 带宽 32 GB/s)+ **硬件指标** section(toggle)+ **无日志** section(DOM:logsSection=false)
  - `after-detail-workload-sections.png`:workload 卡(类型/Kind/副本 3/3/节点/Pod 数)+ **业务指标** section(metricsTitle 实证 "业务指标" ≠ npu 的 "硬件指标")+ **日志** section(容器选择 + live + 真实 log 行)
  - bug 修复实证:fix 前 wlState.workloadCard=false(stale)· fix 后 workloadCard/metricsSection/logsSection/containerSelect 全 true

## Carry-forward

- **完整 Grafana iframe**(非 fallback)需 path B(`make dev-up` 或 docker-compose grafana)· T301 render baseline 若要 iframe 实图走 path B · 结构已验(本 task path A fallback)。
- **T204**(顶栏 preset bar):与右栏正交 · 复用 Deploy/PresetGrid。
- **T205**(退役):Metrics/Logs 页壳删 · 其复用组件(DashboardTabs/VarSelectors/LogViewer/GrafanaPanel)保留(本 task LogsSection/MetricsSection 引用 LogViewer/GrafanaPanel · 不可删)· 横向扫确认。
- **pod 日志粒度**:当前 pod → 父 workload 日志流(端点 per-workload)· 若要 pod 级过滤 → 后续(容器选择已是 pod 内粒度近似)。

## §0a.11 compliance

- 单 task 串行 · strict verify(typecheck+lint+test+render-verify 全过 · render-verify 抓到并修集成 bug)· commit · **不 push**(累积到 phase-12-complete tag)
- 无 subagent · 无共享契约改动(消费既有 api-contract · 不动 → 无 gen:types)
- rhythm:T203 done → 续 T204(顶栏 preset bar)· 不停
