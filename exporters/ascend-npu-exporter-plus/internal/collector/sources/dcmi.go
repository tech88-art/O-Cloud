package sources

import "context"

// dcmi.go — real NPU telemetry "DCMI-preferred" read path
// (P13-T-102 · ADR-0024 §2 Decision D + §4(c)).
//
// **exec over cgo** (ADR-0024 §4(c) red line): the Phase 13 body reads
// every telemetry field (utilization% / memory / HBM bandwidth / temp /
// power) from `npu-smi` stdout via os/exec — it does NOT bind Huawei's
// libdcmi.so. npu-smi is the userspace face of the same DCMI library, so
// the data is identical, but staying on exec keeps the default
// CGO_ENABLED=0 GOARCH=arm64 cross-compile (ADR-0020) green WITHOUT a
// build tag. A future direct libdcmi.so cgo binding (lower scrape
// latency on very dense hosts) is the reserved `//go:build dcmi` variant
// per ADR-0024 §4(c): real body behind the tag, no-cgo stub by default,
// so `cross-compile-arm64` (ci.yml) compiles the stub and never breaks.
//
// Naming: this is the "DCMI source" the helm selector
// (sourceType=dcmi / real) and exporters/CLAUDE.md §5 fallback chain
// ("DCMI → npu-smi") name. Today DCMI and npu-smi share one read
// mechanism (npu-smi exec); the distinct type preserves the selector
// surface + the build-tag swap point without an `if real {}` branch in
// the collector layer (decoupling-seam invariant · ADR-0024 §2 Decision G).

// DCMISource is the preferred real telemetry source. It delegates to the
// no-cgo npu-smi reader; the type exists so sources.Select can express the
// "DCMI-preferred, npu-smi-fallback" chain (exporters/CLAUDE.md §5) and so
// a future cgo libdcmi binding can replace the delegate behind a build tag
// without touching callers.
type DCMISource struct {
	delegate Source
}

// DCMIConfig captures DCMISource construction parameters. It mirrors
// NPUSMIConfig because the no-cgo body reads via npu-smi; the cgo variant
// (build-tag) would add a LibraryPath / device-socket field here.
type DCMIConfig struct {
	// BinaryPath is the absolute path to npu-smi (empty → PATH lookup).
	BinaryPath string
	// NodeName is stamped on every NPUSample.NodeName ($NODE_NAME in the
	// DaemonSet).
	NodeName string
	// DefaultModel backs NPUSample.Model when the board view omits a chip
	// name (empty → "Ascend910B").
	DefaultModel string
}

// NewDCMISource constructs a DCMISource. Construction never fails; a
// missing npu-smi binary surfaces as ErrNoCommand at the first ReadNPUs
// (see NewNPUSMISource).
//
// The no-cgo body delegates to the npu-smi reader, so today DCMIConfig and
// NPUSMIConfig carry identical fields and convert directly. When the cgo
// libdcmi variant lands (build-tag · ADR-0024 §4(c)) it will add a
// LibraryPath / device-socket field to DCMIConfig; the conversion below
// then stops compiling and forces this seam to be revisited.
func NewDCMISource(cfg DCMIConfig) *DCMISource {
	return &DCMISource{delegate: NewNPUSMISource(NPUSMIConfig(cfg))}
}

// newDCMISourceWithDelegate is the test seam: it injects an arbitrary
// Source delegate so the DCMI wrapper is exercised without a lab npu-smi
// binary. Production code calls NewDCMISource.
func newDCMISourceWithDelegate(delegate Source) *DCMISource {
	return &DCMISource{delegate: delegate}
}

// ReadNPUs returns live per-device telemetry via the delegate reader.
func (s *DCMISource) ReadNPUs(ctx context.Context) ([]NPUSample, error) {
	return s.delegate.ReadNPUs(ctx)
}
