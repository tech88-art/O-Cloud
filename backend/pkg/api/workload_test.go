// Package api — workload handler tests (P1-T-201).
//
// We build a real mock.Source against a tmp dir holding a minimal
// workloads.json. Mirrors the pattern used by cluster_test.go / node_test.go
// / npu_test.go so any field rename in model.{Workload,WorkloadDetail,Pod}
// or contract drift surfaces here.
//
// Fixture covers the key permutations:
//   - 1 PD-pair inference workload (ai-inference / qwen-8b-pd, status=running)
//     with pods + relations[pd-pair]
//   - 1 plain inference workload (ai-inference / pi-3b, status=running)
//   - 1 benchmark Job (benchmark / vllm-bench, status=running)
//   - 1 pending training workload (training / llama2-7b-finetune, status=pending)
//
// Total: 4 entries spanning 3 namespaces, 3 types, 2 statuses — enough to
// exercise every filter dimension in the AC without lugging the full
// set-a-small fixture into the test.
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

// workloadFixtureJSON mirrors the shape of
// configs/mock-data/set-a-small/workloads.json (top-level object with a
// "workloads" array). The qwen-8b-pd entry models a real PD-disaggregated
// inference deployment with pods + relations[pd-pair] so the detail handler
// is exercised on the AC's headline case.
const workloadFixtureJSON = `{
  "workloads": [
    {
      "name": "qwen-8b-pd",
      "namespace": "ai-inference",
      "type": "inference",
      "kind": "InferenceService",
      "status": "running",
      "replicas": {"desired": 2, "ready": 2},
      "nodeNames": ["worker-site-a-01", "worker-site-a-02"],
      "npuUsage": {"allocated": 2, "slices": ["worker-site-a-01-npu-1", "worker-site-a-02-npu-1"]},
      "pods": [
        {
          "name": "qwen-8b-pd-prefill-0",
          "namespace": "ai-inference",
          "nodeName": "worker-site-a-01",
          "status": "Running",
          "containers": [
            {
              "name": "prefill",
              "image": "mindie/vllm-ascend:0.11.0",
              "resources": {"cpu": "16", "memory": "64Gi", "npuSlices": ["worker-site-a-01-npu-1"]}
            }
          ]
        },
        {
          "name": "qwen-8b-pd-decode-0",
          "namespace": "ai-inference",
          "nodeName": "worker-site-a-02",
          "status": "Running",
          "containers": [
            {
              "name": "decode",
              "image": "mindie/vllm-ascend:0.11.0",
              "resources": {"cpu": "16", "memory": "64Gi", "npuSlices": ["worker-site-a-02-npu-1"]}
            }
          ]
        }
      ],
      "relations": [
        {"from": "qwen-8b-pd-prefill-0", "to": "qwen-8b-pd-decode-0", "type": "pd-pair"}
      ],
      "createdAt": "2026-05-11T10:00:00Z",
      "labels": {"app.kubernetes.io/name": "qwen-8b-pd"}
    },
    {
      "name": "pi-3b",
      "namespace": "ai-inference",
      "type": "inference",
      "kind": "Deployment",
      "status": "running",
      "replicas": {"desired": 3, "ready": 3},
      "nodeNames": ["worker-site-a-02"],
      "createdAt": "2026-05-15T09:00:00Z"
    },
    {
      "name": "vllm-bench",
      "namespace": "benchmark",
      "type": "benchmark",
      "kind": "Job",
      "status": "running",
      "replicas": {"desired": 1, "ready": 1},
      "nodeNames": ["worker-site-a-01"],
      "createdAt": "2026-05-17T08:00:00Z"
    },
    {
      "name": "llama2-7b-finetune",
      "namespace": "training",
      "type": "training",
      "kind": "Job",
      "status": "pending",
      "replicas": {"desired": 1, "ready": 0},
      "createdAt": "2026-05-17T11:00:00Z"
    }
  ]
}`

// newWorkloadsTestHandler writes workloadFixtureJSON to a tmp dir and returns
// a Handler whose registry serves it. Pattern matches newNodesTestHandler in
// node_test.go.
func newWorkloadsTestHandler(t *testing.T) *Handler {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "workloads.json"), []byte(workloadFixtureJSON), 0o600))

	src := mocksrc.NewSource(dir)
	reg := &datasource.Registry{
		Sources: map[string]datasource.Source{"mock": src},
		Mapping: map[string]string{"workloads": "mock"},
	}
	return NewHandler(reg, nil)
}

func TestListWorkloads_HappyPath_ReturnsAllWorkloads(t *testing.T) {
	h := newWorkloadsTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var got []model.Workload
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got, 4, "fixture has 4 workloads")
	// Order is fixture-order; assert the first record's headline fields.
	assert.Equal(t, "qwen-8b-pd", got[0].Name)
	assert.Equal(t, "ai-inference", got[0].Namespace)
	assert.Equal(t, "inference", got[0].Type)
	assert.Equal(t, "running", got[0].Status)
	require.NotNil(t, got[0].Replicas)
	assert.Equal(t, 2, got[0].Replicas.Desired)
	assert.Equal(t, 2, got[0].Replicas.Ready)
}

func TestListWorkloads_FilterByStatusRunning_ReturnsThreeRunning(t *testing.T) {
	h := newWorkloadsTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads?status=running", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var got []model.Workload
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got, 3, "3 of 4 fixture workloads are status=running")
	for _, w := range got {
		assert.Equal(t, "running", w.Status)
	}
}

func TestListWorkloads_FilterByNamespace_ReturnsTwoInAI(t *testing.T) {
	h := newWorkloadsTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads?namespace=ai-inference", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var got []model.Workload
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got, 2, "fixture has 2 ai-inference workloads")
	for _, w := range got {
		assert.Equal(t, "ai-inference", w.Namespace)
	}
}

func TestListWorkloads_FilterByTypeBenchmark_ReturnsOnlyBench(t *testing.T) {
	h := newWorkloadsTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads?type=benchmark", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var got []model.Workload
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got, 1, "only vllm-bench is type=benchmark")
	assert.Equal(t, "vllm-bench", got[0].Name)
}

func TestListWorkloads_FilterMiss_ReturnsEmptyArray(t *testing.T) {
	h := newWorkloadsTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads?namespace=does-not-exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	// Must be an empty JSON array, not null. Frontend renders directly.
	assert.JSONEq(t, `[]`, rec.Body.String())
}

func TestGetWorkloadDetail_HappyPath_PDPair_PodsAndRelations(t *testing.T) {
	h := newWorkloadsTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads/ai-inference/qwen-8b-pd", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var got model.WorkloadDetail
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "qwen-8b-pd", got.Name)
	assert.Equal(t, "ai-inference", got.Namespace)
	require.Len(t, got.Pods, 2, "PD pair must yield 2 pods")
	assert.Equal(t, "qwen-8b-pd-prefill-0", got.Pods[0].Name)
	assert.Equal(t, "worker-site-a-01", got.Pods[0].NodeName)
	require.Len(t, got.Pods[0].Containers, 1)
	assert.Equal(t, "prefill", got.Pods[0].Containers[0].Name)
	require.NotNil(t, got.Pods[0].Containers[0].Resources)
	assert.Equal(t, []string{"worker-site-a-01-npu-1"}, got.Pods[0].Containers[0].Resources.NPUSlices)

	require.Len(t, got.Relations, 1, "PD pair must yield 1 pd-pair relation")
	assert.Equal(t, "qwen-8b-pd-prefill-0", got.Relations[0].From)
	assert.Equal(t, "qwen-8b-pd-decode-0", got.Relations[0].To)
	assert.Equal(t, "pd-pair", got.Relations[0].Type)
}

func TestGetWorkloadDetail_Unknown_Returns404(t *testing.T) {
	h := newWorkloadsTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads/ai-inference/does-not-exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)

	var errResp model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, CodeNotFound, errResp.Code)
	assert.Equal(t, "workload", errResp.Details["resource"])
	assert.Equal(t, "ai-inference", errResp.Details["namespace"])
	assert.Equal(t, "does-not-exist", errResp.Details["name"])
}

func TestListWorkloads_NoSourceConfigured_Returns500(t *testing.T) {
	// Registry with no sources at all → handler must surface InternalError,
	// not panic. Mirrors TestListNodes_NoSourceConfigured_Returns500.
	h := NewHandler(&datasource.Registry{
		Sources: map[string]datasource.Source{},
		Mapping: map[string]string{},
	}, nil)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	var errResp model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, CodeInternalError, errResp.Code)
}

// ---- 500 path on source failure ----------------------------------------
//
// erroringWorkloadSource fails ListWorkloads / GetWorkloadDetail with a
// sentinel error. We embed mock.Source so other Source methods inherit
// ErrNotImplemented (handler tests for /workloads don't hit them, but the
// type still has to satisfy the interface).

type erroringWorkloadSource struct {
	*mocksrc.Source
}

var errWorkloadBoom = errors.New("synthetic workload source failure")

func (e *erroringWorkloadSource) ListWorkloads(_ context.Context, _ model.WorkloadFilter) ([]*model.Workload, error) {
	return nil, errWorkloadBoom
}

func (e *erroringWorkloadSource) GetWorkloadDetail(_ context.Context, _ string, _ string) (*model.WorkloadDetail, error) {
	return nil, errWorkloadBoom
}

func newErroringWorkloadRouter(t *testing.T) *gin.Engine {
	t.Helper()
	src := &erroringWorkloadSource{Source: mocksrc.NewSource("")}
	reg := &datasource.Registry{
		Sources: map[string]datasource.Source{"mock": src},
		Mapping: map[string]string{"workloads": "mock"},
	}
	h := NewHandler(reg, nil)
	return NewRouter(h, RouterOptions{})
}

func TestListWorkloads_SourceErrors_Returns500(t *testing.T) {
	router := newErroringWorkloadRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)

	var errResp model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, CodeInternalError, errResp.Code)
}

func TestGetWorkloadDetail_SourceErrors_Returns500(t *testing.T) {
	router := newErroringWorkloadRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads/ai-inference/qwen-8b-pd", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)

	var errResp model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, CodeInternalError, errResp.Code)
}

// ---- Real set-a-small fixture sanity check -----------------------------
//
// The P1-T-201 AC explicitly calls out "10 workloads from set-a-small" — wire
// that contract straight into a test so any drift in the canonical fixture
// (configs/mock-data/set-a-small/workloads.json) is caught at PR time, not
// by a frontend reviewer eyeballing the demo. Path is relative to this test
// file (backend/pkg/api/) → repo root.
//
// Skips when the fixture is absent (lets the package still compile + test
// in worktrees that haven't pulled the configs/ submodule, though Phase 1
// always has it on a shared dev branch).

func TestListWorkloads_SetASmall_FixtureCount(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "..", "configs", "mock-data", "set-a-small")
	if _, err := os.Stat(filepath.Join(fixturePath, "workloads.json")); err != nil {
		t.Skipf("set-a-small fixture missing (%v) — skipping integration sanity check", err)
	}

	src := mocksrc.NewSource(fixturePath)
	reg := &datasource.Registry{
		Sources: map[string]datasource.Source{"mock": src},
		Mapping: map[string]string{"workloads": "mock"},
	}
	h := NewHandler(reg, nil)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var got []model.Workload
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Len(t, got, 10, "P1-T-201 AC: set-a-small ships 10 workloads")
}

func TestGetWorkloadDetail_SetASmall_QwenPDPair(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "..", "configs", "mock-data", "set-a-small")
	if _, err := os.Stat(filepath.Join(fixturePath, "workloads.json")); err != nil {
		t.Skipf("set-a-small fixture missing (%v) — skipping integration sanity check", err)
	}

	src := mocksrc.NewSource(fixturePath)
	reg := &datasource.Registry{
		Sources: map[string]datasource.Source{"mock": src},
		Mapping: map[string]string{"workloads": "mock"},
	}
	h := NewHandler(reg, nil)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads/ai-inference/qwen-8b-pd", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var got model.WorkloadDetail
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "qwen-8b-pd", got.Name)
	require.GreaterOrEqual(t, len(got.Pods), 2, "PD pair should yield prefill + decode pods")
	require.GreaterOrEqual(t, len(got.Relations), 1, "PD pair should yield 1 pd-pair relation")
	assert.Equal(t, "pd-pair", got.Relations[0].Type)
}
