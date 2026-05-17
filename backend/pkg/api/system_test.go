package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	mocksrc "github.com/example/ocloud-edge/backend/pkg/datasource/mock"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// newTestRouter wires an engine with a possibly-empty registry so each test
// keeps its own state. Centralized here so all _test.go files use the same
// bootstrap path.
func newTestRouter(t *testing.T, reg *datasource.Registry) *Handler {
	t.Helper()
	h := NewHandler(reg, nil)
	return h
}

func TestHealthz_EmptyRegistry_ReturnsOK(t *testing.T) {
	h := newTestRouter(t, &datasource.Registry{Sources: map[string]datasource.Source{}})
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var got model.Health
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "ok", got.Status)
	assert.Empty(t, got.Datasources, "expected empty datasources map with no sources registered")
}

func TestHealthz_WithRegisteredMockSource_ReportsOK(t *testing.T) {
	reg := &datasource.Registry{
		Sources: map[string]datasource.Source{
			"mock": mocksrc.NewSource(""),
		},
	}
	h := newTestRouter(t, reg)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var got model.Health
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "ok", got.Status)
	assert.Equal(t, "ok", got.Datasources["mock"])
}

func TestVersion_ReturnsHardcodedVersion(t *testing.T) {
	h := newTestRouter(t, nil)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/version", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var got model.Version
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, CurrentVersion, got.Version)
	// Commit / BuildTime are wired via -ldflags; in plain `go test` they
	// stay empty, which is fine for the contract (both are omitempty).
}

func TestVersion_ResponseIsJSON(t *testing.T) {
	h := newTestRouter(t, nil)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/version", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))
}
