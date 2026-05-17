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

// Build constructs a Registry from a config.
//
// PHASE-1: this is a stub. T101+ will register concrete implementations:
//   - mock  → datasource/mock.NewSource
//   - k8s   → datasource/k8s.NewSource         (PHASE-2)
//   - crd   → datasource/crd.NewSource         (PHASE-2)
//   - prom  → datasource/prometheus.NewSource  (PHASE-2)
//   - cm    → datasource/configmap.NewSource   (PHASE-2)
//
// For now Build returns an empty Registry when no datasource is enabled (so
// healthz can still report `status: ok` with an empty datasources map).
//
// TODO(@P1-T-101): wire mock source here.
func Build(cfg *config.Config) (*Registry, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil config")
	}
	reg := &Registry{
		Sources: map[string]Source{},
		Mapping: map[string]string{},
	}
	// Mapping is copied through verbatim so handlers can already query it.
	for resource, sourceName := range cfg.Mapping {
		reg.Mapping[resource] = sourceName
	}
	// PHASE-1: no actual sources constructed — concrete wiring follows in T101+.
	return reg, nil
}
