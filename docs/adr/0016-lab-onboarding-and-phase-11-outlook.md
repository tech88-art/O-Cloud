# ADR-0016: Lab onboarding 4th attempt + Phase 11+ 前瞻(M4 closer 配套 forward decision)

- **状态**:Accepted(policy lock for T102 4th attempt + Phase 11+ candidate streams enumeration)(2026-05-21 — Phase 10 P10-T-002)
- **日期**:2026-05-21
- **决策者**:协调者(用户)
- **相关**:ADR-0011 §3 Lab gating 政策(本 ADR 是 §3 carry tally 4th attempt entry 的 forward-looking ADR · 不修改 §3 policy core 但 codify 4th attempt rationale + 5th carry policy posture re-eval triggers)/ ADR-0001 v3(K8s baseline + 真硬件演进路径)/ ADR-0009 §4 npu-dra-driver path A partition-aware allocator(if K8s 1.36 + KEP-4815 GA per Phase 10 W3 T202 entry · Phase 11+ default carry · 本 ADR §3 candidate stream #6)/ ADR-0013 §5 Open question (a)(O2 IMS R1 v05.00+ spec migration · 本 ADR §3 candidate stream #8)/ ADR-0010 §7 Volcano gang-scheduling(本 ADR §3 candidate stream #7)/ `docs/checkpoint-phase9.md` §6 Phase 10 handoff brief(10 candidate workstreams · 本 ADR §3 8 streams 与之 cross-ref)/ `docs/phase10-plan.md` §1 Out-of-scope list(8 streams 来源)/ `docs/architecture.md` §1.3 Phase 10 row "真实硬件对接 + 演示打磨" · §13 review-table Phase 10 row · §14.1 风险表 row 1 "昇腾 910B 真机访问受限"

---

## §1 Context

### §1.1 Lab gating 3 prior defers 总览(ADR-0011 §3 carry tally cross-ref)

`docs/adr/0011-npu-dynamic-slicing-and-source-interface.md` §3 lab gating policy 自 Phase 7 起 codify:**default = defer · 仅当用户在 W2 entry meeting 明确 "lab access available" 时执行 lab-conditional task**。截至 Phase 9 end 累计 3 次推迟,均走 default policy 路径:

| Phase | Task | Date | 用户 signal at entry | Outcome | Carry # |
|---|---|---|---|---|---|
| Phase 7 | P7-T-101(Source.RealAscend impl on real silicon) | 2026-05-20 | no signal | deferred Phase 8 | 1st |
| Phase 8 | P8-T-105(Source.RealAscend body) | 2026-05-21 @ 988ec11 | no signal | deferred Phase 9 | 2nd |
| Phase 9 | P9-T-106(Source.RealAscend body + cann-driver-matrix stamp) | 2026-05-21 @ ac9e336 | no signal | deferred Phase 10 | **3rd** |

`docs/architecture.md` §1.3 Phase 10 row "真实硬件对接 + 演示打磨" 与 §14.1 风险表 row 1 "昇腾 910B 真机访问受限 · 影响 Phase 5+ 卡住 · 缓解 Phase 1-4 全部基于 Mock + 仿真,硬件就位再切" 同此 carry 主线 — Phase 10 是 plan-from-day-1 "硬件就位再切"的预定 phase。

`docs/research/k8s-partitionable-devices-spike.md`(P7-T-106 / P8-T-101)+ synthetic ring fixture(P7-T-104-v2 hard-fail upgrade + P8-T-104 reseed + P9-T-103 kind smoke ext)继续 cover CI 与 demo flow · lab smoke 不阻塞主干交付。

### §1.2 Phase 10 是 4th attempt 节点(arch §1.3 row 实质 deliverable)

`docs/phase10-plan.md` §1 Scope summary 主线 2 = **真实硬件对接** 主线 3 = **完整 multi-pool / multi-tenant 演示打磨**。两主线交汇点:**P10-T-102 [LAB-CONDITIONAL · 4th attempt] Source.RealAscend body** + **P10-T-201 真实硬件 multi-pool / multi-tenant 演示打磨**。

T102 4th attempt 同 P7-T-101 + P8-T-105 + P9-T-106 spirit 沿用 ADR-0011 §3 gating policy + Allowed Paths:
- 用户在 Phase 10 W2 entry meeting 明示 "lab access available · 在 W2-DX 起 N 天" → main agent 派 T102 subagent · brief 以 `[LAB-CONDITIONAL]` 开头
- 默认 / 无明确信号 → T102 走 deferred 路径(devlog + ADR-0011 §3 carry tally 4th entry + 本 ADR §2 Decision B carry escalate)

**为什么本 ADR 不并入 ADR-0011 §3**:ADR-0011 §3 是 lab gating 政策 *本体*(decision matrix + carry tally · across all phases 复用)。本 ADR 是 Phase 10 节点 *上下文*(4th attempt rationale · 5th carry posture re-eval triggers · T201 fallback path detail · Phase 11+ candidate streams enumeration)— 政策 + 上下文 分两 ADR 避免 ADR-0011 历史负担 + 本 ADR 是 Phase 10 forward-looking artifact。

### §1.3 本 ADR 不涉及

- ADR-0011 §3 lab gating *policy core* 修改 — 政策 default 不变(no signal → defer) · 仅在 §3 carry tally 表加 P10-T-102 4th attempt entry + 本 ADR cross-ref(P10-T-002 Allowed Paths 含)
- T102 Allowed Paths 具体清单 — 沿用 P9-T-106 Allowed Paths(`docs/phase10-plan.md` §3 P10-T-102 详)
- cann-driver-matrix 内容 — `docs/cann-driver-matrix.md`(P4-T-002 landed · lab gating entry gate)是 T102 entry gate · 本 ADR 不动其内容
- Phase 11+ task package 详细拆解 — 仅做 *candidate streams enumeration* + open questions · Phase 11+ entry meeting 起草 Phase 11 plan 时 detailed task packages

---

## §2 Decision

### §2.1 Decision A:T102 4th attempt detailed gating policy

**承诺范围**(沿用 ADR-0011 §3 default policy · P9-T-106 模式):

**Subagent brief prefix**:`[LAB-CONDITIONAL]` · 与 P7-T-101 + P8-T-105 + P9-T-106 一致。

**Allowed Paths**(per P9-T-106 + ADR-0011 §3 spirit · 详 phase10-plan §3 P10-T-102):
- `operators/npu-dra-driver/internal/source/real_ascend.go`(body)
- `operators/npu-dra-driver/internal/source/real_ascend_test.go`(unit tests)
- `tests/lab/phase10/`(只有 actually landed 时存在 · deferral 路径不落)
- `docs/cann-driver-matrix.md`(verification stamp · CANN X / driver Y / kernel Z 当日实测)
- `docs/adr/0011-npu-dynamic-slicing-and-source-interface.md`(§3 carry tally 表 4th entry update + Outcome 列填 "landed at SHA" 或 "deferred to Phase 11+ · 4th defer")
- `docs/devlog/phase-10-t102.md`

**Acceptance(deliverable contract)**:
- `npu-smi` real-binary call replace stubbed parser(P7-T-005 scaffold remains test fixture)
- `ResourceSlice` attribute populate from real silicon(node serial + HCCS ring topology + per-card AICore + memory · 同 P5-T-001 schema)
- PD-pair real placement hard-assertion(prefer-hccs-ring annotation 与真硬件 HCCS adjacency 匹配 · Phase 7 P7-T-008 synthetic 默认走 hard-fail-upgrade)
- cann-driver-matrix verification stamp(当日 lab session 实测 CANN X.Y.Z / NPU driver A.B.C / kernel K.L · 入 `docs/cann-driver-matrix.md` 表)
- `[LAB-CONDITIONAL]` smoke pass on real silicon(`tests/lab/phase10/install.sh` + `assert.sh` 5+ hard-asserts)

**Gating decision branches**(per Phase 10 W2 entry meeting · 同 phase7-plan §6 / phase8-plan §6 / phase9-plan §6 模式):

| 用户 W2 entry signal | T102 | T201 真硬件演示 | Checkpoint 记录 |
|---|---|---|---|
| "lab access available · 在 W2-DX 起 N 天 · cann X / driver Y / silicon: Ascend 910B Z 板 · access window N 天" | 派 | 真硬件路径 land | "T102 landed on real silicon at YYYY-MM-DD; CANN X / driver Y 入 cann-driver-matrix.md" + T201 master demo script 跑真硬件 multi-pool |
| "lab access deferred / Phase 10 窗口内不可得" | skip + 落 deferred devlog | synthetic ring fallback path(详 §2 Decision C) | "T102 4th defer Phase 11+ per W2 entry meeting YYYY-MM-DD" + ADR-0011 §3 carry tally 表 4th entry "deferred Phase 11+" |
| 无明确信号(default · 同 P7+P8+P9) | skip + 落 deferred devlog | synthetic ring fallback path | "T102 deferred Phase 11+ (no lab signal · 4th carry · per ADR-0011 §3 default policy + ADR-0016 §2 Decision B 5th carry posture re-eval trigger)" |

### §2.2 Decision B:5th carry to Phase 11+ rationale + posture re-evaluation triggers

**5th carry rationale**(若 T102 4th 仍 defer):
- 4 phases consecutive defer · 同 default policy 下游 — *政策本身是否仍 fit Phase 11+* 进入实质考虑空间
- 真硬件 fully landed 在 milestone 边界:M4 工程化对外 closer = Phase 10 · 若 M4 closer 时仍未 light up 真硬件,M5+ milestone 命名 + scope spine 必须正视(详 §4 Open question (c))
- synthetic ring fixture(`set-b-multi-ring` · Phase 7 P7-T-104-v2 hard-fail upgrade + Phase 8 P8-T-104 reseed + Phase 9 P9-T-103 kind smoke ext)5 phases 累计 + Phase 10 T107 ext 后 · 实质已 cover *CI 主干 + demo flow* 全部 — 真硬件 stamp 价值是 *verification* 而非 *deliverable substrate*

**Posture re-evaluation triggers**(任一触发即重评:政策是否从 "default = defer" 翻转 "default = light-up if not blocked")。3 distinct triggers:

| Trigger | 触发条件 | 重评 action |
|---|---|---|
| (1) Phase 11+ entry meeting | Phase 10 close-out + Phase 11+ plan 起草 chat | main agent (or 协调者) 提出 "lab gating policy 是否 default-defer 仍 fit M5+?" agenda item · default policy 翻转 / 维持 在 Phase 11+ plan ADR 中 codify |
| (2) Ad-hoc lab signal | 任意 phase 中期 chat 用户 "lab access materialized · 有 N 天 window" | main agent 立即评估当前 phase open tasks · 是否插入 T102-equivalent task · 沿 ADR-0011 §3 "Lab access 在 W2 期间临时材料化" exception path |
| (3) M5+ milestone reset | Phase 11+ scope spine 重定向(e.g. M5 production · M5 multi-site · M5 SLA hardening 等) → 真硬件 stamp 不再是 milestone closer 而成为 entry 前提 | milestone scope 起草时直接将 真硬件 lab onboarding 列为 *prerequisite* 而非 *deliverable* · 改变 carry default 语义 |

**5th carry checkpoint 记录格式**(if applicable at T204):
```
T102 5th defer to Phase 11+ · lab access not materialized in Phase 10 window ·
synthetic ring fixture(set-b-multi-ring · Phase 7+8+9+10 cumulative)continues
to cover CI + demo flow · Phase 11+ entry meeting per ADR-0016 §2 Decision B
trigger (1) reconsider gating-default policy or M5 milestone reset trigger (3)
```

### §2.3 Decision C:T201 fallback path detail(synthetic ring 演示路径 if T102 deferred)

**Phase 10 T201 真硬件 multi-pool / multi-tenant 演示打磨** 是 arch §1.3 Phase 10 row 实质 deliverable。**两路径**:

**路径 P(primary · T102 landed)**:
- T201 master demo script 跑真硬件 multi-pool flow:多 modelservice + 多 NPUSlicePool + Quota enforce + busy-idle scale + cache singleton + O2 DMS NB endpoint
- demo recording fixture 跑真硬件 stdout(kubectl + 真 npu-smi output)
- checkpoint § Phase 10 row 标 "with real Ascend hardware · CANN X / driver Y verified"
- arch §1.3 + §13 + §14.1 row 1 cross-ref "硬件就位" 实现 — 全 deliverable 兑现

**路径 F(fallback · T102 deferred 4th carry)**:
- T201 master demo script 跑 synthetic ring fixture flow(`set-b-multi-ring` · `tests/e2e/kind/phase10/` 模拟环境):
  - npu-dra-driver 走 `Source.SyntheticRing`(`operators/npu-dra-driver/internal/source/synthetic_ring.go` · Phase 7 P7-T-103+T104 baseline)
  - HCCS adjacency by ring fixture(P7-T-104-v2 hard-fail upgrade 后)
  - PD-pair placement 走 prefer-hccs-ring annotation 命中 synthetic ring · 同主路径行为
  - cache singleton + Quota + multi-pool + busy-idle scale + O2 DMS 全 land(不依赖真硬件)
- demo recording fixture 跑 synthetic stdout(kubectl + mocked npu-smi output)
- checkpoint § Phase 10 row 标 "with synthetic ring fallback"(明示不挂 "真硬件" 标签)
- arch §1.3 Phase 10 row 实质 deliverable accounting:**80% landed**(20% 缺真硬件 stamp · 完整 multi-pool / multi-tenant / Quota / cache / O2 DMS / propagation 全 land)

**Fallback 路径与 5th carry 关系**:
- Fallback 路径不是 Phase 11+ 入口 prerequisite · 是 Phase 10 *本期* 完成度的 graceful posture
- checkpoint § Phase 10 deliverable accounting 显示 80% landed + 20% lab-stamp 缺(以 sub-bullet 明示) · 让 Phase 11+ 决策者(用户)清晰看到何处需要补
- Phase 10 tag `phase-10-complete` 落定无论 P 或 F 路径(同 phase7-/phase8-/phase9-complete spirit · tag 与 lab gating 状态无关 per ADR-0011 §3)

---

## §3 Phase 11+ Candidate streams enumeration(8 streams)

**入 Phase 10 plan §1 Out-of-scope 来源** + checkpoint-phase9.md §6 Phase 10 handoff brief 补 + arch §13 Phase 11+ row(本 ADR T002 起草后由 T203 docs 大整理 入 §13 review-table)。8 streams 按 most-actionable 排序(同 phase9 §6 spirit):

### Stream 1 · 真实 multi-site / multi-机房 deployment(M4→M5 主线候选)

**Context**:Phase 10 ships single Karmada control + 1 member cluster (kind cluster 模拟 multi-site) · Karmada propagation 第一波 ships via T103/T104。Phase 11+ = 真 2+ member cluster · 跨机房 / 跨 region · K8s federation 真物理硬件。

**Prereq**:Phase 10 Karmada cross-cluster informer aggregation 验证 · lab onboarding 至少 1 member node 真硬件可用(Stream 与 T102 4th carry outcome 联动)。

**估算**:M5 candidate spine · 2-3 month scope estimate(详 Phase 11+ plan)。

### Stream 2 · Live migration of HCCL ranks + per-Pod RDMA bandwidth quota

**Context**:Phase 8 "重启切片" pattern sidesteps live migration · ADR-0012 §5 NPUVerticalScaler mutation model `annotation patch + claim_controller restart-bundle` — *not* live · Phase 8 deliverable 由 P&L 折中接受。Phase 11+ if HCCL live migration kernel/driver support materialises → 真 live migration path · annotation + claim_controller restart-bundle 模式 deprecated。

**Prereq**:CNI-level gaps per `docs/cni-hccl-research.md` §5 需 close · `huawei_ascend_npu_cni_plugin` 与 `hccn_tool` 必须暴露 live-migration 接口 / kernel-level HCCL rank migration · Phase 11+ if Huawei roadmap aligned。

**估算**:1-2 month · 强依赖 vendor signal(Huawei roadmap)。

### Stream 3 · Real fabric switch integration(SONiC / Cumulus / Arista API · 真 spine-leaf hops)

**Context**:ADR-0007 fabric discovery via LLDP 已 defer · Phase 7+ ADR-0010 §229 同 defer · 真物理 switch SDK call(SONiC Klish CLI · Cumulus NCLU · Arista eAPI)是 Phase 11+ on real switching gear 路径。

**Prereq**:Phase 11+ multi-site deployment land(Stream 1)+ 物理 switching gear(SONiC switch / 真 spine-leaf topology hardware)。

**估算**:2-3 weeks per switch SDK · 真物理 testbed 依赖。

### Stream 4 · vLLM PD 分离 production-grade SLA(P99 latency + multi-tenant isolation guarantees)

**Context**:Phase 5 PD Router webhook + ADR-0008 demo-grade · Phase 10 polish lab 验证(若 T102 landed) · Phase 11+ if production deployment signal → 真 multi-tenant P99 latency budget · isolation guarantees(per-tenant rate limit · per-tenant cache · burst control)。

**Prereq**:Phase 10 真硬件 stamp(T102 landed 或 5th carry path graceful)+ production SLO signal(用户 / 甲方真业务 P99 / P99.9 budget input)。

**估算**:1-2 month · SLO engineering depth dependent。

### Stream 5 · OIDC IdP 完整集成(Keycloak / Dex 部署 + 真 RBAC + audit log integration)

**Context**:Phase 10 T103 ships OIDC *client* + K8s SA + TokenReview · 但 IdP 部署(Keycloak / Dex)+ 真 federation provider integration(Active Directory · LDAP · Google Workspace · etc)Phase 11+ if 甲方真 IdP onboarding signal。

**Prereq**:甲方 IdP 选定(Keycloak vs Dex vs 外部 SaaS · 取决于甲方 governance) · ADR-0013 §6 Open question (a) "authn/z full deployment posture" 在 Phase 10 polish 时 close。

**估算**:0.5-1 month · IdP choice dependent。

### Stream 6 · KEP-4815 Partitionable Devices GA wait(if T202 deferred)

**Context**:`docs/research/k8s-partitionable-devices-spike.md`(P7-T-106 / P8-T-101 · KEP-4815 Beta confirmed at Phase 7)。Phase 10 T202 [DECISION-GATED] · 若 T003 baseline bump 1.36 + KEP-4815 GA per re-WebFetch at W3 entry → land partition-aware allocator(ADR-0009 §4 path A migration);否则 Phase 11+ wait。

**Prereq**:K8s 1.36+ baseline land(Stream 与 T003 outcome cascade) + KEP-4815 GA confirmed by upstream(K8s release notes · sig-node tracker)。

**估算**:land = 2-3d · 但触发条件等待是 main blocker(K8s release cadence)。

### Stream 7 · Volcano gang-scheduling install(if T108 2nd defer)

**Context**:`docs/research/volcano-gang-scheduling-spike.md`(P8-T-106 · 路径 A 推荐)· Phase 9 P9-T-101 deferred default policy · Phase 10 T108 [DECISION-GATED · 2nd defer or land] · 若 W2 entry training-job demo signal → install。否则 Phase 11+ wait training-job demo materialisation。

**Prereq**:training-job demo 实质需求(用户 / 甲方明示 Phase 11+ 训练场景 demo) · Volcano v1.10.x+ helm install · PodGroup atomicity + `schedulerName=volcano` opt-in。

**估算**:1-2d if installed · 0d wait if continued defer。

### Stream 8 · O2 IMS R1 v05.00+ spec migration(if breaking · T103 defer carry)

**Context**:Phase 9 ADR-0013 §5 (a) v04.00 lock · Phase 10 T103 W1 entry re-WebFetch v05.00 status · 若 non-breaking → 增量 migration in T103 · 若 breaking → carry Phase 11+ migration as separate ADR · T103 polish 仍 land 在 v04.00 baseline。

**Prereq**:O-RAN ALLIANCE WG6 release cadence v05.00 materialisation + spec drift surface acceptance check。

**估算**:0.5-1 month per major version migration · O-RAN spec drift breadth dependent。

### §3.x 与 checkpoint-phase9.md §6 cross-ref

checkpoint-phase9.md §6 列 10 candidate workstreams for Phase 10 · 本 ADR §3 8 streams 是 Phase 11+ candidate(*Phase 10 close-out 后剩余 + 新生*)· 两 list 不重叠。Phase 10 candidate workstreams(checkpoint §6) → Phase 10 task chain consume(per `docs/phase10-plan.md`) → Phase 11+ candidate streams(本 §3)是 *剩余* + *新生 forward issue* enumeration。

---

## §4 Open questions

### (a) Lab access posture re-evaluation timing

ADR-0011 §3 default policy(无 signal → defer)是 Phase 7-9 实战 codify。4th carry(Phase 10 T102 if continued defer) 触发 §2 Decision B re-eval trigger (1) · 但具体 *timing* 仍 open:

- Phase 11+ entry meeting 直接重评(default trigger)
- Phase 10 mid-phase(W2 / W3)若已确知 lab 不可得 + Phase 11+ scope 也无 lab onboard signal → 提早重评
- 等 5th defer 实际发生再重评(保守 path)

**当前倾向**:Phase 11+ entry meeting 重评 + 若 mid-phase Phase 11+ scope clarify signal 出现可 escalate · M5+ milestone scope 是触发点。

### (b) Phase 11+ scope primary spine

8 streams 是 candidate list · 但 Phase 11+ *primary spine* 仍 open(Phase 11+ 一般 5-6 weeks calendar · 单 phase 不可能全 8 stream land):

- Spine A:**真生产化**(Stream 1 multi-site + Stream 4 P99 SLA + Stream 5 OIDC IdP 完整)→ M5 = production hardening milestone
- Spine B:**真硬件 fully landed + 高级特性**(T102 5th carry land + Stream 6 Partitionable Devices + Stream 2 live migration)→ M5 = hardware-native milestone
- Spine C:**生态扩展**(Stream 7 Volcano gang training-job + Stream 8 O2 IMS v05.00+ + Karmada control HA)→ M5 = ecosystem extension milestone

**当前倾向**:depend on 甲方 / 用户 Phase 10 close-out 实际反馈 + lab access posture · Phase 11+ entry meeting 起草 Phase 11 plan 时 decide。

### (c) Milestone naming Phase 11+(M5? 真生产化?)

arch §1.3 milestone 序列截至 M4(Phase 10 closer)· Phase 11+ milestone naming open:

- **M5 真生产化**(Spine A 主线)— 与 §3 Stream 1+4+5 主线 align
- **M5 真硬件-native**(Spine B 主线)— 若 T102 5th carry 触发 milestone reset(§2 Decision B trigger 3)
- **M5 生态扩展**(Spine C 主线)— 与 §3 Stream 7+8 align
- **M5+ 多 milestone 拆**(若 Phase 11+ scope 实质 > 1 phase calendar)— Phase 11+/M5 仅 Spine A subset · Phase 12+/M6 Spine B subset · etc

**当前倾向**:Phase 11+ entry meeting 起草 Phase 11 plan 时 decide · 与 §4 Open question (b) primary spine 联动。

---

## §5 引用

### 上游(本 ADR 决策依据)
- ADR-0011 §3 Lab gating 政策(本 ADR 沿用 default policy · 不修改 policy core)
- ADR-0011 §3 carry tally 表(P7+P8+P9 = 3 prior defers · 本 ADR T002 起草后加 P10-T-102 4th attempt entry via Allowed Paths)
- `docs/architecture.md` §1.3 Phase 10 row · §13 review-table · §14.1 风险表 row 1
- `docs/checkpoint-phase9.md` §6 Phase 10 handoff brief(10 candidate workstreams · 与本 §3 8 streams 不重叠)
- `docs/phase10-plan.md` §1 Scope summary + §1 Out-of-scope list(8 streams 主来源) + §3 P10-T-002 acceptance(本 ADR 是 T002 deliverable)
- `docs/research/k8s-partitionable-devices-spike.md`(P7-T-106 / P8-T-101 · Stream 6 依据)
- `docs/research/volcano-gang-scheduling-spike.md`(P8-T-106 · Stream 7 依据)
- `docs/cni-hccl-research.md` §5(P5-T-105 · Stream 2 CNI-level gaps 依据)
- `docs/cann-driver-matrix.md`(P4-T-002 · T102 4th attempt entry gate)

### 下游(本 ADR 后续工作 / 触发 Phase 11+ entry)
- P10-T-102 [LAB-CONDITIONAL · 4th attempt] Source.RealAscend body(本 ADR §2 Decision A 是 policy lock · subagent brief 起手 reference)
- P10-T-201 真硬件 multi-pool / multi-tenant 演示打磨(§2 Decision C 路径 P 或 F selector)
- P10-T-203 docs 大整理 + Phase 11+ 前瞻(本 ADR §3 8 streams 入 arch §13 Phase 11+ row · arch §13 review-table promote)
- P10-T-204 Phase 10 checkpoint + tag(本 ADR §2 Decision B 5th carry path 在 checkpoint § Phase 10 row 落定)
- Phase 11+ entry meeting · Phase 11 plan 起草 — §4 Open questions (a) (b) (c) decide

### 上游 commit chain(决策时 grep-verified)
- P7-T-101 deferred Phase 8(ADR-0011 §3 carry tally 第 1 行 · phase-7-complete @ bf0c03c 系列)
- P8-T-105 deferred Phase 9 @ 988ec11(ADR-0011 §3 carry tally 第 2 行)
- P9-T-106 deferred Phase 10 @ ac9e336(ADR-0011 §3 carry tally 第 3 行)

---

**END of ADR-0016**
