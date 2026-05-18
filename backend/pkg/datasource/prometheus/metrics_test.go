package prometheus

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// fakePromServer spins an httptest.Server that returns the supplied
// status code + JSON body. Captures the request URL.Query so callers
// can assert the expanded PromQL hit the wire.
type fakePromServer struct {
	*httptest.Server
	lastQuery url.Values
}

func newFakePromServer(status int, body string) *fakePromServer {
	fs := &fakePromServer{}
	fs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.lastQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	return fs
}

const happyResponse = `{
  "status": "success",
  "data": {
    "resultType": "matrix",
    "result": [
      {
        "metric": {"__name__": "node_cpu_seconds_total", "instance": "worker-1"},
        "values": [[1716000000, "0.42"], [1716000030, "0.51"]]
      }
    ]
  }
}`

func TestQueryMetric_HappyPath_ExpandedPromQLOnTheWire(t *testing.T) {
	srv := newFakePromServer(http.StatusOK, happyResponse)
	defer srv.Close()

	src, err := NewSource(Options{URL: srv.URL})
	require.NoError(t, err)

	resp, err := src.QueryMetric(context.Background(),
		"node_cpu_util",
		map[string]string{"node": "worker-1"},
		model.TimeRange{Start: "1716000000", End: "1716000030", Step: "30s"})
	require.NoError(t, err)
	require.Len(t, resp.Result, 1)
	assert.Equal(t, "worker-1", resp.Result[0].Metric["instance"])
	require.Len(t, resp.Result[0].Values, 2)

	// Variables substituted into the PromQL.
	got := srv.lastQuery.Get("query")
	assert.Contains(t, got, `instance="worker-1"`)
	assert.NotContains(t, got, "$node", "$node placeholder should be gone")

	// Time range threaded through.
	assert.Equal(t, "1716000000", srv.lastQuery.Get("start"))
	assert.Equal(t, "1716000030", srv.lastQuery.Get("end"))
	assert.Equal(t, "30s", srv.lastQuery.Get("step"))
}

func TestQueryMetric_MissingRequiredVar(t *testing.T) {
	src, err := NewSource(Options{URL: "http://prom"})
	require.NoError(t, err)

	_, err = src.QueryMetric(context.Background(),
		"workload_throughput",
		nil, // workload var absent
		model.TimeRange{})
	assert.ErrorIs(t, err, ErrMissingVariable)
}

func TestQueryMetric_UnknownTemplate(t *testing.T) {
	src, err := NewSource(Options{URL: "http://prom"})
	require.NoError(t, err)

	_, err = src.QueryMetric(context.Background(),
		"no_such_template", nil, model.TimeRange{})
	assert.ErrorIs(t, err, ErrUnknownTemplate)
}

func TestQueryMetric_OptionalVarBecomesDotStar(t *testing.T) {
	// npu_aicore_util has 3 optional vars (node / npu / slice); the
	// expanded PromQL should substitute `.*` for the missing ones.
	srv := newFakePromServer(http.StatusOK, happyResponse)
	defer srv.Close()
	src, err := NewSource(Options{URL: srv.URL})
	require.NoError(t, err)

	_, err = src.QueryMetric(context.Background(),
		"npu_aicore_util",
		map[string]string{"npu": "worker-1-npu-0"},
		model.TimeRange{Start: "1716000000", End: "1716000030"})
	require.NoError(t, err)

	got := srv.lastQuery.Get("query")
	assert.Contains(t, got, `npu_id="worker-1-npu-0"`)
	assert.Contains(t, got, `instance=".*"`, "missing node → .*")
	assert.Contains(t, got, `slice_id=".*"`, "missing slice → .*")
}

func TestQueryMetric_UpstreamNon200_ReturnsErrUpstream(t *testing.T) {
	srv := newFakePromServer(http.StatusInternalServerError, `{"status":"error","errorType":"internal","error":"boom"}`)
	defer srv.Close()
	src, err := NewSource(Options{URL: srv.URL})
	require.NoError(t, err)

	_, err = src.QueryMetric(context.Background(),
		"node_cpu_util",
		map[string]string{"node": "x"},
		model.TimeRange{Start: "1", End: "2"})
	assert.ErrorIs(t, err, ErrUpstream)
}

func TestQueryMetric_PrometheusStatusErrorBody(t *testing.T) {
	// 200 OK but JSON body says status="error" — also wrap as ErrUpstream.
	body := `{"status":"error","errorType":"bad_data","error":"unparseable query"}`
	srv := newFakePromServer(http.StatusOK, body)
	defer srv.Close()
	src, err := NewSource(Options{URL: srv.URL})
	require.NoError(t, err)

	_, err = src.QueryMetric(context.Background(),
		"node_cpu_util", map[string]string{"node": "x"}, model.TimeRange{})
	assert.ErrorIs(t, err, ErrUpstream)
}

func TestQueryMetric_BearerTokenHeader(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))
	defer srv.Close()

	src, err := NewSource(Options{URL: srv.URL, BearerToken: "secret-token"})
	require.NoError(t, err)
	_, err = src.QueryMetric(context.Background(),
		"node_cpu_util", map[string]string{"node": "x"}, model.TimeRange{})
	require.NoError(t, err)
	assert.Equal(t, "Bearer secret-token", seen)
}

func TestNewSource_RequiresURL(t *testing.T) {
	_, err := NewSource(Options{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "URL is required")
}

func TestCapabilities_OnlyMetrics(t *testing.T) {
	src, err := NewSource(Options{URL: "http://prom"})
	require.NoError(t, err)
	caps := src.Capabilities()
	assert.True(t, caps.Metrics)
	assert.False(t, caps.Clusters)
	assert.False(t, caps.Nodes)
	assert.False(t, caps.Workloads)
}

func TestSource_StubsReturnErrCapabilityUnavailable(t *testing.T) {
	src, err := NewSource(Options{URL: "http://prom"})
	require.NoError(t, err)
	ctx := context.Background()

	_, e := src.ListClusters(ctx)
	assert.ErrorIs(t, e, datasource.ErrCapabilityUnavailable)
	_, e = src.ListNodes(ctx, model.NodeFilter{})
	assert.ErrorIs(t, e, datasource.ErrCapabilityUnavailable)
	_, e = src.ListNPUs(ctx, "x")
	assert.ErrorIs(t, e, datasource.ErrCapabilityUnavailable)
	_, e = src.ListWorkloads(ctx, model.WorkloadFilter{})
	assert.ErrorIs(t, e, datasource.ErrCapabilityUnavailable)
	_, e = src.StreamEvents(ctx, model.StreamEventsOptions{})
	assert.ErrorIs(t, e, datasource.ErrCapabilityUnavailable)
}

func TestExpandPromQL_SubstitutesAllOccurrences(t *testing.T) {
	tpl := `metric{a="$x",b="$x",c="$y"}`
	got := expandPromQL(tpl, map[string]string{"x": "AAA", "y": "BBB"})
	assert.Equal(t, `metric{a="AAA",b="AAA",c="BBB"}`, got)
}

func TestExpandPromQL_PreservesNonVarDollarless(t *testing.T) {
	tpl := `histogram_quantile(0.95, sum by (le) (rate(foo[1m])))`
	assert.Equal(t, tpl, expandPromQL(tpl, nil))
}

func TestResolveTimeRange_DefaultsApplied(t *testing.T) {
	start, end, step := resolveTimeRange(model.TimeRange{})
	assert.NotEmpty(t, start)
	assert.NotEmpty(t, end)
	assert.Equal(t, "30s", step)
}

func TestParsePromResponse_DecodeError(t *testing.T) {
	_, err := parsePromResponse(strings.NewReader(`{not json`))
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrUpstream), "decode errors are not ErrUpstream")
}

func TestParsePromResponse_EmptyResult(t *testing.T) {
	body := `{"status":"success","data":{"resultType":"matrix","result":[]}}`
	out, err := parsePromResponse(strings.NewReader(body))
	require.NoError(t, err)
	assert.NotNil(t, out)
	assert.Empty(t, out.Result)
}

func TestParsePromResponse_StatusError(t *testing.T) {
	body := `{"status":"error","errorType":"bad_data","error":"parse error"}`
	_, err := parsePromResponse(strings.NewReader(body))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUpstream)
}

// Sanity: ensure encoding/json is wired (parse path exercised).
func TestParsePromResponse_FullMatrix(t *testing.T) {
	// Decode the canonical happyResponse — ensure values are not
	// destructured (the contract is `[][]any`).
	out, err := parsePromResponse(strings.NewReader(happyResponse))
	require.NoError(t, err)
	require.Len(t, out.Result, 1)
	require.Len(t, out.Result[0].Values, 2)
	// Each entry is [timestamp, value] where timestamp is a JSON
	// number (decoded as float64) and value is a string.
	first := out.Result[0].Values[0]
	require.Len(t, first, 2)
	assert.IsType(t, float64(0), first[0])
	assert.IsType(t, "", first[1])
}

func TestStrconvI64(t *testing.T) {
	assert.Equal(t, "0", strconvI64(0))
	assert.Equal(t, "1", strconvI64(1))
	assert.Equal(t, "1716000000", strconvI64(1716000000))
}

// Sanity smoke against the testify json import to avoid dropping it.
func TestJSONImportSanity(t *testing.T) {
	var x map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"a":1}`), &x))
	assert.EqualValues(t, 1, x["a"])
}
