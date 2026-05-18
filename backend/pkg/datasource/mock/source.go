// Package mock provides a placeholder Source implementation.
//
// PHASE-1 scope (P1-T-005): all methods return ErrNotImplemented. T101-T203
// will fill them in resource-by-resource.
package mock

import (
	"context"
	"errors"
	"sync"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// ErrNotImplemented is the sentinel returned by every Source method in this
// stub. Tests can errors.Is against it to assert we have not yet wired a
// method.
var ErrNotImplemented = errors.New("mock source: method not implemented")

// Source is the mock datasource stub. Construction is intentionally trivial
// at this phase — T101 wires Cluster fixtures from configs/mock-data/set-a-small/.
// Subsequent tasks (T103/T104/T201/...) extend this struct with their own
// once+slice cache fields for nodes / npus / workloads / etc.
type Source struct {
	// fixturesPath is the directory of canned JSON fixtures (e.g.
	// "./configs/mock-data/set-a-small"). Empty path → loaders treat the
	// dataset as empty (used in unit tests with no disk dependency).
	fixturesPath string

	// clusters cache (T101). sync.Once gives lazy thread-safe load; clustersErr
	// is sticky so a bad fixture file isn't re-parsed on every request.
	clustersOnce sync.Once
	clusters     []*model.Cluster
	clustersErr  error
}

// NewSource returns a fresh mock.Source. fixturesPath is the directory of
// canned JSON (e.g. "./configs/mock-data/set-a-small"). T101 will start
// reading it.
func NewSource(fixturesPath string) *Source {
	return &Source{fixturesPath: fixturesPath}
}

// Compile-time check: mock.Source must satisfy datasource.Source.
var _ datasource.Source = (*Source)(nil)

// ---- Identity ----

func (s *Source) Name() string { return "mock" }

func (s *Source) Capabilities() datasource.Capabilities {
	// PHASE-1: T101 enables Clusters. Other resources land in later tasks.
	return datasource.Capabilities{
		Clusters: true,
	}
}

// ---- Cluster (T101 — see cluster.go for ListClusters / GetCluster) ----

func (s *Source) GetTopology(ctx context.Context, clusterID string, depth string) (*model.Topology, error) {
	return nil, ErrNotImplemented
}

// ---- Node / NPU ----

func (s *Source) ListNodes(ctx context.Context, filter model.NodeFilter) ([]*model.Node, error) {
	return nil, ErrNotImplemented
}

func (s *Source) GetNodeDetail(ctx context.Context, name string) (*model.NodeDetail, error) {
	return nil, ErrNotImplemented
}

func (s *Source) ListNPUs(ctx context.Context, nodeName string) ([]*model.NPU, error) {
	return nil, ErrNotImplemented
}

// ---- Pool ----

func (s *Source) ListNPUSlicePools(ctx context.Context) ([]*model.NPUSlicePool, error) {
	return nil, ErrNotImplemented
}

// ---- Workload ----

func (s *Source) ListWorkloads(ctx context.Context, filter model.WorkloadFilter) ([]*model.Workload, error) {
	return nil, ErrNotImplemented
}

func (s *Source) GetWorkloadDetail(ctx context.Context, namespace, name string) (*model.WorkloadDetail, error) {
	return nil, ErrNotImplemented
}

func (s *Source) GetWorkloadLogs(ctx context.Context, namespace, name string, opts model.LogOptions) (*model.LogPage, error) {
	return nil, ErrNotImplemented
}

// ---- Deploy ----

func (s *Source) ListPresets(ctx context.Context) ([]*model.Preset, error) {
	return nil, ErrNotImplemented
}

func (s *Source) GetPreset(ctx context.Context, id string) (*model.PresetDetail, error) {
	return nil, ErrNotImplemented
}

func (s *Source) Deploy(ctx context.Context, req *model.DeployRequest) (*model.DeployResponse, error) {
	return nil, ErrNotImplemented
}

func (s *Source) DeleteDeploy(ctx context.Context, deployID string) error {
	return ErrNotImplemented
}

// ---- Metrics ----

func (s *Source) QueryMetric(ctx context.Context, templateID string, vars map[string]string, timeRange model.TimeRange) (*model.MetricQueryResponse, error) {
	return nil, ErrNotImplemented
}
