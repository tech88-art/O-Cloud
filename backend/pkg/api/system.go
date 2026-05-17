package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// CurrentVersion is hard-coded for Phase 1 (P1-T-005). When release tooling
// lands, the Makefile will inject this via -ldflags and the constant becomes
// the fallback.
const CurrentVersion = "0.1.0"

// Build metadata — wired by Makefile -ldflags at build time. Empty when running
// `go run` directly.
//
//nolint:gochecknoglobals // injected via -ldflags
var (
	BuildCommit    = ""
	BuildTimestamp = ""
)

// Healthz handles GET /api/v1/healthz.
//
// Contract: components.schemas.Health. Status is "ok" when every enabled
// datasource self-reports ok; "degraded" otherwise. With no enabled
// datasources (Phase-1 scaffold), status is "ok" and datasources is empty.
func (h *Handler) Healthz(c *gin.Context) {
	resp := model.Health{
		Status:      "ok",
		Datasources: collectDatasourceHealth(h.Registry),
	}
	// Promote to degraded if any source self-reports error.
	for _, s := range resp.Datasources {
		if s == "error" {
			resp.Status = "degraded"
			break
		}
	}
	c.JSON(http.StatusOK, resp)
}

// Version handles GET /api/v1/version.
//
// Contract: components.schemas.Version.
func (h *Handler) Version(c *gin.Context) {
	c.JSON(http.StatusOK, model.Version{
		Version:   CurrentVersion,
		Commit:    BuildCommit,
		BuildTime: BuildTimestamp,
	})
}

// collectDatasourceHealth walks the registry and returns a name → status map.
// Phase-1 scaffold: empty registry → empty map (legal per contract).
//
// PHASE-2: each Source will expose a Ping/Health method; until then any
// registered source is reported "ok".
func collectDatasourceHealth(reg *datasource.Registry) map[string]string {
	out := map[string]string{}
	if reg == nil {
		return out
	}
	for name := range reg.Sources {
		out[name] = "ok"
	}
	return out
}
