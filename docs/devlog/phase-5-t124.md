# Phase 5 · T-124 · inference-operator label value: `<ns>/<name>` → `<name>` (K8s label-value regex)

**Date**: 2026-05-20
**Trigger**: kind smoke CI iteration after T123

## Failure observed

T123's RBAC fix unblocked the controller's cache informer; CI advanced
past "install inference-operator chart" and the ModelService fixture
was created. The next step (wait for status.phase to leave Pending)
timed out after 30s with the controller logging this Reconcile error
in a tight retry loop:

```
ERROR Reconciler error
  {"controller":"modelservice",
   "ModelService":{"name":"smoke-ms","namespace":"ocloud-system"},
   "error":"reconcile children: ResourceClaimTemplate.resource.k8s.io
     \"smoke-ms-prefill-claim\" is invalid:
     [metadata.labels: Invalid value: \"ocloud-system/smoke-ms\":
      a valid label must be an empty string or consist of alphanumeric
      characters, '-', '_' or '.', and must start and end with an
      alphanumeric character (e.g. 'MyValue', or 'my_value', or '12345',
      regex used for validation is
      '(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?'),
      spec.metadata.labels: Invalid value: \"ocloud-system/smoke-ms\":
      a valid label must be ..."
```

## Root cause

`claim_builder.go:53` constructed:

```go
msRef := ms.Namespace + "/" + ms.Name  // e.g. "ocloud-system/smoke-ms"
```

and used the same value for both:
- the annotation `ocloud.edge.example.com/model-service-ref` (where `/`
  in values is legal), AND
- the label `inference.ocloud.edge.example.com/model-service` (where
  `/` in values is REJECTED by K8s validation — the regex
  `(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?` permits only
  alphanumerics, `-`, `_`, `.`).

K8s label-value semantics differ from label-key semantics. Keys are
qualified names (`<domain>/<name>`); VALUES are flat tokens. The
project's mental model conflated the two, dating back to P5-T-007
(Phase 5 W1) where the same `msRef` variable was used in both slots.

The bug existed everywhere the controller set `LabelModelService`:
1. `claim_builder.go:64,74` (ResourceClaimTemplate ObjectMeta + Spec.ObjectMeta)
2. `deployment_builder.go:117,128` (Deployment ObjectMeta + PodTemplate)
3. `phases.go:212` (countClaimsAllocated comparison)
4. `pd_router.go:115` (webhook lookup against Pod label)

Unit tests passed because they used the same `"ns/ms-1"` string on both
sides of the comparison; envtest passed because the test pods were
posted through the webhook HTTP path (Decode-only) rather than created
via the API server's validation chain — the test never hit the actual
ValidateLabels path.

## Why tests didn't catch it

- `phases_test.go` constructs `ResourceClaim` fixtures in memory and
  passes them to `countClaimsAllocated` directly. No K8s API → no
  label validation.
- `pd_router_test.go` constructs Pod objects in memory, serializes to
  JSON, hand-builds an `admission.Request`. The webhook decodes and
  reads the label — the validation that would normally happen at
  Pod creation time is bypassed because the test simulates the
  admission phase (which runs BEFORE etcd persist & validation).
- `modelservice_controller_test.go` uses controller-runtime's fake
  client, which is essentially a tracker map keyed by GVK+namespace+name.
  It does not run admission/validation webhooks.
- envtest's apiserver DOES validate, but the envtest webhook tests
  only POST to the webhook handler directly; they never CREATE a Pod
  through the apiserver. The validation never fires.

This is a known gap in the test infrastructure. Phase 6+ should add
either (a) a kind-based integration test that actually creates the
Deployments and asserts they admit, or (b) a stand-alone test that
runs `validation.ValidateLabels` on the controller's built labels map
to assert format compliance.

## Fix

Decouple the two encodings:

- **Annotation** (`ocloud.edge.example.com/model-service-ref` on the
  ResourceClaimTemplate.spec) keeps `<namespace>/<name>` — the
  npu-dra-driver claim controller (T002) reads it and copies onto
  `NPUSliceAllocation.spec.modelServiceRef`. This is the
  cross-controller stable identifier and survives namespace lookups.

- **Label** (`inference.ocloud.edge.example.com/model-service`)
  carries just `ms.Name`. Namespace is implicit from the labelled
  object's own `.metadata.namespace` (all places the label is set —
  ResourceClaimTemplate, Deployment, PodTemplate — live in
  `ms.Namespace`).

- **PD Router webhook** reconstructs the qualified ref at lookup:

  ```go
  nameVal := pod.Labels[LabelModelService]
  msRef := req.Namespace + "/" + nameVal
  bindings, err := h.listBindings(ctx, msRef)
  ```

  so the comparison against `NPUSliceAllocation.spec.modelServiceRef`
  (which the claim controller copied from the annotation) still works.

- **phases.go `countClaimsAllocated`** drops the namespace+name
  concatenation; compares the label directly to `ms.Name`. Safe
  because the caller (`modelservice_controller.go:243`) already passes
  `client.InNamespace(ms.Namespace)` when listing.

## Files touched

- `operators/inference-operator/internal/controller/claim_builder.go`
- `operators/inference-operator/internal/controller/deployment_builder.go`
- `operators/inference-operator/internal/controller/phases.go`
- `operators/inference-operator/internal/webhook/pd_router.go`
- `operators/inference-operator/internal/controller/modelservice_controller_test.go` (assertion `ns-pd/ms-pd` → `ms-pd`)
- `operators/inference-operator/internal/controller/phases_test.go` (`msRef` → `ms-c`)
- `operators/inference-operator/internal/webhook/pd_router_test.go` (all pod labels `ns/ms-1` → `ms-1`)
- `operators/inference-operator/internal/webhook/pd_router_envtest_test.go` (3 pods)

## Verify

Local:
- `go build ./...` clean
- `go vet ./...` clean
- `go test ./internal/controller/... ./internal/webhook/... -count=1` →
  both packages pass (controller 0.807s, webhook 0.487s)

K8s rule we satisfied:
> A valid label value must be 63 characters or less (can be empty),
> must begin and end with an alphanumeric character ([a-z0-9A-Z]),
> with dashes (-), underscores (_), dots (.), and alphanumerics
> between.

`smoke-ms` passes; `ocloud-system/smoke-ms` does not.

## Expected next-CI flow

ResourceClaimTemplate admits → Deployments admit → Pods admit (the
mutating webhook now sees label `smoke-ms`, reconstructs
`ocloud-system/smoke-ms`, lists NPUSliceAllocations) → status.phase
advances from Pending → assert step's 30s wait satisfied.

If a subsequent step fails it will be downstream of object admission
(allocation phase progress, deployment rollout readiness, status
condition aggregation).
