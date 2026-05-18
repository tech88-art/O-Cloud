// Package datasource defines the core abstraction the backend uses to fetch
// data. Implementations live in subpackages (mock, k8s, crd, prometheus,
// configmap) and are wired by factory.go according to configs/config.yaml.
//
// Adding a method here forces every implementation to add it (compile-time
// enforcement) — see backend/CLAUDE.md §4.1.
package datasource

import (
	"context"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// TopologyOptions carries optional flags accepted by GetTopologyWithFabric.
//
// We chose a struct (instead of additional positional args) so future fabric-
// related flags — e.g. `IncludeBindings`, fabric tier filters — can land
// without changing the interface signature again. See ADR-0004 §"Topology API
// extension" for the design intent.
type TopologyOptions struct {
	// IncludeFabric, when true, asks the source to add `type=switch` nodes
	// and `type=fabric-link` edges to the returned topology. When false, the
	// returned graph is byte-equivalent to GetTopology(ctx, id, depth) — a
	// zero-regression contract callers can rely on.
	IncludeFabric bool
}

// Capabilities is the self-declared set of methods a Source supports.
//
// A Source returns false for methods it stubs (returns ErrNotImplemented).
// The factory uses this to mount only supported handlers when a config maps a
// resource to a source that lacks coverage.
type Capabilities struct {
	Clusters  bool
	Topology  bool
	Nodes     bool
	NPUs      bool
	Pools     bool
	Workloads bool
	Logs      bool
	Presets   bool
	Deploy    bool
	Metrics   bool
	// Events indicates the source can stream WSMessage events via
	// StreamEvents. Mock flips this on; k8s/crd will once an informer-backed
	// stream lands (PHASE-2).
	Events bool
}

// Source is the contract every datasource implements. See backend/CLAUDE.md
// §4.1 — this MUST stay in sync with what handlers in pkg/api/ call.
type Source interface {
	// Identity
	Name() string
	Capabilities() Capabilities

	// Cluster
	ListClusters(ctx context.Context) ([]*model.Cluster, error)
	GetCluster(ctx context.Context, id string) (*model.Cluster, error)
	GetTopology(ctx context.Context, clusterID string, depth string) (*model.Topology, error)
	// GetTopologyWithFabric is the option-bag variant of GetTopology. It is
	// kept as a separate method (rather than extending GetTopology's
	// signature) so existing call sites and test doubles that override
	// GetTopology stay binary-compatible. Implementations MUST return the
	// same bytes as GetTopology(ctx, clusterID, depth) when opts is the zero
	// value — P1-T-211 AC depends on this zero-regression contract.
	//
	// PHASE-2: when a future fabric flag joins TopologyOptions, callers
	// migrate by adding the option; the legacy GetTopology stays untouched.
	GetTopologyWithFabric(ctx context.Context, clusterID string, depth string, opts TopologyOptions) (*model.Topology, error)

	// Node / NPU / Slice
	ListNodes(ctx context.Context, filter model.NodeFilter) ([]*model.Node, error)
	GetNodeDetail(ctx context.Context, name string) (*model.NodeDetail, error)
	ListNPUs(ctx context.Context, nodeName string) ([]*model.NPU, error)

	// Pool (CRD source territory; mock can return mock data)
	ListNPUSlicePools(ctx context.Context) ([]*model.NPUSlicePool, error)

	// Workload
	ListWorkloads(ctx context.Context, filter model.WorkloadFilter) ([]*model.Workload, error)
	GetWorkloadDetail(ctx context.Context, namespace, name string) (*model.WorkloadDetail, error)
	GetWorkloadLogs(ctx context.Context, namespace, name string, opts model.LogOptions) (*model.LogPage, error)

	// Deploy
	ListPresets(ctx context.Context) ([]*model.Preset, error)
	GetPreset(ctx context.Context, id string) (*model.PresetDetail, error)
	Deploy(ctx context.Context, req *model.DeployRequest) (*model.DeployResponse, error)
	DeleteDeploy(ctx context.Context, deployID string) error

	// Metrics
	QueryMetric(ctx context.Context, templateID string, vars map[string]string, timeRange model.TimeRange) (*model.MetricQueryResponse, error)

	// Events streams WSMessage envelopes from the source. Implementations
	// return a receive-only channel that closes when (a) the underlying
	// fixture/stream is drained, or (b) ctx is cancelled. Callers MUST drain
	// the channel until close to avoid leaking the producer goroutine.
	//
	// PHASE-1: mock.Source replays configs/mock-data/.../events.json at the
	// declared offsets (real-time) — see backend/pkg/datasource/mock/events.go.
	// PHASE-2: k8s.Source will adapt informer events; capability flag gates
	// which source the /ws/topology handler picks up.
	StreamEvents(ctx context.Context, opts model.StreamEventsOptions) (<-chan *model.WSMessage, error)
}
