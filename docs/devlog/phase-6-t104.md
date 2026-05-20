# P6-T-104 · inference-operator Prometheus metrics

- **Commit**: <pending>
- **Date**: 2026-05-20
- **Duration**: plan 1d · actual ~35min

## Intent

Land 3 Prometheus collectors on the inference-operator manager,
exposed via the existing controller-runtime metrics server. Chart
adds a separate metrics Service + opt-in ServiceMonitor so Prometheus
Operator-equipped clusters can scrape automatically.

## Path adaptations

- **Plan scope reduced from 4 → 3 collectors**: plan acceptance
  itself called out the "allocator picks" counter as dropped because
  inference-operator doesn't call the allocator directly (DRA
  ResourceClaim machinery handles it). The 3 collectors I shipped:
  `phase_transitions_total`, `pdrouter_decisions_total`,
  `reconcile_duration_seconds`. The 4th (allocator picks) is a
  Phase 7+ candidate for the npu-dra-driver module.

- **`metricsBindAddress` stays `:8082`** (existing default), not
  `:8081` as plan suggested. The existing inference-operator binary
  already uses `:8082` for metrics and `:8081` for health probes;
  swapping breaks operator muscle memory and existing operator
  documentation. Plan port number was incidental.

- **Webhook decision recording via deferred wrap**: instead of
  threading `metrics.RecordWebhookDecision(...)` through every of
  the 7+ return points in `pd_router.Handle`, refactored Handle
  into a thin outer that defers `recordDecisionMetric(resp)` then
  calls a renamed `handle()` inner returning the response. Inspector
  branches on `resp.Allowed` / `resp.Patches != nil` to map to the
  3 decision label values. One-shot wiring, all existing return
  paths unchanged.

- **Separate metrics Service**: existing `service.yaml` only exposes
  the PD Router webhook port (443→9443) and is gated by
  `pdRouter.enabled`. A new `service-metrics.yaml` (gated by
  `metrics.enabled`) exposes port 8082 — operators can scrape
  metrics even if they disable the webhook (e.g. during phased
  rollout).

- **`logr.Logger` import added to pd_router.go**: needed because
  the new inner `handle()` function takes `lg logr.Logger`. The
  existing imports use `log.FromContext` from
  `sigs.k8s.io/controller-runtime/pkg/log` which returns
  `logr.Logger` already; just had to import the type for the
  parameter signature.

## Debugging trail

- **`go mod tidy` after adding prometheus import**: clean, no new
  external deps (prometheus client_golang already pulled in via
  controller-runtime).

- **First `go vet` passed**: no surprises in the metrics package.

- **All 4 metrics tests + 14 controller tests + 18 webhook tests
  PASS**: 36 total inference-operator tests pass after the wiring.

- **helm template render check**: with metrics.enabled (default
  true) the chart now renders 9 K8s objects (was 8 in Phase 5 — added
  the metrics Service). ServiceMonitor opt-in path verified by
  `--set metrics.serviceMonitor.enabled=true`.

## Key decisions

- **Histogram, not Summary**: histograms aggregate across multiple
  replicas at scrape time via Prometheus _sum/_count math; summaries
  require client-side quantile estimation that doesn't aggregate.
  When inference-operator scales to >1 replica (HA), histograms
  give cleaner SLO calculation.

- **Bucket boundaries: 1ms → 10s**: covers the expected Reconcile
  duration distribution. Most reconciles run in single-digit ms
  (Get + Patch path); longer paths happen on resource creation +
  pool resolution + child reconciliation. 10s upper bucket catches
  pathological cases without bloating histogram cardinality.

- **`recordDecisionMetric` infers from response, not label arg**:
  the alternative (capture decision label string before each return)
  was more code + bug-prone. The current inspector branches on
  `resp.Allowed` + presence of patches; behavior is deterministic +
  any future return value automatically classifies correctly.

- **PhaseTransitions guarded against same-phase recomputes**: `if
  string(base.Status.Phase) != string(decision.Phase)` wraps the
  call. Recompute-same-phase happens every Reconcile tick when
  Provisioning re-evaluates Deployment readiness — recording each
  would inflate the counter ~360× per hour per ModelService.

- **ServiceMonitor opt-in default false**: Prometheus Operator may
  not be installed in every cluster; rendering an unknown CRD
  manifest would fail `helm install`. Default false lets the chart
  install everywhere; operators flip the toggle when their cluster
  has the CRDs.

## Verification

P3 三维度:

- **Existence**:
  - `git status --short` →
    - 5 new files: `internal/metrics/metrics.go`,
      `internal/metrics/metrics_test.go`,
      `deploy/helm-charts/inference-operator/templates/service-metrics.yaml`,
      `deploy/helm-charts/inference-operator/templates/servicemonitor.yaml`,
      devlog
    - 5 modified: pd_router.go, modelservice_controller.go, go.mod,
      go.sum, values.yaml, DESIGN.md
  - `grep -c 'inference_' operators/inference-operator/internal/metrics/metrics.go`
    → 3 metric names present

- **Completeness** (plan §4 P6-T-104 acceptance):
  - 3 Prometheus collectors registered ✅ (PhaseTransitions /
    WebhookDecisions / ReconcileDuration)
  - `:8082/metrics` exposed via existing metricsserver ✅
  - Phase transition counter increments per state change ✅
    (TestRecordPhaseTransition + same-phase guard in controller)
  - Webhook decision counter per request ✅ (TestRecordWebhookDecision
    + deferred wrap in Handle)
  - Reconcile duration histogram ✅ (TestObserveReconcileDuration)
  - Service template port 8082 exposed ✅ (service-metrics.yaml)
  - ServiceMonitor template opt-in ✅ (servicemonitor.yaml)
  - values.yaml `metrics.enabled` + `metrics.serviceMonitor.enabled`
    toggles ✅
  - `helm lint --strict` clean ✅
  - `make test` clean ✅ (4 metrics + 14 controller + 18 webhook
    tests pass)

- **Correctness**:
  - `go vet ./...` exit 0
  - `go mod tidy` exit 0
  - `go test ./...` exit 0 (all 36 tests pass)
  - `helm template` default renders 9 K8s objects
    (1 Certificate + 1 ClusterRole + 1 ClusterRoleBinding +
    1 Deployment + 1 Issuer + 1 MutatingWebhookConfiguration +
    2 Services [webhook + metrics] + 1 ServiceAccount)
  - `helm template --set metrics.serviceMonitor.enabled=true`
    renders ServiceMonitor cleanly
  - DESIGN.md §5.1 added (Metrics exposition section)

## Carry-forward

- **T106 kind smoke**: should `curl inference-operator-metrics:8082/metrics`
  + grep for the 3 metric families. Plan §3 P6-T-106 acceptance
  mentions "scrapes /metrics from inference-operator" — that's the
  hook.

- **Phase 7+ allocator metrics**: npu-dra-driver gets its own
  metrics package mirroring this one. Likely candidates:
  - `npudriver_allocator_decisions_total{strategy, outcome}`
  - `npudriver_resourceslice_publish_seconds`
  - `npudriver_allocation_orphan_total`

- **kube-prometheus-stack integration label**: operators using
  kube-prometheus-stack need to add `prometheus: kube-prometheus`
  to `metrics.serviceMonitor.labels` so the bundled Prometheus
  selects this monitor. Documented in DESIGN.md §5.1.

- **Webhook decision label cardinality**: 3 fixed values
  (allowed_no_patch, patched, denied). Adding more in future
  requires care — Prometheus label cardinality is a known
  performance issue.

- **T102 backend**: when `/api/v1/workloads sliceBindings[]` field
  lands, the workloads handler may want its own metrics. Out of
  scope here.
