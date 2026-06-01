# P12-T-004 · ADR-0022 + frontend/docs/one-page-workspace.md(one-page UI IA)

- **Commit**: (this commit)
- **Date**: 2026-06-01
- **Duration**: 1d plan / ~0.6d actual(docs/DESIGN · 无 runtime code)

## Intent

W1 最后一个 foundation task · codify Track C one-page workspace 信息架构:
- ADR-0022:IA 4 region(顶栏 preset bar · 左资源树 · 中拓扑 · 右信息栏)+ ReactFlow 续用 + 右栏 selection dispatch + 4 页退役策略
- frontend DESIGN(6 section per §14.2):右栏 section 状态机 + focus/isolate 算法 + edge 渲染契约 + Splitter 持久化
- close ADR-0019 §4 (b) 退役 deep-link + (c) Splitter 折叠默认

为 T201-T205 提供 UI IA lock + component map(改/新/退役 与 plan §5 Allowed Paths 一致)。

## Path adaptations

1. **frontend/docs/ 目录不存在** · Write 创建(模块 DESIGN 首次落 frontend/docs/ · 同 backend/docs/ pattern per §14.2)。
2. **ADR-0019 §4 (b) deep-link close 用现有 catch-all**:不需新建 redirect 逻辑 · App.tsx:30 已有 `<Route path="*" element={<Navigate to="/overview" replace />} />`(grep-verified)· 删 4 路由后旧 deep-link fall through catch-all → `/` · 无 404 · ADR-0022 §2 Decision D codify 此(零新增逻辑 · 复用现状)。

## Key decisions

- **ReactFlow 续用 · 非 G6**(ADR-0022 §2 Decision B):G6 5.x 重写 = F01 charter Phase 13+ · Phase 12 同 phase 双线(IA 收敛 + 渲染器迁移)risk 高 · ReactFlow 现状已支持自定义 edge/node(加 network 绿边 + hover 足够)
- **右栏 selection dispatch**(§2 Decision C):资源(cluster..slice)→ 硬件 Grafana + 无日志 · 负载(workload/pod)→ 业务 Grafana + 日志(容器选择)· 指标/日志右栏 section 而非独立页(吸收 metrics/logs 页)
- **4 页退役 = 删路由 + catch-all 保 deep-link + 复用组件保留**(§2 Decision D · close ADR-0019 §4 b):删页壳 · WorkloadTable/PresetGrid/DeployWizard/DashboardTabs/LogViewer 保留(panel/bar 复用)· 无 404 无死引用
- **Splitter 折叠默认 = 左展开 + 右展开 empty 态**(§4 (a) · close ADR-0019 §4 c):左树主导航默认展开 · 右栏默认展开显 empty 提示(三状态)· 宽度/折叠 Zustand 持久化
- **focus 手势留 §4 (b) open**:倾向显式 focus 按钮/菜单(避免与双击展开切片 expandedNPUs 冲突)· T202 实现 confirm — 不在 ADR 锁死交互细节(留实现弹性)
- **component map 与 plan §5 T201-T205 Allowed Paths 1:1 核对**(P4 横向):改 15 文件 + 新 5 文件 + 退役 4 页壳 · grep frontend 结构确认每个文件真存在(Layout/index.tsx · Sider.tsx · Overview/{index,TopologyView,DetailPanel} · TopologyGraph.tsx · topologyStore.ts · GrafanaPanel · LogViewer · Deploy/{PresetGrid,DeployWizard} · 各页)

## Verification

P3 三项验证(docs/DESIGN · 无 compile):

- **存在性**:ADR-0022 + frontend/docs/one-page-workspace.md + 本 devlog 落地 · component map 列的现有文件全部 glob-verified 真存在(App.tsx / Layout/{index,Sider} / Overview/{index,TopologyView,DetailPanel} / TopologyGraph.tsx / topologyStore.ts / store/index.ts / GrafanaPanel/* / LogViewer/* / hooks/useLogsWS.ts / Deploy/{PresetGrid,DeployWizard} / Workloads / Metrics / Logs / services/{cluster,workload,preset,logs})
- **完整性**:plan T004 acceptance 3 项核 — ① ADR §2 Decision A-D(IA region / ReactFlow / 右栏 dispatch / 退役策略)✓ ② frontend DESIGN 6 section(架构概览 / 接口契约 / 生命周期 / 错误处理 / 扩展点 / 集成示例 + 参考)✓ ③ §3 component map 改/新/退役 与 T201-T205 Allowed Paths 一致 ✓
- **正确性**:catch-all redirect claim grep-verified(App.tsx:30)· Splitter antd ^5.21(package.json ^5.21.0 · T201 实现核查可用性 note 写入 ADR §2.1)· edge 渲染契约与 ADR-0021 边类型(network/hccs/runs-on/fabric-link/binds-to/pd-pair)逐一对应 · 右栏 dispatch 与 TopologyNode.type enum(cluster/nodepool/node/npu/slice/workload/pod)对应

## Carry-forward

- **P12-T-201 layout shell**:ADR-0022 §2 Decision A region + AntD Splitter(§2.1 实现核查 Splitter 可用性)· §4 (a) 左展开+右展开默认 · DESIGN §2.1 panel state + §6.3 Splitter 持久化
- **P12-T-202 拓扑增强**:ADR-0021 边 + ADR-0022 §2 Decision B ReactFlow · DESIGN §2.2 edge 渲染契约表 + §6.1 focus 算法 · §4 (b) focus 手势 confirm
- **P12-T-203 右栏**:ADR-0022 §2 Decision C dispatch · DESIGN §2.3 dispatch + §6.2 Grafana 映射 · MetricsSection/LogsSection 新组件
- **P12-T-204 顶栏 preset bar**:ADR-0022 §2 Decision A 顶栏 · PresetBar 复用 Deploy/{PresetGrid,DeployWizard}
- **P12-T-205 退役 4 页**:ADR-0022 §2 Decision D · 删页壳 + catch-all 保 deep-link + 复用组件保留 · /poc/topology 一并清理
- **render-verify(plan §8)**:T201-T205 各 ≥1 张 Playwright after-*.png · before-*.png 已 commit 84546c1

## §0a.11 compliance

- §0a.11 docs/DESIGN 例外:main agent 直接做 · strict verify = acceptance 3 项 + component map glob 核对 + catch-all grep
- rhythm(v2):W1 4 task(T001-T004)全 docs/contract gate 完成 · commit 后续 W2(T101 Track A or T104 Track B)· 不停 · 不 push(累积到 phase-12-complete tag)
