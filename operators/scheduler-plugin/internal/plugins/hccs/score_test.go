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
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

// K8s 1.34 plugin contract: Score signature is `Score(ctx, fwk.CycleState,
// pod, fwk.NodeInfo)`. The wiring helper builds a `*framework.NodeInfo` via
// `framework.NewNodeInfo()` which satisfies the `fwk.NodeInfo` interface.

// fakeAllocationLister implements AllocationLister for tests by returning a
// pre-populated per-ModelService allocation list.
type fakeAllocationLister struct {
	byMS map[string][]*SimpleAllocation
	err  error
}

func newFakeAllocationLister() *fakeAllocationLister {
	return &fakeAllocationLister{byMS: map[string][]*SimpleAllocation{}}
}

func (f *fakeAllocationLister) ListByModelService(ms string) ([]*SimpleAllocation, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byMS[ms], nil
}

func (f *fakeAllocationLister) addAllocation(ms, nodeName, device string) {
	f.byMS[ms] = append(f.byMS[ms], &SimpleAllocation{
		Name:            "alloc-" + device,
		ModelServiceRef: ms,
		NodeName:        nodeName,
		Device:          device,
		Phase:           AllocationPhaseAllocated,
	})
}

// makePodWithMS builds a Pod carrying the model-service label.
func makePodWithMS(ms string) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod-scored",
			Namespace: "default",
			Labels:    map[string]string{ModelServiceLabel: ms},
		},
	}
}

// makePodWithoutMS builds a Pod without the model-service label (neutral
// Score case).
func makePodWithoutMS() *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod-no-ms",
			Namespace: "default",
		},
	}
}

// runPreScoreAndScore wires PreScore + Score against the plugin for a
// single (pod, nodeName) tuple. Encapsulates the framework.CycleState
// dance so each test case stays focused on its assertion. K8s 1.34 Score
// takes `fwk.NodeInfo` interface — we build a *framework.NodeInfo via
// NewNodeInfo + SetNode and pass it (satisfies the interface).
func runPreScoreAndScore(t *testing.T, p *HCCSTopology, pod *v1.Pod, nodeName string) int64 {
	t.Helper()
	cs := framework.NewCycleState()
	status := p.PreScore(context.Background(), cs, pod, nil)
	if !status.IsSuccess() {
		t.Fatalf("PreScore returned non-success: %v: %s", status.Code(), status.Message())
	}
	ni := framework.NewNodeInfo()
	ni.SetNode(&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}})
	score, status := p.Score(context.Background(), cs, pod, ni)
	if !status.IsSuccess() {
		t.Fatalf("Score returned non-success: %v: %s", status.Code(), status.Message())
	}
	return score
}

// TestScore exercises the 5 ADR-0010 §2 Score tiers + the no-MS-label
// neutral case (6 sub-tests). Plan §3 P6-T-005 acceptance asks for 5
// envtest cases — implemented here via the fake lister abstraction.
func TestScore(t *testing.T) {
	t.Run("no model-service label → neutral 50", func(t *testing.T) {
		sLister := newFakeLister()
		sLister.addSlice("worker-a", makeDevice("npu-0", 0, "Healthy"))
		aLister := newFakeAllocationLister()
		p := NewForTest(nil, sLister, aLister)

		score := runPreScoreAndScore(t, p, makePodWithoutMS(), "worker-a")
		if score != ScoreNeutral {
			t.Fatalf("score = %d, want %d (neutral, no MS label)", score, ScoreNeutral)
		}
	})

	t.Run("MS label with 0 siblings → neutral 50", func(t *testing.T) {
		sLister := newFakeLister()
		sLister.addSlice("worker-a", makeDevice("npu-0", 0, "Healthy"))
		aLister := newFakeAllocationLister()
		p := NewForTest(nil, sLister, aLister)

		score := runPreScoreAndScore(t, p, makePodWithMS("ns/llama"), "worker-a")
		if score != ScoreNeutral {
			t.Fatalf("score = %d, want %d (neutral, no siblings yet)", score, ScoreNeutral)
		}
	})

	t.Run("MS label with sibling on ring 0 + this node has ring 0 → same 100", func(t *testing.T) {
		sLister := newFakeLister()
		sLister.addSlice("worker-a",
			makeDevice("npu-0", 0, "Healthy"),
			makeDevice("npu-1", 0, "Healthy"),
		)
		sLister.addSlice("worker-b",
			makeDevice("npu-0", 0, "Healthy"),
			makeDevice("npu-1", 0, "Healthy"),
		)
		aLister := newFakeAllocationLister()
		// Sibling already allocated on worker-a/npu-0 (ring 0).
		aLister.addAllocation("ns/llama", "worker-a", "npu-0")
		p := NewForTest(nil, sLister, aLister)

		// Score worker-b — it has devices on ring 0 same as sibling.
		score := runPreScoreAndScore(t, p, makePodWithMS("ns/llama"), "worker-b")
		if score != ScoreSame {
			t.Fatalf("score = %d, want %d (same ring co-location)", score, ScoreSame)
		}
	})

	t.Run("MS label with siblings on rings 0+1 → 100 (multi-ring same)", func(t *testing.T) {
		sLister := newFakeLister()
		sLister.addSlice("worker-a",
			makeDevice("npu-0", 0, "Healthy"),
			makeDevice("npu-1", 0, "Healthy"),
			makeDevice("npu-2", 1, "Healthy"),
		)
		sLister.addSlice("worker-b",
			makeDevice("npu-0", 1, "Healthy"),
		)
		aLister := newFakeAllocationLister()
		aLister.addAllocation("ns/llama", "worker-a", "npu-0") // ring 0
		aLister.addAllocation("ns/llama", "worker-a", "npu-2") // ring 1
		p := NewForTest(nil, sLister, aLister)

		// worker-b has device on ring 1, which is in {0,1} occupied.
		score := runPreScoreAndScore(t, p, makePodWithMS("ns/llama"), "worker-b")
		if score != ScoreSame {
			t.Fatalf("score = %d, want %d (multi-ring same)", score, ScoreSame)
		}
	})

	t.Run("MS label with sibling on adjacent ring (per Args.Adjacency) → 70", func(t *testing.T) {
		sLister := newFakeLister()
		// worker-a has only ring 5; sibling allocated on ring 5.
		sLister.addSlice("worker-a", makeDevice("npu-0", 5, "Healthy"))
		// worker-b has only ring 4; adjacency 4 ↔ 5.
		sLister.addSlice("worker-b", makeDevice("npu-0", 4, "Healthy"))
		aLister := newFakeAllocationLister()
		aLister.addAllocation("ns/llama", "worker-a", "npu-0") // ring 5

		args := defaultArgs()
		args.Adjacency = map[string][]int32{
			"4": {5},
			"5": {4},
		}
		p := NewForTest(args, sLister, aLister)

		score := runPreScoreAndScore(t, p, makePodWithMS("ns/llama"), "worker-b")
		if score != ScoreAdjacent {
			t.Fatalf("score = %d, want %d (adjacent ring)", score, ScoreAdjacent)
		}
	})

	t.Run("MS label with sibling on disjoint ring (no adjacency) → 30", func(t *testing.T) {
		sLister := newFakeLister()
		sLister.addSlice("worker-a", makeDevice("npu-0", 0, "Healthy"))
		sLister.addSlice("worker-b", makeDevice("npu-0", 7, "Healthy"))
		aLister := newFakeAllocationLister()
		aLister.addAllocation("ns/llama", "worker-a", "npu-0") // ring 0

		// No adjacency configured → 0 vs 7 is disjoint.
		p := NewForTest(nil, sLister, aLister)

		score := runPreScoreAndScore(t, p, makePodWithMS("ns/llama"), "worker-b")
		if score != ScoreDisjoint {
			t.Fatalf("score = %d, want %d (disjoint ring)", score, ScoreDisjoint)
		}
	})

	t.Run("Phase 7 T008 · default 910B 8-card adjacency → adjacent ring 70", func(t *testing.T) {
		sLister := newFakeLister()
		// worker-a has device on ring 0; sibling allocated on ring 0.
		sLister.addSlice("worker-a", makeDevice("npu-0", 0, "Healthy"))
		// worker-b has device on ring 1; default 8-card adj has 0↔{1,3} so
		// ring 1 is adjacent to ring 0.
		sLister.addSlice("worker-b", makeDevice("npu-0", 1, "Healthy"))
		aLister := newFakeAllocationLister()
		aLister.addAllocation("ns/llama", "worker-a", "npu-0") // ring 0

		args := defaultArgs()
		args.Adjacency = DefaultAdjacency910B8Card()
		p := NewForTest(args, sLister, aLister)

		// worker-a is same-ring → 100
		if score := runPreScoreAndScore(t, p, makePodWithMS("ns/llama"), "worker-a"); score != ScoreSame {
			t.Fatalf("worker-a score = %d, want %d (same-ring)", score, ScoreSame)
		}
		// worker-b is adjacent-ring per default 8-card adj → 70
		if score := runPreScoreAndScore(t, p, makePodWithMS("ns/llama"), "worker-b"); score != ScoreAdjacent {
			t.Fatalf("worker-b score = %d, want %d (adjacent-ring per DefaultAdjacency910B8Card)",
				score, ScoreAdjacent)
		}
	})

	t.Run("Phase 7 T008 · explicit empty Adjacency falls back to binary 100/30", func(t *testing.T) {
		sLister := newFakeLister()
		sLister.addSlice("worker-a", makeDevice("npu-0", 0, "Healthy"))
		sLister.addSlice("worker-b", makeDevice("npu-0", 1, "Healthy"))
		aLister := newFakeAllocationLister()
		aLister.addAllocation("ns/llama", "worker-a", "npu-0")

		args := defaultArgs()
		args.Adjacency = map[string][]int32{} // explicit empty
		p := NewForTest(args, sLister, aLister)

		// worker-a same-ring → 100
		if score := runPreScoreAndScore(t, p, makePodWithMS("ns/llama"), "worker-a"); score != ScoreSame {
			t.Fatalf("worker-a score = %d, want %d (same-ring)", score, ScoreSame)
		}
		// worker-b ring 1, sibling ring 0 — without adjacency map this is
		// DISJOINT (no 70 tier reachable) → 30 binary fallback
		if score := runPreScoreAndScore(t, p, makePodWithMS("ns/llama"), "worker-b"); score != ScoreDisjoint {
			t.Fatalf("worker-b score = %d, want %d (disjoint · binary fallback · no adjacency map)",
				score, ScoreDisjoint)
		}
	})

	t.Run("MS label with sibling but node has no candidate device → 0", func(t *testing.T) {
		sLister := newFakeLister()
		sLister.addSlice("worker-a", makeDevice("npu-0", 0, "Healthy"))
		// worker-b has no NPU devices at all (empty slice list).
		aLister := newFakeAllocationLister()
		aLister.addAllocation("ns/llama", "worker-a", "npu-0")

		p := NewForTest(nil, sLister, aLister)

		score := runPreScoreAndScore(t, p, makePodWithMS("ns/llama"), "worker-b")
		if score != ScoreNoCandidate {
			t.Fatalf("score = %d, want %d (no candidate device)", score, ScoreNoCandidate)
		}
	})
}

// TestBuildAdjacency covers the Args.Adjacency string→int conversion.
func TestBuildAdjacency(t *testing.T) {
	t.Run("nil → nil", func(t *testing.T) {
		if got := buildAdjacency(nil); got != nil {
			t.Fatalf("buildAdjacency(nil) = %v, want nil", got)
		}
	})
	t.Run("typed", func(t *testing.T) {
		got := buildAdjacency(map[string][]int32{"0": {1, 2}, "1": {0}})
		if len(got) != 2 {
			t.Fatalf("got %d entries, want 2", len(got))
		}
		if _, ok := got[0][1]; !ok {
			t.Fatal("0 should be adjacent to 1")
		}
		if _, ok := got[0][2]; !ok {
			t.Fatal("0 should be adjacent to 2")
		}
		if _, ok := got[1][0]; !ok {
			t.Fatal("1 should be adjacent to 0")
		}
	})
	t.Run("malformed key silently skipped", func(t *testing.T) {
		got := buildAdjacency(map[string][]int32{"bad": {1}, "0": {1}})
		if _, ok := got[0]; !ok {
			t.Fatal("valid key 0 should remain")
		}
		if len(got) != 1 {
			t.Fatalf("got %d entries, want 1 (bad key skipped)", len(got))
		}
	})
}
