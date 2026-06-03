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
	"errors"
	"strings"
	"testing"
)

// TestParseBoardInfo covers the P13-T-101 board parser against a captured
// `npu-smi info -t board -i 0` shape (ADR-0024 §2 Decision B). Key spelling
// variants (unit suffix, alternate names) are exercised to lock the
// tolerant-parser contract (ADR-0024 §3 fallback for lab output drift).
func TestParseBoardInfo(t *testing.T) {
	in := strings.Join([]string{
		"\tNPU ID                         : 0",
		"\tChip Count                     : 1",
		"\tChip Name                      : Ascend910B",
		"\tAicore Count                   : 32",
		"\tHBM Capacity(MB)               : 65536",
		"\tNUMA Node                      : 1",
		"\tHealth                         : OK",
		"\tSoftware Version               : 24.1.0",
		"\tFirmware Version               : 7.1.0.5",
	}, "\n")

	info, err := ParseBoardInfo(in, 0)
	if err != nil {
		t.Fatalf("ParseBoardInfo: %v", err)
	}
	if info.DeviceID != 0 {
		t.Fatalf("DeviceID = %d, want 0", info.DeviceID)
	}
	if info.ChipName != "Ascend910B" {
		t.Fatalf("ChipName = %q, want Ascend910B", info.ChipName)
	}
	if info.AICores != 32 {
		t.Fatalf("AICores = %d, want 32", info.AICores)
	}
	if info.MemorySizeMiB != 65536 {
		t.Fatalf("MemorySizeMiB = %d, want 65536 (unit suffix stripped)", info.MemorySizeMiB)
	}
	if info.NumaNode != 1 {
		t.Fatalf("NumaNode = %d, want 1", info.NumaNode)
	}
	if info.Health != HealthHealthy {
		t.Fatalf("Health = %q, want Healthy (from OK)", info.Health)
	}
	if info.DriverVersion != "24.1.0" {
		t.Fatalf("DriverVersion = %q, want 24.1.0", info.DriverVersion)
	}
	if info.FirmwareVersion != "7.1.0.5" {
		t.Fatalf("FirmwareVersion = %q, want 7.1.0.5", info.FirmwareVersion)
	}
}

// TestParseBoardInfoNoRecognisedKeys asserts ErrParse when the output has
// no recognised Key : Value lines (truncated / unexpected format → caller
// soft-falls per realascend.List capacity fallback).
func TestParseBoardInfoNoRecognisedKeys(t *testing.T) {
	_, err := ParseBoardInfo("+----------+\n| garbage  |\n+----------+\n", 3)
	if !errors.Is(err, ErrParse) {
		t.Fatalf("err = %v, want errors.Is(err, ErrParse)", err)
	}
}

// TestParseHealth covers both the "Health :" line form and the bare-token
// fallback form of npu-smi health output.
func TestParseHealth(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want HealthState
	}{
		{"kv ok", "\tHealth                         : OK", HealthHealthy},
		{"kv warning", "\tHealth Status                  : Warning", HealthUnhealthy},
		{"bare healthy", "NPU 0 is Healthy", HealthHealthy},
		{"bare alarm", "device 0: Alarm raised", HealthUnhealthy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseHealth(tc.in)
			if err != nil {
				t.Fatalf("ParseHealth(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("ParseHealth(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestParseHealthNoToken asserts ErrParse when no health token is present
// (so Source.Watch degrades to publisher-tick per ADR-0024 §2 Decision B).
func TestParseHealthNoToken(t *testing.T) {
	if _, err := ParseHealth("nothing useful here\n"); !errors.Is(err, ErrParse) {
		t.Fatalf("err = %v, want errors.Is(err, ErrParse)", err)
	}
}
