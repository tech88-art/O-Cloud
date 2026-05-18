// Package mock provides a placeholder Source implementation.
//
// PHASE-1 scope (P1-T-005): all methods return ErrNotImplemented. T101-T203
// will fill them in resource-by-resource.
package mock

import (
	"context"
	"errors"
	"sync"

	"github.com/example/ocloud-edge/backend/pkg/aggregator"
	"github.com/example/ocloud-edge/backend/pkg/datasource"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// ErrNotImplemented is the sentinel returned by every Source method in this
// stub. Tests can errors.Is against it to assert we have not yet wired a
// method.
var ErrNotImplemented = errors.New("mock source: method not implemented")

// Source is the mock datasource. Per-resource lazy caches live on the struct
// so each Source instance loads its own fixtures from fixturesPath. Each
// loader file (cluster.go, node.go, ...) defines its own cache fields below
// and handles empty fixturesPath semantics (some treat empty as no-op for
// test ergonomics; node.go falls back to DefaultFixturesPath).
type Source struct {
	// fixturesPath is the directory containing canned JSON fixtures (e.g.
	// "./configs/mock-data/set-a-small"). Empty path → resource-specific
	// behavior (see each loader).
	fixturesPath string

	// Clusters cache (P1-T-101). sync.Once gives lazy thread-safe load;
	// clustersErr is sticky so a bad fixture file isn't re-parsed.
	clustersOnce sync.Once
	clusters     []*model.Cluster
	clustersErr  error

	// Nodes cache (P1-T-103). Per-instance lazy load; tests reset via
	// resetNodesStateForTest.
	nodesOnce sync.Once
	nodes     []model.NodeDetail
	nodesErr  error

	// NPUs cache (P1-T-104). Lazy load merges npus.json + slices.json so the
	// returned NPUs already carry slice status (see npu.go loadNPUs).
	npusOnce sync.Once
	npus     []*model.NPU
	npusErr  error

	// Slices cache (P1-T-102). loadNPUs already reads slices.json but attaches
	// them onto their parent NPU and discards the flat array. Topology needs
	// the flat array (per-cluster slice nodes), so we cache it separately.
	// Both caches share the same on-disk fixture; whichever loads first
	// triggers the other-or not, they're independent sync.Once'd.
	slicesOnce sync.Once
	slices     []model.NPUSlice
	slicesErr  error

	// Switches / Links caches (P1-T-211, ADR-0004). Loaded lazily only when a
	// caller asks for GetTopologyWithFabric — the GetTopology path never
	// triggers these, so the zero-regression contract is preserved at load
	// level too (no extra disk I/O on the legacy code path). Files are
	// optional: missing networkSwitches.json / networkLinks.json yields a nil
	// slice + nil error (some fixture sets won't ship them).
	switchesOnce sync.Once
	switches     []aggregator.NetworkSwitch
	switchesErr  error

	linksOnce sync.Once
	links     []aggregator.NetworkLink
	linksErr  error

	// Events cache (P1-T-105). Loaded once from events.json; StreamEvents
	// reads from this slice and replays each entry at its declared offset
	// from event[0]. Reuse across subscribers is safe because Event values
	// hold json.RawMessage payloads that the replayer only reads.
	eventsOnce sync.Once
	events     []model.Event
	eventsErr  error

	// Workloads cache (P1-T-201). Lazy load from workloads.json. We cache the
	// full WorkloadDetail records (pods + relations included); ListWorkloads
	// projects to the list-view Workload, GetWorkloadDetail returns the full
	// record.
	workloadsOnce sync.Once
	workloads     []model.WorkloadDetail
	workloadsErr  error

	// Topology-workloads cache (P1-T-213, ADR-0005). Reads workloads.json into
	// the aggregator.WorkloadInput shape — captures the per-pod `bindings`
	// array T013 added to the schema (the model.Pod struct doesn't surface
	// bindings yet; the migration is a follow-up RFC). Decoupled from
	// workloadsOnce/workloads so the existing /workloads handler logic is
	// untouched.
	topoWorkloadsOnce sync.Once
	topoWorkloads     []aggregator.WorkloadInput
	topoWorkloadsErr  error

	// Presets cache (P1-T-203). Lazy load from presets.json — entries are
	// stored as the PresetDetail superset so GetPreset returns the manifest
	// while ListPresets projects out the slim Preset view.
	presetsOnce sync.Once
	presets     []*model.PresetDetail
	presetsErr  error

	// Deploy state (P1-T-202). Guards mutations to the workloads / slices
	// caches that Deploy / DeleteDeploy perform, plus the deployId index that
	// maps a generated deployId back to its workload (namespace/name) so
	// DeleteDeploy can locate the entry without a linear scan + name parse.
	//
	// deployMu MUST be acquired AFTER loadWorkloads / loadNPUs return — those
	// loaders are sync.Once gated and serialize internally, so we never hold
	// deployMu across disk I/O.
	deployMu      sync.Mutex
	deployCounter uint64
	// deployIndex: deployId → "namespace/name" key into s.workloads.
	// Populated on Deploy success, cleared on DeleteDeploy. Keeps DELETE O(N)
	// over the index instead of O(N) over the workload slice on every request.
	deployIndex map[string]string
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
	// PHASE-1: T101 enables Clusters, T102 enables Topology, T103 enables
	// Nodes, T104 enables NPUs, T105 enables Events, T201 enables Workloads,
	// T203 enables Presets, T204 enables Metrics (white-listed PromQL +
	// var-slice). Others flip on as later tasks land.
	return datasource.Capabilities{
		Clusters:  true,
		Topology:  true,
		Nodes:     true,
		NPUs:      true,
		Events:    true,
		Workloads: true,
		Logs:      true,
		Presets:   true,
		Deploy:    true,
		Metrics:   true,
	}
}

// ---- Cluster (T101 — see cluster.go for ListClusters / GetCluster) ----
//
// GetTopology lives in topology.go (P1-T-102).

// ---- Node / NPU ----
//
// ListNodes + GetNodeDetail moved to node.go (P1-T-103).
// ListNPUs moved to npu.go (P1-T-104).

// ---- Pool ----

func (s *Source) ListNPUSlicePools(ctx context.Context) ([]*model.NPUSlicePool, error) {
	return nil, ErrNotImplemented
}

// ---- Workload ----
//
// ListWorkloads + GetWorkloadDetail moved to workload.go (P1-T-201).
// GetWorkloadLogs + StreamWorkloadLogs moved to logs.go (P1-T-301).

// ---- Deploy ----
//
// ListPresets / GetPreset moved to preset.go (P1-T-203).
// Deploy / DeleteDeploy moved to deploy.go (P1-T-202).

// ---- Metrics ----
//
// QueryMetric + ListTemplates live in metrics.go (P1-T-204 — white-listed
// PromQL templates + var-slice synthesis).
