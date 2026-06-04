# P13-fix-004 · doc-gap backfill (CLAUDE.md §12 + 2 module DESIGN.md)

- **Commit**: this commit (main agent · post-closer doc-completeness backfill)
- **Date**: 2026-06-03

## Scope

Closer-time documentation audit found 3 hard gaps (CLAUDE.md §14.2 + guide currency):

1. **root `CLAUDE.md` §12「当前 Phase」stale** — still said "Phase 0 已完成 / Phase 1 即将
   启动" (13 phases behind). Refreshed to "项目已收官 · Phase 13 complete · M7 final · 不规划
   Phase 14" + full phase ledger + carry/optional list. (README §当前阶段 + architecture
   §1.3/§13 were already current; only §12 was missed — never in any task's Allowed Paths.)
2. **`operators/pool-operator/DESIGN.md` missing** — core IMS operator (4 CRD + 4 reconciler)
   had only a kubebuilder `// TODO(user)` boilerplate README. Wrote a grounded DESIGN.md from
   the actual reconcilers (selector→aggregate, Strategy→totalSlices `floor(32/aiCore)×npuCount`,
   HCCS topology aggregation, finalizer, ResourceSlice cross-watch contract per ADR-0010 §5).
3. **`exporters/ascend-npu-exporter-plus/DESIGN.md` missing** — wrote from the actual source
   selector seam (`sources.Select`), the scrape-time Collector pattern, the no-cgo DCMI→npu-smi
   exec body (ADR-0024 §4(c)), and the exact metric contract.

Result: DESIGN.md now present for **all 9 operator/exporter modules**.

## Finding surfaced while documenting (flagged, NOT fixed here — out of doc scope)

- **🔴 NodePool arm64 exclusion is stale vs ADR-0020.** `nodepool_controller.go` filters out
  `kubernetes.io/arch=arm64` nodes citing the *original* "amd64-only" assumption. ADR-0020
  (Phase 12) flipped the target platform to **aarch64 鲲鹏 920** → on a real Kunpeng cluster
  NodePool would exclude every node (status.nodes empty, totalCPU/Mem zero). NPUPool /
  NPUSlicePool are arch-agnostic (aggregate by `Ascend910B` capacity) so the real **NPU**
  path is unaffected — but NodePool CPU/Mem rollup is broken on the actual target. Recorded
  in `operators/pool-operator/DESIGN.md` §5 + flagged as a separate task (needs code + test + CI,
  out of this docs change).

## Verify

- factual citations grep-checked against code: `SetCondition`∈utils.go ✓ · `NPUSMIConfig`/
  `NewNPUSMISource`/`ErrNoCommand`∈npu_smi.go ✓ · metric names `exporter_collect_duration_seconds`
  / `ascend_slice_aicore_count|memory_used_bytes|allocated_to_pod` exact ✓ (corrected 2 drafted
  names) · referenced phase plans exist ✓.
- docs-only · `git status` = root CLAUDE.md (M) + 2 new DESIGN.md + this devlog.
