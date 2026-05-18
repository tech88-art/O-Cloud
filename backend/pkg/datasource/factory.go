package datasource

import (
	"fmt"

	"github.com/example/ocloud-edge/backend/pkg/config"
)

// Registry holds the Source instances built from config, plus the mapping of
// resource → source-name. handlers read via sourceFor(resourceKey).
//
// Phase 1: only the "mock" entry exists. Phase 2 adds k8s/prometheus/crd/configmap.
type Registry struct {
	Sources map[string]Source
	Mapping map[string]string // resource → source name
}

// SourceFor returns the source assigned to a logical resource (e.g. "clusters",
// "metrics", "presets"). Returns nil if either the mapping or the source is
// missing — handlers must defend against nil to keep degraded mode working.
func (r *Registry) SourceFor(resource string) Source {
	if r == nil {
		return nil
	}
	srcName, ok := r.Mapping[resource]
	if !ok {
		return nil
	}
	return r.Sources[srcName]
}

// Build constructs a Registry from a config + a pre-built map of sources
// (keyed by datasource name, e.g. "mock", "mock-a", "mock-b"). Source
// construction lives in main (or test code) to avoid an import cycle: each
// concrete source package (datasource/mock, datasource/k8s, ...) imports
// datasource for the Source interface, so datasource cannot import them back.
//
// PHASE-1: only the "mock" family is wired (P1-T-005 + P1-T-101/103
// follow-up). Multiple mock instances (mock-a / mock-b / ...) are supported
// per the P1-T-303 dataset-swap contract — main.go uses each datasources
// entry's `path` field to point the corresponding mock.Source at its own
// fixture directory. PHASE-2: k8s / crd / prometheus / configmap join.
//
// Build only registers sources whose corresponding datasources entry has
// enabled=true (silently dropping the others). Mapping is copied verbatim;
// the SourceFor resolver returns nil for any mapping entry whose target
// source isn't in the registered set (handlers must defend against nil).
//
// "数据源切换只改配置" (P1-T-303): swapping the active dataset is a yaml-
// only edit — change `datasources.mock.path` or flip `mapping.<resource>`
// between `mock-a` / `mock-b`, restart, no recompile. See factory_test.go
// for the pinned invariant.
func Build(cfg *config.Config, sources map[string]Source) (*Registry, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil config")
	}
	reg := &Registry{
		Sources: map[string]Source{},
		Mapping: map[string]string{},
	}
	for resource, sourceName := range cfg.Mapping {
		reg.Mapping[resource] = sourceName
	}
	for name, src := range sources {
		entry, ok := cfg.Datasources[name]
		if !ok || !entry.Enabled || src == nil {
			continue
		}
		reg.Sources[name] = src
	}
	return reg, nil
}
