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

// ---- Topology (P1-T-102) -------------------------------------------------
//
// The topology endpoint joins clusters + nodes + npus + slices. Tests write
// minimal but realistic fixtures for all four into a tmp dir, then assert
// the response shape end-to-end. We reuse the cluster fixture payload from
// `fixturePayload` for clusters.json and supply node/npu/slice fixtures
// inline below.

const topologyNodeFixture = `{
  "nodes": [
    {
      "name": "worker-site-a-01",
      "clusterId": "cluster-prod-a-01",
      "role": ["worker"],
      "status": "Ready",
      "arch": "amd64",
      "os": "Ubuntu 22.04",
      "npuCount": 2
    },
    {
      "name": "worker-site-a-02",
      "clusterId": "cluster-prod-a-01",
      "role": ["worker"],
      "status": "Ready",
      "arch": "amd64",
      "os": "Ubuntu 22.04",
      "npuCount": 1
    }
  ]
}`

const topologyNPUFixture = `{
  "npus": [
    {"id": "worker-site-a-01-npu-0", "nodeName": "worker-site-a-01", "model": "Ascend910B", "index": 0, "vramMiB": 65536, "aiCoreTotal": 32, "hccsGroup": "hccs-0", "status": "healthy", "sliceMode": "fixed-template"},
    {"id": "worker-site-a-01-npu-1", "nodeName": "worker-site-a-01", "model": "Ascend910B", "index": 1, "vramMiB": 65536, "aiCoreTotal": 32, "hccsGroup": "hccs-0", "status": "healthy", "sliceMode": "whole"},
    {"id": "worker-site-a-02-npu-0", "nodeName": "worker-site-a-02", "model": "Ascend910B", "index": 0, "vramMiB": 65536, "aiCoreTotal": 32, "hccsGroup": "hccs-0", "status": "degraded", "sliceMode": "whole"}
  ]
}`

const topologySliceFixture = `{
  "slices": [
    {"id": "worker-site-a-01-npu-0-slice-0", "parentNPU": "worker-site-a-01-npu-0", "template": "vir02", "aiCore": 8, "vramMiB": 16384, "status": "allocated", "allocatedTo": {"namespace": "ai-inference", "podName": "pod-0", "containerName": "main"}},
    {"id": "worker-site-a-01-npu-0-slice-1", "parentNPU": "worker-site-a-01-npu-0", "template": "vir02", "aiCore": 8, "vramMiB": 16384, "status": "available"}
  ]
}`

// writeTopologyFixtures drops all four files (clusters, nodes, npus, slices)
// into a fresh tmp dir and returns the dir path. The fixture is the standard
// 1-cluster / 2-node / 3-npu / 2-slice tree used by every topology test.
func writeTopologyFixtures(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, payload := range map[string]string{
		"clusters.json": fixturePayload,
		"nodes.json":    topologyNodeFixture,
		"npus.json":     topologyNPUFixture,
		"slices.json":   topologySliceFixture,
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(payload), 0o600))
	}
	return dir
}

// newTopologyRouter builds a router whose registered "mock" source loads from
// the supplied fixture dir, with both "clusters" and "topology" mapped to it.
func newTopologyRouter(t *testing.T, fixturePath string) *gin.Engine {
	t.Helper()
	src := mocksrc.NewSource(fixturePath)
	reg := &datasource.Registry{
		Sources: map[string]datasource.Source{"mock": src},
		Mapping: map[string]string{"clusters": "mock", "topology": "mock"},
	}
	h := NewHandler(reg, nil)
	return NewRouter(h, RouterOptions{})
}

func TestGetClusterTopology_DepthSlice_FullGraph(t *testing.T) {
	router := newTopologyRouter(t, writeTopologyFixtures(t))

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/clusters/cluster-prod-a-01/topology?depth=slice", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var topo model.Topology
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &topo))

	// 1 cluster + 2 nodes + 3 NPUs + 2 slices = 8.
	assert.Len(t, topo.Nodes, 8)
	// 2 cluster→node + 3 node→npu + 2 npu→slice = 7.
	assert.Len(t, topo.Edges, 7)

	// Spot-check a slice node carries its allocatedTo payload.
	var foundAllocated bool
	for _, n := range topo.Nodes {
		if n.ID == "worker-site-a-01-npu-0-slice-0" {
			assert.Equal(t, "slice", n.Type)
			assert.Equal(t, "allocated", n.Status)
			if at, ok := n.Attributes["allocatedTo"].(map[string]interface{}); ok {
				assert.Equal(t, "ai-inference", at["namespace"])
				foundAllocated = true
			}
			break
		}
	}
	assert.True(t, foundAllocated, "expected allocated slice node in graph")

	require.NotNil(t, topo.Meta)
	assert.Equal(t, "cluster-prod-a-01", topo.Meta.ClusterID)
}

func TestGetClusterTopology_DepthNode_OnlyClusterAndNodes(t *testing.T) {
	router := newTopologyRouter(t, writeTopologyFixtures(t))

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/clusters/cluster-prod-a-01/topology?depth=node", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var topo model.Topology
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &topo))

	// 1 cluster + 2 nodes only.
	assert.Len(t, topo.Nodes, 3)
	assert.Len(t, topo.Edges, 2)
	for _, n := range topo.Nodes {
		assert.NotContains(t, []string{"npu", "slice"}, n.Type,
			"depth=node should not contain %q nodes", n.Type)
	}
}

func TestGetClusterTopology_DefaultDepth_Slice(t *testing.T) {
	// No ?depth= → handler defaults to slice.
	router := newTopologyRouter(t, writeTopologyFixtures(t))

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/clusters/cluster-prod-a-01/topology", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var topo model.Topology
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &topo))
	assert.Len(t, topo.Nodes, 8)
	assert.Len(t, topo.Edges, 7)
}

func TestGetClusterTopology_UnknownCluster_Returns404(t *testing.T) {
	router := newTopologyRouter(t, writeTopologyFixtures(t))

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/clusters/nonexistent/topology", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)

	var got model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, CodeNotFound, got.Code)
	assert.Equal(t, "cluster", got.Details["resource"])
	assert.Equal(t, "nonexistent", got.Details["id"])
}

func TestGetClusterTopology_SourceError_Returns500(t *testing.T) {
	// Reuse erroringSource and override GetTopology to fail.
	src := &erroringTopologySource{Source: mocksrc.NewSource("")}
	reg := &datasource.Registry{
		Sources: map[string]datasource.Source{"mock": src},
		Mapping: map[string]string{"topology": "mock"},
	}
	h := NewHandler(reg, nil)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/clusters/anything/topology", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	var got model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, CodeInternalError, got.Code)
}

// erroringTopologySource makes only GetTopology fail; other methods inherit
// the stub Source behavior. Keeps tests honest about which call path
// surfaces the 500.
type erroringTopologySource struct {
	*mocksrc.Source
}

func (e *erroringTopologySource) GetTopology(_ context.Context, _ string, _ string) (*model.Topology, error) {
	return nil, errBoom
}

// ---- Topology fabric (P1-T-211 / ADR-0004) ------------------------------
//
// Fabric tests reuse writeTopologyFixtures + the canonical 1-cluster /
// 2-node / 3-npu / 2-slice tree, layering in networkSwitches.json +
// networkLinks.json that the new loader picks up.

const topologyNetworkSwitchesFixture = `{
  "networkSwitches": [
    {
      "id": "switch-tor-a-01",
      "name": "ToR-Site-A-01",
      "type": "tor",
      "location": "site-a",
      "portsTotal": 48,
      "portsUsed": 6,
      "bandwidthGbps": 100,
      "vlans": ["vlan-100-mgmt", "vlan-200-fabric"],
      "status": "up"
    }
  ]
}`

// topologyNetworkLinksFixture covers 3 ToR↔worker links — matches the
// canonical set-a-small shape the AC count (3 fabric-link edges) refers to.
// Note "worker-site-a-03" appears here but NOT in topologyNodeFixture; the
// orphan-endpoint guard SHOULD drop it. To exercise the "1 cluster + 3
// nodes + 1 switch + 3 fabric-link" AC count we add a 3rd worker via
// topologyNodeFixture3Workers used by the fabric tests.
const topologyNetworkLinksFixture = `{
  "networkLinks": [
    {"id": "link-tor01-worker01", "from": "switch-tor-a-01", "to": "worker-site-a-01", "bandwidthGbps": 100, "medium": "fiber", "utilization": 18.5, "rttUs": 5.4},
    {"id": "link-tor01-worker02", "from": "switch-tor-a-01", "to": "worker-site-a-02", "bandwidthGbps": 100, "medium": "fiber", "utilization": 32.1, "rttUs": 6.2},
    {"id": "link-tor01-worker03", "from": "switch-tor-a-01", "to": "worker-site-a-03", "bandwidthGbps": 100, "medium": "fiber", "utilization": 12.7, "rttUs": 5.1}
  ]
}`

// topologyNodeFixture3Workers is the 3-worker variant mirroring the
// canonical set-a-small/nodes.json shape (the original
// topologyNodeFixture only has 2 workers so the T102 zero-regression test
// stays a tight 2-worker case). Used by the fabric AC test that asserts
// the "1 cluster + 3 nodes + 1 switch + 3 fabric-link" count.
const topologyNodeFixture3Workers = `{
  "nodes": [
    {"name": "worker-site-a-01", "clusterId": "cluster-prod-a-01", "role": ["worker"], "status": "Ready", "arch": "amd64", "os": "Ubuntu 22.04", "npuCount": 2},
    {"name": "worker-site-a-02", "clusterId": "cluster-prod-a-01", "role": ["worker"], "status": "Ready", "arch": "amd64", "os": "Ubuntu 22.04", "npuCount": 1},
    {"name": "worker-site-a-03", "clusterId": "cluster-prod-a-01", "role": ["worker"], "status": "Ready", "arch": "amd64", "os": "Ubuntu 22.04", "npuCount": 0}
  ]
}`

// writeTopologyFixturesWithFabric layers fabric fixtures on top of the T102
// tree (2 workers). Used by zero-regression / bad-value tests where the
// orphan-endpoint guard correctly drops the 3rd link.
func writeTopologyFixturesWithFabric(t *testing.T) string {
	t.Helper()
	dir := writeTopologyFixtures(t)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "networkSwitches.json"),
		[]byte(topologyNetworkSwitchesFixture), 0o600))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "networkLinks.json"),
		[]byte(topologyNetworkLinksFixture), 0o600))
	return dir
}

// writeTopologyFixturesWithFabric3Workers swaps in the 3-worker nodes
// fixture so all three links resolve. Used by the AC-count test asserting
// "1 cluster + 3 nodes + 1 switch + 3 fabric-link".
func writeTopologyFixturesWithFabric3Workers(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, payload := range map[string]string{
		"clusters.json":        fixturePayload,
		"nodes.json":           topologyNodeFixture3Workers,
		"npus.json":            topologyNPUFixture,
		"slices.json":          topologySliceFixture,
		"networkSwitches.json": topologyNetworkSwitchesFixture,
		"networkLinks.json":    topologyNetworkLinksFixture,
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(payload), 0o600))
	}
	return dir
}

func TestGetClusterTopology_IncludeFabricTrue_DepthNode_AddsSwitchAndLinks(t *testing.T) {
	// AC verbatim: "?depth=node&includeFabric=true → 4 nodes (1 cluster +
	// 3 nodes) + 1 switch + 3 fabric-link edges". The 3-worker fixture
	// matches the canonical set-a-small shape so all 3 ToR links resolve.
	router := newTopologyRouter(t, writeTopologyFixturesWithFabric3Workers(t))

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/clusters/cluster-prod-a-01/topology?depth=node&includeFabric=true", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var topo model.Topology
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &topo))

	// 1 cluster + 3 nodes + 1 switch = 5 nodes total.
	// 3 cluster→node "contains" + 3 fabric-link = 6 edges total.
	assert.Len(t, topo.Nodes, 5)
	assert.Len(t, topo.Edges, 6)

	// Count fabric-link edges specifically — the AC's "3 fabric-link edges".
	var fabricEdges int
	for _, e := range topo.Edges {
		if e.Type == "fabric-link" {
			fabricEdges++
		}
	}
	assert.Equal(t, 3, fabricEdges, "expected 3 fabric-link edges")

	// Count switch nodes specifically — the AC's "1 switch".
	var switches int
	var theSwitch model.TopologyNode
	for _, n := range topo.Nodes {
		if n.Type == "switch" {
			switches++
			theSwitch = n
		}
	}
	assert.Equal(t, 1, switches, "expected exactly 1 switch node")
	assert.Equal(t, "switch-tor-a-01", theSwitch.ID)
	assert.Equal(t, "ToR-Site-A-01", theSwitch.Label)
	assert.Equal(t, "up", theSwitch.Status)
	assert.Equal(t, "tor", theSwitch.Attributes["switchType"])
}

func TestGetClusterTopology_IncludeFabricFalse_ZeroRegression(t *testing.T) {
	// AC: default (?includeFabric=false) is byte-equivalent to T102 even
	// when networkSwitches.json / networkLinks.json exist on disk.
	router := newTopologyRouter(t, writeTopologyFixturesWithFabric(t))

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/clusters/cluster-prod-a-01/topology?depth=slice", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var topo model.Topology
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &topo))

	// Same counts as TestGetClusterTopology_DepthSlice_FullGraph.
	assert.Len(t, topo.Nodes, 8)
	assert.Len(t, topo.Edges, 7)

	for _, n := range topo.Nodes {
		assert.NotEqual(t, "switch", n.Type,
			"switch node leaked into includeFabric=false response: %v", n.ID)
	}
	for _, e := range topo.Edges {
		assert.NotEqual(t, "fabric-link", e.Type,
			"fabric-link edge leaked into includeFabric=false response")
	}
}

func TestGetClusterTopology_IncludeFabric_BadValueTreatedAsFalse(t *testing.T) {
	// AC quality-of-life: a frontend typo like ?includeFabric=yes shouldn't
	// 400; it should fall through to false (zero-regression).
	router := newTopologyRouter(t, writeTopologyFixturesWithFabric(t))

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/clusters/cluster-prod-a-01/topology?depth=slice&includeFabric=yes", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var topo model.Topology
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &topo))
	assert.Len(t, topo.Nodes, 8, "bad includeFabric should not add fabric")
	assert.Len(t, topo.Edges, 7)
}
