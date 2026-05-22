# P11-T-001 · ADR-0017 Phase 11 entry decisions(起草 + Accepted)

- **Commit**: (this commit)
- **Date**: 2026-05-22
- **Duration**: 0.5d plan / ~0.4d actual(docs-only · 无 build/test)

## Intent

锁 Phase 11 entry 4 决策 + close-out ADR-0016 §4 三项 open questions:
- Spine A 真生产化 foundation **subset**(不是全 Spine A · 不是 B/C/D)
- Milestone naming = **M5 真生产化 foundation**(foundation 后缀 anti-over-promise)
- Lab gating 5th attempt **default-defer 维持**(trigger 1 fire · trigger 3 留 Phase 12+)
- Chart packaging spine 优先级 6 档(T003 → T004/T005/T006 → T007 → T008)

为 W1 task chain(T002-T008)起手提供 policy lock,subagent brief 起手可 reference 本 ADR 而不必每次重述决策。

## Path adaptations(plan literal vs codebase reality)

2 处文字偏离 plan · 按 P10-T-001 同 pattern 处理:

1. **Plan "§5.1 演示后端 §3.2 数据源抽象" cross-ref 语义错位**(同 P10-T-001 Path adaptation #3 遗留):
   - arch.md §3.2 实际是 "前端"(line 176)· 不是 "数据源抽象"
   - "数据源抽象" 概念在 §5.1 datasource.Source interface(line 322)+ ADR-0001 §10 — 本 task Allowed Paths 不含 ADR-0001 · 无法走 P10-T-001 fallback
   - 解决:arch.md §5.1 cache 行 inline cross-ref T003 + ADR-0017 §2 Decision D(line 313 同 P10-T-001 落点)· arch.md §3.2 前端 表后加一行 forward note 指向 P11-T-105 Frontend src/ extension + ADR-0017(plan 字面 §3.2 cross-ref 落地)· 两 cross-ref 互补 · 不重复

2. **§1.3 phase 路线图表 Phase 11 row 不添加**(per plan literal "§1.3 phase roadmap unchanged"):
   - 这是有意的 conservatism:§1.3 table 是 M1-M4 历史 stable view · Phase 11+ row 加在 §13 review-table(同 Phase 10 row "in flight via..." 落点 pattern)· §1.3 等 phase-11-complete tag 时 由 T203/T204 docs 大整理一并 promote
   - 解决:严格遵守 plan 字面 · §1.3 unchanged · 仅 §13 加 Phase 11 row(在 Phase 10 行之上 · per upper-row newer convention 推测;若 review 反馈 ordering 应反 · T203 修正)

## Debugging trail

无 build / test fail · 纯 docs · 但有 2 个小决策反复:

1. **Phase 11 row 在 §13 ordering 修正一轮** — 首次插入把 Phase 11 row 放在 Phase 10 row *之上*(因 Edit 工具最初 anchor 选了 Phase 10 行 · 在其前插入实现最简)· verify 阶段 `git diff` 复核发现 §13 review-table 2026-05-17 评审追加段的实际历史 ordering 是 "newest at end"(Phase 4 → 5 → 7 → 9 → 9 → 10 · Phase 5+ / Phase 6 之后另有 RFC-003 段独立追加)· Phase 11 应该 follow Phase 10。修法:Edit revert 把 Phase 11 row 从 Phase 10 之上撤掉 · 在 Phase 5+(下一段 header 前最后一行)之前重插一行 · Phase 11 自此成为 review-table 该段末行。**教训**:Edit 完后必 `git diff` 复核 ordering 是否合理 · 不能"插入成功"即认正确(P3 "Verify-before-claim" + P4 "纵向 + 横向" 体现)。
2. **§2 Decision A 主线 / 子线表是否复述 phase11-plan.md §0 Goal** — 复述会 bloat ADR · 但跳过会让 ADR 读者必须跳读到 plan · 选 *简表 + cross-ref*(table 含 task 列 + plan 自带详 Allowed Paths)· 不复述 Allowed Paths / Acceptance(plan §3-§5 已 own)。

## Key decisions

- **Spine = A 真生产化 foundation subset**(不是 B 真硬件-native / 不是 C 生态扩展 / 不是 D 多 milestone 拆):
  - Spine A 完整 = Stream 1 multi-site + Stream 4 P99 SLA + Stream 5 OIDC IdP 完整 · 单 phase 5-6 week 不可能全 land
  - 实际可 land = Stream 1 foundation(Karmada 第一波 + 2 member kind cluster 单机模拟)+ Stream 5 part 1(OIDC client + TokenReview chart wiring substrate · 完整 IdP 留 Phase 12+)+ 9 chart packaging primary work 全 close · 这就是 "foundation subset"
- **拒绝 Spine B**:T101 5th attempt default-defer 维持 → 真硬件 stamp 仍 80% 缺(synthetic ring fallback 同 Phase 10 path)· Spine B 强依赖真硬件 stamp · 与 default-defer policy 抵触
- **拒绝 Spine C**:Volcano gang-scheduling 需 training-job demo 实质需求 · 当前 inference + multi-cluster 主导 · 无 training signal
- **Milestone = M5 真生产化 foundation**(foundation 后缀关键):
  - 不 over-promise:Phase 11 *不* close 完整 production hardening · 完整 P99 SLA / 完整 OIDC IdP / 真多机房 / Karmada HA / vLLM PD 分离 production-grade isolation 全留 Phase 12+
  - *closes* production hardening *foundation*:chart packaging 闭环 + Karmada propagation 第一波 + Frontend src/ Workload page 3 indicators
  - Phase 12+/M6 在此 foundation 上加 production hardening 完整 cohort · M6 naming 留 Phase 12+ entry meeting
- **Lab gating 5th attempt default-defer 维持**(沿用 ADR-0011 §3 policy core · 不修改):
  - Trigger 1(Phase 11+ entry meeting)FIRED at this ADR · 重评结论 = default-defer 维持
  - Trigger 2(Ad-hoc lab signal)stays armed
  - Trigger 3(M5+ milestone reset)保留 Phase 12+ entry meeting · 不在本 ADR fire(若 5th 仍 defer + Phase 11 W3 80% landed 后 Phase 12+ entry 起草时 evaluate 3 选项 a/b/c)
- **Chart packaging spine 6 档优先级**(per checkpoint-phase10 §6 + phase11-plan §2):
  - 1st T003 demo-backend(Lease leader-elect substrate 已 land · T201 真 multi-site demo 必需)
  - 2nd-4th T004/T005/T006 IMS chart(3 controller body 已 land · 独立 module · 用户 batch cue → parallel subagent;default serial main agent · per §0a.11)
  - 5th T007 sched-plugin NRT bundle(known-issues #12 完整 close 循环最终步 · 与 chart packaging 独立 module · 不阻塞)
  - 6th T008 inference-operator chart edit(0.5d 最简单 · W1 末尾 short task)

## Verification

P3 三项验证维度 全过(docs-only task · 无 compile/test):

- **存在性**:`ls docs/adr/0017-phase-11-entry-decisions.md` ✓ ;ADR §5 引用 8 行 ADR / checkpoint / arch cross-ref 全部对应真存在文件(grep verified)
- **完整性**:plan §3 P11-T-001 acceptance 8 项逐项核 — §1 Context(Phase 10 closer + M4 milestone + ADR-0016 §4 + checkpoint-phase10 §6)+ §2 Decision A(spine = chart packaging + Karmada 第一波 + Frontend src/ · 不含 P99 SLA / 完整 IdP / 真多机房)+ §2 Decision B(milestone = M5 真生产化 foundation · foundation 明示)+ §2 Decision C(lab default-defer 维持 5th + trigger 1 fire + trigger 3 保留 Phase 12+)+ §2 Decision D(6 档优先级排序)+ §3 Phase 11 scope detailed enumeration(20 main + 4 sub-track)+ §4 Open questions 3 项(Karmada HA · Frontend i18n · Lease Pod 数)+ §5 引用 full list — 全部覆盖
- **正确性**:ADR §1 Context cited fact 全部 grep-verified(`04cc009` Phase 10 tag · `7203587` / `3729bb3` post-tag fix · `5a17fe3` plan commit · ADR-0016 §2-§4 内容 cross-ref · checkpoint-phase10 §6 9 chart packaging primary work + 8 candidate streams 项数 verified · ADR-0011 §3 default policy 内容 cross-ref);决策 rationale 与 ADR-0016 §3 Stream 1-8 enumeration 严格对应(本 ADR §2 Decision A 提 Stream 1+5 in-scope · Stream 2/3/4/6/7/8 留 Phase 12+ · §3 8 streams 表 grep-verified 无遗漏)

## Carry-forward

- **P11-T-002 ADR-0018 Karmada deployment topology**:本 ADR §2 Decision A 主线 2 + §2 Decision D 7th-9th 子契约 · T002 起手 reference 本 ADR §2 Decision A "host + 2 member kind cluster minimum"
- **P11-T-003 demo-backend chart**:本 ADR §2 Decision D 1st 优先级 · T003 subagent brief 起手 reference 本 ADR + ADR-0015 §3.3 substrate
- **P11-T-004/T005/T006 3 IMS chart**:本 ADR §2 Decision D 2nd-4th 优先级 · 独立 module · 用户 batch cue → parallel subagent;default serial main agent per §0a.11
- **P11-T-101 LAB-CONDITIONAL 5th**:本 ADR §2 Decision C policy lock · subagent brief 起手 `[LAB-CONDITIONAL]` · W1 entry meeting 用户 signal 决定 lab-cond 路径 vs deferred 路径
- **P11-T-203 docs 大整理 + Phase 12+ 前瞻**:本 ADR §2 Decision B M5 真生产化 foundation milestone narrative + §3 Phase 11 scope enumeration + §4 Open questions 3 项 follow-up
- **P11-T-204 Phase 11 checkpoint + tag**:本 ADR §2 Decision C 5th carry outcome stamp 在 checkpoint § Phase 11 row 落定 · M5 真生产化 foundation milestone CLOSER announcement
- **Phase 12+ entry meeting · M6 milestone naming**:本 ADR §2 Decision C trigger 3 评估窗口 + §2 Decision B "Phase 12+/M6 在此 foundation 上加 production hardening cohort" forward note · Phase 12 plan 起草时 codify
- **Open questions 3 项 entry meeting confirm**:Karmada HA 单 replica 仍 default · Frontend i18n 倾向 en-US + zh-CN 双语 entry confirm · demo-backend Lease replicaCount 2 default 不动

## §0a.10 / §0a.11 compliance

- 本 task 在 fresh execute session 起 · plan session 已 commit + 停在 `5a17fe3`(Phase 11 plan)+ 数次 P11-fix session 各自 commit + 停 · 本 session 是 T001 execute(per memory `feedback_plan_vs_execute_session_split.md`)
- §0a.11 docs-only 例外:main agent 直接做 · 无 subagent · strict verify = grep cross-refs + acceptance 8 项逐项核 + arch.md edits 落点确认 + devlog Carry-forward 跨 task ref 实际存在性 grep
- commit 后停 · 不主动续 T002 / T003 · 等用户指明下一 task(per memory `feedback_strict_per_task_verify.md`)
- 不 push remote · 累积到 phase-11-complete tag(per memory `feedback_push_at_phase_tag_only.md`)
