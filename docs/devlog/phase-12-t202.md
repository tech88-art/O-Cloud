# P12-T-202 · 拓扑增强 — network 绿边 + 带宽 hover + PCIE/HCCS + runs-on + focus/isolate

- **Commit**: (this commit)
- **Date**: 2026-06-01
- **Duration**: 2.5d plan / ~1d actual

## Intent

per ADR-0021/0022 消费 T104/T105 全保真拓扑数据,前端渲染:
- ① node↔node `network` 绿边 + hover 带宽 tooltip;npu↔npu `hccs` 紫虚线 + hover;node↔switch `fabric-link` 蓝(复用 + 升级为 hover);节点内 PCIE(host↔NPU)hover
- ② 非 NPU workload→node `runs-on` 边(灰虚线 · 区别 binds-to 青实线)
- ③ focus/isolate:选资源 → 只显其 contains 子树 + 落点负载 · 隐藏无关 · 可清除

## ⚠️ 最关键发现 · 边从未渲染过(handle 缺失 · 跨 phase 潜伏 bug)

render-verify 首跑:**totalEdges = 0**(连 contains 树边都没有 · 仅 nodes)。DOM probe 确认 `.react-flow__edge-path = 0` · `.react-flow__edges` 容器存在但空 · 无 console error。

**根因**:ReactFlow 自定义 node **必须含 `<Handle>` 锚点**,否则边无处连 → 一条都不渲染(官方约定)。`TopoNode`(T108b 引入的自定义 node 渲染器)是纯 `<div>` · **从无 Handle** → **边自 T108b 起就从未渲染过**(phase 1-11 拓扑只有孤立 node · 无连线 · before-overview.png 复看确认)。这是潜伏跨 phase 的 bug · T201 render-verify 也没专门查边(只查 node 渲染)。

**Fix**:`TopoNode` 加 1 对隐藏 Handle(target=Top · source=Bottom · `isConnectable=false` · opacity 0)· dagre TB 布局正好 top→bottom。复验:base 27 边 · fabric on 66 边(含 30 条 bandwidth 边)· 全渲染。**T202 的"绿边"前提就是先让边能渲染** —— 否则后续所有 edge 工作都是 no-op。

## Key decisions

- **BandwidthEdge 自定义 edge**(`BandwidthEdge.tsx`):network/hccs/fabric-link 用它(`type:'bandwidth'` + `edgeTypes` 注册)· 渲 BaseEdge + 透明 16px hit-path(细线 hover 宽容)+ EdgeLabelRenderer tooltip 显 bandwidthGBps/medium/utilization/hccsGroup · 属性缺失则不显 tooltip(三状态 · 不崩)。
- **边样式**(edgeRenderingFor · greppable 表已更新):network=绿 `#52c41a` 2px 实 · hccs=紫 `#722ed1` 1.5px 虚 · fabric-link=蓝 `#69b1ff` 1.5px 实 · runs-on=灰 `#8c8c8c` 1px 虚(区别 binds-to 青实)· 其余默认灰。
- **PCIE 走 node hover title**(非独立 edge):NPU node title append `· PCIe 32 GB/s`(实证 "Ascend910B#0 · PCIe 32 GB/s")· DetailPanel 的 pcie 留 T203 prominent 呈现。DESIGN §2.2 允许"npu 节点呈现"。
- **focus 走显式 button(ADR-0022 §4(b) 选项 b)**:画布 ReactFlow `<Panel top-right>` 按钮锚定当前 selection(`聚焦选中` ↔ `退出聚焦`)· **不用手势**(避与双击展开切片冲突)。filter 算法(DESIGN §6.1):anchor + contains 子树 BFS + 一跳 binds-to/runs-on/allocated 落点负载 + pd-pair 伙伴 · stale anchor 不存在则返回全图(不 blank)。store 加 `focusedNodeId`/`setFocusedNode`。

## Path / scope notes

- **TopologyGraph.tsx 是 T202 Allowed Path**(本 task owner)· T201 已先加 ResizeObserver re-fit(carry-forward),本 task 在其上叠加 edge/handle/focus —— 无冲突。
- **未 export 纯函数 `filterTopologyForFocus`/`layoutWithDagre`**:react-refresh/only-export-components lint(`--max-warnings 0`)禁止组件文件混导出非组件。改为**经组件测**(Overview.test stub 暴露 edges + 渲 Panel children)→ 真实路径覆盖 focus filter + edge 分类 + focus toolbar · 不破 lint · 不做大文件抽取(T202 风险控制)。

## Verification(strict per-task · P3)

- **typecheck** ✓ · **lint**(`--max-warnings 0`)✓ · **test** `vitest run` → **13 files / 114 passed**(+3 T202:edge 分类 network→bandwidth/green · focus filter 隐藏无关 node · focus toolbar 按钮 set/clear)
- **render-verify**(Playwright · 非 Preview MCP · 实证非臆测):
  - `after-overview-fabric.png`:绿 network(worker↔worker↔worker)+ 紫虚 hccs(npu↔npu)+ 蓝 fabric(ToR→worker)+ 灰 contains 树 + switch node + degraded NPU · 全渲染
  - `after-edge-bandwidth-tooltip.png` + 文本实证:hover network 边 → **"节点间网络 | 带宽: 25 GB/s | 介质: roce | 利用率: 18%"**
  - PCIE node title 实证:`[data-testid=topo-node-npu] [title]` = **"Ascend910B#0 · PCIe 32 GB/s"**
  - `after-overview-focus.png`:focus Ascend910B#2 → node 29→1(DOM 实证)+ `退出聚焦` 按钮 + 右栏 NPU 详情
  - edge handle fix 实证:base 27 边 · fabric on 66 边(30 bandwidth)

## Carry-forward

- **T203**(右栏):DetailPanel 加 pcieBandwidthGBps prominent 显示(本 task 仅 node hover title)· workload dispatch + 指标/日志 section。
- **同 rank 边路由**:network(worker↔worker 同 rank)/ hccs(npu↔npu 同 rank)用 top/bottom handle → bezier 略绕(可见但非最优)· 若要水平直连需加 left/right handle + per-type sourceHandle/targetHandle(T202 不做 · 可见性已满足 · 视觉优化留后续)。
- **focus re-center**:T201 ResizeObserver re-fit 让 focus 后画布自动 re-fit 到子集(nodesAfterFocus=1 时单 node 居中)· 与 focus 协同正常。
- **gating 对齐**(handover §3):network+hccs 跟 showFabric · runs-on 跟 showWorkloads · 前端 toggle 已对齐(toggle on 才 emit → 才渲)。

## §0a.11 compliance

- 单 task 串行 · strict verify(typecheck+lint+test+render-verify 全过)· commit · **不 push**(累积到 phase-12-complete tag)
- 无 subagent · 无共享契约改动(消费既有契约 · 不动 api-contract.yaml → 无需 gen:types)
- rhythm:T202 done → 续 T203(右栏)· 不停
