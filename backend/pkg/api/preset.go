// Package api — Preset handlers (P1-T-203).
//
// Endpoints:
//   - GET /api/v1/presets           → []Preset (slim view, no manifest)
//   - GET /api/v1/presets/:presetId → PresetDetail (full manifest + params)
//
// Contract: see docs/api-contract.yaml /api/v1/presets + /api/v1/presets/{presetId}
// + components.schemas.{Preset, PresetDetail}.
package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	mocksrc "github.com/example/ocloud-edge/backend/pkg/datasource/mock"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// presetResourceKey is the registry mapping key for preset handlers. Mirrors
// the `mapping.presets` entry in config.yaml (per backend/CLAUDE.md §4.2).
const presetResourceKey = "presets"

// ListPresets handles GET /api/v1/presets.
//
// Returns a JSON array of components.schemas.Preset (slim view, manifest
// omitted). Empty fixture → 200 with `[]` per the OpenAPI contract.
//
// Errors:
//   - no datasource mapped for presets → 500 InternalError
//   - source.ListPresets returns error  → 500 InternalError
func (h *Handler) ListPresets(c *gin.Context) {
	src := h.presetSource()
	if src == nil {
		h.Logger.Error("no datasource mapped for presets")
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"no datasource mapped for presets", nil)
		return
	}
	presets, err := src.ListPresets(c.Request.Context())
	if err != nil {
		h.Logger.Error("list presets", zap.String("source", src.Name()), zap.Error(err))
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"failed to list presets", nil)
		return
	}
	// Contract demands an array, never null. Allocate an empty slice when the
	// source returns nil so encoding/json emits `[]` not `null`.
	if presets == nil {
		presets = []*model.Preset{}
	}
	c.JSON(http.StatusOK, presets)
}

// GetPreset handles GET /api/v1/presets/:presetId.
//
// Returns components.schemas.PresetDetail on 200. 404 with the canonical Error
// envelope when the id does not exist; mock.ErrPresetNotFound is the sentinel
// the mock source uses, any other error is treated as a 500.
func (h *Handler) GetPreset(c *gin.Context) {
	id := c.Param("presetId")
	if id == "" {
		respondError(c, http.StatusBadRequest, CodeBadRequest,
			"presetId is required", nil)
		return
	}
	src := h.presetSource()
	if src == nil {
		h.Logger.Error("no datasource mapped for presets")
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"no datasource mapped for presets", nil)
		return
	}
	preset, err := src.GetPreset(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, mocksrc.ErrPresetNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound,
				"preset not found",
				map[string]interface{}{"resource": "preset", "id": id})
			return
		}
		h.Logger.Error("get preset",
			zap.String("source", src.Name()),
			zap.String("id", id),
			zap.Error(err))
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"failed to get preset", nil)
		return
	}
	c.JSON(http.StatusOK, preset)
}

// presetSource resolves the source backing /presets via the registry mapping.
// Falls back to the lone registered source when no mapping entry exists —
// keeps minimal configs (one mock source, no explicit mapping) working. Same
// shape as node.go's nodeSource so handler files stay symmetric.
func (h *Handler) presetSource() datasource.Source {
	if h == nil || h.Registry == nil {
		return nil
	}
	if src := h.Registry.SourceFor(presetResourceKey); src != nil {
		return src
	}
	if len(h.Registry.Sources) == 1 {
		for _, s := range h.Registry.Sources {
			return s
		}
	}
	return nil
}
