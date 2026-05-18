// Package prometheus implements a Phase 2 prometheus.Source that
// fronts a real Prometheus HTTP API. Mirrors mock.Source's
// templateDef contract so the swap is yaml-only (P1-T-303 / ADR-0006).
//
// Scope (P2-T-007):
//   - QueryMetric — POST/GET against /api/v1/query_range with the
//     templated PromQL, parse Prometheus's matrix response.
//   - Capabilities returns only Metrics=true. Every other Source
//     method returns datasource.ErrCapabilityUnavailable; sibling
//     Phase 2 sources (k8s / crd / configmap) cover those.
//
// Out of scope (later P2-T-1xx):
//   - Streaming / WS push (prometheus has /api/v1/series for
//     subscription-style use; not needed for the page contract).
//   - Authentication beyond a static bearer token (BasicAuth, mTLS,
//     OIDC all stay PHASE-2+ — see backend/CLAUDE.md §9 security).
package prometheus

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// Options configures NewSource. URL is the only required field;
// everything else has sensible defaults for the dev compose stack.
type Options struct {
	// URL of the Prometheus HTTP API root (e.g. http://prometheus:9090).
	// No trailing slash required; the QueryMetric path is appended.
	URL string

	// BearerToken, when non-empty, is sent as
	// `Authorization: Bearer <token>` on every request. For unsecured
	// dev Prometheus leave it blank.
	BearerToken string

	// HTTPClient overrides the default *http.Client (used by tests
	// against httptest.NewServer). Nil → 30s-timeout default.
	HTTPClient *http.Client
}

// Source talks to a real Prometheus HTTP API. Construct via
// NewSource(Options); never share between processes (the HTTP
// client is per-Source). Goroutine-safe — QueryMetric can be called
// from concurrent handler invocations.
type Source struct {
	url    string
	bearer string
	client *http.Client
}

// NewSource validates the options and returns a ready Source.
// Returns an error when URL is blank — callers that pass an empty
// URL almost certainly have a config bug; failing fast is friendlier
// than producing 500s on every query.
func NewSource(opts Options) (*Source, error) {
	if opts.URL == "" {
		return nil, errors.New("prometheus: Options.URL is required")
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Source{
		url:    trimTrailingSlash(opts.URL),
		bearer: opts.BearerToken,
		client: client,
	}, nil
}

// Compile-time check: *Source must satisfy datasource.Source.
var _ datasource.Source = (*Source)(nil)

func (s *Source) Name() string { return "prometheus" }

func (s *Source) Capabilities() datasource.Capabilities {
	return datasource.Capabilities{
		Metrics: true,
	}
}

// ---- Stubs returning ErrCapabilityUnavailable ----
//
// Every Source method that isn't Metrics is fielded by the k8s.Source
// or mock.Source instead. The mapping table in main.go's factory.Build
// dispatches per-resource; a request to /clusters won't ever reach this
// Source, but we still need to satisfy the interface.

func (s *Source) ListClusters(_ context.Context) ([]*model.Cluster, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) GetCluster(_ context.Context, _ string) (*model.Cluster, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) GetTopology(_ context.Context, _, _ string) (*model.Topology, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) GetTopologyWithFabric(_ context.Context, _, _ string, _ datasource.TopologyOptions) (*model.Topology, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) ListNodes(_ context.Context, _ model.NodeFilter) ([]*model.Node, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) GetNodeDetail(_ context.Context, _ string) (*model.NodeDetail, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) ListNPUs(_ context.Context, _ string) ([]*model.NPU, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) ListNPUSlicePools(_ context.Context) ([]*model.NPUSlicePool, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) ListWorkloads(_ context.Context, _ model.WorkloadFilter) ([]*model.Workload, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) GetWorkloadDetail(_ context.Context, _, _ string) (*model.WorkloadDetail, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) GetWorkloadLogs(_ context.Context, _, _ string, _ model.LogOptions) (*model.LogPage, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) StreamWorkloadLogs(_ context.Context, _, _ string, _ model.LogStreamOptions) (<-chan *model.LogLine, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) ListPresets(_ context.Context) ([]*model.Preset, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) GetPreset(_ context.Context, _ string) (*model.PresetDetail, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) Deploy(_ context.Context, _ *model.DeployRequest) (*model.DeployResponse, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) DeleteDeploy(_ context.Context, _ string) error {
	return datasource.ErrCapabilityUnavailable
}
func (s *Source) StreamEvents(_ context.Context, _ model.StreamEventsOptions) (<-chan *model.WSMessage, error) {
	return nil, datasource.ErrCapabilityUnavailable
}

// ---- Errors ----

// ErrUnknownTemplate is returned by QueryMetric when the templateId
// isn't in the white-list. Handler maps to 400 BadRequest.
var ErrUnknownTemplate = errors.New("prometheus: unknown template id")

// ErrMissingVariable is returned when a template's required variable
// is absent from the vars map. Handler maps to 400 BadRequest.
var ErrMissingVariable = errors.New("prometheus: missing required variable")

// ErrUpstream wraps any non-2xx Prometheus response. Handler maps to
// 502 Bad Gateway when seen.
var ErrUpstream = errors.New("prometheus: upstream error")

// ---- helpers ----

// trimTrailingSlash drops one trailing slash if present so the
// QueryMetric path concatenation doesn't produce `//api/v1/...`.
func trimTrailingSlash(u string) string {
	if u == "" {
		return u
	}
	if u[len(u)-1] == '/' {
		return u[:len(u)-1]
	}
	return u
}
