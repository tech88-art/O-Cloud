# P4-T-007 · backend prometheus/client_golang dep + /metrics Prometheus self-endpoint

- **Commit**: 7f1056e
- **Date**: 2026-05-19
- **Duration**: plan 0.5d · actual ~45min

## Intent

Add `github.com/prometheus/client_golang` direct dep + mount a `/metrics` Prometheus self-metrics endpoint at the gin engine ROOT (outside `/api/v1` so Phase 9 RBAC middleware doesn't cover scrapes). Default Go runtime + process collectors. Existing `/api/v1/metrics/query|templates|grafana-url` (frontend-facing) stay untouched.

## Path adaptations

**Plan literal vs codebase reality (P3 honesty)**:
- Plan said: `backend/internal/server/server.go` + `backend/cmd/server/main.go`
- Reality: `backend/pkg/api/router.go` (gin engine setup) + `backend/cmd/demo-backend/main.go` (entry). `backend/internal/server/` does not exist.
- Resolution: adapted paths to actual layout — `backend/pkg/api/prom_metrics.go` (new) + `backend/pkg/api/router.go` (small edit) + `backend/cmd/demo-backend/main.go` (small edit). Same plan intent (M1 user-intent > rule literal). Documented adaptation in commit footer.

## Debugging trail

- `go get github.com/prometheus/client_golang@v1.23.2` to pin the version. v1.23.2 matches what `operators/pool-operator/go.mod` already has as indirect — avoids version drift at the workspace level.
- Plan acceptance bullet "/metrics lives at gin engine root, NOT under /api/v1". Verified by adding `TestMetricsEndpoint_NotOnAPIv1`: GET `/api/v1/metrics` returns 404 (route not registered there); GET `/metrics` returns 200 with Prometheus content. The defensive test catches a future refactor that accidentally puts the route under v1.
- Initial smoke against `localhost:8080/metrics` returned 404. Investigated — `netstat` showed PID 15696 listening on 8080 (stale `demo-backend.exe` from previous Phase 3 run). Killed via PowerShell `Stop-Process -Id 15696 -Force`, restarted the freshly-built binary, smoke succeeded. Lesson: when local smoke unexpectedly 404s on a freshly-built service, check `netstat -ano | grep <port>` for a stale instance first.

## Key decisions

- **Separate registry, not DefaultRegisterer**. `prometheus.NewRegistry()` instead of `prometheus.DefaultRegisterer`. Isolates backend metrics from any library that might register at `init()` time. Documented in `NewMetricsRegistry()` godoc.
- **Defensive 503 when MetricsRegistry == nil**. Plan literal doesn't require it; added as a "fail soft" so a future main() refactor that forgets to call `NewMetricsRegistry()` returns "metrics registry not configured" instead of nil-panicking. Tested via `TestMetricsEndpoint_NilRegistryReturns503`.
- **EnsureMetricsContentType helper**. promhttp emits `text/plain; version=0.0.4; charset=utf-8` (full MIME with params). Tests assert via substring `text/plain` so a future client_golang version that adds new params doesn't break the assertion.

## Verification

P3 三维度:
- Existence: `grep prometheus/client_golang backend/go.mod` → 1 direct dep
- Completeness: `go test ./pkg/api -run Metrics -v` → 4/4 sub-tests PASS
- Correctness: live `curl /metrics`: 22 `go_memstats_*` + 6 `process_*` lines after fresh `./bin/demo-backend` startup (commit footer captures the actual output)

## Carry-forward

- T008 registers the 3 `ocloud_backend_*` counter families against this same registry.
- T106 ships `tests/e2e/kind/backend_metrics_test.sh` exercising both the default collectors + (after warm-up) the Ocloud counters.
- The Handler struct gained a `MetricsRegistry *prometheus.Registry` field — Phase 9 RBAC middleware design must continue to mount /metrics at engine root, not under /api/v1.
