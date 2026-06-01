# ADR-0019: Phase 12 entry decisions(spine = 平台真实化 aarch64 鲲鹏 + openEuler + 拓扑数据全保真 + 前端 one-page workspace · milestone M6 平台真实化 + 操作台一体化 naming reset · production-hardening cohort → Phase 13+ · 3 active carry tracks 维持 default-defer)

- **状态**:Accepted(Phase 12 entry decision lock · per Phase 12 plan P12-T-001 · 2026-06-01)
- **日期**:2026-06-01
- **决策者**:协调者(用户 · 2026-06-01 chat entry meeting 两决策)
- **相关**:`docs/phase12-candidate-streams.md` §Spine candidates(本 ADR §2 Decision A spine = §Spine D naming reset 变体 · 非 §Spine A production-hardening)+ §Active carry tracks(本 ADR §2 Decision C 3 carry tracks 维持 default-defer outcome record)/ ADR-0017 §2 Decision B "M6 在此基础加 production hardening cohort" forward note(本 ADR §2 Decision B naming reset 该 forward note)/ ADR-0001 §13 "不支持 ARM / 鲲鹏"(本 ADR §2 Decision A Track A 反转目标 · supersede 详 ADR-0020 sibling)/ ADR-0011 §3 lab gating 政策(本 ADR §2 Decision C Track A 6th carry 沿用 default-defer policy core · 不修改)/ ADR-0004 inter-node fabric topology(NetworkLink BandwidthGbps/Medium/Utilization/RTT 已有 = Track B network 边复用源 · 详 ADR-0021)/ ADR-0005 pod-in-topology fusion(workload/pod 节点 + binds-to/pd-pair 边 = Track B workload→node 边补充点 + Track C 拓扑基础)/ ADR-0006 wire-type enum regen(TopologyNode/Edge 联合类型 native = Track C 类型安全前提)/ ADR-0013/0014(O2 DMS / Quota · Workload schema 3 indicator P11-T-105 landed = Track C 右栏 workload 信息源)/ `docs/checkpoint-phase11.md` §6 Phase 12+ handoff brief / `docs/demo-runbook.html` Step 1 拓扑 SVG(Track C 视觉北极星)/ `docs/architecture.md` §1.3 phase 路线图(M1-M5 landed · M6 由本 ADR 启航)+ §13 review-table(本 ADR Allowed Paths 含 Phase 12 row "in flight")/ `docs/phase12-plan.md` §1-§9(16 task + 3 track + render-verify posture)/ ADR-0020(T002 sibling · 平台 target supersede ADR-0001 §13)/ ADR-0021(T003 sibling · 拓扑契约扩展)/ ADR-0022(T004 sibling · one-page UI IA)

---

## §1 Context

### §1.1 Phase 11 closer · M5 真生产化 foundation subset milestone landed

Phase 11 落 20 task chain + tag `phase-11-complete`(@ HEAD of dev post-CI gate · per `docs/checkpoint-phase11.md` + arch §13 Phase 11 row)· **M5 真生产化 foundation subset milestone CLOSER**:
- 2 entry ADRs(ADR-0017 4 Decisions + ADR-0018 Karmada deployment topology)
- 6 chart packaging spine 闭环(demo-backend Lease leader-elect + 3 IMS chart + sched-plugin NRT bundle + inference-operator env wire · known-issues #12/#13 close)
- Karmada propagation 第一波 production-grade(host + 2 member kind cluster · 4 PropagationPolicy + cross-cluster informer + ClusterQuota CRD)
- Frontend src/ Workload page extension 完整(3 indicators + backend handler bridge + i18n en-US/zh-CN)
- O2 DMS authn chart wiring(OIDC client + TokenReview SA env · Stream 5 part 1 substrate)
- 3 deferred outcomes(T101 LAB 5th carry + T108 Volcano 3rd defer + T202 Partitionable Devices 2nd defer · 全 default policy)
- 4 fix commits in-phase(P11-fix-001..005 · dev-stack docker-compose + exporter metric + dashboard template-var)

post-tag CI gate 落定 → dev HEAD 全绿 → M5 真完成。

### §1.2 candidate-streams §Spine candidates 留 Phase 12 entry decide

`docs/phase12-candidate-streams.md`(Phase 11 W3 P11-T-203 起草)§Spine candidates 列 4 候选留 Phase 12 entry meeting 拍板:
- **Spine A continuation 真生产化 production-hardening cohort**(高优先级 candidate · ADR-0017 §2 Decision B forward note 预设):Karmada HA + 完整 OIDC IdP + ClusterQuota webhook B 完整 + Vault Secret + vLLM PD 分离 production-grade SLA(P99)
- **Spine B 真硬件-native cohort**(Track A 6th carry trigger 3 路径):lab onboarding + Source.RealAscend + cann-driver-matrix stamp + Partitionable Devices
- **Spine C 生态扩展 cohort**:Volcano gang training-job + O2 IMS R1 v05.00+ migration + adapters
- **Spine D 多 milestone 拆**:若 scope > 1 phase calendar → 多 phase 拆

candidate-streams §Spine candidates 末行明示:"**M6 milestone naming**:留 Phase 12+ entry meeting decide(per ADR-0017 §2 Decision B forward note · 与 Spine 选择联动)"。本 ADR §2 Decision A+B 是该 open 的 close-out。

### §1.3 2026-06-01 用户 entry meeting 两决策

用户 2026-06-01 chat Phase 12 entry meeting 给 **两决策**,redirect Phase 12 spine(非 candidate-streams 预设的 Spine A production-hardening):

- **用户决策 1 · 真实化目标平台**:把 ADR-0001 §13 "仅支持 amd64 / x86_64" 的简化决策,翻转为**真实目标平台 aarch64 鲲鹏(Kunpeng 920)+ openEuler 节点 OS**。rationale = 这是华为 Atlas 800 推理服务器的原生配置(Kunpeng 920 host CPU + 昇腾 910B NPU + openEuler),让样机贴近真实部署形态。→ **Track A 平台真实化**。
- **用户决策 2 · 拓扑数据全保真 + 操作台一体化**:① 拓扑数据走全保真(补 NPU PCIE 带宽 · node↔node network 边 · npu↔npu HCCS 边 · 非 NPU workload→node 边 · edge 带宽/利用率属性供前端 hover)· ② 前端从 5 路由(overview/workloads/deploy/metrics/logs)重构为**单页工作台**(one-page workspace · 资源树侧栏 + 拓扑画布 + 右信息栏 可隐藏可拖拽 · 吸收 workloads/deploy/metrics/logs 四页 · 顶栏预置应用 bar)。→ **Track B 拓扑全保真** + **Track C 前端 one-page workspace**(数据保真喂前端渲染 · 一体化操作台 · 两者耦合)。

**两决策 → 三 co-equal track**:决策 1 = Track A;决策 2 = Track B + Track C(数据保真 + 渲染该数据的操作台一体)。

### §1.4 本 ADR 不涉及

- **ADR-0001 §13 supersede 详细 + 平台 target 级联**(Dockerfile multi-arch / CI arm64 / CANN aarch64 / helm arch 亲和 / install.sh openEuler)— 拆 ADR-0020(T002 sibling)· 本 ADR §2 Decision A 仅 codify spine 含 Track A
- **拓扑契约扩展详**(PCIE 字段 / network/hccs/workload→node 边 / 带宽属性)— 拆 ADR-0021(T003 sibling · 本 phase 唯一 api-contract RFC)
- **one-page UI IA 详**(region 布局 / 右栏 dispatch 状态机 / 4 页退役策略)— 拆 ADR-0022(T004 sibling)+ `frontend/docs/one-page-workspace.md` 模块 DESIGN
- **各 task 详 Allowed Paths / Acceptance** — 由 `docs/phase12-plan.md` §3-§6 task package overview 承载 · 本 ADR §3 仅 enumerate 16 task cross-ref §2 · 不复述
- **ADR-0011 §3 lab gating policy core 修改** — 政策 default 不变(no signal → defer)· §2 Decision C 仅 codify Track A 6th carry posture(Phase 12 平台化 = 构建 target 反转 ≠ 真硬件验证)

---

## §2 Decision

### §2.1 Decision A:Phase 12 primary spine = 平台真实化(aarch64 鲲鹏 + openEuler)+ 拓扑数据全保真 + 前端 one-page workspace

**candidate-streams §Spine candidates close-out**:**非 Spine A production-hardening · 而是 candidate-streams §Spine D "多 milestone 拆" 的 naming reset 变体** —— Phase 12 由用户 2026-06-01 两决策 redirect 到 **UI + 平台真实化 spine**。

**Phase 12 spine 由 3 co-equal track 构成**(per `docs/phase12-plan.md` §0 Goal + §1 Scope summary):

| Track | 描述 | task |
|---|---|---|
| **A · 平台真实化** | amd64-only(ADR-0001 §13)翻转为 aarch64 鲲鹏(Kunpeng 920)+ openEuler 真实目标平台 · 9 Dockerfile multi-arch buildx(`linux/amd64,linux/arm64`)+ CI arm64 交叉编译 + CANN matrix aarch64 + helm `nodeAffinity kubernetes.io/arch=arm64` + install.sh/single-node openEuler + mock `arch/os` + root/project CLAUDE.md §2/§5 改写 | T101 + T102 + T103 +(T002 ADR-0020 + T105 mock arch/os) |
| **B · 拓扑数据全保真** | aggregator 仅 emit `contains/fabric-link/binds-to/pd-pair` → 补 NPU `pcieBandwidthGBps` + `network`(node↔node)边 + `hccs`(npu↔npu)边 + workload→node 边 + edge `bandwidthGBps/medium/utilization` 属性 · aggregator emit + mock fixtures + schema.json | T104 + T105 +(T003 ADR-0021 + api-contract) |
| **C · 前端 one-page workspace** | 5 路由收敛为单页工作台 · AntD `Splitter` 可隐藏可拖拽左树 + 右栏 · 拓扑绿色 network 连线 + edge hover 带宽 + PCIE/HCCS 渲染 + workload→node 连线 + focus/isolate 过滤 · 右栏吸收 workload/pod + 指标 section(硬件/业务 grafana)+ 日志 section + 顶栏 preset bar · 退役 4 页 · 继续 ReactFlow(G6 charter 留后续) | T201 + T202 + T203 + T204 + T205 +(T004 ADR-0022 + frontend DESIGN) |

**为什么不选 Spine A production-hardening**(原 candidate-streams 高优先级 candidate):
- 用户 2026-06-01 明确优先**真实平台 + 操作台一体化**(样机贴近真实部署形态 Atlas 800 + demo 操作体验),而非 production hardening
- Spine A 完整(Karmada HA + 完整 OIDC IdP + Vault Secret + vLLM PD P99 SLA)强依赖**甲方 SLO 输入 + IdP 选定 + 真业务 traffic** —— 这三块 input 当前仍未到位(同 ADR-0017 §2.1 Stream 4 留 Phase 12+ 的 rationale)
- → production-hardening cohort 顺延 **Phase 13+**(§2 Decision B naming reset 后的真 production-hardening milestone)

**为什么是 Spine D 变体而非全新 Spine**:
- candidate-streams §Spine D 已 enumerate "若 scope > 1 phase calendar → 多 milestone 拆" 的可能 —— Phase 12 平台真实化 + UI 一体化即一个独立 milestone slot,production-hardening 顺延是 Spine D "拆" 的实例
- 不是 Spine B 真硬件-native:Phase 12 Track A 是**构建/部署 target** 反转(交叉编译 + helm template 校验可在 amd64 开发机完成),**不**含真鲲鹏 + 昇腾集群运行验证(那是 Track A lab carry · §2 Decision C)

### §2.2 Decision B:Milestone = M6 平台真实化 + 操作台一体化(naming reset · production-hardening cohort → Phase 13+)

**ADR-0017 §2 Decision B forward note "M6 在此基础加 production hardening cohort" 的 naming reset**:

- ADR-0017 §2 Decision B 预设 M6 = production hardening cohort(在 M5 foundation 之上)· candidate-streams §Spine candidates 末行 "M6 naming 留 Phase 12 entry decide · 与 Spine 选择联动"
- 用户 2026-06-01 两决策 redirect spine → **M6 reset 为 "平台真实化 + 操作台一体化"**(working name · per plan §0 Milestone)
- 原 production-hardening cohort(Karmada HA / 完整 OIDC IdP Keycloak/Dex / ClusterQuota webhook B 完整 / Vault Secret cross-cluster / vLLM PD P99 SLA)**顺延 Phase 13+** 作为**真 production-hardening milestone**

**naming reset 不 over-claim**:
- M6 *closes* 平台真实化(arm64/openEuler build target + helm arch 亲和 · 交叉编译 + template 校验)+ 操作台一体化(one-page workspace 吸收 4 页 + 拓扑全保真渲染)
- M6 *不* close 真硬件运行验证(真鲲鹏 + 昇腾集群 · Track A lab carry · §2 Decision C)· *不* close production hardening(Phase 13+)
- Phase 13+ entry meeting 决定 M7 naming + production-hardening cohort 启动 posture

### §2.3 Decision C:3 active carry tracks 维持 default-defer · 与 Phase 12 scope 无 direct dependency

**candidate-streams §Active carry tracks(Phase 12+ entry W1 必评估)outcome record**:

3 active carry tracks 全维持 **default-defer**(与 Phase 12 spine 无 direct dependency):

- **Track A · Lab gating 6th carry**(ADR-0011 §3 + ADR-0017 §2 Decision C trigger 3):
  - **关键 disambiguation**:Phase 12 "平台真实化(Track A spine)" = **构建/部署 target** 反转(amd64-only → arm64/openEuler · 交叉编译 + buildx 双架构 + helm template arch 亲和校验 · 全可在 amd64 开发机完成)· **≠** "真硬件运行验证"(真鲲鹏 920 + 昇腾 910B 集群 `helm install` + Pod Ready + 真 NPU 拓扑 · lab-gated)
  - lab gating 6th attempt posture:**default-defer 维持**(沿用 ADR-0011 §3 policy core · 5 phase consecutive defer P7-P11 后第 6 次)· 真硬件 stamp 留 Phase 13+
  - **唯一 conditional**:若 lab signal materialise at any W entry(用户 chat "lab access available")→ 加真硬件 smoke;否则交叉编译 + helm template 校验交付(同 ADR-0011 §3 lab gating spirit + ADR-0017 §2 Decision C trigger 2 stays armed)
- **Track B · Volcano gang-scheduling 4th carry**(ADR-0010):无 training-job demo signal · 维持 defer(`tests/e2e/kind/phase11/install.sh VOLCANO_ENABLED=1` 路径已 ready · Phase 12 inference + 平台/UI 主导 · 无 training signal)
- **Track C · Partitionable Devices 3rd carry**(ADR-0009):K8s 1.34 baseline 不 unlock + KEP-4815 仍 Beta(GA timing unconfirmed)· 维持 defer(Phase 12 scope 与 K8s 1.36 baseline bump 无 direct dependency)

**为什么 3 tracks 全 defer**:Phase 12 spine(平台 build target + 拓扑数据 + UI)与真硬件 / 训练 demo / K8s 1.36 baseline 均**无 direct dependency** —— 3 carry tracks 的 trigger 条件(lab signal / training signal / K8s 1.36 GA)Phase 12 entry 均未 materialise · ADR-0019 §2 Decision C codify 维持 default-defer,不在 Phase 12 spine。

### §2.4 Decision D:3 track 优先级 + contract gate 串行依赖

**3 track co-equal,但有 contract gate 串行依赖序**(per `docs/phase12-plan.md` §1 uncertainty profile (c) + §2 mermaid DAG):

| 优先级 / 序 | task | 排序 rationale |
|---|---|---|
| **W1 gate(first)** | **T001 ADR-0019 entry**(本 ADR) | 其余 15 task 的 decision gate · spine/milestone/carry/优先级 lock |
| **W1 平台 ADR** | **T002 ADR-0020 aarch64 + openEuler**(supersede ADR-0001 §13) | Track A gate · ADR-0020 平台 target 未 land 前先做 Dockerfile(T101)/CI(T102)/helm(T103)不合规 |
| **W1 契约 ADR + 扩展** | **T003 ADR-0021 + api-contract.yaml**(本 phase 唯一 api-contract RFC · main-agent serial 共享契约) | **contract gate** · Track B 后端(T104)+ Track C 前端 gen:types(T201/T202/T203)未 land T003 前无法 gen types |
| **W1 UI IA ADR** | **T004 ADR-0022 + frontend DESIGN** | Track C gate · one-page IA region + 右栏 dispatch + 退役策略 lock T201-T205 |
| **W2 Track A ∥ Track B** | **T101/T102/T103**(Track A · deploy+docs+configs)∥ **T104/T105**(Track B · backend+configs) | 跨模块可 parallel if user batch · default serial · 注 T105 mock schema.json 是共享契约 → main-agent serial · T105 + T103 都碰 `configs/` 但 T103 不碰 mock |
| **W3 Track C** | **T201**(layout shell · gate)→ **T202/T203/T204/T205** | T201 必 land 后 T202-T205 才有 shell 挂载 · 同 `frontend/**` 模块 default serial · 依赖 T003 gen:types + T104 后端边 + T004 UI IA |
| **Closer** | **T301**(e2e one-page)→ **T302**(checkpoint + tag) | T301 全链 land 后 run · T302 docs-only + tag `phase-12-complete` |

**§0a.5 chat+ADR self-RFC posture**:用户 2026-06-01 两决策即 entry meeting chat 批准(§0a.5 step 1-2)· ADR-0019/0020/0021/0022 是 ADR-side codification(§0a.5 step 3 audit trail)· 共享契约改动(api-contract.yaml T003 · schema.json T105)**只能 main-agent 串行**(per root CLAUDE.md §11 + §6)。

**本 phase 无 W-entry decision gate**(与 Phase 11 T101/T108/T202 不同):3 active carry tracks 与 Phase 12 scope 无 direct dependency(§2 Decision C)· 唯一 conditional = Track A 真 aarch64 lab signal(§2 Decision C · §4 (a))。

---

## §3 Phase 12 scope detailed enumeration(16 task · 4 W1 + 5 W2 + 5 W3 + 2 Closer)

**cross-ref `docs/phase12-plan.md` §2 task package overview**(不在本 ADR 复述详 Allowed Paths / Acceptance):

```
W1 Foundation(4 task · docs/contract gate · 全 main-agent serial · §2 Decision D)
├── P12-T-001  ADR-0019 Phase 12 entry decisions(本 ADR · spine + milestone M6 reset + carry-defer + 优先级 + contract gate)
├── P12-T-002  ADR-0020 aarch64 鲲鹏 + openEuler target(supersede ADR-0001 §13 · §2 Decision A Track A gate)
├── P12-T-003  ADR-0021 + api-contract.yaml 拓扑全保真扩展(§2 Decision A Track B contract gate · 唯一 api-contract RFC)
└── P12-T-004  ADR-0022 + frontend/docs/one-page-workspace.md(§2 Decision A Track C UI IA gate)

W2 Track A 平台化 ∥ Track B 后端数据(§2 Decision D · 跨模块 parallel if user batch · default serial)
├── P12-T-101  [Track A] 9 Dockerfile multi-arch(backend + 7 operator + exporter · buildx linux/arm64 + GOARCH)
├── P12-T-102  [Track A] CI arm64 交叉编译矩阵 + build-images buildx(若 workflow 存在)
├── P12-T-103  [Track A] CANN matrix aarch64 + helm arch 亲和 + install.sh/single-node openEuler
├── P12-T-104  [Track B] backend aggregator emit network/hccs/workload→node 边 + PCIE + 带宽属性 + model/handler
└── P12-T-105  [Track B] mock-data set-a/b/c arch/os + PCIE/HCCS/node 互联带宽 fixtures + schema.json(共享 schema · serial)

W3 Track C 前端 one-page(§2 Decision D · 依赖 T003 gen:types + T104 后端边 + T004 UI IA)
├── P12-T-201  Layout shell:删 AntSider nav + 路由收敛 + AntD Splitter 可隐藏可拖拽左树 + 右栏
├── P12-T-202  拓扑增强:绿色 network 连线 + edge hover 带宽 tooltip + PCIE/HCCS 渲染 + workload→node 连线 + focus/isolate 过滤
├── P12-T-203  右栏:DetailPanel 吸收 workload/pod + 指标 section(硬件/业务 grafana toggle)+ 日志 section(负载/容器 toggle)
├── P12-T-204  顶栏预置应用 bar(PresetGrid/DeployWizard fold + hover 详情 + deploy)
└── P12-T-205  退役 /workloads /deploy /metrics /logs 路由 + i18n cleanup + Vitest 更新

Closer(2 task)
├── P12-T-301  Playwright e2e one-page flow 重写 + kind smoke arch 校验(若适用)+ render-verify 截图
└── P12-T-302  docs 大整理 + checkpoint-phase12 + tag phase-12-complete + M6 milestone announcement
```

**Task count**:16 · per `docs/phase12-plan.md` §2 task package overview。**Render-verify(plan §8)**:Track C 各 task(T201-T205)acceptance 必含 ≥1 张 Playwright `after-*.png` 截图 · 前基线 `before-*.png` 已在执行 session 起手前 capture(commit 84546c1 · 5 页)。

---

## §4 Open questions

### (a) 真 aarch64 lab 验证 posture(Phase 12 交叉编译 vs Phase 13+ 真硬件)

Track A 平台真实化是**构建 target** 反转 · 真硬件运行验证(真鲲鹏 920 + 昇腾 910B 集群)posture 仍 open:
- 选项 a:Phase 12 仅交叉编译(`GOARCH=arm64 go build`)+ buildx 双架构镜像 + helm template arch 亲和渲染校验 · 真硬件 stamp 留 Phase 13+(default · 同 ADR-0011 §3 lab gating spirit + §2 Decision C Track A 6th carry)
- 选项 b:若 lab signal materialise at any W entry → 加真硬件 smoke(Track A lab carry light-up · §2 Decision C 唯一 conditional)

**当前倾向**:选项 a(Phase 12 交叉编译 + template 校验 · 真硬件留 Phase 13+)· lab signal 未 materialise 则 6th carry · per §2 Decision C + ADR-0011 §3 default-defer policy。

### (b) 退役 4 页(/workloads /deploy /metrics /logs)是否保留 deep-link fallback

Track C one-page workspace 吸收 4 页逻辑后,4 路由处置:
- 选项 a:redirect → `/`(保留 deep-link · 避免外部书签 / demo-runbook 链接 404 · POC 阶段保守)
- 选项 b:直接删路由(逻辑已 fold 入 panel/bar · 死路由清理彻底 · 无 unused route)

**当前倾向**:倾向选项 a(redirect → `/`)· 但**最终由 T004 ADR-0022 §2 Decision D codify** + T205 实施 · 需核查 demo-runbook.html / e2e spec 是否有 4 页 deep-link 依赖(有 → redirect 保留;无 → 可删)。

### (c) Splitter 折叠态默认(左树展开 / 右栏收起?)

one-page workspace 左资源树 + 右信息栏均 AntD `Splitter` 可隐藏可拖拽,初始折叠态 open:
- 左资源树:默认展开(资源树是主导航入口 · 用户先选资源才有右栏内容)
- 右信息栏:默认展开 or 收起?(未选资源时右栏空 · 收起更简洁 · 但选中后需展开)

**当前倾向**:左树默认展开 + 右栏默认展开(选中前显 empty/提示态 · 维持三状态 frontend CLAUDE §4.8)· 宽度/折叠态 Zustand 持久化 · **最终由 T004 ADR-0022 §4 + T201 实施 confirm**。

---

## §5 引用

### 上游(本 ADR 决策依据)

- `docs/phase12-candidate-streams.md` §Spine candidates(本 ADR §2 Decision A spine = Spine D naming reset 变体 close-out)+ §Active carry tracks(§2 Decision C 3 carry tracks outcome record)
- ADR-0017 §2 Decision B forward note "M6 在此基础加 production hardening cohort"(本 ADR §2 Decision B naming reset 该 note)
- ADR-0001 §13 "不支持 ARM / 鲲鹏"(本 ADR §2 Decision A Track A 反转目标 · supersede 详 ADR-0020 sibling)
- ADR-0011 §3 lab gating 政策(本 ADR §2 Decision C Track A 6th carry 沿用 default-defer · policy core 不修改)
- ADR-0004 / ADR-0005 / ADR-0006(本 ADR §2 Decision A Track B 拓扑边复用源 + Track C 类型前提 · 详 ADR-0021)
- ADR-0013 / ADR-0014(Workload schema 3 indicator P11-T-105 landed · 本 ADR §2 Decision A Track C 右栏 workload 信息源)
- `docs/checkpoint-phase11.md` §6 Phase 12+ handoff brief
- `docs/demo-runbook.html` Step 1 拓扑 SVG(本 ADR §2 Decision A Track C 视觉北极星)
- `docs/architecture.md` §1.3 phase 路线图(M1-M5 landed · M6 由本 ADR 启航)+ §13 review-table(本 ADR Allowed Paths 含 Phase 12 row "in flight")

### 下游(本 ADR 后续工作 / 触发 Phase 12 execution)

- P12-T-002 ADR-0020 aarch64 + openEuler target(本 ADR §2 Decision A Track A · supersede ADR-0001 §13)
- P12-T-003 ADR-0021 + api-contract.yaml 拓扑全保真扩展(本 ADR §2 Decision A Track B · contract gate · §2 Decision D)
- P12-T-004 ADR-0022 + frontend DESIGN one-page UI IA(本 ADR §2 Decision A Track C · §4 (b)(c) open question codify)
- P12-T-101/T102/T103 Track A 平台化(本 ADR §2 Decision A Track A · §2 Decision D W2)
- P12-T-104/T105 Track B 后端数据 + mock(本 ADR §2 Decision A Track B · §2 Decision D W2)
- P12-T-201..T205 Track C 前端 one-page(本 ADR §2 Decision A Track C · §2 Decision D W3 · §4 (b)(c) 实施)
- P12-T-301 e2e one-page + render-verify(本 ADR §3 render-verify posture)
- P12-T-302 Phase 12 checkpoint + tag(本 ADR §2 Decision B M6 平台真实化 + 操作台一体化 milestone CLOSER announcement · §2 Decision C 3 carry tracks outcome stamp)
- Phase 13+ entry meeting(本 ADR §2 Decision B production-hardening cohort 顺延 + §2 Decision C Track A 真 aarch64 lab carry closer 评估窗口)

### 上游 commit chain(决策时 grep-verified)

- Phase 11 tag `phase-11-complete`(HEAD of dev post-CI gate · per checkpoint-phase11)
- Phase 12 plan @ `7e9b1d2`(`docs/phase12-plan.md` 初版 平台真实化 + 全保真 + one-page)+ `d382a59`(Dockerfile multi-arch 保留本机 amd64 验证 + §8 render verify)+ `e61469e`(§8 render-verify Playwright + 前/后基线)+ `84546c1`(Phase 12 render baseline 5 before-*.png + capture script)
- 本 ADR 是 Phase 12 plan §3 P12-T-001 deliverable

---

**END of ADR-0019**
