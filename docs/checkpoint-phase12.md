# Checkpoint — Phase 12 (M6 平台真实化 + 操作台一体化)

> **landed `phase-12-complete` (2026-06-01)** · 16 task chain T001→T302 · 3 entry sessions(plan → W1+W2 execute → W3+closer execute）。
> Milestone **M6**:aarch64 鲲鹏 + openEuler 真实目标平台(supersede ADR-0001 §13)+ 拓扑数据全保真 + 前端 one-page workspace(吸收 workloads/deploy/metrics/logs 四页)。
> 上游:`docs/phase12-plan.md` · ADR-0019/0020/0021/0022 · 承 `docs/checkpoint-phase11.md` §6 handoff。

---

## 1. 16/16 task deliverable table

| Task | Track | 交付 | commit |
|---|---|---|---|
| T001 | W1 | ADR-0019 Phase 12 entry decisions(spine + M6 naming reset + 3 carry default-defer) | 95eb824 |
| T002 | W1 | ADR-0020 aarch64 鲲鹏 + openEuler target(supersede ADR-0001 §13 amd64-only) | 5e94265 |
| T003 | W1 | ADR-0021 + api-contract 拓扑全保真扩展 + `gen:types` regen(本 phase 唯一契约 RFC) | 4bc7097 |
| T004 | W1 | ADR-0022 one-page workspace UI IA + `frontend/docs/one-page-workspace.md` DESIGN | 9dd75ee |
| T101 | W2·A | 10 Dockerfile multi-arch buildx(`linux/amd64,linux/arm64` · arm64=target) | 40e3a97 |
| T102 | W2·A | arm64 cross-compile CI matrix job(追加 · 不替换 amd64) | d628117 |
| T103 | W2·A | CANN aarch64 matrix + helm `nodeAffinity arch=arm64`(soft)+ install.sh openEuler dnf | b026616 |
| T104 | W2·B | aggregator emit `network`/`hccs`/`runs-on` 边 + NPU `pcieBandwidthGBps`(ADR-0021) | fb68979 |
| T105 | W2·B | mock arch/os arm64 openEuler + PCIE/HCCS/network 带宽 fixtures + schema.json | c102015 |
| T201 | W3·C | layout shell:删 AntSider nav + AntD `Splitter`(可隐藏可拖拽持久化) | 5b8e243 |
| T202 | W3·C | 拓扑增强:network 绿边 + 带宽 hover tooltip + PCIE/HCCS + runs-on + focus/isolate | 21e3585 |
| T203 | W3·C | 右栏 DetailPanel 吸收 workload/pod + 硬件/业务指标 section + 日志 section | 1ef4d2c |
| T204 | W3·C | 顶栏预置应用 bar(hover 详情 + click → DeployWizard) | 9aebf24 |
| T205 | W3·C | 退役 /workloads /deploy /metrics /logs + POC 路由 + i18n cleanup(0 orphan) | e4d0d77 |
| T301 | Closer | Playwright e2e one-page flow(9 test)+ render baseline + helm arm64 渲染校验 | e0c0b54 |
| T302 | Closer | 本 checkpoint + arch/README/candidate-streams + tag `phase-12-complete` | (this) |

---

## 2. 3-track 摘要

### Track A · 平台真实化(aarch64 鲲鹏 + openEuler · *构建 target* 反转)
ADR-0020 翻转 ADR-0001 §13 amd64-only → 真实目标平台 = 华为 Atlas 800 原生配置(Kunpeng 920 host + 昇腾 910B + openEuler)。10 Dockerfile multi-arch buildx(`GOARCH=${TARGETARCH}` 静态跨编译 · CGO off · 无需换 base)· CI 追加 arm64 cross-compile matrix(amd64 保留)· helm `nodeAffinity kubernetes.io/arch=arm64`(**soft** preferredDuringScheduling — kind smoke 仍 amd64)· install.sh openEuler dnf 分支。**amd64 保留本机 dev/CI/render-verify** —— 非 arm64-only。**真鲲鹏运行验证 lab-gated 留 Phase 13+**(Track A 6th carry)· Phase 12 交付 = 交叉编译通过 + buildx 双架构 + helm template arch 亲和渲染校验(T301 实证 9/9 chart)。

### Track B · 拓扑数据全保真(后端 + mock)
ADR-0021 扩契约:NPU `pcieBandwidthGBps`(host↔NPU)+ aggregator emit `network`(node↔node)/`hccs`(npu↔npu 同 hccsGroup ring)/`runs-on`(非 NPU workload→node)边 + edge `bandwidthGBps`/`medium`/`utilization` 属性(单位 GB/s 同轴)。network+hccs 跟 `IncludeFabric` · runs-on 跟 `IncludeWorkloads` gating · toggle off → byte-equivalent 旧图(零回归)。mock set-a/b/c 走 **minimal-diff transform**(非 regenerate · 防 curated 数据丢失)+ generator 同步 emit(future SoT)。

### Track C · 前端 one-page workspace(吸收 4 页)
ADR-0022:5 路由 + AntSider nav → 单页工作台(`/overview`)。AntD `Splitter`(左树 + 中拓扑 + 右栏 · 可隐藏可拖拽 · 持久化)· 拓扑消费 Track B 全保真数据(BandwidthEdge hover + focus/isolate)· 右栏 selection-dispatch(资源→硬件 Grafana 无日志 / 负载→业务 Grafana + 日志 section)· 顶栏 preset bar(复用 DeployWizard)· 退役 4 页 + POC(catch-all 保 deep-link)+ i18n 0 orphan。

---

## 3. ADR forward notes

- **ADR-0019**(entry):spine 三线全 land · M6 naming reset 兑现 · production-hardening cohort 顺延 Phase 13+ · 3 carry default-defer 维持。
- **ADR-0020**(aarch64 + openEuler target):构建 target 反转全 land(Dockerfile/CI/helm/install.sh/mock)· **ADR-0001 §13 amd64-only SUPERSEDED**(arch §6 line 783 forward note 兑现)· 真硬件运行验证 = Track A Phase 13+ carry。
- **ADR-0021**(拓扑全保真):契约 + aggregator + mock + 前端渲染全闭环 · 单位 §4(b) `bandwidthGbps`(switch legacy Gbps)vs `bandwidthGBps`(新 GB/s)统一留后续 · utilization 现 fixture 静态 · 真 telemetry Phase 13+。
- **ADR-0022**(one-page UI IA):4 Decision 全兑现(IA region · ReactFlow 续用 · 右栏 dispatch · 4 页退役 catch-all)· §4 open questions close-out:折叠默认(左展开+右展开)· focus 手势(选项 b 显式 button)· 窄屏 responsive 留后续。

---

## 4. Test posture

- **前端单测**:119(T204 峰值)→ **80**(T205 退役 4 页测试后)· 13→10→**10 file**(Layout/Overview/PresetBar/DeployWizard + 6 component)· 退役页测试随页删 · DeployWizard 覆盖经 `tests/DeployWizard.test.tsx` 保留(直接渲染 wizard)。
- **e2e**:`tests/e2e/tests/workspace.spec.ts` **9 test**(shell+WS · 树选 · NPU 卡+PCIe+指标 · slice · fabric edge · focus · preset wizard · deep-link redirect · WS ffwd)· 替换 5 per-page spec · `npx playwright test` 9/9 green(复用 :8080/:3000)。
- **render-verify**(plan §8 强制 · Phase 11 closer 漏跑的补偿):每 Track C task ≥1 Playwright 截图 · 存 `docs/screenshots/phase12/after-*.png`(11 张:workspace 默认/折叠/fabric/tooltip/focus · NPU/workload 右栏 · preset bar/hover/wizard · retire)· before(5 页 phase-11)对照见 §5。
- **后端**:aggregator 93.5% cover · go vet + gofmt clean(T104)。
- **arch render**:helm-template 9/9 chart 渲出 arm64 nodeAffinity(T301 实证)。
- **build**:`pnpm build` ✓(3679 modules · 无死码)· `>500kB chunk` 是 pre-existing bundle warning(G6 dep 未用 · Phase 13+ G6 重写才用 · code-split 留后续)。

---

## 5. Scope adaptations / 关键路径偏离(render-verify 抓到的)

> Phase 12 §8 把 render-verify 升为 Track C 每 task acceptance 一等公民 —— 抓到 3 个单测漏掉的真 bug:

1. **T201 拓扑画布高度 balloon → 4321px**:`height:100%` 挂在 `minHeight:100vh` 上(允许增长)· 高资源树撑爆 · fitView 把 node 居中到屏外。Fix → workspace flex-fill(T204 进一步去 magic number · root height:100vh + flex 链)。
2. **T202 边从未渲染(handle 缺失)**:自定义 `TopoNode` 无 `<Handle>` → ReactFlow 边一条不画(T108b 起潜伏)· 加隐藏 target/source handle → base 27 边 / fabric on 66 边渲出。
3. **T203 选 workload → 右栏 stale**:DetailPanel 用默认 flags 查 topology · graph 用带 showWorkloads 查 · workload node 不在 DetailPanel 拓扑 → stale。Fix → DetailPanel 读 showFabric/showWorkloads 同参(react-query de-dupe)。

跨 task-Allowed-Path touch(同 owner 同 session · justified · devlog 记):T201 改 TopologyGraph(fitView re-fit)· T202 同文件叠 edge/handle · T204 改 Overview styles(flex 重构)。

---

## 6. Phase 13+ handoff brief

- **Production-hardening cohort**(ADR-0019 §2 Decision B · 原 M6 production-hardening 顺延):Karmada control HA + 完整 OIDC IdP + ClusterQuota webhook B 完整 + Vault Secret + vLLM PD P99 SLA。
- **真 aarch64 集群验证 + 真硬件 stamp**(Track A 6th carry closer):真鲲鹏 920 + 昇腾 910B + openEuler 集群 `helm install` + Pod Ready + 真 NPU 拓扑 + 真 PCIE/HCCS/network telemetry(替换 fixture 静态值)。
- **3 active carry tracks 维持 default-defer**:Track A lab(6th)· Track B Volcano(4th)· Track C Partitionable Devices(3rd · KEP-4815 仍 Beta)。
- **Go v1 ResourceSlice schema migration cohort**(P10-fix-001 carry · 5 module lockstep · 与 K8s 1.36 baseline bump 同期)。
- **Frontend UX Track-2 F01**:G6 5.x 拓扑重写(`@antv/g6` dep 已在 · 若 ReactFlow 在 set-c-stress 大基数渲染瓶颈 materialise)· TopologyView/TopologyGraph props 契约保持便于替换。
- **小尾巴**:bundle code-split(移未用 G6 dep)· bandwidth 单位统一(Gbps vs GB/s)· 窄屏 responsive · EmptyState JSDoc stale 示例 · screenshots README after-* 段。

---

## 7. CI gate(post-tag)

`phase-12-complete` tag push 后 GitHub Actions gate(per memory `feedback_post_tag_ci_gate`):看全 workflow · 修所有 ❌ 直到 dev HEAD 全绿。**注意**:本 phase 新加 `cross-compile-arm64` matrix(10 模块)首次 CI 跑 · 已本机 + T301 helm render 实证 · CI 环境首跑留意。修复链 P12-fix-NNN(若需)。

---

**END · Phase 12 landed · 16/16 · M6 平台真实化 + 操作台一体化兑现。**
