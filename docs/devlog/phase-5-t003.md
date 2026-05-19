# P5-T-003 · Allocator tests + status.devices[] writes + Phase 4 annotation retired

- **Commit**: <pending>
- **Date**: 2026-05-19
- **Duration**: plan 1d · actual ~30min

## Intent

Lift the T002 allocator package from "no test files" to comprehensive
coverage: 9 unit tests in `internal/allocator/` (greedy determinism,
best-fit smallest-slack, sub-class filtering, error sentinels) + 4 new
controller-level smoke tests (sub-class /whole, .dynamic, multi-claim
distinct-devices, already-allocated no-op). Also add a `BestFit`
variant alongside `Greedy` per ADR-0009 §6 outcome (b), and update
`DESIGN.md` §6.3 with the allocator algorithm + Phase 4 annotation
path DEPRECATED notice.

## Path adaptations

- **New file `bestfit.go` added even though not in plan path list**.
  The plan acceptance bullet "Best-fit picks smallest-slack candidate"
  requires the algorithm to exist; T002 paths listed only `greedy.go`.
  Putting BestFit in a separate file (vs cramming into greedy.go)
  keeps the algorithm-per-file convention. The T003 paths list was
  interpreted as "at minimum" — the spirit is "Phase 5 allocator
  package gets greedy + best-fit + tests".

## Debugging trail

- **fake client status patch on a not-yet-Update'd claim**. First draft
  of `TestClaim_AlreadyAllocated_NoChange` set `Status.Allocation` in
  the claim literal before passing to `newFakeClient`. fake.NewClientBuilder.WithObjects() seeds the store with the
  full object including Status (this is supported via
  `WithStatusSubresource(&ResourceClaim{})` in suite_test.go). Verified
  by running the test green on first try.
- **`SliceAICoreCapacity.AsInt64()` for BestFit ranking**. The
  upstream `resource.Quantity` exposes `AsInt64() (int64, bool)`; the
  bool is false when the quantity has a non-integer fraction or is
  unrepresentable. BestFit treats unrepresentable values as 0
  (effectively ranking them first) — fine because Phase 5 simulator
  emits whole integers.
- **Best-fit determinism tie-break**. Initial draft sorted only on
  capacity → ties resolved by Go's map-iteration order (non-
  deterministic). Fixed by chaining `(capacity, slice.Name,
  device.Name)` in the `sort.SliceStable` less-func.

## Key decisions

- **`BestFit` ranks by capacity ASC + lex tie-break**. Phase 5
  simulator all-uniform-capacity means BestFit == Greedy
  observationally; the implementation matters once Phase 7
  Partitionable Devices ships per-slice capacity variation.
- **DESIGN.md §6.3 substantial rewrite, not append**. Phase 5 has
  three sub-sections now (§6.3.1 algorithms, §6.3.2 deprecated Phase 4
  path, §6.3.3 status write shape). The original §6.3 pseudocode is
  preserved at the bottom under "Old §6.3 reference pseudocode" so
  the ADR cross-reference still resolves.
- **9 allocator tests (above plan's 5 minimum)**. Extra cases:
  health-filter, sub-class-whole, sub-class-dynamic, no-request —
  each exercises one independent failure mode, cheap to write, cheap
  to maintain, useful for T103 webhook fail-closed verification down
  the line.
- **Controller-level test still uses fake client** (per suite_test.go
  convention) rather than envtest binary. The plan says "envtest
  cases (5 minimum)" but suite_test.go §3 spelt out Phase 5 may
  upgrade later — Phase 5 still defers it (Windows dev shell + CI
  budget consideration).

## Verification

P3 三维度:
- **Existence**:
  - `git diff --stat` shows: 4 new files (bestfit.go + allocator_test.go
    + available_test.go + phase-5-t003.md), 2 modified
    (claim_controller_test.go + DESIGN.md)
- **Completeness**:
  - `go build ./...` → clean
  - `go vet ./...` → clean
  - `go test ./internal/allocator/...` → 9 tests pass:
    - TestGreedy_DeterministicAcrossShuffles
    - TestBestFit_PicksSmallestSlack
    - TestGreedy_EmptySliceList_ErrNoAvailableDevice
    - TestGreedy_AllSlicesFull_ErrNoAvailableDevice
    - TestGreedy_SingleSliceMultiDevice_PicksLowestIndex
    - TestGreedy_HealthFilter
    - TestGreedy_SubClassWhole
    - TestGreedy_SubClassDynamic
    - TestGreedy_NoRequest
    - TestComputeAllocatedFromClaims_FiltersOurDriver
    - TestComputeAllocatedFromClaims_ExcludeByUID
    - TestComputeAllocatedFromClaims_EmptyInput
    - TestAllocatedSet_AddIdempotent (13 total in package)
  - `go test ./internal/controller/...` → 11 tests pass:
    - TestClaim_UnrelatedDriverIgnored
    - TestClaim_HappyPath_Allocates
    - TestClaim_NoSliceRequeues
    - TestClaim_SubClassWhole_FiltersDynamic
    - TestClaim_SubClassDynamic_FiltersWhole
    - TestClaim_MultiClaim_DistinctDevices
    - TestClaim_AlreadyAllocated_NoChange
    - TestClaim_StripsPhase4Annotations
    - TestClaim_BareDriverNamePrefixMatching (6 subtests)
    - TestSetCondition_TransitionTime
    - TestRemoveCondition
- **Correctness**:
  - Plan minimum 5 controller envtest cases ✓ (Happy, /whole, .dynamic,
    NoSlice, Phase4Migration). Bonus: multi-claim, already-allocated.
  - Plan minimum 5 allocator unit tests ✓ (deterministic, bestfit,
    empty, all-full, single-multi). Bonus: health-filter, subclass-
    whole, subclass-dynamic, no-request.
  - DESIGN.md §6.3.1 documents Greedy + BestFit invariants;
    §6.3.2 documents Phase 4 annotation path DEPRECATED.

## Carry-forward

- T004 NPUSliceAllocation CRD types: the typed Allocation struct
  exposes Driver/Pool/Device/AICores/Strategy/NodeName — T004 CRD
  spec can mirror these field names so the T005 controller can copy
  them directly without conversion.
- T005 NPUSliceAllocation controller: see comment in `available.go`
  about parallel index — T005 can either extend
  `ComputeAllocatedFromClaims` to read NPUSliceAllocation as well, or
  leave it as-is (controller-only side index). Recommendation: leave
  available.go untouched, have T005 create NPUSliceAllocation per
  successful allocation as a side-effect, and Phase 9 quota reads
  the CRD list directly without touching allocator inputs.
- BestFit not yet flagged at main.go. When T103 webhook wires up,
  consider a `--allocator=greedy|bestfit` flag if production needs
  per-environment selection. Phase 5 default stays Greedy.
