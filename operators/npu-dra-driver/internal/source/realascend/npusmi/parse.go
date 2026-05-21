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
	"sort"
	"strconv"
	"strings"
)

// ParseTopoMatrix parses the text output of `npu-smi info -t topo` into
// a per-device TopoEntry list with Ring populated via connected-components
// analysis of the HCCS edges. NumaNode + Health are populated from
// optional fixture-only sidecar comments (lines like
// "# numa: NPU0=0 NPU4=1" / "# health: NPU0=Healthy NPU3=Unhealthy");
// absent sidecars leave them at default (0 / Unknown).
//
// The Phase 7 W1 parser is intentionally narrow:
//
//   - Expects the upstream Huawei legend (X / HCCS / PIX / SYS)
//   - Builds an undirected graph where HCCS = edge
//   - Walks connected components to assign Ring IDs in deterministic
//     order (component containing the lowest-indexed device gets
//     Ring=0; next component Ring=1; etc.)
//   - Tolerant of leading/trailing blank lines + comment lines
//   - Strict on row count vs column count mismatch (returns ErrParse)
//
// Phase 7 T101 lab body adds:
//   - Multi-section parsing (some npu-smi versions interleave topo
//     with chip info; we don't need that fragmented input today)
//   - Locale-aware whitespace handling (kernel locale = en_US.UTF-8
//     by convention but lab envs vary)
//
// **References**:
//   - Huawei docs · `npu-smi info -t topo` output format + legend
//   - ADR-0011 §2 Source contract · QueryTopology returns rings
//   - parse_test.go covers 5 cases per phase7-plan §3 T005 acceptance
func ParseTopoMatrix(in string) ([]TopoEntry, error) {
	lines := strings.Split(in, "\n")

	// First pass: extract header row + body rows + optional sidecar
	// comments for numa/health.
	var headerCols []string // device labels from header (e.g. "NPU0".."NPU7")
	var bodyRows [][]string // per-row [rowLabel, cell0, cell1, ...]
	numaHints := map[int]int32{}
	healthHints := map[int]HealthState{}

	// Sidecar comment parser: matches "# numa: NPU0=0 NPU4=1" style.
	parseSidecar := func(line string) {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#") {
			return
		}
		body := strings.TrimSpace(strings.TrimPrefix(line, "#"))
		switch {
		case strings.HasPrefix(body, "numa:"):
			parseHintsInto(strings.TrimSpace(strings.TrimPrefix(body, "numa:")), func(k int, v string) {
				if n, err := strconv.Atoi(v); err == nil {
					numaHints[k] = int32(n)
				}
			})
		case strings.HasPrefix(body, "health:"):
			parseHintsInto(strings.TrimSpace(strings.TrimPrefix(body, "health:")), func(k int, v string) {
				healthHints[k] = HealthState(v)
			})
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			parseSidecar(trimmed)
			continue
		}
		if strings.HasPrefix(trimmed, "Legend") || strings.HasPrefix(trimmed, "X ") || strings.HasPrefix(trimmed, "SYS") || strings.HasPrefix(trimmed, "HCCS ") || strings.HasPrefix(trimmed, "PIX") {
			// Legend section after the matrix; ignore.
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		if isHeaderRow(fields) {
			headerCols = fields
			continue
		}
		bodyRows = append(bodyRows, fields)
	}

	if len(headerCols) == 0 {
		return nil, fmt.Errorf("%w: no header row (expected NPU0 NPU1 ...)", ErrParse)
	}
	if len(bodyRows) == 0 {
		return nil, fmt.Errorf("%w: no body rows", ErrParse)
	}

	n := len(headerCols)
	for i, row := range bodyRows {
		// Each body row should have rowLabel + N cells = N+1 fields.
		if len(row) != n+1 {
			return nil, fmt.Errorf("%w: row %d (%q) has %d fields, want %d (1 label + %d cells)",
				ErrParse, i, row[0], len(row), n+1, n)
		}
	}

	// Build connected components on HCCS edges.
	adj := make(map[int]map[int]bool, n)
	for i := 0; i < n; i++ {
		adj[i] = map[int]bool{}
	}
	for r, row := range bodyRows {
		// Skip self diag — cell at col r+1 is "X".
		for c := 1; c <= n; c++ {
			if c-1 == r {
				continue
			}
			if row[c] == "HCCS" {
				adj[r][c-1] = true
				adj[c-1][r] = true
			}
		}
	}

	// BFS to label connected components in order of lowest device id.
	ringOf := make(map[int]int32, n)
	nextRing := int32(0)
	for start := 0; start < n; start++ {
		if _, seen := ringOf[start]; seen {
			continue
		}
		// BFS from start.
		queue := []int{start}
		ringOf[start] = nextRing
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for nb := range adj[cur] {
				if _, seen := ringOf[nb]; seen {
					continue
				}
				ringOf[nb] = nextRing
				queue = append(queue, nb)
			}
		}
		nextRing++
	}

	// Compose TopoEntry list.
	out := make([]TopoEntry, 0, n)
	for i := 0; i < n; i++ {
		ent := TopoEntry{
			DeviceID: i,
			Ring:     ringOf[i],
			NumaNode: numaHints[i], // 0 default
			Health:   HealthUnknown,
		}
		if h, ok := healthHints[i]; ok {
			ent.Health = h
		}
		out = append(out, ent)
	}
	// Stable order by DeviceID asc (already by construction; sort for
	// defensiveness).
	sort.Slice(out, func(i, j int) bool { return out[i].DeviceID < out[j].DeviceID })

	return out, nil
}

// isHeaderRow returns true when the row consists entirely of "NPU\d+"
// tokens (the topo matrix header is a list of column labels).
func isHeaderRow(fields []string) bool {
	if len(fields) < 2 {
		return false
	}
	for _, f := range fields {
		if !strings.HasPrefix(f, "NPU") {
			return false
		}
		_, err := strconv.Atoi(strings.TrimPrefix(f, "NPU"))
		if err != nil {
			return false
		}
	}
	return true
}

// parseHintsInto parses "NPUk=v NPUm=v ..." style hints from sidecar
// comment lines and invokes the given setter for each parsed pair.
func parseHintsInto(s string, set func(devID int, value string)) {
	for _, tok := range strings.Fields(s) {
		eq := strings.Index(tok, "=")
		if eq <= 3 { // need at least "NPU0" before "="
			continue
		}
		head := tok[:eq]
		value := tok[eq+1:]
		if !strings.HasPrefix(head, "NPU") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(head, "NPU"))
		if err != nil {
			continue
		}
		set(n, value)
	}
}
