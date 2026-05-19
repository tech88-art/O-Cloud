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

// TestSimulator_ReadSlices_HappyPath: fixture with 3 slices (1
// Allocated + 2 Idle) returns 3 samples with the correct
// AllocatedTo-nil / non-nil split.
func TestSimulator_ReadSlices_HappyPath(t *testing.T) {
	path := writeTempJSON(t, `{
      "npus": [
        {"id":"n-0","nodeName":"node-a","model":"Ascend910B","utilizationSeed":40.0,"memoryUsedSeedBytes":8589934592,"memoryTotalBytes":68719476736,"hbmBandwidthSeedBytesPerSecond":400000000000,"healthy":true}
      ],
      "slices": [
        {"id":"n-0-vir04-0","npuId":"n-0","nodeName":"node-a","template":"vir04","aiCoreCount":4,"memoryUsedSeedBytes":17179869184,"allocatedNamespace":"ocloud-system","allocatedPod":"qwen-prefill-0"},
        {"id":"n-0-vir04-1","npuId":"n-0","nodeName":"node-a","template":"vir04","aiCoreCount":4,"memoryUsedSeedBytes":0},
        {"id":"n-0-vir04-2","npuId":"n-0","nodeName":"node-a","template":"vir04","aiCoreCount":4,"memoryUsedSeedBytes":0}
      ]
    }`)

	src, err := NewSimulatorSource(path)
	require.NoError(t, err)

	slices, err := src.ReadSlices(context.Background())
	require.NoError(t, err)
	require.Len(t, slices, 3)

	// Identity fields preserved verbatim.
	assert.Equal(t, "n-0-vir04-0", slices[0].ID)
	assert.Equal(t, "n-0", slices[0].NPUID)
	assert.Equal(t, "node-a", slices[0].NodeName)
	assert.Equal(t, "vir04", slices[0].Template)
	assert.Equal(t, int32(4), slices[0].AICoreCount)

	// Allocated/Idle classification: only slice[0] is allocated.
	require.NotNil(t, slices[0].AllocatedTo)
	assert.Equal(t, "ocloud-system", slices[0].AllocatedTo.Namespace)
	assert.Equal(t, "qwen-prefill-0", slices[0].AllocatedTo.Pod)
	assert.Nil(t, slices[1].AllocatedTo)
	assert.Nil(t, slices[2].AllocatedTo)
}

// TestSimulator_ReadSlices_EmptyList: missing or empty "slices" block
// produces no samples without error.
func TestSimulator_ReadSlices_EmptyList(t *testing.T) {
	path := writeTempJSON(t, `{"npus": [], "slices": []}`)
	src, err := NewSimulatorSource(path)
	require.NoError(t, err)
	slices, err := src.ReadSlices(context.Background())
	require.NoError(t, err)
	assert.Empty(t, slices)
}

// TestSimulator_ReadSlices_MissingBlock: a JSON with no "slices" key
// at all (T007-era fixture) is still valid; ReadSlices returns empty.
func TestSimulator_ReadSlices_MissingBlock(t *testing.T) {
	path := writeTempJSON(t, `{
      "npus": [
        {"id":"n-0","nodeName":"node-a","model":"Ascend910B","utilizationSeed":40.0,"memoryUsedSeedBytes":8589934592,"memoryTotalBytes":68719476736,"hbmBandwidthSeedBytesPerSecond":400000000000,"healthy":true}
      ]
    }`)
	src, err := NewSimulatorSource(path)
	require.NoError(t, err)
	slices, err := src.ReadSlices(context.Background())
	require.NoError(t, err)
	assert.Empty(t, slices)
}

// TestSimulator_PerturbationWithin10Percent_Slices: seed memory 1000 ->
// every sample must be in [900, 1100] across 100 reads.
func TestSimulator_PerturbationWithin10Percent_Slices(t *testing.T) {
	path := writeTempJSON(t, `{
      "npus": [],
      "slices": [
        {"id":"s-0","npuId":"n-0","nodeName":"node-a","template":"vir04","aiCoreCount":4,"memoryUsedSeedBytes":1000}
      ]
    }`)
	src, err := NewSimulatorSource(path)
	require.NoError(t, err)

	for i := 0; i < 100; i++ {
		slices, err := src.ReadSlices(context.Background())
		require.NoError(t, err)
		require.Len(t, slices, 1)
		// AICoreCount + identity fields stable across reads.
		assert.Equal(t, int32(4), slices[0].AICoreCount)
		// Memory perturbed within +/- 10% of seed (1000 -> [900, 1100]).
		assert.GreaterOrEqual(t, slices[0].MemoryUsedBytes, uint64(900),
			"iter %d mem=%d below 900 band", i, slices[0].MemoryUsedBytes)
		assert.LessOrEqual(t, slices[0].MemoryUsedBytes, uint64(1100),
			"iter %d mem=%d above 1100 band", i, slices[0].MemoryUsedBytes)
	}
}

// TestSimulator_ReadSlices_PartialAllocatedFieldsTreatedAsIdle:
// AllocatedNamespace without AllocatedPod (or vice-versa) is a
// malformed Allocated record; sim treats it as Idle.
func TestSimulator_ReadSlices_PartialAllocatedFieldsTreatedAsIdle(t *testing.T) {
	path := writeTempJSON(t, `{
      "npus": [],
      "slices": [
        {"id":"s-ns-only","npuId":"n-0","nodeName":"node-a","template":"vir04","aiCoreCount":4,"memoryUsedSeedBytes":0,"allocatedNamespace":"ocloud-system"},
        {"id":"s-pod-only","npuId":"n-0","nodeName":"node-a","template":"vir04","aiCoreCount":4,"memoryUsedSeedBytes":0,"allocatedPod":"orphan-pod"}
      ]
    }`)
	src, err := NewSimulatorSource(path)
	require.NoError(t, err)
	slices, err := src.ReadSlices(context.Background())
	require.NoError(t, err)
	require.Len(t, slices, 2)
	assert.Nil(t, slices[0].AllocatedTo, "namespace-only should be Idle")
	assert.Nil(t, slices[1].AllocatedTo, "pod-only should be Idle")
}
