# P6-T-007 · Binpack ScorePlugin

- **Commit**: <pending>
- **Date**: 2026-05-20
- **Duration**: plan 0.5d · actual ~25min

## Intent

Land the Binpack ScorePlugin per ADR-0010 §4 — internal ~50 LOC Score
impl (no volcano dep). Default disabled; operators opt in via chart
values.yaml. NPU resource weighted 5x over CPU/Memory to bias
binpacking toward consolidating NPU workloads (Phase 8 vertical-scaling
substrate).

## Path adaptations

- **Plan estimated ~50 LOC; actual ~150 LOC across binpack.go +
  args.go**: the Score formula itself is ~25 LOC, but adding args
  parsing (mirroring the hccs/args.go pattern with default
  substitution + Weight bounds validation + DeepCopyObject) +
  podRequestedTotals helper + requestedFor/allocatableFor helpers
  pushed the count up. Not bloat — same boilerplate that hccs has
  (manual DeepCopyObject because we're not a Kubebuilder project).

- **Binpack test exposed a bug in scoreNodeForPod**: my first cut had
  `args.Enabled=false` check only in the wrapper `Score(...)` method,
  not in the testable `scoreNodeForPod(...)` core. The "disabled →
  0" test case caught this immediately (failed with score=21 instead
  of 0). Moved the check into `scoreNodeForPod` so both code paths
  honor `Enabled`. Lesson: any testable-core function should
  enforce all its preconditions itself, not delegate to a wrapper.

- **Replaced T002 placeholder plugin.go**: the binpack package
  previously had `plugin.go` (the T002 scaffold placeholder with
  Name() only). T007 supersedes it with `binpack.go`. Deleted
  `plugin.go` rather than co-locate to avoid `Name redeclared in
  this block` vet error. The Name const + Binpack struct + factory
  all migrate to binpack.go intact.

## Debugging trail

- **`framework.NodeInfo.Allocatable` access**: had to recall that
  CPU is in MilliCPU (int64 millicores), Memory in Memory (int64
  bytes), and extended resources in ScalarResources map (int64
  counts). Built `allocatableFor` switch to surface the right
  field per ResourceName. Same pattern for `requestedFor` (Pod
  CPU is MilliValue, others are Value).

- **NewNodeInfo + SetNode**: NewNodeInfo takes optional Pods; SetNode
  populates Allocatable from `node.Status.Allocatable`. Tests build
  Node with Status.Allocatable populated, then call SetNode.

- **All 9 sub-tests pass in 0.47s**: disabled / single-resource NPU /
  multi-resource default weights / empty allocatable / zero-request
  Pod / clamped ratio (over-request) + 3 parseArgs cases.

## Key decisions

- **Clamp ratio at 1.0**: a Pod that over-requests (e.g. wants 100
  NPUs on an 8-NPU node) would otherwise produce score > 100. Clamp
  ensures output stays in [0..100] without needing a separate
  normalization pass. This also means over-requesting Pods are
  treated as "consumes the entire node" — appropriate for binpack
  (they certainly can't share with anyone else).

- **Skip resources where weight ≤ 0**: defensive against
  malformed user config. Args parsing doesn't reject zero/negative
  per-resource weights (only the overall Weight is bounded), so the
  scoring loop silently skips them. Documented in code.

- **No KubeSchedulerConfiguration changes in T007**: chart wiring
  is T101's job; T007 only changes the binary. The plan acceptance
  was specifically about "Score logic + tests", not chart.

- **Skip nodes with zero allocatable for ALL weighted resources**:
  if every weighted resource has either no Pod request or no node
  allocatable, totalWeight=0 → score=0. Effectively "skip the node"
  per plan acceptance.

- **Pod summing across containers**: `podRequestedTotals` adds
  Requests across all containers. K8s scheduler upstream uses the
  same pattern (`v1helper.PodRequests`); I inlined to avoid
  depending on internal-only helpers from K8s.

## Verification

P3 三维度:

- **Existence**:
  - `git status --short` → 4 files changed (DESIGN.md, plugin.go
    deleted) + 3 new (binpack.go, args.go, binpack_test.go) +
    devlog
  - `ls operators/scheduler-plugin/internal/plugins/binpack/` →
    args.go, binpack.go, binpack_test.go (3 files, plugin.go
    removed)

- **Completeness** (plan §3 P6-T-007 acceptance):
  - Internal thin impl, no volcano dep ✅
  - Score formula `sum(weight*requested/allocatable)/sum(weight)` ✅
  - Default ResourceWeights `{cpu:1, memory:1, npu.../devices:5}` ✅
  - 4 envtest cases ✅ + bonus 2 (zero-request, clamped) = 6 Score
    cases total
  - Default disabled ✅
  - `go test ./internal/plugins/binpack/... -timeout 30s` clean ✅
    (9 sub-tests pass in 0.47s)

- **Correctness**:
  - `go vet ./...` exit 0
  - `go build ./...` exit 0
  - `go test ./internal/plugins/...` exit 0, 37 sub-tests pass
    (HCCS 26 + NUMA 2 + Binpack 9)
  - DESIGN.md §5.3 updated to operative; LOC count documented

## Carry-forward

- **T008 integration tests** will exercise Binpack alongside HCCS
  filter+score. Default-disabled means Binpack stays a no-op in the
  smoke run unless the integration test opts it in via Args.

- **T101 chart**: values.yaml should expose `binpack.enabled` toggle
  defaulting to false; KubeSchedulerConfiguration include Binpack
  in score.enabled[] but framework will respect the Args.Enabled
  short-circuit.

- **Phase 8 binpack policy**: Phase 8 vertical-scaling needs binpack
  to bias toward freeing nodes. T007 default weights (NPU=5,
  cpu=memory=1) already do that. If Phase 8 wants more aggressive
  bias, the values.yaml ResourceWeights override is the lever.

- **T002 placeholder cleanup**: `plugin.go` in `internal/plugins/
  binpack/` deleted as part of this commit. T002 scaffold's
  identical placeholder in `internal/plugins/hccs/` was already
  superseded by T004; `internal/plugins/numa/` still uses the
  placeholder pattern (per T006 deferral).
