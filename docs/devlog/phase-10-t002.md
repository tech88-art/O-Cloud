# P10-T-002 · ADR-0016 lab onboarding 4th attempt + Phase 11+ 前瞻

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 0.5d plan / ~0.5d actual

## Intent

Codify Phase 10 节点 lab gating 4th attempt 上下文(rationale + 5th carry posture re-evaluation triggers + T201 fallback path) + Phase 11+ 8 candidate streams enumeration · 为 P10-T-102 subagent brief + P10-T-201 真硬件演示打磨 + P10-T-204 checkpoint + Phase 11+ entry meeting 提供 forward-looking artifact。本 task 不修改 ADR-0011 §3 policy core — 政策 / 上下文 分离 · ADR-0011 是 policy 本体 · ADR-0016 是 Phase 10 节点上下文 + 前瞻。

## Path adaptations(plan literal vs codebase reality)

3 处偏离 / 选择:

1. **8 streams 列表 plan §1 Out-of-scope vs plan §3 P10-T-002 acceptance text 不一致**:
   - Plan §1 Out-of-scope(plan 顶部)8 items:HCCL live migration / fabric LLDP / OIDC 完整 / Karmada control HA / O2 IMS v05.00+ / vLLM PD production SLA / 真 multi-site / Real fabric switch
   - Plan §3 P10-T-002 acceptance §2 Decision C text 列 8 streams:real multi-site / live migration / fabric switch / production SLA / OIDC IdP 完整 / KEP-4815 GA wait / Volcano gang-scheduling / O2 IMS v05.00+
   - 差异:plan §1 含 Karmada control HA · 不含 KEP-4815/Volcano;plan §3 acceptance text 反之
   - 解决:用 plan §3 P10-T-002 acceptance text(它是 T002 deliverable contract)· §3.x 加 cross-ref 段 explicit 与 checkpoint-phase9 §6 10 Phase 10 candidate 不重叠

2. **arch §13 Phase 10 row 是 ADD 不是 promote**:
   - Plan acceptance "arch §13 review-table Phase 10 row '真实硬件对接 + 演示打磨' promote 加 'in flight via T102+T201 + ADR-0016 fallback path'"
   - 实际 grep §13 review-table:只有 Phase 7/8/9 rows(880 line table)· Phase 10 row 不存在
   - 解决:add new Phase 10 row 到 §13 table 末尾(line 877 之后)· 同 Phase 7/8/9 "landed" rows 风格 + Phase 9 "in flight via ADR-XXXX" 风格 综合 · "promote" 语义 = "add row with in-flight status"

3. **ADR-0011 §3 carry tally 表加第 4 行 Outcome TBD**:
   - 前 3 行 Outcome 是已知(deferred Phase N+1)· 第 4 行 P10-T-102 在 W2 entry 未发生 · Outcome 必须 TBD
   - 解决:表 4th 行 Outcome 列填 "TBD (W2 entry)" + "若 5th carry → 触发 ADR-0016 §2 Decision B posture re-eval trigger (1)" · 与 ADR-0016 §2 Decision B 联动 · 等 T102 实际 outcome 时 update

## Debugging trail

无 build / test fail · 纯 docs · 但有 2 处验证点:

- **arch §13 Phase 10 row 不存在**:grep `^\| Phase 10 \|` arch.md 只找到 §1.3 roadmap line 48 一处 · §13 table 内无 Phase 10 row · 验证 "promote" 实际是 "add" · 避免误以为 Phase 10 row 已存 + 错误 Edit
- **ADR-0011 §3 carry tally policy core 保留**:plan 说 "small edit · §3 carry tally 表 4th entry"· 我 explicit 在 ADR-0016 §1.3 + ADR-0011 §3 update 注脚 标 "policy core 不修改" · 防 ADR-0011 §3 decision matrix / Subagent brief 约定 / Phase 7 入口决策矩阵 等核心被误改

## Key decisions

- **政策 / 上下文 分离**:ADR-0011 是 lab gating policy 本体(decision matrix + carry tally · across all phases 复用)· ADR-0016 是 Phase 10 节点上下文(4th attempt rationale · 5th carry trigger 集 · T201 fallback path · Phase 11+ outlook)· 不并入 ADR-0011 §3 避免历史负担 + 保 ADR-0011 简洁
- **沿用 ADR-0011 §3 default policy**:no signal → defer · `[LAB-CONDITIONAL]` subagent brief prefix · 与 P7/P8/P9 一致 · T102 4th attempt 不引入新 gating semantic
- **5th carry posture re-eval 3 distinct triggers**:Phase 11+ entry meeting / ad-hoc lab signal / M5+ milestone reset · 三 trigger 任一启动 lab gating policy 是否翻转评估
- **T201 fallback path 80% / 20% accounting**:路径 P 真硬件 100% deliverable · 路径 F synthetic ring 80% landed(20% 缺真硬件 stamp)· checkpoint § Phase 10 row 用不同标签明示 — 让 Phase 11+ 决策者清晰看到 milestone 完成度
- **Phase 11+ 8 streams 来源 = plan §3 acceptance**:不是 plan §1 Out-of-scope(两 list 略不同) · 因为 plan §3 P10-T-002 acceptance 是 deliverable contract
- **§4 Open questions 3 项 留 Phase 11+ entry meeting decide**:lab posture timing / Phase 11+ primary spine(A 真生产化 / B 真硬件-native / C 生态扩展) / M5+ milestone naming

## Verification

P3 三项验证维度 全过(docs-only task · 无 compile/test):

- **存在性**:`ls docs/adr/0016-lab-onboarding-and-phase-11-outlook.md` ✓;`grep -n ADR-0016` 在 ADR-0011 + arch.md 各 1 处命中 + arch §14.1 row 1 1 处命中
- **完整性**:plan §3 P10-T-002 acceptance 7 项逐项核(§1 Context cites 7 sources / §2 Decision A 4th attempt detailed gating + Allowed Paths reference / §2 Decision B 5th carry rationale + 3 distinct posture re-eval triggers / §2 Decision C T201 fallback path detail / §3 Phase 11+ 8 streams / §4 3 Open questions / ADR-0011 §3 + arch §13 + §14.1 row 1 cross-ref · 3 cross-ref status flip)
- **正确性**:所有 carry tally SHA grep 验证(P7-T-101 commit 在 phase-7-complete chain · P8-T-105 @ 988ec11 · P9-T-106 @ ac9e336);8 streams 与 plan §3 acceptance text 1:1 mapping;3 triggers 与 ADR-0011 §3 既有 "Lab access 临时材料化" exception path spirit 一致

## Carry-forward

- **P10-T-102 [LAB-CONDITIONAL · 4th attempt]**:subagent brief 起手直接 reference ADR-0016 §2 Decision A Allowed Paths + Acceptance deliverable contract;Outcome 决定 ADR-0011 §3 carry tally 第 4 行填 "landed at SHA" 或 "5th carry Phase 11+"
- **P10-T-201 真硬件 multi-pool 演示打磨**:§2 Decision C 路径 P / F selector — depending on T102 outcome · master demo script + recording fixture 与 fallback path 配套
- **P10-T-203 docs 大整理**:本 ADR §3 Phase 11+ 8 streams 入 arch §13 Phase 11+ row · ADR-0016 是 §3 source-of-truth
- **P10-T-204 Phase 10 checkpoint + tag**:§2 Decision B 5th carry path checkpoint 记录格式已 codify · 直接 paste 用
- **Phase 11+ entry meeting**:§4 Open questions 3 项 driven Phase 11 plan 起草 · Phase 11+ scope primary spine (A/B/C) 选择 driven M5+ milestone naming

## §0a.10 / §0a.11 compliance + memory adherence

- 本 task 在同 execute session 起 T002 · 紧接 T001 commit `ba26935` · per `feedback_strict_per_task_verify.md` "默认按 plan 顺序连续推进"
- §0a.11 docs-only 例外:main agent 直接做 · 无 subagent · strict verify
- per `feedback_push_at_phase_tag_only.md`:commit 后停 · 不问 push · 不主动 push · 累积本地等 phase tag
