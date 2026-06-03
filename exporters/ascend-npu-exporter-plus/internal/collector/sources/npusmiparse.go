package sources

import (
	"strconv"
	"strings"
)

// npusmiparse.go — tolerant text parser for `npu-smi info` views.
//
// P13-T-102 · ADR-0024 §2 Decision D. The exporter is a standalone Go
// module (exporters/CLAUDE.md §3.1: no client-go, no cross-module import),
// so it cannot reuse operators/npu-dra-driver's npusmi package. The
// `Key : Value` parsing approach is mirrored here (T101 board.go is the
// sibling reference): split on the first ":", normalise the key
// (lower-case + collapse whitespace + strip a trailing unit suffix in
// parentheses), and match a curated alias set.
//
// **No cgo** (ADR-0024 §4(c)): every real telemetry field this exporter
// emits is reachable from `npu-smi` stdout via os/exec — there is no
// libdcmi.so binding here, so the default CGO_ENABLED=0 GOARCH=arm64
// cross-compile (ADR-0020) stays green without a build tag.
//
// **Tolerance / lab drift** (ADR-0024 §3 single-point fallback): unknown
// keys are ignored and absent keys leave the accumulator field untouched,
// so a lab driver that spells a key differently extends the alias set here
// + a npu_smi_test.go case rather than breaking the whole scrape.

// npuUsage is the partial NPUSample accumulated from the npu-smi views.
// Pointers distinguish "field absent in this view" (nil → keep prior /
// model default) from "field present and zero".
type npuUsage struct {
	chipName     string
	aicorePct    *float64
	memUsedMiB   *uint64
	memTotalMiB  *uint64
	hbmUsedMiB   *uint64
	hbmTotalMiB  *uint64
	hbmBWMiBps   *uint64
	temperatureC *float64
	powerW       *float64
	health       *bool
}

// mergeNPUUsage folds src into dst, with src taking precedence for any
// field src populated (so a later view — e.g. `-t usages` after
// `-t common` — refines earlier values). Used to compose one NPUSample
// from multiple npu-smi sub-views.
func mergeNPUUsage(dst, src *npuUsage) {
	if src.chipName != "" {
		dst.chipName = src.chipName
	}
	if src.aicorePct != nil {
		dst.aicorePct = src.aicorePct
	}
	if src.memUsedMiB != nil {
		dst.memUsedMiB = src.memUsedMiB
	}
	if src.memTotalMiB != nil {
		dst.memTotalMiB = src.memTotalMiB
	}
	if src.hbmUsedMiB != nil {
		dst.hbmUsedMiB = src.hbmUsedMiB
	}
	if src.hbmTotalMiB != nil {
		dst.hbmTotalMiB = src.hbmTotalMiB
	}
	if src.hbmBWMiBps != nil {
		dst.hbmBWMiBps = src.hbmBWMiBps
	}
	if src.temperatureC != nil {
		dst.temperatureC = src.temperatureC
	}
	if src.powerW != nil {
		dst.powerW = src.powerW
	}
	if src.health != nil {
		dst.health = src.health
	}
}

// parseNPUSMIDevice parses the `Key : Value` text of one npu-smi device
// view (`-t common`, `-t usages`, `-t board`, or the default `-i <id>`
// dump) into an npuUsage. Returns ok=false when no recognised key was
// seen (truncated output / unexpected format) so the caller can soft-fall
// to the model default for that device without poisoning the whole node.
func parseNPUSMIDevice(out string) (npuUsage, bool) {
	var u npuUsage
	any := false

	for _, line := range strings.Split(out, "\n") {
		key, val, ok := splitNPUSMIKV(line)
		if !ok {
			continue
		}
		switch normNPUSMIKey(key) {
		case "chip name", "name", "chip type":
			u.chipName = val
			any = true
		case "aicore usage rate", "aicore usage", "aicore rate", "ai core usage rate", "npu real-time utilization", "aicore utilization":
			if f, ok := parsePercent(val); ok {
				u.aicorePct = &f
				any = true
			}
		case "memory usage rate", "ddr usage rate":
			// Some drivers report only a percentage; fold into mem used
			// vs total below if the absolute bytes views are absent. We
			// keep the percentage out of NPUSample (which is bytes-based)
			// and rely on the absolute Memory/HBM lines; mark "any" so a
			// usages-only device is not treated as unparseable.
			if _, ok := parsePercent(val); ok {
				any = true
			}
		case "memory used", "ddr memory used", "ddr used":
			if v, ok := parseMiB(val); ok {
				u.memUsedMiB = &v
				any = true
			}
		case "memory capacity", "memory size", "ddr capacity", "ddr memory size":
			if v, ok := parseMiB(val); ok {
				u.memTotalMiB = &v
				any = true
			}
		case "hbm used", "hbm memory used", "hbm usage":
			if v, ok := parseMiB(val); ok {
				u.hbmUsedMiB = &v
				any = true
			}
		case "hbm capacity", "hbm size", "hbm memory size":
			if v, ok := parseMiB(val); ok {
				u.hbmTotalMiB = &v
				any = true
			}
		case "hbm bandwidth", "hbm read bandwidth", "hbm bw":
			if v, ok := parseMiB(val); ok {
				u.hbmBWMiBps = &v
				any = true
			}
		case "temperature", "temp":
			if f, ok := parseFloatField(val); ok {
				u.temperatureC = &f
				any = true
			}
		case "power", "chip power", "npu power":
			if f, ok := parseFloatField(val); ok {
				u.powerW = &f
				any = true
			}
		case "health", "health status":
			h := healthFromNPUSMI(val)
			u.health = &h
			any = true
		}
	}
	return u, any
}

// splitNPUSMIKV splits a "Key : Value" line on the first ":" and trims
// both sides. Returns ok=false for blank lines, box-drawing borders
// (+---+, ===), and comment lines.
func splitNPUSMIKV(line string) (key, val string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "+") || strings.HasPrefix(line, "=") {
		return "", "", false
	}
	i := strings.Index(line, ":")
	if i < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:]), true
}

// normNPUSMIKey lower-cases a key, strips a trailing unit suffix in
// parentheses (e.g. "Memory Used(MB)" → "memory used"), and collapses
// internal whitespace to single spaces.
func normNPUSMIKey(k string) string {
	if p := strings.IndexByte(k, '('); p >= 0 {
		k = k[:p]
	}
	return strings.Join(strings.Fields(strings.ToLower(k)), " ")
}

// firstNPUSMIField returns the first whitespace-delimited token of a
// value, dropping a trailing unit ("65536 MB" → "65536").
func firstNPUSMIField(v string) string {
	f := strings.Fields(v)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// parsePercent parses a utilization value in [0,100]. Tolerates a
// trailing "%" and a unit token ("42 %" / "42%").
func parsePercent(v string) (float64, bool) {
	s := strings.TrimSuffix(firstNPUSMIField(v), "%")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// parseFloatField parses the first numeric token of a value (drops unit
// suffix). Used for temperature / power.
func parseFloatField(v string) (float64, bool) {
	f, err := strconv.ParseFloat(firstNPUSMIField(v), 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// parseMiB parses an integer MiB value (npu-smi memory/HBM fields report
// MB == MiB by Ascend convention). Drops a unit suffix.
func parseMiB(v string) (uint64, bool) {
	n, err := strconv.ParseInt(firstNPUSMIField(v), 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return uint64(n), true
}

// healthFromNPUSMI maps an npu-smi health value to the Healthy bool.
// npu-smi reports OK / Healthy / Normal for good; Warning / Alarm /
// Error / Unhealthy / Abnormal for degraded.
func healthFromNPUSMI(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "ok", "healthy", "normal", "0":
		return true
	default:
		return false
	}
}

// mibToBytes converts a MiB count to bytes.
func mibToBytes(mib uint64) uint64 { return mib * 1024 * 1024 }
