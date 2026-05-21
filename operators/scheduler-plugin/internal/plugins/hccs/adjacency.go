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
	"fmt"
	"strconv"
)

// DefaultAdjacency910B8Card returns the Phase 7 P7-T-008 default
// HCCS-ring adjacency map for a typical 910B 8-card server per
// ADR-0010 §256 + ADR-0011 expected topology shape `0↔1↔2↔3↔0`.
//
// The map describes "which other rings are 1-hop adjacent to a given
// ring" — Score uses this to grade nodes 100 (same ring) / 70
// (adjacent ring) / 30 (disjoint ring) / 0 (no eligible device).
//
// Topology:
//
//	   ring 0 — ring 1
//	     |       |
//	   ring 3 — ring 2
//
// Each ring has exactly 2 neighbors (closed loop). Operators with a
// different physical topology override via the chart's
// `hccsTopology.adjacency` value (chart's values.yaml carries this
// map as the rendered KubeSchedulerConfiguration plugin args default
// — see deploy/helm-charts/scheduler-plugin/values.yaml).
//
// Returns a fresh map on every call so callers (chart-rendered config
// + Go tests) can mutate without polluting subsequent calls.
//
// **References**:
//   - ADR-0010 §256 risk row (HCCS Adjacency map empty default · Phase
//     7 T008 ships the 910B 8-card default)
//   - ADR-0011 (Phase 7 ships fallback path + 8-card adjacency as
//     chart default)
//   - phase7-plan.md §3 P7-T-008 (this task)
func DefaultAdjacency910B8Card() map[string][]int32 {
	return map[string][]int32{
		"0": {1, 3},
		"1": {0, 2},
		"2": {1, 3},
		"3": {2, 0},
	}
}

// BuildAdjacency validates + parses an adjacency spec (string-keyed
// for JSON-encodability in KubeSchedulerConfiguration plugin args)
// into a int32-keyed map for runtime use.
//
// Phase 7 P7-T-008 introduces this as the public counterpart to the
// existing internal buildAdjacency helper (which only handles the
// runtime-form conversion). BuildAdjacency adds:
//
//   - Key validation: every key must parse as a non-negative int32
//   - Value validation: every value must be a non-negative int32
//     (kubebuilder int32 type already enforces this; BuildAdjacency
//     defensively re-validates)
//   - Self-loop rejection (a ring listed as adjacent to itself is a
//     misconfiguration — Score's 100 tier already handles same-ring)
//
// Returns ErrMalformedAdjacency-wrapped error on any violation.
// Callers (chart pre-flight checks + Phase 7 T008 unit tests) use
// this to fail-fast at config-time rather than at first scheduling
// attempt.
func BuildAdjacency(spec map[string][]int32) (map[int32][]int32, error) {
	if len(spec) == 0 {
		return nil, nil
	}
	out := make(map[int32][]int32, len(spec))
	for kStr, vs := range spec {
		k64, err := strconv.ParseInt(kStr, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("%w: key %q is not an int32: %v",
				ErrMalformedAdjacency, kStr, err)
		}
		if k64 < 0 {
			return nil, fmt.Errorf("%w: key %q is negative", ErrMalformedAdjacency, kStr)
		}
		k := int32(k64)
		dedup := make(map[int32]struct{}, len(vs))
		validated := make([]int32, 0, len(vs))
		for _, v := range vs {
			if v < 0 {
				return nil, fmt.Errorf("%w: value %d under key %q is negative",
					ErrMalformedAdjacency, v, kStr)
			}
			if v == k {
				return nil, fmt.Errorf("%w: self-loop (ring %d listed as adjacent to itself)",
					ErrMalformedAdjacency, k)
			}
			if _, seen := dedup[v]; seen {
				continue
			}
			dedup[v] = struct{}{}
			validated = append(validated, v)
		}
		out[k] = validated
	}
	return out, nil
}

// ErrMalformedAdjacency is the sentinel returned by BuildAdjacency
// when the spec violates structural invariants. Callers can
// errors.Is(err, ErrMalformedAdjacency) to discriminate from other
// scheduler config errors.
var ErrMalformedAdjacency = fmt.Errorf("hccs: malformed adjacency spec")
