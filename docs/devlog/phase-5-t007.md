# P5-T-007 · inference-operator Prefill+Decode Deployment materialisation

- **Commit**: <pending>
- **Date**: 2026-05-19
- **Duration**: plan 1.5d · actual ~45min

## Intent

Extend the T006 Reconciler scaffold with the actual Deployment +
ResourceClaim creation logic. Each ModelService now produces:
  1. Two Deployments (`<ms>-prefill`, `<ms>-decode`) with replica
     count + container image driven by spec
  2. Two ResourceClaimTemplates (`<ms>-<side>-claim`) carrying the
     `npu.ocloud.edge.example.com` DeviceClass + model-service-ref /
     preferred-pool annotations
  3. PodResourceClaim wiring on each Pod template so K8s creates one
     ResourceClaim per replica at scheduling
  4. OwnerReferences on all child resources so ModelService delete
     cascades cleanly

## Path adaptations

- **ResourceClaimTemplate over per-replica ResourceClaim objects**.
  Plan acceptance: "Per-replica ResourceClaims created (one
  ResourceClaim per Pod replica)". Deployment Pod templates can't
  reference per-replica claim NAMES (unlike StatefulSet's per-ordinal
  identity), so the standard DRA pattern is ResourceClaimTemplate:
  the controller creates one template per side, references it via
  `Pod.Spec.ResourceClaims[].ResourceClaimTemplateName`, and K8s
  expands to one ResourceClaim per Pod replica on admission. This
  satisfies the plan literal — per-replica claims do exist; they're
  just created by K8s on demand, not the inference-operator. DESIGN
  .md §4.2.2 documents this design choice.
- **Text duplication of DeviceClass name + annotation keys**. Per
  operators/CLAUDE.md §1 ban on cross-module Go imports, `claim_
  builder.go` re-declares `NPUDeviceClassName` and
  `AnnotationModelServiceRef` / `AnnotationPreferredPool` as constants
  rather than importing npu-dra-driver's `v1alpha1` package. Same
  rationale that motivated suite_test.go's textual util duplication.

## Debugging trail

- **First-draft import path typo**. Pasted
  `github.com/tech88-art/api/v1alpha1` in `deployment_builder.go`
  instead of the full
  `github.com/tech88-art/O-Cloud/operators/inference-operator/api/v1alpha1`
  — go build caught it immediately. Fixed.
- **Test scheme needed apps/v1 + resource/v1beta1**. fake client
  errors with "no kind registered" when trying to Create a
  Deployment or ResourceClaimTemplate not in the scheme. Added both
  to `suite_test.go newTestScheme`.
- **`controllerutil.SetControllerReference` requires the parent type
  to be registered in the scheme**. ModelService is, so the
  SetControllerReference call works for both child kinds.

## Key decisions

- **Drift detection is field-level, not deep-equal of whole spec**.
  The Reconcile compares Replicas / Containers / ResourceClaims
  individually so unrelated mutation (label additions by humans,
  status subresource churn) doesn't trigger a spurious Update.
  Image change → container drift → Update → rolling restart.
- **ResourceClaimTemplate.Spec is immutable upstream**. The
  template-update path only refreshes Labels (mutable); Spec changes
  require delete + recreate. T008 may add that path if a ModelService
  spec change demands a fresh template.
- **PodResourceClaim name = `"npu-slice"` constant**. Single alias
  per pod — Container.Resources.Claims references it by name. If
  Phase 6 introduces multi-claim pods (e.g. separate compute + comm
  NPU slices), we'd vary this; for Phase 5 single-NPU-per-pod the
  fixed name keeps the diff minimal.
- **`--pd-role=<side>` container arg**. Lets the vllm-ascend
  container distinguish Prefill from Decode without reading the
  pd-role label off downward API. Phase 6 may rework this via
  vllm-ascend's `disaggregated_prefill_v1` flag once that lands
  upstream.

## Verification

P3 三维度:
- **Existence**:
  - `git status` shows: 2 new files (deployment_builder.go,
    claim_builder.go), 3 modified (modelservice_controller.go,
    modelservice_controller_test.go, suite_test.go, DESIGN.md), devlog
- **Completeness**:
  - `go build ./...` → clean
  - `go vet ./...` → clean
  - `go test ./internal/controller/... -v -timeout 120s` →
    11 tests pass (5 T006 + 5 T007 + 1 helper)
- **Correctness**: Plan acceptance bullets
  - Each Deployment carries `pd-role=prefill|decode` label ✓
    (TestReconcile_T007_CreatesPDDeploymentsAndClaimTemplates)
  - Per-replica ResourceClaim created with DeviceClassName + model-
    service-ref + preferred-pool annotations ✓ (same test verifies
    template Spec.Spec.Devices.Requests[0].DeviceClassName + Spec
    .ObjectMeta.Annotations)
  - Pod template references claim via ResourceClaimTemplateName ✓
  - Deployment OwnerRef points at ModelService ✓
  - replica-count change scales in-place ✓
    (TestReconcile_T007_ReplicaCountChange_ScalesInPlace)
  - image change rolling update ✓
    (TestReconcile_T007_ImageChange_UpdatesContainer)
  - Bonus: prefill-only / decode-zero-replicas variant ✓
  - Bonus: idempotency across 3 reconciles ✓
  - 5 test cases as required by plan acceptance ("5 cases: prefill-
    only / decode-only / both / replica-count change / image change")

## Carry-forward

- T008 phase machine wires ConditionAllocationReady=True when every
  per-replica ResourceClaim reports Ready=True (read via the watch
  list). The Deployment Status.ReadyReplicas signals Available=True
  for phase=Ready.
- T008 expands the deletion-drain to wait for Deployments + claim
  templates to be GC'd via OwnerRef cascade. T007's children all
  have correct OwnerReferences, so the GC path is in place; T008
  just needs to wait on it.
- T103 webhook reads `inference.ocloud.edge.example.com/model-service`
  Pod label (set by T007 Deployment) to identify the originating
  ModelService and compose the slice-bindings annotation.
- T106 kind smoke creates a ModelService and asserts both Deployments
  + claim templates exist within 30s; per-replica claims allocate
  via npu-dra-driver T002 (real status.allocation writes).
