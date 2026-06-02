# P12-fix-001 · 站点视图运行手册化(runbook-style site view)

- **Commit**: (this commit · post `phase-12-complete`)
- **Date**: 2026-06-01
- **Trigger**: 用户反馈 —— "站点视图渲染效果应该是类似 runbook 手册中的拓扑图示,清晰明了,而不是 demo 中的粗糙视图"。

## Intent

T202 的拓扑用 ReactFlow dagre 自动布局 + 通用节点框(cluster/node/npu 各为一个 ranked box),视觉"粗糙"且 sprawl。`docs/demo-runbook.html` Step 1(ADR-0022 / one-page-workspace DESIGN §7 明列的"视觉北极星")是 curated SVG:**worker 卡片 + 卡内 NPU 圆点 2×N NUMA 网格**。本 fix 把站点视图重做成 runbook 同款。

## 改动(`TopologyGraph.tsx` 单文件)

- **去 dagre**:删 `dagre` import + `layoutWithDagre` → 新 `layoutSiteTopology`(structured · 纯函数):cluster pill 居中顶 · worker 卡片横排 · 每卡 NPU 圆点 2×4 NUMA 网格(卡内定位 · NUMA-0 上 / NUMA-1 下)· switch/workload/pod/slice 分行排上/下。
- **3 个新 renderer**:`ClusterNode`(圆角 pill · `cluster · site` · site 读 `attributes.location`="site-a-shanghai")· `WorkerCardNode`(卡片 chrome:蓝边 + 标题 + "N× Ascend 910B" + NUMA 标签)· `NpuDotNode`(圆点 · 绿健康/红降级 · 选中 ring · hover title 含 PCIe)。node type 映射:cluster→cluster · node/nodepool→worker · npu→npudot · 其余→TopoNode(switch/workload/pod/slice 保留原卡)。
- **NPU 仍是真 ReactFlow 节点**(zIndex 3 浮在卡片上)→ 点选 / hccs/binds-to 边 / focus / 双击展开切片**全保留**(没改交互模型)。
- **edges**:drop worker→npu contains(NPU 在卡内 · 连线是丑 stub · 容器关系靠视觉)· 其余(cluster→worker contains · network 绿 · hccs 紫 · fabric 蓝 · runs-on/pd/binds-to)不变。
- **fitView**:minZoom 0.55→0.5 · padding 0.05→0.12(适配卡片行宽)。

## Verification(strict)

- **typecheck + lint**(`--max-warnings 0`)✓ · **vitest** 10 files / **80 passed**(node 仍按 id 渲染 · 单测零改)· **e2e** `npx playwright test` **9/9 passed**(npu 选 / 双击 slice / fabric bandwidth edge / focus / preset / deep-link 全过)。
- **render-verify**(Playwright · 3 态):
  - `after-siteview.png`(默认 · hero):cluster pill "cluster-prod-a-01 · site-a-shanghai" + 3 worker 卡 × 8 NPU 圆点 2×4 NUMA 网格 + worker-site-a-02 红降级点 + cluster→worker 干净连线 —— **runbook 同款**。
  - `after-siteview-fabric.png`:+ ToR-Site-A-01 switch(⚡)+ 蓝 fabric-link + worker↔worker 绿 network 边(30 bandwidth edge)· 不 sprawl。
  - 实证:3 卡 / 24 圆点 / clusterText="cluster-prod-a-01 · site-a-shanghai" / 圆点 borderRadius 50%。

## Notes / carry

- **hccs intra-card 边**:npu↔npu 同卡 · fabric on 时是卡底小弧线(轻微 · 可接受)· 若要更净可后续给 NPU 圆点加侧 handle 直连。
- **workloads-on(全开)态**:ReactFlow 自动 fit 缩小 · workload/pod 分行 · 信息密但有序(max-detail 态 · 非 hero)。
- 这是 **post-tag polish**(phase-12-complete 已 push + CI 全绿)· 单文件 · 不动契约/后端/测试结构。
