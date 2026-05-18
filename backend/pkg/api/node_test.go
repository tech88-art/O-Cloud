// Package api — node handler tests (P1-T-103).
//
// We construct a real mock.Source pointing at a tmp dir holding a minimal
// nodes.json. This exercises the same load path the production code uses,
// keeping the test honest about JSON shape drift (any rename in
// model.Node{,Detail} or contract would surface here).
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	mocksrc "github.com/example/ocloud-edge/backend/pkg/datasource/mock"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// nodeFixtureJSON is a minimal but realistic three-node fixture: two workers
// + one control-plane. Mirrors the shape of
// configs/mock-data/set-a-small/nodes.json (top-level object with "nodes" key).
const nodeFixtureJSON = `{
  "nodes": [
    {
      "name": "worker-site-a-01",
      "clusterId": "cluster-prod-a-01",
      "role": ["worker"],
      "status": "Ready",
      "cpu": {"raw": "96"},
      "memory": {"raw": "768Gi"},
      "arch": "amd64",
      "kernelVersion": "5.15.0-105-generic",
      "os": "Ubuntu 22.04.4 LTS",
      "kubeletVersion": "v1.31.0",
      "npuCount": 8,
      "numa": [
        {"id": 0, "cpus": [0,1,2,3], "memory": {"raw":"384Gi"}, "npus": ["worker-site-a-01-npu-0"]}
      ],
      "networkInterfaces": [
        {"name": "eth0", "mac": "aa:bb:cc:dd:ee:00", "ips": ["10.0.10.11"], "speed": "25Gbps"}
      ],
      "storage": [{"device": "/dev/nvme0n1", "size": {"raw": "2Ti"}, "type": "nvme"}],
      "labels": {"topology.kubernetes.io/zone": "zone-1"}
    },
    {
      "name": "worker-site-a-02",
      "clusterId": "cluster-prod-a-01",
      "role": ["worker"],
      "status": "Ready",
      "cpu": {"raw": "96"},
      "memory": {"raw": "768Gi"},
      "arch": "amd64",
      "npuCount": 8
    },
    {
      "name": "control-site-a-01",
      "clusterId": "cluster-prod-a-01",
      "role": ["control-plane"],
      "status": "Ready",
      "cpu": {"raw": "32"},
      "memory": {"raw": "128Gi"},
      "arch": "amd64",
      "npuCount": 0
    }
  ]
}`

// newNodesTestHandler writes nodeFixtureJSON to a tmp dir, builds a Source
// pointing at that dir, and returns a Handler whose registry serves it.
func newNodesTestHandler(t *testing.T) *Handler {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "nodes.json"), []byte(nodeFixtureJSON), 0o600))

	src := mocksrc.NewSource(dir)
	reg := &datasource.Registry{
		Sources: map[string]datasource.Source{"mock": src},
		Mapping: map[string]string{"nodes": "mock"},
	}
	return NewHandler(reg, nil)
}

func TestListNodes_HappyPath_ReturnsAllNodes(t *testing.T) {
	h := newNodesTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var got []model.Node
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got, 3, "fixture has 3 nodes")
	// Order is fixture-order; assert the first to catch field drift.
	assert.Equal(t, "worker-site-a-01", got[0].Name)
	assert.Equal(t, "cluster-prod-a-01", got[0].ClusterID)
	assert.Equal(t, []string{"worker"}, got[0].Role)
	assert.Equal(t, "Ready", got[0].Status)
	require.NotNil(t, got[0].CPU)
	assert.Equal(t, "96", got[0].CPU.Raw)
	assert.Equal(t, 8, got[0].NPUCount)
}

func TestListNodes_FilterByRoleWorker_ReturnsWorkersOnly(t *testing.T) {
	h := newNodesTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?role=worker", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var got []model.Node
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got, 2, "fixture has 2 worker nodes")
	for _, n := range got {
		assert.Contains(t, n.Role, "worker")
		assert.NotContains(t, n.Role, "control-plane")
	}
}

func TestListNodes_FilterByRoleControlPlane_ReturnsOnlyControl(t *testing.T) {
	h := newNodesTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?role=control-plane", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var got []model.Node
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got, 1)
	assert.Equal(t, "control-site-a-01", got[0].Name)
}

func TestListNodes_FilterByClusterIdMiss_ReturnsEmptyArray(t *testing.T) {
	h := newNodesTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes?clusterId=cluster-does-not-exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	// Must be an empty JSON array, not null. Frontend renders directly.
	assert.JSONEq(t, `[]`, rec.Body.String())
}

func TestGetNodeDetail_HappyPath_ReturnsFullDetail(t *testing.T) {
	h := newNodesTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/worker-site-a-01", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var got model.NodeDetail
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "worker-site-a-01", got.Name)
	require.Len(t, got.NUMA, 1)
	assert.Equal(t, 0, got.NUMA[0].ID)
	require.Len(t, got.NetworkInterfaces, 1)
	assert.Equal(t, "eth0", got.NetworkInterfaces[0].Name)
	require.Len(t, got.Storage, 1)
	assert.Equal(t, "nvme", got.Storage[0].Type)
}

func TestGetNodeDetail_UnknownName_Returns404(t *testing.T) {
	h := newNodesTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/unknown", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)

	var errResp model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, CodeNotFound, errResp.Code)
	assert.Equal(t, "node", errResp.Details["resource"])
	assert.Equal(t, "unknown", errResp.Details["name"])
}

func TestListNodes_NoSourceConfigured_Returns500(t *testing.T) {
	// Registry with no sources at all → handler must surface InternalError, not panic.
	h := NewHandler(&datasource.Registry{
		Sources: map[string]datasource.Source{},
		Mapping: map[string]string{},
	}, nil)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	var errResp model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, CodeInternalError, errResp.Code)
}
