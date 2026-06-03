# `tests/sla/` — vLLM PD P99 latency SLA harness

> Load-test harness + **documented default SLO** for the vllm-ascend PD
> inference path (P13-T-206 · [ADR-0025](../../docs/adr/0025-production-hardening-architecture.md)
> §2 Decision E + §4(b) swap point). Pairs with the controller-side
> latency-aware scaling in `operators/inference-operator` (the scaler scales UP
> when measured P99 breaches the SLO).

## Default SLO (REFERENCE · customer swaps)

| Metric | Default | Where to swap |
|---|---|---|
| **P99 end-to-end request latency** | **2000 ms** | harness: `-slo-ms` / `SLO_MS` env · controller: `NPUVERTICAL_SCALER_P99_SLO_MS` env (chart) · code default: `metrics.DefaultP99SLOMillis` |

> **2000ms is a deliberately loose REFERENCE** for an 8B PD model on a single
> 910B — NOT a customer SLA contract. A real SLO is workload + token-budget +
> hardware specific. The customer sets the real number per ADR-0025 §4(b); every
> consumer above reads it from one swappable place.

## Run (lab · against a real PD endpoint)

```bash
# Port-forward or in-cluster Service DNS to the vllm-ascend PD endpoint:
TARGET_URL=http://qwen-pd.ocloud-system.svc.cluster.local:8000/v1/completions \
  go run ./tests/sla -n 500 -c 20 -slo-ms 2000

# Flags (all have env fallbacks): -url/TARGET_URL -n/REQUESTS -c/CONCURRENCY
#   -slo-ms/SLO_MS -method/METHOD -body/BODY -timeout/TIMEOUT_S
```

Exit code: **0** when P99 ≤ SLO and 0 errors; **1** on breach/errors (so it
gates a lab run / CI-against-lab job); **2** when `-url` is unset.

Sample output:

```
== P99 SLA load test ==
  url=http://qwen-pd...:8000/v1/completions
  requests=500 concurrency=20 slo=2000ms

  ok=500 errors=0
  p50=410.2ms p90=980.5ms p99=1734.8ms max=2010.1ms

PASS: P99 1734.8ms <= SLO 2000ms (0 errors)
```

## Self-check (offline · no endpoint)

The percentile math runs in CI-style locally without any endpoint:

```bash
go test ./tests/sla        # percentile_test.go — nearest-rank P50/P90/P99 + Summarize
go build ./tests/sla       # harness compiles (stdlib-only · arm64-clean)
```

## Controller-side (server P99 · what the scaler reads)

The operator does NOT compute P99 client-side — it queries the **server-side**
P99 from the vllm-ascend latency histogram via Prometheus (the harness above
*drives* the load that fills that histogram). The scaler's query
(`operators/inference-operator/internal/metrics/latency.go`):

```promql
histogram_quantile(0.99,
  sum(rate(vllm:e2e_request_latency_seconds_bucket{model_service="<ms>"}[5m])) by (le))
```

When this exceeds `NPUVERTICAL_SCALER_P99_SLO_MS` the scaler overrides a
stay/idle decision to **busy** (scale up to defend the SLO · never scales down
on a breach).

### Grafana panel

`deploy/grafana-dashboards/` ships no inference dashboard (dashboards are
provisioned via kube-prometheus-stack at deploy time), so no JSON panel is
checked in here. To add a P99 SLA panel, use the PromQL above as the panel
query with a threshold line at the SLO; the metric is already scraped once the
vllm-ascend ServiceMonitor is in place.

## Notes

- **stdlib-only · own `go.mod`** — builds/runs on the arm64 lab node without the
  operator module graph; not in the CI Go matrix (lab-driven tool · its
  percentile unit test is the offline correctness gate).
- **Real P99 measurement is LAB-GATED** (needs real 910B + a served model · plan
  §8); the offline layer proves the harness + percentile logic + the controller
  query shape.
