# P7-T-107 · Phase 7 checkpoint + tag phase-7-complete

- **Commit**: (this commit)
- **Date**: 2026-05-20
- **Duration**: plan 0.5d / actual ~0.5d(checkpoint 文档 + README + arch §13 row promote + phase7-plan tail filled + tag)

## Intent

Per phase7-plan §4 P7-T-107:write the Phase 7 checkpoint document mirroring `docs/checkpoint-phase6.md` structure · update README + architecture §13 · fill phase7-plan tail · land `phase-7-complete` git tag at this commit。

## Path adaptations

- 计划 §4-T107 Allowed Paths 全部 covered:
  - `docs/checkpoint-phase7.md`(NEW · 6 sections per checkpoint-phase6 pattern: deliverables + wired diagram + test posture + DoD reconciliation + known issues + Phase 8 handoff brief)✅
  - `docs/architecture.md` §13 Phase 7 评审追加 row 状态字段从 "landed 2026-05-20 P7-T-001 → ADR-0011 ..." 升级为 "landed phase-7-complete (2026-05-20) — ... 14 task chain bd6f223..bf0c03c"(列出全部 commit hash 范围 + 三个 gating 决策 outcomes)✅
  - `docs/known-issues.md`(no new entries — Phase 7 net-new entries 已在各自 task 落 · 此 commit 不再加)✅(no-op per Phase 7 evidence)
  - `docs/phase7-plan.md` "Phase 7 actual landing" 段填具体 outcomes(lab-gating + W2 gating + test posture summary)✅
  - `README.md` current-phase line 升级 Phase 6 → Phase 7 + lab-gating outcome explicit + 三 deferred 项 ⏳ 标 + Phase 8 next-phase pointer ✅
  - git tag `phase-7-complete` at this commit ✅

## Phase 7 status summary (15/15 statuses)

W1 = 8/8 ALL LANDED:
- T001 ADR-0011 · T002 doc-only · T003 schedulerName · T004 Source interface · T005 npu-smi scaffold · T006 NPUSliceTemplate CRD · T007 template engine · T008 HCCS adjacency

W2 = 6/7 LANDED + 1 lab-deferred:
- T101 deferred Phase 10(default policy)· T102 doc-only · T103+T104 combined · T105 partial(controller wiring deferred Phase 10)· T106 spike · T107(this)
- Phase 7 tag lands regardless of T101 per ADR-0011 §3 + ADR-0011 §後果 row 4

Gating outcomes:
- Lab access:no signal in W2 entry → default = defer T101 to Phase 10
- sched-plugins v0.32.x:GA confirmed but K8s 1.32 baseline pin can't absorb(known-issues #12)→ T002 doc-only
- vllm-ascend v0.12+:GA confirmed (v0.18.0 latest) but CI image-pull access unverified → T102 doc-only

## Key decisions

- **Checkpoint structure mirror Phase 6**:6 sections(deliverables + wired + test posture + DoD reconciliation + known issues + handoff)· operator + contributor switch from Phase 6 to Phase 7 review 时熟悉度 zero ramp
- **Phase 8 handoff §6 顺序**:1. K8s baseline bump (highest-leverage · unblocks 3 deferred items in 1 task)· 2. busy-idle vertical scaling controller(arch §13 candidate · 读 NPUSliceTemplate substrate)· 3. AllocateBundle controller wiring T105-v2 · 4. Partition-aware allocator(if KEP-4815 GA in Phase 8 window)
- **README current-phase narrative explicit about ⏳ deferred items**:Phase 6 README 也用同样模式 · 用户 / contributor 一眼看出 Phase 7 实际 ship 范围 + 哪些拖延到下一 phase
- **arch §13 row 升级保留 commit hash 范围**(bd6f223..bf0c03c):14 task chain · 让审阅者 grep commit history 一次性看完 Phase 7
- **Tag at T107 commit · 不在 push 之前**:本仓库 PR + approval 流程在实际操作中走 main agent commit + 用户 push(per memory `reference_github_creds.md`)· tag 落地后用户 push 时自然带上(git push --follow-tags 或 git push --tags)

## Verification

- 存在性:
  - `docs/checkpoint-phase7.md` 6 sections + Phase 8 handoff brief ✅
  - `docs/architecture.md` §13 评审追加 Phase 7 row 升级(commit hash 范围 + 3 gating outcomes + Phase 8 baseline-bump triangle 引用)✅
  - `docs/phase7-plan.md` "Phase 7 actual landing" 段填 outcomes(替换 _To be filled_ 占位)✅
  - `README.md` current-phase line 升级 + Phase 8 next-phase pointer ✅
  - `docs/devlog/phase-7-t107.md`(this file)✅
  - `git tag phase-7-complete` created at this commit(local · push 由用户)✅
- 完整性(verified by execution):
  - `git log --oneline | head -15` 显示 Phase 7 commit chain bd6f223..bf0c03c + T107(this commit)+ phase 7 plan + Phase 6 series ✅
  - `git tag -l 'phase-*'` shows 9 tags(0-baseline + 1-complete + w1/w2/w3 + 2-complete + 3-complete + 4-complete + 5-complete + 6-complete + 7-complete)✅
  - Phase 6 checkpoint section count(7 sections)与 Phase 7 checkpoint section count(6 sections · Phase 7 没有 W2 polish "Architecture §5.6 cross-ref" extra section)approximately equivalent · structure 一致
- 正确性(grep cross-ref):
  - checkpoint §6 handoff brief 列 9 个 tags(0-baseline ... 7-complete)与 `git tag -l 'phase-*'` 实际数一致
  - checkpoint §4 DoD reconciliation 含 15 个 task statuses(W1 8 + W2 7)+ gating outcomes 明示 · 与 phase7-plan §5 DoD 表格条目数一致
- 注释:**T107 不 push remote**(per CLAUDE.md §6 直接 push dev 禁忌 + sandbox 阻拦)· local commit + local tag · 用户决定何时 push 到 origin。reference_github_creds memory + agent-coordination §0a.12 push protocol 适用 · `git push --follow-tags origin dev` 由用户主动执行

## Carry-forward

- **Phase 8 entry session 必读**(已在 checkpoint §6 列):
  1. `docs/checkpoint-phase7.md`(current · 全面 Phase 7 状态 + Phase 8 handoff)
  2. `docs/research/k8s-partitionable-devices-spike.md`(P7-T-106 · KEP-4815 status + cost matrix)
  3. `docs/known-issues.md` #11(P7-T-003 closes)+ #12(P7-T-002 defers)
  4. `docs/devlog/phase-7-t102.md`(ProxyImage gating decision context)
  5. `docs/devlog/phase-7-t105.md`(controller wiring deferred · T105-v2 recipe)
- **Phase 8 W1 candidate "K8s baseline bump" 单 task triangle**:per checkpoint §6 第 1 项 + phase-7-t106 spike §6 + known-issues #12 推荐路径 · 同 1-2d 任务 unblock(a) NumaAffinity wrap (b) ProxyImage chart flip (c) Partitionable Devices Beta enable
- **git push 由用户主动执行**:per CLAUDE.md §6 PR + approval 流程 · 此 commit 落地后用户 `git push --follow-tags origin dev` ship 全部 phase-7 commits + tag 到 origin。memory reference_github_creds.md 含 Windows cmgr 已 cached creds · 直接 push 通无需 token 操作
- **Phase 7 → Phase 8 plan 起草由新 session 完成**(per memory feedback_plan_vs_execute_session_split.md):Phase 8 plan 写作不在本 execute session 范围 · 用户 / 新 plan session 起草 phase8-plan.md 后再起 Phase 8 T001 execute session
