// Package api — workload handlers (P1-T-201).
//
// Endpoints:
//   - GET /api/v1/workloads                       → []Workload (filters: namespace, type, status)
//   - GET /api/v1/workloads/:namespace/:name      → WorkloadDetail (404 if missing)
//
// Contract: see docs/api-contract.yaml /api/v1/workloads +
// /api/v1/workloads/{namespace}/{name} + components.schemas.{Workload,
// WorkloadDetail, Pod}. WorkloadDetail extends Workload with `pods` +
// `relations` (PD-pair / sidecar / init / peer) inline.
package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	mocksrc "github.com/example/ocloud-edge/backend/pkg/datasource/mock"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// workloadResourceKey is the mapping key used by Registry.SourceFor to resolve
// the datasource backing /workloads. Defined here (not in router) so the
// handler is self-documenting about which mapping entry it consults.
const workloadResourceKey = "workloads"

// ListWorkloads handles GET /api/v1/workloads.
//
// Query params (all optional, AND-combined):
//   - namespace : restrict to one namespace
//   - type      : inference | benchmark | training | other
//   - status    : pending | running | succeeded | failed | unknown
//
// We do NOT validate the enum values here — the OpenAPI spec marks the enums
// declaratively, but a typo in the frontend shouldn't break the page. Unknown
// values simply filter out every workload, surfacing as `[]`. This mirrors the
// node.go handler's permissive behavior on filter values.
//
// Returns an empty array (never nil) on no match.
func (h *Handler) ListWorkloads(c *gin.Context) {
	src := h.workloadSource()
	if src == nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"no datasource configured for workloads", nil)
		return
	}

	filter := model.WorkloadFilter{
		Namespace:            c.Query("namespace"),
		Type:                 c.Query("type"),
		Status:               c.Query("status"),
		IncludeSliceBindings: parseWorkloadBoolQuery(c, "includeSliceBindings"),
		IncludeO2DMSExposed:  parseWorkloadBoolQuery(c, "includeO2DMSExposed"),
		IncludeQuotaUsage:    parseWorkloadBoolQuery(c, "includeQuotaUsage"),
		IncludeScaleHistory:  parseWorkloadBoolQuery(c, "includeScaleHistory"),
	}

	out, err := src.ListWorkloads(c.Request.Context(), filter)
	if err != nil {
		h.Logger.Error("ListWorkloads failed",
			zap.String("source", src.Name()),
			zap.Error(err))
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"failed to list workloads", map[string]interface{}{
				"source": src.Name(),
			})
		return
	}

	// Contract demands an array, never null. Allocate an empty slice when the
	// source returns nil so encoding/json emits `[]` not `null`.
	if out == nil {
		out = []*model.Workload{}
	}
	c.JSON(http.StatusOK, out)
}

// GetWorkloadDetail handles GET /api/v1/workloads/:namespace/:name.
//
// Returns 404 with the canonical Error envelope when the workload is not
// found. mock.ErrWorkloadNotFound is the canonical signal used by the mock
// source; any other error becomes a 500.
func (h *Handler) GetWorkloadDetail(c *gin.Context) {
	namespace := c.Param("namespace")
	name := c.Param("name")
	if namespace == "" || name == "" {
		respondError(c, http.StatusBadRequest, CodeBadRequest,
			"namespace and name path parameters are required", nil)
		return
	}

	src := h.workloadSource()
	if src == nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"no datasource configured for workloads", nil)
		return
	}

	detail, err := src.GetWorkloadDetail(c.Request.Context(), namespace, name)
	if err != nil {
		if errors.Is(err, mocksrc.ErrWorkloadNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound,
				"workload not found", map[string]interface{}{
					"resource":  "workload",
					"namespace": namespace,
					"name":      name,
				})
			return
		}
		h.Logger.Error("GetWorkloadDetail failed",
			zap.String("source", src.Name()),
			zap.String("namespace", namespace),
			zap.String("name", name),
			zap.Error(err))
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"failed to get workload detail", map[string]interface{}{
				"source": src.Name(),
			})
		return
	}

	c.JSON(http.StatusOK, detail)
}

// parseWorkloadBoolQuery converts a Gin query param into a bool. Empty /
// missing / unparseable → false. Same truthy parsing as
// strconv.ParseBool (1/t/T/TRUE/true/True). Mirrors cluster.go's
// parseTopologyBool but takes the Gin context directly so the handler
// reads cleaner (P6-T-102).
func parseWorkloadBoolQuery(c *gin.Context, key string) bool {
	raw := c.Query(key)
	if raw == "" {
		return false
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false
	}
	return v
}

// workloadSource resolves the source backing /workloads via the registry
// mapping, with a single-source fallback so a minimal config (one mock source,
// no explicit mapping) still works. Mirrors node.go's nodeSource and npu.go's
// npuSource for symmetry.
func (h *Handler) workloadSource() datasource.Source {
	if h == nil || h.Registry == nil {
		return nil
	}
	if src := h.Registry.SourceFor(workloadResourceKey); src != nil {
		return src
	}
	// Fallback: if exactly one source is registered, use it.
	if len(h.Registry.Sources) == 1 {
		for _, s := range h.Registry.Sources {
			return s
		}
	}
	return nil
}
