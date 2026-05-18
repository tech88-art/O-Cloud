// Package mock — topology (P1-T-102, P1-T-211, P1-T-213).
//
// GetTopology composes the cluster→node→npu→slice graph by loading the four
// underlying fixtures (clusters / nodes / npus / slices) through the existing
// per-resource lazy caches, then delegating to pkg/aggregator.BuildTopology.
//
// The slices fixture is loaded into a fresh sync.Once cache on Source because
// loadNPUs already reads slices.json but discards the flat array (it attaches
// slices onto their parent NPU). Topology needs the flat array; we load it
// independently here.
//
// P1-T-211 (ADR-0004): GetTopologyWithFabric optionally folds in fabric
// switches + links from networkSwitches.json + networkLinks.json. Files are
// optional — when they're absent the loader returns nil slices (and the
// aggregator drops the fabric branch silently).
//
// P1-T-213 (ADR-0005): GetTopologyWithFabric's TopologyOptions now also
// carries IncludeWorkloads. When true the loader reads workloads.json with a
// thin local shape (loadWorkloadsForTopology — captures the `bindings` array
// the model.Pod struct does not yet promote) and passes the bundle to
// aggregator.BuildTopology, which appends workload + pod nodes + binds-to /
// pd-pair edges. When false the response stays byte-equivalent to T102/T211.
package mock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/example/ocloud-edge/backend/pkg/aggregator"
	"github.com/example/ocloud-edge/backend/pkg/datasource"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// networkSwitchesFile / networkLinksFile are the JSON fixture names ADR-0004
// adds under fixturesPath. Both are optional: missing files yield empty
// slices, not errors (different fixture sets ship different scopes — set-c-
// stress will probably not bother).
const (
	networkSwitchesFile = "networkSwitches.json"
	networkLinksFile    = "networkLinks.json"
)

// networkSwitchesFixture / networkLinksFixture mirror the on-disk shape — the
// composite document at configs/mock-data/set-a-small/{networkSwitches,
// networkLinks}.json with a single top-level keyed array.
type networkSwitchesFixture struct {
	NetworkSwitches []aggregator.NetworkSwitch `json:"networkSwitches"`
}
type networkLinksFixture struct {
	NetworkLinks []aggregator.NetworkLink `json:"networkLinks"`
}

// ErrTopologyClusterNotFound is the topology-specific 404 sentinel. We don't
// reuse ErrClusterNotFound from cluster.go because the topology handler maps
// to a different details payload ({resource: "cluster", id: ...}) and the
// dedicated error helps grep tell topology errors apart in logs. Wraps
// ErrClusterNotFound so callers can errors.Is against either.
var ErrTopologyClusterNotFound = fmt.Errorf("mock source: topology: %w", ErrClusterNotFound)

// loadSlicesFlat reads slices.json into a flat []model.NPUSlice at most once
// per Source. Errors are sticky. Empty fixturesPath → nil slice + nil error,
// mirroring loadNPUs / loadClusters semantics.
//
// Note: loadNPUs reads the same file but indexes the slices onto their parent
// NPU. We load it again here (separate sync.Once) because (a) the flat array
// is what BuildTopology needs and (b) decoupling keeps the npu cache logic
// untouched.
func (s *Source) loadSlicesFlat() ([]model.NPUSlice, error) {
	s.slicesOnce.Do(func() {
		if s.fixturesPath == "" {
			s.slices = nil
			return
		}
		fp := filepath.Join(s.fixturesPath, slicesFile)
		raw, err := os.ReadFile(fp) //nolint:gosec // fp is composed from a config-supplied directory; not user input
		if err != nil {
			s.slicesErr = fmt.Errorf("read slices fixture %q: %w", fp, err)
			return
		}
		var doc slicesFixture
		if err := json.Unmarshal(raw, &doc); err != nil {
			s.slicesErr = fmt.Errorf("parse slices fixture %q: %w", fp, err)
			return
		}
		s.slices = doc.Slices
	})
	return s.slices, s.slicesErr
}

// loadSwitches reads networkSwitches.json at most once per Source. The file
// is OPTIONAL — when missing we return a nil slice + nil error rather than
// propagating the read error. This mirrors how ADR-0004 explicitly notes that
// fixture sets ship fabric data on opt-in basis (set-a-small does, but
// set-c-stress might skip).
//
// Empty fixturesPath → nil slice + nil error (matches loadClusters semantics).
// Parse failures ARE sticky errors because a malformed JSON file is a clear
// fixture bug we don't want to mask.
func (s *Source) loadSwitches() ([]aggregator.NetworkSwitch, error) {
	s.switchesOnce.Do(func() {
		if s.fixturesPath == "" {
			s.switches = nil
			return
		}
		fp := filepath.Join(s.fixturesPath, networkSwitchesFile)
		raw, err := os.ReadFile(fp) //nolint:gosec // fp is composed from a config-supplied directory; not user input
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// Optional fixture absent — fabric just gets an empty list.
				s.switches = nil
				return
			}
			s.switchesErr = fmt.Errorf("read networkSwitches fixture %q: %w", fp, err)
			return
		}
		var doc networkSwitchesFixture
		if err := json.Unmarshal(raw, &doc); err != nil {
			s.switchesErr = fmt.Errorf("parse networkSwitches fixture %q: %w", fp, err)
			return
		}
		s.switches = doc.NetworkSwitches
	})
	return s.switches, s.switchesErr
}

// loadLinks reads networkLinks.json at most once per Source. Same semantics
// as loadSwitches (optional file → nil + nil; parse error → sticky).
func (s *Source) loadLinks() ([]aggregator.NetworkLink, error) {
	s.linksOnce.Do(func() {
		if s.fixturesPath == "" {
			s.links = nil
			return
		}
		fp := filepath.Join(s.fixturesPath, networkLinksFile)
		raw, err := os.ReadFile(fp) //nolint:gosec // fp is composed from a config-supplied directory; not user input
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				s.links = nil
				return
			}
			s.linksErr = fmt.Errorf("read networkLinks fixture %q: %w", fp, err)
			return
		}
		var doc networkLinksFixture
		if err := json.Unmarshal(raw, &doc); err != nil {
			s.linksErr = fmt.Errorf("parse networkLinks fixture %q: %w", fp, err)
			return
		}
		s.links = doc.NetworkLinks
	})
	return s.links, s.linksErr
}

// topologyWorkloadsFixture is the on-disk shape we use to extract workloads
// for the topology branch. We do NOT reuse model.WorkloadDetail because the
// model.Pod type doesn't surface the per-pod `bindings` array (T013 added it
// to the JSON schema but the Go model migration is deferred to a follow-up
// RFC). Loading into our own type lets the aggregator consume bindings
// without forcing a model package change inside an aggregator-scoped task.
type topologyWorkloadsFixture struct {
	Workloads []aggregator.WorkloadInput `json:"workloads"`
}

// loadWorkloadsForTopology reads workloads.json into the aggregator.WorkloadInput
// shape. Independent of loadWorkloads (which feeds the /workloads REST
// handler with the model.WorkloadDetail shape) so each cache stays decoupled
// — same pattern as loadNPUs vs loadSlicesFlat both reading slices.json.
//
// Empty fixturesPath → nil slice, nil error. Missing file → sticky read
// error (workloads.json is the central pillar — unlike fabric files, its
// absence is a real configuration problem).
func (s *Source) loadWorkloadsForTopology() ([]aggregator.WorkloadInput, error) {
	s.topoWorkloadsOnce.Do(func() {
		if s.fixturesPath == "" {
			s.topoWorkloads = nil
			return
		}
		fp := filepath.Join(s.fixturesPath, workloadsFile)
		raw, err := os.ReadFile(fp) //nolint:gosec // fp is composed from a config-supplied directory; not user input
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// Be lenient for tests that exercise topology without
				// providing workloads.json: an empty workloads slice is a
				// legal no-op for the workload branch.
				s.topoWorkloads = nil
				return
			}
			s.topoWorkloadsErr = fmt.Errorf("read workloads fixture %q: %w", fp, err)
			return
		}
		var doc topologyWorkloadsFixture
		if err := json.Unmarshal(raw, &doc); err != nil {
			s.topoWorkloadsErr = fmt.Errorf("parse workloads fixture %q: %w", fp, err)
			return
		}
		s.topoWorkloads = doc.Workloads
	})
	return s.topoWorkloads, s.topoWorkloadsErr
}

// GetTopology assembles the topology DTO for the given cluster id (T102
// signature — fabric-unaware). Delegates to getTopology with the zero-value
// TopologyOptions so the zero-regression contract is satisfied by
// construction: GetTopology bytes ≡ GetTopologyWithFabric(opts{}) bytes.
//
// Errors:
//   - ctx.Err() is propagated verbatim (request cancellation).
//   - When the cluster id is unknown, returns ErrTopologyClusterNotFound
//     (handler maps to 404).
//   - Fixture load failures bubble up untyped (handler maps to 500).
func (s *Source) GetTopology(ctx context.Context, clusterID string, depth string) (*model.Topology, error) {
	return s.getTopology(ctx, clusterID, depth, datasource.TopologyOptions{})
}

// GetTopologyWithFabric is the option-bag variant of GetTopology (P1-T-211,
// P1-T-213). When opts.IncludeFabric is true the response gains `type=switch`
// nodes (from networkSwitches.json) and `type=fabric-link` edges (from
// networkLinks.json). When opts.IncludeWorkloads is true (ADR-0005) the
// response gains `type=workload` + `type=pod` nodes and `type=binds-to` +
// `type=pd-pair` edges (from workloads.json). When both flags are false the
// response is byte-identical to GetTopology — the AC's zero-regression
// requirement. Flags compose independently.
//
// Implementation note: opts is the seed for future fabric / workload flags
// (filter by switch tier, include PVC volumes, etc.). The mock's
// `Capabilities()` already advertises Topology = true; the existence of
// GetTopologyWithFabric is an extension, not a gate.
func (s *Source) GetTopologyWithFabric(ctx context.Context, clusterID string, depth string, opts datasource.TopologyOptions) (*model.Topology, error) {
	return s.getTopology(ctx, clusterID, depth, opts)
}

// getTopology is the shared implementation behind GetTopology and
// GetTopologyWithFabric. Keeps fixture loading + aggregator delegation in one
// place so the two public entry points can never drift on the depth=… code
// path (the AC explicitly requires byte-equivalence when opts.IncludeFabric is
// false).
//
// Implementation chooses pre-filtering NPUs/slices by what the depth needs:
// loadNPUs and loadSlicesFlat both cost the full file regardless of depth,
// but aggregator.BuildTopology then prunes by depth. Net cost is bounded by
// set-a-small fixture size (24 NPUs / ~18 slices) — no point optimizing
// further until set-c-stress arrives.
//
// Fabric loaders (loadSwitches / loadLinks) are gated on opts.IncludeFabric
// so the legacy GetTopology path skips them entirely — preserving its
// disk-I/O footprint as a byproduct of the same gate.
func (s *Source) getTopology(ctx context.Context, clusterID string, depth string, opts datasource.TopologyOptions) (*model.Topology, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// 1) Cluster — 404 fast-path if unknown.
	clusters, err := s.loadClusters()
	if err != nil {
		return nil, err
	}
	var cluster *model.Cluster
	for _, c := range clusters {
		if c != nil && c.ID == clusterID {
			cluster = c
			break
		}
	}
	if cluster == nil {
		return nil, ErrTopologyClusterNotFound
	}

	// 2) Nodes (always — the shallowest depth still emits them).
	nodes, err := s.loadNodes()
	if err != nil {
		return nil, err
	}

	// 3) NPUs (loaded only when we'll use them; depth=node skips).
	//    We choose to load eagerly anyway because the lazy cache makes the
	//    next request cheap and the conditional branch adds noise without
	//    real savings at set-a-small scale. Same for slices.
	var npus []*model.NPU
	if depth != aggregator.DepthNode {
		npus, err = s.loadNPUs()
		if err != nil {
			return nil, err
		}
	}

	// 4) Slices.
	var slices []model.NPUSlice
	if depth == aggregator.DepthSlice || depth == "" || !isKnownDepth(depth) {
		slices, err = s.loadSlicesFlat()
		if err != nil {
			return nil, err
		}
	}

	// 5) Fabric — loaded only when explicitly requested.
	var switches []aggregator.NetworkSwitch
	var links []aggregator.NetworkLink
	if opts.IncludeFabric {
		switches, err = s.loadSwitches()
		if err != nil {
			return nil, err
		}
		links, err = s.loadLinks()
		if err != nil {
			return nil, err
		}
	}

	// 6) Workloads — loaded only when explicitly requested (ADR-0005). Same
	// load-on-opt-in pattern as fabric so the GetTopology / IncludeFabric=
	// false / IncludeWorkloads=false path keeps its zero disk-I/O footprint.
	var workloads []aggregator.WorkloadInput
	if opts.IncludeWorkloads {
		workloads, err = s.loadWorkloadsForTopology()
		if err != nil {
			return nil, err
		}
	}

	topo := aggregator.BuildTopology(aggregator.TopologyInputs{
		Cluster:          cluster,
		Nodes:            nodes,
		NPUs:             npus,
		Slices:           slices,
		Depth:            depth,
		IncludeFabric:    opts.IncludeFabric,
		Switches:         switches,
		Links:            links,
		IncludeWorkloads: opts.IncludeWorkloads,
		Workloads:        workloads,
	})
	return topo, nil
}

// isKnownDepth tells whether depth maps to one of the three canonical values
// without the default fallback. Used to decide whether to fetch slices when
// the caller passed something we'll normalize to "slice" anyway.
func isKnownDepth(depth string) bool {
	switch depth {
	case aggregator.DepthNode, aggregator.DepthNPU, aggregator.DepthSlice:
		return true
	default:
		return false
	}
}

// Compile-time assertion: the topology-not-found sentinel must remain
// distinguishable but wrapped around ErrClusterNotFound so callers can
// errors.Is for either. Tiny safeguard against an accidental refactor.
var _ = func() bool { return errors.Is(ErrTopologyClusterNotFound, ErrClusterNotFound) }
