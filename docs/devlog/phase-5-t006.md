# P5-T-006 · inference-operator ModelService controller scaffold

- **Commit**: <pending>
- **Date**: 2026-05-19
- **Duration**: plan 1.5d · actual ~1h

## Intent

Land the first Reconciler body in the inference-operator package:
ModelServiceReconciler. T006 ships the scaffold (finalizer add,
NPUSlicePool resolution, phase Pending → Provisioning); T007 lands
Deployment + ResourceClaim materialisation; T008 lands the full phase
machine (→ Ready / Failed). Helm chart `deploy/helm-charts/inference-
operator/` also lands per plan path list.

## Path adaptations

- **NPUSlicePool read via `unstructured.Unstructured`, not typed
  import**. operators/CLAUDE.md §1 forbids cross-module Go imports
  ("module path 不交叉依赖"). The plan acceptance asked for
  "resolve spec.npuSlicePoolRef" — using unstructured satisfies the
  contract without violating the module boundary. `poolGVK` constant
  pins the lookup.
- **Pool readiness via `status.totalSlices > 0` heuristic**. NPUSlicePool
  doesn't expose a top-level Phase enum (verified by reading
  `operators/pool-operator/api/v1alpha1/npuslicepool_types.go`). The
  simplest "is the pool ready to serve allocations" predicate is
  `totalSlices > 0` — pool-operator's NPUSlicePool Reconcile populates
  this when the npu-dra-driver publishes ResourceSlices for the
  underlying NPU pool. T007 may refine to also require
  `availableSlices >= prefill.replicas + decode.replicas` before
  allocating claims.
- **No envtest, fake client per project convention**.
  npu-dra-driver/internal/controller/suite_test.go §1 documents the
  convention; mirrored verbatim here. Phase 6+ may upgrade.
- **Cross-module pool fixture via Unstructured in tests**.
  `newPoolFixture` builds an unstructured pool object so the fake
  client's pool-not-found case returns NotFound cleanly without
  registering pool-operator types in the test scheme.

## Debugging trail

- **First draft suite_test.go tried `AddKnownTypeWithName(gvk,
  &unstructured.Unstructured{})`**. Compile-time failure: runtime
  .Scheme expects a concrete Go type, not the generic Unstructured.
  Pivoted to letting the fake client handle unstructured natively —
  it does, as long as `SetGroupVersionKind` is called on the object
  before `WithObjects`. Removed the scheme registration entirely;
  fake client + `client.Get` against `&unstructured.Unstructured{}`
  works.
- **Finalizer round-trip test deletion-path**. The first draft
  expected `Get(...)` after deletion to succeed with the finalizer
  removed; in practice the fake client GC's the object once the last
  finalizer is removed. Test now accepts both branches (NotFound OR
  found-without-finalizer) — matches real K8s behavior.

## Key decisions

- **One Reconcile invariant: each pass at most one of (add finalizer,
  resolve pool, set provisioning, set failed)**. Avoids tangled
  multi-step writes per reconcile that can race with watches on the
  status subresource. The early-return-with-Requeue pattern (after
  finalizer add) keeps the workqueue cadence predictable.
- **`unstructured.NestedInt64(..., "status", "totalSlices")`**. Soft
  fallback when the field is missing → `found=false`, treated as
  "pool has no slices yet" → markWaitingForPool. Doesn't error out
  the Reconcile for a pool the publisher hasn't observed yet.
- **`ConditionPoolUnresolved` polarity**: True means "found"
  (resolved), False means "missing" (unresolved). Slightly inverted
  from the literal name but matches K8s status convention (the
  *Resolved* / *Unresolved* state is the value, the condition name
  pins the topic).
- **`reasonScaffold = "ProvisioningScaffold"` for the
  pool-found-but-T006 case**. T007 will replace this reason with
  `AwaitingClaims` once claims are created; T008 with `AllocationReady`
  when claims report Ready. Forward-compat: consumer code can match
  any of these reasons to "ModelService is provisioning".

## Verification

P3 三维度:
- **Existence**:
  - `git status` shows: 4 new go files in internal/controller +
    8 new helm chart files in deploy/helm-charts/inference-operator/
    + 1 modified main.go + 1 modified DESIGN.md + devlog
- **Completeness**:
  - `go build ./...` → clean
  - `go vet ./...` → clean
  - `go test ./internal/controller/... -v -timeout 60s` →
    6 tests pass (FinalizerAdded / HappyPath_Provisioning /
    PoolNotFound_PhaseFailed / PoolFoundButEmpty_WaitingForPool /
    Deletion_FinalizerRemoved / SetCondition_Smoke)
  - `helm lint --strict deploy/helm-charts/inference-operator` →
    `1 chart(s) linted, 0 chart(s) failed`
  - `helm template test ...` renders 4 kinds:
    ClusterRole, ClusterRoleBinding, Deployment, ServiceAccount
- **Correctness**:
  - Plan minimum 3 envtest cases satisfied + 3 bonus:
    - happy-path (FinalizerAdded + HappyPath_Provisioning) ✓
    - pool-not-found (PoolNotFound_PhaseFailed) ✓
    - finalizer add+drain (FinalizerAdded + Deletion_FinalizerRemoved) ✓
    - bonus: pool-found-but-empty + condition-helper smoke
  - DESIGN.md §4.2.1 documents Phase 5 controller architecture (data
    flow + finalizer states + cross-reference to ADR-0008)

## Carry-forward

- T007 wires Deployment + ResourceClaim builders. The Reconciler
  returns early at `markProvisioning` today; T007 inserts the
  Deployment + claim materialisation step before returning.
- T007 must NOT add a typed import on pool-operator or npu-dra-driver
  for the claim creation either — `resourceapi "k8s.io/api/resource/
  v1beta1"` is the upstream type (no Ocloud-side helper needed).
- T008 expands the deletion-drain to wait for owned Deployments
  (`appsv1.DeploymentList` with the model-service label selector)
  + ResourceClaims to be GC'd before unblocking deletion.
- T101 cert-manager dependency in `Chart.yaml`. T102 adds
  `templates/mutatingwebhookconfiguration.yaml` + webhook port (9443)
  on Deployment.
- T103 webhook handler reads NPUSliceAllocation list via the
  `ocloud.edge.example.com/claim-namespace` + `claim-name` labels
  set by npu-dra-driver T005 — no new GVK needed beyond what RBAC
  already grants here.
- T106 kind smoke installs this chart + creates a ModelService
  referencing an existing NPUSlicePool fixture; asserts phase
  reaches Ready within timeout.
