// Package k8s implements the Phase 2 K8s-apiserver-backed
// datasource.Source. Lands per docs/phase2-plan.md, beginning with
// P2-T-001 (clusters + nodes) and accumulating informers / NPUs /
// workloads / logs across the subsequent task packages.
//
// Architecture choices (locked here so the rest of P2-T-00x doesn't
// have to relitigate them):
//
//   - client-go default: in-cluster ServiceAccount (kubeconfig path
//     empty) OR a path-on-disk kubeconfig. main.go passes whichever
//     `cfg.Datasources["k8s"].Kubeconfig` carries.
//   - Construction returns a *Source synchronously; informer / watch
//     wiring lands in P2-T-004 to keep the skeleton independently
//     testable (P2-T-001 only needs a list-against-fake-client).
//   - Capabilities flips on incrementally as each P2-T-00x lands.
//     P2-T-001 turns on Clusters + Nodes; every other method returns
//     datasource.ErrCapabilityUnavailable until its task lands.
//   - The constructor accepts a `kubernetes.Interface` for testability;
//     production callers pass the real clientset, tests pass
//     `fake.NewSimpleClientset(seed...)`.
package k8s

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// Source is the K8s-backed datasource.Source. Construct via NewSource or
// NewSourceWithClient (the second variant is the test surface — pass a
// fake.NewSimpleClientset).
type Source struct {
	client kubernetes.Interface

	// clusterID is the synthetic id we expose via ListClusters. The
	// apiserver doesn't carry a stable "cluster id" of its own; we pull
	// it from the kube-system/cluster-info ConfigMap when present, fall
	// back to a constructor-supplied override, and finally synthesize a
	// "kubernetes" literal so the wire shape always carries SOMETHING.
	clusterID string
	clusterName string
}

// Options bundles construction-time knobs. All fields are optional;
// callers that pass a zero Options get sensible defaults.
type Options struct {
	// KubeconfigPath is the path to a kubeconfig file. Empty → use the
	// in-cluster config (only works when the binary runs as a Pod with a
	// ServiceAccount mounted under /var/run/secrets).
	KubeconfigPath string

	// ClusterIDOverride, if non-empty, is used verbatim as the cluster id
	// in ListClusters / GetCluster results. Falls back to a discovery
	// chain otherwise (cluster-info ConfigMap → "kubernetes").
	ClusterIDOverride string

	// ClusterNameOverride mirrors ClusterIDOverride for the human-facing
	// name. Empty → reuse cluster id.
	ClusterNameOverride string
}

// NewSource builds a production Source against a real apiserver. Tests
// should prefer NewSourceWithClient to avoid the rest config dance.
func NewSource(opts Options) (*Source, error) {
	cfg, err := loadRESTConfig(opts.KubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("k8s: load rest config: %w", err)
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("k8s: build clientset: %w", err)
	}
	return NewSourceWithClient(client, opts), nil
}

// NewSourceWithClient builds a Source against a caller-supplied
// kubernetes.Interface. Used by unit tests with the fake clientset.
//
// The constructor itself is synchronous and side-effect-free; cluster-id
// resolution happens lazily on the first ListClusters call so the
// constructor never blocks on a slow apiserver.
func NewSourceWithClient(client kubernetes.Interface, opts Options) *Source {
	return &Source{
		client:      client,
		clusterID:   opts.ClusterIDOverride,
		clusterName: opts.ClusterNameOverride,
	}
}

// Compile-time check: *Source must satisfy datasource.Source so
// factory.Build can wire it via map[string]datasource.Source.
var _ datasource.Source = (*Source)(nil)

// ---- Identity ----

func (s *Source) Name() string { return "k8s" }

func (s *Source) Capabilities() datasource.Capabilities {
	// P2-T-001 turns on Clusters + Nodes. Every subsequent P2-T-00x
	// flips one or more additional caps in the same struct literal so
	// the progression is easy to grep ("Capabilities()" in commits).
	return datasource.Capabilities{
		Clusters: true,
		Nodes:    true,
	}
}

// ---- Stubs returning ErrCapabilityUnavailable ----
//
// These satisfy the Source interface today. Each method's body is
// replaced wholesale by the corresponding P2-T-00x task.

func (s *Source) GetTopology(_ context.Context, _, _ string) (*model.Topology, error) {
	return nil, datasource.ErrCapabilityUnavailable
}

func (s *Source) GetTopologyWithFabric(_ context.Context, _, _ string, _ datasource.TopologyOptions) (*model.Topology, error) {
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

func (s *Source) QueryMetric(_ context.Context, _ string, _ map[string]string, _ model.TimeRange) (*model.MetricQueryResponse, error) {
	return nil, datasource.ErrCapabilityUnavailable
}

func (s *Source) StreamEvents(_ context.Context, _ model.StreamEventsOptions) (<-chan *model.WSMessage, error) {
	return nil, datasource.ErrCapabilityUnavailable
}

// ---- loadRESTConfig ----

// loadRESTConfig produces a *rest.Config from a path or in-cluster.
//
// Empty path → in-cluster ServiceAccount (the binary must be running
// inside a Pod with a SA token mounted; standard K8s pattern).
//
// Non-empty path → kubeconfig loader; lets local dev / E2E test runners
// point at any cluster.
func loadRESTConfig(kubeconfigPath string) (*rest.Config, error) {
	if kubeconfigPath == "" {
		cfg, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("in-cluster config: %w", err)
		}
		return cfg, nil
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("kubeconfig %q: %w", kubeconfigPath, err)
	}
	return cfg, nil
}

// errFromAPIServer normalises a kubernetes/client-go error. NotFound
// gets a Source-local sentinel so the dispatcher can render a 404; all
// other errors pass through wrapped with a "k8s:" prefix to make
// origin obvious in logs.
//
// Not yet used by source.go itself; cluster.go and node.go consume it
// in the next file additions below.
func errFromAPIServer(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("k8s %s: %w", op, err)
}

// ErrResourceNotFound is the canonical 404 sentinel returned by per-
// resource accessors (GetCluster / GetNodeDetail / ...). Handlers
// errors.Is against it to flip the HTTP status; sibling sources have
// their own equivalent (e.g. mocksrc.ErrWorkloadNotFound) — handlers
// chain the checks.
var ErrResourceNotFound = errors.New("k8s: resource not found")
