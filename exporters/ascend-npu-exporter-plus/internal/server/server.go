// Package server wires the HTTP endpoints (/metrics, /healthz).
//
// Phase 3 (P3-T-006) exposes:
//   - GET /metrics → promhttp handler over the supplied Registry
//   - GET /healthz → 200 OK with body "ok\n"
//
// Caller owns the *http.Server lifecycle (ListenAndServe + Shutdown).
package server

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Handler returns the multiplexer wiring /metrics and /healthz. Tests use this
// directly via httptest.NewServer to avoid managing an *http.Server lifecycle.
func Handler(reg *prometheus.Registry) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux
}

// New returns an *http.Server bound to addr and serving the Handler.
//
// The returned *http.Server has zero timeouts (defaults). Phase 3+ may layer
// timeouts and TLS on top; the skeleton does not impose them.
func New(addr string, reg *prometheus.Registry) *http.Server {
	return &http.Server{
		Addr:    addr,
		Handler: Handler(reg),
	}
}
