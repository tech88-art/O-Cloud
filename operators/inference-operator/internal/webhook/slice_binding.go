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

package webhook

import (
	"fmt"
	"sort"
	"strings"
)

// SliceBinding is one entry in the npu.huawei.com/slice-bindings
// annotation value (T103). It mirrors a NPUSliceAllocation row
// summarised down to what a downstream consumer (PD Router proxy,
// Phase 6 scheduler-plugin) cares about for routing decisions.
type SliceBinding struct {
	Node    string
	Pool    string
	Device  string
	AICores int32
	// Phase indicates the underlying NPUSliceAllocation's lifecycle
	// state (Allocated / Released / Orphaned). The encoder filters
	// out non-Allocated entries by default.
	Phase string
}

// EncodeBindings returns the annotation value for a list of bindings.
// Format: comma-separated `<node>/<pool>/<device>:<aiCores>` entries
// in lexicographic order by (node, pool, device) for determinism
// (consumers diffing the annotation across reconciles see stable
// output for stable input). Phase 5 simulator convention: node==pool,
// so most users will observe `<node>/<node>/<device>:<aiCores>`.
//
// Empty input → empty string (caller decides whether to skip the
// annotation entirely or write empty).
func EncodeBindings(bindings []SliceBinding) string {
	if len(bindings) == 0 {
		return ""
	}
	sorted := append([]SliceBinding(nil), bindings...)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.Node != b.Node {
			return a.Node < b.Node
		}
		if a.Pool != b.Pool {
			return a.Pool < b.Pool
		}
		return a.Device < b.Device
	})
	parts := make([]string, 0, len(sorted))
	for _, b := range sorted {
		parts = append(parts, fmt.Sprintf("%s/%s/%s:%d", b.Node, b.Pool, b.Device, b.AICores))
	}
	return strings.Join(parts, ",")
}

// FilterAllocated returns the subset of bindings whose Phase ==
// "Allocated" (the npu.ocloud.edge.example.com/v1alpha1
// .NPUSliceAllocationPhaseAllocated constant from npu-dra-driver,
// duplicated as a string here per operators/CLAUDE.md §1).
func FilterAllocated(bindings []SliceBinding) []SliceBinding {
	out := make([]SliceBinding, 0, len(bindings))
	for _, b := range bindings {
		if b.Phase == phaseAllocated {
			out = append(out, b)
		}
	}
	return out
}

// CountByPhase returns (allocated, orphaned, other) counts. Phase 5
// T103 fail-closed contract: when all bindings are Orphaned, the
// webhook returns Denied with reason NoAvailableNPUSlices.
func CountByPhase(bindings []SliceBinding) (allocated, orphaned, other int) {
	for _, b := range bindings {
		switch b.Phase {
		case phaseAllocated:
			allocated++
		case phaseOrphaned:
			orphaned++
		default:
			other++
		}
	}
	return
}

// String constants mirrored from
// operators/npu-dra-driver/api/v1alpha1.NPUSliceAllocationPhase*.
// Cross-module Go imports forbidden per operators/CLAUDE.md §1; text
// duplication is the convention.
const (
	phaseAllocated = "Allocated"
	phaseReleased  = "Released"
	phaseOrphaned  = "Orphaned"
)

// _ silences "unused" linters for the phase constants that downstream
// consumers (T103 mutation logic) reference but the encoder itself
// does not.
var _ = phaseReleased
