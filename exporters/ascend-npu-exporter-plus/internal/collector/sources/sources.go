// Package sources defines the NPU sample read interface and three
// implementations: simulator (Phase 3) and DCMI / npu-smi stubs that
// land in Phase 4+ when real Ascend hardware is wired.
package sources

import (
	"context"
	"errors"
)

// ErrSourceNotAvailable is returned by Phase-4+ source stubs (dcmi, npu_smi)
// when called in Phase 3. Callers select the simulator source instead.
var ErrSourceNotAvailable = errors.New("source not available; install real driver or use simulator")

// Source is the read-side interface every NPU sample provider implements.
// Phase 3 ships a simulator; Phase 4+ DCMI + npu-smi land per arch §1.3.
type Source interface {
	// ReadNPUs returns one NPUSample per physical NPU device visible to
	// this exporter instance.
	ReadNPUs(ctx context.Context) ([]NPUSample, error)
}

// NPUSample is one snapshot of a physical NPU's live state.
type NPUSample struct {
	// ID is the per-device identifier (e.g. "worker-site-a-01-npu-0").
	ID string
	// NodeName is the K8s Node hosting this NPU.
	NodeName string
	// Model is the device model (e.g. "Ascend910B").
	Model string
	// AICore is the AI Core utilization percentage in [0, 100].
	AICore float64
	// MemoryUsedBytes is the HBM occupancy at sample time.
	MemoryUsedBytes uint64
	// MemoryTotalBytes is the HBM capacity (fixed per device model).
	MemoryTotalBytes uint64
	// HBMBandwidthBytesPerSecond is the realised HBM bandwidth at sample time.
	HBMBandwidthBytesPerSecond uint64
	// Healthy reports whether the device passes its self-check.
	Healthy bool
}
