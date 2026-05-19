# Backend observability — detailed design

> Status: Phase 4 (T007 + T008) — Prometheus self-metrics endpoint + 3 Ocloud counter families.
> Phase 5+ may add HTTP request metrics, tracing (OpenTelemetry), and dedicated dashboard wiring.

## 1. 架构概览

```
┌──────────────────────────────────────────────────────────────────────┐
│                       demo-backend (Phase 1+2+3+4)                   │
│                                                                      │
│  ┌─────────────────────┐         ┌────────────────────────────────┐  │
│  │ gin engine          │         │ NewMetricsRegistry()           │  │
│  │  - r.Use(Recovery)  │         │  - NewGoCollector              │  │
│  │  - r.Use(Logging)   │         │  - NewProcessCollector         │  │
│  │  - r.Use(CORS opt)  │         │  - ocloud_backend_cache_evict_ │  │
│  │                     │         │  - ocloud_backend_cache_hits_  │  │
│  │  /api/v1/*  ◄──────┼────┐    │  - ocloud_backend_dispatch_    │  │
│  │  /ws/*       (P3+)  │    │    │     calls_total                │  │
│  │  /metrics  ◄────────┼────┼────┤  (3 ocloud + 2 default Go)     │  │
│  └─────────────────────┘    │    └────────────────────────────────┘  │
│                              │                                        │
│                  /metrics increments via:                             │
│                              │                                        │
│   ┌──────────────────────────┴──────────────────────────────────┐    │
│   │ Registry.SourceFor("clusters")                              │    │
│   │  → DispatchCounter.WithLabelValues("mock","clusters").Inc() │    │
│   │ (per-handler call site, see pkg/datasource/factory.go)       │    │
│   └──────────────────────────────────────────────────────────────┘    │
│                                                                      │
│   ┌─────────────────────────────────────────────────────────────┐    │
│   │ cache.LRU.Get(key) on hit                                   │    │
│   │  → HitsCounterVec.WithLabelValues(opts.Resource).Inc()      │    │
│   │ cache.LRU evict (any reason)                                │    │
│   │  → EvictionCounterVec.WithLabelValues(opts.Resource).Inc()  │    │
│   │ (per-cache-instance, see pkg/cache/lru.go)                   │    │
│   └─────────────────────────────────────────────────────────────┘    │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
            │                                              ▲
            │ HTTP scrape (GET /metrics)                   │
            ▼                                              │
┌──────────────────────────────────────────────────────────────────────┐
│            Prometheus (or kube-prometheus-stack)                     │
│  - auto-discover via Service annotations:                            │
│      prometheus.io/scrape: "true"                                    │
│      prometheus.io/port:   "8080"                                    │
│      prometheus.io/path:   "/metrics"                                │
└──────────────────────────────────────────────────────────────────────┘
```

**Position**: backend self-metrics endpoint at the gin engine root (NOT
under `/api/v1`). The endpoint serves the Prometheus text exposition
format using the `github.com/prometheus/client_golang` registry +
promhttp handler. The 3 Ocloud counter families are registered against
the same registry and incremented at the call sites in cache + datasource.

**Cross-references**:
- `docs/architecture.md` §3.5 (监控与日志 table) — demo-backend self-metrics row
- `docs/adr/0001-phase0-key-decisions.md` §5 v3 — Phase 4 dual-path
- `backend/README.md §Observability` — operator-facing summary
- `tests/e2e/kind/backend_metrics_test.sh` — CI assertion (P4-T-106)

## 2. 数据流

### 2.1 /metrics scrape flow

```
Prometheus scraper (kube-prometheus-stack)
   │ GET /metrics
   ▼ HTTP request hits gin engine
gin route GET /metrics
   │ outside the /api/v1 group → no auth middleware coverage (Phase 9 RBAC isolation)
   │ matched by router.go: r.GET(metricsRoutePath, h.metricsRoute())
   ▼
Handler.metricsRoute() (prom_metrics.go):
   │ if h.MetricsRegistry == nil → 503 "metrics registry not configured"
   │ else → promhttp.HandlerFor(reg, opts).ServeHTTP(c.Writer, c.Request)
   ▼ Prometheus library walks the registry, emits text format
HTTP response: 200 OK, Content-Type "text/plain; version=0.0.4; charset=utf-8"
```

### 2.2 Dispatch counter increment flow

```
Handler (e.g. ListClusters in cluster.go):
   src := h.Registry.SourceFor("clusters")
                   │
                   ▼
Registry.SourceFor(resource string):
   1. nil-check r → return nil
   2. lookup r.Mapping[resource] → srcName
   3. lookup r.Sources[srcName] → src
   4. if src != nil && r.DispatchCounter != nil:
        r.DispatchCounter.WithLabelValues(srcName, resource).Inc()
        // = ocloud_backend_dispatch_calls_total{datasource="mock",endpoint="clusters"}
   5. return src
```

Misses (nil result) do NOT increment. The handler-side error path
(returning 404 / 500) is the right observation site for misses;
Phase 5+ HTTP middleware can add request_duration_seconds_bucket etc.

### 2.3 Cache hit + eviction counter flow

```
Handler / aggregator opt-in to instrumented cache:
   cache.New(Options{
       MaxEntries:         100,
       TTL:                30 * time.Second,
       Resource:           "topology",
       EvictionCounterVec: promCounters.CacheEviction,
       HitsCounterVec:     promCounters.CacheHits,
   })

cache.LRU.Get(key):
   if TTL > 0 && time.Since(addedAt) > TTL:
     // TTL eviction path → emit eviction counter, then return miss
     pendingReason = TTL → hashicorp evict → onEvicted callback → counter++
     return zero, false
   v, ok := c.c.Get(key)
   if ok && HitsCounterVec != nil:
     HitsCounterVec.WithLabelValues(Resource).Inc()  // = ocloud_backend_cache_hits_total{resource="topology"}
   return v, ok

cache.LRU.Add(key, value):  // capacity eviction may fire
   hashicorp lru.Add → if full → onEvicted callback with pendingReason=Capacity
                                  → EvictionCounterVec.Inc()

cache.LRU.Remove(key):       // explicit eviction
   pendingReason = Explicit → hashicorp Remove → onEvicted → EvictionCounterVec.Inc()
```

Misses (Get returning false) do NOT increment HitsCounterVec — only
hits. Cache hit ratio = `cache_hits / (cache_hits + dispatch_calls{endpoint=X})`.

## 3. 接口契约

### 3.1 HTTP endpoint

```
GET /metrics
   Response:
     200 OK
     Content-Type: text/plain; version=0.0.4; charset=utf-8
     <body: Prometheus text exposition format>

   Body content (default + Phase 4 Ocloud):
     # default Go runtime collectors (NewGoCollector)
     go_gc_duration_seconds{quantile="0"} ...
     go_memstats_alloc_bytes ...
     go_goroutines ...
     # default process collector (NewProcessCollector)
     process_cpu_seconds_total ...
     process_resident_memory_bytes ...
     # Phase 4 Ocloud counters (after observed labelsets)
     ocloud_backend_dispatch_calls_total{datasource="mock",endpoint="clusters"} 1
     ...
```

**Edge cases**:
- `MetricsRegistry == nil` (misconfigured main.go) → 503 with body "metrics registry not configured"
- CounterVec with zero observed labelsets → emits no `# HELP` / `# TYPE` / data lines for that family. Verify via warming the counter at least once before assertion (see P4-T-106 warm-up step).

### 3.2 Three Ocloud counter families

| Metric                                  | Type      | Labels                   | Help text |
| --------------------------------------- | --------- | ------------------------ | --------- |
| `ocloud_backend_cache_eviction_total`   | CounterVec | `resource`               | Number of cache evictions (capacity / TTL / explicit) per logical resource. |
| `ocloud_backend_cache_hits_total`       | CounterVec | `resource`               | Number of cache hits per logical resource. Cache misses are NOT counted here; the resulting source dispatch is counted by `ocloud_backend_dispatch_calls_total`. |
| `ocloud_backend_dispatch_calls_total`   | CounterVec | `datasource`, `endpoint` | Number of Source dispatches (Registry.SourceFor returning non-nil) per datasource and logical resource endpoint. Increments regardless of cache hit/miss — pair with `ocloud_backend_cache_hits_total` to read the cache-hit ratio. |

**Plan deviation note (P4-T-008)**: plan literal said `endpoint="<path>"`
(HTTP path). The actual `endpoint` label carries the **logical resource
key** (e.g. `"clusters"`, `"nodes"`, `"topology"`) — dispatch is
selected by resource at the Registry.SourceFor call site, not by URL.
Future Phase 5+ HTTP-level metrics (request_duration_seconds_bucket
by path) would land in a separate middleware.

### 3.3 Go types (pkg/api/prom_metrics.go)

```go
package api

const (
    MetricCacheEvictionTotal  = "ocloud_backend_cache_eviction_total"
    MetricCacheHitsTotal      = "ocloud_backend_cache_hits_total"
    MetricDispatchCallsTotal  = "ocloud_backend_dispatch_calls_total"
)

type MetricsCounters struct {
    CacheEviction *prometheus.CounterVec  // labels: resource
    CacheHits     *prometheus.CounterVec  // labels: resource
    DispatchCalls *prometheus.CounterVec  // labels: datasource, endpoint
}

func NewMetricsRegistry() *prometheus.Registry                          // T007 entrypoint
func NewMetricsRegistryWithCounters() (*prometheus.Registry, MetricsCounters)  // T008 entrypoint used by main.go

func MetricsHandler(reg *prometheus.Registry) gin.HandlerFunc
func EnsureMetricsContentType(contentType string) bool  // test helper
```

The Handler struct (router.go) carries `MetricsRegistry *prometheus.Registry`
so handlers can read the registry if they ever need to register their
own collectors (Phase 5+ scenario).

### 3.4 Cache Options extensions (pkg/cache/options.go)

```go
type Options struct {
    MaxEntries int            // existing
    TTL        time.Duration  // existing
    OnEvict    func(...)      // existing

    // P4-T-008 nil-safe prometheus hooks.
    EvictionCounterVec *prometheus.CounterVec  // labels: resource
    HitsCounterVec     *prometheus.CounterVec  // labels: resource
    Resource           string                  // label value; required when either Vec set
}

// Validate rejects "counter set without Resource label" (added at T008).
func (o Options) Validate() error
```

### 3.5 Registry extension (pkg/datasource/factory.go)

```go
type Registry struct {
    Sources         map[string]Source                  // existing
    Mapping         map[string]string                  // existing
    DispatchCounter *prometheus.CounterVec             // P4-T-008 nil-safe
}

// SourceFor increments DispatchCounter on successful lookups (src != nil)
// when DispatchCounter is non-nil.
func (r *Registry) SourceFor(resource string) Source
```

## 4. 生命周期

### 4.1 Startup

```
main.go runServer():
   ... parse config + build logger ...
   ... construct sources (mock / k8s / prom / crd / configmap) ...
   reg, err := datasource.Build(cfg, sources)
   handler := api.NewHandler(reg, logger)
   handler.GrafanaBaseURL = cfg.Grafana.BaseURL
   //
   // P4-T-007 + P4-T-008 wiring:
   promReg, promCounters := api.NewMetricsRegistryWithCounters()
   handler.MetricsRegistry = promReg
   reg.DispatchCounter = promCounters.DispatchCalls
   _ = promCounters  // CacheEviction + CacheHits reserved for future handler/aggregator caches
   //
   router := api.NewRouter(handler, api.RouterOptions{EnableCORS: cfg.Server.EnableCORS})
   //
   srv := &http.Server{Addr: ":"+cfg.Server.Port, Handler: router, ...}
   srv.ListenAndServe()
```

### 4.2 Counter family registration

`NewMetricsRegistry` / `NewMetricsRegistryWithCounters` is called exactly
once at startup. The 3 CounterVecs are registered with the registry
immediately; observed labelsets accumulate over runtime.

### 4.3 Shutdown

Counters do not need cleanup. The registry is GC'd when the handler is
collected. Prometheus scrapes during the shutdown window may see
last-observed counts (acceptable; counters are monotonic).

## 5. 错误处理

| Error                                       | Severity | Action |
| ------------------------------------------- | -------- | ------ |
| MetricsRegistry == nil at request time       | degrade  | return 503 with hint "not configured" |
| promhttp internal error during scrape        | tolerate | promhttp emits its own error response; backend's logging middleware records the 5xx |
| CounterVec registration conflict (duplicate name) | startup  | `MustRegister` panics — caught at init since we own the registry |
| Cache Options.Validate rejects no-Resource label when counter set | startup  | `cache.New` returns error — caller (handler/aggregator) handles |
| Counter increment failure                   | n/a      | prometheus/client_golang increments are infallible (no I/O) |

## 6. 扩展点

### 6.1 Phase 5+: HTTP request metrics (middleware)

A future `pkg/middleware/metrics.go` could add:
```go
ocloud_backend_http_requests_total{method, path, status_class}     // CounterVec
ocloud_backend_http_request_duration_seconds_bucket{method, path}  // HistogramVec
```
This complements the dispatch counter (logical resource view) with a
URL view. Phase 5 scope item per backend roadmap.

### 6.2 Phase 5+: Tracing (OpenTelemetry)

`docs/architecture.md` §3.5 records "链路追踪: Jaeger 或 OpenTelemetry"
deferred to Phase 5+. When OTel lands, add a tracing exporter
alongside the metrics registry; instrument the gin engine and
datasource calls.

### 6.3 Phase 5+: Hook cache counters into real cache instances

`main.go` currently keeps `promCounters.CacheEviction` + `CacheHits`
unused (the `_ = promCounters` discard). To wire them, extend the
Handler struct with `CacheCounters api.MetricsCounters` so handlers
that construct local caches (e.g. topology aggregator) opt in:
```go
import "github.com/example/ocloud-edge/backend/pkg/cache"

func (h *Handler) buildTopologyCache() *cache.LRU[string, *Topology] {
    c, _ := cache.New[string, *Topology](cache.Options{
        MaxEntries:         100,
        TTL:                30 * time.Second,
        Resource:           "topology",
        EvictionCounterVec: h.CacheCounters.CacheEviction,
        HitsCounterVec:     h.CacheCounters.CacheHits,
    })
    return c
}
```

### 6.4 Phase 9 RBAC isolation guarantee

The `/metrics` route MUST remain outside the `/api/v1` group. Phase 9
RBAC middleware (per arch §13 Phase 9 review row) will land on /api/v1
exclusively. Tests/e2e/kind/backend_metrics_test.sh's
`TestMetricsEndpoint_NotOnAPIv1` defends this contract.

## 7. 集成示例

### 7.1 Local smoke

```bash
cd backend
make build
./bin/demo-backend --config configs/config.dev.yaml &

# Warm dispatch counter:
curl -fsS http://localhost:8080/api/v1/clusters >/dev/null
curl -fsS http://localhost:8080/api/v1/nodes >/dev/null

# Inspect /metrics:
curl -fsS http://localhost:8080/metrics | grep ocloud_backend_
# ocloud_backend_dispatch_calls_total{datasource="mock",endpoint="clusters"} 1
# ocloud_backend_dispatch_calls_total{datasource="mock",endpoint="nodes"} 1
```

### 7.2 Kind smoke (CI)

```bash
bash tests/e2e/kind/install.sh up
# ... wait for demo-backend rollout ...
BACKEND_URL=http://localhost:30080 bash tests/e2e/kind/backend_metrics_test.sh
```

Expected output:
```
== warm dispatch counter via /api/v1/clusters ==
== fetch /metrics ==
---HTTP_CODE: 200
---CONTENT_TYPE: text/plain; version=0.0.4; charset=utf-8
OK: 200 + text/plain content-type
OK: go_memstats_* present
OK: 1 ocloud_backend_* series present
== backend_metrics_test PASS ==
```

### 7.3 Stand-alone Prometheus scrape config

```yaml
scrape_configs:
  - job_name: ocloud-backend
    static_configs:
      - targets:
          - demo-backend.ocloud-system.svc.cluster.local:8080
    metrics_path: /metrics
    scrape_interval: 30s
    scrape_timeout: 10s
```

### 7.4 kube-prometheus-stack auto-discovery

The Service at `tests/e2e/kind/manifests/demo-backend.yaml` carries:
```yaml
annotations:
  prometheus.io/scrape: "true"
  prometheus.io/port:   "8080"
  prometheus.io/path:   "/metrics"
```

KPS's default scrape config (kubernetes-services job) picks this up
automatically — no extra Prometheus config needed in a KPS-managed
cluster.

### 7.5 Grafana panel (Phase 6+)

Cache hit ratio query (PromQL):
```promql
rate(ocloud_backend_cache_hits_total{resource="$resource"}[5m])
/
(
  rate(ocloud_backend_cache_hits_total{resource="$resource"}[5m])
  +
  rate(ocloud_backend_dispatch_calls_total{endpoint="$resource"}[5m])
)
```

Dispatch fan-out panel:
```promql
sum by (datasource) (rate(ocloud_backend_dispatch_calls_total[5m]))
```

## 8. 参考

- ADRs: `docs/adr/0001-phase0-key-decisions.md` §5 v3 (Phase 4 dual-path) — the Phase 9 RBAC isolation rationale
- Architecture: `docs/architecture.md` §3.5 (监控与日志 table — demo-backend 自指标 row)
- Plan: `docs/phase4-plan.md` §3 P4-T-007 (endpoint + dep) + P4-T-008 (counter families) + P4-T-106 (CI smoke + Observability docs)
- Devlog: `docs/devlog/phase-4-t007.md` · `phase-4-t008.md` · `phase-4-t106.md`
- README: `backend/README.md §Observability` — operator-facing user docs
- Code: `backend/pkg/api/prom_metrics.go` · `backend/pkg/api/router.go` · `backend/pkg/cache/options.go` · `backend/pkg/cache/lru.go` · `backend/pkg/datasource/factory.go` · `backend/cmd/demo-backend/main.go`
- Tests: `backend/pkg/api/prom_metrics_test.go` (4 cases) · `backend/pkg/cache/lru_test.go` (5 prom cases) · `backend/pkg/datasource/factory_test.go` (2 dispatch cases) · `tests/e2e/kind/backend_metrics_test.sh` (CI smoke)
- Manifests: `tests/e2e/kind/manifests/demo-backend.yaml` (Service annotations for KPS auto-discovery)
