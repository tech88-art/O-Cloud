// Package sources defines the NPU sample read interface and its
// implementations: the simulator (replays a JSON snapshot · demo profile)
// and the real DCMI / npu-smi readers (P13-T-102 · real profile · live
// telemetry via the npu-smi CLI, no cgo per ADR-0024 §4(c)).
//
// **Decoupling-seam invariant** (ADR-0024 §2 Decision G): this package IS
// the seam. Select() maps a SourceType (mock / simulator / real) to a
// concrete Source; the collector layer (npu / slice / workload.go)
// consumes the returned interface uniformly and has no `if real {}`
// branch. demo / real profile difference is expressed here (source
// selection) + chart values (simulator.enabled), not in shared code.
package sources

import (
	"context"
	"errors"
	"fmt"
)

// ErrSourceNotAvailable is the sentinel for a real source that cannot be
// constructed for the requested SourceType (e.g. SourceTypeReal without a
// reachable npu-smi). It is also returned by Select for an unknown type.
var ErrSourceNotAvailable = errors.New("source not available; install real driver or use simulator")

// SourceType selects which Source implementation Select builds.
type SourceType string

const (
	// SourceTypeSimulator replays a JSON snapshot (demo profile · the
	// long-lived functional validation台 · ADR-0023 §2 Decision E). NPU +
	// slice samples come from the file at SelectConfig.SimulatorPath.
	SourceTypeSimulator SourceType = "simulator"

	// SourceTypeReal reads live telemetry from real Ascend silicon via the
	// npu-smi CLI (real profile · ADR-0024 §2 Decision D). Prefers the
	// DCMI-preferred reader; npu-smi is the same no-cgo mechanism today
	// (exporters/CLAUDE.md §5 fallback chain).
	SourceTypeReal SourceType = "real"

	// SourceTypeMock is an alias of simulator kept for selector-surface
	// symmetry with backend datasource config (which uses mock / k8s /
	// crd). For the exporter, "mock" == replay-a-file == simulator.
	SourceTypeMock SourceType = "mock"
)

// SelectConfig carries everything Select needs to build the chosen Source.
// Fields irrelevant to the chosen type are ignored.
type SelectConfig struct {
	// Type selects the implementation. Empty → SourceTypeSimulator (demo
	// default, matching chart simulator.enabled=true).
	Type SourceType

	// SimulatorPath is the JSON snapshot path for the simulator / mock
	// types. Required for those types.
	SimulatorPath string

	// NodeName is stamped on real NPUSamples ($NODE_NAME in the DaemonSet).
	// Real types only.
	NodeName string

	// NPUSMIBinaryPath is the absolute npu-smi path for real types. Empty
	// → PATH lookup.
	NPUSMIBinaryPath string

	// DefaultModel backs NPUSample.Model when npu-smi omits a chip name
	// (real types · empty → "Ascend910B").
	DefaultModel string
}

// Select builds the Source for cfg.Type. It is the single profile-aware
// decision point (the decoupling seam · ADR-0024 §2 Decision G): callers
// (main.go) pick a type from config; everything downstream consumes the
// returned Source uniformly.
//
// For simulator / mock it also returns a SliceSource (the simulator
// implements both); real types return (Source, nil, …) because slice-level
// telemetry on real silicon comes from the DRA driver's ResourceSlice
// status (backend aggregation · T103), not from this exporter's device
// reads — the exporter's real slice series stay simulator-fed until that
// path lands (carry-forward · devlog).
func Select(cfg SelectConfig) (Source, SliceSource, error) {
	switch cfg.Type {
	case "", SourceTypeSimulator, SourceTypeMock:
		if cfg.SimulatorPath == "" {
			return nil, nil, fmt.Errorf("%w: simulator source needs a JSON path", ErrSourceNotAvailable)
		}
		sim, err := NewSimulatorSource(cfg.SimulatorPath)
		if err != nil {
			return nil, nil, err
		}
		return sim, sim, nil
	case SourceTypeReal:
		// DCMI-preferred, npu-smi-fallback — both no-cgo today (ADR-0024
		// §4(c)). The DCMI wrapper delegates to the npu-smi reader, so a
		// future cgo libdcmi binding swaps behind a build tag without
		// changing this call site.
		src := NewDCMISource(DCMIConfig{
			BinaryPath:   cfg.NPUSMIBinaryPath,
			NodeName:     cfg.NodeName,
			DefaultModel: cfg.DefaultModel,
		})
		return src, nil, nil
	default:
		return nil, nil, fmt.Errorf("%w: unknown source type %q", ErrSourceNotAvailable, cfg.Type)
	}
}

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
	// TemperatureCelsius is the on-die temperature at sample time. Added
	// in P11-fix-002 to back the npu-detail / node-detail temperature
	// panels; synthesised by SimulatorSource when seed is missing.
	TemperatureCelsius float64
	// PowerWatts is the instantaneous power draw at sample time. Same
	// provenance as TemperatureCelsius.
	PowerWatts float64
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
	// AICoreUtilization is the AI Core utilization percentage of this
	// slice, in [0, 100]. Added in P11-fix-002 to back workload-resource
	// dashboard panels via the `ascend_npu_slice_util_percent` series.
	AICoreUtilization float64
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
