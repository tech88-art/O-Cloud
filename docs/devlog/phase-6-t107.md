# P6-T-107 · Phase 6 checkpoint + tag

- **Commit**: <pending>
- **Date**: 2026-05-20
- **Duration**: plan 0.5d · actual ~30min

## Intent

Seal Phase 6 with a `phase-6-complete` git tag + comprehensive
checkpoint doc covering 13 landed tasks + 2 deferred with documented
forward path. Update architecture.md §13 + README current-phase line
+ phase6-plan trailing "actual landing" block per the Phase 5
precedent.

## Path adaptations

- **13/15 instead of 15/15**: T006 (NumaAffinity upstream wrap)
  and T102/T103 (backend+frontend workloads sliceBindings)
  intentionally deferred with documented forward path. Phase 5
  closed 15/15; Phase 6 carries the two deferrals into a "Phase 6
  polish" follow-up window. Per the project's deferral pattern
  (T006 ADR-0010 §3 update; T002 P3 disclosure on envtest), 13/15
  is acceptable at the phase tag with the deferrals visibly
  carried in checkpoint §1 + §6.

- **README current-phase pointed at Phase 6 / Phase 7**: matches the
  Phase 5 → Phase 6 pattern. Phase 4 details moved to "早期阶段"
  line.

- **architecture.md §13 review-table Phase 6 row promoted**: from
  "in flight" (set at T001) to "landed phase-6-complete" with all
  task SHAs referenced + carry-forward items called out.

- **phase6-plan.md trailing "actual landing" block**: matches Phase
  5 precedent. Documents what landed + what deferred + test posture
  + checkpoint cross-ref.

## Debugging trail

(N/A — pure docs)

## Key decisions

- **Tag the deferrals openly, don't hide them**: T006 NUMA placeholder
  + T102/T103 workloads sliceBindings are real Phase 6 plan items
  that don't ship. Checkpoint §1 marks them ⏳ with reason; §5 DoD
  reconciliation flags them in the W2 list with the deferral rationale
  inline. Future agents reading the checkpoint see exactly where to
  pick up.

- **Phase 6 = M3 milestone substrate**: this is the architectural
  story. HCCS-aware scheduling is the M3 differentiator per
  architecture.md §1.3 Phase roadmap. Phase 6 ships the substrate
  (plugin + chart + metrics + schema substrate); Phase 7 hardens
  it with real-hardware integration.

- **Phase 7 handoff focuses on the 6 candidate workstreams**: real
  hardware integration, NPU 动态切分, NumaAffinity wrap upgrade,
  HCCS placement assertion tightening, deployment_builder
  schedulerName injection, Standard-K8s 1.34+ DRA spike. The
  T102/T103 carry-over + vllm-ascend v0.12+ assessment are noted
  separately under "Carry-over from Phase 6".

- **Test posture matrix split by sub-project**: 42 scheduler-plugin
  + 40 inference-operator sub-tests = 82 total Phase 6 tests
  passing locally. Plus pool-operator T003 envtest cases deferred
  to CI per Windows UAC limitation (compiles clean; runtime CI
  validates).

## Verification

P3 三维度:

- **Existence**:
  - `docs/checkpoint-phase6.md` — new file with 7 sections (overview,
    what's wired, capabilities matrix, test posture, DoD, known
    issues, Phase 7 handoff)
  - `docs/architecture.md` §13 — Phase 6 row promoted to "landed
    phase-6-complete"
  - `README.md` line 5 — current-phase line points at Phase 6
  - `docs/phase6-plan.md` — trailing "actual landing" block added
  - `docs/devlog/phase-6-t107.md` — this file

- **Completeness** (plan §4 P6-T-107 acceptance):
  - All 13 landed tasks have a row in checkpoint §1 with commit
    SHA ✅
  - 2 deferred tasks (T006, T102/T103) explicitly marked ⏳ with
    rationale ✅
  - Phase 7 seed has 6+ candidate workstreams + carry-over from
    Phase 6 + tools needed ✅ (checkpoint §7)
  - README.md current-phase points at phase-6-complete ✅
  - Tag `phase-6-complete` lands on this commit (next bash run
    via the workflow's release path) ⏳ to be applied after
    checkpoint commit

- **Correctness**:
  - All commit SHAs in checkpoint §1 verified against `git log`
    output
  - Capability matrix entries cross-reference the actual file
    paths in the repo
  - Test posture table reflects actual passing test counts (verified
    on local `go test` runs per task devlogs)
  - DoD checkboxes (W1, W2, Out of scope) align with plan §5

## Carry-forward

- **Tag application**: after this commit lands on dev, run:
    `git tag phase-6-complete <commit-sha>`
    `git push origin phase-6-complete`
  Per Phase 5 precedent the tag goes on the checkpoint commit.

- **T102/T103 follow-up**: when user approves the api-contract.yaml
  RFC for `sliceBindings[]` field, a Phase 6 polish task wires the
  backend extension + frontend page. May ship as a sub-tag
  `phase-6.1` or fold into Phase 7 W2.

- **T006 follow-up**: monitor sched-plugins releases. When v0.32.x
  ships with K8s 1.32 compat, run a "NumaAffinity wrap upgrade"
  task (3 file edits per devlog phase-6-t006.md §carry-forward).

- **Phase 7 entry**: depends on real-cluster lab access (Ascend
  910B silicon + CANN 8.1 + Mellanox CX-6+ NICs). Plan drafted
  in a separate plan-only session per CLAUDE.md §0a.10 plan/
  execute split convention.
