# P5-T-004 · NPUSliceAllocation CRD types + groupversion + scheme

- **Commit**: <pending>
- **Date**: 2026-05-19
- **Duration**: plan 0.5d · actual ~45min

## Intent

Land the first real Ocloud CRD (`NPUSliceAllocation`) in the
`npu.ocloud.edge.example.com/v1alpha1` group. Phase 4 deliberately
deferred this — the package held typed-helper structs only
(AscendDevice / AscendClaimAnnotations) and PROJECT carried no
`crdVersion` line. T004 introduces the SchemeBuilder wiring, types
file, regenerated DeepCopy, CRD YAML manifest, PROJECT update,
manager scheme registration, and 5 round-trip / DeepCopy / scheme
tests. T005 will land the controller body.

## Path adaptations

- **Chose option A (npu-dra-driver/api/v1alpha1) per plan recommendation**.
  The plan T004 §"Decision needed at task entry" listed three options;
  option A keeps the CRD alongside `DriverName` const and adjacent
  helper types, with the claim controller (T002) and audit controller
  (T005) sharing the same Go module. Phase 9 quota controller will
  reach in via dynamic client per the standard upstream pattern.
- **PROJECT update added the NPUSliceAllocation resource entry with
  `crdVersion: v1` + `controller: true`**. The existing AscendDevice
  entry stays unchanged (no crdVersion → controller-gen does not emit
  a CRD YAML for that helper struct).

## Debugging trail

- **`controller-gen v0.20.1` install via GOSUMDB=off**. First attempt
  `go install ...@v0.20.1` failed with "verifying module: ...
  sumdb/sum.golang.org/supported: EOF" — the corporate proxy can't
  reach sum.golang.org. Re-run with `GOSUMDB=off` succeeded into
  `./bin/controller-gen.exe`. Matched Phase 4 T102 convention (pin to
  Makefile-listed v0.20.1, not the v0.21.0 in user's `~/go/bin`).
- **`paths=./...` errored with "no Go files in <top>"**. controller-gen
  inspects the cwd's package list before recursing; the top of the
  module has no .go files (Kubebuilder layout) so the wildcard
  trips. Worked around by listing `paths=./api/v1alpha1/...` etc.
  individually. The CRD manifest still landed correctly under
  `config/crd/bases/` despite an RBAC-pass warning about empty
  internal/ subdir (the relevant generator pass completed).
- **No `make` on Windows**. Worked around by invoking controller-gen
  directly with the flags the Makefile's `manifests` + `generate`
  targets would set. Phase 4 T102 documented the same workaround.

## Key decisions

- **SliceReference typed struct over flat string fields**. Plan path
  bullet asked for `sliceRef SliceReference` (typed); the alternative
  would be three top-level strings (`driver`/`pool`/`device`). Typed
  is more grep-friendly (e.g. T005 controller can pass an entire
  SliceReference rather than three positionals), and matches the
  shape of upstream `DeviceRequestAllocationResult` (Driver/Pool/
  Device fields) so the T005 controller copies straight through.
- **NodeName duplicated outside SliceRef**. Phase 5 simulator
  convention is `slice.Pool.Name == slice.NodeName`. But Phase 7
  real-Ascend may run a multi-tenancy split where pool != node
  (pool = logical group of NPUs that may span hosts). Keeping
  NodeName at top-level Spec makes that future split a non-breaking
  field addition.
- **ModelServiceRef in Spec, not annotation**. T005 + T103
  consumers do not have to round-trip through
  `metadata.annotations["ocloud.edge.example.com/model-service-ref"]`
  to find the binding. Annotations stay live on the claim itself; the
  Spec field is the audit-log representation.
- **Phase enum with only 3 values (Allocated / Released / Orphaned)**.
  No Pending / Provisioning — by the time NPUSliceAllocation exists,
  the underlying ResourceClaim has already been allocated (T002
  writes both atomically). Phase=Pending would be a contradiction.
- **No NamespaceRef in Spec.ClaimRef strict shape**. ClaimRef is
  `corev1.ObjectReference` so it carries Namespace + Name + UID +
  APIVersion. Round-trip test fixture exercises this.

## Verification

P3 三维度:
- **Existence**:
  - `git diff --stat` shows: 4 new files (groupversion_info.go,
    npusliceallocation_types.go, npusliceallocation_types_test.go,
    config/crd/bases/npu.ocloud.edge.example.com_npusliceallocations.yaml),
    2 modified (zz_generated.deepcopy.go regen, PROJECT, cmd/main.go)
- **Completeness**:
  - `go build ./...` → clean
  - `go vet ./...` → clean
  - `go test ./api/v1alpha1/...` → 4 tests added (JSONRoundTrip with
    3 subtests + DeepCopy + DeepCopyObject + SchemeRegistration)
    pass; existing TestRoundTrip / TestAttributeValidation /
    TestClaimAnnotationsRoundTrip still pass (no regression)
  - controller-gen output: NPUSliceAllocation, NPUSliceAllocationList,
    NPUSliceAllocationSpec, NPUSliceAllocationStatus DeepCopy methods
    emitted into zz_generated.deepcopy.go
- **Correctness**:
  - CRD YAML round-trip via `python yaml.safe_load`:
    `kind=CustomResourceDefinition`, `scope=Cluster`,
    `singular=npusliceallocation`, `shortNames=[npua]`,
    `versions=[v1alpha1]`, `subresources=[status]`, 6 printer columns
    (Claim, Device, Node, AICores, Phase, Age)
  - SchemeRegistration test: `scheme.New(GroupVersion.WithKind(...))`
    returns the expected type for both NPUSliceAllocation and
    NPUSliceAllocationList without error

## Carry-forward

- T005 controller body:
  - Watches NPUSliceAllocation + reads owning ResourceClaim via owner
    ref → cascade behavior via K8s GC
  - On ResourceClaim allocation, claim_controller.go (T002) needs a
    new helper that creates an NPUSliceAllocation under owner-ref
    (this lands in T005 not T004 — T002 does NOT create them today)
  - Status.Phase machine: Allocated (default at create) →
    Released (claim GC pending) → Orphaned (>30s with dangling
    owner-ref); test fixtures live in T005
- T005 main.go: register `AllocationReconciler` behind
  `--enable-allocation-controller` flag (default true)
- T005 helm chart: extend
  `deploy/helm-charts/npu-dra-driver/templates/rbac.yaml` to add
  `npusliceallocations get/list/watch/create/update/patch/delete`
  + `npusliceallocations/status update/patch` (currently the chart
  has no RBAC for the new CRD)
- T005 helm chart: `templates/crd.yaml` (or `config/crd/bases/...`
  copy into chart) so `helm install` registers the CRD automatically
  alongside the deployment
- T006 inference-operator phase machine + T103 PD Router webhook
  read NPUSliceAllocation to (a) phase=Ready aggregation and
  (b) `slice-bindings` annotation value. No T004 changes needed for
  those consumers — the CRD types are stable from T004.
