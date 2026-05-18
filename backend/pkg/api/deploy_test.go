// Package api — Deploy handler tests (P1-T-202).
//
// Coverage map (AC §5+ httptest cases):
//   - Deploy_Auto_HappyPath_Returns201           — auto mode picks first
//     available slice → 201 + deployId + status:accepted
//   - Deploy_Manual_HappyPath_Returns201         — manual mode hits an
//     available slice
//   - Deploy_Manual_SliceAlreadyAllocated_409    — manual targeting an
//     already-allocated slice
//   - Deploy_UnknownPresetID_Returns404          — body refers to a preset
//     that does not exist
//   - Deploy_InvalidBody_Returns400              — missing required fields
//   - Delete_HappyPath_Returns204                — round-trip: deploy then
//     delete + slice freed
//   - Delete_UnknownDeployID_Returns404          — bare DELETE
//   - Deploy_SourceErrors_Returns500             — synthetic source failure
//     surfaces as InternalError
//
// We build a real mock.Source against a tmp dir holding presets.json +
// npus.json + slices.json. workloads.json is INTENTIONALLY omitted so the
// loadWorkloads path exercises its "no fixture → nil cache + lazy append"
// branch, which is what Deploy relies on. Mirrors workload_test.go /
// preset_test.go scaffolding.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	mocksrc "github.com/example/ocloud-edge/backend/pkg/datasource/mock"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// deployPresetsFixture — minimal presets.json with one single-NPU inference
// preset. Requirements.npuCount=1 means each replica needs 1 slice; keeps
// arithmetic simple in tests below.
const deployPresetsFixture = `{
  "presets": [
    {
      "id": "pi-3b",
      "name": "Pi 3B Inference",
      "kind": "inference",
      "modelSize": "3B",
      "runtime": "mindie",
      "requirements": {
        "npuCount": 1,
        "npuModel": "Ascend910B",
        "vramMiBPerNPU": 8192
      },
      "description": "single-card inference"
    }
  ]
}`

// deployNPUsFixture — two NPUs on the same node, each "healthy". Slices live
// in slices.json; the loader joins them onto NPU.Slices.
const deployNPUsFixture = `{
  "npus": [
    {"id": "worker-a-npu-0", "nodeName": "worker-a", "model": "Ascend910B", "status": "healthy", "sliceMode": "fixed-template"},
    {"id": "worker-a-npu-1", "nodeName": "worker-a", "model": "Ascend910B", "status": "healthy", "sliceMode": "fixed-template"}
  ]
}`

// deploySlicesFixture — four slices total. slice-A0 available, slice-A1
// allocated (used by the 409 test), slice-B0/B1 available (for the auto-mode
// happy path).
const deploySlicesFixture = `{
  "slices": [
    {"id": "slice-A0", "parentNPU": "worker-a-npu-0", "template": "vir01", "aiCore": 4, "vramMiB": 8192, "status": "available"},
    {"id": "slice-A1", "parentNPU": "worker-a-npu-0", "template": "vir01", "aiCore": 4, "vramMiB": 8192, "status": "allocated",
     "allocatedTo": {"namespace": "ai-inference", "podName": "existing-pod", "containerName": "main"}},
    {"id": "slice-B0", "parentNPU": "worker-a-npu-1", "template": "vir01", "aiCore": 4, "vramMiB": 8192, "status": "available"},
    {"id": "slice-B1", "parentNPU": "worker-a-npu-1", "template": "vir01", "aiCore": 4, "vramMiB": 8192, "status": "available"}
  ]
}`

// deployWorkloadsFixture — empty workloads list. loadWorkloads requires the
// file to exist when fixturesPath is non-empty (see workload.go), so we ship
// a no-op fixture rather than an empty dir.
const deployWorkloadsFixture = `{"workloads": []}`

// newDeployTestHandler stamps the four fixture files into a tmp dir + returns
// a Handler wired to a mock source mapped under "deploy". workloads.json is
// shipped empty so Deploy's append path starts from zero.
func newDeployTestHandler(t *testing.T) (*Handler, *mocksrc.Source) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "presets.json"), []byte(deployPresetsFixture), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "npus.json"), []byte(deployNPUsFixture), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "slices.json"), []byte(deploySlicesFixture), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "workloads.json"), []byte(deployWorkloadsFixture), 0o600))

	src := mocksrc.NewSource(dir)
	reg := &datasource.Registry{
		Sources: map[string]datasource.Source{"mock": src},
		Mapping: map[string]string{"deploy": "mock", "workloads": "mock"},
	}
	return NewHandler(reg, nil), src
}

func postDeploy(t *testing.T, router *gin.Engine, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	buf, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/deploy", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestDeploy_Auto_HappyPath_Returns201(t *testing.T) {
	h, _ := newDeployTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	body := model.DeployRequest{
		PresetID:  "pi-3b",
		Namespace: "ai-inference",
		Replicas:  1,
		Scheduling: &model.DeployScheduling{
			Mode: "auto",
		},
	}
	rec := postDeploy(t, router, body)

	require.Equal(t, http.StatusCreated, rec.Code, "body=%s", rec.Body.String())

	var got model.DeployResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.NotEmpty(t, got.DeployID, "deployId must be generated")
	assert.True(t, strings.HasPrefix(got.DeployID, "d-"), "deployId convention: d-<n>")
	assert.Equal(t, "accepted", got.Status)
	assert.Equal(t, "ai-inference", got.Namespace)
	require.Len(t, got.ScheduledNodes, 1, "single replica should land on one node")
	assert.Equal(t, "worker-a", got.ScheduledNodes[0])
}

func TestDeploy_Manual_HappyPath_Returns201(t *testing.T) {
	h, _ := newDeployTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	body := model.DeployRequest{
		PresetID:  "pi-3b",
		Namespace: "ai-inference",
		Replicas:  1,
		Scheduling: &model.DeployScheduling{
			Mode: "manual",
			ManualPlacement: []model.ManualPlacement{
				{NodeName: "worker-a", NPUSliceIDs: []string{"slice-B0"}},
			},
		},
	}
	rec := postDeploy(t, router, body)

	require.Equal(t, http.StatusCreated, rec.Code, "body=%s", rec.Body.String())
	var got model.DeployResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.NotEmpty(t, got.DeployID)
	assert.Equal(t, "accepted", got.Status)
}

func TestDeploy_Manual_SliceAlreadyAllocated_Returns409(t *testing.T) {
	h, _ := newDeployTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	body := model.DeployRequest{
		PresetID:  "pi-3b",
		Namespace: "ai-inference",
		Replicas:  1,
		Scheduling: &model.DeployScheduling{
			Mode: "manual",
			ManualPlacement: []model.ManualPlacement{
				// slice-A1 is allocated in fixture → must yield 409.
				{NodeName: "worker-a", NPUSliceIDs: []string{"slice-A1"}},
			},
		},
	}
	rec := postDeploy(t, router, body)

	require.Equal(t, http.StatusConflict, rec.Code, "body=%s", rec.Body.String())

	var errResp model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, CodeConflict, errResp.Code)
	assert.Equal(t, "slice", errResp.Details["resource"])
	assert.Equal(t, "slice-allocated", errResp.Details["reason"])
	assert.Contains(t, errResp.Message, "slice-A1", "message should name the offending slice")
}

func TestDeploy_UnknownPresetID_Returns404(t *testing.T) {
	h, _ := newDeployTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	body := model.DeployRequest{
		PresetID:  "does-not-exist",
		Namespace: "ai-inference",
		Replicas:  1,
	}
	rec := postDeploy(t, router, body)

	require.Equal(t, http.StatusNotFound, rec.Code, "body=%s", rec.Body.String())

	var errResp model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, CodeNotFound, errResp.Code)
	assert.Equal(t, "preset", errResp.Details["resource"])
	assert.Equal(t, "does-not-exist", errResp.Details["id"])
}

func TestDeploy_InvalidBody_MissingPresetID_Returns400(t *testing.T) {
	h, _ := newDeployTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	// Missing presetId — Source layer surfaces ErrInvalidDeployRequest →
	// handler maps to 400.
	body := model.DeployRequest{
		Namespace: "ai-inference",
		Replicas:  1,
	}
	rec := postDeploy(t, router, body)

	require.Equal(t, http.StatusBadRequest, rec.Code, "body=%s", rec.Body.String())
	var errResp model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, CodeBadRequest, errResp.Code)
}

func TestDeploy_InvalidBody_NotJSON_Returns400(t *testing.T) {
	h, _ := newDeployTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/deploy", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var errResp model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, CodeBadRequest, errResp.Code)
}

func TestDeleteDeploy_HappyPath_Returns204(t *testing.T) {
	h, _ := newDeployTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	// Deploy first so the index has an entry.
	body := model.DeployRequest{
		PresetID:  "pi-3b",
		Namespace: "ai-inference",
		Replicas:  1,
		Scheduling: &model.DeployScheduling{
			Mode: "auto",
		},
	}
	rec := postDeploy(t, router, body)
	require.Equal(t, http.StatusCreated, rec.Code)
	var dr model.DeployResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &dr))
	require.NotEmpty(t, dr.DeployID)

	// Then DELETE.
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/deploy/"+dr.DeployID, nil)
	delRec := httptest.NewRecorder()
	router.ServeHTTP(delRec, delReq)

	assert.Equal(t, http.StatusNoContent, delRec.Code)
	// 204 contract: no body.
	assert.Empty(t, delRec.Body.String())
}

func TestDeleteDeploy_UnknownID_Returns404(t *testing.T) {
	h, _ := newDeployTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/deploy/d-999", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	var errResp model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, CodeNotFound, errResp.Code)
	assert.Equal(t, "deploy", errResp.Details["resource"])
	assert.Equal(t, "d-999", errResp.Details["id"])
}

func TestDeploy_NoSourceConfigured_Returns500(t *testing.T) {
	// Registry with no sources at all → handler must surface InternalError,
	// not panic. Mirrors TestListWorkloads_NoSourceConfigured_Returns500.
	h := NewHandler(&datasource.Registry{
		Sources: map[string]datasource.Source{},
		Mapping: map[string]string{},
	}, nil)
	router := NewRouter(h, RouterOptions{})

	body := model.DeployRequest{PresetID: "pi-3b", Namespace: "ai-inference"}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/deploy", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	var errResp model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, CodeInternalError, errResp.Code)
}

// ---- 500 path on source failure ----------------------------------------
//
// erroringDeploySource surfaces a sentinel error from Deploy / DeleteDeploy.
// Embeds the mock source so the rest of the interface is satisfied by the
// stubs (no test in this file exercises the other methods).

type erroringDeploySource struct {
	*mocksrc.Source
}

var errDeployBoom = errors.New("synthetic deploy source failure")

func (e *erroringDeploySource) Deploy(_ context.Context, _ *model.DeployRequest) (*model.DeployResponse, error) {
	return nil, errDeployBoom
}

func (e *erroringDeploySource) DeleteDeploy(_ context.Context, _ string) error {
	return errDeployBoom
}

func newErroringDeployRouter(t *testing.T) *gin.Engine {
	t.Helper()
	src := &erroringDeploySource{Source: mocksrc.NewSource("")}
	reg := &datasource.Registry{
		Sources: map[string]datasource.Source{"mock": src},
		Mapping: map[string]string{"deploy": "mock"},
	}
	h := NewHandler(reg, nil)
	return NewRouter(h, RouterOptions{})
}

func TestDeploy_SourceErrors_Returns500(t *testing.T) {
	router := newErroringDeployRouter(t)

	body := model.DeployRequest{
		PresetID:  "pi-3b",
		Namespace: "ai-inference",
		Replicas:  1,
	}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/deploy", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	var errResp model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, CodeInternalError, errResp.Code)
}

func TestDeleteDeploy_SourceErrors_Returns500(t *testing.T) {
	router := newErroringDeployRouter(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/deploy/d-123", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	var errResp model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, CodeInternalError, errResp.Code)
}
