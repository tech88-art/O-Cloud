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
	"context"
	"errors"
	"strings"
	"testing"
)

// TestParseEmptyInput covers Phase 7 P7-T-005 acceptance case 1/5:
// an empty / whitespace-only input surfaces ErrParse (no header row).
func TestParseEmptyInput(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"empty string", ""},
		{"whitespace only", "  \n\n\t\n"},
		{"comments only", "# comment 1\n# comment 2\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseTopoMatrix(tc.in)
			if err == nil {
				t.Fatal("expected ErrParse, got nil")
			}
			if !errors.Is(err, ErrParse) {
				t.Fatalf("expected errors.Is(err, ErrParse), got %v", err)
			}
		})
	}
}

// TestParse8CardFixture covers Phase 7 P7-T-005 acceptance case 2/5:
// the 8-card 910B fixture decomposes into 2 HCCS rings of 4 NPUs each.
func TestParse8CardFixture(t *testing.T) {
	fake := &FakeClient{Fixture: "8card", NodeID: "test-node"}
	entries, err := fake.QueryTopo(context.Background())
	if err != nil {
		t.Fatalf("QueryTopo: %v", err)
	}
	if len(entries) != 8 {
		t.Fatalf("len(entries) = %d, want 8", len(entries))
	}
	// Ring 0 should be NPU0-3; Ring 1 should be NPU4-7.
	for i, e := range entries {
		if e.DeviceID != i {
			t.Fatalf("entry[%d].DeviceID = %d, want %d", i, e.DeviceID, i)
		}
		wantRing := int32(0)
		if i >= 4 {
			wantRing = 1
		}
		if e.Ring != wantRing {
			t.Fatalf("entry[%d].Ring = %d, want %d (NPU%d should be ring %d)", i, e.Ring, wantRing, i, wantRing)
		}
		if e.NodeID != "test-node" {
			t.Fatalf("entry[%d].NodeID = %q, want \"test-node\"", i, e.NodeID)
		}
	}
	// NUMA sidecar hints: NPU0-3 → numa 0; NPU4-7 → numa 1.
	for i, e := range entries {
		wantNuma := int32(0)
		if i >= 4 {
			wantNuma = 1
		}
		if e.NumaNode != wantNuma {
			t.Fatalf("entry[%d].NumaNode = %d, want %d (from sidecar)", i, e.NumaNode, wantNuma)
		}
	}
}

// TestParse16CardFixture covers Phase 7 P7-T-005 acceptance case 3/5:
// the 16-card 910B-pro fixture decomposes into 4 HCCS rings.
func TestParse16CardFixture(t *testing.T) {
	fake := &FakeClient{Fixture: "16card", NodeID: "test-node-pro"}
	entries, err := fake.QueryTopo(context.Background())
	if err != nil {
		t.Fatalf("QueryTopo: %v", err)
	}
	if len(entries) != 16 {
		t.Fatalf("len(entries) = %d, want 16", len(entries))
	}
	// Ring assignment: 0..3 → ring 0; 4..7 → ring 1; 8..11 → ring 2; 12..15 → ring 3
	for i, e := range entries {
		wantRing := int32(i / 4)
		if e.Ring != wantRing {
			t.Fatalf("entry[%d].Ring = %d, want %d (NPU%d should be ring %d)", i, e.Ring, wantRing, i, wantRing)
		}
	}
}

// TestParseUnhealthyHint covers Phase 7 P7-T-005 acceptance case 4/5:
// per-device Health hints from sidecar are honored; default is
// HealthUnknown for devices not listed.
func TestParseUnhealthyHint(t *testing.T) {
	// Build a minimal 2-device topo + health sidecar (one unhealthy).
	in := strings.Join([]string{
		"# health: NPU0=Healthy NPU1=Unhealthy",
		"       NPU0   NPU1",
		"NPU0   X      HCCS",
		"NPU1   HCCS   X",
	}, "\n")
	entries, err := ParseTopoMatrix(in)
	if err != nil {
		t.Fatalf("ParseTopoMatrix: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].Health != HealthHealthy {
		t.Fatalf("entries[0].Health = %q, want HealthHealthy (from sidecar)", entries[0].Health)
	}
	if entries[1].Health != HealthUnhealthy {
		t.Fatalf("entries[1].Health = %q, want HealthUnhealthy (from sidecar)", entries[1].Health)
	}
	// Both should be in the same ring (HCCS edge).
	if entries[0].Ring != entries[1].Ring {
		t.Fatalf("ring mismatch: %d vs %d (HCCS-connected pair should share ring)",
			entries[0].Ring, entries[1].Ring)
	}
}

// TestParseMalformedRow covers Phase 7 P7-T-005 acceptance case 5/5:
// row count vs column count mismatch surfaces ErrParse (caller can
// retry or fail-fast — Phase 7 T101 retries once before surfacing).
func TestParseMalformedRow(t *testing.T) {
	// Header declares 3 NPUs but row only provides 2 cells (truncated).
	in := strings.Join([]string{
		"       NPU0   NPU1   NPU2",
		"NPU0   X      HCCS", // missing NPU2 cell
		"NPU1   HCCS   X      PIX",
		"NPU2   PIX    PIX    X",
	}, "\n")
	_, err := ParseTopoMatrix(in)
	if err == nil {
		t.Fatal("expected ErrParse on truncated row, got nil")
	}
	if !errors.Is(err, ErrParse) {
		t.Fatalf("expected errors.Is(err, ErrParse), got %v", err)
	}
}
