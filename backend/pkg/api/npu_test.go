// Package api — NPU handler tests (P1-T-104).
//
// Builds a real mock.Source against a tmp dir holding minimal npus.json +
// slices.json fixtures. Same pattern as cluster_test.go / node_test.go — keeps
// the JSON shape honest (any rename in model.NPU / model.NPUSlice or contract
// drift will surface here).
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

// npuFixtureJSON is a 2-node × 3-NPU subset of set-a-small npus.json shape.
// Mixes sliceMode whole/fixed-template/dynamic so we can assert the joined
// slices land on the right parent.
const npuFixtureJSON = `{
  "npus": [
    {
      "id": "worker-site-a-01-npu-0",
      "nodeName": "worker-site-a-01",
      "model": "Ascend910B",
      "index": 0,
      "vramMiB": 65536,
      "aiCoreTotal": 32,
      "numaNode": 0,
      "hccsGroup": "worker-site-a-01-hccs-0",
      "status": "healthy",
      "sliceMode": "whole",
      "usage": {"aiCoreUtilization": 5, "vramUtilization": 4, "vramUsedMiB": 2621}
    },
    {
      "id": "worker-site-a-01-npu-2",
      "nodeName": "worker-site-a-01",
      "model": "Ascend910B",
      "index": 2,
      "vramMiB": 65536,
      "aiCoreTotal": 32,
      "numaNode": 0,
      "hccsGroup": "worker-site-a-01-hccs-0",
      "status": "healthy",
      "sliceMode": "fixed-template",
      "usage": {"aiCoreUtilization": 31, "vramUtilization": 28, "vramUsedMiB": 18350}
    },
    {
      "id": "worker-site-a-02-npu-0",
      "nodeName": "worker-site-a-02",
      "model": "Ascend910B",
      "index": 0,
      "vramMiB": 65536,
      "aiCoreTotal": 32,
      "numaNode": 0,
      "hccsGroup": "worker-site-a-02-hccs-0",
      "status": "degraded",
      "sliceMode": "whole",
      "usage": {"aiCoreUtilization": 0, "vramUtilization": 0, "vramUsedMiB": 0}
    }
  ]
}`

// sliceFixtureJSON wires two slices onto worker-site-a-01-npu-2 (fixed-template
// mode). NPU-0 on both nodes has no slices → loader leaves them as empty []
// (still serialized; AC requires "slices" key visible).
const sliceFixtureJSON = `{
  "slices": [
    {
      "id": "worker-site-a-01-npu-2-slice-0",
      "parentNPU": "worker-site-a-01-npu-2",
      "template": "vir02",
      "aiCore": 8,
      "vramMiB": 16384,
      "status": "allocated",
      "allocatedTo": {"namespace": "ai-inference", "podName": "pod-vir02-0", "containerName": "main"},
      "usage": {"aiCoreUtilization": 29.6, "vramUtilization": 55.9, "vramUsedMiB": 9158}
    },
    {
      "id": "worker-site-a-01-npu-2-slice-1",
      "parentNPU": "worker-site-a-01-npu-2",
      "template": "vir02",
      "aiCore": 8,
      "vramMiB": 16384,
      "status": "available"
    }
  ]
}`

// newNPUTestHandler writes both fixtures to a tmp dir and returns a Handler.
func newNPUTestHandler(t *testing.T) *Handler {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "npus.json"), []byte(npuFixtureJSON), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "slices.json"), []byte(sliceFixtureJSON), 0o600))

	src := mocksrc.NewSource(dir)
	reg := &datasource.Registry{
		Sources: map[string]datasource.Source{"mock": src},
		Mapping: map[string]string{"npus": "mock"},
	}
	return NewHandler(reg, nil)
}

func TestListNPUs_HappyPath_ReturnsNodesNPUs(t *testing.T) {
	h := newNPUTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/worker-site-a-01/npus", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var got []model.NPU
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got, 2, "fixture has 2 NPUs on worker-site-a-01")
	for _, npu := range got {
		assert.Equal(t, "worker-site-a-01", npu.NodeName)
		assert.Equal(t, "Ascend910B", npu.Model)
	}
}

func TestListNPUs_SliceStatusVisible(t *testing.T) {
	h := newNPUTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/worker-site-a-01/npus", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	// Slice mode is always serialized; slices array shows up when non-empty
	// (model.NPU.Slices uses omitempty per the OpenAPI optional schema).
	body := rec.Body.String()
	assert.Contains(t, body, `"sliceMode":"whole"`)
	assert.Contains(t, body, `"sliceMode":"fixed-template"`)
	assert.Contains(t, body, `"slices":[`, "fixed-template NPU must serialize its slices")

	// Cross-check the joined slices land on the right parent.
	var got []model.NPU
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	var npu2 *model.NPU
	for i := range got {
		if got[i].ID == "worker-site-a-01-npu-2" {
			npu2 = &got[i]
		}
	}
	require.NotNil(t, npu2, "fixture must yield npu-2")
	require.Len(t, npu2.Slices, 2, "npu-2 has 2 slices in slices.json")
	assert.Equal(t, "worker-site-a-01-npu-2-slice-0", npu2.Slices[0].ID)
	assert.Equal(t, "allocated", npu2.Slices[0].Status)
	require.NotNil(t, npu2.Slices[0].AllocatedTo)
	assert.Equal(t, "pod-vir02-0", npu2.Slices[0].AllocatedTo.PodName)
	assert.Equal(t, "available", npu2.Slices[1].Status)
}

func TestListNPUs_UnknownNode_Returns404(t *testing.T) {
	h := newNPUTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/does-not-exist/npus", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)

	var errResp model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, CodeNotFound, errResp.Code)
	assert.Equal(t, "node", errResp.Details["resource"])
	assert.Equal(t, "does-not-exist", errResp.Details["name"])
}

// ---- 500 path -----------------------------------------------------------
//
// erroringNPUSource fails ListNPUs with a sentinel error. We embed mock.Source
// so other Source methods inherit ErrNotImplemented stubs (handler tests for
// /npus don't hit them, but the type still has to satisfy the interface).

type erroringNPUSource struct {
	*mocksrc.Source
}

var errNPUBoom = errors.New("synthetic NPU source failure")

func (e *erroringNPUSource) ListNPUs(_ context.Context, _ string) ([]*model.NPU, error) {
	return nil, errNPUBoom
}

func newErroringNPURouter(t *testing.T) *gin.Engine {
	t.Helper()
	src := &erroringNPUSource{Source: mocksrc.NewSource("")}
	reg := &datasource.Registry{
		Sources: map[string]datasource.Source{"mock": src},
		Mapping: map[string]string{"npus": "mock"},
	}
	h := NewHandler(reg, nil)
	return NewRouter(h, RouterOptions{})
}

func TestListNPUs_SourceErrors_Returns500(t *testing.T) {
	router := newErroringNPURouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/worker-site-a-01/npus", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)

	var errResp model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, CodeInternalError, errResp.Code)
}
