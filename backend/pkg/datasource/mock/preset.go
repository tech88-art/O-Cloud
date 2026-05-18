// Package mock — Preset fixtures (P1-T-203).
//
// Loads presets once (lazy) from <fixturesPath>/presets.json. The composite
// set-a-small document keys this as `presets` at the top level; we only read
// that array. Each fixture entry carries the full superset (Preset fields +
// manifest + dependencies), so a single load covers both ListPresets (returns
// the slim Preset view) and GetPreset (returns the PresetDetail superset).
//
// fixturesPath empty → loader treats it as "no fixtures" and returns an empty
// slice (same convention as cluster.go / npu.go — keeps unit tests that
// construct a Source without disk dependencies cheap).
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

// ErrPresetNotFound is returned by GetPreset when no preset matches the id.
// Wrapped so the handler can errors.Is against it and emit 404.
var ErrPresetNotFound = errors.New("preset not found")

// presetsFile is the JSON fixture name under fixturesPath.
const presetsFile = "presets.json"

// presetsFixture is the on-disk shape: a composite document where we only
// read the top-level `presets` array. Other keys are ignored.
type presetsFixture struct {
	Presets []*model.PresetDetail `json:"presets"`
}

// loadPresets reads presetsFile from fixturesPath at most once per Source.
// Errors are sticky — a bad fixture file is not re-parsed on every request.
//
// fixturesPath empty → no-op (returns nil slice, nil error). Lets tests
// inject an erroring Source by embedding mocksrc.NewSource("").
func (s *Source) loadPresets() ([]*model.PresetDetail, error) {
	s.presetsOnce.Do(func() {
		if s.fixturesPath == "" {
			s.presets = nil
			return
		}
		path := filepath.Join(s.fixturesPath, presetsFile)
		raw, err := os.ReadFile(path) //nolint:gosec // path comes from server config, not user input
		if err != nil {
			s.presetsErr = fmt.Errorf("read presets fixture %q: %w", path, err)
			return
		}
		var doc presetsFixture
		if err := json.Unmarshal(raw, &doc); err != nil {
			s.presetsErr = fmt.Errorf("parse presets fixture %q: %w", path, err)
			return
		}
		s.presets = doc.Presets
	})
	return s.presets, s.presetsErr
}

// ListPresets returns the slim Preset view (without manifest / parameters) for
// every preset in the fixture set. Empty fixture → empty slice, never nil
// (handler relies on json encoding a non-nil slice as `[]`).
func (s *Source) ListPresets(ctx context.Context) ([]*model.Preset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	all, err := s.loadPresets()
	if err != nil {
		return nil, err
	}
	out := make([]*model.Preset, 0, len(all))
	for _, p := range all {
		if p == nil {
			continue
		}
		// Copy the embedded Preset; manifest / parameters stay on the detail
		// view only. Shallow copy is safe because Preset fields are values or
		// already-copied pointers (Requirements points at fixture-owned data
		// that callers should not mutate; same convention as cluster.go).
		preset := p.Preset
		out = append(out, &preset)
	}
	return out, nil
}

// GetPreset returns the full PresetDetail matching id, or ErrPresetNotFound.
func (s *Source) GetPreset(ctx context.Context, id string) (*model.PresetDetail, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	all, err := s.loadPresets()
	if err != nil {
		return nil, err
	}
	for _, p := range all {
		if p != nil && p.ID == id {
			return p, nil
		}
	}
	return nil, ErrPresetNotFound
}
