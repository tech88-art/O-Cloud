package registry

import (
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findFamily returns the first MetricFamily with the given name from gathered
// metrics, or nil if not present.
func findFamily(t *testing.T, mfs []*dto.MetricFamily, name string) *dto.MetricFamily {
	t.Helper()
	for _, mf := range mfs {
		if mf.GetName() == name {
			return mf
		}
	}
	return nil
}

func TestNew_RegistersBuildInfo(t *testing.T) {
	reg := New("1.2.3", "abcd", "go1.24")
	require.NotNil(t, reg)

	mfs, err := reg.Gather()
	require.NoError(t, err)

	mf := findFamily(t, mfs, "exporter_build_info")
	require.NotNil(t, mf, "exporter_build_info family must be registered")
	require.Equal(t, dto.MetricType_GAUGE, mf.GetType())
	require.Len(t, mf.GetMetric(), 1)

	m := mf.GetMetric()[0]
	labels := map[string]string{}
	for _, lp := range m.GetLabel() {
		labels[lp.GetName()] = lp.GetValue()
	}
	assert.Equal(t, "1.2.3", labels["version"])
	assert.Equal(t, "abcd", labels["commit"])
	assert.Equal(t, "go1.24", labels["go_version"])
	require.NotNil(t, m.Gauge)
	assert.InDelta(t, 1.0, m.Gauge.GetValue(), 0.0001)
}

func TestNew_RegistersCollectDuration(t *testing.T) {
	reg := New("1.0", "x", "go1.24")
	mfs, err := reg.Gather()
	require.NoError(t, err)

	// Histograms with no observations may still surface in Gather output as a
	// family with metric type HISTOGRAM but zero metrics; with our label-vec
	// design they only surface after at least one observation. Confirm the
	// package-level pointer is wired and the family appears once we observe.
	require.NotNil(t, CollectDuration, "CollectDuration must be assigned by New")

	CollectDuration.WithLabelValues("probe").Observe(0.001)
	mfs, err = reg.Gather()
	require.NoError(t, err)

	mf := findFamily(t, mfs, "exporter_collect_duration_seconds")
	require.NotNil(t, mf, "exporter_collect_duration_seconds family must be registered")
	assert.Equal(t, dto.MetricType_HISTOGRAM, mf.GetType())
}

func TestCollectDuration_LabelExpansion(t *testing.T) {
	reg := New("1.0", "y", "go1.24")
	require.NotNil(t, reg)
	require.NotNil(t, CollectDuration)

	// Should not panic and should record one observation.
	CollectDuration.WithLabelValues("test-collector").Observe(0.1)

	mfs, err := reg.Gather()
	require.NoError(t, err)
	mf := findFamily(t, mfs, "exporter_collect_duration_seconds")
	require.NotNil(t, mf)

	var sampleCount uint64
	for _, m := range mf.GetMetric() {
		for _, lp := range m.GetLabel() {
			if lp.GetName() == "collector" && lp.GetValue() == "test-collector" {
				sampleCount = m.Histogram.GetSampleCount()
			}
		}
	}
	assert.Equal(t, uint64(1), sampleCount, "expected exactly 1 observation for test-collector")
}
