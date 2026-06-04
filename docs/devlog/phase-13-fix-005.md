# P13-fix-005 · NodePool include arm64 Kunpeng nodes (ADR-0020)

- **Commit**: this commit (main agent · closer forward-fix off `dev`)
- **Date**: 2026-06-03
- **Surfaced by**: P13-fix-004 doc-completeness backfill (commit `34838c0`) flagged this
  in `operators/pool-operator/DESIGN.md` §5 (🔴 bullet) + `phase-13-fix-004.md` as a
  separate code+test task; this commit is that task.

## Problem

`nodepool_controller.go` dropped every Node with `kubernetes.io/arch=arm64`, citing the
*original* amd64-only assumption (ADR-0001 §13, sourced in the comment as "architecture.md
§1.2"). **ADR-0020 (Phase 12) superseded ADR-0001 §13** and flipped the real deployment
target to aarch64 鲲鹏 Kunpeng 920 + 昇腾 910B + openEuler (Atlas 800 native). On a real
cluster every node is arm64 → the old guard would empty `status.nodes` and zero
`totalCPU`/`totalMemory`. NPUPool/NPUSlicePool aggregate by `huawei.com/Ascend910B`
capacity (arch-agnostic), so the **NPU** path was never affected — only NodePool's CPU/Mem
rollup was broken on the actual target platform.

## Decision: remove the arch filter entirely (not invert)

Considered (a) invert to exclude amd64, (b) keep a configurable filter, (c) remove all arch
filtering. Chose **(c)** because:
- arm64 = deployment target **and** amd64 is explicitly retained for dev/CI/render-verify
  (ADR-0020 §2.1 dual-track) — both must count.
- helm arch affinity is **soft** `preferredDuringScheduling` (ADR-0020 §2.4 ·
  build-and-production-validation.md §… "arm64 优先、不硬性排他") → the project's own posture
  is non-exclusive; a hard arch drop in NodePool contradicts it.
- NPUPool/NPUSlicePool never filtered by arch → removing it makes NodePool consistent with
  the other 3 pools. Arch is a scheduling concern, not a pool-aggregation concern.

## Cascade (P4 vertical + horizontal)

- **code** `nodepool_controller.go` — 3 spots: (1) type-doc Reconcile contract bullet,
  (2) the reconcile loop `if arch==arm64 { continue }` removed, (3) NoNodeMatched message
  "matched no **amd64** Nodes" → "matched no Nodes".
- **test** `nodepool_controller_test.go` — flipped "arm64 excluded" → "arm64 included: 3
  amd64 + 2 arm64 → all 5 counted (56 CPU / 224Gi)"; added "pure arm64 Kunpeng cluster"
  regression guard (3×arm64 → 48 CPU / 192Gi, status NOT empty); kept happy-path amd64 spec
  (proves dev nodes still work).
- **docs** `pool-operator/DESIGN.md` — §3.2 reconcile flow ("+排除 arm64" → arch-agnostic),
  §5 🔴 forward-fix bullet → ✅ resolved, §7 ADR-0020 reference wording.
- horizontal grep `kubernetes.io/arch == "arm64"` exclusion: **only** this controller had it
  (helm charts already use *soft include* affinity). checkpoint-phase13 does not list it as a
  carry → no stale cross-ref left. The `phase-13-fix-004.md` mention is left as history (it
  correctly records when the issue was *flagged*).

## Verify

- **baseline on pristine tree (before edits)**: `go test ./...` = Ran 23/23, **17 Passed /
  6 Failed**. The 6 are all `NoKindMatchError: ResourceSlice resource.k8s.io/v1beta1`
  (4 NPUPool hccs + 2 NPUSlicePool) — envtest cache is K8s 1.35, which no longer serves
  `resource.k8s.io/v1beta1` (suite_test.go:27 registers v1beta1). Unrelated to arch. This
  pristine-first run is the "same 6 fail on unchanged code" confirmation (stronger than
  `git stash`).
- **after edits**: Ran 24/24, **18 Passed / 6 Failed** — same 6 ResourceSlice failures,
  +1 pass = the new pure-arm64 spec. No NodePool spec fails.
- `go build ./...` ✓ · `go vet ./...` ✓ · `CGO_ENABLED=0 GOARCH=arm64 go build ./...` ✓
  (also `GOOS=linux GOARCH=arm64` ✓ — the real target).
- gofmt: working tree is CRLF (`core.autocrlf=true`; git stores LF per `ls-files --eol`
  `i/lf w/crlf`) so `gofmt -l` flags all 12 controller files — a local artifact, CI sees LF.
  My edited regions + the whole controller .go are content-gofmt-clean. A pre-existing
  alignment deviation in the untouched happy-path spec (lines 114-116, present in HEAD) was
  left alone — CI does not gate gofmt, and §13 single-responsibility says don't churn
  unrelated code.
