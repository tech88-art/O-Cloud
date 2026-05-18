// Package mock — topology (P1-T-102).
//
// GetTopology composes the cluster→node→npu→slice graph by loading the four
// underlying fixtures (clusters / nodes / npus / slices) through the existing
// per-resource lazy caches, then delegating to pkg/aggregator.BuildTopology.
//
// The slices fixture is loaded into a fresh sync.Once cache on Source because
// loadNPUs already reads slices.json but discards the flat array (it attaches
// slices onto their parent NPU). Topology needs the flat array; we load it
// independently here.
package mock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/example/ocloud-edge/backend/pkg/aggregator"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

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

// GetTopology assembles the topology DTO for the given cluster id.
//
// Errors:
//   - ctx.Err() is propagated verbatim (request cancellation).
//   - When the cluster id is unknown, returns ErrTopologyClusterNotFound
//     (handler maps to 404).
//   - Fixture load failures bubble up untyped (handler maps to 500).
//
// Implementation chooses pre-filtering NPUs/slices by what the depth needs:
// loadNPUs and loadSlicesFlat both cost the full file regardless of depth,
// but aggregator.BuildTopology then prunes by depth. Net cost is bounded by
// set-a-small fixture size (24 NPUs / ~18 slices) — no point optimizing
// further until set-c-stress arrives.
func (s *Source) GetTopology(ctx context.Context, clusterID string, depth string) (*model.Topology, error) {
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

	topo := aggregator.BuildTopology(aggregator.TopologyInputs{
		Cluster: cluster,
		Nodes:   nodes,
		NPUs:    npus,
		Slices:  slices,
		Depth:   depth,
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
