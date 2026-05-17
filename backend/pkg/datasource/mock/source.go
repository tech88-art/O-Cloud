// Package mock provides a placeholder Source implementation.
//
// PHASE-1 scope (P1-T-005): all methods return ErrNotImplemented. T101-T203
// will fill them in resource-by-resource.
package mock

import (
	"context"
	"errors"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// ErrNotImplemented is the sentinel returned by every Source method in this
// stub. Tests can errors.Is against it to assert we have not yet wired a
// method.
var ErrNotImplemented = errors.New("mock source: method not implemented")

// Source is the mock datasource stub. Construction is intentionally trivial
// at this phase — T101 will give it a path-to-fixtures argument and load
// JSON from configs/mock-data/set-a-small/.
type Source struct {
	// fixturesPath is reserved for T101+; held here so callers already know
	// the constructor shape.
	fixturesPath string
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
	// PHASE-1: stub declares no real capabilities until T101+ implements them.
	return datasource.Capabilities{}
}

// ---- Cluster ----

func (s *Source) ListClusters(ctx context.Context) ([]*model.Cluster, error) {
	return nil, ErrNotImplemented
}

func (s *Source) GetCluster(ctx context.Context, id string) (*model.Cluster, error) {
	return nil, ErrNotImplemented
}

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
