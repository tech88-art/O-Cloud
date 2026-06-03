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
//   - Authentication beyond a bearer token (BasicAuth, mTLS all stay
//     PHASE-2+ — see backend/CLAUDE.md §9 security).
//
// Auth token (P13-T-201 / ADR-0025 §2 Decision A): production reads an
// in-cluster ServiceAccount token from the projected-volume mount
// (/var/run/secrets/kubernetes.io/serviceaccount/token). The kubelet
// auto-rotates this file (bound tokens expire ~1h), so the token is read
// from disk per-request (with a short TTL cache to avoid a syscall on every
// query) rather than cached once at startup — a startup-cached token would
// go stale on rotation and start producing 401s. A static BearerToken is
// retained for unsecured dev Prometheus / local-dev.
package prometheus

import (
	"context"
	"errors"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// defaultSATokenPath is the projected ServiceAccount token mount path used
// by in-cluster pods. The kubelet refreshes the file before the bound token
// expires (TokenRequest projected volume · auto-rotate).
const defaultSATokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

// tokenSource yields the current bearer token to send upstream. Returning
// ("", nil) means "send no Authorization header" (unsecured dev Prometheus).
type tokenSource interface {
	Token() (string, error)
}

// staticTokenSource returns a fixed token (dev / local mode).
type staticTokenSource struct{ token string }

func (s staticTokenSource) Token() (string, error) { return s.token, nil }

// fileTokenSource reads the bearer token from a file (the projected SA
// token mount) on demand. It caches the value for ttl to avoid a disk read
// on every request while still picking up the kubelet's rotation within ttl.
// Goroutine-safe.
type fileTokenSource struct {
	path string
	ttl  time.Duration
	now  func() time.Time // injectable clock for tests

	mu       sync.Mutex
	cached   string
	cachedAt time.Time
	loaded   bool
}

func (f *fileTokenSource) Token() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.now()
	if f.loaded && now.Sub(f.cachedAt) < f.ttl {
		return f.cached, nil
	}
	b, err := os.ReadFile(f.path)
	if err != nil {
		// On a refresh failure keep serving the last good token (the file
		// may be mid-rotation); only hard-fail if we never loaded one.
		if f.loaded {
			return f.cached, nil
		}
		return "", err
	}
	f.cached = trimToken(string(b))
	f.cachedAt = now
	f.loaded = true
	return f.cached, nil
}

// Options configures NewSource. URL is the only required field;
// everything else has sensible defaults for the dev compose stack.
type Options struct {
	// URL of the Prometheus HTTP API root (e.g. http://prometheus:9090).
	// No trailing slash required; the QueryMetric path is appended.
	URL string

	// BearerToken, when non-empty, is sent as
	// `Authorization: Bearer <token>` on every request. Intended for
	// unsecured/dev Prometheus or local-dev with a pre-shared token.
	// Mutually exclusive with the SA-token path: a non-empty BearerToken
	// wins (explicit override).
	BearerToken string

	// ServiceAccountTokenPath, when non-empty (and BearerToken empty),
	// makes the Source read the bearer token from this file on each
	// request (with a short TTL cache). Tests point this at a temp file to
	// exercise rotation without a real SA mount. When empty, the Source
	// auto-detects the in-cluster projected mount (see NewSource).
	ServiceAccountTokenPath string

	// DisableInClusterTokenAutodetect, when true, suppresses the automatic
	// fallback to the in-cluster SA token mount. Set by callers that want
	// a strictly unsecured Source even inside a cluster (rare). Default
	// false → in-cluster pods authenticate with their SA token transparently.
	DisableInClusterTokenAutodetect bool

	// TokenRefreshTTL bounds how long a file-sourced token is cached
	// before the next read. Zero → 1 minute (well under the ~1h bound
	// token lifetime). Ignored for static BearerToken.
	TokenRefreshTTL time.Duration

	// HTTPClient overrides the default *http.Client (used by tests
	// against httptest.NewServer). Nil → 30s-timeout default.
	HTTPClient *http.Client

	// nowFunc overrides the clock used by the file token source's TTL
	// (test seam). Nil → time.Now.
	nowFunc func() time.Time
}

// Source talks to a real Prometheus HTTP API. Construct via
// NewSource(Options); never share between processes (the HTTP
// client is per-Source). Goroutine-safe — QueryMetric can be called
// from concurrent handler invocations.
type Source struct {
	url    string
	tokens tokenSource
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
		tokens: buildTokenSource(opts),
		client: client,
	}, nil
}

// buildTokenSource resolves the auth strategy from Options (precedence:
// explicit static BearerToken → explicit file path → auto-detected
// in-cluster SA mount → none). The auto-detect keeps the production path
// working without extra config (in-cluster pods authenticate with their
// projected SA token) while dev compose — where the mount is absent —
// degrades to no Authorization header.
func buildTokenSource(opts Options) tokenSource {
	if opts.BearerToken != "" {
		return staticTokenSource{token: opts.BearerToken}
	}
	path := opts.ServiceAccountTokenPath
	if path == "" && !opts.DisableInClusterTokenAutodetect {
		// Only adopt the in-cluster mount when it actually exists, so the
		// dev compose stack (no SA volume) stays unsecured instead of
		// 500-ing on a missing file.
		if _, err := os.Stat(defaultSATokenPath); err == nil {
			path = defaultSATokenPath
		}
	}
	if path == "" {
		return staticTokenSource{token: ""} // unsecured dev: no header
	}
	ttl := opts.TokenRefreshTTL
	if ttl <= 0 {
		ttl = time.Minute
	}
	now := opts.nowFunc
	if now == nil {
		now = time.Now
	}
	return &fileTokenSource{path: path, ttl: ttl, now: now}
}

// trimToken strips trailing whitespace/newlines that a token file may carry.
func trimToken(s string) string {
	// Tokens are opaque base64url with no internal whitespace; trim only
	// surrounding whitespace (the file typically has no trailing newline,
	// but be defensive).
	end := len(s)
	for end > 0 {
		c := s[end-1]
		if c == '\n' || c == '\r' || c == ' ' || c == '\t' {
			end--
			continue
		}
		break
	}
	start := 0
	for start < end {
		c := s[start]
		if c == '\n' || c == '\r' || c == ' ' || c == '\t' {
			start++
			continue
		}
		break
	}
	return s[start:end]
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
