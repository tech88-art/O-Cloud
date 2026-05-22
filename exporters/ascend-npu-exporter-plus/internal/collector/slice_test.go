package collector

import (
	"context"
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/collector/sources"
)

// fakeSliceSource implements sources.SliceSource for unit tests.
// registry.CollectDuration is wired by the init() in npu_test.go
// (same package), so the timer dispatch here is safe.
type fakeSliceSource struct {
	samples []sources.SliceSample
	err     error
}

func (f *fakeSliceSource) ReadSlices(_ context.Context) ([]sources.SliceSample, error) {
	return f.samples, f.err
}

// allocPod is a tiny helper for fixture construction.
func allocPod(ns, pod string) *sources.AllocatedPod {
	return &sources.AllocatedPod{Namespace: ns, Pod: pod}
}

// gatherFamily runs Collect on the collector and returns the first
// MetricFamily matching name (or nil).
func gatherFamily(t *testing.T, col prometheus.Collector, name string) *dto.MetricFamily {
	t.Helper()
	reg := prometheus.NewPedanticRegistry()
	require.NoError(t, reg.Register(col))
	mfs, err := reg.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() == name {
			return mf
		}
	}
	return nil
}

// TestSliceCollector_HappyPath_12SlicesMixed: 4 Allocated + 8 Idle ->
// 12 aicore + 12 memory + 4 allocated_to_pod series.
func TestSliceCollector_HappyPath_12SlicesMixed(t *testing.T) {
	samples := []sources.SliceSample{
		// 4 Allocated (PD prefill/decode mix).
		{ID: "n0-vir04-0", NPUID: "n0", NodeName: "w-01", Template: "vir04", AICoreCount: 4,
			MemoryUsedBytes: 17179869184, AllocatedTo: allocPod("ocloud-system", "qwen-8b-prefill-0")},
		{ID: "n0-vir04-1", NPUID: "n0", NodeName: "w-01", Template: "vir04", AICoreCount: 4,
			MemoryUsedBytes: 17179869184, AllocatedTo: allocPod("ocloud-system", "qwen-8b-decode-0")},
		{ID: "n2-vir08-0", NPUID: "n2", NodeName: "w-02", Template: "vir08", AICoreCount: 8,
			MemoryUsedBytes: 34359738368, AllocatedTo: allocPod("ocloud-system", "qwen-8b-prefill-1")},
		{ID: "n3-vir08-0", NPUID: "n3", NodeName: "w-02", Template: "vir08", AICoreCount: 8,
			MemoryUsedBytes: 34359738368, AllocatedTo: allocPod("ocloud-system", "qwen-8b-decode-1")},
		// 8 Idle (free).
		{ID: "n1-vir04-0", NPUID: "n1", NodeName: "w-01", Template: "vir04", AICoreCount: 4},
		{ID: "n1-vir04-1", NPUID: "n1", NodeName: "w-01", Template: "vir04", AICoreCount: 4},
		{ID: "n2-vir08-1", NPUID: "n2", NodeName: "w-02", Template: "vir08", AICoreCount: 8},
		{ID: "n3-vir08-1", NPUID: "n3", NodeName: "w-02", Template: "vir08", AICoreCount: 8},
		{ID: "n4-vir04-0", NPUID: "n4", NodeName: "w-03", Template: "vir04", AICoreCount: 4},
		{ID: "n4-vir04-1", NPUID: "n4", NodeName: "w-03", Template: "vir04", AICoreCount: 4},
		{ID: "n4-vir04-2", NPUID: "n4", NodeName: "w-03", Template: "vir04", AICoreCount: 4},
		{ID: "n4-vir04-3", NPUID: "n4", NodeName: "w-03", Template: "vir04", AICoreCount: 4},
	}
	src := &fakeSliceSource{samples: samples}
	col := NewSliceCollector(src)

	assert.Equal(t, 12, testutil.CollectAndCount(col, "ascend_slice_aicore_count"))
	assert.Equal(t, 12, testutil.CollectAndCount(col, "ascend_slice_memory_used_bytes"))
	// Free slices intentionally omit the allocated_to_pod series so
	// count(ascend_slice_allocated_to_pod) gives allocation rate.
	assert.Equal(t, 4, testutil.CollectAndCount(col, "ascend_slice_allocated_to_pod"))
}

// TestSliceCollector_SourceError: source error -> zero samples emitted.
func TestSliceCollector_SourceError(t *testing.T) {
	src := &fakeSliceSource{err: errors.New("simulated source failure")}
	col := NewSliceCollector(src)

	assert.Equal(t, 0, testutil.CollectAndCount(col, "ascend_slice_aicore_count"))
	assert.Equal(t, 0, testutil.CollectAndCount(col, "ascend_slice_memory_used_bytes"))
	assert.Equal(t, 0, testutil.CollectAndCount(col, "ascend_slice_allocated_to_pod"))
}

// TestSliceCollector_DynamicSlice_EmptyTemplateLabel: a Dynamic-strategy
// slice (Template=="") still produces aicore_count + memory_used_bytes
// series, with the template label literally "". This guarantees
// PromQL queries that group by template see Dynamic slices as a
// distinct bucket and not silently dropped.
func TestSliceCollector_DynamicSlice_EmptyTemplateLabel(t *testing.T) {
	samples := []sources.SliceSample{
		{ID: "dyn-0", NPUID: "n0", NodeName: "w-01", Template: "", AICoreCount: 6,
			MemoryUsedBytes: 25769803776,
			AllocatedTo: allocPod("ocloud-system", "dyn-pod-0")},
	}
	src := &fakeSliceSource{samples: samples}
	col := NewSliceCollector(src)

	mf := gatherFamily(t, col, "ascend_slice_aicore_count")
	require.NotNil(t, mf, "ascend_slice_aicore_count family must be present")
	require.Len(t, mf.GetMetric(), 1)

	labels := map[string]string{}
	for _, lp := range mf.GetMetric()[0].GetLabel() {
		labels[lp.GetName()] = lp.GetValue()
	}
	assert.Equal(t, "", labels["template"], "Dynamic slice must carry template=\"\"")
	assert.Equal(t, "dyn-0", labels["slice_id"])
	assert.Equal(t, "n0", labels["npu_id"])
	assert.Equal(t, "w-01", labels["node"])
}

// TestSliceCollector_FreeSliceNoAllocatedMetric: a free slice
// (AllocatedTo==nil) emits aicore + memory but NOT allocated_to_pod.
func TestSliceCollector_FreeSliceNoAllocatedMetric(t *testing.T) {
	samples := []sources.SliceSample{
		{ID: "free-0", NPUID: "n0", NodeName: "w-01", Template: "vir04",
			AICoreCount: 4, MemoryUsedBytes: 0, AllocatedTo: nil},
	}
	src := &fakeSliceSource{samples: samples}
	col := NewSliceCollector(src)

	assert.Equal(t, 1, testutil.CollectAndCount(col, "ascend_slice_aicore_count"))
	assert.Equal(t, 1, testutil.CollectAndCount(col, "ascend_slice_memory_used_bytes"))
	assert.Equal(t, 0, testutil.CollectAndCount(col, "ascend_slice_allocated_to_pod"))
}

// TestSliceCollector_Describe: Describe must emit exactly 4 Desc:
// aicore_count, memory_used_bytes, allocated_to_pod (P3-T-101) +
// aicore_utilization (P11-fix-002).
func TestSliceCollector_Describe(t *testing.T) {
	src := &fakeSliceSource{}
	col := NewSliceCollector(src)
	ch := make(chan *prometheus.Desc, 8)
	col.Describe(ch)
	close(ch)
	count := 0
	for range ch {
		count++
	}
	assert.Equal(t, 4, count)
}

// TestSliceCollector_AllocatedPodLabels: verify the allocated_to_pod
// series carries the right namespace/pod labels and value 1.
func TestSliceCollector_AllocatedPodLabels(t *testing.T) {
	samples := []sources.SliceSample{
		{ID: "n0-vir08-0", NPUID: "n0", NodeName: "w-01", Template: "vir08",
			AICoreCount: 8, MemoryUsedBytes: 34359738368,
			AllocatedTo: allocPod("inference", "qwen-prefill-0")},
	}
	src := &fakeSliceSource{samples: samples}
	col := NewSliceCollector(src)

	mf := gatherFamily(t, col, "ascend_slice_allocated_to_pod")
	require.NotNil(t, mf)
	require.Len(t, mf.GetMetric(), 1)

	m := mf.GetMetric()[0]
	labels := map[string]string{}
	for _, lp := range m.GetLabel() {
		labels[lp.GetName()] = lp.GetValue()
	}
	assert.Equal(t, "n0-vir08-0", labels["slice_id"])
	assert.Equal(t, "n0", labels["npu_id"])
	assert.Equal(t, "inference", labels["namespace"])
	assert.Equal(t, "qwen-prefill-0", labels["pod"])
	require.NotNil(t, m.Gauge)
	assert.InDelta(t, 1.0, m.Gauge.GetValue(), 0.0001)
}
