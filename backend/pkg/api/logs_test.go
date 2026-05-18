// Package api — workload logs handler tests (P1-T-301).
//
// Reuses the same mock fixture shape as workload_test.go so the 404 path,
// the container filter, and the tail bounds are exercised against realistic
// workload data. The WS handler is exercised end-to-end via httptest.NewServer
// + gorilla/websocket dialer (the topology WS tests use the same pattern in
// ws_test.go).
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	mocksrc "github.com/example/ocloud-edge/backend/pkg/datasource/mock"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// logsFixtureJSON is a minimal workloads.json with one running + one failed
// workload. The "qwen-8b-pd" pod has two containers (prefill / decode) so
// the container filter test has something to filter against.
const logsFixtureJSON = `{
  "workloads": [
    {
      "name": "qwen-8b-pd",
      "namespace": "ai-inference",
      "type": "inference",
      "kind": "InferenceService",
      "status": "running",
      "pods": [
        {
          "name": "qwen-8b-pd-prefill-0",
          "namespace": "ai-inference",
          "containers": [
            {"name": "prefill", "image": "mindie/vllm-ascend:0.11.0"},
            {"name": "decode", "image": "mindie/vllm-ascend:0.11.0"}
          ]
        }
      ]
    },
    {
      "name": "oom-test-model",
      "namespace": "training",
      "type": "training",
      "kind": "Deployment",
      "status": "failed",
      "pods": [
        {
          "name": "oom-test-model-0",
          "namespace": "training",
          "containers": [{"name": "main", "image": "registry.local/oom-test:latest"}]
        }
      ]
    }
  ]
}`

func newLogsTestHandler(t *testing.T) *Handler {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "workloads.json"), []byte(logsFixtureJSON), 0o600))

	src := mocksrc.NewSource(dir)
	reg := &datasource.Registry{
		Sources: map[string]datasource.Source{"mock": src},
		Mapping: map[string]string{"workloads": "mock", "logs": "mock"},
	}
	return NewHandler(reg, nil)
}

func TestGetWorkloadLogs_HappyPath_ReturnsDefaultTail(t *testing.T) {
	h := newLogsTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/workloads/ai-inference/qwen-8b-pd/logs", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var got model.LogPage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	// Default tail is 200 per the OpenAPI default + mock's logTailDefault.
	assert.Len(t, got.Lines, 200, "default tail returns 200 lines")
	assert.False(t, got.HasMore, "mock phase 1 always reports HasMore=false")
	assert.Nil(t, got.NextCursor, "mock phase 1 emits no cursor")

	// Container should be one of the workload's pod containers — verify the
	// rotation is using real names, not a synthetic fallback.
	containers := map[string]int{}
	for _, ln := range got.Lines {
		containers[ln.Container]++
		assert.NotEmpty(t, ln.Message, "every line carries a message")
	}
	assert.Contains(t, containers, "prefill")
	assert.Contains(t, containers, "decode")
}

func TestGetWorkloadLogs_TailParam_HonorsExplicitCount(t *testing.T) {
	h := newLogsTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/workloads/ai-inference/qwen-8b-pd/logs?tail=10", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got model.LogPage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Len(t, got.Lines, 10, "tail=10 returns exactly 10 lines")

	// Lines must be chronologically ascending — important for the frontend
	// streaming viewer that appends new lines at the bottom.
	for i := 1; i < len(got.Lines); i++ {
		assert.True(t,
			!got.Lines[i].Timestamp.Before(got.Lines[i-1].Timestamp),
			"line %d timestamp must be >= line %d", i, i-1)
	}
}

func TestGetWorkloadLogs_ContainerFilter_RestrictsToOneContainer(t *testing.T) {
	h := newLogsTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/workloads/ai-inference/qwen-8b-pd/logs?tail=50&container=prefill", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got model.LogPage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Lines, 50)
	for _, ln := range got.Lines {
		assert.Equal(t, "prefill", ln.Container,
			"container filter must drop non-matching containers")
	}
}

func TestGetWorkloadLogs_FailedWorkload_MixesErrorLines(t *testing.T) {
	h := newLogsTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/workloads/training/oom-test-model/logs?tail=300", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got model.LogPage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))

	levels := map[string]int{}
	for _, ln := range got.Lines {
		levels[ln.Level]++
	}
	// Failed workloads mix ~30% ERROR + 20% WARN per the generator; with 300
	// lines we should see *some* of each — the assertion is loose to stay
	// stable across RNG seeds.
	assert.Greater(t, levels["ERROR"], 10, "failed workload should emit ERROR lines")
	assert.Greater(t, levels["WARN"], 5, "failed workload should emit WARN lines")
}

func TestGetWorkloadLogs_NotFound_Returns404(t *testing.T) {
	h := newLogsTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/workloads/ai-inference/no-such-workload/logs", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "workload not found")
}

func TestGetWorkloadLogs_InvalidTail_Returns400(t *testing.T) {
	h := newLogsTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	cases := []struct {
		raw    string
		reason string
	}{
		{"abc", "non-numeric tail"},
		{"-5", "negative tail"},
		{"99999", "tail exceeds max"},
	}
	for _, c := range cases {
		t.Run(c.reason, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet,
				"/api/v1/workloads/ai-inference/qwen-8b-pd/logs?tail="+c.raw, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusBadRequest, rec.Code, c.reason)
		})
	}
}

// --- WS tests ---

func TestWSLogs_HappyPath_StreamsLogMessages(t *testing.T) {
	h := newLogsTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	srv := httptest.NewServer(router)
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	u.Scheme = "ws"
	u.Path = "/ws/logs/ai-inference/qwen-8b-pd"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, u.String(), nil)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	// Read at least one message (cadence is 1s; allow up to 3s for the first
	// tick to land inside CI variance).
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	mt, raw, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, websocket.TextMessage, mt)

	var env model.WSMessage
	require.NoError(t, json.Unmarshal(raw, &env))
	assert.Equal(t, "log.line", env.Type, "log stream envelope must use the canonical WSMessage type")

	var line model.LogLine
	require.NoError(t, json.Unmarshal(env.Payload, &line))
	assert.NotEmpty(t, line.Message)
	assert.NotEmpty(t, line.Container)
	assert.False(t, line.Timestamp.IsZero())
}

func TestWSLogs_NotFound_Returns404BeforeUpgrade(t *testing.T) {
	h := newLogsTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	srv := httptest.NewServer(router)
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	u.Scheme = "ws"
	u.Path = "/ws/logs/ai-inference/no-such-workload"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, resp, err := websocket.DefaultDialer.DialContext(ctx, u.String(), nil)
	require.Error(t, err, "dial must fail when the server refuses upgrade")
	require.NotNil(t, resp, "server must respond with an HTTP status, not a TCP close")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"non-existent workload should 404 before WS upgrade")
}

func TestWSLogs_MissingNamespace_RoutesAround(t *testing.T) {
	// Sanity: the route /ws/logs/:ns/:name requires both path params; a URL
	// without them must not reach the handler (Gin returns its own 404).
	h := newLogsTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/ws/logs/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.True(t, rec.Code == http.StatusNotFound || strings.Contains(rec.Body.String(), "404"),
		"empty path must not match the WSLogs handler")
}
