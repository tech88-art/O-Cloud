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

// resourceClusters is the registry key for cluster handlers. Mirrors the
// `mapping.clusters` entry in config.yaml (per backend/CLAUDE.md §4.2).
const resourceClusters = "clusters"

// resourceTopology is the registry key for topology handler. Topology may be
// served by a different source than /clusters in later phases (k8s + crd
// joined), so the mapping is separate. Falls back to the clusters source via
// topologySource() when no explicit topology mapping exists.
const resourceTopology = "topology"

// defaultTopologyDepth mirrors the OpenAPI `default: slice` for the depth
// query param.
const defaultTopologyDepth = "slice"

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

// GetClusterTopology handles GET /api/v1/clusters/:clusterId/topology.
//
// Query params:
//   - depth (string, default "slice"): node | npu | slice. Unknown values
//     fall through to "slice" — same as the aggregator default. We don't
//     emit 400 here because the OpenAPI spec marks the param as enum +
//     default rather than strict, and a frontend typo shouldn't break the
//     page.
//   - includeFabric (bool, default false; P1-T-211 / ADR-0004): when true the
//     response gains `type=switch` nodes + `type=fabric-link` edges drawn
//     from the source's fabric fixtures. Default false preserves T102
//     bytes (zero-regression AC). Truthy parsing follows strconv.ParseBool —
//     a bad value yields false rather than 400 (frontend-typo grace).
//   - includeWorkloads (bool, default false; P1-T-213 / ADR-0005): when true
//     the response gains `type=workload` + `type=pod` nodes plus
//     `type=binds-to` (pod→slice) and `type=pd-pair` (pod↔pod) edges drawn
//     from the source's workloads fixture. Default false preserves
//     T102/T211 bytes. Same lenient bool parsing as includeFabric.
//     includeFabric and includeWorkloads compose independently.
//
// Errors:
//   - cluster id unknown → 404 with the canonical Error envelope.
//   - no datasource mapped for topology nor clusters → 500.
//   - any other source error → 500.
//
// Contract: returns components.schemas.Topology (G6-compatible graph).
func (h *Handler) GetClusterTopology(c *gin.Context) {
	id := c.Param("clusterId")
	if id == "" {
		respondError(c, http.StatusBadRequest, CodeBadRequest,
			"clusterId is required", nil)
		return
	}

	depth := c.DefaultQuery("depth", defaultTopologyDepth)
	includeFabric := parseTopologyBool(c.Query("includeFabric"))
	includeWorkloads := parseTopologyBool(c.Query("includeWorkloads"))

	src := h.topologySource()
	if src == nil {
		h.Logger.Error("no datasource mapped for topology")
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"no datasource mapped for topology", nil)
		return
	}

	// Dispatch: when either fabric or workloads is requested we go through
	// the option-bag signature; otherwise we keep calling the legacy
	// GetTopology so any test double that overrides only that method (e.g.
	// erroringTopologySource) still funnels its sentinel error through.
	var topo *model.Topology
	var err error
	if includeFabric || includeWorkloads {
		topo, err = src.GetTopologyWithFabric(c.Request.Context(), id, depth,
			datasource.TopologyOptions{
				IncludeFabric:    includeFabric,
				IncludeWorkloads: includeWorkloads,
			})
	} else {
		topo, err = src.GetTopology(c.Request.Context(), id, depth)
	}
	if err != nil {
		// ErrTopologyClusterNotFound wraps ErrClusterNotFound, so a single
		// errors.Is on the leaf sentinel catches both. Order matters less
		// here — we only emit one 404 shape.
		if errors.Is(err, mocksrc.ErrClusterNotFound) {
			respondError(c, http.StatusNotFound, CodeNotFound,
				"cluster not found",
				map[string]interface{}{"resource": "cluster", "id": id})
			return
		}
		h.Logger.Error("get topology",
			zap.String("source", src.Name()),
			zap.String("clusterId", id),
			zap.String("depth", depth),
			zap.Bool("includeFabric", includeFabric),
			zap.Bool("includeWorkloads", includeWorkloads),
			zap.Error(err))
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"failed to get topology", nil)
		return
	}
	c.JSON(http.StatusOK, topo)
}

// parseTopologyBool converts a raw query param value (e.g. `?includeFabric=`,
// `?includeWorkloads=`) into a bool. Empty / missing / unparseable → false.
// Accepts the same truthy spellings strconv.ParseBool does (1/t/T/TRUE/true/
// True). Bad values do NOT 400 — see the depth handling note for the
// rationale (frontend typos shouldn't break the page).
func parseTopologyBool(raw string) bool {
	if raw == "" {
		return false
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false
	}
	return v
}

// topologySource resolves the source backing /clusters/:id/topology. Prefers
// an explicit topology mapping, falls back to the clusters mapping (most
// configs alias them), then to the lone registered source. Mirrors the
// pattern in node.go / npu.go.
func (h *Handler) topologySource() datasource.Source {
	if h == nil || h.Registry == nil {
		return nil
	}
	if src := h.Registry.SourceFor(resourceTopology); src != nil {
		return src
	}
	if src := h.Registry.SourceFor(resourceClusters); src != nil {
		return src
	}
	if len(h.Registry.Sources) == 1 {
		for _, s := range h.Registry.Sources {
			return s
		}
	}
	return nil
}
