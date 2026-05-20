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
	"context"
	"fmt"
	"strconv"

	v1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

// Score tiers per ADR-0010 §2 Score table.
const (
	ScoreNoCandidate = int64(0)   // node has no candidate NPU device at all
	ScoreDisjoint    = int64(30)  // node's rings disjoint from siblings'
	ScoreAdjacent    = int64(70)  // node's rings adjacent to siblings' (per Args.Adjacency)
	ScoreNeutral     = int64(50)  // no MS label OR no siblings yet (no preference)
	ScoreSame        = int64(100) // node has at least one device on a sibling-occupied ring
)

// hccsStateKey is the CycleState key Score reads PreScore output from.
// Scheduler framework convention is `<plugin>StateKey` with the plugin's
// Name() as prefix.
const hccsStateKey framework.StateKey = "HCCSTopologyState"

// hccsState is the per-Pod CycleState payload Score consumes. Built by
// PreScore (one List of NPUSliceAllocations per Pod scheduling cycle); read
// by Score on every candidate node.
type hccsState struct {
	// ringsOccupied is the set of rings sibling Pods of the same
	// ModelService currently hold. Empty when there are no siblings yet
	// OR when the Pod carries no model-service label.
	ringsOccupied map[int64]struct{}

	// adjacency precomputes Args.Adjacency map[string][]int32 into an
	// int64 indexed nested-set form so Score's inner loop is O(1) per
	// ring lookup. Populated even when ringsOccupied is empty so future
	// re-execution doesn't need to re-parse.
	adjacency map[int64]map[int64]struct{}
}

// Clone returns the receiver unchanged — CycleState's clone contract is
// satisfied by sharing the snapshot because hccsState is read-only after
// PreScore writes it.
func (s *hccsState) Clone() framework.StateData {
	return s
}

// Compile-time interface assertions for the Score side. Plugin.go has
// the Plugin + FilterPlugin assertions; this adds Score + PreScore.
var (
	_ framework.PreScorePlugin = &HCCSTopology{}
	_ framework.ScorePlugin    = &HCCSTopology{}
)

// PreScore runs once per Pod scheduling cycle (before Score is invoked
// per-node). Looks up sibling allocations for the Pod's ModelService and
// stashes a precomputed ring-occupation set in CycleState.
//
// Behaviour matrix (ADR-0010 §2 Score precondition):
//   - Pod missing the model-service label → write empty state (Score
//     returns ScoreNeutral on every node)
//   - Pod with label, no siblings allocated → write empty state (same as above)
//   - Pod with label + siblings → write ringsOccupied set
//   - allocationLister or sliceLister nil → write empty state (graceful
//     degradation; Score also returns ScoreNeutral when state is empty)
func (p *HCCSTopology) PreScore(
	_ context.Context,
	cs *framework.CycleState,
	pod *v1.Pod,
	_ []*framework.NodeInfo,
) *framework.Status {
	state := &hccsState{
		adjacency: buildAdjacency(p.args.Adjacency),
	}

	modelService := pod.Labels[ModelServiceLabel]
	if modelService == "" || p.allocationLister == nil || p.sliceLister == nil {
		cs.Write(hccsStateKey, state)
		return nil
	}

	allocs, err := p.allocationLister.ListByModelService(modelService)
	if err != nil {
		return framework.NewStatus(framework.Error,
			fmt.Sprintf("HCCSTopology PreScore: list NPUSliceAllocation for %q: %v", modelService, err))
	}

	// Skip the Pod's own claim allocations if any happen to be in the
	// list — Score is meant to read sibling state, not self. We can't
	// reliably identify "self" without claim names, so the heuristic is:
	// if alloc Name == Pod.Name we drop it. Good-enough for Phase 6
	// simulator scope; revisit when production claim names diverge from
	// Pod names.
	state.ringsOccupied = make(map[int64]struct{}, len(allocs))
	for _, alloc := range allocs {
		if alloc.Name == pod.Name {
			continue
		}
		ring, ok := lookupDeviceRing(p.sliceLister, alloc.NodeName, alloc.Device)
		if !ok {
			continue
		}
		state.ringsOccupied[ring] = struct{}{}
	}

	cs.Write(hccsStateKey, state)
	return nil
}

// Score implements framework.ScorePlugin per ADR-0010 §2 Score table.
// Returns one of the constant ScoreXxx values + nil Status. Errors return
// Status.Code()=Error so the framework can decide whether to abort.
func (p *HCCSTopology) Score(
	_ context.Context,
	cs *framework.CycleState,
	pod *v1.Pod,
	nodeName string,
) (int64, *framework.Status) {
	state, ok := readHCCSState(cs)
	if !ok {
		// PreScore was skipped (unusual — framework calls PreScore unconditionally
		// when the plugin declares it). Fall back to permissive neutral.
		return ScoreNeutral, nil
	}

	// Pod has no MS label or no siblings allocated yet → neutral.
	if len(state.ringsOccupied) == 0 {
		// Distinguish "no MS label" (genuinely neutral) from "MS label but
		// node has no candidate device" — the latter should be discouraged.
		// Cheap check: pod label presence determines neutral vs probe.
		if _, hasLabel := pod.Labels[ModelServiceLabel]; !hasLabel {
			return ScoreNeutral, nil
		}
		// Pod has label, no siblings yet — still neutral (first Pod of a
		// new ModelService has no co-location preference).
		return ScoreNeutral, nil
	}

	if p.sliceLister == nil {
		// Can't introspect node rings → fall back to neutral.
		return ScoreNeutral, nil
	}

	nodeRings, err := nodeRingsFor(p.sliceLister, nodeName)
	if err != nil {
		return 0, framework.NewStatus(framework.Error,
			fmt.Sprintf("HCCSTopology Score: list ResourceSlices for %q: %v", nodeName, err))
	}
	if len(nodeRings) == 0 {
		return ScoreNoCandidate, nil
	}

	// Tier 1: same ring as a sibling.
	for r := range nodeRings {
		if _, ok := state.ringsOccupied[r]; ok {
			return ScoreSame, nil
		}
	}

	// Tier 2: adjacent to a sibling (per Args.Adjacency).
	for r := range nodeRings {
		adj := state.adjacency[r]
		for occupied := range state.ringsOccupied {
			if _, ok := adj[occupied]; ok {
				return ScoreAdjacent, nil
			}
		}
	}

	// Tier 3: disjoint.
	return ScoreDisjoint, nil
}

// ScoreExtensions returns nil — framework auto-normalises to [0..100] when
// extensions are absent, which is exactly what we want (our ScoreXxx
// constants already live in [0..100]).
func (p *HCCSTopology) ScoreExtensions() framework.ScoreExtensions {
	return nil
}

// readHCCSState retrieves the PreScore-stashed state. Returns ok=false when
// PreScore was skipped or wrote nothing.
func readHCCSState(cs *framework.CycleState) (*hccsState, bool) {
	raw, err := cs.Read(hccsStateKey)
	if err != nil {
		return nil, false
	}
	s, ok := raw.(*hccsState)
	return s, ok
}

// buildAdjacency converts HCCSTopologyArgs.Adjacency (string-keyed for
// JSON-encodability) into the int64 nested-set form Score uses for O(1)
// lookups. Malformed keys are silently skipped — same fail-soft posture
// as parsePreferredRings on the Filter side.
func buildAdjacency(raw map[string][]int32) map[int64]map[int64]struct{} {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[int64]map[int64]struct{}, len(raw))
	for kStr, vs := range raw {
		k, err := strconv.ParseInt(kStr, 10, 64)
		if err != nil {
			continue
		}
		set := make(map[int64]struct{}, len(vs))
		for _, v := range vs {
			set[int64(v)] = struct{}{}
		}
		out[k] = set
	}
	return out
}

// nodeRingsFor returns the set of rings present on healthy devices of
// nodeName, sourced from ResourceSlices the npu-dra-driver published.
// Empty set → node has no healthy NPU device the plugin recognises.
func nodeRingsFor(l sliceLister, nodeName string) (map[int64]struct{}, error) {
	slices, err := l.ListForNode(nodeName)
	if err != nil {
		return nil, err
	}
	out := map[int64]struct{}{}
	for _, slice := range slices {
		for _, dev := range slice.Spec.Devices {
			if !deviceHealthy(dev) {
				continue
			}
			if dev.Basic == nil {
				continue
			}
			ringAttr, ok := dev.Basic.Attributes[AttrHCCSRing]
			if !ok || ringAttr.IntValue == nil {
				continue
			}
			out[*ringAttr.IntValue] = struct{}{}
		}
	}
	return out, nil
}

// lookupDeviceRing returns the hccs_ring of a single (nodeName, deviceName)
// tuple, used by PreScore to translate sibling allocations into ring IDs.
// Returns ok=false when the tuple isn't found or the device lacks the
// attribute.
func lookupDeviceRing(l sliceLister, nodeName, deviceName string) (int64, bool) {
	slices, err := l.ListForNode(nodeName)
	if err != nil {
		return 0, false
	}
	for _, slice := range slices {
		for _, dev := range slice.Spec.Devices {
			if dev.Name != deviceName {
				continue
			}
			if dev.Basic == nil {
				return 0, false
			}
			ringAttr, ok := dev.Basic.Attributes[AttrHCCSRing]
			if !ok || ringAttr.IntValue == nil {
				return 0, false
			}
			return *ringAttr.IntValue, true
		}
	}
	return 0, false
}
