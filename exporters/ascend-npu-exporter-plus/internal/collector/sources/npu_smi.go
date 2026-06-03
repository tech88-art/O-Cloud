package sources

import (
	"context"
	"sort"
	"strconv"
	"strings"
)

// npu_smi.go — real NPU telemetry read via the `npu-smi` CLI
// (P13-T-102 · ADR-0024 §2 Decision D). The Phase 4 stub
// (ReadNPUs → ErrSourceNotAvailable) is now a real body that shells out
// to npu-smi (ADR-0011 §3 lab-gating flipped by ADR-0024 §2 Decision A).
//
// Read mechanism (no cgo · ADR-0024 §4(c)):
//
//   - device enumeration: `npu-smi info -l` → NPU IDs
//   - per device, merge `npu-smi info -t common -i <id>`
//     (temperature / power / health / memory) + `-t usages -i <id>`
//     (aicore utilization% / HBM used / HBM bandwidth) +
//     `-t board -i <id>` (chip name / capacity) into one NPUSample
//
// Every field NPUSample carries is reachable from npu-smi stdout, so there
// is no libdcmi.so binding and no `//go:build dcmi` tag — the default
// CGO_ENABLED=0 GOARCH=arm64 cross-compile (ADR-0020) stays green.
//
// **Decoupling-seam invariant** (ADR-0024 §2 Decision G): this body lives
// BELOW the Source seam. NPUCollector consumes the same NPUSample shape the
// simulator returns; there is no `if real {}` branch in the collector
// layer. demo profile (simulator) regression is unaffected.
//
// **Lab fallback** (ADR-0024 §3): a per-device view that does not parse is
// soft (the device is still emitted with whatever fields did parse + model
// defaults); enumeration failure surfaces the error so a mis-targeted node
// (no driver) fails loudly rather than emitting fabricated data.

// defaultNPUMemoryTotalBytes backs NPUSample.MemoryTotalBytes when neither
// the board nor common view surfaces HBM capacity. 64 GiB is the 910B HBM
// size; mirrors the simulator seed convention so vram_used_percent is
// computable across demo/real profiles.
const defaultNPUMemoryTotalBytes uint64 = 64 * 1024 * 1024 * 1024

// NPUSMISource reads live NPU telemetry by invoking the npu-smi CLI. It
// implements sources.Source (ReadNPUs).
type NPUSMISource struct {
	runner   npuSMIRunner
	nodeName string
	model    string
}

// NPUSMIConfig captures NPUSMISource construction parameters.
type NPUSMIConfig struct {
	// BinaryPath is the absolute path to npu-smi. Empty → PATH lookup of
	// "npu-smi".
	BinaryPath string
	// NodeName is stamped on every NPUSample.NodeName (the K8s Node this
	// exporter pod runs on; the DaemonSet passes $NODE_NAME). Empty → "".
	NodeName string
	// DefaultModel backs NPUSample.Model when the board view does not
	// surface a chip name. Empty → "Ascend910B".
	DefaultModel string
}

// NewNPUSMISource constructs an NPUSMISource backed by the npu-smi CLI
// (production path). Construction never fails — a missing binary surfaces
// as ErrNoCommand at the first ReadNPUs so the exporter starts and serves
// its meta metrics rather than crash-looping a node that is mid-driver
// install.
func NewNPUSMISource(cfg NPUSMIConfig) *NPUSMISource {
	model := cfg.DefaultModel
	if model == "" {
		model = "Ascend910B"
	}
	return newNPUSMISourceWithRunner(&execRunner{binaryPath: cfg.BinaryPath}, cfg.NodeName, model)
}

// newNPUSMISourceWithRunner is the test seam: it injects an arbitrary
// npuSMIRunner (a canned runner in table-tests) so the real body runs
// without a lab npu-smi binary. Production code calls NewNPUSMISource.
func newNPUSMISourceWithRunner(runner npuSMIRunner, nodeName, model string) *NPUSMISource {
	return &NPUSMISource{runner: runner, nodeName: nodeName, model: model}
}

// ReadNPUs enumerates the node's NPUs via `npu-smi info -l` and projects
// each device's merged telemetry views into an NPUSample.
func (s *NPUSMISource) ReadNPUs(ctx context.Context) ([]NPUSample, error) {
	listOut, err := s.runner.run(ctx, "info", "-l")
	if err != nil {
		return nil, err
	}
	ids := parseNPUList(listOut)

	out := make([]NPUSample, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.readOne(ctx, id))
	}
	return out, nil
}

// readOne composes one NPUSample for device id by merging the common /
// usages / board views. A view that fails to run or parse is skipped
// (soft per-device per ADR-0024 §3); the resulting sample carries whatever
// fields did parse plus model defaults.
func (s *NPUSMISource) readOne(ctx context.Context, id int) NPUSample {
	var u npuUsage
	for _, args := range [][]string{
		{"info", "-t", "common", "-i", strconv.Itoa(id)},
		{"info", "-t", "usages", "-i", strconv.Itoa(id)},
		{"info", "-t", "board", "-i", strconv.Itoa(id)},
	} {
		out, err := s.runner.run(ctx, args...)
		if err != nil {
			continue
		}
		if parsed, ok := parseNPUSMIDevice(out); ok {
			mergeNPUUsage(&u, &parsed)
		}
	}
	return s.composeSample(id, &u)
}

// composeSample turns the merged npuUsage into an NPUSample, applying
// model + capacity defaults and preferring HBM figures over generic
// memory figures (910B telemetry surfaces HBM as the device memory).
func (s *NPUSMISource) composeSample(id int, u *npuUsage) NPUSample {
	model := s.model
	if u.chipName != "" {
		model = normaliseChipName(u.chipName)
	}

	total := defaultNPUMemoryTotalBytes
	switch {
	case u.hbmTotalMiB != nil && *u.hbmTotalMiB > 0:
		total = mibToBytes(*u.hbmTotalMiB)
	case u.memTotalMiB != nil && *u.memTotalMiB > 0:
		total = mibToBytes(*u.memTotalMiB)
	}

	var used uint64
	switch {
	case u.hbmUsedMiB != nil:
		used = mibToBytes(*u.hbmUsedMiB)
	case u.memUsedMiB != nil:
		used = mibToBytes(*u.memUsedMiB)
	}
	if used > total {
		used = total
	}

	var aicore float64
	if u.aicorePct != nil {
		aicore = clamp(*u.aicorePct, 0, 100)
	}
	var bw uint64
	if u.hbmBWMiBps != nil {
		bw = mibToBytes(*u.hbmBWMiBps)
	}
	var temp float64
	if u.temperatureC != nil {
		temp = *u.temperatureC
	}
	var power float64
	if u.powerW != nil {
		power = *u.powerW
	}
	healthy := true
	if u.health != nil {
		healthy = *u.health
	}

	return NPUSample{
		ID:                         deviceID(s.nodeName, id),
		NodeName:                   s.nodeName,
		Model:                      model,
		AICore:                     aicore,
		MemoryUsedBytes:            used,
		MemoryTotalBytes:           total,
		HBMBandwidthBytesPerSecond: bw,
		TemperatureCelsius:         temp,
		PowerWatts:                 power,
		Healthy:                    healthy,
	}
}

// parseNPUList extracts NPU IDs from `npu-smi info -l` output. The list
// view enumerates devices with "NPU ID : <n>" lines; the parser is
// tolerant of the surrounding "Total Count" / "Chip Count" lines and any
// box-drawing. Falls back to an empty slice when no IDs are found (the
// caller then emits zero samples — a node with no visible NPUs).
func parseNPUList(out string) []int {
	seen := map[int]bool{}
	for _, line := range strings.Split(out, "\n") {
		key, val, ok := splitNPUSMIKV(line)
		if !ok {
			continue
		}
		if normNPUSMIKey(key) != "npu id" {
			continue
		}
		if n, err := strconv.Atoi(firstNPUSMIField(val)); err == nil {
			seen[n] = true
		}
	}
	ids := make([]int, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

// deviceID mirrors the simulator / realascend device-name convention
// ("<node>-npu-<index>") so the same physical NPU carries an identical
// npu_id label across demo / real profiles — keeps Grafana panels and
// PromQL joins profile-agnostic.
func deviceID(node string, index int) string {
	return node + "-npu-" + strconv.Itoa(index)
}

// normaliseChipName collapses npu-smi chip-name spellings ("Ascend 910B" /
// "910B") to the dashboard model label convention ("Ascend910B"). Leaves
// unrecognised names verbatim.
func normaliseChipName(name string) string {
	n := strings.TrimSpace(name)
	compact := strings.ReplaceAll(n, " ", "")
	if compact == "" {
		return n
	}
	if strings.HasPrefix(strings.ToLower(compact), "ascend") {
		return compact
	}
	if strings.HasPrefix(compact, "910") || strings.HasPrefix(compact, "310") {
		return "Ascend" + compact
	}
	return n
}
