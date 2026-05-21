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

package hccs

import (
	"errors"
	"sort"
	"testing"
)

// TestDefaultAdjacency910B8CardRingClosure covers Phase 7 P7-T-008
// acceptance case 1/4: the default 8-card 910B adjacency map is a
// closed ring `0↔1↔2↔3↔0` per ADR-0010 §256.
func TestDefaultAdjacency910B8CardRingClosure(t *testing.T) {
	def := DefaultAdjacency910B8Card()
	if len(def) != 4 {
		t.Fatalf("DefaultAdjacency910B8Card() has %d rings, want 4 (0/1/2/3)", len(def))
	}
	// Each ring has exactly 2 neighbors.
	for ring, neighbors := range def {
		if len(neighbors) != 2 {
			t.Fatalf("ring %q has %d neighbors, want 2 (closed ring)", ring, len(neighbors))
		}
	}
	// Verify the exact shape — Score tier 70 depends on this.
	expected := map[string][]int32{
		"0": {1, 3},
		"1": {0, 2},
		"2": {1, 3},
		"3": {2, 0},
	}
	for ring, want := range expected {
		got := def[ring]
		gotSorted := append([]int32(nil), got...)
		wantSorted := append([]int32(nil), want...)
		sort.Slice(gotSorted, func(i, j int) bool { return gotSorted[i] < gotSorted[j] })
		sort.Slice(wantSorted, func(i, j int) bool { return wantSorted[i] < wantSorted[j] })
		if len(gotSorted) != len(wantSorted) {
			t.Fatalf("ring %q neighbors len mismatch: got %v, want %v", ring, gotSorted, wantSorted)
		}
		for i := range gotSorted {
			if gotSorted[i] != wantSorted[i] {
				t.Fatalf("ring %q neighbors mismatch: got %v, want %v", ring, gotSorted, wantSorted)
			}
		}
	}
}

// TestBuildAdjacencyCustomMapParse covers Phase 7 P7-T-008 acceptance
// case 2/4: BuildAdjacency parses a well-formed custom map (e.g. an
// operator-specific override) into int32-keyed form, deduplicating
// repeated values within a row.
func TestBuildAdjacencyCustomMapParse(t *testing.T) {
	in := map[string][]int32{
		"0": {1, 2, 2, 3}, // dup 2 → deduplicated
		"5": {6},
	}
	out, err := BuildAdjacency(in)
	if err != nil {
		t.Fatalf("BuildAdjacency: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len(out) = %d, want 2", len(out))
	}
	if got := out[0]; len(got) != 3 {
		t.Fatalf("out[0] = %v, want 3 elements (dup 2 deduplicated)", got)
	}
	if got := out[5]; len(got) != 1 || got[0] != 6 {
		t.Fatalf("out[5] = %v, want [6]", got)
	}
}

// TestBuildAdjacencyEmptyMapNoAdjacency covers Phase 7 P7-T-008
// acceptance case 3/4: empty input → nil output (Score falls back to
// binary 100/30 grading without the 70 tier).
func TestBuildAdjacencyEmptyMapNoAdjacency(t *testing.T) {
	cases := []struct {
		name string
		in   map[string][]int32
	}{
		{name: "nil input", in: nil},
		{name: "empty map", in: map[string][]int32{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := BuildAdjacency(tc.in)
			if err != nil {
				t.Fatalf("BuildAdjacency: %v", err)
			}
			if out != nil {
				t.Fatalf("out = %+v, want nil for empty input (Score binary 100/30 fallback)", out)
			}
		})
	}
}

// TestBuildAdjacencyMalformedRejects covers Phase 7 P7-T-008
// acceptance case 4/4: malformed inputs (non-int key, negative key,
// negative value, self-loop) surface ErrMalformedAdjacency.
func TestBuildAdjacencyMalformedRejects(t *testing.T) {
	cases := []struct {
		name string
		in   map[string][]int32
	}{
		{name: "non-int key", in: map[string][]int32{"abc": {1}}},
		{name: "negative key", in: map[string][]int32{"-1": {0}}},
		{name: "negative value", in: map[string][]int32{"0": {-1}}},
		{name: "self-loop", in: map[string][]int32{"3": {3, 1}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildAdjacency(tc.in)
			if err == nil {
				t.Fatal("expected ErrMalformedAdjacency, got nil")
			}
			if !errors.Is(err, ErrMalformedAdjacency) {
				t.Fatalf("expected errors.Is(err, ErrMalformedAdjacency), got %v", err)
			}
		})
	}
}
