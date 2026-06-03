package sources

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cannedRunner is a test npuSMIRunner that returns fixture output keyed by
// the joined args, or a per-key error. It captures real `npu-smi info`
// shapes so the source body runs deterministically without a lab host
// (ADR-0024 §3 captured-fixture verification · plan §8 offline layer).
type cannedRunner struct {
	out  map[string]string
	errs map[string]error
}

func (c *cannedRunner) run(_ context.Context, args ...string) (string, error) {
	key := strings.Join(args, " ")
	if c.errs != nil {
		if err, ok := c.errs[key]; ok {
			return "", err
		}
	}
	if c.out != nil {
		if v, ok := c.out[key]; ok {
			return v, nil
		}
	}
	return "", errors.New("cannedRunner: no fixture for " + key)
}

// --- captured npu-smi fixtures (sanitized · 8-card 910B host) ----------

// npuListFixture8Card is `npu-smi info -l` for an 8-card host.
const npuListFixture8Card = `	Total Count                    : 8
	NPU ID                         : 0
	Chip Count                     : 1
	NPU ID                         : 1
	Chip Count                     : 1
	NPU ID                         : 2
	Chip Count                     : 1
	NPU ID                         : 3
	Chip Count                     : 1
	NPU ID                         : 4
	Chip Count                     : 1
	NPU ID                         : 5
	Chip Count                     : 1
	NPU ID                         : 6
	Chip Count                     : 1
	NPU ID                         : 7
	Chip Count                     : 1
`

// commonFixture is `npu-smi info -t common -i <id>`: temperature / power /
// health / device memory.
func commonFixture(temp, power int, health string, memUsedMiB, memTotalMiB int) string {
	return strings.Join([]string{
		"\tNPU ID                         : 0",
		"\tChip Name                      : Ascend 910B",
		"\tHealth                         : " + health,
		"\tTemperature(C)                 : " + strconv.Itoa(temp),
		"\tPower(W)                       : " + strconv.Itoa(power),
		"\tMemory Used(MB)                : " + strconv.Itoa(memUsedMiB),
		"\tMemory Capacity(MB)            : " + strconv.Itoa(memTotalMiB),
	}, "\n")
}

// usagesFixture is `npu-smi info -t usages -i <id>`: aicore utilization% /
// HBM used / HBM bandwidth.
func usagesFixture(aicorePct, hbmUsedMiB, hbmTotalMiB, hbmBWMiBps int) string {
	return strings.Join([]string{
		"\tNPU ID                         : 0",
		"\tAICore Usage Rate(%)           : " + strconv.Itoa(aicorePct),
		"\tHBM Usage Rate(%)              : 50",
		"\tHBM Used(MB)                   : " + strconv.Itoa(hbmUsedMiB),
		"\tHBM Capacity(MB)               : " + strconv.Itoa(hbmTotalMiB),
		"\tHBM Bandwidth(MB)              : " + strconv.Itoa(hbmBWMiBps),
	}, "\n")
}

// boardFixture is `npu-smi info -t board -i <id>`: chip name + AI-cores.
const boardFixture = `	NPU ID                         : 0
	Chip Name                      : Ascend910B
	Aicore Count                   : 32
	Firmware Version               : 7.1.0.5
`

// runnerFor8Cards builds a cannedRunner that answers -l + the three
// per-device views for all 8 devices with the same per-device fixtures
// (varying only by id where it matters for the assertions below).
func runnerFor8Cards() *cannedRunner {
	out := map[string]string{
		"info -l": npuListFixture8Card,
	}
	for id := 0; id < 8; id++ {
		i := strconv.Itoa(id)
		out["info -t common -i "+i] = commonFixture(55+id, 280+id, "OK", 8192+id*1024, 65536)
		out["info -t usages -i "+i] = usagesFixture(40+id, 16384+id*1024, 65536, 400000)
		out["info -t board -i "+i] = boardFixture
	}
	return &cannedRunner{out: out}
}

// TestNPUSMISource_ReadNPUs_8Card asserts the full read path: enumerate 8
// devices, merge the three views, and compose 8 NPUSamples with the
// expected fields + MiB→bytes conversion.
func TestNPUSMISource_ReadNPUs_8Card(t *testing.T) {
	src := newNPUSMISourceWithRunner(runnerFor8Cards(), "worker-a", "Ascend910B")

	samples, err := src.ReadNPUs(context.Background())
	require.NoError(t, err)
	require.Len(t, samples, 8)

	// Device 0 spot-check.
	s0 := samples[0]
	assert.Equal(t, "worker-a-npu-0", s0.ID)
	assert.Equal(t, "worker-a", s0.NodeName)
	assert.Equal(t, "Ascend910B", s0.Model) // "Ascend 910B" normalised
	assert.InDelta(t, 40.0, s0.AICore, 0.001)
	// HBM figures preferred over generic memory: used = 16384 MiB.
	assert.Equal(t, uint64(16384)*1024*1024, s0.MemoryUsedBytes)
	assert.Equal(t, uint64(65536)*1024*1024, s0.MemoryTotalBytes)
	assert.Equal(t, uint64(400000)*1024*1024, s0.HBMBandwidthBytesPerSecond)
	assert.InDelta(t, 55.0, s0.TemperatureCelsius, 0.001)
	assert.InDelta(t, 280.0, s0.PowerWatts, 0.001)
	assert.True(t, s0.Healthy)

	// Device 7 carries the per-id offsets (id-scaled fixture).
	s7 := samples[7]
	assert.Equal(t, "worker-a-npu-7", s7.ID)
	assert.InDelta(t, 47.0, s7.AICore, 0.001)
	assert.InDelta(t, 62.0, s7.TemperatureCelsius, 0.001)

	// Samples are ordered by device index (parseNPUList sorts).
	for i := 0; i < 8; i++ {
		assert.Equal(t, "worker-a-npu-"+strconv.Itoa(i), samples[i].ID)
	}
}

// TestNPUSMISource_ReadNPUs_EnumerationError asserts an `info -l` failure
// surfaces (mis-targeted node fails loudly, no fabricated samples).
func TestNPUSMISource_ReadNPUs_EnumerationError(t *testing.T) {
	src := newNPUSMISourceWithRunner(&cannedRunner{
		errs: map[string]error{"info -l": ErrNoCommand},
	}, "worker-a", "Ascend910B")
	_, err := src.ReadNPUs(context.Background())
	require.ErrorIs(t, err, ErrNoCommand)
}

// TestNPUSMISource_ReadNPUs_PerDeviceSoftFallback asserts a per-device
// view failure is soft: the device is still emitted with model defaults
// for the missing fields (ADR-0024 §3 single-point fallback).
func TestNPUSMISource_ReadNPUs_PerDeviceSoftFallback(t *testing.T) {
	out := map[string]string{
		"info -l": "\tNPU ID                         : 0\n\tNPU ID                         : 1\n",
		// Device 0 fully answered.
		"info -t common -i 0": commonFixture(60, 300, "OK", 8192, 65536),
		"info -t usages -i 0": usagesFixture(42, 16384, 65536, 400000),
		"info -t board -i 0":  boardFixture,
		// Device 1: only board answers; common + usages error.
		"info -t board -i 1": boardFixture,
	}
	errs := map[string]error{
		"info -t common -i 1": errors.New("device 1 hot-reset"),
		"info -t usages -i 1": errors.New("device 1 hot-reset"),
	}
	src := newNPUSMISourceWithRunner(&cannedRunner{out: out, errs: errs}, "worker-a", "Ascend910B")

	samples, err := src.ReadNPUs(context.Background())
	require.NoError(t, err)
	require.Len(t, samples, 2)

	// Device 1: missing telemetry → defaults (util 0, model default total,
	// healthy true), but the device is still present (not dropped).
	s1 := samples[1]
	assert.Equal(t, "worker-a-npu-1", s1.ID)
	assert.Equal(t, 0.0, s1.AICore)
	assert.Equal(t, defaultNPUMemoryTotalBytes, s1.MemoryTotalBytes)
	assert.True(t, s1.Healthy)
	assert.Equal(t, "Ascend910B", s1.Model) // from board fixture
}

// TestNPUSMISource_Unhealthy asserts a degraded health token maps to
// Healthy=false.
func TestNPUSMISource_Unhealthy(t *testing.T) {
	out := map[string]string{
		"info -l":             "\tNPU ID                         : 0\n",
		"info -t common -i 0": commonFixture(90, 350, "Warning", 60000, 65536),
		"info -t usages -i 0": usagesFixture(95, 60000, 65536, 380000),
		"info -t board -i 0":  boardFixture,
	}
	src := newNPUSMISourceWithRunner(&cannedRunner{out: out}, "worker-a", "Ascend910B")
	samples, err := src.ReadNPUs(context.Background())
	require.NoError(t, err)
	require.Len(t, samples, 1)
	assert.False(t, samples[0].Healthy)
	// Used clamped to total (60000 < 65536, no clamp here) and util clamped.
	assert.InDelta(t, 95.0, samples[0].AICore, 0.001)
}

// TestNPUSMISource_NoDevices asserts an empty `info -l` yields zero
// samples without error (a node with no visible NPUs).
func TestNPUSMISource_NoDevices(t *testing.T) {
	src := newNPUSMISourceWithRunner(&cannedRunner{
		out: map[string]string{"info -l": "\tTotal Count                    : 0\n"},
	}, "worker-a", "Ascend910B")
	samples, err := src.ReadNPUs(context.Background())
	require.NoError(t, err)
	assert.Empty(t, samples)
}

// TestParseNPUList covers the device-enumeration parser against the
// captured 8-card fixture + a dedup case.
func TestParseNPUList(t *testing.T) {
	ids := parseNPUList(npuListFixture8Card)
	assert.Equal(t, []int{0, 1, 2, 3, 4, 5, 6, 7}, ids)

	// Duplicate NPU ID lines (some drivers repeat in chip blocks) dedup.
	dup := "\tNPU ID : 0\n\tNPU ID : 0\n\tNPU ID : 2\n"
	assert.Equal(t, []int{0, 2}, parseNPUList(dup))

	// No NPU ID lines → empty.
	assert.Empty(t, parseNPUList("garbage\n+----+\n"))
}

// TestNormaliseChipName locks the chip-name → model-label normalisation.
func TestNormaliseChipName(t *testing.T) {
	cases := map[string]string{
		"Ascend 910B": "Ascend910B",
		"Ascend910B":  "Ascend910B",
		"910B":        "Ascend910B",
		"310P":        "Ascend310P",
		"":            "",
	}
	for in, want := range cases {
		assert.Equal(t, want, normaliseChipName(in), "normaliseChipName(%q)", in)
	}
}
