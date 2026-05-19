# P5-T-005 · NPUSliceAllocation controller + claim-controller audit-log creation

- **Commit**: <pending>
- **Date**: 2026-05-19
- **Duration**: plan 1d · actual ~1h

## Intent

Add the `AllocationReconciler` that drives the NPUSliceAllocation
audit-log lifecycle (Allocated → Released → Orphaned, per ADR-0009 §5
step 3 + DESIGN.md §6.3.4) plus the claim-controller hook that
*creates* the audit entry on every successful claim allocation. Helm
chart RBAC + values toggle + deployment arg passthrough land in the
same task; main.go gains the `--enable-allocation-controller` flag
defaulting true.

## Path adaptations

- **suite_test.go updated to register v1alpha1 scheme**. Not in T005
  Allowed Paths but unavoidable — the claim controller now calls
  `r.Client.Create(audit)` where `audit` is `*v1alpha1
  .NPUSliceAllocation`; without the scheme registration the fake
  client errors `no kind registered`. Scope creep is genuinely
  necessary, not optional. Added `WithStatusSubresource(...,
  &v1alpha1.NPUSliceAllocation{})` so allocation_controller_test
  status patches take effect.
- **Best-effort audit creation tolerated**. In a real cluster, if
  status.allocation was patched but the audit Create errored
  (transient), the next reconcile sees Status.Allocation != nil and
  early-returns. The early-return path now calls
  `ensureAllocationAudits` so the audit eventually catches up.

## Debugging trail

- **vet caught "declared and not used: name" in multi-claim test**.
  First draft of `TestAllocation_MultiClaim_DistinctEntries` had a
  for-loop that declared `name, _ = ` and then never used `name`.
  Replaced with direct slice literal. vet found this before runtime;
  go test would have errored too.
- **Fake client + Status subresource for NPUSliceAllocation**. Without
  `WithStatusSubresource(&v1alpha1.NPUSliceAllocation{})` the fake
  client treats `Status().Patch(audit, ...)` as a regular patch — but
  controller-runtime's interface contract is "Status subresource has
  its own path". The orphan/released tests verify `Status.Phase` so
  the registration is mandatory.
- **Orphan grace test deterministic with injected clock**. Plan T005
  wants the 30s default observable; tests can't wait 30 wall-clock
  seconds. Injected `GracePeriod` + `Now` fields on
  AllocationReconciler so `TestAllocation_Orphan_AfterGracePeriod` can
  set `Now = func() time.Time { return now }` + `GracePeriod = 30s` +
  AllocatedAt = `now - 2min` → orphan path exercised in microseconds.

## Key decisions

- **`createAllocationAudit` on first allocation + `ensureAllocationAudits`
  on re-reconcile**. Two-helper pattern: first emission carries the
  rich Allocation struct (NodeName/AICores/Strategy/ModelServiceRef);
  later re-emissions (resync of a missing audit) reconstruct from
  claim.Status.Allocation alone — degraded but sufficient. Avoids a
  race where status patch succeeds but audit Create fails, leaving
  the audit absent forever.
- **Cascade-delete via OwnerReference, no direct delete from
  AllocationReconciler**. The reconciler observes the K8s GC outcome;
  it never deletes audit entries itself. Phase=Orphaned exists for
  human inspection when GC fails.
- **OrphanGracePeriod constant + GracePeriod field override**. 30s
  default per plan acceptance; tests override to make timing
  deterministic. Production deploys can override via main.go flag
  later if real-cluster experience shows 30s is too tight (CI kind
  clusters with slow GC may want 60s — values.yaml override called
  out in DESIGN.md §"Phase 5 orphan timing").
- **Cascade-delete test verifies OwnerRef shape, not actual deletion**.
  Fake client doesn't run K8s GC. Test asserts Controller=true +
  BlockOwnerDeletion=true on the OwnerRef. The kind smoke (T106)
  exercises real GC.
- **Audit name `<claim.ns>-<claim.name>-<device>` deterministic**.
  RFC 1123 compliant — both claim and device names are DNS labels.
  Operator-readable in `kubectl get npua`. Re-running the controller
  on a recreated claim produces the same audit name → AlreadyExists
  → idempotent (treated as success).

## Verification

P3 三维度:
- **Existence**:
  - `git diff --stat` shows 7 changed files: 2 new
    (allocation_controller.go, allocation_controller_test.go, devlog),
    5 modified (claim_controller.go, suite_test.go, cmd/main.go,
    deploy/.../values.yaml, deploy/.../templates/deployment.yaml,
    deploy/.../templates/rbac.yaml, DESIGN.md)
- **Completeness**:
  - `go build ./...` → clean
  - `go vet ./...` → clean (vet caught one issue mid-development;
    fixed)
  - `go test ./...` → all 5 packages green, 16 controller-package
    tests pass (5 new allocation + 11 claim)
  - `helm lint --strict deploy/helm-charts/npu-dra-driver` →
    `1 chart(s) linted, 0 chart(s) failed`
  - `helm template test ...` renders 4 npusliceallocations RBAC
    rules (resources + resources/status) + `--enable-allocation-
    controller` deployment arg + Pod annotation
- **Correctness**:
  - Plan minimum 4 envtest cases:
    - Happy ✓ (`TestAllocation_HappyPath_PhaseAllocated`)
    - Cascade delete ✓ (`TestAllocation_CascadeDelete_OwnerRefIsSet`)
    - Multi-claim ✓ (`TestAllocation_MultiClaim_DistinctEntries`)
    - Orphan ✓ (`TestAllocation_Orphan_AfterGracePeriod`)
    - Bonus: Released within grace
      (`TestAllocation_Released_WithinGracePeriod`)
  - DESIGN.md §6.3.4 documents NPUSliceAllocation lifecycle +
    chart wiring + Phase 5 invariants

## Carry-forward

- T006 ModelService scaffold uses NPUSliceAllocation list to phase
  status of PD-pair: `phase=Ready` requires per-replica claims to
  have AllocatedDeviceStatus + corresponding NPUSliceAllocation
  Available=True conditions.
- T103 PD Router webhook reads NPUSliceAllocation list by the
  `ocloud.edge.example.com/claim-namespace` + `claim-name` labels
  to compose the `npu.huawei.com/slice-bindings` annotation value.
  Label scheme is set in `createAllocationAudit`.
- Phase 9 quota controller will list NPUSliceAllocations by claim-
  namespace label to compute per-tenant allocation counts. Spec
  fields are stable from T004.
- main.go flag `--enable-allocation-controller=true` default works
  in production; T106 kind smoke sets it implicitly via helm.
- Orphan grace tuning: 30s baseline; if kind CI flakes, bump via
  `--enable-allocation-controller=true` + ENV-injected GracePeriod
  in a later task (no API change required since GracePeriod is a
  field on the Reconciler).
