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

// SliceSample is one snapshot of a NPU slice (vir04 / vir08 fixed template
// or a Dynamic slice). Slices are sub-allocations of a parent NPU device.
type SliceSample struct {
	// ID is the unique slice identifier (e.g. "worker-site-a-01-npu-0-vir04-0").
	ID string
	// NPUID is the parent NPU device this slice partitions.
	NPUID string
	// NodeName is the K8s Node hosting the parent NPU.
	NodeName string
	// Template is the SliceTemplate name (e.g. "vir04", "vir08") for
	// FixedTemplate slices; empty for Dynamic-strategy slices.
	Template string
	// AICoreCount is the AI Core allocation of this slice instance.
	AICoreCount int32
	// MemoryUsedBytes is the realised HBM usage of this slice at sample time.
	MemoryUsedBytes uint64
	// AllocatedTo is the Pod owning this slice; nil when the slice is free.
	AllocatedTo *AllocatedPod
}

// AllocatedPod identifies the K8s Pod owning an Allocated slice.
type AllocatedPod struct {
	Namespace string
	Pod       string
}

// SliceSource is the read-side interface for slice samples. Phase 3
// simulator implements it; Phase 4+ DCMI / npu-smi sources will add
// their own implementations alongside ReadNPUs.
//
// Separate from Source (NPU samples) so Phase-4+ stub sources are not
// forced to implement slice reads they cannot satisfy until the DRA
// driver ships.
type SliceSource interface {
	// ReadSlices returns one SliceSample per materialised slice across
	// all NPUs this source observes.
	ReadSlices(ctx context.Context) ([]SliceSample, error)
}
