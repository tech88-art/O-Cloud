# P4-T-107 · Phase 4 checkpoint + tag phase-4-complete

- **Commit**: 803b429
- **Date**: 2026-05-19
- **Duration**: plan 0.5d · actual ~45min

## Intent

Land the Phase 4 retrospective at `docs/checkpoint-phase4.md` covering deliverables (15/15) + capabilities matrix + DoD reconciliation + handoff brief, append a Phase 4 demo appendix to `docs/demo.md`, bump `README.md` "current phase" label, and create the `phase-4-complete` git tag.

## Path adaptations

None — pure docs + git.

## Debugging trail

- Used `checkpoint-phase3.md` as the structural template — same 7 sections in the same order. The capabilities matrix table (§3) was the longest single piece — 30 capabilities × source paths × originating task IDs.
- The Phase 4 → Phase 5 handoff brief (§7) names 5 candidate scope items in the order Phase 5 should tackle them (allocation logic + NPUSliceAllocation CRD + inference-operator controller body + PD Router webhook impl + NPU pod network research). Recommends starting with PD Router webhook scaffold alongside inference-operator controller package init since they live in the same binary per ADR-0008.
- Demo appendix (`docs/demo.md`) reused the Phase 3 appendix structure — bring-up commands + 3 verification checks + limitations + teardown.
- `git tag phase-4-complete` lands directly on the checkpoint commit (single-author dev branch — this IS the merge commit per plan literal).

## Key decisions

- **DoD reconciliation explicitly cites commit SHA per item**. Reader can `git show <sha>` to verify each Must Have / Should Have bullet was actually shipped. Pure-docs Must Have items (ADR-0001 v3, CANN matrix) cite the docs commit; code items (scaffold, publisher, claim controller) cite the code commit.
- **Test posture table is brutally honest about envtest gaps**. Marked the pool-operator T102 envtest cases with ⚠️ ("envtest binaries unavailable in Windows shell") + the kind live runs with ⚠️ (deferred to CI). Future debuggers know what was actually run locally vs deferred.
- **Known issues §6 documents zero new issues**. Phase 4 surfaced zero new known-issues entries during W2 integration. The Phase 3 inheritance (#7 helm-lint / #8 real-cluster E2E / #9 dev stub) all resolved at phase-3-complete. Explicit empty-state notation prevents future readers from thinking "the section is missing".
- **Schema-drift items documented inline, not promoted to known-issues**. The v1beta1 ResourceClaimStatus no-Conditions field + the no-cross-sub-project-imports convention both live in code comments + commit footers. The checkpoint §6 footnotes them but doesn't escalate to known-issues — they're design decisions, not bugs.

## Verification

P3 三维度:
- Existence: `git log --oneline -16` confirms 14 task commits + 1 plan commit + 1 operating-rule commit, all SHAs cross-referenced in checkpoint §1 table
- Completeness: `wc -l docs/checkpoint-phase4.md` → 294 lines covering all 7 sections per phase3 template
- Correctness: `grep -c "P4-T-" docs/checkpoint-phase4.md` → all 15 task IDs present

## Carry-forward

- `phase-4-complete` tag is the entry-point for Phase 5. `git checkout phase-4-complete` reproduces the exact state Phase 5 starts from.
- Phase 5 plan will live at `docs/phase5-plan.md` — should reference checkpoint-phase4 §7 handoff brief as the seed.
- README "current phase" / "下一阶段" labels updated; future phase entry sessions update these via `sed` or simple Edit.
- This devlog file (phase-4-t107.md) is the LAST Phase 4 artifact — Phase 4 closed.
