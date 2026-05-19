// Package datasource — factory tests (P1-T-303).
//
// The point of this file is to formalize the invariant that
// `datasource.Build` honors the config's `mapping` table at runtime — i.e.
// changing `mapping.<resource>` in YAML and restarting the binary is enough
// to swap which source backs a logical resource. This is the "数据源切换只
// 改配置" promise from the Phase 1 DoD; we lock it in here so a future
// refactor that accidentally hard-wires a mapping fails CI.
package datasource

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/ocloud-edge/backend/pkg/config"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

func newDispatchTestCounter(t *testing.T) *prometheus.CounterVec {
	t.Helper()
	return prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ocloud_backend_dispatch_calls_total_test",
	}, []string{"datasource", "endpoint"})
}

func dispatchCount(t *testing.T, c *prometheus.CounterVec, datasource, endpoint string) float64 {
	t.Helper()
	return testutil.ToFloat64(c.WithLabelValues(datasource, endpoint))
}

// stubSource is a hollow Source implementation used only for the wiring
// tests below. It carries a name() distinguisher so the assertions can tell
// "did Build pick source A or source B" without needing to construct two
// concrete mock.Source instances against fixture directories.
type stubSource struct {
	id           string
	capabilities Capabilities
}

func (s *stubSource) Name() string             { return s.id }
func (s *stubSource) Capabilities() Capabilities { return s.capabilities }

// The remainder of the interface is satisfied with panic-on-call stubs.
// Build doesn't invoke them; tests below only read Name() / Capabilities().
func (s *stubSource) ListClusters(_ context.Context) ([]*model.Cluster, error) {
	panic("not used in factory tests")
}
func (s *stubSource) GetCluster(_ context.Context, _ string) (*model.Cluster, error) {
	panic("not used in factory tests")
}
func (s *stubSource) GetTopology(_ context.Context, _, _ string) (*model.Topology, error) {
	panic("not used in factory tests")
}
func (s *stubSource) GetTopologyWithFabric(_ context.Context, _, _ string, _ TopologyOptions) (*model.Topology, error) {
	panic("not used in factory tests")
}
func (s *stubSource) ListNodes(_ context.Context, _ model.NodeFilter) ([]*model.Node, error) {
	panic("not used in factory tests")
}
func (s *stubSource) GetNodeDetail(_ context.Context, _ string) (*model.NodeDetail, error) {
	panic("not used in factory tests")
}
func (s *stubSource) ListNPUs(_ context.Context, _ string) ([]*model.NPU, error) {
	panic("not used in factory tests")
}
func (s *stubSource) ListNPUSlicePools(_ context.Context) ([]*model.NPUSlicePool, error) {
	panic("not used in factory tests")
}
func (s *stubSource) ListWorkloads(_ context.Context, _ model.WorkloadFilter) ([]*model.Workload, error) {
	panic("not used in factory tests")
}
func (s *stubSource) GetWorkloadDetail(_ context.Context, _, _ string) (*model.WorkloadDetail, error) {
	panic("not used in factory tests")
}
func (s *stubSource) GetWorkloadLogs(_ context.Context, _, _ string, _ model.LogOptions) (*model.LogPage, error) {
	panic("not used in factory tests")
}
func (s *stubSource) StreamWorkloadLogs(_ context.Context, _, _ string, _ model.LogStreamOptions) (<-chan *model.LogLine, error) {
	panic("not used in factory tests")
}
func (s *stubSource) ListPresets(_ context.Context) ([]*model.Preset, error) {
	panic("not used in factory tests")
}
func (s *stubSource) GetPreset(_ context.Context, _ string) (*model.PresetDetail, error) {
	panic("not used in factory tests")
}
func (s *stubSource) Deploy(_ context.Context, _ *model.DeployRequest) (*model.DeployResponse, error) {
	panic("not used in factory tests")
}
func (s *stubSource) DeleteDeploy(_ context.Context, _ string) error {
	panic("not used in factory tests")
}
func (s *stubSource) QueryMetric(_ context.Context, _ string, _ map[string]string, _ model.TimeRange) (*model.MetricQueryResponse, error) {
	panic("not used in factory tests")
}
func (s *stubSource) StreamEvents(_ context.Context, _ model.StreamEventsOptions) (<-chan *model.WSMessage, error) {
	panic("not used in factory tests")
}

func TestBuild_RespectsMappingTable(t *testing.T) {
	srcA := &stubSource{id: "mock-a"}
	srcB := &stubSource{id: "mock-b"}

	cfg := &config.Config{
		Datasources: map[string]config.DatasourceConfig{
			"mock-a": {Enabled: true, Path: "../mock-data/set-a-small"},
			"mock-b": {Enabled: true, Path: "../mock-data/set-b-small"},
		},
		Mapping: map[string]string{
			"clusters":  "mock-a",
			"topology":  "mock-a",
			"workloads": "mock-b",
			"logs":      "mock-b",
		},
	}

	reg, err := Build(cfg, map[string]Source{
		"mock-a": srcA,
		"mock-b": srcB,
	})
	require.NoError(t, err)

	// The clusters resource resolves to mock-a; workloads resolves to mock-b.
	// If a refactor accidentally hard-coded mock as the only source, these
	// would both come back the same — and this assertion catches it.
	assert.Same(t, srcA, reg.SourceFor("clusters"))
	assert.Same(t, srcA, reg.SourceFor("topology"))
	assert.Same(t, srcB, reg.SourceFor("workloads"))
	assert.Same(t, srcB, reg.SourceFor("logs"))
}

func TestBuild_SkipsDisabledDatasources(t *testing.T) {
	srcA := &stubSource{id: "mock-a"}
	srcB := &stubSource{id: "mock-b"}

	cfg := &config.Config{
		Datasources: map[string]config.DatasourceConfig{
			"mock-a": {Enabled: true},
			"mock-b": {Enabled: false}, // turned off
		},
		Mapping: map[string]string{
			"clusters":  "mock-a",
			"workloads": "mock-b", // maps to a disabled source
		},
	}

	reg, err := Build(cfg, map[string]Source{
		"mock-a": srcA,
		"mock-b": srcB,
	})
	require.NoError(t, err)

	assert.Same(t, srcA, reg.SourceFor("clusters"))
	// Mapping points at mock-b but Build dropped it because Enabled=false.
	// The handler must defend against this nil — the resource is unmapped
	// in effect.
	assert.Nil(t, reg.SourceFor("workloads"),
		"disabled datasources must not surface even when mapping references them")
}

func TestBuild_MappingChangeAtRuntimeNeedsNoRecompile(t *testing.T) {
	// "Runtime" here is "a second invocation of Build with a fresh config"
	// — same binary, different YAML. This is the canonical "P1-T-303 mapping
	// swap" demo: same `sources` slice, different `cfg.Mapping`, different
	// resolution.
	srcMain := &stubSource{id: "mock-main"}
	srcAlt := &stubSource{id: "mock-alt"}
	sources := map[string]Source{
		"mock-main": srcMain,
		"mock-alt":  srcAlt,
	}

	cfgA := &config.Config{
		Datasources: map[string]config.DatasourceConfig{
			"mock-main": {Enabled: true},
			"mock-alt":  {Enabled: true},
		},
		Mapping: map[string]string{
			"clusters": "mock-main",
			"logs":     "mock-main",
		},
	}
	regA, err := Build(cfgA, sources)
	require.NoError(t, err)
	assert.Same(t, srcMain, regA.SourceFor("clusters"))
	assert.Same(t, srcMain, regA.SourceFor("logs"))

	// Same binary, different mapping: just rebuild Registry with a new
	// config — no source code change.
	cfgB := &config.Config{
		Datasources: cfgA.Datasources,
		Mapping: map[string]string{
			"clusters": "mock-main",
			"logs":     "mock-alt", // swapped
		},
	}
	regB, err := Build(cfgB, sources)
	require.NoError(t, err)
	assert.Same(t, srcMain, regB.SourceFor("clusters"))
	assert.Same(t, srcAlt, regB.SourceFor("logs"),
		"changing the mapping in config alone must redirect resource lookup")
}

func TestBuild_NilConfigReturnsError(t *testing.T) {
	_, err := Build(nil, nil)
	require.Error(t, err)
}

func TestSourceFor_NilRegistry(t *testing.T) {
	var reg *Registry
	assert.Nil(t, reg.SourceFor("anything"))
}

func TestSourceFor_MissingMapping(t *testing.T) {
	reg := &Registry{Sources: map[string]Source{}, Mapping: map[string]string{}}
	assert.Nil(t, reg.SourceFor("clusters"))
}

func TestSourceFor_DispatchCounter_NilSafe(t *testing.T) {
	// Plan acceptance: Phase 3 behaviour preserved when DispatchCounter is nil.
	src := &stubSource{id: "mock"}
	reg := &Registry{
		Sources: map[string]Source{"mock": src},
		Mapping: map[string]string{"clusters": "mock"},
	}
	if got := reg.SourceFor("clusters"); got != src {
		t.Errorf("nil counter must not change lookup behaviour")
	}
}

func TestSourceFor_DispatchCounter_Observable(t *testing.T) {
	counter := newDispatchTestCounter(t)
	src := &stubSource{id: "mock"}
	reg := &Registry{
		Sources:         map[string]Source{"mock": src},
		Mapping:         map[string]string{"clusters": "mock", "nodes": "mock"},
		DispatchCounter: counter,
	}

	// 3 calls on /clusters + 2 calls on /nodes.
	for i := 0; i < 3; i++ {
		_ = reg.SourceFor("clusters")
	}
	for i := 0; i < 2; i++ {
		_ = reg.SourceFor("nodes")
	}
	// 1 lookup for unmapped resource — must NOT increment any label set
	// (Phase 4 only counts successful dispatches).
	if got := reg.SourceFor("unmapped"); got != nil {
		t.Errorf("unmapped resource should yield nil")
	}

	if got := dispatchCount(t, counter, "mock", "clusters"); got != 3 {
		t.Errorf("dispatch counter for mock/clusters = %v, want 3", got)
	}
	if got := dispatchCount(t, counter, "mock", "nodes"); got != 2 {
		t.Errorf("dispatch counter for mock/nodes = %v, want 2", got)
	}
}
