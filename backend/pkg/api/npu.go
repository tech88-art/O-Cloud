// Package api — NPU handlers (P1-T-104).
//
// Endpoint:
//   - GET /api/v1/nodes/:nodeName/npus → []NPU (filtered by nodeName,
//     each NPU includes slice status — sliceMode + slices array)
//
// Contract: see docs/api-contract.yaml /api/v1/nodes/{nodeName}/npus +
// components.schemas.{NPU, NPUSlice, NPUUsage}.
package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	mocksrc "github.com/example/ocloud-edge/backend/pkg/datasource/mock"
)

// npuResourceKey is the registry mapping key for NPU handlers. Mirrors the
// `mapping.npus` entry in config.yaml (per backend/CLAUDE.md §4.2).
const npuResourceKey = "npus"

// ListNPUs handles GET /api/v1/nodes/:nodeName/npus.
//
// Returns the NPUs whose nodeName matches the URL parameter. Each returned
// NPU carries its slices array (per the AC for P1-T-104 — slice status MUST
// be visible).
//
// 404 when the node has no NPUs in the fixture (treated as "unknown node"
// per the mock's single-resource view; see mock/npu.go for the rationale).
// 500 when no datasource is wired for npus or the source fails.
func (h *Handler) ListNPUs(c *gin.Context) {
	name := c.Param("nodeName")
	if name == "" {
		respondError(c, http.StatusBadRequest, CodeBadRequest,
			"nodeName path parameter is required", nil)
		return
	}

	src := h.npuSource()
	if src == nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"no datasource configured for npus", nil)
		return
	}

	npus, err := src.ListNPUs(c.Request.Context(), name)
	if err != nil {
		if errors.Is(err, mocksrc.ErrNPUNodeNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound,
				"node not found", map[string]interface{}{
					"resource": "node",
					"name":     name,
				})
			return
		}
		h.Logger.Error("ListNPUs failed",
			zap.String("source", src.Name()),
			zap.String("nodeName", name),
			zap.Error(err))
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"failed to list NPUs", map[string]interface{}{
				"source": src.Name(),
			})
		return
	}

	c.JSON(http.StatusOK, npus)
}

// npuSource resolves the source backing /npus via the registry mapping. Falls
// back to the single registered source when no mapping entry exists — keeps
// minimal configs (one mock source, no explicit mapping) working. Same shape
// as node.go's nodeSource so handler files stay symmetric.
func (h *Handler) npuSource() datasource.Source {
	if h == nil || h.Registry == nil {
		return nil
	}
	if src := h.Registry.SourceFor(npuResourceKey); src != nil {
		return src
	}
	if len(h.Registry.Sources) == 1 {
		for _, s := range h.Registry.Sources {
			return s
		}
	}
	return nil
}
