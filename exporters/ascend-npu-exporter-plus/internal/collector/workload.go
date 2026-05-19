package collector

import (
	"context"
	"log"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/collector/sources"
	"github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/registry"
)

// WorkloadCollector emits per-{namespace,pod,container} PID-bound NPU
// metrics by polling a CgroupSource on each scrape (T102).
//
// Two metric families:
//
//	ascend_workload_npu_seconds_total{namespace,pod,container,npu_id}
//	    Cumulative wall-clock seconds during which a container had at
//	    least one slice running on the labeled NPU. Counter.
//	ascend_workload_active_slices{namespace,pod,container}
//	    Number of slices a container currently has bound to any NPU.
//	    Gauge.
//
// Phase 3 emits 1.0 per scrape for npu_seconds_total (simulator path);
// Phase 5+ wires the real-/proc reader once inference-operator consumes
// these series for PD Router routing decisions (ADR-0008). The
// `--enable-workload-correlation` flag (default OFF) is registered by
// cmd/exporter-plus/main.go in the main-agent collation commit, not in
// this task.
type WorkloadCollector struct {
	source *sources.CgroupSource

	npuSecondsTotal *prometheus.Desc
	activeSlices    *prometheus.Desc
}

// NewWorkloadCollector constructs a WorkloadCollector backed by the
// given CgroupSource.
func NewWorkloadCollector(source *sources.CgroupSource) *WorkloadCollector {
	return &WorkloadCollector{
		source: source,
		npuSecondsTotal: prometheus.NewDesc(
			"ascend_workload_npu_seconds_total",
			"Cumulative wall-clock seconds during which a container had at least one slice running on the labeled NPU.",
			[]string{"namespace", "pod", "container", "npu_id"},
			nil,
		),
		activeSlices: prometheus.NewDesc(
			"ascend_workload_active_slices",
			"Number of slices a container currently has bound to any NPU.",
			[]string{"namespace", "pod", "container"},
			nil,
		),
	}
}

// Describe implements prometheus.Collector.
func (c *WorkloadCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.npuSecondsTotal
	ch <- c.activeSlices
}

// Collect implements prometheus.Collector. Records scrape duration into
// registry.CollectDuration under the "workload" collector label.
func (c *WorkloadCollector) Collect(ch chan<- prometheus.Metric) {
	start := time.Now()
	defer func() {
		if registry.CollectDuration != nil {
			registry.CollectDuration.WithLabelValues("workload").Observe(time.Since(start).Seconds())
		}
	}()

	samples, err := c.source.ReadWorkloads(context.Background())
	if err != nil {
		log.Printf("workload collector: source.ReadWorkloads failed: %v", err)
		return
	}
	for _, s := range samples {
		ch <- prometheus.MustNewConstMetric(c.npuSecondsTotal, prometheus.CounterValue,
			s.NPUSecondsDelta, s.Namespace, s.Pod, s.Container, s.NPUID)
		ch <- prometheus.MustNewConstMetric(c.activeSlices, prometheus.GaugeValue,
			float64(s.ActiveSlices), s.Namespace, s.Pod, s.Container)
	}
}
