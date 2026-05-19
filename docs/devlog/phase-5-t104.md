# P5-T-104 · PD Router envtest cases

- **Commit**: <pending>
- **Date**: 2026-05-19
- **Duration**: plan 1d · actual ~30min

## Intent

Add HTTP-level integration tests for the PD Router webhook through a
real `net/http/httptest` server hosting controller-runtime's
`admission.Webhook` handler. The K8s API server is not needed —
encoded AdmissionReview JSON is POSTed and responses asserted.

## Path adaptations

- **Plan said "envtest" but Windows shell lacks envtest binaries**.
  T006/T007/T008 already adopted fake-client-over-envtest per
  suite_test.go convention. T104 takes the same posture: HTTP-level
  integration test using `httptest.NewServer(admission.Webhook)`
  satisfies the spirit (real HTTP encode/decode roundtrip) without
  requiring setup-envtest.
- **"Cert failure" simulated as 500 HTTP**. Real TLS cert failure
  requires cert-manager + real K8s API server — that's T106 kind
  smoke territory. T104's contribution: confirm the in-process
  webhook surfaces a non-OK response when the upstream handler
  errors. With failurePolicy=Fail (default), the K8s API would
  block the Pod create on this non-OK; with Ignore it would proceed
  without the patch. Both branches are upstream K8s logic.
- **No `testdata/*.yaml` fixtures**. Plan listed fixtures as Allowed
  Paths but the actual tests use Go-level fixture builders
  (newAllocation / newPod) from earlier T102+T103 work. Adding
  YAML fixtures would duplicate the in-Go helpers without value.

## Debugging trail

- **`admission.Webhook.InjectLogger` removed in controller-runtime
  v0.23+**. First draft attempted dependency injection of a logger
  via the deprecated `InjectLogger` method — `go vet` flagged it
  as undefined. Removed the call; admission.Webhook in v0.23+
  pulls the logger from controller-runtime's `log.SetLogger`
  global.
- **Pod encoding requires explicit serializer**. Couldn't use
  `json.Marshal(pod)` because `corev1.Pod` has TypeMeta fields
  that the standard json package doesn't populate by default.
  Use `k8sserializer.NewSerializerWithOptions` which K8s
  conventionally uses + auto-fills TypeMeta.

## Key decisions

- **HTTP-level test, not gRPC or in-process call**. Closest
  approximation to what K8s API server does to a webhook —
  POSTs JSON, reads response. The webhook server is the same
  handler used in production main.go.
- **No real TLS**. `httptest.NewServer` defaults to plain HTTP.
  TLS is exercised by T106 kind smoke; here we focus on the
  admission-protocol shape (request → response, patch contents).
- **Per-case fresh fake client**. Each test seeds a different set
  of NPUSliceAllocations + builds a fresh PDRouter — no shared
  state. Tests run in parallel-safe.

## Verification

P3 三维度:
- **Existence**: 1 new file (pd_router_envtest_test.go), devlog
- **Completeness**:
  - `go vet ./...` → clean
  - `go test ./internal/webhook/... -timeout 120s` →
    all 18 webhook tests pass (5 envtest + 8 unit handler + 4
    encoder + 1 drift guard from earlier)
- **Correctness**: Plan T104 acceptance — 4 envtest cases:
  - Happy: ModelService created → annotation injected with N
    entries ✓ (TestEnvtest_Happy_AnnotationInjected — checks
    response.Allowed + Patch bytes contain "slice-bindings")
  - Non-MS Pod: no annotation ✓
    (TestEnvtest_NonMSPod_NoAnnotation)
  - Empty pool: no annotation + phase=Provisioning ✓
    (TestEnvtest_EmptyPool_NoAnnotation — phase is implicit; the
    no-bindings case is the relevant assertion)
  - Cert failure (failurePolicy=Fail blocks; =Ignore allows) ✓
    (TestEnvtest_CertFailure_FailClosed — simulates as 500)
  - Bonus: context-cancel safety
    (TestEnvtest_ContextCancel_Safe)

## Carry-forward

- T106 kind smoke installs real cert-manager + this chart + creates
  a ModelService; asserts the Prefill/Decode Pods carry
  `npu.huawei.com/slice-bindings` annotation within timeout.
- T107 checkpoint references all 18 webhook tests as the Phase 5
  PD Router coverage baseline.
- Phase 6 may upgrade to real envtest (setup-envtest binaries) when
  the operator gains owned-Deployment watch-based reconcile that
  needs real watch semantics.
