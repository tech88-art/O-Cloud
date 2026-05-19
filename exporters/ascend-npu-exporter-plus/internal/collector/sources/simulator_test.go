package sources

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTempJSON drops content into a fresh file under t.TempDir() and
// returns the path. Cleanup is handled by t.TempDir().
func writeTempJSON(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sim.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestNewSimulatorSource_Happy(t *testing.T) {
	path := writeTempJSON(t, `{
      "npus": [
        {"id":"n-0","nodeName":"node-a","model":"Ascend910B","utilizationSeed":40.0,"memoryUsedSeedBytes":8589934592,"memoryTotalBytes":68719476736,"hbmBandwidthSeedBytesPerSecond":400000000000,"healthy":true},
        {"id":"n-1","nodeName":"node-a","model":"Ascend910B","utilizationSeed":55.0,"memoryUsedSeedBytes":12884901888,"memoryTotalBytes":68719476736,"hbmBandwidthSeedBytesPerSecond":450000000000,"healthy":true},
        {"id":"n-2","nodeName":"node-b","model":"Ascend910B","utilizationSeed":70.0,"memoryUsedSeedBytes":17179869184,"memoryTotalBytes":68719476736,"hbmBandwidthSeedBytesPerSecond":500000000000,"healthy":false}
      ]
    }`)

	src, err := NewSimulatorSource(path)
	require.NoError(t, err)

	samples, err := src.ReadNPUs(context.Background())
	require.NoError(t, err)
	require.Len(t, samples, 3)

	// Sample 0 identity fields preserved verbatim.
	assert.Equal(t, "n-0", samples[0].ID)
	assert.Equal(t, "node-a", samples[0].NodeName)
	assert.Equal(t, "Ascend910B", samples[0].Model)
	assert.Equal(t, uint64(68719476736), samples[0].MemoryTotalBytes)
	assert.True(t, samples[0].Healthy)
	assert.False(t, samples[2].Healthy)

	// Utilization within +/- 10% band of each seed.
	assert.InDelta(t, 40.0, samples[0].AICore, 4.0)
	assert.InDelta(t, 55.0, samples[1].AICore, 5.5)
	assert.InDelta(t, 70.0, samples[2].AICore, 7.0)

	// Memory used never exceeds total.
	for _, s := range samples {
		assert.LessOrEqual(t, s.MemoryUsedBytes, s.MemoryTotalBytes)
	}
}

func TestNewSimulatorSource_FileMissing(t *testing.T) {
	_, err := NewSimulatorSource(filepath.Join(t.TempDir(), "does-not-exist.json"))
	require.Error(t, err)
}

func TestNewSimulatorSource_MalformedJSON(t *testing.T) {
	path := writeTempJSON(t, `{this-is-not-json`)
	_, err := NewSimulatorSource(path)
	require.Error(t, err)
}

func TestSimulator_PerturbationWithin10Percent(t *testing.T) {
	// Seed utilization 50.0 -> every sample must be in [45.0, 55.0].
	path := writeTempJSON(t, `{
      "npus": [
        {"id":"n-0","nodeName":"node-a","model":"Ascend910B","utilizationSeed":50.0,"memoryUsedSeedBytes":1000,"memoryTotalBytes":10000,"hbmBandwidthSeedBytesPerSecond":1000,"healthy":true}
      ]
    }`)
	src, err := NewSimulatorSource(path)
	require.NoError(t, err)

	for i := 0; i < 100; i++ {
		samples, err := src.ReadNPUs(context.Background())
		require.NoError(t, err)
		require.Len(t, samples, 1)
		assert.GreaterOrEqual(t, samples[0].AICore, 45.0,
			"iter %d util=%f below 45.0 band", i, samples[0].AICore)
		assert.LessOrEqual(t, samples[0].AICore, 55.0,
			"iter %d util=%f above 55.0 band", i, samples[0].AICore)
	}
}

func TestSimulator_EmptyNPUList(t *testing.T) {
	path := writeTempJSON(t, `{"npus": []}`)
	src, err := NewSimulatorSource(path)
	require.NoError(t, err)
	samples, err := src.ReadNPUs(context.Background())
	require.NoError(t, err)
	assert.Empty(t, samples)
}
