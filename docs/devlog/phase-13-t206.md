# P13-T-206 · [A5] vLLM PD P99 SLA harness + latency-aware scaling

- **Commit**: this commit (main agent)
- **Date**: 2026-06-03
- **Duration**: plan 2.5d vs actual ~1d (real P99 measurement is lab-gated · T301).

## Intent

Close build-doc §5 item "推理 SLA" per ADR-0025 §2 Decision E: (1) a load-test harness that drives the
vllm-ascend PD endpoint and measures P99 against a **documented default SLO**; (2) controller-side
**latency-aware scaling** — the NPUVerticalScaler scales UP when measured P99 breaches the SLO, not just
on NPU utilization; (3) the default SLO documented with a single customer swap point.

## Design

- **Server-side P99, not client-side, in the operator.** The end-to-end latency histogram is exposed by
  the external vllm-ascend PD proxy (Phase 6 P6-T-105 · `vllm:e2e_request_latency_seconds_bucket`),
  scraped by Prometheus. So the operator does NOT host a histogram — `internal/metrics/latency.go`
  builds a `histogram_quantile(0.99, ...)` PromQL and runs it through the **existing Ingestor's
  CustomPromQL path** (no new Prometheus client). `QueryP99LatencyMillis` converts the histogram's
  seconds → ms. Avoided registering a dead Go histogram nothing observes (M4 no-padding).
- **Latency-aware override is opt-in + scale-UP-only.** New scaler field `LatencySLOMillis` (0 =
  disabled · backward-compatible). Step 4.5: after the utilization decision, if enabled and measured P99
  > SLO, override a stay/idle decision to **busy** — never overrides toward idle (a latency breach must
  not idle the service). Fail-open: no latency signal (NoData/err/nil ingestor) → utilization decision
  stands. Reuses `r.Ingestor` (same Prometheus) — the scaler now issues 2 queries/tick when enabled.
- **`tests/sla/` is a standalone stdlib-only Go module** (own `go.mod`): a concurrent HTTP load
  generator + client-side `Percentile` (nearest-rank) + `Summarize`. Own module so it builds/runs on the
  arm64 lab node without the operator module graph; not in the CI Go matrix (lab-driven tool · its
  `percentile_test.go` is the offline correctness gate). Exit 0 when P99 ≤ SLO + 0 errors → doubles as a
  lab gate. The operator's server-side P99 and the harness's client-side P99 are independent
  computations (no shared code across the module boundary — by design, per operators/CLAUDE.md §1).

## Path adaptations (transparent · P3)

- **`cmd/main.go` not in T206 Allowed Paths**, but the feature has no activation path without it
  (LatencySLOMillis defaults to 0/disabled). Added a 4-line env read
  (`NPUVERTICAL_SCALER_P99_SLO_MS` → `nvsr.LatencySLOMillis`, 0/invalid = disabled). Minimal same-module
  wiring · documented (mirrors the T204 main.go note).
- **No Grafana JSON.** `deploy/grafana-dashboards/` ships NO inference dashboard (dashboards are
  provisioned via kube-prometheus-stack at deploy time). Fabricating a full dashboard JSON would be
  padding (M4); instead the P99 SLA PromQL + threshold guidance is documented in `tests/sla/README.md`
  for operators to add a panel. The plan listed the Grafana path as "if needed" — not needed.

## Verification (offline · plan §8 layer 1)

- operator: `go build ./...` + `go vet ./internal/...` clean · `CGO_ENABLED=0 GOARCH=arm64 go build
  ./...` clean.
- new metrics tests (3): `BuildP99LatencyPromQL` (histogram_quantile + le + model_service + window) ·
  `QueryP99LatencyMillis` (0.85s → 850ms) · NoData/nil → ok=false (fail-open).
- new scaler tests (2): `LatencyOverrideScalesUp` (util=50 stay + P99 5000ms > 2000 SLO → forced busy +
  ScaleEvent) · `LatencyUnderSLONoOverride` (P99 500ms < SLO → stay). Full controller + metrics packages
  green (no regression to Phase 8/9 + T204 tests).
- `tests/sla`: `go test ./...` (percentile nearest-rank P50/P90/P99 + Summarize + empty) · `go build`
  amd64 + `CGO_ENABLED=0 GOARCH=arm64 go build` clean · no-`-url` guard exits 2.

**Real-machine (LAB-GATED · T301)**: `TARGET_URL=<PD endpoint> go run ./tests/sla -n 500 -c 20` against
a real Qwen-8B PD on 910B → actual P99 measured + reported vs the default SLO (build-doc §5 推理 SLA · the
达标 judgement uses the default SLO · customer swaps the threshold). The offline layer proves the harness
+ percentile + the controller query shape + the override logic.

## Carry-forward

- **T301**: real PD load-test run + the controller latency override observed end-to-end (P99 from the
  real vllm histogram driving a real scale-up).
- Customer SLO swap: set `NPUVERTICAL_SCALER_P99_SLO_MS` (chart) + `-slo-ms` (harness) to the real SLA.
