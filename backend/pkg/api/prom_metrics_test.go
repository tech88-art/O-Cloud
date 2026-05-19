// Phase 4 (P4-T-007) Prometheus self-metrics endpoint tests.
package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newMetricsTestRouter(t *testing.T) (*gin.Engine, *Handler) {
	t.Helper()
	h := NewHandler(nil, nil)
	h.MetricsRegistry = NewMetricsRegistry()
	r := NewRouter(h, RouterOptions{})
	return r, h
}

func TestMetricsEndpointReturns200(t *testing.T) {
	r, _ := newMetricsTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /metrics: expected 200, got %d", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if !EnsureMetricsContentType(ct) {
		t.Errorf("GET /metrics: content-type should start with text/plain, got %q", ct)
	}

	body, err := io.ReadAll(w.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	out := string(body)

	// Default collectors should produce at least the standard go_gc /
	// go_memstats / process_ series. We sample one representative metric
	// from each family so a future client_golang renaming does not silently
	// shrink coverage.
	for _, want := range []string{
		"go_gc",        // go_gc_duration_seconds or go_gc_gogc_percent (varies by version)
		"go_memstats_", // go_memstats_alloc_bytes etc.
		"process_",     // process_cpu_seconds_total etc.
	} {
		if !strings.Contains(out, want) {
			t.Errorf("GET /metrics: missing expected substring %q", want)
		}
	}
}

func TestMetricsEndpoint_NilRegistryReturns503(t *testing.T) {
	// Defensive: when MetricsRegistry is nil (misconfigured main), the
	// /metrics handler must not panic — it returns 503 with a hint.
	h := NewHandler(nil, nil) // MetricsRegistry left nil
	r := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("nil registry should yield 503, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "not configured") {
		t.Errorf("nil registry response should hint 'not configured', got %q", w.Body.String())
	}
}

func TestMetricsEndpoint_NotOnAPIv1(t *testing.T) {
	// Plan acceptance: /metrics lives at engine root, NOT under /api/v1.
	// Verify by asserting GET /api/v1/metrics returns 404 (or at least is
	// NOT served by the prometheus scrape handler), while GET /metrics
	// does serve it.
	r, _ := newMetricsTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// /api/v1/metrics is NOT a registered route in this codebase (only
	// /api/v1/metrics/query and /api/v1/metrics/templates exist), so the
	// expected response is 404 NotFound. Crucially, the body must NOT
	// contain Prometheus metric output.
	if w.Code == http.StatusOK && strings.Contains(w.Body.String(), "go_memstats_") {
		t.Errorf("/api/v1/metrics should NOT return Prometheus content; that path is reserved for /api/v1 frontend handlers")
	}
}

func TestExistingMetricsAPIv1RoutesStillRegistered(t *testing.T) {
	// Defensive regression: T007 must not break the existing frontend-
	// facing /api/v1/metrics/query + /api/v1/metrics/templates routes.
	r, _ := newMetricsTestRouter(t)

	for _, path := range []string{
		"/api/v1/metrics/templates",
		"/api/v1/metrics/query",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		// We don't assert 200 (these routes may need a backing datasource
		// to succeed), but we do assert "not 404" — the handler must be
		// registered. Empty Handler+Registry yields 500 / 400 etc., never
		// 404 for a registered route.
		if w.Code == http.StatusNotFound {
			t.Errorf("path %s should remain registered; got 404", path)
		}
	}
}
