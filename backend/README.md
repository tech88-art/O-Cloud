# demo-backend

Go service behind the O-Cloud demo frontend. REST + WebSocket over `/api/v1`,
multi-source data abstraction (mock → k8s → prometheus → crd → configmap as
phases land).

**Source of truth for module rules:** [`./CLAUDE.md`](./CLAUDE.md) — every
contributor (human or AI agent) MUST read it before touching files here.

## Quick start

```bash
cd backend
make tidy
make build       # → bin/demo-backend
make run         # uses configs/config.dev.yaml, listens on :8080

# verify
curl -s localhost:8080/api/v1/healthz | jq
curl -s localhost:8080/api/v1/version | jq
```

## Layout

See `CLAUDE.md` §3. Don't reorder without an RFC.

## Observability (Phase 4)

The demo-backend exposes a Prometheus self-metrics endpoint at the gin
engine root — **outside** the `/api/v1` group so Phase 9 RBAC middleware
will not block scrapes.

```bash
curl -s localhost:8080/metrics | head -20
```

### Endpoint

`GET /metrics` returns the Prometheus text-exposition format with
`Content-Type: text/plain; version=0.0.4; charset=utf-8`. The endpoint
is registered by `pkg/api/prom_metrics.go` and mounted via
`pkg/api/router.go` outside the `/api/v1` group.

### Counter families

Three Ocloud-prefixed counters land alongside the default Go runtime +
process collectors:

| Metric                                  | Labels                   | Semantics |
| --------------------------------------- | ------------------------ | --------- |
| `ocloud_backend_cache_eviction_total`   | `resource`               | Cache evictions per logical resource (capacity / TTL / explicit — all cause types counted) |
| `ocloud_backend_cache_hits_total`       | `resource`               | Cache hits per logical resource. Misses are NOT counted here; the resulting source dispatch is captured by `ocloud_backend_dispatch_calls_total`. |
| `ocloud_backend_dispatch_calls_total`   | `datasource`, `endpoint` | Successful `Registry.SourceFor` lookups. `datasource` = source name (e.g. `mock` / `k8s`); `endpoint` = logical resource key (e.g. `clusters` / `topology`). Misses (nil result) are NOT counted. |

Cache vs dispatch reading: a request that hits cache increments
`ocloud_backend_cache_hits_total{resource=X}` only; a request that misses
cache increments `ocloud_backend_dispatch_calls_total{datasource=Y,endpoint=X}`
(the resulting source call) **without** touching the hits counter. Cache-hit
ratio for resource X is `cache_hits / (cache_hits + dispatch_calls{endpoint=X})`.

### Prometheus scrape config

The `tests/e2e/kind/manifests/demo-backend.yaml` Service carries the
standard kube-prometheus-stack auto-discovery annotations:

```yaml
prometheus.io/scrape: "true"
prometheus.io/port:   "8080"
prometheus.io/path:   "/metrics"
```

For stand-alone Prometheus, drop a 5-line scrape stanza:

```yaml
scrape_configs:
  - job_name: ocloud-backend
    static_configs:
      - targets:
          - demo-backend.ocloud-system.svc.cluster.local:8080
    metrics_path: /metrics
```

### Local smoke

`tests/e2e/kind/backend_metrics_test.sh` runs as part of the kind e2e job
(P4-T-106). To exercise it without the full cluster:

```bash
make build && ./bin/demo-backend --config configs/config.dev.yaml &
curl -fsS http://localhost:8080/api/v1/clusters >/dev/null      # warm dispatch
curl -fsS http://localhost:8080/metrics | grep ocloud_backend_
```

Expect at least one `ocloud_backend_dispatch_calls_total{datasource="mock",endpoint="clusters"}` line.

## Phase 1 scaffold (P1-T-005)

This commit ships the scaffold only:

- `cmd/demo-backend/main.go` — Cobra + Viper entrypoint.
- `pkg/api/{system,router,errors}.go` — only `/healthz` and `/version`.
- `pkg/datasource/source.go` — full `Source` interface.
- `pkg/datasource/mock/source.go` — stub implementing every method as
  `ErrNotImplemented`. T101+ fills these in.
- `pkg/{model,config,cache,middleware}/` — supporting infrastructure.

Run `make test` to confirm `/healthz` and `/version` work end-to-end.
