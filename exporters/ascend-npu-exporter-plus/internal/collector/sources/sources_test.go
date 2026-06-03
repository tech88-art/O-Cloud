package sources

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeSimJSON drops a minimal valid simulator file and returns its path.
func writeSimJSON(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sim.json")
	content := `{"npus":[{"id":"n-0","nodeName":"node-a","model":"Ascend910B","utilizationSeed":40.0,"memoryUsedSeedBytes":8589934592,"memoryTotalBytes":68719476736,"hbmBandwidthSeedBytesPerSecond":400000000000,"healthy":true}]}`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// TestSelect_SimulatorReturnsBothSources asserts the simulator / mock
// types return a Source AND a SliceSource (the simulator implements both).
func TestSelect_SimulatorReturnsBothSources(t *testing.T) {
	path := writeSimJSON(t)
	for _, typ := range []SourceType{"", SourceTypeSimulator, SourceTypeMock} {
		npuSrc, sliceSrc, err := Select(SelectConfig{Type: typ, SimulatorPath: path})
		require.NoError(t, err, "type %q", typ)
		require.NotNil(t, npuSrc, "type %q npu source", typ)
		require.NotNil(t, sliceSrc, "type %q slice source", typ)

		samples, err := npuSrc.ReadNPUs(context.Background())
		require.NoError(t, err)
		assert.Len(t, samples, 1)
	}
}

// TestSelect_SimulatorMissingPath asserts the simulator type without a
// path is an error (caller must supply the snapshot).
func TestSelect_SimulatorMissingPath(t *testing.T) {
	_, _, err := Select(SelectConfig{Type: SourceTypeSimulator})
	require.ErrorIs(t, err, ErrSourceNotAvailable)
}

// TestSelect_RealReturnsNPUSourceOnly asserts the real type returns a
// Source and a nil SliceSource (real slice telemetry comes from the DRA
// driver / backend, not this exporter · carry-forward). Construction
// succeeds even without npu-smi (the error surfaces at read time).
func TestSelect_RealReturnsNPUSourceOnly(t *testing.T) {
	npuSrc, sliceSrc, err := Select(SelectConfig{
		Type:             SourceTypeReal,
		NPUSMIBinaryPath: "/nonexistent/npu-smi-xyz",
		NodeName:         "worker-a",
	})
	require.NoError(t, err)
	require.NotNil(t, npuSrc)
	assert.Nil(t, sliceSrc, "real source must not return a SliceSource")

	// The real source is a *DCMISource (DCMI-preferred chain).
	_, ok := npuSrc.(*DCMISource)
	assert.True(t, ok, "real source should be DCMI-preferred (*DCMISource)")

	// First read surfaces ErrNoCommand on a host without npu-smi.
	_, readErr := npuSrc.ReadNPUs(context.Background())
	require.ErrorIs(t, readErr, ErrNoCommand)
}

// TestSelect_UnknownType asserts an unrecognised type is an error.
func TestSelect_UnknownType(t *testing.T) {
	_, _, err := Select(SelectConfig{Type: SourceType("bogus")})
	require.ErrorIs(t, err, ErrSourceNotAvailable)
}
