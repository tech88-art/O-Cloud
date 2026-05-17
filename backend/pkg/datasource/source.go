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
}
