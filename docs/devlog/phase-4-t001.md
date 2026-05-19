# P4-T-001 · ADR-0001 v3 dual-path roadmap

- **Commit**: f5c21f7
- **Date**: 2026-05-19
- **Duration**: plan 0.5d · actual ~30min

## Intent

Update the master architectural-decision record (ADR-0001 §5) for the post-phase-3 reality: KubeEdge ≤ v1.22 has no DRA support, no official Ascend DRA driver exists, and the Phase 4 npu-dra-driver scaffold is the standard-K8s spike vehicle. Make the dual-path stance (Edge: Device Plugin v1; Standard-K8s: optional DRA) explicit so Phase 5 doesn't have to re-litigate it.

## Path adaptations

None — pure docs.

## Debugging trail

- Initially read §5 v1 + v2 and discovered v2 already records all 6 facts the plan demands. Spent 5 min understanding what v3 actually adds (vs being a verbatim re-statement): the **explicit dual-path formatting** (Edge vs Standard-K8s as separate bullets) plus **Phase 4 scaffold landing references** as concrete deliverables, not future tense.
- §7 (DRA driver scaffold plan) currently says "Phase 4 / Phase 7 范围" — needed a status table mapping each Phase 4 deliverable to its task ID (T003 scaffold / T004 types / T005 publisher / T006 claim controller / T101 chart / T105 design).
- architecture.md §13 review-table 2026-05-17 row was the right insertion point for Phase 4 row-update (not a new row).

## Key decisions

- **Preserve v1 + v2 verbatim**: ADR audit trail principle (per CLAUDE.md global P1 — historical record is sacred even when superseded). Appended v3 as a third sub-section.
- **Cite specific upstream versions**: K8s 1.34 (2025-09-01) + 1.36 (2026-05-07) + KubeEdge v1.22 (2026-04-12) + dra-example-driver v0.2.1 (2026-01-09). Per P1 numbers-need-source.
- **Phase 7 forward note kept short**: 2 lines on Partitionable Devices (KEP-4815) est. K8s 1.37 GA. Not the place for full Phase 7 design — that's ADR-0009 §4 (T105).

## Verification

P3 三维度:
- Existence: `grep "ADR-0001 v3" docs/architecture.md` → 2 hits (§3.4 line 221, §13 line 792)
- Completeness: `grep markers in ADR-0001` → 5 hits (v3 修订 / 双轨路径 / scaffold 落地依据 / v3 与 v2 区别 / §7 状态表)
- Correctness: read §5 v3 line-by-line against plan's 6 required facts — all present

## Carry-forward

- T003 scaffold needs to link back to ADR-0001 v3 §7 in operators/npu-dra-driver/README.md (done at T003).
- T002 cann-driver-matrix should cross-ref ADR-0001 v3 §5 in matrix doc header (done at T002).
- T105 ADR-0009 expands the slice ↔ ResourceClaim mapping that v3 only mentions in passing.
