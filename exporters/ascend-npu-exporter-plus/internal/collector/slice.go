// Package collector — slice.go (P3-T-101)
//
// SliceCollector turns a sources.SliceSource into per-slice Prometheus
// gauges. It is wired alongside NPUCollector by main.go; this file
// owns only the Collector implementation. Phase 4+ will swap the
// simulator-backed SliceSource for DCMI / npu-smi without touching
// this collector.
package collector

import (
	"context"
	"log"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/collector/sources"
	"github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/registry"
)

// SliceCollector emits per-slice Prometheus metrics by polling a
// sources.SliceSource on each Collect() call (scrape-time).
//
// Metric families:
//
//	ascend_slice_aicore_count{slice_id,npu_id,node,template}
//	ascend_slice_memory_used_bytes{slice_id,npu_id,node}
//	ascend_slice_allocated_to_pod{slice_id,npu_id,namespace,pod}
//	  -- emitted ONLY for allocated slices; free slices omit the
//	  series so count() / sum() yield true allocation rate.
type SliceCollector struct {
	source sources.SliceSource

	aicoreCount       *prometheus.Desc
	memoryUsedBytes   *prometheus.Desc
	allocatedToPod    *prometheus.Desc
	aicoreUtilization *prometheus.Desc
}

// NewSliceCollector constructs a SliceCollector backed by the given
// SliceSource.
func NewSliceCollector(source sources.SliceSource) *SliceCollector {
	return &SliceCollector{
		source: source,
		aicoreCount: prometheus.NewDesc(
			"ascend_slice_aicore_count",
			"AI Core allocation for one NPU slice instance.",
			[]string{"slice_id", "npu_id", "node", "template"},
			nil,
		),
		memoryUsedBytes: prometheus.NewDesc(
			"ascend_slice_memory_used_bytes",
			"HBM memory currently used by one NPU slice instance, in bytes.",
			[]string{"slice_id", "npu_id", "node"},
			nil,
		),
		allocatedToPod: prometheus.NewDesc(
			"ascend_slice_allocated_to_pod",
			"1 when the slice is bound to a Pod. Missing series for free slices so count() gives allocation rate.",
			[]string{"slice_id", "npu_id", "namespace", "pod"},
			nil,
		),
		// P11-fix-002: per-slice utilization. Emitted only for allocated
		// slices so workload-resource dashboard `{workload=~"$workload"}`
		// resolves; `workload` label carries the bound Pod name.
		aicoreUtilization: prometheus.NewDesc(
			"ascend_npu_slice_util_percent",
			"AI Core utilization of one NPU slice instance, as a percentage in [0, 100].",
			[]string{"slice_id", "npu_id", "node", "template", "workload"},
			nil,
		),
	}
}

// Describe implements prometheus.Collector.
func (c *SliceCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.aicoreCount
	ch <- c.memoryUsedBytes
	ch <- c.allocatedToPod
	ch <- c.aicoreUtilization
}

// Collect implements prometheus.Collector. Records scrape duration into
// registry.CollectDuration under the "slice" collector label.
func (c *SliceCollector) Collect(ch chan<- prometheus.Metric) {
	start := time.Now()
	defer func() {
		if registry.CollectDuration != nil {
			registry.CollectDuration.WithLabelValues("slice").Observe(time.Since(start).Seconds())
		}
	}()

	samples, err := c.source.ReadSlices(context.Background())
	if err != nil {
		log.Printf("slice collector: source.ReadSlices failed: %v", err)
		return
	}
	for _, s := range samples {
		ch <- prometheus.MustNewConstMetric(c.aicoreCount, prometheus.GaugeValue,
			float64(s.AICoreCount), s.ID, s.NPUID, s.NodeName, s.Template)
		ch <- prometheus.MustNewConstMetric(c.memoryUsedBytes, prometheus.GaugeValue,
			float64(s.MemoryUsedBytes), s.ID, s.NPUID, s.NodeName)
		if s.AllocatedTo != nil {
			ch <- prometheus.MustNewConstMetric(c.allocatedToPod, prometheus.GaugeValue,
				1.0, s.ID, s.NPUID, s.AllocatedTo.Namespace, s.AllocatedTo.Pod)
			ch <- prometheus.MustNewConstMetric(c.aicoreUtilization, prometheus.GaugeValue,
				s.AICoreUtilization, s.ID, s.NPUID, s.NodeName, s.Template, s.AllocatedTo.Pod)
		}
	}
}
