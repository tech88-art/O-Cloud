// Package mock — node fixtures.
//
// Loads nodes once (lazy) from <fixturesPath>/nodes.json and serves the in-memory
// slice. fixturesPath is the directory passed to NewSource (e.g. the
// configs/mock-data/set-a-small constant referenced by backend/CLAUDE.md §4.2);
// if it is empty we fall back to ./configs/mock-data/set-a-small relative to
// the working directory.
//
// PHASE-1 (P1-T-103). When T101 (clusters) merges first and introduces a
// shared constant for the default path, callers should switch to that — see
// the DefaultFixturesPath constant in this file which is the placeholder.
package mock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// DefaultFixturesPath is the relative directory under which mock fixtures live.
// If T101 lands a canonical constant first, callers should prefer that one
// (see backend/CLAUDE.md §4.2) — this is a fallback so node.go is self-contained
// while T101 is still in flight on its own worktree.
const DefaultFixturesPath = "./configs/mock-data/set-a-small"

// ErrNodeNotFound is returned by GetNodeDetail when no fixture matches the
// requested name. Wrapped errors elsewhere can errors.Is against it.
var ErrNodeNotFound = errors.New("mock source: node not found")

// nodesFile is the unmarshal shape of nodes.json. The fixture in
// configs/mock-data/set-a-small/nodes.json is a top-level object with a
// `nodes` array (alongside other resources); we ignore everything else.
type nodesFile struct {
	Nodes []model.NodeDetail `json:"nodes"`
}

// resetNodesStateForTest clears the lazy cache so tests can reload a tmp
// fixture. Production code never calls this.
//
//nolint:unused // used by tests in this package; kept for future _test.go expansion
func (s *Source) resetNodesStateForTest() {
	s.nodesOnce = sync.Once{}
	s.nodes = nil
	s.nodesErr = nil
}

// loadNodes reads nodes.json from fixturesPath on first call. Subsequent calls
// return the cached slice (or cached error). State is per-Source instance so
// tests can construct a fresh Source against a tmp dir without interference.
func (s *Source) loadNodes() ([]model.NodeDetail, error) {
	s.nodesOnce.Do(func() {
		path := s.fixturesPath
		if path == "" {
			path = DefaultFixturesPath
		}
		fp := filepath.Join(path, "nodes.json")
		data, err := os.ReadFile(fp) //nolint:gosec // fp is composed from a config-supplied directory; not user input
		if err != nil {
			s.nodesErr = fmt.Errorf("read mock nodes fixture %q: %w", fp, err)
			return
		}
		var raw nodesFile
		if err := json.Unmarshal(data, &raw); err != nil {
			s.nodesErr = fmt.Errorf("parse mock nodes fixture %q: %w", fp, err)
			return
		}
		s.nodes = raw.Nodes
	})
	return s.nodes, s.nodesErr
}

// ListNodes returns nodes matching filter. The filter is AND across non-empty
// fields. Role match is "any of node.Role contains filter.Role".
func (s *Source) ListNodes(_ context.Context, filter model.NodeFilter) ([]*model.Node, error) {
	all, err := s.loadNodes()
	if err != nil {
		return nil, err
	}
	out := make([]*model.Node, 0, len(all))
	for i := range all {
		nd := &all[i].Node
		if filter.ClusterID != "" && nd.ClusterID != filter.ClusterID {
			continue
		}
		if filter.Role != "" && !containsRole(nd.Role, filter.Role) {
			continue
		}
		if filter.PoolName != "" {
			// PHASE-1: set-a-small has no node-pool membership data in nodes.json
			// → filter narrows to zero. Phase-2 source will join against pools.
			continue
		}
		// Copy out so callers can't mutate the cached fixture.
		n := *nd
		out = append(out, &n)
	}
	return out, nil
}

// GetNodeDetail returns the full NodeDetail for the requested name, or
// ErrNodeNotFound.
func (s *Source) GetNodeDetail(_ context.Context, name string) (*model.NodeDetail, error) {
	all, err := s.loadNodes()
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].Name == name {
			nd := all[i] // copy
			return &nd, nil
		}
	}
	return nil, ErrNodeNotFound
}

// containsRole reports whether the role list (`control-plane` / `worker` /
// `edge`) contains target.
func containsRole(roles []string, target string) bool {
	for _, r := range roles {
		if r == target {
			return true
		}
	}
	return false
}
