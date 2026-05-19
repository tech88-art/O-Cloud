# Development log (`docs/devlog/`)

> Per-task development journal capturing **the trail commit messages don't carry** — false starts, error chases, design-decision rationale, and pitfalls discovered along the way. Purpose: a future debugger (human or AI) can reconstruct **why** something was done, not just what.

## 1. Convention

### 1.1 When to write

- **Per task**: every plan task package (`PN-T-NNN`) gets one devlog file at commit time. Tag it via the commit footer:
  ```
  Devlog: docs/devlog/phase-N-tNNN.md
  ```
- **Retroactive** (allowed once at end of phase): if a phase landed without per-task devlog discipline, write back-fill files at checkpoint time using the commit history + reconstructed debugging memory. Phase 4 is the first retroactive batch (this README arrived after `phase-4-complete` tag).

### 1.2 File naming

```
docs/devlog/phase-<phase>-t<task-id>.md
```

Examples:
- `docs/devlog/phase-4-t001.md`
- `docs/devlog/phase-4-t101.md`
- `docs/devlog/phase-5-t001.md` (future)

### 1.3 File template

```markdown
# P<phase>-T-<id> · <one-line goal>

- **Commit**: <sha>
- **Date**: YYYY-MM-DD
- **Duration**: <plan-estimated d/h vs actual>

## Intent
(One paragraph — what the task was supposed to achieve, in plain words. Not duplicating commit message.)

## Path adaptations (if any)
(Plan literal vs codebase reality — paths/versions/conventions that drifted, and the resolution.)

## Debugging trail
(Bullet list of false starts + errors encountered + how each was resolved. This is the highest-value section for future debugging.)

## Key decisions
(Choices made and why — link to relevant ADR / memory rule if applicable.)

## Verification
(What proved the task done — commands run, outputs cited, P3 三项验证维度.)

## Carry-forward
(What downstream tasks need to know / inherit / be careful about.)
```

### 1.4 Length

Target **20-80 lines per file**. Devlog is NOT a re-statement of the commit message — focus on the trail and rationale. If a task ran perfectly with no surprises, a 15-line file is fine (and itself useful: "no issues" tells future debugger this surface is stable).

## 2. Going-forward enforcement

Root `CLAUDE.md` §15 (added 2026-05-19, post phase-4-complete) enforces:

1. Every task commit message ends with `Devlog: docs/devlog/phase-<N>-t<NNN>.md`.
2. The referenced devlog file must exist in the same commit (not a later cleanup pass).
3. Checkpoint commits (`PN-T-107` / `PN-T-XXX`) verify devlog coverage = task count (15/15 for Phase 4 etc.).

CI hook (Phase 5+ candidate, not blocking yet): `.github/workflows/devlog-coverage.yml` could grep commit footers + check file presence.

## 3. Phase 4 retroactive batch

Phase 4 landed 15 tasks (T001-T008 W1 + T101-T107 W2). All 15 devlog files arrived in a single back-fill commit after the `phase-4-complete` tag:

| Task    | File                                       | Highlight |
| ------- | ------------------------------------------ | --------- |
| T001    | [phase-4-t001.md](phase-4-t001.md)         | ADR-0001 v3 — pure docs, clean run |
| T002    | [phase-4-t002.md](phase-4-t002.md)         | `[ref YYYY-MM-DD]` vs `[verified YYYY-MM-DD]` honesty choice |
| T003    | [phase-4-t003.md](phase-4-t003.md)         | Plan stale go.mod versions vs pool-operator actual — followed pool-operator |
| T004    | [phase-4-t004.md](phase-4-t004.md)         | PROJECT no-CRD resource entry — `crdVersion` omitted convention |
| T005    | [phase-4-t005.md](phase-4-t005.md)         | NodeName *string→string compile fail; fake client over envtest; CRLF/LF blank-line in npus.json edit |
| T006    | [phase-4-t006.md](phase-4-t006.md)         | v1beta1 ResourceClaimStatus has no Conditions field → annotations pivot |
| T007    | [phase-4-t007.md](phase-4-t007.md)         | Plan `internal/server/` path doesn't exist → pkg/api adaptation |
| T008    | [phase-4-t008.md](phase-4-t008.md)         | Dispatch counter label "endpoint" semantic (resource key, not HTTP path) |
| T101    | [phase-4-t101.md](phase-4-t101.md)         | `.Files.Get` chart-time vs runtime confusion + files/ directory copy |
| T102    | [phase-4-t102.md](phase-4-t102.md)         | controller-gen v0.20.1 vs v0.21.0 cosmetic-only CRD diff revert |
| T103    | [phase-4-t103.md](phase-4-t103.md)         | `*ModelService does not implement Object` until controller-gen ran |
| T104    | [phase-4-t104.md](phase-4-t104.md)         | Plan `scripts/install.sh` vs actual `tests/e2e/kind/install.sh` dual-track |
| T105    | [phase-4-t105.md](phase-4-t105.md)         | ADR-0009 — pure docs, design discovery worth recording |
| T106    | [phase-4-t106.md](phase-4-t106.md)         | Counter family registered but emits no lines without observed labelset — warm-up step needed |
| T107    | [phase-4-t107.md](phase-4-t107.md)         | Checkpoint + tag — clean run |

## 4. Related docs

- `docs/checkpoint-phase4.md` — Phase 4 retrospective + handoff brief (the "what" — devlog is the "why")
- `docs/phase4-plan.md` — Phase 4 plan (the "should")
- `docs/adr/0001-phase0-key-decisions.md` v3 — primary architectural decisions
- `docs/adr/0009-npu-dra-driver.md` — npu-dra-driver semantic design (Phase 5 entry reading)
- Module DESIGN.md (Phase 4 new):
  - `operators/npu-dra-driver/DESIGN.md`
  - `operators/inference-operator/DESIGN.md`
  - `backend/docs/observability.md` (Phase 4 self-metrics design)
