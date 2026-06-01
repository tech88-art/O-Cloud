# P12-T-301 · Playwright e2e one-page flow + render baseline + arch render-check

- **Commit**: (this commit)
- **Date**: 2026-06-01
- **Duration**: 1.5d plan / ~0.5d actual

## Intent

per plan §6 T301 把 5 per-page e2e spec 重写为单页 workspace flow + 刷新 render baseline + arm64 nodeAffinity 渲染校验:
- 删 topology/workloads/deploy/metrics/logs.spec → 1 个 `workspace.spec.ts`(树选 → 拓扑 focus → 右栏资源/负载 + 指标/日志 → 顶栏 preset deploy + deep-link redirect)
- baseline-screenshots.mjs 从截 5 页改截 workspace 关键态(5 页已 redirect)
- helm-template arm64 nodeAffinity 渲染校验(real cluster lab-gated)

## workspace.spec.ts 覆盖(9 test · 全 green)

1. shell:preset bar + 树 + 拓扑 + cluster/node by data-id + **edges 渲染(>0 · T202 handle fix 回归守卫)** + npu≥8 + preset chip≥1 + WS handshake `open`
2. 树选 resource → DetailPanel node card sync
3. NPU 选 → npu card + **PCIe** + **硬件指标 section** + **无 logs section**(ADR-0022 §2.3 dispatch)
4. 双击 NPU → slice children 入图
5. fabric toggle → **bandwidth edge(network/hccs)渲染**(BandwidthEdge hit-path)
6. focus button → 选 resource → 隔离子树(node count 降 + clear-focus 现)
7. preset chip → **deploy wizard 开**(step-mode 可见)
8. **deep-link `/workloads` → redirect `/overview`**(catch-all · 无 404)
9. WS fastforward replay → lastEventAt 更新

## ⚠️ e2e 调试(首跑 6/9 · 修 3）

1. **ReactFlow node `.click()` 被 panner 吞**(NPU 选 + focus 两 test 挂)：ReactFlow pan-on-drag listener 吞合成 `.click()`(topology.spec 老注释有载)→ 改 `dispatchEvent('click')`。focus 加 `toBeEnabled()` 等选择 registered 再点。
2. **`deploy-wizard` testid 在 `.ant-modal-root`(Playwright 判 hidden)**:wizard 实际开了(截图证)· 但 root wrapper AntD 折叠态 → 改断言 `deploy-wizard-step-mode`(modal 内容 · open 才渲 · 可靠 visible 信号)。
- 修后 **9/9 pass(23.7s · 复用 running servers)**。

## baseline-screenshots.mjs(workspace 状态制)

- 5-page PAGES → workspace STATES:`overview`(默认)+ `overview-fabric`(fabric on · 绿/紫/蓝边)· 每态 goto /overview + 等 topology-graph + settle + setup(交互)+ 截 `<phase>-<state>.png`。
- `before-*.png`(phase-11 · 5 页)保留为历史基线 · `after-*` = workspace 态 · T302 checkpoint 配对出 5→1 collapse 对比。
- 实跑 `PHASE=after` → after-overview.png + after-overview-fabric.png 刷新(canonical)。

## arm64 nodeAffinity 渲染校验(plan §6 "若适用")

- **helm-template 渲染实证**(非 cluster · helm v3.16):`helm template demo-backend | grep -A3 kubernetes.io/arch` → `operator: In · values: [arm64]` · **9/9 chart 渲出 arm64 affinity**(T103 soft preferredDuringScheduling)。
- **kind smoke 不加 arm64 cluster 校验**:kind 集群 amd64(soft affinity 不破 amd64 调度)· 真 arm64 鲲鹏集群运行验证 **lab-gated 留 Phase 13+**(plan §7 Track A carry)· 故 "若适用" → 模板级渲染校验足够 · 不加冗余 amd64 kind smoke。

## Verification(strict per-task · P3)

- **e2e** `npx playwright test` → **9/9 passed**(workspace.spec · chromium · 复用 :8080/:3000)
- **arch render** helm-template → 9 chart arm64 affinity 渲出(实证)
- **baseline** after-overview.png + after-overview-fabric.png 刷新(workspace 默认 + fabric 态)
- 删 5 old spec · 无残留 page-route e2e(grep 确认 testDir 仅 workspace.spec)

## Carry-forward

- **T302**(docs整理):更新 `docs/screenshots/phase12/README.md` after-* 段(本 task 聚焦 e2e/baseline · README 属 docs整理 scope)· checkpoint 出 before(5 页)/after(workspace 态)对比表 + DATA 对比(路由 5→1 · bundle · 测试数)。
- **真 arm64 e2e**:lab-gated(Phase 13+ · 真鲲鹏 + 昇腾 910B 集群 helm install + Pod Ready + 真 NPU 拓扑)。
- **e2e 浏览器**:仅 chromium(firefox/webkit 留后续 · CI runner 成本)。

## §0a.11 compliance

- 单 task 串行 · strict verify(e2e 9/9 + arch render + baseline 全过)· commit · **不 push**(累积到 phase-12-complete tag)
- 无 subagent · 无共享契约改动
- rhythm:T301 done → 续 T302(checkpoint + tag phase-12-complete + push → CI gate · 最后一 task)· 不停
