# P10-T-108 · [DECISION-GATED · 2nd defer] Volcano gang-scheduling → deferred Phase 11+(default policy · no training-job demo signal)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 0d(default policy 命中 · doc-only)

## Outcome

Per Phase 9 P9-T-101 default policy continued + Phase 10 plan §3 P10-T-108 acceptance:用户 Phase 10 W2 entry chat 未明示 "Phase 10 training-job demo 需要 gang" · default 命中 · T108 走 **2nd defer Phase 11+** 路径。

**2-phase carry tally**:
- Phase 9 P9-T-101(1st carry · 2026-05-21 @ 07bcf1a · doc-only deferred per default policy)
- Phase 10 P10-T-108(2nd defer · 2026-05-21 · default policy)

## Reasoning

- Phase 10 主线 1+2+3 全 inference-focused(IMS 3 services + 真硬件对接 + multi-pool 演示) · 无 training-job demo deliverable
- Volcano binary install + PodGroup atomicity + `schedulerName=volcano` opt-in 是 1-2d 工作量(per `docs/research/volcano-gang-scheduling-spike.md` §4 路径 A)· Phase 11+ if training-job demo materialise

## ADR-0010 §7 + status 留待 T204 checkpoint batch update

Phase 10 T108 deferred outcome 与其他 DECISION-GATED tasks(T202 KEP-4815)+ LAB-CONDITIONAL(T102 5th carry)一起 T204 checkpoint phase-10-complete tag 时 ADR-0010 §7 + ADR-0009 §4 + ADR-0011 §3 batch carry tally update。

## Phase 11+ re-eval triggers

任意 phase 中期 chat 用户 signal "Phase 11+ training-job demo" → trigger ad-hoc re-eval(per ADR-0010 §7 + ADR-0016 §3 Stream 7 if Volcano gang-scheduling 加 candidate list)。
