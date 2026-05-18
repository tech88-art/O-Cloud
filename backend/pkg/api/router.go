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
type Handler struct {
	Registry *datasource.Registry
	Logger   *zap.Logger
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

		// Presets (P1-T-203)
		v1.GET("/presets", h.ListPresets)
		v1.GET("/presets/:presetId", h.GetPreset)

		// PHASE-1: later tasks extend with /deploy, /metrics, ...
	}

	// WebSocket endpoints sit OUTSIDE /api/v1 per docs/api-contract.yaml
	// (comment block §"WebSocket 通道"). P1-T-105 mounts /ws/topology;
	// later phases add /ws/workloads, /ws/logs/*, /ws/metrics.
	r.GET("/ws/topology", h.WSTopology)

	return r
}
