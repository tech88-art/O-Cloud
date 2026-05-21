# P7-T-002 · NumaAffinity upgrade attempted → doc-only fallback (Phase 8 re-attempt)

- **Commit**: (this commit)
- **Date**: 2026-05-20
- **Duration**: plan 0.5d (upgrade) / 0.25d (doc-only) · actual ~0.5d (attempted upgrade · reverted · wrote doc-only)

## Intent

Phase 6 T006 shipped NumaAffinity as a Name()-only placeholder because upstream `sigs.k8s.io/scheduler-plugins/pkg/noderesourcetopology` v0.31.8 referenced K8s 1.31's `framework.GVK` symbol which K8s 1.32 removed. Phase 7 T002 gating decision: re-check if sched-plugins v0.32.x is GA → if YES flip placeholder to wrap upstream; if NO doc-only fallback.

## Gating decision outcome

**v0.32.7 IS GA (since 2024-08-06)** · also v0.33.5 (2024-10-27) and v0.34.7 (2025-04-20 latest) — verified via `go list -m -versions sigs.k8s.io/scheduler-plugins`. But attempting the upgrade in our K8s 1.32 baseline-pinned env hits a transitive dep brick wall. **Take doc-only fallback**;Phase 8 candidate re-attempt requires K8s baseline bump first.

## Path adaptations

Upgrade path attempt (reverted clean):
1. `go get sigs.k8s.io/scheduler-plugins@v0.32.7` succeeded — bumped all k8s.io/* indirect deps from v0.31.8 / v0.32.0 mix → uniform v0.32.7
2. Bumped `go.mod` replace block uniformly v0.32.0 → v0.32.7 (Edit replace_all on " v0.32.0\n" pattern)
3. `go mod tidy` then surfaced:
   ```
   k8s.io/kubernetes/cmd/kube-scheduler/app imports
       k8s.io/kubernetes/pkg/scheduler imports
       k8s.io/kubernetes/pkg/apis/core/validation imports
       k8s.io/kubernetes/pkg/apis/core/v1 imports
       k8s.io/apimachinery/pkg/api/{safe,operation,validate}: module k8s.io/apimachinery@latest found (v0.36.1, replaced by k8s.io/apimachinery@v0.32.7), but does not contain package ...
   ```
4. These `pkg/api/{safe,operation,validate}` packages are POST-v0.32 additions to apimachinery (v1.33+ era) but the resolved transitive expectation reaches them — likely via a sched-plugins or k8s.io/kubernetes leaf-package import that go's import graph follows. Our K8s 1.32 baseline pin cannot absorb this without baseline bump
5. Revert clean: `git checkout go.mod go.sum` + delete attempted args.go + restore plugin.go/plugin_test.go to Phase 6 state

## Debugging trail

- **First confusion**: WebSearch initially returned "latest v0.30.6 / v0.29.7" — looked like v0.32 wasn't GA. Then a second WebSearch result included "documentation dated April 18, 2026 (from the installation guide) mentions release v0.34.7". Conflicting — resolved by `go list -m -versions sigs.k8s.io/scheduler-plugins` which authoritative listed v0.32.7 / v0.33.5 / v0.34.7 all GA
- **Second confusion**: `grep -rl "apimachinery/pkg/api/safe" $GOMODCACHE/k8s.io/kubernetes@v1.32.7/` returned 0 matches — suggesting v1.32.7 doesn't import these packages. But `go mod tidy` clearly errored saying it follows the import chain through them. The Go MVS resolver may be aggregating cross-cutting requirements from sched-plugins v0.32.7's own go.mod that doesn't show in grep
- **Third confusion**: thought maybe just re-bumping replace block from v0.32.0 → v0.32.7 would fix. It DID address the original "v0.32.0 lacks foo" error but exposed the deeper "v0.32.7 still lacks pkg/api/safe" — confirming the issue is k8s minor (1.32 vs 1.33+) not patch (0.32.0 vs 0.32.7)

## Key decisions

- **Doc-only fallback chosen** instead of forcing the upgrade through:
  - Per CLAUDE.md M4 "价值聚焦 > 工作量驱动": upgrading would require K8s baseline bump (Phase 8 scope · 触发 kind smoke / kubeconfig / dep audit / known-issues #6/7 reverification) for a Phase 7 secondary feature (NumaAffinity Scoring is documentary · HCCS is primary scheduler concern · per ADR-0010 §3 NUMA weight 2 vs HCCS weight 5)
  - Per CLAUDE.md P3 "诚实优先": v0.32.x IS GA but our env can't absorb cleanly. Document the attempted upgrade + actual blocker honestly, not "GA so we flipped"
  - Per phase7-plan.md §3 T002 doc-only Allowed Paths: explicitly anticipates this branch
- **Updated 3 docs** instead of just 2 plan-listed:
  - DESIGN.md §5.2 (plan-listed): refresh deferral note with the attempted-upgrade outcome
  - ADR-0010 §3 T006 落地状态 (plan-listed: "ADR-0010 small edit if needed"): same refresh
  - known-issues.md NEW #12 (plan-listed: "re-affirm T006 deferral"): created new entry capturing the Phase 7 attempt result + Phase 8 recipe
- **#11 cross-ref**: known-issues #11 entry "scheduler-plugin opt-in via schedulerName" updated to reference P7-T-003 as the resolution task (T003 hasn't landed yet — entry stays OPEN, but the proposed-resolution sentence points to T003)

## Verification

- 存在性:
  - `git diff go.mod go.sum` = empty (revert verified) ✅
  - `git diff operators/scheduler-plugin/internal/plugins/numa/` = empty (revert verified) ✅
  - No new files outside doc-only Allowed Paths ✅
- 完整性:
  - DESIGN.md §5.2 API drift table now has 5 rows (v0.30/0.31/0.32/0.33/0.34) with GA status + compat columns
  - DESIGN.md §5.2 added "P7-T-002 attempted upgrade outcome" subsection + Phase 8 candidate forward path (5 steps)
  - ADR-0010 §3 added "P7-T-002 升级尝试结果" paragraph
  - known-issues #12 new entry with: symptom / Phase 7 attempt outcome / observable symptom / workaround / Phase 8 recipe
- 正确性:
  - `grep -nE "P7-T-002|sched-plugins v0.32" docs/known-issues.md` confirms #12 entry placed between #11 and #9 (insertion point per existing reverse-chronological grouping)
  - `grep -nE "v0\.32\.7" operators/scheduler-plugin/DESIGN.md docs/adr/0010-scheduler-plugin.md docs/known-issues.md` confirms 5+ uses of v0.32.7 specifically (matches WebFetch GA confirmation)

## Carry-forward

- **T003** (schedulerName injection) — when it lands, flip known-issues #11 to RESOLVED + cross-ref T003 commit SHA
- **T103** (kind smoke ext) — the NumaAffinity step it'd add "assert NumaAffinity in profile" stays OFF per T002 doc-only outcome; T103 instead asserts NumaAffinity REMAINS omitted (parity with Phase 6 T106 "NumaAffinity NOT in rendered config" backstop). Note this in T103 spec subagent brief
- **Phase 8 candidate task list** (preserve in checkpoint-phase7.md §"Phase 8 seed"):
  - K8s baseline bump 1.32 → 1.33 (or 1.34) — pick based on Phase 8 entry calendar + upstream Mind support status
  - sched-plugins matching minor bump
  - NumaAffinity wrap landing (rerun T002 acceptance plan with full upgrade path)
  - kind smoke `kindest/node` baseline bump
  - Re-audit known-issues for K8s baseline-dependent entries
- **Phase 7 T107 checkpoint** should record T002 as "doc-only fallback landed; T002 acceptance met for doc-only branch; full upgrade re-deferred Phase 8" in §5 DoD reconciliation
