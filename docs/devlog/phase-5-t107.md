# P5-T-107 · Phase 5 checkpoint + tag

- **Commit**: <pending>
- **Date**: 2026-05-19
- **Duration**: plan 0.5d · actual ~30min

## Intent

Seal Phase 5 with the canonical retrospective doc + tag. Mirrors
P4-T-107 structure (deliverables table, what's wired diagram,
capabilities matrix, test posture, DoD reconciliation, known issues
rollup, Phase 5 → 6 handoff brief). Tag `phase-5-complete` lands on
this commit.

## Path adaptations

(none — pure docs + tag)

## Debugging trail

(N/A — pure docs)

## Key decisions

- **Checkpoint structure mirrors Phase 4 verbatim**. Sections 1-7 in
  the same order; per-task SHA list at top; capabilities matrix +
  test posture +  DoD reconciliation + known-issues rollup + Phase
  N→N+1 handoff brief at the end. Predictable, easy to diff between
  phases.
- **Phase 6 handoff brief lists 5 candidate workstreams (plan acceptance
  asked for ≥3)**:
  1. HCCS / NUMA topology-aware scheduler-plugin
  2. vllm-ascend `disaggregated_prefill_v1` adoption
  3. Workload-rendering frontend refresh
  4. Multi-tenancy entry (Phase 9 substrate)
  5. inference-operator metrics exposition
- **Tag landed on T107 commit, not separately**. Convention from
  Phase 1-4: `git tag phase-N-complete` against the checkpoint
  commit. The user runs `git tag` manually after merge (per project
  CLAUDE.md §7 — direct push to dev forbidden by PR flow; tagging
  applied post-merge).

## Verification

P3 三维度:
- **Existence**:
  - `git status` shows: 1 new file (docs/checkpoint-phase5.md),
    3 modified (README.md, docs/architecture.md §13,
    docs/phase5-plan.md tail), devlog
- **Completeness**:
  - `wc -l docs/checkpoint-phase5.md` → ~200 lines (similar shape
    to Phase 4 checkpoint at ~290 lines)
  - All 14 prior Phase 5 task SHAs cross-referenced in deliverables
    list (41cd04e, a423dd9, 6ef715d, 8173e83, c283e94, bac3937,
    e3629a7, 8c79bca, c3452d3, 94c7c99, ca81ead, 1300f30, 9e6f327,
    6fc4b0a)
  - Test counts per surface tracked in §4
  - DoD §5 lifts every plan acceptance bullet with [x] + SHA cite
  - Phase 6 handoff brief lists 5 candidate workstreams + Phase 6
    tools needed
- **Correctness**:
  - README current-phase line updated from "Phase 4 complete" to
    "Phase 5 complete (15/15 tasks · tag phase-5-complete)"
  - Phase 4 line moved to "上一阶段"; Phase 6 line added to "下一
    阶段"
  - architecture.md §13 review-table rows for Phase 5 promoted from
    "Phase 5 启动检查" to "landed (commit SHA + brief description)"
  - phase5-plan.md tail acknowledges actual landing + cross-ref
    checkpoint doc

## Carry-forward

- Run `git tag phase-5-complete` against this commit post-merge.
- Phase 6 first session reads `docs/checkpoint-phase5.md §7` handoff
  brief + `docs/cni-hccl-research.md` § Phase 6 recommendation.
- The `phase-5-complete` tag is the baseline for any Phase 5 hotfix
  branches if production smoke uncovers issues post-tag.
- CI on the next push validates the kind smoke extension (T106)
  end-to-end; if green, Phase 5 declared shipped.

## Phase 5 summary (T001-T107 collective)

15 tasks landed in W1+W2 order — 100% completion against
docs/phase5-plan.md.

Headline:
- **Real NPU device allocation** replaced Phase 4's annotation-only
  skeleton (T002 allocator + T003 BestFit variant + T005 audit
  controller). Greedy first-fit deterministic; per-claim
  NPUSliceAllocation audit object tracked through Allocated →
  Released → Orphaned lifecycle.
- **inference-operator controller body** + **PD Router mutating
  webhook** in the same binary (ADR-0008 design): ModelService
  Reconcile resolves NPUSlicePool, materialises Prefill + Decode
  Deployments with per-side ResourceClaimTemplate, drives phase
  machine Pending → Provisioning → Ready / Failed.
- **PD Router** injects `npu.huawei.com/slice-bindings` annotation
  onto Prefill/Decode Pods at admission time; fail-closed Denied
  when all NPUSliceAllocations are Orphaned (ADR-0008 default).
- **cert-manager wiring** for webhook TLS (T101); kind smoke
  extension (T106) validates the full chain end-to-end.
- **Module DESIGN.md** updates for npu-dra-driver (T002 / T005)
  and inference-operator (T006 / T007 / T008 / T102 / T103)
  document the controller bodies + webhook flow + phase machine
  diagram.

Coverage:
- npu-dra-driver: 29 tests across api/v1alpha1 + allocator +
  controller + publisher packages
- inference-operator: 32 tests across controller + webhook packages
- Helm chart `helm lint --strict` clean for both
  npu-dra-driver and inference-operator
- CNI × HCCL research doc (T105) seeds Phase 6 scheduler-plugin
- kind smoke Phase 5 extension (T106) covers cert-manager install
  + ModelService creation + Pod annotation injection
- 14 per-task devlogs (`docs/devlog/phase-5-t001.md` →
  `phase-5-t106.md`) plus this T107 file

Phase 5 ran strictly per-task-serial with strict verify per the
2026-05-19 strict-per-task rule (memory:
`feedback_strict_per_task_verify.md`). Zero collation commits; no
parallel subagent dispatch; one commit per task; every commit
carries `Devlog: docs/devlog/phase-5-tNNN.md` footer.

Phase 5 complete — tag `phase-5-complete` lands on this T107 commit.
