// Package collector hosts prometheus.Collector implementations.
//
// Phase 3 (P3-T-007) lands the NPU-level collector that turns a
// sources.Source into per-device Prometheus metrics. Slice-level
// (T101) and PID-level (T102) collectors land on the same Source
// interface.
package collector

import (
	"context"
	"log"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/collector/sources"
	"github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/registry"
)

// NPUCollector emits per-NPU Prometheus metrics by polling a sources.Source
// on each Collect() call (scrape-time).
type NPUCollector struct {
	source sources.Source

	utilization     *prometheus.Desc
	memoryUsed      *prometheus.Desc
	hbmBandwidth    *prometheus.Desc
	temperature     *prometheus.Desc
	powerWatts      *prometheus.Desc
	memoryTotal     *prometheus.Desc
	vramUsedPercent *prometheus.Desc
}

// NewNPUCollector constructs an NPUCollector backed by the given Source.
func NewNPUCollector(source sources.Source) *NPUCollector {
	return &NPUCollector{
		source: source,
		utilization: prometheus.NewDesc(
			"ascend_npu_utilization_percent",
			"AI Core utilization of an Ascend NPU device, as a percentage in [0, 100].",
			[]string{"npu_id", "node", "model"},
			nil,
		),
		memoryUsed: prometheus.NewDesc(
			"ascend_npu_memory_used_bytes",
			"HBM memory currently occupied on an Ascend NPU device, in bytes.",
			[]string{"npu_id", "node"},
			nil,
		),
		hbmBandwidth: prometheus.NewDesc(
			"ascend_npu_hbm_bandwidth_bytes_per_second",
			"Realised HBM bandwidth on an Ascend NPU device, in bytes/second.",
			[]string{"npu_id", "node"},
			nil,
		),
		// P11-fix-002: 4 new descriptors backing npu-detail / node-detail
		// dashboard panels that previously rendered No-data.
		temperature: prometheus.NewDesc(
			"ascend_npu_temperature_celsius",
			"On-die temperature of an Ascend NPU device, in degrees Celsius.",
			[]string{"npu_id", "node"},
			nil,
		),
		powerWatts: prometheus.NewDesc(
			"ascend_npu_power_watts",
			"Instantaneous power draw of an Ascend NPU device, in watts.",
			[]string{"npu_id", "node"},
			nil,
		),
		memoryTotal: prometheus.NewDesc(
			"ascend_npu_vram_total_bytes",
			"HBM memory capacity of an Ascend NPU device, in bytes (fixed per model).",
			[]string{"npu_id", "node", "model"},
			nil,
		),
		vramUsedPercent: prometheus.NewDesc(
			"ascend_npu_vram_used_percent",
			"HBM memory occupancy as a percentage in [0, 100]; derived from memory_used/memory_total.",
			[]string{"npu_id", "node"},
			nil,
		),
	}
}

// Describe implements prometheus.Collector.
func (c *NPUCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.utilization
	ch <- c.memoryUsed
	ch <- c.hbmBandwidth
	ch <- c.temperature
	ch <- c.powerWatts
	ch <- c.memoryTotal
	ch <- c.vramUsedPercent
}

// Collect implements prometheus.Collector. Records scrape duration into
// registry.CollectDuration under the "npu" collector label.
func (c *NPUCollector) Collect(ch chan<- prometheus.Metric) {
	start := time.Now()
	defer func() {
		if registry.CollectDuration != nil {
			registry.CollectDuration.WithLabelValues("npu").Observe(time.Since(start).Seconds())
		}
	}()

	samples, err := c.source.ReadNPUs(context.Background())
	if err != nil {
		log.Printf("NPU collector: source.ReadNPUs failed: %v", err)
		return
	}
	for _, s := range samples {
		ch <- prometheus.MustNewConstMetric(c.utilization, prometheus.GaugeValue, s.AICore,
			s.ID, s.NodeName, s.Model)
		ch <- prometheus.MustNewConstMetric(c.memoryUsed, prometheus.GaugeValue, float64(s.MemoryUsedBytes),
			s.ID, s.NodeName)
		ch <- prometheus.MustNewConstMetric(c.hbmBandwidth, prometheus.GaugeValue, float64(s.HBMBandwidthBytesPerSecond),
			s.ID, s.NodeName)
		ch <- prometheus.MustNewConstMetric(c.temperature, prometheus.GaugeValue, s.TemperatureCelsius,
			s.ID, s.NodeName)
		ch <- prometheus.MustNewConstMetric(c.powerWatts, prometheus.GaugeValue, s.PowerWatts,
			s.ID, s.NodeName)
		ch <- prometheus.MustNewConstMetric(c.memoryTotal, prometheus.GaugeValue, float64(s.MemoryTotalBytes),
			s.ID, s.NodeName, s.Model)
		if s.MemoryTotalBytes > 0 {
			pct := 100.0 * float64(s.MemoryUsedBytes) / float64(s.MemoryTotalBytes)
			ch <- prometheus.MustNewConstMetric(c.vramUsedPercent, prometheus.GaugeValue, pct,
				s.ID, s.NodeName)
		}
	}
}
