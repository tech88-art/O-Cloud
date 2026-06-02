# 交接 · 拓扑视觉打磨 session — for the next session

> **2026-06-01** · 本 session 完成 Phase 12 全部任务(T201→T302 · 16/16 + tag `phase-12-complete`)+ 3 个拓扑 polish(站点卡片化 / 连线降噪 / 分层下钻),全部 push 到 `origin/dev` · CI 全绿。下一 session **专注拓扑视觉打磨**。本文是接手入口,读完即可开工。

---

## 0. TL;DR

- **进度**:Phase 12 完整交付 + 拓扑已从"粗糙 dagre 图"重做成 **runbook 同款站点视图**(worker 卡片 + NPU 圆点 NUMA 网格)+ **contextual edges 降噪** + **点击下钻 + 面包屑**。
- **Git**:branch `dev` · HEAD `75807c1` = `origin/dev`(已 push)· **工作树干净** · CI gate 全绿(CI ✓ + e2e-kind ✓ · helm-lint 纯前端改动未触发)。
- **下一步**:**只打磨拓扑视觉**(下方 §4 backlog)· 单文件为主(`TopologyGraph.tsx`)· 每改必 render-verify(§5)。
- **北极星**:`docs/demo-runbook.html` Step 1 拓扑 SVG(§3 已对齐)+ 业界 focus+context / drill-down 共识(§6)。

---

## 1. 拓扑现状(本 session 已做 · 3 个 polish commit)

| commit | 做了什么 |
|---|---|
| `ca7fb11` P12-fix-001 | **站点卡片化**:去 dagre 自动布局 → `layoutSiteTopology` 结构化布局。cluster pill(`cluster·site`)顶部 · worker = **卡片**(蓝边 + 标题 + `N× Ascend 910B` + NUMA-0/1 标签)横排 · NPU = **圆点**(绿健康/红降级)2×4 NUMA 网格 in 卡内 · switch/workload/pod/slice 分行 |
| `3b349e4` P12-fix-002 | **连线降噪 contextual edges**:contains 骨架(cluster→worker)常显 · 跨节点边(network/hccs/fabric/runs-on/binds-to/pd)默认 `hidden` · 仅 active 节点(hover 优先否则 selected)的边显示 → Fabric+Workloads 全开 idle 也只剩骨架(33→3 边),无毛球 |
| `75807c1` P12-fix-003 | **点击下钻 + 面包屑**:dbl-click 节点 = 下钻(setFocusedNode · 复用 `filterTopologyForFocus`)· 面包屑(Panel top-left)`站点 → cluster → worker → npu` · 点 crumb 下钻该层 · 站点 crumb 返回 · 下钻 NPU 自动展开切片 |

render-verify 截图证据(repo 内):`docs/screenshots/phase12/after-siteview.png`(默认 hero)· `after-ctx-idle.png`(全开无毛球)· `after-ctx-selected.png`(选节点显其边)· `after-drill-node.png`(下钻 + 面包屑)。

---

## 2. TopologyGraph 架构(单文件:`frontend/src/components/TopologyGraph/TopologyGraph.tsx`)

读这个文件即可。关键符号(行号近似,以实际为准):

- **`layoutSiteTopology(topology, selectedNodeId)`** (~661):结构化布局核心。worker 卡片横排 + NPU 圆点 in-card 2×4 网格 + cluster pill + switch/workload/pod/slice 分行。返回 ReactFlow `Node[]` + `Edge[]`。**drop worker→npu contains 边**(NPU 在卡内 · 容器关系靠视觉)。已**去 dagre**(纯手工定位)。
- **renderers**:`ClusterNode`(pill ~382)· `WorkerCardNode`(卡片 chrome ~423 · NPU 圆点是独立节点浮在卡上 · zIndex)· `NpuDotNode`(圆点 ~481 · 绿/红 + 选中 ring + PCIe hover title)· `TopoNode`(~256 · switch/workload/pod/slice 仍用旧卡)· `BandwidthEdge.tsx`(network/hccs/fabric hover tooltip)。`rendererTypeFor()` (~519) 映射 topology type → renderer。
- **contextual edges**(降噪 · TopologyGraphInner ~895):`hoveredId`(local state)· `activeEdgeNodeId = hoveredId ?? selectedNodeId` · `displayEdges` 给非骨架且非 incident 的边打 `hidden` · ReactFlow `onNodeMouseEnter/Leave`。
- **focus/drill**:`focusedNodeId`(store)+ `filterTopologyForFocus`(~590 · anchor + contains 子树 + 落点负载)· dbl-click → setFocusedNode(TopologyView)· 下钻 NPU 时 `effectiveExpanded` 自动加它(露切片)。
- **breadcrumb**(~ in Inner):由 focus 节点 contains 祖先链派生 · AntD `Breadcrumb` in Panel top-left · `data-testid="topology-breadcrumb"` / `breadcrumb-site`。
- **常量**:`CARD_W=188 CARD_H=196 CARD_GAP=44 NPU_CIRCLE=26 NPU_GRID_COLS=4` · `FIT_VIEW_OPTIONS={minZoom:0.5,padding:0.12}`。
- **配色**(与 runbook 同):绿 `#52c41a` · 红 `#ff4d4f` · 蓝 `#1677ff` · PD 橙 `#ff7a45` · HCCS 紫 `#722ed1`。
- **fitView re-fit**:TopologyGraphInner 有 ResizeObserver re-fit(P12-T-201 · Splitter 异步测量后重 fit · 别删)。

数据:NPU 节点 `attributes.pcieBandwidthGBps`/`hccsGroup` · 边 `attributes.bandwidthGBps/medium/utilization`(ADR-0021)· cluster 节点 `attributes.location`="site-a-shanghai"。

---

## 3. 视觉北极星 = `docs/demo-runbook.html` Step 1

内联 SVG(~line 396-468):cluster·site 标题 · 3 worker 卡(140×190)· 卡内 8 NPU 圆点(2×4 · r=9)· NUMA-0/1 标签 · PD pair 橙虚曲线 · fabric 浅蓝虚线短链。**当前站点视图已对齐此设计**(§1 fix-001)· 进一步打磨向它看齐 + §6 业界手法。

---

## 4. 打磨 backlog(下一 session 的活 · 优先级建议)

用户明确要的打磨方向(我已提议 · 待做):
1. **节点内 HCCS ring**(P1):下钻进 worker 时,把它的 npu↔npu HCCS 默认画成卡内环/弧(当前 HCCS 也走 contextual 隐藏 · 下钻层应默认显本节点 HCCS · runbook 的"HCCS group 共址"是调度卖点)。
2. **active 节点连线高亮 / 流动动画**(P1 · Datadog 式):选中/hover 时其边加粗 + 可选 stroke-dash 流动(BandwidthEdge 已有 hover · 加 selected 强调)。
3. **面包屑去重**(P2):`站点` + `cluster-prod-a-01` 单集群时语义重复 · 合并(或单集群时省 cluster crumb)。
4. **同 rank 边路由**(P2):network(worker↔worker 同排)/ hccs(npu↔npu 同卡)当前用 top/bottom handle → bezier 略绕 · 加 left/right handle + per-type sourceHandle/targetHandle 走水平直连更净。
5. **workloads 全开态密度**(P3):workload/pod 分行 + ReactFlow 自动缩小 · 信息密 · 可考虑 workload 卡更紧凑 / 分组 / 仅下钻显。
6. **bandwidth 单位统一**(P3 · ADR-0021 §4b):switch legacy `bandwidthGbps`(Gbps)vs 新 `bandwidthGBps`(GB/s)· tooltip 统一标注。
7. **空状态/降级动画**(P3):红降级 NPU 加 pulse(runbook `.npu.deg` 有 `animation:pulse`)。

> 注意 P4 横向:改 NPU 圆点/卡片样式时,3 个 renderer(Cluster/Worker/Npu)+ TopoNode + BandwidthEdge 配色保持一致(集中常量,别散弹)。

---

## 5. 怎么 render-verify(必做 · plan §8 纪律)

```
# 双进程 dev stack(本 session 可能还在跑 :8080/:3000 · 否则启动)
终端1: cd backend && ./bin/demo-backend.exe -c configs/config.dev.yaml   # mock :8080
终端2: cd frontend && pnpm dev                                            # Vite :3000(HMR 改 .tsx 即时热刷)
浏览器: http://localhost:3000
```
- **截图用 Playwright**(写临时 `tests/e2e/probe-*.mjs` · 从 **repo 根**跑 `node tests/e2e/probe-*.mjs` · 用完删)· **不要用 Preview MCP screenshot**(本 app WS 长连 · network-idle 不达 · 5 次 1 成)。
- probe 套路:`page.goto('http://localhost:3000/overview', {waitUntil:'domcontentloaded'})` → `waitForSelector('[data-testid="topology-graph"]')` → `waitForTimeout(3500)` → 交互(toggle/click/dblclick · ReactFlow 节点用 `dispatchEvent('click')` 不是 `.click()` · panner 吞合成 click)→ `page.screenshot()` + `page.evaluate()` 读 DOM 实证。
- 关键 testid:`topology-graph` / `topo-node-{cluster,node,npu}` / `topo-node-slice` / `bandwidth-edge-hit-*` / `topology-breadcrumb` / `breadcrumb-site` / `topology-focus-selected` / `fabric-toggle-switch` / `workloads-toggle-switch`。

每改必跑(strict verify · per-task):
```
cd frontend && pnpm typecheck && pnpm lint && pnpm test          # 80 unit
cd tests/e2e && npx playwright test                               # 10 e2e(复用 :8080/:3000)
```

---

## 6. 业界设计参考(本 session 已调研 · 北极星补充)

- **focus+context / 聚焦邻居**(降噪共识):默认不全画 · 选中才显其连线。Weave Scope(层间下钻)· Datadog Device Topology(点设备才显依赖 + Group By + 默认隐藏未监控)· Cambridge Intelligence("为用户工作流设计,非照搬数据模型")。
- **semantic zoom / drill-down**:Weave Scope 拓扑层下钻 · "所有信息可达但不同时可见"。**当前已实现 click-to-drill + breadcrumb + contextual edges** · 打磨在其上做(动画 / HCCS ring / 路由)。
- 源:datadoghq.com/blog/service-map · docs.datadoghq.com/network_monitoring/devices/topology · (Weave Scope / Cambridge Intelligence hairball)。

---

## 7. Git + push 协议

- 工作树干净 · HEAD `75807c1` = origin/dev · CI 绿。
- **打磨提交**:每个视觉改进单独 commit(`fix(frontend): ...` + Devlog 行 + co-author)· 跑全 verify · **按用户节奏 push**(memory `feedback_push_at_phase_tag_only`:不每 commit 主动 push · 用户说"提交"再推 · 推后看 CI gate `feedback_post_tag_ci_gate`)。
- creds:Windows cmgr 已存 · 直接 `git push` 通(`reference_github_creds`)· 别 embed token。
- CI 状态查(无 gh CLI):`curl -H "Authorization: Bearer $TOKEN" api.github.com/repos/tech88-art/O-Cloud/actions/runs?per_page=6`(TOKEN 从 `git credential fill`)· **不要按 short-SHA 过滤**(API 要 full SHA · 本 session 踩过坑:short-SHA 返回空 → JSON 解析报错)。

---

## 8. 下一 session 启动 checklist

```
1. 读本文(docs/devlog/topology-polish-handover.md)
2. 读 frontend/CLAUDE.md(§6 样式 · §7 TopologyGraph 约定)+ 根 CLAUDE.md
3. 看 docs/demo-runbook.html Step 1 拓扑(北极星)+ docs/screenshots/phase12/after-siteview.png(现状)
4. 读 frontend/src/components/TopologyGraph/TopologyGraph.tsx(§2 符号地图)+ BandwidthEdge.tsx
5. git fetch && git status(确认 dev 干净 · = origin/dev 75807c1)
6. 启 dev stack(§5)· 浏览器开 /overview 看现状
7. 从 §4 backlog 挑一项 · 单 commit · 每改 render-verify(§5)· typecheck+lint+test+e2e 全过
```

---

**END · 拓扑已 runbook 化 + 降噪 + 可下钻 · 下一 session 在此基础上打磨视觉(HCCS ring / 连线动画 / 路由 / 面包屑)· 单文件为主 · 每改截图验证。**
