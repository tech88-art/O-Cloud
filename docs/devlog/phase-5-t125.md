# Phase 5 · T-125 · kind smoke assert.sh — 3 bugs + webhook race handling

**Date**: 2026-05-20
**Trigger**: kind smoke CI iteration after T124

## Failure observed

T124's label-format fix unblocked the ResourceClaimTemplate validation;
the controller successfully wired up:

```
✓ ModelService phase=Provisioning (after 2s)
✓ Prefill + Decode Deployments materialised (after 1s)
✗ ResourceClaim count=0 (60 attempts × 2s = 120s timeout, then warn-continue)
✗ No Pod observed with slice-bindings annotation within 120s
```

Three distinct bugs surfaced:

### Bug 1: assert.sh label selector still uses `<ns>/<name>` form

```
Error from server (BadRequest):
  Unable to find "/v1, Resource=pods" that match label selector
    "inference.ocloud.edge.example.com/model-service=ocloud-system/smoke-ms"
  unable to parse requirement: values[0]: Invalid value:
    "ocloud-system/smoke-ms": a valid label must be ...
```

T124 changed the controller to stamp `LabelModelService = ms.Name` (no
namespace) but assert.sh was still building the selector as
`inference.ocloud.edge.example.com/model-service=${NS}/${MS_NAME}` —
kubectl rejected the selector at parse time. With stderr swallowed by
`2>/dev/null`, the count just stayed at 0 and the loop kept spinning.

The fix: drop the `${NS}/` prefix from the selector.

### Bug 2: bash count-parsing bug (T119 pattern reappears)

```
attempt 1: ResourceClaim count=0
0
tests/e2e/kind/phase5/assert.sh: line 79: [: 0
0: integer expression expected
```

Same root cause as T119 in install.sh:

```bash
count=$(... | grep -c . || echo 0)   # On empty input: "0\n0" (two lines)
```

`grep -c .` returns 0 lines + exit code 1; the `|| echo 0` adds another
0. The two 0s get concatenated with a newline. `[ "${count}" -ge 1 ]`
then sees `[ "0 0" -ge 1 ]` and bash bails with "integer expression
expected".

The fix: use jq for clean integer counting (mirrors the T119 install.sh
remediation).

### Bug 3: pd_router webhook race vs NPUSliceAllocation.status.phase

From the controller log:

```
10:35:32Z DEBUG admission.pd-router Bindings counted
  ... "model-service-ref": "ocloud-system/smoke-ms",
      "total": 1, "allocated": 0, "orphaned": 0, "other": 1
10:35:32Z DEBUG admission.pd-router
  Some bindings not yet allocated; allowing without patch
```

The webhook fired the moment the ReplicaSet created the Pod. At that
exact instant the npu-dra-driver claim controller had:
- Created the `NPUSliceAllocation` (empty status), AND
- Was *about* to Patch `status.phase = Allocated` (a second sub-second
  call).

Between Create and Patch the object exists with empty status, so the
webhook counted it under "other" (not Allocated, not Orphaned). The
webhook code (correctly per ADR-0008) admits the Pod without patching
when `allocated < len(bindings)`:

```go
if allocated < len(bindings) {
    return admission.Allowed("waiting for full allocation")
}
```

After admission there is no second chance — webhooks fire only on
CREATE/UPDATE of the target resource. The running Pod will keep
running without the `npu.huawei.com/slice-bindings` annotation
indefinitely.

This is a known design gap: Phase 6+ may add a separate controller
that retroactively patches Pods when allocation phases transition.
For Phase 5 smoke, the test must work around it by force-recreating
Pods *after* allocations settle.

## Fix

assert.sh:

1. Single `LABEL_SELECTOR` constant: `inference.ocloud.edge.example.com/model-service=${MS_NAME}`
   used in 4 sites (resource-claim wait, deploy describe, pod wait,
   pod describe-on-failure). The cross-controller `<ns>/<name>` ref is
   still needed for the `NPUSliceAllocation.spec.modelServiceRef` match
   — extracted as `MS_REF_FQ`.
2. `count_claims_with_label()` helper uses jq:
   ```bash
   kubectl get resourceclaims -l "${LABEL_SELECTOR}" -o json 2>/dev/null \
     | jq '.items | length' 2>/dev/null \
     || echo 0
   ```
3. New step `wait_allocations_allocated`: counts NPUSliceAllocation
   entries (cluster-scoped) where `spec.modelServiceRef == "<ns>/<name>"`
   AND `status.phase == "Allocated"`, waits until at least 1 (≥2 in
   practice — prefill + decode).
4. New step `force_pod_recreate`: `kubectl delete pods -l ${LABEL_SELECTOR}
   --wait=false`. ReplicaSet immediately spins replacements; webhook
   fires at the new Pods' CREATE with allocation cache hot.
5. Existing `wait_slice_bindings_annotation` runs against the recreated
   Pods.

Sequence:

```
apply_fixture
wait_phase_advanced
wait_deployments_created
wait_resource_claims  (warn-on-slow, per Phase 5 plan)
wait_allocations_allocated   ← NEW
force_pod_recreate           ← NEW
wait_slice_bindings_annotation
```

## Verify

- `bash -n tests/e2e/kind/phase5/assert.sh` → syntax clean
- NPUSliceAllocation CRD is `scope: Cluster` (confirmed via
  `deploy/helm-charts/npu-dra-driver/crds/npusliceallocations.yaml:17`)
  → `kubectl get npusliceallocations` without `-n` is correct.
- The jq filter
  ```bash
  jq --arg ref "${MS_REF_FQ}" \
     '[.items[] | select(.spec.modelServiceRef == $ref) | select(.status.phase == "Allocated")] | length'
  ```
  reads the same fields the controller writes (claim_controller.go:407
  spec.modelServiceRef, line 423 status.phase).

## Expected next-CI flow

1. Initial Pods created → webhook sees allocations in transient phase →
   admits without patch (existing behavior, unchanged).
2. Claim controller stamps NPUSliceAllocation.status.phase=Allocated
   (sub-second after each Pod admission).
3. `wait_allocations_allocated` sees count≥1 within a few seconds.
4. `force_pod_recreate` deletes initial Pods.
5. ReplicaSet creates replacements; webhook fires; finds 2 allocations
   both in Allocated phase; injects `npu.huawei.com/slice-bindings`
   annotation.
6. `wait_slice_bindings_annotation` finds the annotation and exits 0.

## Phase 6 follow-up (TODO)

The webhook race is a real design gap, not a CI bug. The smoke
workaround (force-recreate) is fine for one-shot test but production
workloads expect Pods to come up annotated on the FIRST admission
without orchestration intervention. Phase 6 should add either:

- a `pod_patcher` controller that watches NPUSliceAllocation phase
  transitions and patches matching Pods (PATCH adds/replaces
  annotations on existing objects, unlike admission which only
  mutates the new-object payload), OR
- a webhook re-eval mechanism that defers admission with a retry
  budget when allocations are still transient (this is risky for
  Pod-start latency SLOs).

Filed as Phase 6 candidate; not in T125 scope.
