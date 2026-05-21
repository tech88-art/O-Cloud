# P10-T-202 · [DECISION-GATED] Partitionable Devices Beta + partition-aware allocator → deferred Phase 11+(KEP-4815 仍 Beta · 1.34 baseline 不 unlock · doc-only refresh)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 0d(KEP-4815 GA 条件不 met · doc-only)

## Outcome

Per Phase 10 plan §3 P10-T-202 decision branches:
- T003 bumped 1.36 AND KEP-4815 GA per re-WebFetch → land
- T003 bumped 1.34/1.35 only OR KEP-4815 still Beta per re-WebFetch → defer Phase 11+

**实际 T003 outcome**(P10-T-003 commit `8d70efa`):baseline bumped to **K8s 1.34**(option B 务实)· **不是 1.36**。
**KEP-4815 status**(WebFetch 2026-05-21 at T002 entry):**Beta in K8s 1.36**(per `docs/adr/0016-lab-onboarding-and-phase-11-outlook.md` §3 Stream 6)· GA not targeted yet · `DRAPartitionableDevices` feature gate Beta only。

→ **两 decision branch 都 trigger defer Phase 11+** · doc-only refresh outcome。

## ADR-0009 §4 path A migration carry

Phase 10 plan §3 P10-T-202 acceptance · doc-only refresh:`docs/adr/0009-npu-dra-driver.md` §4 partition-aware allocator section status 留 carry · ADR-0016 §3 Stream 6 已 codify Phase 11+ wait trigger(K8s 1.36+ baseline land AND KEP-4815 GA per re-WebFetch)。

实际 ADR-0009 update 推到 T204 checkpoint batch · 与其他 deferred outcomes(T102 5th carry · T108 2nd defer)一起 update。

## Reasoning

- KEP-4815 Beta in 1.36 是 *Beta*(`DRAPartitionableDevices` 在 1.36 默认 OFF · feature gate 必须显式 enable + 仍可能 break compat in 1.37)
- 现 1.34 baseline 是 sched-plugins v0.34.7 lockstep 的唯一 feasible target(per P10-T-003 option B)
- 1.35 / 1.36 bump cascade:sched-plugins v0.35+/v0.36+ 未发布 + kindest/node v1.36 image 缺
- T202 路径 A migration 即使 1.36 bump 也需要 KEP-4815 GA 才有 production confidence · Beta 在 K8s 一般 1-2 minor 后 GA

## Phase 11+ re-eval triggers

任意 phase 中期 chat 用户 signal "KEP-4815 GA confirmed + K8s 1.36+ bump ready" · 或 sched-plugins v0.35+/v0.36+ 发布(reopen baseline bump decision)→ ad-hoc T202 reopen per ADR-0016 §3 Stream 6 trigger。
