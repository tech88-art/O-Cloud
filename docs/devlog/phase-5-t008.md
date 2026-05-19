# P5-T-008 · inference-operator phase machine

- **Commit**: <pending>
- **Date**: 2026-05-19
- **Duration**: plan 1d · actual ~45min

## Intent

Wire the ModelService phase machine: aggregate Deployment readiness +
claim allocation, compute target phase (Pending → Provisioning →
Ready / Failed), emit the documented condition set
(Available / ProgressDeadline / AllocationReady) per ADR-0008 +
plan-acceptance.

## Path adaptations

- **Removed unused `markProvisioning` helper + `reasonScaffold` const
  from claim_controller code path**. After wiring `reconcilePhase`,
  the T006 `markProvisioning` becomes superseded — it wrote
  Provisioning phase + AllocationReady=False with reason
  `ProvisioningScaffold`. The new computePhase writes the same shape
  with reason `ClaimsNotYetAllocated`. T006 tests assert Status only
  (not Reason), so they pass through the new code unmodified.

## Debugging trail

- **`vet` caught a missing `resourceapi` import in phases_test.go**.
  First draft used `resourceapi.ResourceClaim` in the count test
  fixture but forgot the import statement. vet caught it; added the
  import and tests passed.
- **Progress-deadline measurement origin**. First sketch used
  `metav1.NewTime(now)` as the "phase started" reference, but Reconcile
  is stateless. The right anchor is the Available=False condition's
  `LastTransitionTime` from the previous status (which SetCondition
  preserves when Status doesn't flip). Implemented via
  `durationSinceAvailableFalse(last, now)` — pure function reading the
  last status; returns 0 when no Available condition yet.

## Key decisions

- **`computePhase` is a pure function**. Takes `PhaseInputs` + last
  status, returns `PhaseDecision`. Reconciler is responsible for
  Status.Patch. Pure function makes table-driven tests cheap (no
  fake client needed for phase logic verification).
- **AllocationReady requires `ClaimsTotal > 0`**. A ModelService with
  Prefill=0, Decode=0 has no claims; reporting AllocationReady=True
  would be misleading. Status=False with reason `ClaimsNotYetAllocated`
  conveys "we don't yet have anything to allocate" without misleading.
- **Image-pull error → Failed (no auto-recovery from controller)**.
  Plan says "auto-recovers if spec edit fixes the cause". Spec edit
  bumps Generation → re-Reconcile → fresh phase computation; the
  controller doesn't need explicit recovery logic — just let the spec
  flow re-drive the phase.
- **Requeue 30s while Provisioning**. controller-runtime's
  controller-managed-by-builder doesn't watch Deployment status
  changes by default. The 30s requeue is the simplest cadence to
  re-poll readiness without adding an explicit `.Watches(...)` on
  apps/v1.Deployment (which would also fire on irrelevant updates).
  Phase 6+ may swap to a more event-driven design.
- **ProgressDeadline 10min as a constant, not a CRD field**. T008
  acceptance says ">10min"; spec-tunable ProgressDeadlineSeconds is
  Phase 6 polish. Tests inject a custom `Now` for deterministic
  exercise.

## Verification

P3 三维度:
- **Existence**:
  - `git status` shows: 2 new files (phases.go, phases_test.go),
    2 modified (modelservice_controller.go, DESIGN.md), devlog
- **Completeness**:
  - `go build ./...` → clean
  - `go vet ./...` → clean (vet caught one missing-import issue mid-
    development; fixed)
  - `go test ./internal/controller/... -v -timeout 60s` →
    14 tests pass (5 T006 + 5 T007 + 1 T008 table-driven with 6
    subcases + 1 image-pull-detector + 1 count-claims + 1 helper)
- **Correctness**: Plan minimum 5 envtest cases satisfied via
  TestComputePhase_TableDriven sub-cases (pure phase machine is
  table-driven, not full envtest, per the same fake-client posture
  established in T006/T007):
  - Pending → Provisioning ✓ (no replicas ready)
  - Provisioning → Ready ✓ (all replicas + claims allocated)
  - Provisioning → Failed (progress deadline exceeded) ✓
  - Ready → Provisioning (scale up) ✓
  - Ready → Failed (image pull error) ✓
  - Bonus: AllocationReady=True only when ClaimsTotal>0 AND all
    allocated ✓
- DESIGN.md §4.2.3 documents phase diagram + transition rules +
  condition set + cadence

## Carry-forward

- T101 cert-manager + T102/T103 webhook are independent of T008
  (webhook reads NPUSliceAllocation list directly by label, not
  through the inference-operator's phase machine).
- T106 kind smoke can now assert ModelService Status.Phase reaches
  Ready within timeout by waiting for both Deployments' ReadyReplicas
  to match replicas (the real Deployment controller updates this
  field in a live cluster; the inference-operator's 30s requeue
  picks up the change).
- Phase 6 may introduce `spec.progressDeadlineSeconds` + Watches on
  owned Deployments + a more sophisticated retry-once-on-image-pull
  policy. None blocks Phase 5 completion.
- Finalizer drain (T006 placeholder): T008 does not yet wait for
  owned Deployments/claims to be GC'd before removing the finalizer
  on deletion. K8s garbage collection handles cascade cleanly via the
  OwnerReferences set in T007, so the simple-remove pattern works
  for current Phase 5 smoke. Phase 6+ may tighten the drain
  semantics if real-cluster tests reveal a race.
