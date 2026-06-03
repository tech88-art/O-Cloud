/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package npusmi

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseBoardInfo parses the text output of `npu-smi info -t board -i <id>`
// (and the adjacent `-t common` / `-t health` views, which share the same
// "Key : Value" line shape) into a DeviceInfo.
//
// The Ascend npu-smi board view is a flat list of `Key : Value` lines, e.g.
//
//	NPU ID                         : 0
//	Chip Name                      : Ascend910B
//	Aicore Count                   : 32
//	HBM Capacity(MB)               : 65536
//	NUMA Node                      : 0
//	Health                         : OK
//	Software Version               : 24.1.0
//	Firmware Version               : 7.1.0.5
//
// The parser is intentionally tolerant (P13-T-101 · ADR-0024 §3 真硬件集成
// fallback): it splits on the first ":" per line, normalises the key
// (lower-case + collapsed whitespace + stripped unit suffix), and matches
// a curated set of aliases. Unknown keys are ignored; absent keys leave
// DeviceInfo fields at their zero value. A lab driver that emits a variant
// spelling extends the alias sets here + a board_test.go case (ADR-0024
// §3 single-point fallback: deliver body + captured-fixture test + extend
// parser on real-output drift).
func ParseBoardInfo(out string, devID int) (*DeviceInfo, error) {
	info := &DeviceInfo{DeviceID: devID, Health: HealthUnknown}
	any := false

	for _, line := range strings.Split(out, "\n") {
		key, val, ok := splitKV(line)
		if !ok {
			continue
		}
		switch normKey(key) {
		case "chip name", "name", "chip type":
			info.ChipName = val
			any = true
		case "aicore count", "ai core count", "aicore", "aicore num":
			if n, err := strconv.Atoi(firstField(val)); err == nil {
				info.AICores = int32(n)
				any = true
			}
		case "hbm capacity", "memory size", "memory capacity", "hbm size", "ddr capacity":
			if n, err := strconv.ParseInt(firstField(val), 10, 64); err == nil {
				info.MemorySizeMiB = n
				any = true
			}
		case "numa node", "numa", "numa id":
			if n, err := strconv.Atoi(firstField(val)); err == nil {
				info.NumaNode = int32(n)
				any = true
			}
		case "health", "health status":
			info.Health = healthFromText(val)
			any = true
		case "software version", "driver version", "cann version":
			info.DriverVersion = val
			any = true
		case "firmware version":
			info.FirmwareVersion = val
			any = true
		}
	}

	if !any {
		return nil, fmt.Errorf("%w: board info for device %d had no recognised Key : Value lines", ErrParse, devID)
	}
	return info, nil
}

// ParseHealth extracts a device health state from npu-smi output. It reuses
// the board "Key : Value" shape (looks for a Health / Health Status line)
// and falls back to scanning the raw text for a known health token so the
// cheap `npu-smi info -t health` view (which may print just the state)
// still resolves.
func ParseHealth(out string) (HealthState, error) {
	for _, line := range strings.Split(out, "\n") {
		key, val, ok := splitKV(line)
		if !ok {
			continue
		}
		switch normKey(key) {
		case "health", "health status":
			return healthFromText(val), nil
		}
	}
	// No "Health :" line — scan for a bare token (some -t health views
	// print only the state).
	switch {
	case containsFold(out, "Healthy"), containsFold(out, "OK"):
		return HealthHealthy, nil
	case containsFold(out, "Unhealthy"), containsFold(out, "Warning"),
		containsFold(out, "Alarm"), containsFold(out, "Error"):
		return HealthUnhealthy, nil
	}
	return HealthUnknown, fmt.Errorf("%w: no health token in output", ErrParse)
}

// healthFromText maps an npu-smi health value to the HealthState enum.
// npu-smi reports OK / Healthy for good, and Warning / Alarm / Error /
// Unhealthy for degraded; anything else is Unknown.
func healthFromText(v string) HealthState {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "ok", "healthy", "normal", "0":
		return HealthHealthy
	case "warning", "alarm", "error", "unhealthy", "abnormal":
		return HealthUnhealthy
	default:
		return HealthUnknown
	}
}

// splitKV splits a "Key : Value" line on the first ":" and trims both
// sides. Returns ok=false for blank lines, comment lines, and lines with
// no ":".
func splitKV(line string) (key, val string, ok bool) {
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

// normKey lower-cases a key and strips a trailing unit suffix in
// parentheses (e.g. "HBM Capacity(MB)" → "hbm capacity") plus collapses
// internal runs of whitespace to single spaces.
func normKey(k string) string {
	if p := strings.IndexByte(k, '('); p >= 0 {
		k = k[:p]
	}
	return strings.Join(strings.Fields(strings.ToLower(k)), " ")
}

// firstField returns the first whitespace-delimited token of a value,
// dropping any trailing unit ("65536 MB" → "65536").
func firstField(v string) string {
	f := strings.Fields(v)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// containsFold is a case-insensitive substring test.
func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
