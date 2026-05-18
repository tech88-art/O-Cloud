package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	mocksrc "github.com/example/ocloud-edge/backend/pkg/datasource/mock"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// fixturePayload is the minimal composite JSON shape the mock loader expects
// from configs/mock-data/set-a-small/clusters.json.
const fixturePayload = `{
  "meta": {"name": "test", "version": "0.1.0"},
  "clusters": [
    {
      "id": "cluster-prod-a-01",
      "name": "cluster-prod-a-01",
      "role": "edge-single",
      "location": "site-a-shanghai",
      "status": "healthy",
      "kubernetesVersion": "v1.31.0",
      "nodeCount": 3,
      "npuCount": 24,
      "labels": {"environment": "prod"}
    },
    {
      "id": "cluster-prod-b-02",
      "name": "cluster-prod-b-02",
      "role": "small-cluster",
      "status": "degraded",
      "nodeCount": 5
    }
  ]
}`

// writeFixture drops a clusters.json into a fresh temp dir and returns the
// dir path. t.TempDir() cleans up automatically.
func writeFixture(t *testing.T, payload string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "clusters.json")
	require.NoError(t, os.WriteFile(path, []byte(payload), 0o600))
	return dir
}

// newClusterRouter builds a router whose `clusters` mapping points at a real
// mock.Source loaded from a temp fixture directory. Used by happy + 404 paths.
func newClusterRouter(t *testing.T, fixturePath string) *gin.Engine {
	t.Helper()
	src := mocksrc.NewSource(fixturePath)
	reg := &datasource.Registry{
		Sources: map[string]datasource.Source{"mock": src},
		Mapping: map[string]string{"clusters": "mock"},
	}
	h := NewHandler(reg, nil)
	return NewRouter(h, RouterOptions{})
}

func TestListClusters_HappyPath_ReturnsAll(t *testing.T) {
	router := newClusterRouter(t, writeFixture(t, fixturePayload))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))

	var got []model.Cluster
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got, 2)
	assert.Equal(t, "cluster-prod-a-01", got[0].ID)
	assert.Equal(t, "edge-single", got[0].Role)
	assert.Equal(t, "healthy", got[0].Status)
	assert.Equal(t, 3, got[0].NodeCount)
	assert.Equal(t, 24, got[0].NPUCount)
	assert.Equal(t, "cluster-prod-b-02", got[1].ID)
	assert.Equal(t, "degraded", got[1].Status)
}

func TestListClusters_EmptyFixture_ReturnsEmptyArray(t *testing.T) {
	router := newClusterRouter(t, writeFixture(t, `{"clusters":[]}`))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	// Contract requires `[]`, not `null`.
	assert.Equal(t, "[]", rec.Body.String())
}

func TestGetCluster_HappyPath_ReturnsSingle(t *testing.T) {
	router := newClusterRouter(t, writeFixture(t, fixturePayload))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/cluster-prod-a-01", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var got model.Cluster
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "cluster-prod-a-01", got.ID)
	assert.Equal(t, "site-a-shanghai", got.Location)
	assert.Equal(t, "v1.31.0", got.KubernetesVersion)
	assert.Equal(t, "prod", got.Labels["environment"])
}

func TestGetCluster_UnknownID_Returns404(t *testing.T) {
	router := newClusterRouter(t, writeFixture(t, fixturePayload))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/does-not-exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)

	var got model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, CodeNotFound, got.Code)
	assert.Equal(t, "cluster", got.Details["resource"])
	assert.Equal(t, "does-not-exist", got.Details["id"])
}

// ---- 500 path -----------------------------------------------------------
//
// erroringSource fails every Source method with sentinel errors. We embed
// mock.Source so the file stays a one-liner per method as new resources
// land. Only the two methods T101 exercises override; others inherit the
// stub behavior (ErrNotImplemented) — which is fine because tests don't
// hit them.

type erroringSource struct {
	*mocksrc.Source
}

var errBoom = errors.New("synthetic source failure")

func (e *erroringSource) ListClusters(_ context.Context) ([]*model.Cluster, error) {
	return nil, errBoom
}

func (e *erroringSource) GetCluster(_ context.Context, _ string) (*model.Cluster, error) {
	return nil, errBoom
}

func newErroringRouter(t *testing.T) *gin.Engine {
	t.Helper()
	src := &erroringSource{Source: mocksrc.NewSource("")}
	reg := &datasource.Registry{
		Sources: map[string]datasource.Source{"mock": src},
		Mapping: map[string]string{"clusters": "mock"},
	}
	h := NewHandler(reg, nil)
	return NewRouter(h, RouterOptions{})
}

func TestListClusters_SourceErrors_Returns500(t *testing.T) {
	router := newErroringRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)

	var got model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, CodeInternalError, got.Code)
}

func TestGetCluster_SourceErrors_Returns500(t *testing.T) {
	router := newErroringRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/anything", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)

	var got model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, CodeInternalError, got.Code)
}

func TestGetCluster_NoMapping_Returns500(t *testing.T) {
	// Registry with mock source but no clusters→mock mapping: SourceFor returns nil.
	reg := &datasource.Registry{
		Sources: map[string]datasource.Source{"mock": mocksrc.NewSource("")},
		Mapping: map[string]string{},
	}
	h := NewHandler(reg, nil)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/x", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
}
