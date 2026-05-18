// Package api — metrics handlers (P1-T-204, RFC-003 hardened with var-slice).
//
// Endpoints:
//   - POST /api/v1/metrics/query    → MetricQueryResponse for one template
//   - GET  /api/v1/metrics/templates → []MetricTemplate (white-list)
//
// White-list rationale (spec / backend/CLAUDE.md):
//   The contract forbids arbitrary PromQL passthrough for security — only the
//   templates registered in pkg/datasource/mock/metrics.go (or any future
//   prometheus.Source) may be queried. Variables get substituted server-side.
//
// var-slice dimension (RFC-003 hardened per spec F4a "最细切分粒度"):
//   The three NPU utilization templates (npu_aicore_util / npu_vram_util /
//   npu_bandwidth_util) accept `var-slice=<sliceId>`. The handler accepts
//   slice references via either the JSON body (variables.slice) or the
//   conventional `var-slice` query param — frontend dashboards typically
//   ship vars as query params so they roundtrip in the URL.
//
// Contract: see docs/api-contract.yaml /api/v1/metrics/query +
// /api/v1/metrics/templates + components.schemas.{MetricQueryRequest,
// MetricQueryResponse, MetricTemplate}.
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

// resourceMetrics is the registry mapping key for metrics handlers. Mirrors
// the `mapping.metrics` entry in config.yaml (per backend/CLAUDE.md §4.2).
const resourceMetrics = "metrics"

// metricQueryRequest is the wire shape for POST /metrics/query. Mirrors
// components.schemas.MetricQueryRequest. We define it inline (rather than
// importing from model) because the OpenAPI schema is the source of truth
// and this handler is the only consumer.
type metricQueryRequest struct {
	TemplateID string            `json:"templateId"`
	Variables  map[string]string `json:"variables"`
	Range      model.TimeRange   `json:"range"`
}

// QueryMetric handles POST /api/v1/metrics/query and the convenience
// GET shape /api/v1/metrics/query?templateId=...&var-foo=bar&from=...&to=...
//
// Why support GET as well: the AC explicitly tests
// `/metrics/query?templateId=npu_aicore_util&var-slice=...` which is the
// shape Grafana / frontend dashboards use to embed metric panels. POST
// remains canonical per the OpenAPI contract; both paths share the same
// validation logic.
func (h *Handler) QueryMetric(c *gin.Context) {
	req, err := parseMetricQuery(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeBadRequest, err.Error(), nil)
		return
	}
	if req.TemplateID == "" {
		respondError(c, http.StatusBadRequest, CodeBadRequest,
			"templateId is required", nil)
		return
	}

	src := h.metricsSource()
	if src == nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"no datasource configured for metrics", nil)
		return
	}

	resp, err := src.QueryMetric(c.Request.Context(), req.TemplateID, req.Variables, req.Range)
	if err != nil {
		// Unknown template / missing required variable / unknown slice id are
		// all client errors. Mock surfaces them via typed sentinels so the
		// handler can map cleanly to 400 with the offending detail.
		switch {
		case errors.Is(err, mocksrc.ErrUnknownTemplate):
			respondError(c, http.StatusBadRequest, CodeBadRequest,
				"unknown templateId", map[string]interface{}{
					"templateId": req.TemplateID,
				})
			return
		case errors.Is(err, mocksrc.ErrMissingVariable):
			respondError(c, http.StatusBadRequest, CodeBadRequest,
				err.Error(), map[string]interface{}{
					"templateId": req.TemplateID,
				})
			return
		case errors.Is(err, mocksrc.ErrUnknownSlice):
			respondError(c, http.StatusBadRequest, CodeBadRequest,
				"unknown slice id", map[string]interface{}{
					"templateId": req.TemplateID,
					"slice":      req.Variables["slice"],
				})
			return
		}
		h.Logger.Error("QueryMetric failed",
			zap.String("source", src.Name()),
			zap.String("templateId", req.TemplateID),
			zap.Error(err))
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"failed to query metric", map[string]interface{}{
				"source": src.Name(),
			})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// ListTemplates handles GET /api/v1/metrics/templates. Returns the white-list
// in declaration order; clients can rely on the order being stable across
// requests (it's the package-level slice in mock/metrics.go).
//
// Returns 500 when no metrics source is configured — different from the
// /query handler in that the templates list is a static white-list, but the
// access path still goes through the Source interface so PHASE-2 can swap
// in a prometheus source without changing the handler.
func (h *Handler) ListTemplates(c *gin.Context) {
	src := h.metricsSource()
	if src == nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"no datasource configured for metrics", nil)
		return
	}

	// Type assert to the mock-specific lister. PHASE-2 (prometheus source)
	// will either implement the same method or we'll lift ListTemplates onto
	// the Source interface — for P1-T-204 the mock is the only template
	// provider, so a concrete type assertion keeps the wiring simple.
	mockSrc, ok := src.(*mocksrc.Source)
	if !ok {
		respondError(c, http.StatusInternalServerError, CodeInternalError,
			"metrics templates not supported by configured source", nil)
		return
	}

	tpls := mockSrc.ListTemplates(c.Request.Context())

	// Render to the contract shape: each template surfaces id / name / promQL
	// + variables[] with {name, required, description}.
	out := make([]map[string]interface{}, 0, len(tpls))
	for _, t := range tpls {
		variables := make([]map[string]interface{}, 0, len(t.Variables))
		for _, v := range t.Variables {
			variables = append(variables, map[string]interface{}{
				"name":        v.Name,
				"required":    v.Required,
				"description": v.Description,
			})
		}
		out = append(out, map[string]interface{}{
			"id":          t.ID,
			"name":        t.Name,
			"unit":        t.Unit,
			"description": t.Description,
			"promQL":      t.PromQL,
			"variables":   variables,
		})
	}
	c.JSON(http.StatusOK, out)
}

// parseMetricQuery accepts both POST JSON body and GET query-string forms.
// Query-string form mirrors what Grafana / frontend embeds use:
//
//	GET /metrics/query?templateId=npu_aicore_util&var-slice=<id>&from=...&to=...&step=30s
//
// JSON form mirrors the OpenAPI contract:
//
//	POST /metrics/query  body={"templateId": "...", "variables": {...}, "range": {...}}
//
// `var-<name>` query params populate variables[name]. The `from`/`to`/`step`
// query params populate range.start/end/step (Grafana convention). For POST
// requests we still merge query params onto the parsed body so callers can
// override defaults in the URL.
func parseMetricQuery(c *gin.Context) (metricQueryRequest, error) {
	var req metricQueryRequest
	if req.Variables == nil {
		req.Variables = map[string]string{}
	}

	if c.Request.Method == http.MethodPost {
		if c.Request.ContentLength > 0 {
			if err := c.ShouldBindJSON(&req); err != nil {
				return req, err
			}
		}
	}
	if req.Variables == nil {
		req.Variables = map[string]string{}
	}

	// Pull templateId from query if absent (GET form). POST body wins when
	// both are present.
	if req.TemplateID == "" {
		req.TemplateID = c.Query("templateId")
	}

	// `var-<name>` params → variables[<name>]. Walk c.Request.URL.Query so
	// we can spot the prefix uniformly.
	for k, v := range c.Request.URL.Query() {
		if !strings.HasPrefix(k, "var-") || len(v) == 0 {
			continue
		}
		name := strings.TrimPrefix(k, "var-")
		if name == "" || v[0] == "" {
			continue
		}
		req.Variables[name] = v[0]
	}

	// Range from query params (Grafana convention: from / to / step).
	if req.Range.Start == "" {
		req.Range.Start = c.Query("from")
	}
	if req.Range.End == "" {
		req.Range.End = c.Query("to")
	}
	if req.Range.Step == "" {
		req.Range.Step = c.Query("step")
	}

	return req, nil
}

// metricsSource resolves the source backing /metrics/* via the registry
// mapping. Falls back to the lone registered source for minimal configs
// (matches the pattern in node.go / npu.go).
func (h *Handler) metricsSource() datasource.Source {
	if h == nil || h.Registry == nil {
		return nil
	}
	if src := h.Registry.SourceFor(resourceMetrics); src != nil {
		return src
	}
	if len(h.Registry.Sources) == 1 {
		for _, s := range h.Registry.Sources {
			return s
		}
	}
	return nil
}
