# P5-T-002 · npu-dra-driver Claim controller real allocator (greedy first-fit)

- **Commit**: <pending>
- **Date**: 2026-05-19
- **Duration**: plan 1.5d · actual ~1.5h

## Intent

Replace the Phase 4 annotation-only AllocationDeferred path with a real
greedy first-fit allocator that writes
`ResourceClaim.Status.Allocation.Devices.Results[]` + `Status.Devices[]`
per ADR-0009 §5 step 2 + §6 outcome (a). The allocator is a small
pluggable package (`internal/allocator/`) so T003 can add table-driven
tests and a best-fit variant without touching the controller.

## Path adaptations

- **Plan path `api/v1alpha1/resourceclaim_types.go` not modified**. The
  plan called out a "small edit — annotation consts marked deprecated".
  Those consts (`AnnotationAllocationDeferred*`) actually live in
  `internal/controller/claim_controller.go`, not in
  `api/v1alpha1/resourceclaim_types.go` (which holds the Phase 5-live
  `AnnotationModelServiceRef` / `AnnotationPreferredPool`). I added the
  `// Deprecated:` notices to the actual location instead. Not touching
  resourceclaim_types.go keeps Phase 5 consumers (PD Router T103) using
  the live constants without churn.
- **No main.go change required**. The plan does not list `cmd/main.go`
  as an Allowed Path. ClaimReconciler now has an `Allocator` field but
  it defaults to `&allocator.Greedy{}` via a nil-check inside
  `allocatorOrDefault()`. The existing struct literal
  `{Client, Scheme, Recorder}` in main.go compiles untouched and the
  zero-value Allocator path works end-to-end. T003+ may wire an
  explicit flag (`--allocator=greedy|best-fit`) later.

## Debugging trail

- **`AllocationResult` field naming clarification**. The v1beta1 type
  graph: `ResourceClaimStatus.Allocation` (`*AllocationResult`) carries
  `Devices` (`DeviceAllocationResult`), which carries `Results`
  (`[]DeviceRequestAllocationResult`). And the parallel
  `Status.Devices` field is `[]AllocatedDeviceStatus`. Both need to be
  populated for a real allocation:
    - `Status.Allocation.Devices.Results[]` — the picked Driver/Pool/
      Device per request, scheduler-visible.
    - `Status.Devices[]` — per-device Ready condition (driver liveness
      signal to consumers).
  Confirmed against the local module cache at
  `~/go/pkg/mod/k8s.io/api@v0.35.0/resource/v1beta1/types.go`
  (lines 1448, 1537, 1566, 1595, 1934).
- **Defensive sort on a copy**. `sort.SliceStable(slices, ...)` would
  mutate the caller's slice; the Phase 5 controller passes
  `slices.Items` from a List call, which is fine to mutate in
  controller-runtime's cached client, but the allocator contract
  promises purity. Using `append([]T(nil), src...)` first preserves
  caller ordering.
- **AllocatedSet exclude key by UID**. ComputeAllocatedFromClaims takes
  an `exclude` parameter so a claim being re-reconciled (already had an
  allocation, status update bounced through the watch) doesn't block
  itself. UID is the stable per-object identifier — re-using
  namespace/name is fragile under quick delete+recreate.

## Key decisions

- **Greedy first-fit by `slice.Name` then `device.Name`**. Determinism
  contract: same input → same output. The publisher's slice names are
  `npu-dra-<node>` and device names follow a `<node>-npu-<index>`
  template (per Phase 4 simulator), so sort order is effectively node
  hostname order then NPU index order. Predictable + diffable across
  kind smoke runs.
- **Health filter inside Greedy**. `Health == Healthy` gates picks
  (Unhealthy / Unknown skipped). Could move to a separate predicate
  later (Phase 7 may want admin override) but Phase 5 keeps health as
  an implicit allocator concern — same scope as ADR-0009 §6.
- **Single-request claim model for Phase 5**. The allocator picks the
  first request and only fills that one. Multi-request claims emit
  ErrNoAvailableDevice on the additional requests; Phase 6 may iterate
  through `claim.Spec.Devices.Requests` and produce N results.
- **Phase 4 annotation strip as a separate reconcile pass**. On first
  encounter of an annotated claim, controller strips the annotations
  via metadata patch + Requeue. Keeps "one concern per reconcile"
  invariant — strip → return → next reconcile sees a clean claim and
  runs the real allocator. Migration test in
  `TestClaim_StripsPhase4Annotations` exercises both passes.
- **Status writes via Status().Patch with MergeFrom**. Avoids
  clobbering other status fields (e.g. ReservedFor written by the
  scheduler) under concurrent edit. The MergeFrom base is a DeepCopy
  taken before any field mutation.

## Verification

P3 三维度:
- **Existence**:
  - `git diff --stat` shows 5 changed files: 3 new in
    `internal/allocator/` (allocator.go / greedy.go / available.go),
    + claim_controller.go rewritten, + claim_controller_test.go
    rewritten.
- **Completeness**:
  - `go build ./...` → clean
  - `go vet ./...` → clean
  - `go test ./internal/controller/... -timeout 60s` → ok 0.553s
  - `go test ./internal/allocator/... -timeout 60s` →
    `[no test files]` (T003 adds real coverage; T002 acceptance
    line allows this — empty package + no errors)
  - `go test ./... -timeout 120s` → all 5 packages green:
    api/v1alpha1 + controller + publisher + cmd + allocator
- **Correctness**:
  - `TestClaim_HappyPath_Allocates`: claim with bare class allocates
    against a 1-device slice; asserts Driver/Pool/Device + Ready
    condition on Status.Devices[0]
  - `TestClaim_NoSliceRequeues`: empty slice store → Requeue=true,
    no allocation, NoAvailableDevice event emitted
  - `TestClaim_StripsPhase4Annotations`: pre-annotated claim → first
    reconcile strips, second allocates
  - `TestClaim_UnrelatedDriverIgnored`: foreign claim is a no-op
  - `TestClaim_BareDriverNamePrefixMatching`: keeps the Phase 4
    isOurClass coverage (exact / .sub / /sub / unrelated / empty /
    look-alike) intact

## Carry-forward

- T003 adds envtest cases (sub-class filtering / multi-claim
  determinism / orphan migration scenarios) + the table-driven
  allocator unit tests called out in T002 acceptance bullet
  `go test ./internal/allocator/...` — will turn `[no test files]`
  into real coverage of `Greedy.Allocate` + `ComputeAllocatedFromClaims`.
- T005 NPUSliceAllocation controller will subscribe to the claim
  watch and create one CRD object per `Status.Allocation` populated
  by this controller. The audit-log entries do NOT change allocator
  inputs (we read ResourceClaim.Status as the canonical source); they
  are a parallel index for Phase 9 quota + Phase 6 scheduler-plugin
  reverse lookup.
- T103 PD Router webhook will reuse `Allocation` struct semantics
  (Driver/Pool/Device + AICores) to build the
  `npu.huawei.com/slice-bindings` annotation; the allocator-emitted
  metadata is preserved on the claim where the webhook reads it.
- main.go remains Phase-4-shape. When T103 ships, main.go gains a
  webhook registration; if T103 also wants a non-default allocator,
  the flag goes in then.
