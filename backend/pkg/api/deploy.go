// Package api — Deploy handlers (P1-T-202).
//
// Endpoints:
//   - POST   /api/v1/deploy             → DeployResponse (201 Created)
//   - DELETE /api/v1/deploy/:deployId   → 204 No Content
//
// Contract: see docs/api-contract.yaml /api/v1/deploy + /api/v1/deploy/{deployId}
// + components.schemas.{DeployRequest, DeployResponse}.
//
// Error mapping the handler owns (mock source surfaces sentinels):
//   - ErrPresetNotFoundForDeploy → 404 NotFound (presetId in body absent)
//   - ErrSliceConflict / ErrInsufficientCapacity → 409 Conflict
//   - ErrInvalidDeployRequest    → 400 BadRequest
//   - ErrDeployNotFound          → 404 NotFound (DELETE)
//   - anything else              → 500 InternalError
package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	mocksrc "github.com/example/ocloud-edge/backend/pkg/datasource/mock"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// deployResourceKey is the registry mapping key for deploy mutations. We
// resolve via Registry.SourceFor("deploy") with the single-source fallback so
// minimal configs (one mock source, no explicit mapping) keep working —
// mirrors workloadSource / presetSource for symmetry.
const deployResourceKey = "deploy"

// Deploy handles POST /api/v1/deploy.
//
// The contract returns 201 Created on success with a DeployResponse body.
// On conflict (manual mode targeting an already-allocated slice, or auto mode
// out of capacity) we emit 409 with details.resource="slice" and details.id
// pointing at the offending slice (manual case) — keeps the frontend able to
// surface a useful message without a second round-trip.
func (h *Handler) Deploy(c *gin.Context) {
	src := h.deploySource()
	if src == nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"no datasource configured for deploy", nil)
		return
	}

	var req model.DeployRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest,
			"invalid JSON body: "+err.Error(), nil)
		return
	}

	resp, err := src.Deploy(c.Request.Context(), &req)
	if err != nil {
		switch {
		case errors.Is(err, mocksrc.ErrInvalidDeployRequest):
			respondError(c, http.StatusBadRequest, CodeBadRequest,
				"invalid deploy request: "+err.Error(),
				map[string]interface{}{"presetId": req.PresetID})
			return
		case errors.Is(err, mocksrc.ErrPresetNotFoundForDeploy):
			respondError(c, http.StatusNotFound, CodeNotFound,
				"preset not found", map[string]interface{}{
					"resource": "preset",
					"id":       req.PresetID,
				})
			return
		case errors.Is(err, mocksrc.ErrSliceConflict),
			errors.Is(err, mocksrc.ErrInsufficientCapacity):
			respondError(c, http.StatusConflict, CodeConflict,
				err.Error(),
				map[string]interface{}{
					"resource": "slice",
					"reason":   conflictReason(err),
				})
			return
		}
		h.Logger.Error("Deploy failed",
			zap.String("source", src.Name()),
			zap.String("presetId", req.PresetID),
			zap.Error(err))
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"failed to deploy", map[string]interface{}{"source": src.Name()})
		return
	}
	c.JSON(http.StatusCreated, resp)
}

// DeleteDeploy handles DELETE /api/v1/deploy/:deployId.
//
// Returns 204 No Content on success; 404 with the canonical Error envelope on
// unknown id. mock.ErrDeployNotFound is the canonical signal — any other error
// becomes 500.
func (h *Handler) DeleteDeploy(c *gin.Context) {
	deployID := c.Param("deployId")
	if deployID == "" {
		respondError(c, http.StatusBadRequest, CodeBadRequest,
			"deployId is required", nil)
		return
	}
	src := h.deploySource()
	if src == nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"no datasource configured for deploy", nil)
		return
	}

	if err := src.DeleteDeploy(c.Request.Context(), deployID); err != nil {
		switch {
		case errors.Is(err, mocksrc.ErrDeployNotFound):
			respondError(c, http.StatusNotFound, CodeNotFound,
				"deploy not found", map[string]interface{}{
					"resource": "deploy",
					"id":       deployID,
				})
			return
		case errors.Is(err, mocksrc.ErrInvalidDeployRequest):
			respondError(c, http.StatusBadRequest, CodeBadRequest,
				err.Error(), nil)
			return
		}
		h.Logger.Error("DeleteDeploy failed",
			zap.String("source", src.Name()),
			zap.String("deployId", deployID),
			zap.Error(err))
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"failed to delete deploy", map[string]interface{}{"source": src.Name()})
		return
	}
	c.Status(http.StatusNoContent)
}

// deploySource resolves the source backing /deploy via the registry mapping.
// Falls back to the lone registered source when no mapping entry exists —
// keeps minimal configs (one mock source, no explicit mapping) working. Same
// shape as workloadSource / presetSource.
func (h *Handler) deploySource() datasource.Source {
	if h == nil || h.Registry == nil {
		return nil
	}
	if src := h.Registry.SourceFor(deployResourceKey); src != nil {
		return src
	}
	if len(h.Registry.Sources) == 1 {
		for _, s := range h.Registry.Sources {
			return s
		}
	}
	return nil
}

// conflictReason classifies the 409 cause without leaking the entire wrapped
// error chain into details.reason — keeps the wire payload terse for the
// frontend toast.
func conflictReason(err error) string {
	switch {
	case errors.Is(err, mocksrc.ErrSliceConflict):
		return "slice-allocated"
	case errors.Is(err, mocksrc.ErrInsufficientCapacity):
		return "insufficient-capacity"
	default:
		return strings.TrimSpace(err.Error())
	}
}
