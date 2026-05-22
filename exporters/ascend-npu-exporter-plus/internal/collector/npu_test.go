package collector

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/collector/sources"
	"github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/registry"
)

// init seeds the package-level registry.CollectDuration so the NPU
// collector's timer has a valid HistogramVec in unit tests. Production
// flow assigns this in registry.New(); tests bypass main.go so we wire
// it directly. Idempotent across multiple test entry points.
func init() {
	if registry.CollectDuration == nil {
		registry.CollectDuration = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "exporter_collect_duration_seconds",
				Help:    "test stub",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"collector"},
		)
	}
}

// fakeSource implements sources.Source for unit tests.
type fakeSource struct {
	samples []sources.NPUSample
	err     error
}

func (f *fakeSource) ReadNPUs(_ context.Context) ([]sources.NPUSample, error) {
	return f.samples, f.err
}

func TestNPUCollector_HappyPath(t *testing.T) {
	src := &fakeSource{samples: []sources.NPUSample{
		{ID: "n-0", NodeName: "node-a", Model: "Ascend910B", AICore: 42.0,
			MemoryUsedBytes: 12884901888, MemoryTotalBytes: 68719476736,
			HBMBandwidthBytesPerSecond: 400000000000, Healthy: true},
		{ID: "n-1", NodeName: "node-a", Model: "Ascend910B", AICore: 75.0,
			MemoryUsedBytes: 30000000000, MemoryTotalBytes: 68719476736,
			HBMBandwidthBytesPerSecond: 550000000000, Healthy: true},
		{ID: "n-2", NodeName: "node-b", Model: "Ascend910B", AICore: 10.0,
			MemoryUsedBytes: 5000000000, MemoryTotalBytes: 68719476736,
			HBMBandwidthBytesPerSecond: 100000000000, Healthy: false},
	}}

	c := NewNPUCollector(src)
	reg := prometheus.NewPedanticRegistry()
	require.NoError(t, reg.Register(c))

	// One sample per Desc per NPU -> 3 samples across each of the 3 families.
	assert.Equal(t, 3, testutil.CollectAndCount(c, "ascend_npu_utilization_percent"))
	assert.Equal(t, 3, testutil.CollectAndCount(c, "ascend_npu_memory_used_bytes"))
	assert.Equal(t, 3, testutil.CollectAndCount(c, "ascend_npu_hbm_bandwidth_bytes_per_second"))

	// Spot-check the utilization labels + values reach the right gauges.
	expected := `
# HELP ascend_npu_utilization_percent AI Core utilization of an Ascend NPU device, as a percentage in [0, 100].
# TYPE ascend_npu_utilization_percent gauge
ascend_npu_utilization_percent{model="Ascend910B",node="node-a",npu_id="n-0"} 42
ascend_npu_utilization_percent{model="Ascend910B",node="node-a",npu_id="n-1"} 75
ascend_npu_utilization_percent{model="Ascend910B",node="node-b",npu_id="n-2"} 10
`
	require.NoError(t, testutil.CollectAndCompare(c, strings.NewReader(expected), "ascend_npu_utilization_percent"))
}

func TestNPUCollector_SourceError(t *testing.T) {
	src := &fakeSource{err: errors.New("boom")}
	c := NewNPUCollector(src)

	// Source error: collector emits zero samples (and logs). Count must be 0
	// across all three metric families.
	assert.Equal(t, 0, testutil.CollectAndCount(c, "ascend_npu_utilization_percent"))
	assert.Equal(t, 0, testutil.CollectAndCount(c, "ascend_npu_memory_used_bytes"))
	assert.Equal(t, 0, testutil.CollectAndCount(c, "ascend_npu_hbm_bandwidth_bytes_per_second"))
}

func TestNPUCollector_Describe(t *testing.T) {
	src := &fakeSource{}
	c := NewNPUCollector(src)
	ch := make(chan *prometheus.Desc, 8)
	c.Describe(ch)
	close(ch)
	count := 0
	for range ch {
		count++
	}
	// Describe must emit exactly 7 Desc: utilization, memory_used,
	// hbm_bandwidth (P3-T-007) + temperature, power_watts, memory_total,
	// vram_used_percent (P11-fix-002).
	assert.Equal(t, 7, count)
}

func TestNPUCollector_EmptySamples(t *testing.T) {
	src := &fakeSource{samples: []sources.NPUSample{}}
	c := NewNPUCollector(src)
	assert.Equal(t, 0, testutil.CollectAndCount(c, "ascend_npu_utilization_percent"))
}
