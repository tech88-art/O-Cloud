// Phase 4 (P4-T-007) Prometheus self-metrics endpoint.
//
// This file mounts `/metrics` on the gin engine root (OUTSIDE the /api/v1
// group) so Prometheus scrapes do NOT pick up future /api/v1 auth middleware
// (Phase 9 RBAC). The /metrics surface advertises the standard
// `go_gc_*` / `go_memstats_*` / `process_*` collectors that prometheus/
// client_golang registers by default; downstream collectors land at
// P4-T-008 (cache eviction + cache hits + dispatch calls).
//
// API note: the existing /api/v1/metrics/query / /api/v1/metrics/templates /
// /api/v1/metrics/grafana-url routes (see metrics.go + grafana.go) are
// FRONTEND-facing demo data routes — completely separate from this
// PROMETHEUS-facing /metrics endpoint despite the shared word "metrics".
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Ocloud-prefixed counter family names (P4-T-008).
const (
	// MetricCacheEvictionTotal counts cache evictions across all
	// in-process cache instances, labelled by logical resource.
	MetricCacheEvictionTotal = "ocloud_backend_cache_eviction_total"

	// MetricCacheHitsTotal counts cache hits across all in-process cache
	// instances, labelled by logical resource.
	MetricCacheHitsTotal = "ocloud_backend_cache_hits_total"

	// MetricDispatchCallsTotal counts Source dispatches (datasource
	// selections) per resource endpoint. Increments on every
	// Registry.SourceFor call regardless of whether the cache layer
	// later short-circuits with a hit — cache hits are tracked
	// separately via MetricCacheHitsTotal.
	MetricDispatchCallsTotal = "ocloud_backend_dispatch_calls_total"
)

// Ocloud-prefixed counter handles (P4-T-008). Constructed by
// NewMetricsRegistry; consumed by downstream packages via the returned
// MetricsCounters struct.
type MetricsCounters struct {
	// CacheEviction labels: resource (logical cache name, e.g.
	// "topology" / "workloads" / "presets"). Incremented per evicted
	// entry regardless of cause (capacity / TTL / explicit).
	CacheEviction *prometheus.CounterVec

	// CacheHits labels: resource. Incremented for every Get() that
	// resolved from cache; misses are NOT counted here (the dispatch
	// counter captures the resulting source call).
	CacheHits *prometheus.CounterVec

	// DispatchCalls labels: datasource (source name, e.g. "mock" /
	// "k8s") + endpoint (logical resource key, e.g. "clusters" /
	// "topology"). Incremented for every Registry.SourceFor lookup that
	// returns a non-nil Source. Label is "endpoint" per plan §3 P4-T-008;
	// here it carries the logical resource key (not the HTTP path)
	// because dispatch is selected by resource, not by URL.
	DispatchCalls *prometheus.CounterVec
}

// NewMetricsRegistry returns a Prometheus registry pre-populated with the
// default Go runtime + process collectors plus the three Ocloud counter
// families introduced at P4-T-008.
//
// The registry is intentionally a *new* registry (not the global
// prometheus.DefaultRegisterer) so the backend's metrics are isolated
// from any in-tree library that might register against the default
// registry at init() time.
//
// Callers receive the registry plus a MetricsCounters handle. main.go
// passes MetricsCounters into the cache + datasource subsystems so
// counter increments happen at the source.
func NewMetricsRegistry() *prometheus.Registry {
	reg, _ := newRegistryWithCounters()
	return reg
}

// NewMetricsRegistryWithCounters is the variant main.go uses when it
// needs the MetricsCounters handle. The single-return NewMetricsRegistry
// is preserved for test ergonomics + Phase 3 callers that don't yet
// instrument cache/dispatch.
func NewMetricsRegistryWithCounters() (*prometheus.Registry, MetricsCounters) {
	return newRegistryWithCounters()
}

func newRegistryWithCounters() (*prometheus.Registry, MetricsCounters) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	mc := MetricsCounters{
		CacheEviction: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: MetricCacheEvictionTotal,
			Help: "Number of cache evictions (capacity / TTL / explicit) per logical resource.",
		}, []string{"resource"}),
		CacheHits: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: MetricCacheHitsTotal,
			Help: "Number of cache hits per logical resource. Cache misses are not counted here; the resulting source dispatch is counted by " + MetricDispatchCallsTotal + ".",
		}, []string{"resource"}),
		DispatchCalls: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: MetricDispatchCallsTotal,
			Help: "Number of Source dispatches (Registry.SourceFor returning non-nil) per datasource and logical resource endpoint. Increments regardless of cache hit/miss — pair with " + MetricCacheHitsTotal + " to read the cache-hit ratio.",
		}, []string{"datasource", "endpoint"}),
	}
	reg.MustRegister(mc.CacheEviction, mc.CacheHits, mc.DispatchCalls)
	return reg, mc
}

// MetricsHandler returns a gin handler that serves the prometheus HTTP
// scrape protocol from reg. Tests build a registry via NewMetricsRegistry,
// pass it here, and exercise the resulting endpoint with httptest. The
// router.go mount point calls this with the Handler's MetricsRegistry.
func MetricsHandler(reg *prometheus.Registry) gin.HandlerFunc {
	h := promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		// Default — ErrorLog nil so promhttp falls back to log.Default()
		// (the backend wires its own zap logger via request-level middleware
		// already; Prometheus scrape paths are too high-volume to log
		// per-request errors here).
		Registry: reg,
	})
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

// metricsContentTypePrefix is the leading byte sequence of the
// Prometheus text exposition content-type. Tests use this to confirm
// `/metrics` returns a Prometheus-compatible response without
// asserting the exact escaped MIME parameters.
const metricsContentTypePrefix = "text/plain"

// EnsureMetricsContentType is a defensive helper used by tests that need
// to verify the Content-Type header carries text/plain. promhttp emits
// the precise MIME (with version + charset params) so a substring check
// is the stable assertion shape.
func EnsureMetricsContentType(contentType string) bool {
	if len(contentType) < len(metricsContentTypePrefix) {
		return false
	}
	return contentType[:len(metricsContentTypePrefix)] == metricsContentTypePrefix
}

// metricsRoutePath is the root-level path the gin engine mounts the
// /metrics endpoint at. Exposed as a constant so router.go and tests use
// the same string.
const metricsRoutePath = "/metrics"

// Handler.metricsRoute returns the gin handler bound to the Handler's
// MetricsRegistry. Used by router.go.
func (h *Handler) metricsRoute() gin.HandlerFunc {
	if h.MetricsRegistry == nil {
		// Defensive: if main forgot to set MetricsRegistry, serve an empty
		// 503 instead of nil-panicking. Phase 4 T007 main.go always sets it.
		return func(c *gin.Context) {
			c.String(http.StatusServiceUnavailable,
				"metrics registry not configured")
		}
	}
	return MetricsHandler(h.MetricsRegistry)
}
