# P4-T-006 · npu-dra-driver ResourceClaim controller skeleton

- **Commit**: d4fc356
- **Date**: 2026-05-19
- **Duration**: plan 1d · actual ~1.5h

## Intent

Land a ClaimReconciler that watches upstream `resource.k8s.io/v1beta1.ResourceClaim`, filters by DeviceClassName prefix (`npu.ocloud.edge.example.com`), and records an "AllocationDeferred=Phase4Skeleton" condition + Event for matching claims. NO real allocation (Phase 5 deliverable per ADR-0001 v3 §7).

## Path adaptations

None on paths. One major schema-drift adaptation (next section).

## Debugging trail

- **Schema-drift discovery — `ResourceClaimStatus` has no Conditions field**. Plan literal acceptance asks: `sets status.conditions[Type=AllocationDeferred]=True with Reason=Phase4Skeleton and Message=...`. After writing the controller + tests + compiling, the tests failed with `claim.Status.Conditions undefined` errors. Investigated `k8s.io/api@v0.35.0/resource/v1beta1/types.go` line 1448-1510 — `ResourceClaimStatus` only carries:
  - `Allocation *AllocationResult`
  - `ReservedFor []ResourceClaimConsumerReference`
  - `Devices []AllocatedDeviceStatus`
  
  The only `Conditions` field is on `AllocatedDeviceStatus` (per-device, presumes allocation occurred). The v1 GA path has the same structure.

  **Resolution**: encoded the deferred state via `metadata.annotations` instead — 4 keys (`ocloud.edge.example.com/allocation-deferred[-reason|-message|-observed-generation]`) + Kubernetes Event for ops visibility. Documented the deviation extensively inline in claim_controller.go's "Schema-drift note" comment block + in commit footer. Phase 5 controller body should lift to `status.devices[].conditions` once real allocation exists.

- **metav1.Time second-resolution test flake**. Wrote `TestSetCondition_TransitionTime` testing that SetCondition's same-status update preserves LastTransitionTime, status-flip bumps it. Used `metav1.Now()` for both timestamps → fails because RFC 3339 truncates to seconds and sub-second test execution returns identical times. Fixed by planting an explicit `metav1.Time{Time: metav1.Now().Add(-1 * time.Hour)}` for the "old" timestamp.

- **Suite_test.go format choice**. Plan: "mirrors pool-operator/internal/controller/suite_test.go shape". pool-operator's suite is Ginkgo + envtest with kube-apiserver binaries. For Phase 4 with no envtest binaries in this Windows shell, I used a plain testing-package harness with `fake.NewClientBuilder().WithStatusSubresource(...)`. Documented the framework deviation in suite_test.go header (~30 line rationale block including 4 numbered points).

## Key decisions

- **isOurClass prefix-matching policy**. Phase 5 will register the DeviceClass; Phase 4 doesn't know its exact name. Resolved by matching: exact equality with `v1alpha1.DriverName` OR prefix `<DriverName>/` (slash sub-class) OR prefix `<DriverName>.` (dot sub-class). Tested with 6 sub-cases: exact / slash-subclass / dot-subclass / unrelated / empty / look-alike. Covers all reasonable Phase 5 class-naming evolutions.
- **Patch over Update**. Uses `client.MergeFrom(claim.DeepCopy())` then `r.Client.Patch(...)` so concurrent annotation mutations by other actors (kubectl edit, inference-operator) don't get clobbered. Slightly more expensive than Update but safer.
- **NotFound on Get = deletion path, return nil**. Phase 4 holds no finalizer (per plan literal: "no finalizer (Phase 4 has nothing to clean up)") → vanished claims yield a clean exit. Tested via `TestClaim_Deletion`.

## Verification

P3 三维度:
- Existence: `git ls-files operators/npu-dra-driver/internal/controller/` → 4 files
- Completeness: `go test ./internal/controller/... -v -timeout 60s` → 12 sub-tests PASS (4 plan-named + 6 prefix-matching + 2 utils sanity)
- Correctness: live `go build` clean; `./bin/manager --help` shows updated `--enable-claim-controller` description (no longer "Reserved (P4-T-006)")

## Carry-forward

- T101 helm chart's ClusterRole pre-grants `resourceclaims/status: update,patch` even though Phase 4 controller writes only to `metadata.annotations`. Pre-granting avoids a chart bump when Phase 5 lifts to status writes.
- T105 ADR-0009 §5 item 6 records the schema-drift note as a known Phase 5 cleanup target.
- Phase 5 implementation note: deletion of annotation keys when claim allocation succeeds is the Phase 5 controller's job. Phase 4 leaves them in place by design.
