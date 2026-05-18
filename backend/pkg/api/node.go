// Package api — node handlers (P1-T-103).
//
// Endpoints:
//   - GET /api/v1/nodes              → []Node     (filters: clusterId, poolName, role)
//   - GET /api/v1/nodes/:nodeName    → NodeDetail (404 if missing)
//
// Contract: see docs/api-contract.yaml /api/v1/nodes + /api/v1/nodes/{nodeName}.
package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	"github.com/example/ocloud-edge/backend/pkg/datasource/mock"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// nodeResourceKey is the mapping key used by Registry.SourceFor to resolve the
// datasource backing /nodes. Defined here (not in router) so the handler is
// self-documenting about which mapping entry it consults.
const nodeResourceKey = "nodes"

// ListNodes handles GET /api/v1/nodes.
//
// Query params (all optional, AND-combined):
//   - clusterId : restrict to one cluster
//   - poolName  : restrict to one node pool
//   - role      : restrict to nodes whose role[] contains the value
//     (e.g. ?role=worker)
//
// Returns an empty array (never nil) on no match.
func (h *Handler) ListNodes(c *gin.Context) {
	src := h.nodeSource()
	if src == nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"no datasource configured for nodes", nil)
		return
	}

	filter := model.NodeFilter{
		ClusterID: c.Query("clusterId"),
		PoolName:  c.Query("poolName"),
		Role:      c.Query("role"),
	}

	out, err := src.ListNodes(c.Request.Context(), filter)
	if err != nil {
		h.Logger.Error("ListNodes failed",
			zap.String("source", src.Name()),
			zap.Error(err))
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"failed to list nodes", map[string]interface{}{
				"source": src.Name(),
			})
		return
	}

	if out == nil {
		out = []*model.Node{}
	}
	c.JSON(http.StatusOK, out)
}

// GetNodeDetail handles GET /api/v1/nodes/:nodeName.
//
// Returns 404 with the canonical Error envelope when the node is not found.
func (h *Handler) GetNodeDetail(c *gin.Context) {
	name := c.Param("nodeName")
	if name == "" {
		respondError(c, http.StatusBadRequest, CodeBadRequest,
			"nodeName path parameter is required", nil)
		return
	}

	src := h.nodeSource()
	if src == nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"no datasource configured for nodes", nil)
		return
	}

	detail, err := src.GetNodeDetail(c.Request.Context(), name)
	if err != nil {
		if errors.Is(err, mock.ErrNodeNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound,
				"node not found", map[string]interface{}{
					"resource": "node",
					"name":     name,
				})
			return
		}
		h.Logger.Error("GetNodeDetail failed",
			zap.String("source", src.Name()),
			zap.String("name", name),
			zap.Error(err))
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"failed to get node detail", map[string]interface{}{
				"source": src.Name(),
			})
		return
	}

	c.JSON(http.StatusOK, detail)
}

// nodeSource resolves the source backing /nodes via the registry mapping, with
// a single-source fallback so a minimal config (one mock source, no explicit
// mapping) still works. Returns nil only when no source is registered at all.
func (h *Handler) nodeSource() datasource.Source {
	if h == nil || h.Registry == nil {
		return nil
	}
	if src := h.Registry.SourceFor(nodeResourceKey); src != nil {
		return src
	}
	// Fallback: if exactly one source is registered, use it. Keeps tests and
	// dev configs concise (no need to spell out every mapping entry).
	if len(h.Registry.Sources) == 1 {
		for _, s := range h.Registry.Sources {
			return s
		}
	}
	return nil
}
