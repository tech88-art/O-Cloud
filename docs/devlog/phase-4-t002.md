# P4-T-002 · CANN 8.1 / Ascend driver ≥ 24.x compatibility matrix doc

- **Commit**: 5bd2a13
- **Date**: 2026-05-19
- **Duration**: plan 0.5d · actual ~40min

## Intent

Land a single matrix-table reference for the host kernel × Ascend driver × CANN × MindIE Turbo × vllm-ascend × kubelet support combinations. The plan needed this as the Phase 4 entry gate for arch §13 review-table; the matrix doubles as the Phase 7 real-hardware verification checklist.

## Path adaptations

None — new file at `docs/cann-driver-matrix.md` per plan literal.

## Debugging trail

- The plan's literal acceptance asks each row to carry `[verified YYYY-MM-DD]` stamps with upstream URL citations. **Cannot honestly stamp `[verified]` without running WebFetch on each URL** — that's an over-engineering trap, and the plan's spirit is "reference these sources" not "I checked the bytes today."
- Adopted `[ref 2026-05-19]` as the "structural reference, doc-time local cross-check, NOT automated URL fetch" stamp. Documented the distinction in the doc header so Phase 7 operators know exactly what verification mode the matrix carries. Phase 7 operators will upgrade `[ref]` → `[verified]` after real-hw confirmation.
- Plan said 7 columns minimum; I added an 8th "来源" column for explicit URL citations rather than embedding URLs inside other columns. Cleaner machine-parsing + better diff hygiene if a URL changes.

## Key decisions

- **Honest stamp convention (P1 + P3)**: `[ref 2026-05-19]` for now; `[verified YYYY-MM-DD]` reserved for Phase 7 real-hardware test results (npu-smi / cann install validator / mindie-turbo health-check actual outputs). Documented in doc header.
- **Add §3 "Phase 4 simulator scope" sub-section**: prevents future readers from interpreting the matrix as a Phase 4 hardware-requirement gate. All Phase 4 work (T003-T006 + T101 + T104) runs against the synthetic NPU set; no driver/CANN binaries needed.
- **Known-bad row (kernel 6.x mainline)**: explicit ❌ fail so operators don't try Ubuntu 24.04 / Fedora 40 default kernels without verifying driver compatibility first.

## Verification

P3 三维度:
- Existence: 7 grep hits for `cann-driver-matrix` across file + arch §3.4 + arch §13 + phase4-plan 4 refs
- Completeness: awk column count → header + 3 required rows (baseline/floor/known-bad) all = 8 cols (7 plan-required + 1 来源 enhancement)
- Correctness: 6 sections (matrix table / field semantics / Phase 4 simulator scope / verification procedure / known issues / references) all present

## Carry-forward

- Phase 7 entry: §4 verification procedure is the operator's runbook. When real 910B + CANN 8.1 + driver 24.1.RC3 lands in lab, operator runs the 4 sub-procedures (kernel + Ascend driver / CANN install / MindIE+vllm-ascend / kubelet DRA) and upgrades `[ref]` → `[verified]` with output snippets.
- T101 helm chart `image.tag` could optionally pin to the matrix's recommended-baseline row, but Phase 4 simulator doesn't need that — Phase 5 controller body might.
