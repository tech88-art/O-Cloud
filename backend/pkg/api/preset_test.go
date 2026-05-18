// Package api — Preset handler tests (P1-T-203).
//
// Builds a real mock.Source against a tmp dir holding a minimal presets.json
// fixture. Same pattern as cluster_test.go / npu_test.go — keeps the JSON
// shape honest (any rename in model.Preset / model.PresetDetail or contract
// drift surfaces here).
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

// presetsFixtureJSON mirrors the set-a-small layout: composite document with a
// top-level `presets` array. Two entries cover the slim Preset fields and the
// PresetDetail superset (manifest); kind values exercise both `inference` and
// `inference-pd` enum members.
const presetsFixtureJSON = `{
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
        "vramMiBPerNPU": 8192,
        "cpu": {"raw": "4"},
        "memory": {"raw": "16Gi"}
      },
      "description": "Lightweight 3B inference.",
      "tags": ["inference", "small"],
      "manifest": "placeholder://helm/pi-3b-values.yaml"
    },
    {
      "id": "qwen-8b-pd",
      "name": "Qwen 8B PD Disaggregated",
      "kind": "inference-pd",
      "modelSize": "8B",
      "runtime": "vllm",
      "requirements": {
        "npuCount": 2,
        "npuModel": "Ascend910B",
        "vramMiBPerNPU": 65536
      },
      "description": "Qwen 8B with prefill / decode disaggregation.",
      "tags": ["inference", "pd"],
      "manifest": "placeholder://helm/qwen-8b-pd-values.yaml"
    }
  ]
}`

// newPresetTestHandler writes presets.json to a tmp dir and returns a Handler
// wired to a mock source mapped under "presets".
func newPresetTestHandler(t *testing.T) *Handler {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "presets.json"), []byte(presetsFixtureJSON), 0o600))

	src := mocksrc.NewSource(dir)
	reg := &datasource.Registry{
		Sources: map[string]datasource.Source{"mock": src},
		Mapping: map[string]string{"presets": "mock"},
	}
	return NewHandler(reg, nil)
}

func TestListPresets_HappyPath_ReturnsAllPresets(t *testing.T) {
	h := newPresetTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/presets", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var got []model.Preset
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got, 2, "fixture has 2 presets")

	byID := make(map[string]model.Preset, len(got))
	for _, p := range got {
		byID[p.ID] = p
	}
	require.Contains(t, byID, "pi-3b")
	require.Contains(t, byID, "qwen-8b-pd")
	assert.Equal(t, "inference", byID["pi-3b"].Kind)
	assert.Equal(t, "inference-pd", byID["qwen-8b-pd"].Kind)
	assert.Equal(t, "Ascend910B", byID["qwen-8b-pd"].Requirements.NPUModel)

	// Slim Preset view must NOT carry manifest — the field lives only on
	// PresetDetail. The body verifies the encoder dropped it.
	assert.NotContains(t, rec.Body.String(), `"manifest"`,
		"ListPresets must project out manifest; it lives on PresetDetail only")
}

func TestGetPreset_HappyPath_ReturnsDetailWithManifest(t *testing.T) {
	h := newPresetTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/presets/qwen-8b-pd", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var got model.PresetDetail
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "qwen-8b-pd", got.ID)
	assert.Equal(t, "Qwen 8B PD Disaggregated", got.Name)
	assert.Equal(t, "inference-pd", got.Kind)
	assert.Equal(t, "vllm", got.Runtime)
	require.NotNil(t, got.Requirements)
	assert.Equal(t, 2, got.Requirements.NPUCount)
	assert.Equal(t, "placeholder://helm/qwen-8b-pd-values.yaml", got.Manifest)
}

func TestGetPreset_UnknownID_Returns404(t *testing.T) {
	h := newPresetTestHandler(t)
	router := NewRouter(h, RouterOptions{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/presets/does-not-exist", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)

	var errResp model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, CodeNotFound, errResp.Code)
	assert.Equal(t, "preset", errResp.Details["resource"])
	assert.Equal(t, "does-not-exist", errResp.Details["id"])
}

// ---- 500 path -----------------------------------------------------------
//
// erroringPresetSource fails ListPresets / GetPreset with a sentinel error.
// Embeds mock.Source so unrelated interface methods inherit the stubs.

type erroringPresetSource struct {
	*mocksrc.Source
}

var errPresetBoom = errors.New("synthetic preset source failure")

func (e *erroringPresetSource) ListPresets(_ context.Context) ([]*model.Preset, error) {
	return nil, errPresetBoom
}

func (e *erroringPresetSource) GetPreset(_ context.Context, _ string) (*model.PresetDetail, error) {
	return nil, errPresetBoom
}

func newErroringPresetRouter(t *testing.T) *gin.Engine {
	t.Helper()
	src := &erroringPresetSource{Source: mocksrc.NewSource("")}
	reg := &datasource.Registry{
		Sources: map[string]datasource.Source{"mock": src},
		Mapping: map[string]string{"presets": "mock"},
	}
	h := NewHandler(reg, nil)
	return NewRouter(h, RouterOptions{})
}

func TestListPresets_SourceErrors_Returns500(t *testing.T) {
	router := newErroringPresetRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/presets", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)

	var errResp model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, CodeInternalError, errResp.Code)
}

func TestGetPreset_SourceErrors_Returns500(t *testing.T) {
	router := newErroringPresetRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/presets/pi-3b", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)

	var errResp model.Error
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Equal(t, CodeInternalError, errResp.Code)
}
