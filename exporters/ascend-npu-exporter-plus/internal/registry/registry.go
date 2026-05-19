// Package registry constructs the Prometheus Registry shared by all collectors.
//
// Phase 3 (P3-T-006) registers only the meta metrics:
//   - exporter_build_info{version,commit,go_version} (value=1)
//   - exporter_collect_duration_seconds{collector} (histogram)
//
// Phase 3+ collectors (P3-T-007/T101/T102) call Registry.Register(...) to add
// their families. Single-process assumption: this exporter does not expect
// multiple Registries in the same process; CollectDuration is therefore a
// package-level variable assigned by New.
package registry

import (
	"github.com/prometheus/client_golang/prometheus"
)

// CollectDuration is the histogram observed by each collector before/after its
// Collect call. Exposed so collector packages can record observations without
// having to plumb the Registry through their constructors. Assigned by New.
var CollectDuration *prometheus.HistogramVec

// New returns a *prometheus.Registry with exporter_build_info and
// exporter_collect_duration_seconds pre-registered.
//
// Callers (cmd/exporter-plus/main.go) typically pass:
//   - version: internal/version.Version
//   - commit:  internal/version.Commit
//   - goVersion: runtime.Version()
func New(version, commit, goVersion string) *prometheus.Registry {
	reg := prometheus.NewRegistry()

	buildInfo := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "exporter_build_info",
			Help: "Build metadata for ascend-npu-exporter-plus. Value is always 1.",
		},
		[]string{"version", "commit", "go_version"},
	)
	buildInfo.WithLabelValues(version, commit, goVersion).Set(1)
	reg.MustRegister(buildInfo)

	CollectDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "exporter_collect_duration_seconds",
			Help:    "Duration of a single collector Collect call.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"collector"},
	)
	reg.MustRegister(CollectDuration)

	return reg
}
