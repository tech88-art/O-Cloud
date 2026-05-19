package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/registry"
)

func TestServer_HealthzReturnsOK(t *testing.T) {
	reg := registry.New("1.0", "abc", "go1.24")
	ts := httptest.NewServer(Handler(reg))
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/healthz")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ok\n", string(body))
}

func TestServer_MetricsExposesBuildInfo(t *testing.T) {
	reg := registry.New("1.0", "abc", "go1.24")
	ts := httptest.NewServer(Handler(reg))
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	bs := string(body)

	// Prometheus text exposition orders labels alphabetically by name.
	expected := `exporter_build_info{commit="abc",go_version="go1.24",version="1.0"} 1`
	assert.Contains(t, bs, expected,
		"expected /metrics body to contain build_info sample with alphabetic labels")
}

func TestServer_MetricsReturnsTextFormat(t *testing.T) {
	reg := registry.New("1.0", "abc", "go1.24")
	ts := httptest.NewServer(Handler(reg))
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	require.NotEmpty(t, ct, "Content-Type must be set by promhttp")
	ok := strings.Contains(ct, "text/plain") || strings.Contains(ct, "application/openmetrics-text")
	assert.True(t, ok, "Content-Type %q must contain text/plain or application/openmetrics-text", ct)
}
