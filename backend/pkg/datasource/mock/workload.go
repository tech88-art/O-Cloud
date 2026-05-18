// Package mock — workload fixtures (P1-T-201).
//
// Loads workloads once (lazy) from <fixturesPath>/workloads.json and serves the
// list / detail views. Same pattern as cluster.go / node.go / npu.go:
//
//   - sync.Once gated load with sticky error caching.
//   - Empty fixturesPath → empty slice (no disk dependency, test-friendly).
//   - Filter is AND across non-empty WorkloadFilter fields.
//
// The fixture shape: a composite document where T201 reads only the
// top-level `workloads` array. Each entry maps directly onto model.WorkloadDetail
// (pods + relations live inline alongside list-view fields). Unknown JSON keys
// in the fixture (metrics, bindings, ...) are silently dropped by encoding/json
// — they're served by other endpoints (workload metrics) or not part of the
// current contract.
package mock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// ErrWorkloadNotFound is the 404 sentinel surfaced by GetWorkloadDetail when
// no fixture entry matches the namespace+name pair. Handler maps it to a 404
// with the canonical Error envelope.
var ErrWorkloadNotFound = errors.New("mock source: workload not found")

// workloadsFile is the JSON fixture name under fixturesPath.
const workloadsFile = "workloads.json"

// workloadsFixture is the on-disk shape of workloads.json — we only read the
// top-level `workloads` array. Other sibling keys (clusters / nodes / ...) are
// ignored here.
//
// We unmarshal into WorkloadDetail (not Workload) because the fixture stores
// the full detail (pods + relations) in the same record. ListWorkloads then
// projects out only the embedded Workload fields per the OpenAPI contract.
type workloadsFixture struct {
	Workloads []model.WorkloadDetail `json:"workloads"`
}

// loadWorkloads reads workloadsFile from fixturesPath at most once per Source.
// Errors are sticky.
//
// fixturesPath empty → returns nil slice, nil error. Mirrors cluster.go /
// npu.go semantics so tests can construct an empty Source cheaply.
func (s *Source) loadWorkloads() ([]model.WorkloadDetail, error) {
	s.workloadsOnce.Do(func() {
		if s.fixturesPath == "" {
			s.workloads = nil
			return
		}
		path := filepath.Join(s.fixturesPath, workloadsFile)
		raw, err := os.ReadFile(path) //nolint:gosec // path comes from server config, not user input
		if err != nil {
			s.workloadsErr = fmt.Errorf("read workloads fixture %q: %w", path, err)
			return
		}
		var doc workloadsFixture
		if err := json.Unmarshal(raw, &doc); err != nil {
			s.workloadsErr = fmt.Errorf("parse workloads fixture %q: %w", path, err)
			return
		}
		s.workloads = doc.Workloads
	})
	return s.workloads, s.workloadsErr
}

// ListWorkloads returns the projected Workload (list-view) for every fixture
// entry passing the filter. Filter fields AND-combine and are matched
// case-sensitive against the corresponding workload fields.
//
// Returns an empty slice (never nil) on no matches; the handler emits `[]`
// per the OpenAPI contract.
func (s *Source) ListWorkloads(ctx context.Context, filter model.WorkloadFilter) ([]*model.Workload, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	all, err := s.loadWorkloads()
	if err != nil {
		return nil, err
	}
	out := make([]*model.Workload, 0, len(all))
	for i := range all {
		w := &all[i].Workload
		if filter.Namespace != "" && w.Namespace != filter.Namespace {
			continue
		}
		if filter.Type != "" && w.Type != filter.Type {
			continue
		}
		if filter.Status != "" && w.Status != filter.Status {
			continue
		}
		// Shallow copy so callers can't mutate the cached fixture.
		wc := *w
		out = append(out, &wc)
	}
	return out, nil
}

// GetWorkloadDetail returns the full WorkloadDetail for the namespace+name
// pair, or ErrWorkloadNotFound if no entry matches.
func (s *Source) GetWorkloadDetail(ctx context.Context, namespace, name string) (*model.WorkloadDetail, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	all, err := s.loadWorkloads()
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].Namespace == namespace && all[i].Name == name {
			wd := all[i] // copy
			return &wd, nil
		}
	}
	return nil, ErrWorkloadNotFound
}
