# P12-T-001 · ADR-0019 Phase 12 entry decisions(起草 + Accepted)

- **Commit**: (this commit)
- **Date**: 2026-06-01
- **Duration**: 0.5d plan / ~0.4d actual(docs-only · 无 build/test)

## Intent

锁 Phase 12 entry 4 决策 + close-out candidate-streams §Spine candidates 留的 entry-decide open:
- Spine = **平台真实化(aarch64 鲲鹏 + openEuler)+ 拓扑数据全保真 + 前端 one-page workspace**(非 Spine A production-hardening · candidate-streams §Spine D naming reset 变体)
- Milestone = **M6 平台真实化 + 操作台一体化**(ADR-0017 §2 Decision B "M6 production hardening" forward note 的 naming reset · 原 cohort 顺延 Phase 13+)
- 3 active carry tracks(lab 6th / Volcano 4th / Partitionable 3rd)**维持 default-defer**(与 Phase 12 scope 无 direct dependency)
- 3 track 优先级 + **contract gate 串行**(T003 api-contract 先 land · gate Track C gen:types)

为 W1 task chain(T002-T004)+ 后续 15 task 起手提供 decision lock,subagent/main agent 起手可 reference 本 ADR 不必每次重述决策。

## Path adaptations(plan literal vs codebase reality)

1. **§1.3 phase 路线图 M6 row 本 task 即添加**(与 Phase 11 T001 相反):Phase 11 T001 devlog Path adaptation #2 记 "§1.3 unchanged · 留 T203/T204 promote"。但 Phase 12 plan §3 P12-T-001 Allowed Paths + Acceptance **显式**要求 "§1.3 phase 路线图 M6 naming" —— 故本 task 直接在 §1.3 加 M6/Phase 12 row(deliberate difference · 非疏漏)。§13 review-table 则同 Phase 11 pattern 加 "in flight" row(T302 promote "landed")。
2. **无独立 `docs/tasks/P12-T-XXX.md` 任务包文件**:Phase 12 task 定义 inline 在 `docs/phase12-plan.md` §3-§6(同 Phase 10/11 节奏)· 不另起 task package 文件 · acceptance 直接读 plan。

## Debugging trail

纯 docs · 无 build/test fail · 1 个 Edit 工具 anchor mismatch:

1. **§13 review-table Phase 12 row 插入 anchor 第一次失败** — 首次 old_string 包含 Phase 11 row 结尾 + 下一行 Phase 5+ row("NPU pod 网络考量（CNI...")· Edit 报 not found。根因 = Phase 5+ row 用半角 `(` 而我 old_string 写了全角 `（`(中文输入惯性)· Edit 的 \uXXXX swap 也未匹配 → 说明 mismatch 在括号字符。**修法**:缩小 anchor 到仅 Phase 11 row 唯一结尾 "详 `docs/checkpoint-phase11.md` §6 Phase 12+ handoff brief。|" 单行 · 一次过。**教训**:多行 anchor 跨已有行时,任一字符(尤其中英标点)不符即整体失败 · 优先选**最小唯一单行** anchor(P4 "Edit 完必 verify" 的前置 · 减少 anchor 面积)。

## Key decisions

- **Spine = 平台真实化 + 拓扑全保真 + one-page UI**(不是 candidate-streams 预设的 Spine A production-hardening):
  - 用户 2026-06-01 两决策 redirect:决策 1 = 真实化目标平台(amd64-only → aarch64 鲲鹏 + openEuler · Atlas 800 原生配置)· 决策 2 = 拓扑全保真 + 操作台一体化 one-page workspace
  - 拒绝 Spine A production-hardening:强依赖甲方 SLO + IdP 选定 + 真业务 traffic(三块 input 未到位)· 顺延 Phase 13+
  - 是 candidate-streams §Spine D "多 milestone 拆" 变体(平台 + UI 一体化是独立 milestone slot · production-hardening 顺延)
- **Milestone M6 naming reset**:ADR-0017 §2 Decision B 预设 M6 = production hardening · 本 ADR reset M6 = 平台真实化 + 操作台一体化 · 原 cohort → Phase 13+ 真 production-hardening milestone
- **关键 disambiguation 写入 §2 Decision C**:Phase 12 "Track A 平台真实化" = **构建/部署 target** 反转(交叉编译 + buildx 双架构 + helm template 校验 · amd64 开发机可完成)· **≠** "真硬件运行验证"(真鲲鹏 + 昇腾集群 · lab-gated · 6th carry · 留 Phase 13+)。不分清会误判 Phase 12 已 close lab carry(P2 边界 + P5 弱链:防 "平台化 = 真硬件验证" 的偷换)
- **contract gate 串行**:T003 api-contract(本 phase 唯一 RFC · main-agent serial 共享契约 per root CLAUDE.md §11)未 land → Track C 前端 gen:types 无法跑 · ADR-0020 平台 target 未记前先做 Dockerfile 不合规 → W1 4 task 全 main-agent serial

## Verification

P3 三项验证维度 全过(docs-only task · 无 compile/test):

- **存在性**:ADR-0019 / arch.md 2 edits / 本 devlog 三文件落地 · ADR §5 引用的 candidate-streams / ADR-0017 §2 Decision B / ADR-0001 §13 / ADR-0011 §3 / ADR-0004/0005/0006/0013/0014 / checkpoint-phase11 / demo-runbook.html / phase12-plan.md 全部 grep-verified 真存在(读取确认:ADR-0001 §13 line 218-222 原文 "不支持 ARM / 鲲鹏 · 仅支持 amd64 / x86_64 + 昇腾 910B · 所有镜像构建仅生成 amd64 layer · mock 数据 arch: amd64 硬约束")
- **完整性**:plan §3 P12-T-001 acceptance 5 项逐项核 — ① ADR §1 Context cites candidate-streams + 2026-06-01 两决策(§1.2 + §1.3)✓ ② §2 Decision A-D(spine §2.1 / milestone §2.2 / carry-defer §2.3 / track 优先级 + contract gate §2.4)✓ ③ §3 16-task scope cross-ref §2 ✓ ④ §4 3 Open questions(真 aarch64 lab posture / 退役 4 页 deep-link / Splitter 折叠默认)✓ ⑤ arch §13 Phase 12 "in flight" row + §1.3 M6 naming row ✓
- **正确性**:ADR §1 Context cited fact grep-verified —— Phase 12 plan 4 commit SHA(`7e9b1d2` 初版 + `d382a59` + `e61469e` + `84546c1` baseline · 与 git log 一致)· candidate-streams §Spine candidates 4 候选 + §Active carry tracks 3 track 项数与源文件一致 · ADR-0017 §2 Decision B forward note 内容 cross-ref · 16 task enumeration 与 plan §2 task package overview 逐 task 对应无遗漏

## Carry-forward

- **P12-T-002 ADR-0020 aarch64 + openEuler target**:本 ADR §2 Decision A Track A · supersede ADR-0001 §13(line 218-222 原文已 verify · T002 改其 status flip "SUPERSEDED by ADR-0020")· root + project CLAUDE.md §2/§5 改写 + 横向扫 amd64/鲲鹏
- **P12-T-003 ADR-0021 + api-contract.yaml**:本 ADR §2 Decision D contract gate · 本 phase 唯一 api-contract RFC · main-agent serial · gen:types drift check 后续 CI 关注点
- **P12-T-004 ADR-0022 + frontend DESIGN**:本 ADR §4 (b) 退役 4 页 deep-link + (c) Splitter 折叠默认 由 T004 ADR-0022 §2 Decision D + §4 codify
- **P12-T-101/T103 Track A · T104/T105 Track B**:§2 Decision A track 表 · §2 Decision D W2 优先级(跨模块 parallel if user batch · default serial · T105 mock schema 共享契约 main-agent serial)
- **P12-T-201..T205 Track C**:依赖 T003 gen:types + T104 后端边 + T004 UI IA · render-verify(plan §8)各 task ≥1 张 Playwright after-*.png(before-*.png 已 commit 84546c1)
- **P12-T-302 checkpoint + tag**:本 ADR §2 Decision B M6 平台真实化 + 操作台一体化 milestone CLOSER · §2 Decision C 3 carry tracks outcome stamp
- **Phase 13+ entry meeting**:本 ADR §2 Decision B production-hardening cohort 顺延 + §2 Decision C Track A 真 aarch64 lab carry closer 评估窗口

## §0a.10 / §0a.11 compliance

- 本 task 在 fresh execute session 起(per memory `feedback_plan_vs_execute_session_split.md`)· plan session 已 commit + 停在 `7e9b1d2`..`84546c1`(Phase 12 plan + render baseline 4 commit)· 本 session 是 T001 execute(用户 "执行 phase 12 plan任务,从 P12-T-001 开始")
- §0a.11 docs-only 例外:main agent 直接做 · 无 subagent · strict verify = grep cross-refs + acceptance 5 项逐项核 + arch.md 2 edits 落点确认 + ADR-0001 §13 原文 Read 确认
- **rhythm(per memory `feedback_strict_per_task_verify` v2)**:commit 后**默认接下一 task**(不每 task 必停 · 仅必要交互时停)· 用户已说 "执行...从 T001 开始" = 连续执行 mandate · 本 session 续 T002(per v2 "用户已说'执行'即不停")
- 不 push remote · 累积到 phase-12-complete tag(per memory `feedback_push_at_phase_tag_only.md` · 全链一次 push 触发 CI gate)
