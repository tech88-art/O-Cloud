# P4-T-008 · backend baseline metrics collectors (cache eviction + cache hits + dispatch calls)

- **Commit**: e380f39
- **Date**: 2026-05-19
- **Duration**: plan 0.5d · actual ~1h

## Intent

Register 3 Ocloud-prefixed counter families against the T007 registry: `ocloud_backend_cache_eviction_total{resource}` + `ocloud_backend_cache_hits_total{resource}` + `ocloud_backend_dispatch_calls_total{datasource, endpoint}`. Cache LRU + datasource Registry both nil-safe (no Phase 3 regression).

## Path adaptations

**Plan literal vs codebase reality**:
- Plan said: `backend/internal/dispatch/dispatch.go` to emit the dispatch counter.
- Reality: there is no `internal/dispatch/` package. Dispatch happens via `Registry.SourceFor(resource)` in `backend/pkg/datasource/factory.go` — the datasource abstraction layer is the "dispatch" surface.
- Resolution: hung the counter on `Registry.SourceFor` instead. Label `endpoint` carries the logical resource key (e.g. `clusters`/`nodes`) not the HTTP path — documented this semantic deviation from plan's literal `endpoint="<path>"` because dispatch is selected by resource, not by URL (a future GET vs POST on the same resource would double-count if endpoint=path).

## Debugging trail

- **Cache package gets a prometheus dep**. Plan literal says "cache.LRU accepts an optional `*prometheus.CounterVec`". This leaks the prometheus type out of the cache package's API. Alternative: define a generic `CounterRecorder interface { Inc(resource string) }` and let prom_metrics.go wire an adapter. Chose plan literal — added `github.com/prometheus/client_golang` to cache's imports. Small explicit dep leak, accepted per plan.
- **Validate enforces Resource label when counter set**. Initial Options had nil Resource as a valid combination → tests passed but operators could mis-label. Tightened `Options.Validate` to reject "counter set without Resource". Made the constructor fail fast at the right boundary. Added `TestLRU_PrometheusCounters_ResourceLabelRequired` to lock it in.
- **Live smoke shows only 2 dispatch series after 2 endpoint hits**. Plan acceptance: "≥3 non-zero series". Repeated curl warmup with 5 distinct endpoints → 5 series of `ocloud_backend_dispatch_calls_total`. Exceeds the bar.
- **Cache counter families show 0 lines**. Prometheus CounterVec emits no `# HELP` / `# TYPE` / value lines until at least one labelset has been observed. Since `main.go` doesn't currently construct any cache.LRU instances with prom counters, `ocloud_backend_cache_*` lines are absent from `/metrics`. Counter families ARE registered (verifiable via the registry contents); just unused at the call site. Documented inline in main.go: `_ = promCounters` retains the handle for future handler/aggregator caches to opt-in.

## Key decisions

- **endpoint label = resource key, not HTTP path**. Plan deviation. Rationale: `Registry.SourceFor(resource)` is the only dispatch call site; passing `endpoint=<resource>` mirrors what the function takes. Future Phase 5+ HTTP-level metrics (request_duration_seconds_bucket) can label by path separately.
- **Misses NOT counted in dispatch**. `Registry.SourceFor` returns nil when mapping or source missing — we skip increment. Handler-side error path (returning 404 / 500) is the right observation site for misses; Phase 5+ HTTP middleware can add that.
- **Misses NOT counted in cache_hits**. Cache `Get()` on miss skips the hits-counter increment. Cache hit ratio = `cache_hits / (cache_hits + dispatch_calls{endpoint=resource})`. Documented in README §Observability.

## Verification

P3 三维度:
- Existence: `grep ocloud_backend_ backend/pkg/api/prom_metrics.go` → 3 metric name consts present
- Completeness: `go test ./pkg/cache ./pkg/datasource ./pkg/api -run "Metrics|Prom|Dispatch" -v` → 11 sub-tests PASS (5 cache + 2 dispatch + 4 prom_metrics)
- Correctness: live smoke `curl /metrics | grep ocloud_backend_` after 5-endpoint warmup → 5 distinct dispatch_calls_total label sets (clusters / nodes / presets / topology / workloads)

## Carry-forward

- Phase 5 handler/aggregator caches that want hit/eviction observability call `cache.New(Options{..., Resource: "name", EvictionCounterVec: promCounters.CacheEviction, HitsCounterVec: promCounters.CacheHits})`. The `promCounters` handle is in scope of `runServer` in main.go — extend Handler struct if downstream code needs it.
- T106 wires the CI smoke against `/metrics` after a warm dispatch. Without the warmup curl, the assertion would flake (counter families register but emit no lines at zero observations).
- Cache hit ratio dashboard query (PromQL): `rate(ocloud_backend_cache_hits_total[5m]) / (rate(ocloud_backend_cache_hits_total[5m]) + rate(ocloud_backend_dispatch_calls_total[5m]))` — Phase 6+ Grafana dashboard.
