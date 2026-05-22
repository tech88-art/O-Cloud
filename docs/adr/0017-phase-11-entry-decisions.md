# ADR-0017: Phase 11 entry decisions(M5 真生产化 foundation spine + lab gating 5th attempt posture + chart packaging spine 优先级)

- **状态**:Accepted(Phase 11 entry decision lock · per Phase 11 plan P11-T-001 · 2026-05-22)
- **日期**:2026-05-22
- **决策者**:协调者(用户)
- **相关**:ADR-0011 §3 lab gating 政策(本 ADR §2 Decision C 沿用 default-defer policy 不修改 policy core · 仅 codify 5th attempt posture rationale)/ ADR-0015 §3.3 demo-backend cache singleton substrate(P10-T-006 landed · 本 ADR §2 Decision D chart packaging 优先级 1st 行)/ ADR-0016 §2 Decision B(3 re-eval triggers · 本 ADR §2 Decision C trigger 1 W1 entry meeting outcome record)/ ADR-0016 §3 Stream 1-8 enumeration(本 ADR §2 Decision A spine 是 Stream 1+5 foundation subset · Stream 2/3/4/6/7/8 留 Phase 12+)/ ADR-0016 §4 Open questions (a)+(b)+(c)(本 ADR §2 Decision A+B+C 是其 close-out)/ `docs/checkpoint-phase10.md` §6 Phase 11+ handoff brief(9 chart packaging primary work + 8 candidate streams · 本 ADR §2 Decision D 优先级排序源)/ `docs/architecture.md` §1.3 phase 路线图(M1-M4 已 landed · M5 由本 ADR 启航)· §13 review-table(本 ADR Allowed Paths 含 Phase 11 row promote)/ `docs/phase11-plan.md` §1-§5(20 main task + 4 Frontend UX sub-track 详) / ADR-0018 Karmada deployment topology(T002 sibling ADR · Karmada propagation 主线 2 拆分子契约)

---

## §1 Context

### §1.1 Phase 10 closer · M4 工程化对外 milestone landed

Phase 10 落 20 task chain + tag `phase-10-complete`(@ `04cc009` per checkpoint-phase10 §0 + §1)· **M4 工程化对外 milestone CLOSER**:
- 3 IMS controller body 全 land(T007/T008/T101 · 33 state transitions + 46 unit tests)
- K8s baseline 三件套 1.32 → 1.34.3 + scheduler framework migration 9 files NodeInfo/CycleState struct→interface + NumaAffinity wrap(known-issues #12 RESOLVED · 5-phase carry closer)
- demo-backend cache singleton substrate(ADR-0015 §3.3 · Lease state machine + 3 Prometheus 指标)
- O2 DMS authn 3 Validator interface 实现 + Quota token-bucket rate algorithm
- T201 master-demo.sh 12-step orchestrator(synthetic ring fallback path · 80% landed deliverable per ADR-0016 §2 Decision C)
- 3 deferred-by-default outcomes:T102 LAB 5th carry + T108 Volcano 2nd defer + T202 Partitionable Devices defer

post-tag CI gate 落 3 fixes(P11-fix-001/002/003 · 详 `docs/devlog/phase-11-fix-001.md` + `-fix-002.md` + `-fix-003.md`) · dev HEAD 全绿 → M4 真完成。

### §1.2 ADR-0016 §4 Open questions 三项均 carry to Phase 11 entry decision

ADR-0016 §4 留 3 项 open 至 Phase 11 entry meeting:
- (a) **Lab access posture re-evaluation timing** — 5th carry 触发 §2 Decision B re-eval trigger (1) · timing 具体 open
- (b) **Phase 11+ scope primary spine** — Spine A 真生产化 / Spine B 真硬件-native / Spine C 生态扩展 / Spine D 多 milestone 拆 · 三/四选一 open
- (c) **Milestone naming** — M5 真生产化 / M5 真硬件-native / M5 生态扩展 / M5+ multi-phase 拆 · open

本 ADR §2 Decision A+B+C 是这 3 项 open 的 close-out。

### §1.3 checkpoint-phase10 §6 handoff brief 给 9 chart packaging primary work + 8 candidate streams

`docs/checkpoint-phase10.md` §6 列出 Phase 10 close-out 时 Phase 11+ 入口 most-actionable 工作:
- **9 chart packaging primary work**:demo-backend chart(#1)+ node-lifecycle-operator chart(#2)+ software-mgmt-operator chart(#3)+ bare-metal-provisioning-operator chart(#4)+ inference-operator chart ProxyImage env wire(#5)+ O2 DMS authn chart wiring(#6)+ Karmada propagation 第一波(#7)+ Quota ClusterQuota CRD(#8)+ Frontend src/ Workload page extension(#9)
- **8 candidate streams**(per ADR-0016 §3 Stream 1-8):全 Phase 11+ candidate · Phase 11 不可能全 8 stream land

本 ADR §2 Decision D 是 9 chart packaging primary work + Phase 11 task package overview 的优先级排序源。

### §1.4 本 ADR 不涉及

- ADR-0011 §3 lab gating *policy core* 修改 — 政策 default 不变(no signal → defer)· §2 Decision C 仅 codify 5th attempt posture rationale + W1 entry meeting outcome record(carry tally 表更新由 T101 devlog Allowed Paths 处理)
- Karmada control-plane deployment topology 详 — 拆 ADR-0018(T002 sibling)· Karmada chart + 2 member kind cluster bootstrap + cross-cluster informer aggregation pattern
- Phase 11+ candidate Stream 2-4/6-8 详 — 留 Phase 12+ entry meeting 起草时 codify(本 ADR §3 仅 enumerate Phase 11 in-scope · 不动 ADR-0016 §3 Stream 2-8)
- Phase 11 各 task 详 Allowed Paths / Acceptance — 由 `docs/phase11-plan.md` §3-§5 task package overview 承载 · 本 ADR 不复述

---

## §2 Decision

### §2.1 Decision A:Phase 11 primary spine = chart packaging 闭环 + Karmada propagation 第一波 + Frontend src/ extension(Spine A 真生产化 foundation subset)

**ADR-0016 §4 Open question (b) Phase 11+ primary spine 选项答复**:**Spine A 真生产化 foundation 的 subset**(不是全 Spine A · 不是 Spine B/C/D)。

**Phase 11 spine 由 3 主线 + 3 子线 构成**(per `docs/phase11-plan.md` §0 Goal + §1 Scope summary):

| 主线 / 子线 | 描述 | task |
|---|---|---|
| **主线 1** chart packaging spine 闭环 | 5 new helm chart(demo-backend + 3 IMS + scheduler-plugin NRT bundle)+ 1 existing chart edit(inference-operator ProxyImage env wire)+ cmd/main.go controller-runtime wire | T003 + T004 + T005 + T006 + T007 + T008 |
| **主线 2** Karmada propagation 第一波 production-grade | host + 2 member kind cluster · Karmada chart deploy · PropagationPolicy + cross-cluster informer aggregation · ClusterQuota CRD | T002 ADR-0018 + T102 + T103 + T104 |
| **主线 3** Frontend src/ Workload page extension 完整 | React/TS Workload page + 3 indicators(O2 DMS exposed badge + Quota usage progress bar + scaleHistory ECharts timeline)+ backend handler bridge 3 fields response | T105 |
| **子线 1** Phase 10 deferred items 全 close | scheduler-plugin NRT CRD bundle(P10-fix-002 carry close)+ inference-operator ProxyImage env wire(P10-T-106 substrate consume)+ Volcano 2nd attempt conditional + Partitionable Devices conditional re-eval | T007 + T008 + T108 + T202 |
| **子线 2** Lab gating 5th attempt | LAB-CONDITIONAL · 与 §2 Decision C 联动 · default-defer 维持 | T101 |
| **子线 3** 真 multi-cluster / multi-site demo 打磨 + docs 大整理 + Phase 12+ 前瞻 + checkpoint+tag | master-demo-multi-site.sh extension · arch §13 promote · Go code v1 schema migration cohort plan · M5 真生产化 foundation milestone CLOSER announcement | T201 + T203 + T204 |

**明示不在 Phase 11 spine**(留 Phase 12+ · 与 ADR-0016 §3 Stream 2/3/4/6/7/8 一致):
- Live migration of HCCL ranks + per-Pod RDMA bandwidth quota(Stream 2 · CNI-level gaps blocking · Phase 12+ if Huawei roadmap aligned)
- Real fabric switch integration(SONiC / Cumulus / Arista API · Stream 3 · 物理 switching gear 依赖)
- vLLM PD 分离 production-grade SLA(P99 latency + multi-tenant isolation guarantees · Stream 4 · 甲方 SLO 输入依赖)
- 完整 OIDC IdP 部署(Keycloak / Dex · Stream 5 part 2 · 甲方 IdP 选定依赖 · Phase 11 仅 ship OIDC client + TokenReview chart wiring substrate via T106)
- K8s 1.34 → 1.36 baseline bump(Stream 6 trigger · Partitionable Devices GA prereq · T202 W3 entry re-WebFetch · 若 KEP-4815 仍 Beta or 1.36 not GA → defer)
- O2 IMS R1 v05.00+ spec migration(Stream 8 · O-RAN WG6 release cadence 依赖)
- Karmada control-plane HA + 真 多机房 multi-site deployment(>2 member cluster · 跨真物理机房 / 跨 region · Phase 11 仅 ship 单机模拟 multi-site)

**Spine A subset 选 rationale**:
- Spine A 真生产化 *完整* 包含 Stream 1(multi-site)+ Stream 4(P99 SLA)+ Stream 5(OIDC IdP 完整)三大块 · Phase 11 单 phase 5-6 week calendar 不可能全 land
- 实际可 land:Stream 1 foundation(Karmada propagation 第一波 + 2 member kind cluster minimum · 单机模拟 multi-site)+ Stream 5 part 1(OIDC client + TokenReview chart wiring substrate · 完整 IdP 部署留 Phase 12+)+ 9 chart packaging primary work 全 close · 这就是 "Spine A 真生产化 foundation subset"
- Stream 4 (P99 SLA) 强依赖甲方 SLO 输入 + 真业务 traffic · Phase 11 没这两块 input → 留 Phase 12+ when signal aligned

### §2.2 Decision B:Milestone = M5 真生产化 foundation(foundation 后缀明示 不 over-promise)

**ADR-0016 §4 Open question (c) Milestone naming 选项答复**:**M5 真生产化 foundation**。

**Foundation 后缀明示语义**:
- Phase 11 *不* close 完整 production hardening — 完整 P99 SLA / 完整 OIDC IdP / 真多机房 / Karmada HA / vLLM PD 分离 production-grade isolation 全留 Phase 12+
- Phase 11 *closes* production hardening *foundation* — chart packaging 闭环(自此 demo-backend + 3 IMS 都可 helm install)+ Karmada propagation 第一波(自此 multi-cluster 真路径 verifiable)+ Frontend src/ Workload page 3 indicators surface(自此 frontend 真消费 Phase 9-10 ADR-0013/0014 substrate)
- 是 *foundation* 不是 *deliverable* — Phase 12+/M6 在此 foundation 上加 production hardening 完整 cohort

**为什么不选 M5 真硬件-native**(Spine B):
- T101 LAB 5th attempt default-defer 维持(§2 Decision C) → 真硬件 stamp 仍 80% 缺(synthetic ring fallback 同 Phase 10 path)
- 若 5th 仍 defer + Phase 12+/M6 entry 起草仍无 lab signal → 该时 trigger 3 M5+ milestone reset 可能成立(详 §2 Decision C)· 但 *现在* Phase 11 entry meeting 不 reset

**为什么不选 M5 生态扩展**(Spine C):
- Volcano gang-scheduling(Stream 7)需 training-job demo 实质需求 · 当前 Phase 11 仍 inference + multi-cluster 主导 · 无 training signal
- O2 IMS R1 v05.00+ spec migration(Stream 8)需 O-RAN WG6 release cadence · 当前 v04.00 baseline 仍 stable

**Phase 12+ milestone naming 不在本 ADR 提前 lock** — 留 Phase 12+ entry meeting 起草时 codify · 视 Phase 11 outcome 与 5th carry posture 决定。

### §2.3 Decision C:Lab gating posture = default-defer 维持 5th attempt · trigger 1 fire · trigger 3 保留 Phase 12+

**ADR-0016 §4 Open question (a) Lab access posture re-evaluation timing + ADR-0016 §2 Decision B re-eval triggers 答复**:

**5th attempt 政策选择**:**default-defer 维持**(ADR-0011 §3 policy core 不修改 · 5th attempt 沿用 same gating model as P7/P8/P9/P10):
- Phase 11 W1 entry meeting 用户 signal:
  - "lab access available · 在 W1-WX 起 N 天 · CANN X / driver Y / silicon: Ascend 910B Z 板" → 派 T101 subagent · brief `[LAB-CONDITIONAL]` · T101 落 ADR-0011 §3 carry tally 5th entry "landed at SHA"
  - "lab access deferred / Phase 11 窗口内不可得" 或无明确信号(default · 同 P7+P8+P9+P10)→ T101 走 deferred 路径 · 落 ADR-0011 §3 carry tally 5th entry "deferred Phase 12+" + 本 ADR §2 Decision C 后续 stamp

**ADR-0016 §2 Decision B 3 triggers 状态**:
- **Trigger 1**(Phase 11+ entry meeting)· **FIRED at this ADR**(Phase 11 W1 entry · 本 ADR §2 Decision C 即 trigger 1 outcome record)· 重评结论 = default-defer 维持 + 5th 仍是 entry-meeting-conditional 而非 default-light-up
- **Trigger 2**(Ad-hoc lab signal)· stays armed · 任意 Phase 11 W1-W3 期间 chat 用户提 "lab access materialized" → 立即评估当前 phase open tasks · T101 仍可在 Phase 11 mid-phase 临时材料化执行
- **Trigger 3**(M5+ milestone reset)· **保留至 Phase 12+ entry meeting**(本 ADR 不 fire trigger 3)· 若 T101 5th 仍 defer + Phase 11 W3 T201 fallback path 80% landed → Phase 12+/M6 entry meeting 起草时:
  - 选项 a:仍 default-defer 维持 6th attempt(若 Phase 12+ scope 与真硬件无 direct dependency)
  - 选项 b:flip default-light-up(若 Phase 12+ scope 强依赖真硬件 · lab onboarding 列为 entry prerequisite)
  - 选项 c:M5+ milestone naming reset(若 Phase 12+ scope 重定向 production / multi-site / SLA hardening · 真硬件 stamp 不再是 milestone closer 而成为 entry 前提)

**为什么 trigger 1 fire 但不 flip default**:
- 5 phase consecutive defer 是政策实际效果 · 但 default-defer rationale 仍 fit:synthetic ring fixture(P7-T-104-v2 + P8-T-104 + P9-T-103 + P10-T-107 + P11-T-107 cumulative 5 phase 累计 + T201 真 multi-site demo 打磨)已 cover CI + demo flow 主干 · 真硬件 stamp 价值是 *verification* 而非 *deliverable substrate*
- 用户 chat 历来 "no signal" 是 stable 输入 · 主动 flip default-light-up 会逼迫 plan 假设 lab availability(实际尚未 materialize)· 政策反而失去 *gating* 语义 · 变成 *aspirational entry block*

**5th carry checkpoint 记录格式**(if T101 deferred at T204 checkpoint commit):
```
T101 5th defer to Phase 12+ · lab access not materialized in Phase 11 window ·
synthetic ring fixture(set-b-multi-ring · Phase 7+8+9+10+11 cumulative)+ T201
master-demo-multi-site.sh 真 Karmada multi-cluster propagation cover 主干 ·
Phase 12+ entry meeting per ADR-0016 §2 Decision B trigger 3 M5+ milestone
reset 评估窗口(选项 a/b/c · 本 ADR §2 Decision C 已 enumerate)
```

### §2.4 Decision D:Chart packaging spine 优先级排序

**checkpoint-phase10 §6 9 chart packaging primary work + Phase 11 plan §2 task package overview 优先级排序**:

| 优先级 | task | 排序 rationale |
|---|---|---|
| **1st(W1 first)** | **T003 demo-backend chart + cmd/main.go controller-runtime leader-elect wire** | ADR-0015 §3.3 singleton substrate(P10-T-006 landed)是 multi-instance failover 闭环 · chart 一 land · T201 真 multi-site demo 打磨可演 cache singleton failover 端到端 · Phase 11 spine 主线 1 起手 |
| **2nd-4th(W1 parallel-eligible)** | **T004 IMS-1 + T005 IMS-2 + T006 IMS-3 helm chart** | 3 IMS controller body P10-T-007+T008+T101 已 land · 3 chart 独立 module · 不交叉(Allowed Paths 各自 `deploy/helm-charts/<chart>/` + 自己 module cmd/main.go)· 用户 batch cue → parallel subagent 派(per §0a.11 strict-verify · main agent 仍 serial verify);默认 serial main agent。T006 含 Redfish/IPMI stub client + Secret 解析 比 T004/T005 多 0.3-0.5d work |
| **5th(W1 独立 sub-chart 工作)** | **T007 scheduler-plugin NRT CRD bundle + numaAffinity.enabled default flip back true** | P10-fix-002 carry close(known-issues #12 完整 close 循环最终步)· NRT subchart / vendored CRD YAML 二选一(Approach B vendored recommended)· 与 chart packaging 4 main 任务独立 · 可与 T003-T006 chart 任 task parallel if 用户 batch |
| **6th(W1 last · 最简单 chart edit)** | **T008 inference-operator chart DEFAULT_PROXY_IMAGE env wire** | P10-T-106 EffectiveProxyImage helper substrate 已 land · 仅 chart template add env var + cmd/main.go startup hook 3-5 行 + 1 integration test · 0.5d 估算 · 适合 W1 末尾 short task |
| **7th(W2 · Karmada deployment 启动)** | **T102 Karmada control-plane chart deploy + 2 member kind cluster bootstrap** | 依赖 T002 ADR-0018(W1)+ T007 sched-plugin NRT bundle(cluster baseline 联动)· 主线 2 起手 |
| **8th-9th(W2 · Karmada propagation + Quota)** | **T103 Karmada PropagationPolicy + T104 ClusterQuota CRD** | 依赖 T102 Karmada control-plane wire · 主线 2 完整闭环 |
| **10th(W2 · Frontend src/ extension)** | **T105 Frontend src/ Workload page extension** | 依赖 P10-T-105 api-contract substrate · 可与 T103/T104 parallel(frontend module 独立)· 主线 3 |
| **11th(W2 · O2 DMS chart wiring)** | **T106 O2 DMS authn chart wiring** | 依赖 P10-T-103 authn substrate · 与 T105 同 W2 polish · Stream 5 part 1 |

**为什么 demo-backend chart 1st**:
- T003 是 Phase 11 spine 主线 1 起手 · 一 land · 后续 4 chart(T004/T005/T006/T008)pattern 沿用 · 风险与 review cost 收敛(后 4 chart 增量 risk 显著降)
- T201 真 multi-site demo 打磨 需 demo-backend Lease leader-elect chart 起 · T003 不 land · T201 cache singleton failover 演示 cannot 真路径

**为什么 T007 sched-plugin NRT bundle 与 chart packaging 同 W1**:
- known-issues #12 完整 close 循环最终步 — Phase 6 sched-plugins v0.32 引入 → P10-T-005 wrap body land → P10-fix-002 chart default false 临时禁(unblock phase6 install)→ P11-T-007 NRT subchart + chart default true 恢复 · 不能 cross-phase 漂浮
- 与 T004-T006 IMS chart 独立 module 不冲突 · 派单工作量平衡

**为什么 T008 inference-operator chart edit 是 W1 last**:
- 工作量最小(0.5d)· 适合 W1 末尾 short task 补刀 · 不阻塞 T201
- P10-T-106 EffectiveProxyImage helper substrate 已 land · T008 仅 wire 通

---

## §3 Phase 11 scope detailed enumeration(20 main task + 4 Frontend UX sub-track)

**cross-ref `docs/phase11-plan.md` §2 task package overview**(20 main + 4 sub-track = 24 tasks · 不在本 ADR 复述详 Allowed Paths / Acceptance):

```
W1 Foundation(8 tasks · 2 ADRs + 4 new chart packaging + 1 NRT CRD bundle + 1 chart env wire)
├── P11-T-001  ADR-0017 — Phase 11 entry decisions(本 ADR · 即 this commit)
├── P11-T-002  ADR-0018 — Karmada control-plane deployment topology(sibling ADR · §2 Decision D + §2 Decision A 主线 2 子契约)
├── P11-T-003  demo-backend helm chart + cmd/main.go controller-runtime leader-elect wire(§2 Decision D 1st)
├── P11-T-004  IMS-1 node-lifecycle-operator helm chart + cmd/main.go(§2 Decision D 2nd)
├── P11-T-005  IMS-2 software-mgmt-operator helm chart + cmd/main.go(§2 Decision D 3rd)
├── P11-T-006  IMS-3 bare-metal-provisioning-operator helm chart + cmd/main.go(§2 Decision D 4th · 含 Redfish/IPMI stub + Secret 解析)
├── P11-T-007  scheduler-plugin NRT CRD bundle + numaAffinity.enabled default flip back true(§2 Decision D 5th)
└── P11-T-008  inference-operator chart DEFAULT_PROXY_IMAGE env wire(§2 Decision D 6th)

W2 Polish + LAB-conditional + Karmada deploy + Karmada propagation + Frontend + smoke + Volcano(8 tasks)
├── P11-T-101  [LAB-CONDITIONAL · 5th attempt] Source.RealAscend body(§2 Decision C 联动 · default-defer 维持)
├── P11-T-102  Karmada control-plane chart deploy + 2 member kind cluster bootstrap(§2 Decision D 7th)
├── P11-T-103  Karmada PropagationPolicy 第一波 + cross-cluster informer aggregation(§2 Decision D 8th)
├── P11-T-104  Quota ClusterQuota CRD + Karmada cross-cluster usage 累计(§2 Decision D 9th)
├── P11-T-105  Frontend src/ Workload page extension(§2 Decision D 10th · 主线 3)
├── P11-T-106  O2 DMS authn chart wiring(§2 Decision D 11th · Stream 5 part 1)
├── P11-T-107  kind smoke E2E Phase 11 extension(10+ assertions cover 全 W1+W2 substrate)
└── P11-T-108  [DECISION-GATED · 2nd attempt or 3rd defer] Volcano gang-scheduling install body(default = 3rd defer Phase 12+ · 同 lab gating spirit)

W3 Closer + Milestone Closer(4 tasks)
├── P11-T-201  真 multi-cluster / multi-site demo 打磨(master-demo-multi-site.sh extension · Karmada propagation 端到端 + 2 member 真切换 + Quota cross-cluster aggregation + cache singleton multi-instance failover)
├── P11-T-202  [DECISION-GATED · re-eval at W3 entry] Partitionable Devices Beta + partition-aware allocator(gated on K8s 1.36+ + KEP-4815 GA · re-WebFetch at W3 entry · default reject)
├── P11-T-203  项目级 docs 大整理 + Phase 12+ 前瞻 + Go code v1 schema migration cohort plan(arch §13 promote + ADR cross-ref audit + devlog index + Phase 1-11 timeline narrative + M5 真生产化 foundation milestone narrative)
└── P11-T-204  Phase 11 checkpoint + tag phase-11-complete + M5 真生产化 foundation milestone CLOSER announcement

Frontend UX Track-2 sub-track(parallel · conditional · 详 `docs/devlog/phase-11-frontend-ux-track-charter.md`)
├── P11-T-F01  (per charter)
├── P11-T-F02  (per charter)
├── P11-T-F03  (per charter)
└── P11-T-F04  (per charter)
```

**Task count**:20 main + 4 sub-track = 24 · per `docs/phase11-plan.md` §2 task package overview。

---

## §4 Open questions

### (a) Karmada control-plane HA posture

ADR-0018 §2 Decision A 锁 host cluster + 2 member kind cluster minimum(单机模拟 multi-site)· 但 Karmada control-plane *自身* HA posture 仍 open:
- Phase 11 ships single Karmada control(1 replica · 不 HA)— 与 demo-backend ADR-0015 §3.3 active-standby + Lease 选主对照 · single-active 与 demo SLA align
- Phase 12+ Karmada control HA(3 replica + etcd HA + leader-elect)— production hardening continuation · 与完整 P99 SLA / 完整 OIDC IdP / 真多机房一起 land

**当前倾向**:Phase 11 single control plane · 与 demo-backend cache singleton 同 posture · 不 stagger HA(stagger 会切断 Phase 11 polish 节奏 · HA 留 Phase 12+ multi-stream cohort)。

### (b) Frontend src/ Workload page extension 是否需要 i18n 引入

Phase 6 frontend 已 ship i18n basics(react-i18next per arch §3.2)· T105 3 indicators(O2 DMS exposed badge + Quota usage progress bar + scaleHistory ECharts timeline)是否做 i18n key extraction 还是 hard-code en-US 单语:
- 选项 a:hard-code en-US — Phase 11 polish speed 优先 · Phase 12+ i18n cohort batch refactor
- 选项 b:i18n key extraction 同期 — 与 Phase 6 i18n infrastructure 对齐 · 不留技术债

**当前倾向**:选项 b — Phase 6 i18n 既已 ship · 不应该在 Phase 11 polish 时引入 hard-code en-US 退化 · 但 i18n key 仅 en-US + zh-CN 双语(per arch §3.2 "中英双语")· 不引入 third language(German / Japanese 等留 Phase 12+ if 甲方 signal)· T105 entry meeting confirm。

### (c) demo-backend chart Lease leader-elect Pod 数

Phase 10 ADR-0015 §3.3 chart `replicaCount: 2` default · §3.5 "demo SLA 可接受 · production Phase 11+ Redis path":
- Phase 11 chart values default 仍 2 — 与 ADR-0015 §3.3 align · failover 5-30s window 对 demo 可接受
- Phase 12+ production replicaCount 3+(more replica reduces failover latency variance · 但需 weighted quorum)— 与 Karmada control HA 同 production hardening cohort

**当前倾向**:Phase 11 chart values default 仍 2 · 不在本 ADR 翻转 · T003 起手 commit + 加 chart values comment "production 推荐 3+ · Phase 12+ HA polish"。

---

## §5 引用

### 上游(本 ADR 决策依据)

- ADR-0011 §3 lab gating 政策(本 ADR §2 Decision C 沿用 default-defer policy core · 不修改)
- ADR-0015 §3.3 demo-backend cache singleton substrate(本 ADR §2 Decision D 1st rationale)
- ADR-0016 §2 Decision B(3 re-eval triggers · 本 ADR §2 Decision C trigger 1 outcome record)
- ADR-0016 §3 Stream 1-8 enumeration(本 ADR §2 Decision A Spine A subset cross-ref · Stream 2/3/4/6/7/8 留 Phase 12+)
- ADR-0016 §4 Open questions (a)+(b)+(c)(本 ADR §2 Decision A+B+C close-out)
- `docs/checkpoint-phase10.md` §6 Phase 11+ handoff brief(9 chart packaging primary work + 8 candidate streams · 本 ADR §2 Decision D 排序源)
- `docs/architecture.md` §1.3 phase 路线图(M1-M4 landed · M5 由本 ADR 启航 · §1.3 表 unchanged per 本 task Allowed Paths)
- `docs/architecture.md` §13 review-table(本 ADR Allowed Paths 含 Phase 11 row promote "in flight via T001-T204")

### 下游(本 ADR 后续工作 / 触发 Phase 11 execution)

- P11-T-002 ADR-0018 Karmada deployment topology(本 ADR §2 Decision A 主线 2 + §2 Decision D 7th-9th 子契约)
- P11-T-003-T008 W1 chart packaging spine 5 tasks(本 ADR §2 Decision D 1st-6th 优先级)
- P11-T-101 LAB-CONDITIONAL 5th attempt(本 ADR §2 Decision C policy lock · subagent brief 起手 reference)
- P11-T-102/T103/T104 Karmada propagation 第一波(本 ADR §2 Decision A 主线 2 · §2 Decision D 7th-9th)
- P11-T-105 Frontend src/ Workload page extension(本 ADR §2 Decision A 主线 3 · §4 Open question (b) i18n 决定 entry meeting)
- P11-T-106 O2 DMS authn chart wiring(本 ADR §2 Decision A Stream 5 part 1 · §2 Decision D 11th)
- P11-T-203 docs 大整理 + Phase 12+ 前瞻(本 ADR §2 Decision B M5 真生产化 foundation milestone narrative + arch §13 promote + Phase 12+ candidate streams 与 ADR-0016 §3 Stream 2-8 cross-ref)
- P11-T-204 Phase 11 checkpoint + tag(本 ADR §2 Decision C 5th carry outcome stamp 在 checkpoint § Phase 11 row 落定 · M5 真生产化 foundation milestone CLOSER announcement)
- Phase 12+ entry meeting · Phase 12 plan 起草 — 本 ADR §2 Decision C trigger 3 评估窗口

### 上游 commit chain(决策时 grep-verified)

- Phase 10 tag `phase-10-complete` @ `04cc009`(per `docs/checkpoint-phase10.md` §0)
- P10-fix-001 @ `7203587`(DRA v1 schema compat · post-tag CI gate fix)
- P10-fix-002 @ `3729bb3`(NumaAffinity default false 临时禁 · 本 ADR §2 Decision D 5th close 循环 trigger)
- Phase 11 plan @ `5a17fe3`(`docs/phase11-plan.md` · 本 ADR 是其 §3 P11-T-001 deliverable)
- P11-fix-001/002/003 @ `bb1d75f` / `77807d1` / `c9d01fc`(dev-stack docker-compose path A bringup + exporter +5 metric + dashboard template-var · 本 ADR §1.1 post-tag CI gate "3 fixes" 来源)
- Frontend UX Track-2 sub-track plan @ `0b5c042`(本 ADR §3 sub-track 4 tasks 来源)
- 演示 runbook @ `e43b223`(独立 demo HTML walkthrough · 与本 ADR 无直接决策依赖)

---

**END of ADR-0017**
