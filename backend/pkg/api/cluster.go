package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	mocksrc "github.com/example/ocloud-edge/backend/pkg/datasource/mock"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// resourceClusters is the registry key for cluster handlers. Mirrors the
// `mapping.clusters` entry in config.yaml (per backend/CLAUDE.md §4.2).
const resourceClusters = "clusters"

// ListClusters handles GET /api/v1/clusters.
//
// Contract: returns a JSON array of components.schemas.Cluster.
// Empty list → 200 with `[]` (per OpenAPI spec, not 404).
//
// Errors:
//   - no datasource mapped for "clusters"  → 500 InternalError
//   - source.ListClusters returns an error → 500 InternalError
func (h *Handler) ListClusters(c *gin.Context) {
	src := h.Registry.SourceFor(resourceClusters)
	if src == nil {
		h.Logger.Error("no datasource mapped for clusters")
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"no datasource mapped for clusters", nil)
		return
	}
	clusters, err := src.ListClusters(c.Request.Context())
	if err != nil {
		h.Logger.Error("list clusters", zap.String("source", src.Name()), zap.Error(err))
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"failed to list clusters", nil)
		return
	}
	// Contract demands an array, never null. Allocate an empty slice when the
	// source returns nil so encoding/json emits `[]` not `null`.
	if clusters == nil {
		clusters = []*model.Cluster{}
	}
	c.JSON(http.StatusOK, clusters)
}

// GetCluster handles GET /api/v1/clusters/:clusterId.
//
// Contract: returns components.schemas.Cluster on 200. 404 with an Error
// envelope when the id does not exist. mock.ErrClusterNotFound is the
// canonical signal used by the mock source; any other error is treated as
// a 500.
func (h *Handler) GetCluster(c *gin.Context) {
	id := c.Param("clusterId")
	if id == "" {
		respondError(c, http.StatusBadRequest, CodeBadRequest,
			"clusterId is required", nil)
		return
	}
	src := h.Registry.SourceFor(resourceClusters)
	if src == nil {
		h.Logger.Error("no datasource mapped for clusters")
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"no datasource mapped for clusters", nil)
		return
	}
	cluster, err := src.GetCluster(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, mocksrc.ErrClusterNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound,
				"cluster not found",
				map[string]interface{}{"resource": "cluster", "id": id})
			return
		}
		h.Logger.Error("get cluster",
			zap.String("source", src.Name()),
			zap.String("id", id),
			zap.Error(err))
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"failed to get cluster", nil)
		return
	}
	c.JSON(http.StatusOK, cluster)
}
