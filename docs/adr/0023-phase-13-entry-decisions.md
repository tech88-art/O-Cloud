# ADR-0023: Phase 13 entry decisions(spine = real 版完整化 + 真硬件点亮 → 项目收口 · milestone M7 真生产化收口 final · 不规划 Phase 14 · lab-gating 翻转 · Bucket A/B scope map · 4 carry 移出核心交付 · demo = 长期一等功能验证台)

- **状态**:Accepted(Phase 13 entry decision lock · per Phase 13 plan P13-T-001 · 2026-06-03)
- **日期**:2026-06-03(codify · 用户两决策 2026-06-02 chat entry meeting)
- **决策者**:协调者(用户 · 2026-06-02 chat 两决策:收官视角 + 真硬件 lab 就位)
- **相关**:`docs/phase13-plan.md` §0 Goal + §1 Scope summary + §7 项目收口(本 ADR §2 Decision A-F 是该 plan 的 entry-gate codification)/ `docs/phase12-candidate-streams.md` §Spine candidates(本 ADR §2 Decision A spine = §Spine A production-hardening cohort 的 *deliverable 视角收口* 变体 · 非 workload 视角 marathon)+ §Active carry tracks(本 ADR §2 Decision F 4 carry 移出核心交付物 outcome record)/ ADR-0019 §2 Decision B "production-hardening cohort → Phase 13+" forward note(本 ADR §2 Decision A/B 兑现该 note · 收口而非顺延)/ ADR-0017 §2 Decision B forward note(M5 foundation 之上 production hardening · 本 ADR 收口)/ ADR-0011 §2 §3(Source 接口 + lab gating policy · 本 ADR §2 Decision C 翻转目标 · supersede 详 ADR-0024 sibling)/ ADR-0016 §2 Decision B(lab posture re-eval triggers "ad-hoc lab access materialise" · 本 ADR §2 Decision C trigger 兑现)/ ADR-0013/0014(O2 DMS authn + 多租户 Quota · 本 ADR §2 Decision D Bucket A 上游 · 详 ADR-0025 sibling)/ ADR-0018(Karmada 拓扑 · 本 ADR §2 Decision D Karmada HA 上游)/ ADR-0020(aarch64 鲲鹏 + openEuler target · 本 ADR §2 Decision C 真硬件验证平台)/ `docs/build-and-production-validation.md` §4.4(真机特有验证 5 项 = Bucket B 逐项 DoD)+ §5(生产硬化验收清单 6 项 = Bucket A 逐项 DoD)/ `docs/checkpoint-phase12.md` §6(Phase 13+ handoff brief)/ `docs/devlog/phase-13-kickoff-handover.md`(本 phase 入口)/ `docs/architecture.md` §1.3 phase 路线图(M1-M6 landed · M7 由本 ADR 启航)+ §13 review-table(本 ADR Allowed Paths 含 Phase 13 row "in flight")/ ADR-0024(T002 sibling · 真硬件 lab 激活 + Bucket B 架构)/ ADR-0025(T003 sibling · 生产硬化架构 Bucket A)

---

## §1 Context

### §1.1 Phase 12 closer · M6 平台真实化 + 操作台一体化 milestone landed · demo/real profile 分离

Phase 12 落 16 task chain + tag `phase-12-complete`(@ HEAD of dev post-CI gate · `f0ce285` · per `docs/checkpoint-phase12.md` + arch §13 Phase 12 row)· **M6 平台真实化 + 操作台一体化 milestone CLOSER**:

- 4 entry ADRs(ADR-0019 entry + ADR-0020 aarch64 鲲鹏 + openEuler target + ADR-0021 拓扑全保真扩展 + ADR-0022 one-page UI IA)
- Track A 平台真实化(amd64-only ADR-0001 §13 → aarch64 鲲鹏 + openEuler · 10 Dockerfile multi-arch buildx + CI arm64 cross-compile matrix + helm `nodeAffinity arch=arm64` + install.sh openEuler)
- Track B 拓扑数据全保真(NPU PCIE 带宽 + node↔node network 边 + npu↔npu HCCS 边 + workload→node 边 + 带宽 hover)
- Track C 前端 one-page workspace(5 路由收敛单页操作台 + Splitter + 右栏 dispatch + preset bar)
- **post-`phase-12-complete` 收尾批**:拓扑视觉 polish ×10(`fix-004..013`)+ **demo/real 两交付版本架构分离**(`3765f5c` · config + deploy profile · 不 fork 代码)+ e2e-kind flake 硬化(`f0ce285`)

**关键:demo/real profile 分离已 land**(`3765f5c`)· **demo 版完整**(Phase 12 deliverable · 全 mock/simulator/amd64 kind 端到端 CI 绿)· **real 版"已接线、可部署、多数资源接真源",但拓扑/部署/真 NPU 分配/生产硬化待 Phase 13**(详 `deploy/profiles/real/README.md` + build-doc §0.1)。

### §1.2 candidate-streams production-hardening cohort + 4 active carry · Phase 13 entry decide

`docs/phase12-candidate-streams.md`(Phase 11 W3 P11-T-203 起草)留两类 open 待 Phase 13 entry 拍板:

- **§Spine A continuation 真生产化 production-hardening cohort**(ADR-0017 §2 Decision B + ADR-0019 §2 Decision B forward note 预设 · 顺延 Phase 13+):Karmada HA + 完整 OIDC IdP + ClusterQuota webhook B 完整 + Vault Secret + vLLM PD 分离 production-grade SLA(P99)· candidate-streams 把该 cohort 描述为 "~3-4 phase" **workload 视角**估算
- **§Active carry tracks(4 个)**:Track A lab gating(ADR-0011 §3 连续 6 phase default-defer)· Track B Volcano gang-scheduling(ADR-0010 4th carry)· Track C Partitionable Devices(ADR-0009 · KEP-4815 Beta)· Go v1 ResourceSlice schema migration(P10-fix-001 carry · 5 module · 随 K8s 1.36)· 加 Frontend G6 5.x 重写(F01 charter)

### §1.3 2026-06-02 用户 entry meeting 两决策

用户 2026-06-02 chat Phase 13 entry meeting 给 **两决策**,把 candidate-streams 的 workload 视角 redirect 为 deliverable 视角收口:

- **用户决策 1 · 收官视角**:"demo 已完成,剩余生产化隔离完成就可以"。→ Phase 13 = **项目收官 phase** · 唯一目标 = 把 real(生产)版剩余 stub/mock/ErrNotImplemented 缺口全部填成真体,让 demo/real 隔离彻底闭合,项目 deliverable 完整交付。candidate-streams "~3-4 phase" 是 *workload* 视角 · 用户以 *deliverable* 视角收口(填平既有 stub ≠ 跨 phase production-hardening marathon)。
- **用户决策 2 · 真硬件 lab 已就位**:真硬件 lab 已可访问 → 翻转 ADR-0011 §3 连续 6 phase 的 lab-gating default-defer → 真硬件点亮(ADR-0016 §2 Decision B trigger "ad-hoc lab access materialise" 兑现)。

**两决策 → 两桶 IN-scope**:决策 1 = Bucket A 生产硬化(纯软件 · 完成 real 版 §5 验收清单);决策 2 = Bucket B 真硬件 body(lab 点亮 · 填 real 版 §4.4 运行时缺口)。

### §1.4 本 ADR 不涉及(拆 sibling ADR / plan)

- **真硬件 lab 激活详 + Bucket B 5 真体架构**(Source.RealAscend / 真 topology / telemetry / deploy / inference · supersede ADR-0011 §3 + decoupling-seam invariant)— 拆 **ADR-0024**(T002 sibling)· 本 ADR §2 Decision C 仅 codify lab-gating 翻转
- **生产硬化 Bucket A 5 架构详**(authz model / Vault secret / ClusterQuota 真强制 / Karmada HA / P99 SLA · right-sizing 边界)— 拆 **ADR-0025**(T003 sibling)· 本 ADR §2 Decision D 仅 codify Bucket A scope = build-doc §5 清单
- **各 task 详 Allowed Paths / Acceptance** — 由 `docs/phase13-plan.md` §3-§6 task package overview 承载 · 本 ADR §3 仅 enumerate 16 task cross-ref §2
- **甲方依赖 swap point 实现**(IdP issuerURL/audience · P99 SLO 阈值)— Dex/default SLO 兜底 land(T202/T206)· 详 ADR-0025 §4

---

## §2 Decision

### §2.1 Decision A:Phase 13 spine = real 版完整化 + 真硬件点亮 → 项目收口

**candidate-streams §Spine A production-hardening cohort close-out**:Phase 13 spine = production-hardening cohort,**但以 deliverable 视角收口而非 workload 视角 marathon**。spine 由 2 桶构成(per `docs/phase13-plan.md` §1 Scope summary):

| 桶 | 描述 | task | DoD 出处 |
|---|---|---|---|
| **Bucket B · 真硬件 body** | lab 点亮 · 填 real 版 4 处运行时缺口:真 NPU 发现/分配(real-Ascend Source body · npu-smi/DCMI)+ 真 telemetry(exporter DCMI/npu-smi 真读 · simulator off)+ 真拓扑聚合(backend `GetTopology` 真体)+ 真 `Deploy()`(k8s source apply)+ 真 CANN + vllm-ascend PD 分离推理 | T101-T105 | build-doc §4.4(5 项真机验证) |
| **Bucket A · 生产硬化(纯软件)** | 完成 real 版 §5 验收清单:OIDC/RBAC(替换静态 token)+ Vault/external-secrets + ClusterQuota webhook B 真强制 + Karmada control-plane HA + vLLM PD P99 SLA harness | T201-T206 | build-doc §5(生产硬化验收清单) |

**为什么是 deliverable 视角收口而非 workload marathon**:

- 用户 2026-06-02 明确 "demo 已完成,剩余生产化隔离完成就可以" —— Phase 13 填的是 *既有架构的 stub*(real-Ascend Source / `GetTopology` / `Deploy()` / authn Validator / ClusterQuota webhook 均 Phase 10-12 已 ship 接口/schema · 缺的是 body),非从零起新 cohort
- demo/real profile 分离(`3765f5c`)已把"填真体"的 surface 收窄到 `Source` 接口缝以下(ADR-0024 §2 Decision G decoupling-seam invariant)· 缝上共享层(aggregator/handler/前端)零改动 → 工作量集中、可收口
- candidate-streams "~3-4 phase" 估算把 production hardening 当 *持续演进 program*;用户以 *一次性补全 real 版* 收口 —— 每 task 1:1 关一个 build-doc §4.4/§5 文档化缺口,不为完整性堆 padding

**每个 task 1:1 对应 build-doc §5 生产硬化验收清单 / §4.4 真机特有验证 / `deploy/profiles/real/README.md` 的一个具体 gap**(per plan §0)。

### §2.2 Decision B:Milestone = M7 真生产化收口(final · 项目 deliverable 在 phase-13-complete 完整交付 · 不规划 Phase 14)

**ADR-0019 §2 Decision B / ADR-0017 §2 Decision B forward note 的收口兑现**:

- ADR-0017/0019 预设 production-hardening cohort 顺延 Phase 13+ 作"真 production-hardening milestone"· candidate-streams 留 "M7 naming + cohort 启动 posture" Phase 13 entry decide
- 用户 2026-06-02 收官决策 → **M7 = 真生产化收口**(final milestone)· **project deliverable 在 `phase-13-complete` 完整交付**:demo 版(Phase 12)+ real 版(Phase 13)两版完整 · demo/real 隔离闭合 · build-doc §4.4/§5 全 🟢(达标判据见 §4 真硬件 fallback)
- **明确 codify:不规划 Phase 14**。candidate-streams "~3-4 phase" 是 workload 视角 · 用户以 deliverable 视角收口 → 4 active carry 显式移出项目核心交付物(§2 Decision F)· **carry 非 phase**(信号触发的可选扩展 · 不构成后续 phase)

**M7 naming 不 over-claim**(P2 边界):

- M7 *closes* real 版完整化(stub/mock/ErrNotImplemented 全填真体)+ demo/real 隔离闭合 + best-effort 真机验证(§4 真硬件 fallback)
- M7 *不* claim 甲方生产环境合同级 SLA / 多区 DR / full IAM —— Bucket A 是 **reference-grade** 右-sized(Dex 参考 IdP / external-secrets 单 Vault / 3-replica 非 full DR / P99 harness 用 default SLO · 详 ADR-0025 §3)· 甲方 swap point 文档化(§4(a)(b))

### §2.3 Decision C:lab-gating 翻转(ADR-0011 §3 6-phase default-defer → light-up · Bucket B 进 scope)

**ADR-0011 §3 lab gating policy 翻转**(本 phase 唯一 decision-gated 已决):

- ADR-0011 §3 lab-gating 连续 6 phase(P7-P12)default-defer(no signal → defer)· ADR-0016 §2 Decision B 设 re-eval trigger "ad-hoc lab access materialise"
- 用户 2026-06-02 lab-ready 信号 → **翻转 light-up**:`real-ascend` Source 从 stub(`ErrNotImplemented`)→ 真体 · Bucket B 不再是 carry · 是本 phase deliverable
- **详架构 ADR-0024**(T002 sibling · 真硬件 lab 激活 + Bucket B 5 真体 + decoupling-seam invariant + 真硬件集成 fallback)· 本 ADR 仅 codify policy 翻转事件

**真硬件验证 posture**(2026-06-03 用户校准 · 详 plan §8):**功能正确性由 demo 持续验**(快 · 无硬件 · CI 常驻 · 长期一等功能验证台 · §2 Decision E)· **真机只验「对接」**(real Source 是否正确读真 npu-smi/DCMI/CANN + 输出与 `Source` 接口同形)· 真机不重验整条功能栈 → lab 依赖面收窄。

### §2.4 Decision D:Bucket A scope = build-doc §5 生产硬化验收清单逐项(right-sized reference-grade)

**Bucket A = 完成 real 版 §5 验收清单 6 项中的 5 项**(第 6 项 Go v1 migration 是 carry · §2 Decision F):

| build-doc §5 项 | Phase 13 task | right-sizing(详 ADR-0025 §3) |
|---|---|---|
| 认证授权(OIDC + SA + TokenReview + RBAC) | T201 + T202 | Dex 参考 IdP(非 full IAM)· 甲方 swap issuerURL/audience |
| 多租户配额强制(ClusterQuota webhook B + 跨集群 usage + fail-open) | T204 | webhook B 读 status.usage 真强制 + Karmada PerCluster populate |
| Karmada 多站点 HA(control-plane HA + PropagationPolicy + cross-cluster RBAC) | T205 | 3-replica + 外置 etcd(非 full DR · 真物理 multi-region LB lab-gated) |
| Secret 管理(Vault) | T203 | external-secrets + 单 Vault backend(非多区集群) |
| 推理 SLA(vLLM PD P99) | T206 | load-test harness + default SLO(非甲方合同 SLO) |

**详架构 ADR-0025**(T003 sibling · authz model / secret / quota / HA / SLA + right-sizing 声明防 P3 gold-plating)。

### §2.5 Decision E:demo 版 = 长期一等功能验证台(本 phase 不动 · clean demo/real 隔离 = 收口判据)

- demo 版(`--profile demo` / `make dev-up` / Phase 12 deliverable)是**长期一等功能验证台** —— 在开发机 amd64 照常起栈(multi-arch Dockerfile amd64 layer 本机构建)· 长期承担功能/CI/演示快速验证 · **非用完即冻**
- Phase 13 **demo 版不动**(回归基线)· real 版填真体 · **real 版切真源不影响 demo 工作**(隔离活在 `Source` 接口缝 · ADR-0024 §2 Decision G · real 逻辑留缝下 / 缝上共享层 profile 无关)
- **clean demo/real 隔离 = 收口判据**:每 Bucket B task 离线层必含 "demo profile 仍起" 回归点 · 禁 `if real {}` 分支污染共享层

### §2.6 Decision F:4 active carry 移出项目核心交付物(信号触发可选扩展 · 不 gating 收口 · 不构成 Phase 14)

**candidate-streams §Active carry tracks outcome record** —— 4 active carry 均**不属 real 版完整化**,降级为信号触发的可选扩展 · 不 gating `phase-13-complete` 收口 · **不构成后续 phase**(per plan §7):

| Carry | 当前状态 | 为何不在 real 版完整化内 | 触发条件(若未来做) |
|---|---|---|---|
| Volcano gang-scheduling | 4th carry(ADR-0010) | 训练 gang-schedule ≠ 推理 real 版完整化 · 生态扩展 | 训练 demo signal materialise |
| Partitionable Devices(KEP-4815) | 3rd carry(ADR-0009) | 上游 KEP 仍 Beta 非 GA · 现自研 DRA 切分已满足 real 版 | KEP-4815 GA + K8s 1.36+ baseline |
| Go v1 ResourceSlice migration | P10-fix-001 carry(5 module v1beta1) | K8s 1.34+ v1beta1 shim 仍 work · 兼容性维护 ≠ 功能缺口 | **lab 集群 K8s ≥1.36**(ADR-0024 §4 entry-check · 若 lab 即 1.36+ → 随 T101 一并迁;否则 shim 保留) |
| Frontend G6 5.x 重写 | F01 charter(ReactFlow 续用) | demo 前端即 real 前端 · 无渲染瓶颈 materialise | ReactFlow set-c-stress 大基数渲染瓶颈 |

---

## §3 Phase 13 scope detailed enumeration(16 task · 3 W1 + 5 W2 + 6 W3 + 2 Closer)

**cross-ref `docs/phase13-plan.md` §2 task package overview**(不在本 ADR 复述详 Allowed Paths / Acceptance):

```
W1 Foundation(3 task · docs/ADR · 全 main-agent serial · entry gate)
├── P13-T-001  ADR-0023 Phase 13 entry decisions(本 ADR · spine + M7 final + lab 翻转 + Bucket A/B scope + demo 验证台 + 4 carry 移出)
├── P13-T-002  ADR-0024 真硬件 lab 激活 + Bucket B 架构(supersede ADR-0011 §3 · §2 Decision C)
└── P13-T-003  ADR-0025 生产硬化架构(Bucket A · §2 Decision D · → build-doc §5 清单 close)

W2 Bucket B · 真硬件 body(5 task · operators+backend+exporters · §2 Decision A/C)
├── P13-T-101  [B1] real-Ascend Source 真体(realascend.go · npu-smi info + DCMI · 复用 parse.go)
├── P13-T-102  [B2] 真 NPU telemetry(exporter dcmi/npu_smi/cgroup 真读 · simulator off)
├── P13-T-103  [B3] backend 真拓扑聚合(k8s/crd GetTopology 真体 · 替换 ErrCapabilityUnavailable)
├── P13-T-104  [B4] backend 真 Deploy()/DeleteDeploy()(k8s source apply)
└── P13-T-105  [B5] 真 CANN + vllm-ascend PD 分离推理(deployment_builder 真 image + model mount + PD Router 真 endpoint)

W3 Bucket A · 生产硬化(6 task · operators+deploy+backend · §2 Decision A/D)
├── P13-T-201  [A1] OIDC/TokenReview validator 真体 + middleware wiring
├── P13-T-202  [A1] Dex 参考 IdP 部署 + RBAC manifest + helm authz wiring
├── P13-T-203  [A2] Vault/external-secrets(替换静态 secret)
├── P13-T-204  [A3] ClusterQuota webhook B 真强制 + 跨集群 usage reconcile
├── P13-T-205  [A4] Karmada control-plane HA(3-replica + 外置 etcd + propagation 加固)
└── P13-T-206  [A5] vLLM PD P99 SLA harness(load-test + P99 测量 + default SLO doc + scaler latency feedback)

Closer(2 task)
├── P13-T-301  真机端到端 E2E(real arm64 集群 helm install real profile · build-doc §4.4 5 项)+ buildx 镜像 push registry + e2e-kind arm64
└── P13-T-302  docs 收官 + checkpoint-phase13 + build-doc §4.4/§5 🔴→🟢 stamp + tag phase-13-complete + 项目收官 announcement(M7 final · 无 Phase 14)
```

**Task count**:16 · per `docs/phase13-plan.md` §2 task package overview。**§0a.5 chat+ADR self-RFC**:Phase 13 引入 3 ADR(0023 entry · 0024 Bucket B · 0025 Bucket A)· **共享契约预计零 YAML 改动**(real 版填的是既有契约的真体)· 若 T204 发现 `ClusterQuotaStatus.Usage` 需补字段 → 走 §0a.5 chat+ADR + main-agent serial。

---

## §4 Open questions

### (a) 甲方 OIDC IdP 选型(Dex 兜底)

Bucket A authz 主路径 = o2-dms `K8sTokenReviewValidator`(in-cluster SA)· 外部 IdP 路径 = `OIDCValidator`(JWKS + JWT):

- **当前倾向 / 兜底**:Dex 参考 IdP land(T202)· 甲方 IdP 选定后 swap `issuerURL`/`audience`(ADR-0025 §4(a))· Dex = 参考实现非 full IAM · swap point 文档化(`deploy/idp/README.md`)
- 不 block 收口:甲方 IdP 输入未到位时 default 兜底即可交付

### (b) P99 SLO 阈值(default SLO 兜底)

推理 SLA 达标判据依赖甲方 SLO 输入:

- **当前倾向 / 兜底**:default SLO 文档化 + 真硬件 P99 测量(T206)· 甲方 SLO 输入后改阈值(ADR-0025 §4(b))· 达标判据用 default SLO · 甲方阈值 swap
- 不 block 收口:SLO 输入未到位时 default 兜底即可交付

### (c) 真硬件单点 block 的 fallback posture(P3 诚实 · P5 最弱环 · 不重开 phase)

Phase 13 最弱环 = 真硬件集成 surface 的未知(synthetic fixture 按*预期* npu-smi/DCMI 格式建模 · 真 910B 驱动/CANN 版本可能偏离):

- **fallback**:若某真硬件点在 lab 驱动/CANN 版本下 block(如某 DCMI 字段缺失 · npu-smi 输出变体)→ **交付该 body + captured-fixture test 通 + 把那一个真机验证点标 lab-driver-gated**(记 devlog + checkpoint §残留)
- 项目仍按 "**软件完整 + best-effort 真机验证**" 收口 · **单点 surface 不 gating 收口 · 不开 Phase 14**
- 详 ADR-0024 §3 真硬件集成风险 + plan §8 真机验证 posture

---

## §5 引用

### 上游(本 ADR 决策依据)

- `docs/phase13-plan.md` §0 Goal + §1 Scope summary + §7 项目收口 + §8 真机验证 posture(本 ADR §2 Decision A-F 是该 plan 的 entry-gate codification)
- `docs/phase12-candidate-streams.md` §Spine candidates(本 ADR §2 Decision A spine = Spine A production-hardening 的 deliverable 视角收口变体 close-out)+ §Active carry tracks(§2 Decision F 4 carry 移出核心交付物 outcome record)
- ADR-0019 §2 Decision B "production-hardening cohort → Phase 13+" forward note(本 ADR §2 Decision A/B 兑现该 note · 收口而非顺延)
- ADR-0017 §2 Decision B forward note(M5 foundation 之上 production hardening · 本 ADR 收口)
- ADR-0011 §2 §3(Source 接口 + lab gating policy · 本 ADR §2 Decision C 翻转目标 · supersede 详 ADR-0024)
- ADR-0016 §2 Decision B(lab posture re-eval trigger "ad-hoc lab access materialise" · 本 ADR §2 Decision C trigger 兑现)
- ADR-0013/0014(O2 DMS authn + 多租户 Quota · 本 ADR §2 Decision D Bucket A 上游 · 详 ADR-0025)
- ADR-0018(Karmada 拓扑 · 本 ADR §2 Decision D Karmada HA 上游)
- ADR-0020(aarch64 鲲鹏 + openEuler target · 本 ADR §2 Decision C 真硬件验证平台)
- `docs/build-and-production-validation.md` §4.4(真机特有验证 5 项 = Bucket B 逐项 DoD)+ §5(生产硬化验收清单 6 项 = Bucket A 逐项 DoD)
- `docs/checkpoint-phase12.md` §6 Phase 13+ handoff brief
- `docs/devlog/phase-13-kickoff-handover.md`(本 phase 入口)
- `docs/architecture.md` §1.3 phase 路线图(M1-M6 landed · M7 由本 ADR 启航)+ §13 review-table(本 ADR Allowed Paths 含 Phase 13 row "in flight")

### 下游(本 ADR 后续工作 / 触发 Phase 13 execution)

- P13-T-002 ADR-0024 真硬件 lab 激活 + Bucket B 架构(本 ADR §2 Decision C · supersede ADR-0011 §3)
- P13-T-003 ADR-0025 生产硬化架构 Bucket A(本 ADR §2 Decision D · → build-doc §5 清单 close)
- P13-T-101..T105 Bucket B 真硬件 body(本 ADR §2 Decision A/C)
- P13-T-201..T206 Bucket A 生产硬化(本 ADR §2 Decision A/D)
- P13-T-301 真机端到端 E2E + buildx 镜像 push(本 ADR §3 closer · §4(c) 真硬件 fallback)
- P13-T-302 Phase 13 checkpoint + tag(本 ADR §2 Decision B M7 真生产化收口 final milestone CLOSER announcement · §2 Decision F 4 carry outcome stamp · **不规划 Phase 14**)

### 上游 commit chain(决策时 grep-verified)

- Phase 12 tag `phase-12-complete`(HEAD of dev post-CI gate · `f0ce285`)
- demo/real profile 分离 `3765f5c`(config + deploy profile · 不 fork 代码)
- Phase 13 plan @ `9165ebe`(`docs/phase13-plan.md` · 真生产化收口 + 真硬件点亮 · M7 final · 无 Phase 14)+ kickoff handover `b4b86a2`(`docs/devlog/phase-13-kickoff-handover.md`)
- 本 ADR 是 Phase 13 plan §3 P13-T-001 deliverable

---

**END of ADR-0023**
