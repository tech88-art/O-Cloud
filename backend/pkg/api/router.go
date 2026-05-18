// Package api wires Gin handlers. Each resource gets its own file (cluster.go,
// node.go, …); router.go is the single mount point.
//
// Phase 1 (P1-T-005) wires only /healthz and /version. T101+ adds the rest.
package api

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	"github.com/example/ocloud-edge/backend/pkg/middleware"
)

// APIPrefix is the version prefix every endpoint sits under (per contract §0).
const APIPrefix = "/api/v1"

// Handler bundles the dependencies handlers reach for. Pass it once at startup
// and let routes close over it — never use package globals for these.
//
// GrafanaBaseURL is read by the /grafana/url handler (P1-T-205). An empty
// value falls back to defaultGrafanaBaseURL — see grafana.go. Set this via
// main after config.Load so production picks up config.yaml's grafana.baseUrl.
type Handler struct {
	Registry       *datasource.Registry
	Logger         *zap.Logger
	GrafanaBaseURL string
}

// NewHandler is the canonical constructor. logger may be nil — we substitute
// a no-op zap so handlers never branch on `if logger != nil`.
func NewHandler(reg *datasource.Registry, logger *zap.Logger) *Handler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Handler{
		Registry: reg,
		Logger:   logger,
	}
}

// RouterOptions controls engine setup.
type RouterOptions struct {
	EnableCORS bool
	CORSConfig middleware.CORSConfig
}

// NewRouter constructs a Gin engine, attaches middleware, and mounts T005
// routes. Tests use this directly via httptest.NewRecorder.
func NewRouter(h *Handler, opts RouterOptions) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.Use(gin.Recovery())
	r.Use(middleware.Logging(h.Logger))
	if opts.EnableCORS {
		corsCfg := opts.CORSConfig
		if len(corsCfg.AllowOrigins) == 0 {
			corsCfg = middleware.DefaultCORSConfig
		}
		r.Use(middleware.CORS(corsCfg))
	}

	v1 := r.Group(APIPrefix)
	{
		v1.GET("/healthz", h.Healthz)
		v1.GET("/version", h.Version)

		// Clusters (P1-T-101)
		v1.GET("/clusters", h.ListClusters)
		v1.GET("/clusters/:clusterId", h.GetCluster)

		// Topology (P1-T-102) — cluster→node→npu→slice graph for G6
		v1.GET("/clusters/:clusterId/topology", h.GetClusterTopology)

		// Nodes (P1-T-103)
		v1.GET("/nodes", h.ListNodes)
		v1.GET("/nodes/:nodeName", h.GetNodeDetail)

		// NPUs (P1-T-104)
		v1.GET("/nodes/:nodeName/npus", h.ListNPUs)

		// Workloads (P1-T-201)
		v1.GET("/workloads", h.ListWorkloads)
		v1.GET("/workloads/:namespace/:name", h.GetWorkloadDetail)

		// Workload logs REST (P1-T-301) — tail + filter; WS stream lives
		// at /ws/logs/:ns/:name outside the /api/v1 group.
		v1.GET("/workloads/:namespace/:name/logs", h.GetWorkloadLogs)

		// Presets (P1-T-203)
		v1.GET("/presets", h.ListPresets)
		v1.GET("/presets/:presetId", h.GetPreset)

		// Deploy (P1-T-202) — POST creates a new mock workload from a preset;
		// DELETE removes it and frees the allocated slices.
		v1.POST("/deploy", h.Deploy)
		v1.DELETE("/deploy/:deployId", h.DeleteDeploy)

		// Metrics (P1-T-204, RFC-003 var-slice)
		v1.POST("/metrics/query", h.QueryMetric)
		v1.GET("/metrics/query", h.QueryMetric)
		v1.GET("/metrics/templates", h.ListTemplates)

		// Grafana embed URL (P1-T-205)
		v1.GET("/grafana/url", h.GetGrafanaURL)

		// PHASE-1: later tasks extend with /deploy, /logs, ...
	}

	// WebSocket endpoints sit OUTSIDE /api/v1 per docs/api-contract.yaml
	// (comment block §"WebSocket 通道"). P1-T-105 mounts /ws/topology;
	// P1-T-301 adds /ws/logs/:ns/:name; later phases add /ws/workloads,
	// /ws/metrics.
	r.GET("/ws/topology", h.WSTopology)
	r.GET("/ws/logs/:namespace/:name", h.WSLogs)

	return r
}
