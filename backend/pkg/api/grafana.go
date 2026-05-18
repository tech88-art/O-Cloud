// Package api — Grafana URL signing handler (P1-T-205).
//
// Endpoint:
//   - GET /api/v1/grafana/url?dashboard=<key>[&var-*=<val>...]
//     → { "url": "<base>/d/<uid>/<slug>?orgId=1&kiosk=tv&var-<...>=<...>" }
//
// Phase 1 just rewrites the request into a Grafana embed URL — there is no
// signing or token issuance yet (mirrors components.schemas.GrafanaURL where
// embedToken is nullable). dashboard keys are gated by a hard-coded whitelist
// because the frontend Metrics page (P1-T-208) only embeds the five fixed
// dashboards listed in deploy/CLAUDE.md §3.6.
//
// Contract: docs/api-contract.yaml /api/v1/grafana/url + GrafanaURL schema.
package api

import (
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

// defaultGrafanaBaseURL is the fallback when neither config.yaml's
// grafana.baseUrl nor the Handler.GrafanaBaseURL field is set. Matches the
// docker-compose dev stack (deploy/dev/docker-compose.yaml — Grafana on 3001).
const defaultGrafanaBaseURL = "http://localhost:3001"

// varParamPrefix is the Grafana dashboard-variable query-string convention
// (e.g. ?var-cluster=cluster-prod-a-01). We forward any key with this prefix
// verbatim, preserving Grafana's own conventions instead of inventing a
// translation layer.
const varParamPrefix = "var-"

// dashboardKeyToRef maps the public, snake_case dashboard keys the frontend
// asks for to Grafana's (uid, slug) pair. Keys are the whitelist; an unknown
// key yields a 400.
//
// The five entries here mirror deploy/CLAUDE.md §3.6 1:1 — keep these in sync
// when a new dashboard JSON lands under deploy/dev/grafana/dashboards/. The
// slug is the kebab-case form of the dashboard title (Grafana's own URL
// convention).
//
// Slug values come from running each dashboard JSON title through Grafana's
// own slugify (lowercased title with non-word chars → "-"); they're stable
// once a dashboard ships.
var dashboardKeyToRef = map[string]dashboardRef{
	"cluster_overview":  {UID: "cluster-overview", Slug: "cluster-overview"},
	"node_detail":       {UID: "node-detail", Slug: "node-detail"},
	"npu_detail":        {UID: "npu-detail", Slug: "npu-detail"},
	"workload_business": {UID: "workload-business", Slug: "workload-business"},
	"workload_resource": {UID: "workload-resource", Slug: "workload-resource"},
}

type dashboardRef struct {
	UID  string
	Slug string
}

// grafanaURLResponse mirrors components.schemas.GrafanaURL. Phase 1 only
// populates `url`; expiresAt and embedToken arrive in Phase 2 when JWT-based
// embed-token signing lands. Both fields are nullable in the contract so
// omitting them is contract-legal.
type grafanaURLResponse struct {
	URL string `json:"url"`
}

// GetGrafanaURL handles GET /api/v1/grafana/url.
//
// Query params:
//   - dashboard (required): whitelist key (see dashboardKeyToRef).
//   - var-*    (optional, repeatable): forwarded verbatim into the Grafana
//     URL as dashboard-variable substitutions.
//
// Errors:
//   - missing or empty dashboard → 400 BadRequest.
//   - dashboard key not in whitelist → 400 BadRequest (Details.allowed lists
//     the accepted keys so the frontend can surface a useful message).
func (h *Handler) GetGrafanaURL(c *gin.Context) {
	key := c.Query("dashboard")
	if key == "" {
		respondError(c, http.StatusBadRequest, CodeBadRequest,
			"dashboard query parameter is required", nil)
		return
	}

	ref, ok := dashboardKeyToRef[key]
	if !ok {
		respondError(c, http.StatusBadRequest, CodeBadRequest,
			"unknown dashboard key",
			map[string]interface{}{
				"dashboard": key,
				"allowed":   allowedDashboardKeys(),
			})
		return
	}

	c.JSON(http.StatusOK, grafanaURLResponse{
		URL: buildGrafanaEmbedURL(h.grafanaBaseURL(), ref, c.Request.URL.Query()),
	})
}

// grafanaBaseURL returns the configured base URL, falling back to the dev
// default when the field is empty. Keeping the fallback in the handler (vs.
// at config-load) means tests can spin up a router without populating
// GrafanaBaseURL and still get a deterministic URL shape.
func (h *Handler) grafanaBaseURL() string {
	if h == nil || strings.TrimSpace(h.GrafanaBaseURL) == "" {
		return defaultGrafanaBaseURL
	}
	// Strip a single trailing slash so `<base>/d/<uid>/...` doesn't yield
	// a "//" segment when operators configure `http://grafana/` with a
	// trailing slash.
	return strings.TrimRight(h.GrafanaBaseURL, "/")
}

// buildGrafanaEmbedURL composes the final Grafana URL. Format:
//
//	<base>/d/<uid>/<slug>?orgId=1&kiosk=tv[&var-...=...]
//
// orgId=1 is the dev stack default (Grafana's "Main Org"); kiosk=tv strips
// the navigation chrome for iframe embedding. Multi-tenancy will replace
// orgId with a per-org lookup in Phase 2.
//
// var-* params are forwarded verbatim. Keys are sorted so the URL is stable
// regardless of map iteration order — important for test assertions and for
// any downstream caching.
func buildGrafanaEmbedURL(base string, ref dashboardRef, q url.Values) string {
	out := url.Values{}
	out.Set("orgId", "1")
	out.Set("kiosk", "tv")

	// Collect var-* keys in sorted order. Each key may carry multiple values
	// (Grafana repeats the param for multi-valued template variables); we
	// preserve all of them.
	varKeys := make([]string, 0, len(q))
	for k := range q {
		if strings.HasPrefix(k, varParamPrefix) {
			varKeys = append(varKeys, k)
		}
	}
	sort.Strings(varKeys)
	for _, k := range varKeys {
		for _, v := range q[k] {
			out.Add(k, v)
		}
	}

	return base + "/d/" + ref.UID + "/" + ref.Slug + "?" + out.Encode()
}

// allowedDashboardKeys returns the whitelist as a sorted []string. Returning
// a fresh slice each call keeps the response map immutable to callers.
func allowedDashboardKeys() []string {
	keys := make([]string, 0, len(dashboardKeyToRef))
	for k := range dashboardKeyToRef {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
