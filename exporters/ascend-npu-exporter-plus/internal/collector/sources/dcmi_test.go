package sources

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubSource is a minimal Source for exercising the DCMI wrapper's
// delegation without a runner.
type stubSource struct {
	samples []NPUSample
	err     error
}

func (s *stubSource) ReadNPUs(_ context.Context) ([]NPUSample, error) {
	return s.samples, s.err
}

// TestDCMISource_DelegatesReads asserts the DCMI wrapper forwards
// ReadNPUs to its delegate verbatim (no-cgo body delegates to npu-smi
// today · ADR-0024 §4(c)).
func TestDCMISource_DelegatesReads(t *testing.T) {
	want := []NPUSample{
		{ID: "worker-a-npu-0", NodeName: "worker-a", Model: "Ascend910B", AICore: 42.0,
			MemoryUsedBytes: 1 << 30, MemoryTotalBytes: 64 << 30, Healthy: true},
	}
	src := newDCMISourceWithDelegate(&stubSource{samples: want})

	got, err := src.ReadNPUs(context.Background())
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// TestDCMISource_PropagatesError asserts a delegate error (e.g. missing
// npu-smi) surfaces through the wrapper unchanged.
func TestDCMISource_PropagatesError(t *testing.T) {
	src := newDCMISourceWithDelegate(&stubSource{err: ErrNoCommand})
	_, err := src.ReadNPUs(context.Background())
	require.ErrorIs(t, err, ErrNoCommand)
}

// TestNewDCMISource_RealConstruction asserts the production constructor
// builds a usable wrapper whose first read surfaces ErrNoCommand when
// npu-smi is absent (the dev-box / mis-targeted-node case) — i.e. it does
// not panic and does not fabricate samples.
func TestNewDCMISource_RealConstruction(t *testing.T) {
	src := NewDCMISource(DCMIConfig{
		BinaryPath: "/nonexistent/npu-smi-xyz",
		NodeName:   "worker-a",
	})
	_, err := src.ReadNPUs(context.Background())
	require.Error(t, err)
	// On a host without npu-smi this is ErrNoCommand; the assertion is the
	// error type contract, not the absence of hardware.
	assert.True(t, errors.Is(err, ErrNoCommand), "want ErrNoCommand, got %v", err)
}
