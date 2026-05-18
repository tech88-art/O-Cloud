// Package api — metrics handler tests (P1-T-204, RFC-003 hardened).
//
// Covers:
//   - ListTemplates lists at least 9 templates (white-list lower bound).
//   - QueryMetric happy path (POST + GET shape) returns 200 with 200 samples.
//   - Unknown templateId → 400 BadRequest.
//   - Missing required variable → 400 BadRequest.
//   - var-slice happy path → 200 with slice-shaped band (AC spec F4a).
//   - Unknown slice id → 400 BadRequest.
//   - Deterministic seeding — same query twice yields identical series.
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	mocksrc "github.com/example/ocloud-edge/backend/pkg/datasource/mock"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// metricsSlicesFixture is a minimal slices.json sufficient to validate
// var-slice happy path + unknown-slice 400. Status values mirror the band
// rules in mock/metrics.go (allocated → high util band, available → low).
const metricsSlicesFixture = `{
  "slices": [
    {
      "id": "worker-site-a-01-npu-3-slice-0",
      "parentNPU": "worker-site-a-01-npu-3",
      "template": "vir02",
      "aiCore": 8,
      "vramMiB": 16384,
      "status": "allocated",
      "allocatedTo": {"namespace": "ai-inference", "podName": "pod-vir02-1", "containerName": "main"}
    },
    {
      "id": "worker-site-a-01-npu-3-slice-1",
      "parentNPU": "worker-site-a-01-npu-3",
      "template": "vir02",
      "aiCore": 8,
      "vramMiB": 16384,
      "status": "available"
    }
  ]
}`

// metricsNPUsFixture is needed only so loadSlicesFlat does not error out — the
// metrics handler itself does not consult NPUs, but a real mock.Source
// constructed against a fixturesPath expects both files to be parseable when
// any one is read (see mock/topology.go loadSlicesFlat semantics).
const metricsNPUsFixture = `{"npus": []}`

// newMetricsTestHandler returns a handler wired against an on-disk
// slices.json + npus.json. The Source has metrics + slice loaders available
// so var-slice tests can validate against real fixture content.
func newMetricsTestHandler(t *testing.T) *Handler {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "slices.json"), []byte(metricsSlicesFixture), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "npus.json"), []byte(metricsNPUsFixture), 0o600))

	src := mocksrc.NewSource(dir)
	reg := &datasource.Registry{
		Sources: map[string]datasource.Source{"mock": src},
		Mapping: map[string]string{"metrics": "mock"},
	}
	return NewHandler(reg, nil)
}

// ---- 1) ListTemplates ------------------------------------------------------

func TestListTemplates_Returns9PlusEntries(t *testing.T) {
	h := newMetricsTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/templates", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var got []map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.GreaterOrEqual(t, len(got), 9, "AC requires 9+ templates; got %d", len(got))

	// Spot-check the three slice-aware templates declare a "slice" variable
	// (AC: var-slice dimension wired per spec F4a).
	want := map[string]bool{
		"npu_aicore_util":    false,
		"npu_vram_util":      false,
		"npu_bandwidth_util": false,
	}
	for _, tpl := range got {
		id, _ := tpl["id"].(string)
		if _, ok := want[id]; !ok {
			continue
		}
		vars, _ := tpl["variables"].([]interface{})
		sliceVarFound := false
		for _, v := range vars {
			vm, _ := v.(map[string]interface{})
			if name, _ := vm["name"].(string); name == "slice" {
				sliceVarFound = true
			}
		}
		assert.True(t, sliceVarFound, "template %q must declare a 'slice' variable (spec F4a 最细切分粒度)", id)
		want[id] = true
	}
	for id, ok := range want {
		assert.True(t, ok, "expected template %q in white-list", id)
	}
}

// ---- 2) QueryMetric — happy path (var-slice — the AC headline case) -------

func TestQueryMetric_VarSlice_ReturnsSliceSpecificSeries(t *testing.T) {
	h := newMetricsTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	// AC: `?templateId=npu_aicore_util&var-slice=<slice-id>` returns slice
	// time series.
	url := "/api/v1/metrics/query?templateId=npu_aicore_util" +
		"&var-slice=worker-site-a-01-npu-3-slice-0"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var resp model.MetricQueryResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Result, 1, "exactly one series per query")

	series := resp.Result[0]
	require.Equal(t, "npu_aicore_util", series.Metric["__name__"])
	require.Equal(t, "worker-site-a-01-npu-3-slice-0", series.Metric["slice"],
		"resolved slice id must echo back in metric labels")
	require.Len(t, series.Values, 200, "AC pins 200 samples per series")

	// Allocated slice → band 55..92 (see mock/metrics.go sliceDerivedBand).
	// Sanity-check the values lie inside the band.
	for i, sample := range series.Values {
		require.Len(t, sample, 2, "sample %d shape", i)
		vs, ok := sample[1].(string)
		require.True(t, ok, "sample value must be string per Prometheus convention")
		v, err := strconv.ParseFloat(vs, 64)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, v, 55.0, "sample %d below allocated band", i)
		assert.LessOrEqual(t, v, 92.0, "sample %d above allocated band", i)
	}
}

// Determinism: same query twice → same samples. Documents the seeded RNG
// contract so a future "let's add noise" refactor cannot silently break it.
func TestQueryMetric_Determinism_RepeatedQuerySameSamples(t *testing.T) {
	h := newMetricsTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	mk := func() *model.MetricQueryResponse {
		body := `{"templateId":"npu_vram_util","variables":{"slice":"worker-site-a-01-npu-3-slice-0"},"range":{"start":"2026-05-18T00:00:00Z","end":"2026-05-18T01:00:00Z"}}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics/query", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		var resp model.MetricQueryResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		return &resp
	}

	a := mk()
	b := mk()
	require.Len(t, a.Result, 1)
	require.Len(t, b.Result, 1)
	require.Equal(t, a.Result[0].Values, b.Result[0].Values,
		"deterministic RNG must yield identical series across calls")
}

// ---- 3) Error cases --------------------------------------------------------

func TestQueryMetric_UnknownTemplate_Returns400(t *testing.T) {
	h := newMetricsTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/metrics/query?templateId=not_a_real_template", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())

	var errResp model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, CodeBadRequest, errResp.Code)
	assert.Equal(t, "not_a_real_template", errResp.Details["templateId"])
}

func TestQueryMetric_MissingRequiredVariable_Returns400(t *testing.T) {
	h := newMetricsTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	// node_cpu_util requires `node`. Calling without it must 400.
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/metrics/query?templateId=node_cpu_util", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())

	var errResp model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, CodeBadRequest, errResp.Code)
	assert.Equal(t, "node_cpu_util", errResp.Details["templateId"])
	assert.Contains(t, errResp.Message, "node",
		"error message should name the missing variable")
}

func TestQueryMetric_UnknownSliceID_Returns400(t *testing.T) {
	h := newMetricsTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/metrics/query?templateId=npu_aicore_util&var-slice=nope-does-not-exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())

	var errResp model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, CodeBadRequest, errResp.Code)
	assert.Equal(t, "nope-does-not-exist", errResp.Details["slice"])
}

// Missing templateId on the query string is its own 400 (templateId is the
// minimum required field per the OpenAPI contract).
func TestQueryMetric_MissingTemplateID_Returns400(t *testing.T) {
	h := newMetricsTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/query", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())
	var errResp model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, CodeBadRequest, errResp.Code)
}

// ---- 4) Workload happy path — non-slice template still returns a series ----

func TestQueryMetric_WorkloadTemplate_HappyPath(t *testing.T) {
	h := newMetricsTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	body := `{"templateId":"workload_throughput","variables":{"workload":"ai-inference/qwen-8b"},"range":{"start":"2026-05-18T00:00:00Z","end":"2026-05-18T00:30:00Z"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics/query", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())

	var resp model.MetricQueryResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Result, 1)
	assert.Equal(t, "workload_throughput", resp.Result[0].Metric["__name__"])
	assert.Equal(t, "ai-inference/qwen-8b", resp.Result[0].Metric["workload"])
	assert.Len(t, resp.Result[0].Values, 200)
}
