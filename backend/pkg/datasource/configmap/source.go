// Package configmap implements a Phase 2 ConfigMap-backed
// datasource.Source. Reads the preset catalog from a single
// ConfigMap (default name `ocloud-presets` in a configurable
// namespace).
//
// Scope (P2-T-103):
//   - ListPresets / GetPreset — JSON-decode each key under
//     ConfigMap.data as a model.PresetDetail.
//   - Capabilities returns only Presets=true; every other Source
//     method returns datasource.ErrCapabilityUnavailable.
//
// Why ConfigMap vs CRD:
//   - Presets are a small static catalog (P1-T-203 shipped 4).
//   - The frontend doesn't need watch semantics for the catalog
//     (presets change at deploy time, not at runtime).
//   - Operators edit `kubectl edit cm ocloud-presets` or apply a
//     YAML — friendlier than a custom CRD for a single key-value
//     map of YAML manifests.
//
// Caching: Phase 2 reads the ConfigMap on every ListPresets call.
// At < 10 presets the apiserver overhead is negligible; Phase 3+
// can layer an informer-backed cache if measurement justifies it.
package configmap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// Default ConfigMap coordinates. Operators that prefer a different
// name/namespace override via Options at construction time.
const (
	defaultConfigMapName      = "ocloud-presets"
	defaultConfigMapNamespace = "ocloud-system"
)

// Options bundles construction-time knobs.
type Options struct {
	KubeconfigPath string

	// Namespace + Name of the preset ConfigMap. Empty values fall
	// back to defaults (`ocloud-system`/`ocloud-presets`).
	Namespace string
	Name      string
}

// Source reads preset catalog ConfigMaps via the typed clientset.
type Source struct {
	client    kubernetes.Interface
	namespace string
	name      string
}

// NewSource builds a production Source against a real apiserver.
// Tests should prefer NewSourceWithClient.
func NewSource(opts Options) (*Source, error) {
	cfg, err := loadRESTConfig(opts.KubeconfigPath)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return NewSourceWithClient(client, opts), nil
}

// NewSourceWithClient lets unit tests pass a fake clientset.
func NewSourceWithClient(client kubernetes.Interface, opts Options) *Source {
	ns := opts.Namespace
	if ns == "" {
		ns = defaultConfigMapNamespace
	}
	name := opts.Name
	if name == "" {
		name = defaultConfigMapName
	}
	return &Source{
		client:    client,
		namespace: ns,
		name:      name,
	}
}

var _ datasource.Source = (*Source)(nil)

func (s *Source) Name() string { return "configmap" }

func (s *Source) Capabilities() datasource.Capabilities {
	return datasource.Capabilities{
		Presets: true,
	}
}

// ---- Errors ----

// ErrPresetNotFound is the 404 sentinel for GetPreset.
var ErrPresetNotFound = errors.New("configmap: preset not found")

// ErrConfigMapNotFound is returned when the underlying ConfigMap
// itself doesn't exist. Handler maps to 500 (mis-configuration)
// rather than 404 — the operator hasn't set up the catalog yet.
var ErrConfigMapNotFound = errors.New("configmap: preset ConfigMap not found")

// ---- ListPresets / GetPreset ----

// ListPresets reads the preset ConfigMap and decodes every data
// key as a model.PresetDetail (then projects to the list-view
// model.Preset). Sort key (id) is lex; ListPresets two calls in
// a row return byte-identical bodies when the ConfigMap hasn't
// changed.
//
// Malformed keys (JSON decode failure) are silently dropped —
// the catalog shouldn't have them in normal operation; the
// alternative ("fail the whole list") would hide the working
// presets from operators chasing a single bad entry.
func (s *Source) ListPresets(ctx context.Context) ([]*model.Preset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	details, err := s.loadAllDetails(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Preset, 0, len(details))
	for _, d := range details {
		p := d.Preset
		out = append(out, &p)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// GetPreset returns the full PresetDetail for one id. 404 sentinel
// when the id is absent from the ConfigMap.
func (s *Source) GetPreset(ctx context.Context, id string) (*model.PresetDetail, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, ErrPresetNotFound
	}
	details, err := s.loadAllDetails(ctx)
	if err != nil {
		return nil, err
	}
	for i := range details {
		if details[i].ID == id {
			return &details[i], nil
		}
	}
	return nil, ErrPresetNotFound
}

// loadAllDetails fetches the ConfigMap, JSON-decodes every data
// key as a PresetDetail. ConfigMap-missing → ErrConfigMapNotFound;
// other apiserver errors pass through wrapped.
func (s *Source) loadAllDetails(ctx context.Context) ([]model.PresetDetail, error) {
	cm, err := s.client.CoreV1().ConfigMaps(s.namespace).
		Get(ctx, s.name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: %s/%s", ErrConfigMapNotFound, s.namespace, s.name)
		}
		return nil, fmt.Errorf("configmap: get ConfigMap: %w", err)
	}
	out := make([]model.PresetDetail, 0, len(cm.Data))
	keys := make([]string, 0, len(cm.Data))
	for k := range cm.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		var d model.PresetDetail
		if err := json.Unmarshal([]byte(cm.Data[k]), &d); err != nil {
			// Bad JSON for this key — skip silently, don't break the
			// catalog over one bad entry. Phase 3 could wire a
			// `h.Logger.Warn` once we plumb a logger through here.
			continue
		}
		if d.ID == "" {
			// Fall back to the data key when the inline `id` field
			// is omitted. Operators that hand-edit the ConfigMap
			// can rely on the key being the canonical id.
			d.ID = k
		}
		out = append(out, d)
	}
	return out, nil
}

// ---- Stubs returning ErrCapabilityUnavailable ----

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
func (s *Source) Deploy(_ context.Context, _ *model.DeployRequest) (*model.DeployResponse, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) DeleteDeploy(_ context.Context, _ string) error {
	return datasource.ErrCapabilityUnavailable
}
func (s *Source) QueryMetric(_ context.Context, _ string, _ map[string]string, _ model.TimeRange) (*model.MetricQueryResponse, error) {
	return nil, datasource.ErrCapabilityUnavailable
}
func (s *Source) StreamEvents(_ context.Context, _ model.StreamEventsOptions) (<-chan *model.WSMessage, error) {
	return nil, datasource.ErrCapabilityUnavailable
}

// loadRESTConfig mirrors the k8s / crd source pattern.
func loadRESTConfig(path string) (*rest.Config, error) {
	if path == "" {
		return rest.InClusterConfig()
	}
	return clientcmd.BuildConfigFromFlags("", path)
}
