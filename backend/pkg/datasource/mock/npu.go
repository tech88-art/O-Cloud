// Package mock — NPU fixtures (P1-T-104).
//
// Loads NPUs once (lazy) from <fixturesPath>/npus.json and joins slices from
// <fixturesPath>/slices.json on parentNPU == npu.id. The returned NPUs already
// carry their slice status so the handler can return them directly per the
// /api/v1/nodes/{nodeName}/npus contract (see docs/api-contract.yaml).
//
// fixturesPath empty → loader treats it as "no fixtures" and returns an empty
// slice (matches cluster.go behavior — keeps tests cheap when they construct
// a Source without any disk dependency).
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

// ErrNPUNodeNotFound is returned by ListNPUs when no NPU in the fixture
// belongs to the requested node name. Distinguishes 404 (unknown node) from
// 200 with empty list (known node but happens to have no NPUs). Handler maps
// it to 404 with the Error envelope.
var ErrNPUNodeNotFound = errors.New("mock source: node has no NPUs (or unknown node)")

// npusFile / slicesFile are the JSON fixture names under fixturesPath.
const (
	npusFile   = "npus.json"
	slicesFile = "slices.json"
)

// npusFixture is the on-disk shape of npus.json. The set-a-small dataset is a
// composite document keyed by resource — we only read `npus` here. Sibling
// `slices` at the top level is empty in the set-a-small layout; the real
// slices live in slices.json and are joined separately.
type npusFixture struct {
	NPUs []*model.NPU `json:"npus"`
}

// slicesFixture is the on-disk shape of slices.json. We only need the `slices`
// array; other sibling keys (clusters/nodes/...) are ignored.
type slicesFixture struct {
	Slices []model.NPUSlice `json:"slices"`
}

// loadNPUs reads npus.json + slices.json from fixturesPath at most once per
// Source. The joined result is cached; errors are sticky.
//
// Both files are required when fixturesPath is non-empty. Slices missing or
// unparseable is a hard error because the AC for P1-T-104 makes slice status
// part of the NPU contract — a silent fallback would hide drift.
//
// fixturesPath empty → no-op (returns nil slice, nil error). Keeps the
// erroringNPUSource test (mocksrc.NewSource("")) cheap.
func (s *Source) loadNPUs() ([]*model.NPU, error) {
	s.npusOnce.Do(func() {
		if s.fixturesPath == "" {
			s.npus = nil
			return
		}

		// 1) Load NPUs.
		npuPath := filepath.Join(s.fixturesPath, npusFile)
		npuRaw, err := os.ReadFile(npuPath) //nolint:gosec // path comes from server config, not user input
		if err != nil {
			s.npusErr = fmt.Errorf("read npus fixture %q: %w", npuPath, err)
			return
		}
		var npuDoc npusFixture
		if err := json.Unmarshal(npuRaw, &npuDoc); err != nil {
			s.npusErr = fmt.Errorf("parse npus fixture %q: %w", npuPath, err)
			return
		}

		// 2) Load slices. Missing file is a hard error (see godoc above).
		slicePath := filepath.Join(s.fixturesPath, slicesFile)
		sliceRaw, err := os.ReadFile(slicePath) //nolint:gosec // path comes from server config, not user input
		if err != nil {
			s.npusErr = fmt.Errorf("read slices fixture %q: %w", slicePath, err)
			return
		}
		var sliceDoc slicesFixture
		if err := json.Unmarshal(sliceRaw, &sliceDoc); err != nil {
			s.npusErr = fmt.Errorf("parse slices fixture %q: %w", slicePath, err)
			return
		}

		// 3) Index slices by parentNPU id, then attach to NPUs that own any.
		// NPUs in whole mode keep nil slices — model.NPU.Slices uses omitempty
		// per the OpenAPI optional schema, so "slices" is just absent in that
		// case. sliceMode (always serialized) carries the "no slices here"
		// signal the AC asks for.
		byParent := make(map[string][]model.NPUSlice, len(sliceDoc.Slices))
		for _, sl := range sliceDoc.Slices {
			byParent[sl.ParentNPU] = append(byParent[sl.ParentNPU], sl)
		}
		for _, npu := range npuDoc.NPUs {
			if npu == nil {
				continue
			}
			if attached, ok := byParent[npu.ID]; ok {
				npu.Slices = attached
			}
		}
		s.npus = npuDoc.NPUs
	})
	return s.npus, s.npusErr
}

// ListNPUs returns every NPU in the fixture whose nodeName equals the supplied
// nodeName. Each NPU includes its slices array (joined from slices.json).
//
// When the fixture has zero NPUs for the requested node, returns
// ErrNPUNodeNotFound so the handler can render 404. (The contract treats
// "unknown node" as 404; we don't distinguish "known node with zero NPUs"
// from "unknown node" in the mock because set-a-small worker nodes always
// have 8 NPUs each and the control-plane node has zero NPUs — the latter is
// effectively "no NPUs to list", same wire shape as 404.)
func (s *Source) ListNPUs(ctx context.Context, nodeName string) ([]*model.NPU, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	all, err := s.loadNPUs()
	if err != nil {
		return nil, err
	}
	out := make([]*model.NPU, 0, len(all))
	for _, npu := range all {
		if npu != nil && npu.NodeName == nodeName {
			// Shallow copy so the caller can't mutate the cached fixture.
			n := *npu
			out = append(out, &n)
		}
	}
	if len(out) == 0 {
		return nil, ErrNPUNodeNotFound
	}
	return out, nil
}
