package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// newGrafanaRouter spins up an api.Router with the supplied baseURL. The
// /grafana/url handler does not touch the datasource registry, so we leave
// Registry nil — keeps tests independent of mock fixture setup.
func newGrafanaRouter(t *testing.T, baseURL string) *gin.Engine {
	t.Helper()
	h := NewHandler(nil, nil)
	h.GrafanaBaseURL = baseURL
	return NewRouter(h, RouterOptions{})
}

// decodeGrafanaURL pulls the URL out of the response body and parses it.
// Returns the parsed url.URL — assertions on path / query / scheme go through
// the parsed struct so we never rely on substring matching against URL-
// encoded query strings.
func decodeGrafanaURL(t *testing.T, rec *httptest.ResponseRecorder) *url.URL {
	t.Helper()
	var got grafanaURLResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	parsed, err := url.Parse(got.URL)
	require.NoError(t, err, "response url must parse: %q", got.URL)
	return parsed
}

func TestGetGrafanaURL_HappyPath_KnownDashboard(t *testing.T) {
	router := newGrafanaRouter(t, "http://grafana.test:3001")

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/grafana/url?dashboard=cluster_overview", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json; charset=utf-8",
		rec.Header().Get("Content-Type"))

	u := decodeGrafanaURL(t, rec)
	assert.Equal(t, "http", u.Scheme)
	assert.Equal(t, "grafana.test:3001", u.Host)
	assert.Equal(t, "/d/cluster-overview/cluster-overview", u.Path)
	q := u.Query()
	assert.Equal(t, "1", q.Get("orgId"))
	assert.Equal(t, "tv", q.Get("kiosk"))
}

func TestGetGrafanaURL_FallbackBaseURL_WhenUnset(t *testing.T) {
	// Empty GrafanaBaseURL → handler substitutes defaultGrafanaBaseURL.
	router := newGrafanaRouter(t, "")

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/grafana/url?dashboard=node_detail", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	u := decodeGrafanaURL(t, rec)
	assert.True(t, strings.HasPrefix(u.String(), defaultGrafanaBaseURL+"/"),
		"expected URL to start with %q, got %q", defaultGrafanaBaseURL, u.String())
	assert.Equal(t, "/d/node-detail/node-detail", u.Path)
}

func TestGetGrafanaURL_TrailingSlashBaseURL_NormalizedOnce(t *testing.T) {
	// Operators sometimes configure `http://grafana/` with a trailing slash.
	// We strip exactly one to keep the path "/d/..." rather than "//d/...".
	router := newGrafanaRouter(t, "http://grafana.test:3001/")

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/grafana/url?dashboard=npu_detail", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	u := decodeGrafanaURL(t, rec)
	assert.Equal(t, "/d/npu-detail/npu-detail", u.Path,
		"trailing slash on base must not produce //d/...")
}

func TestGetGrafanaURL_VarPassthrough_SingleAndMultiple(t *testing.T) {
	router := newGrafanaRouter(t, "http://grafana.test:3001")

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/grafana/url?dashboard=cluster_overview"+
			"&var-cluster=cluster-prod-a-01"+
			"&var-node=worker-site-a-01"+
			"&unrelated=ignored", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	u := decodeGrafanaURL(t, rec)
	q := u.Query()

	// Whitelisted prefix var-* forwards verbatim.
	assert.Equal(t, "cluster-prod-a-01", q.Get("var-cluster"))
	assert.Equal(t, "worker-site-a-01", q.Get("var-node"))

	// Non-var-* params are NOT forwarded (no allow-list bleed).
	assert.Empty(t, q.Get("unrelated"))

	// orgId + kiosk still present.
	assert.Equal(t, "1", q.Get("orgId"))
	assert.Equal(t, "tv", q.Get("kiosk"))
}

func TestGetGrafanaURL_VarPassthrough_RepeatedValues(t *testing.T) {
	// Grafana repeats a query param when a template variable has multiple
	// selected values (e.g. var-node=a&var-node=b). We must preserve all of
	// them; url.Values does this via Add / [] semantics.
	router := newGrafanaRouter(t, "http://grafana.test:3001")

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/grafana/url?dashboard=workload_resource"+
			"&var-node=worker-site-a-01"+
			"&var-node=worker-site-a-02", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	u := decodeGrafanaURL(t, rec)
	nodes := u.Query()["var-node"]
	assert.ElementsMatch(t, []string{"worker-site-a-01", "worker-site-a-02"}, nodes)
}

func TestGetGrafanaURL_UnknownDashboard_Returns400(t *testing.T) {
	router := newGrafanaRouter(t, "http://grafana.test:3001")

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/grafana/url?dashboard=does_not_exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var got model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, CodeBadRequest, got.Code)
	assert.Equal(t, "does_not_exist", got.Details["dashboard"])

	// Details.allowed must list every whitelist key so the frontend can
	// surface "unknown dashboard, did you mean X?" without an extra round
	// trip. Cast: JSON unmarshal yields []interface{} for a string array.
	rawAllowed, ok := got.Details["allowed"].([]interface{})
	require.True(t, ok, "allowed must be a string array, got %T",
		got.Details["allowed"])
	allowed := make([]string, len(rawAllowed))
	for i, v := range rawAllowed {
		allowed[i] = v.(string)
	}
	assert.ElementsMatch(t, []string{
		"cluster_overview",
		"node_detail",
		"npu_detail",
		"workload_business",
		"workload_resource",
	}, allowed)
}

func TestGetGrafanaURL_MissingDashboard_Returns400(t *testing.T) {
	router := newGrafanaRouter(t, "http://grafana.test:3001")

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/grafana/url", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)

	var got model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, CodeBadRequest, got.Code)
}

func TestGetGrafanaURL_AllWhitelistedKeys_Resolve(t *testing.T) {
	// Defensive: each documented key in deploy/CLAUDE.md §3.6 must round-trip
	// through the handler. If someone removes an entry from
	// dashboardKeyToRef, this test catches it before frontend breakage.
	router := newGrafanaRouter(t, "http://grafana.test:3001")

	for _, key := range []string{
		"cluster_overview",
		"node_detail",
		"npu_detail",
		"workload_business",
		"workload_resource",
	} {
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/grafana/url?dashboard="+key, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		require.Equalf(t, http.StatusOK, rec.Code,
			"dashboard key %q must be in whitelist", key)
		u := decodeGrafanaURL(t, rec)
		assert.Contains(t, u.Path, "/d/", "key %q produced %q", key, u.Path)
	}
}
