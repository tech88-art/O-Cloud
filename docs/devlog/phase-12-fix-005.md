# P12-fix-005 · active 连线高亮 + 流动动画(Datadog 式 edge emphasis)

- **Commit**: (this commit · post `phase-12-complete`)
- **Date**: 2026-06-02
- **Trigger**: 拓扑视觉打磨 backlog #2(交接 `topology-polish-handover.md` §4 · P1)· 续 fix-004 后用户确认续做。
- **方向**: 选中 / hover 节点时,其 incident 边在"reveal"基础上再**加粗 + 流动虚线动画**,读作"活动节点的实时连接"。

## 业界依据

Datadog Service Map / Device Topology:点选一个服务/设备 → 高亮它的依赖连线 + 连线上动态流动(traffic flow)。本改动把 fix-002 的 contextual reveal 升级为 reveal + emphasis(只对 active 节点的非骨架边)。

## 改动(复用 fix-002 的 `activeEdgeNodeId` · 最小新增)

- **新 `TopologyGraph.module.css`**:`.flowEdge :global(.react-flow__edge-path)` —— `stroke-dasharray:6 4` + `animation:topoEdgeFlow 0.7s linear infinite`(动 `stroke-dashoffset` → 虚线流动)· `@media (prefers-reduced-motion:reduce)` 关动画(a11y · 保留加粗)。动画走 CSS Module(项目 §6:不内联、不 CSS-in-JS)。
- **`displayEdges` 重构**(TopologyGraphInner):骨架(contains)恒显且不强调;非骨架边 —— 非 incident → `hidden`;incident(= `hoveredId ?? selectedNodeId`)→ reveal + `className=styles.flowEdge` + inline `strokeWidth = 基宽 + 1.25`(保留 per-type 基宽层级)。
- 边类型行为:已有 inline dasharray 的(hccs `5 4` / pd-pair `6 4` / runs-on `4 4`)保留自身虚线只加流动;solid 的(network/fabric-link/binds-to)由 class 补 `6 4` dash → 一致流动。stroke 颜色全保留(edgeRenderingFor 不变)。

与 contextual edges 叠加:idle 只骨架 → hover/选中节点 → 其边亮起 + 流动 → 移开复位。

## Verification(strict · per-task)

- typecheck ✓ · lint `--max-warnings 0` ✓ · vitest **84 passed**(+1:选中 node-1 → network 边 `data-emphasized=1`+`data-hidden=0`+strokeWidth>2;非 incident hccs 边 `emphasized=0`+`hidden=1`)· e2e **10/10**(复用 :8080/:3000 · 无回归)。
- **render-verify**(Playwright probe · DOM 实证 + 截图):Fabric ON · 选中 worker-site-a-01 前 `totalEdges=3/flowing=0`(仅 cluster→worker 骨架)→ 选中后 `totalEdges=6/visible=6/flowing=3`(2 network + 1 fabric-link reveal)· `animationName=_topoEdgeFlow_*`(动画生效)· strokeWidth `2.75px`(加粗)· dash `6 4`。证据:`docs/screenshots/phase12/after-edge-flow.png`(worker-01 → worker-02/03 绿色加粗虚线 + → ToR switch 蓝色虚线)。

## Notes / carry

- hover 与 select 共用 `activeEdgeNodeId`(hover 优先),逻辑同一;jsdom 无 hover,单测走 select 路径,render-verify 走 select(hover 等价)。
- 流动是单测无法断言的视觉,靠 render-verify(`getComputedStyle(path).animationName` ≠ none)实证。
- 这是 post-tag polish 第 5 弹(001 卡片化 / 002 降噪 / 003 下钻 / 004 HCCS 环 / **005 连线高亮+流动**)· 本地 commit · **未 push**(待用户批次 · `feedback_push_at_phase_tag_only`)。
- backlog 剩:③面包屑去重(P2) ④同 rank 路由(P2) ⑤workloads 密度 ⑥bandwidth 单位 ⑦降级 pulse(P3)。
