# P5-T-103 · PD Router mutating logic + slice-bindings annotation injection

- **Commit**: <pending>
- **Date**: 2026-05-19
- **Duration**: plan 1d · actual ~40min

## Intent

Replace the T102 always-Allow scaffold with the real mutating logic
per ADR-0008: for any Pod carrying the
`inference.ocloud.edge.example.com/model-service` label, read the
NPUSliceAllocation entries for that ModelService and inject the
`npu.huawei.com/slice-bindings` annotation. Honor ADR-0008's "fail-
closed default" by Denying admission when every allocation reports
phase=Orphaned.

## Path adaptations

- **NPUSliceAllocation read via unstructured client, not typed import**.
  Same operators/CLAUDE.md §1 constraint that drove T006's pool
  lookup via unstructured. The webhook can't import npu-dra-driver's
  `v1alpha1.NPUSliceAllocation` type, so it lists cluster-wide
  `npu.ocloud.edge.example.com/v1alpha1/NPUSliceAllocationList` via
  unstructured and reads Spec.modelServiceRef + nodeName + sliceRef +
  aiCores via `unstructured.NestedString` / `NestedInt64`.

## Debugging trail

- **Test assertion on patch contents was first too strict**.
  The fake decoder produced patches as
  `{Operation: "add", Path: "/metadata/annotations", Value: map[string]string{...}}` —
  my first `mustMarshal` helper returned a placeholder "<json>" for
  non-string values, so the assertion missed the actual annotation
  content. Fixed by writing a `fmtPatchValue` helper that handles
  `string`, `map[string]string`, and `map[string]interface{}` cases
  for substring search.
- **Slice-binding encoder determinism**. First sketch returned
  bindings in Go map iteration order — that's non-deterministic.
  Added explicit `sort.SliceStable` on `(Node, Pool, Device)` so
  consumers diffing the annotation across reconciles see stable
  output for stable input. `TestEncodeBindings_DeterministicSort`
  verifies.
- **`_ = phaseReleased` to silence unused-const linters**. The
  `phaseAllocated` and `phaseOrphaned` constants are referenced by
  the handler; `phaseReleased` is documented for completeness but
  not used by encoder or counter directly. Blank assignment keeps
  the constant value-tested in the future without lint noise.

## Key decisions

- **`DenyOnOrphaned` field on PDRouter, default true via main.go**.
  Plan T103 acceptance: "All claims Orphaned ... handler returns
  Denied with reason NoAvailableNPUSlices (production override
  toggle in values.yaml)". Implemented as a struct field; main.go
  sets true; T104 envtest sets either depending on case. Helm chart
  override path documented for Phase 6 (`pdRouter.denyOnOrphaned`
  values key — deferred).
- **Partial-allocation → Allowed without patch**. Plan: "best-effort
  enrichment, not blocking". Implementing this as: if any binding
  has Phase != Allocated AND != Orphaned, skip the patch. The
  webhook fires again on the next Pod re-admission (which doesn't
  happen normally) — OR more practically, the downstream PD Router
  proxy reads the annotation when it next polls the API server.
- **Annotation format `<node>/<pool>/<device>:<aiCores>`**. Plan
  literal. Sort order: lex on (node, pool, device). Phase 5
  simulator convention pool==node, so most users see
  `<node>/<node>/<device>:<aiCores>`. Phase 7 multi-node pool ready.
- **Filtering NPUSliceAllocations in-Go after cluster-wide List**.
  Phase 5 cluster sizes are O(claims) = O(replicas) which is small;
  field selectors on custom resources aren't widely supported. Phase
  6 may introduce an indexed FieldIndexer when scaling.
- **Module-local `slice_binding.go` encoder package**. Pure functions
  + types — small enough to test independently from the handler.
  T104 envtest exercises the full pipeline; unit tests in T103
  cover deterministic sort + filter + counter math.

## Verification

P3 三维度:
- **Existence**:
  - `git status` shows: 2 new files (slice_binding.go,
    slice_binding_test.go), 2 modified (pd_router.go,
    pd_router_test.go), main.go small edit, devlog
- **Completeness**:
  - `go build ./...` → clean
  - `go vet ./...` → clean
  - `go test ./internal/webhook/... -v` → 10 + 4 = 14 tests pass:
    - TestHandle_HappyPath_InjectsSliceBindings
    - TestHandle_NoMatchingAllocations_AllowedWithoutPatch
    - TestHandle_AllOrphaned_Denied
    - TestHandle_AllOrphaned_DenyOff_AllowedWithoutPatch
    - TestHandle_PartialAllocation_AllowedWithoutPatch
    - TestHandle_ScaffoldAllowsAll (no-bindings)
    - TestHandle_NonMSPod_Allowed
    - TestHandle_BadPodPayload_AllowedNotDenied
    - TestLabelModelService_MatchesControllerConstant (drift guard)
    - TestAnnotationSliceBindings_FormatStable (drift guard)
    - TestEncodeBindings_DeterministicSort
    - TestEncodeBindings_Empty
    - TestFilterAllocated_DropsNonAllocated
    - TestCountByPhase
- **Correctness**: Plan T103 acceptance:
  - Annotation value `<node>/<pool>/<device>:<aiCores>` ✓
  - Pod missing model-service label → Allowed without patch ✓
  - All claims allocated → annotation has N entries ✓
  - Some claims pending → Allowed without patch ✓
  - All claims Orphaned → Denied with NoAvailableNPUSlices ✓
  - ADR-0008 fail-closed default honored (DenyOnOrphaned=true) ✓

## Carry-forward

- T104 envtest: spin up a real webhook server in a test process and
  post admission.Request HTTP bodies via httptest. Cover:
  - happy (assert annotation set on Pod observed via mock API)
  - non-MS Pod (assert no annotation)
  - empty pool (assert phase=Provisioning + no annotation)
  - cert failure (failurePolicy=Fail blocks Pod create; failurePolicy
    =Ignore Pod created without annotation)
- T106 kind smoke: cert-manager + this chart + ModelService;
  assert Pods carry `npu.huawei.com/slice-bindings` within timeout
  (deterministic value because of sort).
- Phase 6 may:
  - Add `--pd-router-deny-on-orphaned` flag (chart values key) for
    runtime override without rebuild
  - Add FieldIndexer on Spec.modelServiceRef so list-by-MS scales
  - Surface a `webhook` Prometheus counter for "denied / allowed
    no-patch / patched" rates
