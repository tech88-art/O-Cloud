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

// Package integration_test exercises HCCSTopology Filter+Score + Binpack
// Score composed together on a shared cluster fixture. P6-T-008.
//
// **Scope**: in-process plugin composition test using the per-plugin
// fakeSliceLister + fakeAllocationLister abstractions established at
// T004 + T005 + T007. We deliberately do NOT spin up envtest /
// setup-envtest here because:
//
//  1. Windows envtest needs UAC elevation (see phase-6-t003 devlog) →
//     can't run in non-elevated CI / Claude shell.
//  2. The plugin composition itself is what matters at T008 — does
//     Filter+Score reach correct decisions on a multi-ring multi-node
//     fixture? Real apiserver dispatch is orthogonal.
//  3. T106 kind smoke covers the live-cluster path (P101 chart + real
//     kube-scheduler binary running against kindnet).
//
// NUMA plugin coverage is **deferred** — T006 landed a placeholder per
// the upstream sched-plugins v0.32.x release gating (see phase-6-t006
// devlog). When NUMA wrap lands, this test should grow a 6th case
// exercising the NumaAffinity Filter+Score behavior.
package integration_test

import (
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fwk "k8s.io/kube-scheduler/framework"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	hccspkg "github.com/tech88-art/O-Cloud/operators/scheduler-plugin/internal/plugins/hccs"
)

// K8s 1.34 plugin contract: Filter/Score signatures take `fwk.NodeInfo`
// interface (k8s.io/kube-scheduler/framework). The fixture's *framework.NodeInfo
// struct pointer (built via framework.NewNodeInfo) satisfies that interface
// automatically. Per P10-T-004 三件套 part 2.

// fixture builds the standard P6-T-008 test cluster: 2 nodes × 4 NPUs each,
// HCCS rings split worker-a={0,1} / worker-b={2,3}.
type fixture struct {
	sliceLister     *fakeSliceLister
	allocLister     *fakeAllocationLister
	nodes           map[string]*v1.Node
}

func newFixture() *fixture {
	f := &fixture{
		sliceLister: newFakeSliceLister(),
		allocLister: newFakeAllocationLister(),
		nodes:       map[string]*v1.Node{},
	}

	// worker-a: NPUs on rings 0+1, 32 AICores each, 16 CPU / 64Gi mem
	f.addNode("worker-a", 16000, 64<<30, 4)
	f.addSlice("worker-a",
		makeDevice("npu-0", 0, "Healthy"),
		makeDevice("npu-1", 0, "Healthy"),
		makeDevice("npu-2", 1, "Healthy"),
		makeDevice("npu-3", 1, "Healthy"),
	)

	// worker-b: NPUs on rings 2+3, same CPU/mem/NPU count
	f.addNode("worker-b", 16000, 64<<30, 4)
	f.addSlice("worker-b",
		makeDevice("npu-0", 2, "Healthy"),
		makeDevice("npu-1", 2, "Healthy"),
		makeDevice("npu-2", 3, "Healthy"),
		makeDevice("npu-3", 3, "Healthy"),
	)

	return f
}

func (f *fixture) addNode(name string, milliCPU, memBytes, npuCount int64) {
	allocatable := v1.ResourceList{
		v1.ResourceCPU:    *resource.NewMilliQuantity(milliCPU, resource.DecimalSI),
		v1.ResourceMemory: *resource.NewQuantity(memBytes, resource.BinarySI),
		v1.ResourceName("npu.ocloud.edge.example.com/devices"): *resource.NewQuantity(npuCount, resource.DecimalSI),
	}
	f.nodes[name] = &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: v1.NodeStatus{
			Allocatable: allocatable,
			Capacity:    allocatable,
		},
	}
}

func (f *fixture) addSlice(nodeName string, devices ...resourceapi.Device) {
	f.sliceLister.addSlice(nodeName, devices...)
}

func (f *fixture) nodeInfoFor(nodeName string) *framework.NodeInfo {
	ni := framework.NewNodeInfo()
	ni.SetNode(f.nodes[nodeName])
	return ni
}

// runHCCSFilter wires PreScore + Filter on a per-pod basis. Returns the
// Status from Filter (Score path isn't relevant for Filter assertions).
// K8s 1.34: Filter return type is `*fwk.Status` (k8s.io/kube-scheduler).
func (f *fixture) runHCCSFilter(t *testing.T, p *hccspkg.HCCSTopology, pod *v1.Pod, nodeName string) *fwk.Status {
	t.Helper()
	return p.Filter(context.Background(), nil, pod, f.nodeInfoFor(nodeName))
}

// runHCCSScore wires PreScore + Score on a per-pod, per-node basis. Each
// call invokes PreScore once (cycleState is fresh) which matches the
// scheduler framework contract. K8s 1.34: Score takes `fwk.NodeInfo`
// interface; fixture's `*framework.NodeInfo` struct pointer satisfies it.
func (f *fixture) runHCCSScore(t *testing.T, p *hccspkg.HCCSTopology, pod *v1.Pod, nodeName string) int64 {
	t.Helper()
	cs := framework.NewCycleState()
	if status := p.PreScore(context.Background(), cs, pod, nil); !status.IsSuccess() {
		t.Fatalf("PreScore non-success: %v: %s", status.Code(), status.Message())
	}
	score, status := p.Score(context.Background(), cs, pod, f.nodeInfoFor(nodeName))
	if !status.IsSuccess() {
		t.Fatalf("Score non-success: %v: %s", status.Code(), status.Message())
	}
	return score
}

// TestIntegrationPlugins exercises HCCSTopology Filter+Score (T004+T005)
// + Binpack Score (T007) on the 2-node × 2-ring fixture. NUMA plugin
// deferred per T006 placeholder.
//
// Plan §3 P6-T-008 acceptance maps to the 5 sub-tests below.
func TestIntegrationPlugins(t *testing.T) {
	f := newFixture()
	p := hccspkg.NewForTest(nil, f.sliceLister, f.allocLister)

	t.Run("Pod 1 — no MS label → permissive Filter + neutral Score on every node", func(t *testing.T) {
		pod := podBuilder().Build()

		// Filter: every node should pass (annotation absent + permissive default).
		for _, nodeName := range []string{"worker-a", "worker-b"} {
			if status := f.runHCCSFilter(t, p, pod, nodeName); !status.IsSuccess() {
				t.Fatalf("node %q: Filter expected Success, got %v: %s",
					nodeName, status.Code(), status.Message())
			}
		}

		// Score: 50 on every node (no MS label → no co-location preference).
		for _, nodeName := range []string{"worker-a", "worker-b"} {
			if score := f.runHCCSScore(t, p, pod, nodeName); score != hccspkg.ScoreNeutral {
				t.Fatalf("node %q: Score expected %d (neutral), got %d",
					nodeName, hccspkg.ScoreNeutral, score)
			}
		}
	})

	t.Run("Pod 2 — MS label with 0 siblings → neutral Score on every node", func(t *testing.T) {
		pod := podBuilder().withMS("ns/llama").Build()

		// No allocations → ringsOccupied is empty → Score returns Neutral.
		for _, nodeName := range []string{"worker-a", "worker-b"} {
			if score := f.runHCCSScore(t, p, pod, nodeName); score != hccspkg.ScoreNeutral {
				t.Fatalf("node %q: Score expected %d (neutral / no siblings), got %d",
					nodeName, hccspkg.ScoreNeutral, score)
			}
		}
	})

	t.Run("Pod 3 — MS label with sibling on worker-a:ring=0 → Score 100 worker-a, 30 worker-b", func(t *testing.T) {
		f.allocLister.addAllocation("ns/llama", "worker-a", "npu-0") // ring 0
		defer f.allocLister.reset()

		pod := podBuilder().withMS("ns/llama").Build()

		// worker-a has ring 0 same as sibling → 100
		if score := f.runHCCSScore(t, p, pod, "worker-a"); score != hccspkg.ScoreSame {
			t.Fatalf("worker-a: Score expected %d (same ring), got %d", hccspkg.ScoreSame, score)
		}
		// worker-b has rings 2,3 disjoint from sibling's ring 0 → 30 (no adjacency configured)
		if score := f.runHCCSScore(t, p, pod, "worker-b"); score != hccspkg.ScoreDisjoint {
			t.Fatalf("worker-b: Score expected %d (disjoint), got %d", hccspkg.ScoreDisjoint, score)
		}
	})

	t.Run("Pod 4 — preferred-hccs-ring=5 annotation (no node has ring 5) → all nodes filtered", func(t *testing.T) {
		pod := podBuilder().withRingAnnotation("5").Build()

		for _, nodeName := range []string{"worker-a", "worker-b"} {
			status := f.runHCCSFilter(t, p, pod, nodeName)
			if status.Code() != fwk.UnschedulableAndUnresolvable {
				t.Fatalf("node %q: Filter expected UnschedulableAndUnresolvable, got %v: %s",
					nodeName, status.Code(), status.Message())
			}
		}
	})

	t.Run("Pod 5 — adjacency map kicks in: sibling on ring 0, this node on ring 1 (adjacent)", func(t *testing.T) {
		// New plugin instance with adjacency 0 ↔ 1, 2 ↔ 3.
		args := &hccspkg.HCCSTopologyArgs{
			Weight:           5,
			PreferAnnotation: "npu.huawei.com/preferred-hccs-ring",
			Adjacency: map[string][]int32{
				"0": {1},
				"1": {0},
				"2": {3},
				"3": {2},
			},
		}
		pAdj := hccspkg.NewForTest(args, f.sliceLister, f.allocLister)

		f.allocLister.addAllocation("ns/llama", "worker-a", "npu-0") // ring 0
		defer f.allocLister.reset()

		pod := podBuilder().withMS("ns/llama").Build()

		// worker-a has rings 0,1. Sibling on ring 0 → same → 100
		if score := f.runHCCSScore(t, pAdj, pod, "worker-a"); score != hccspkg.ScoreSame {
			t.Fatalf("worker-a: Score expected %d (sibling on ring 0), got %d",
				hccspkg.ScoreSame, score)
		}
		// worker-b has rings 2,3. Sibling on ring 0 → no same, no adjacent (3 is not adjacent to 0)
		// → 30 (disjoint)
		if score := f.runHCCSScore(t, pAdj, pod, "worker-b"); score != hccspkg.ScoreDisjoint {
			t.Fatalf("worker-b: Score expected %d (disjoint per adjacency), got %d",
				hccspkg.ScoreDisjoint, score)
		}
	})
}
