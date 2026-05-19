// Package fabric implements Phase 2 ADR-0007 static-config fabric
// discovery. Reads a YAML file describing switches + links and
// surfaces them as aggregator.NetworkSwitch / NetworkLink so the
// topology builder can render them under `?includeFabric=true`.
//
// Source is NOT a datasource.Source — fabric data feeds the
// topology aggregator directly (not via REST endpoint mapping).
// main.go instantiates *fabric.Source separately and passes it to
// the aggregator constructor (Phase 3+ aggregator-wiring task).
//
// Hot reload is out of scope for Phase 2: edit fabric.yaml +
// restart backend. Phase 3 may add fsnotify if measurement shows
// the restart cost is real.
package fabric

import (
	"context"
	"errors"
	"fmt"
	"os"

	"sigs.k8s.io/yaml"

	"github.com/example/ocloud-edge/backend/pkg/aggregator"
)

// Options configures the static fabric reader.
type Options struct {
	// Path is the on-disk YAML containing the fabric description.
	// Empty path is allowed — the Source then returns empty
	// switches / links from every call (useful for tests / for
	// installations without a fabric definition).
	Path string
}

// Source reads switches + links from a static YAML file once at
// construction time. Concurrent ListSwitches / ListLinks calls
// share the parsed in-memory state — goroutine-safe by read-only
// access.
type Source struct {
	switches []aggregator.NetworkSwitch
	links    []aggregator.NetworkLink
}

// fabricFile mirrors the YAML schema. Both arrays may be empty;
// extra fields are tolerated (forward-compat with schema additions).
type fabricFile struct {
	Switches []aggregator.NetworkSwitch `json:"switches"`
	Links    []aggregator.NetworkLink   `json:"links"`
}

// ErrParse wraps any YAML decode / file-read failure surfaced by
// NewSource. Callers (main.go) treat any error from NewSource as
// fatal so a misconfigured fabric doesn't silently disable the
// fabric layer.
var ErrParse = errors.New("fabric: parse failure")

// NewSource reads + parses the fabric YAML. Empty path → empty
// fabric (no error). Missing file → error (operator typo) so the
// startup log surfaces the cause; nil-path is the supported
// "no fabric" state.
func NewSource(opts Options) (*Source, error) {
	if opts.Path == "" {
		return &Source{}, nil
	}
	raw, err := os.ReadFile(opts.Path) //nolint:gosec // path comes from server config
	if err != nil {
		return nil, fmt.Errorf("%w: read %q: %v", ErrParse, opts.Path, err)
	}
	var doc fabricFile
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%w: yaml %q: %v", ErrParse, opts.Path, err)
	}
	return &Source{
		switches: doc.Switches,
		links:    doc.Links,
	}, nil
}

// ListSwitches returns a defensive copy of the loaded switches.
// Aggregator MAY mutate the returned slice (re-order, filter) so
// we don't share the backing array.
func (s *Source) ListSwitches(_ context.Context) ([]aggregator.NetworkSwitch, error) {
	if s == nil {
		return nil, nil
	}
	out := make([]aggregator.NetworkSwitch, len(s.switches))
	copy(out, s.switches)
	return out, nil
}

// ListLinks returns a defensive copy of the loaded links. Same
// rationale as ListSwitches.
func (s *Source) ListLinks(_ context.Context) ([]aggregator.NetworkLink, error) {
	if s == nil {
		return nil, nil
	}
	out := make([]aggregator.NetworkLink, len(s.links))
	copy(out, s.links)
	return out, nil
}
