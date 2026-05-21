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

package binpack

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

// K8s 1.34 plugin contract: Score signature is `Score(ctx, fwk.CycleState,
// pod, fwk.NodeInfo)`. The test helper builds a `*framework.NodeInfo` via
// `framework.NewNodeInfo()` which satisfies the `fwk.NodeInfo` interface.

// makeNodeInfo builds a framework.NodeInfo with the given allocatable
// resources. NPU goes into ScalarResources via SetNode's automatic
// conversion.
func makeNodeInfo(cpu, memory, npu int64) *framework.NodeInfo {
	allocatable := v1.ResourceList{}
	if cpu > 0 {
		allocatable[v1.ResourceCPU] = *resource.NewMilliQuantity(cpu, resource.DecimalSI)
	}
	if memory > 0 {
		allocatable[v1.ResourceMemory] = *resource.NewQuantity(memory, resource.BinarySI)
	}
	if npu > 0 {
		allocatable[v1.ResourceName(NPUExtendedResource)] = *resource.NewQuantity(npu, resource.DecimalSI)
	}
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		Status: v1.NodeStatus{
			Allocatable: allocatable,
			Capacity:    allocatable,
		},
	}
	ni := framework.NewNodeInfo()
	ni.SetNode(node)
	return ni
}

// makePodWithRequests builds a Pod with the given resource requests
// across a single container.
func makePodWithRequests(cpu, memory, npu int64) *v1.Pod {
	requests := v1.ResourceList{}
	if cpu > 0 {
		requests[v1.ResourceCPU] = *resource.NewMilliQuantity(cpu, resource.DecimalSI)
	}
	if memory > 0 {
		requests[v1.ResourceMemory] = *resource.NewQuantity(memory, resource.BinarySI)
	}
	if npu > 0 {
		requests[v1.ResourceName(NPUExtendedResource)] = *resource.NewQuantity(npu, resource.DecimalSI)
	}
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
		Spec: v1.PodSpec{
			Containers: []v1.Container{{
				Name:  "c",
				Image: "busybox",
				Resources: v1.ResourceRequirements{
					Requests: requests,
				},
			}},
		},
	}
}

// TestBinpackScoreNode exercises the 4 plan-acceptance cases plus a 50%
// utilization sanity check.
func TestBinpackScoreNode(t *testing.T) {
	t.Run("disabled → 0 regardless of input", func(t *testing.T) {
		args := defaultArgs()
		args.Enabled = false
		score := scoreNodeForPod(args, makePodWithRequests(1000, 1<<30, 2),
			makeNodeInfo(8000, 8<<30, 8))
		if score != 0 {
			t.Fatalf("disabled Binpack should return 0, got %d", score)
		}
		// Verify the Score wrapper also short-circuits when disabled.
		p := NewForTest(args, nil)
		ni := makeNodeInfo(8000, 8<<30, 8)
		s, status := p.Score(nil, nil, nil, ni)
		if !status.IsSuccess() || s != 0 {
			t.Fatalf("wrapper disabled path: status=%v score=%d", status, s)
		}
	})

	t.Run("single-resource: NPU 4/8 cores requested on 8-capable node → 50ish", func(t *testing.T) {
		// Use a single-resource weighting so the math is easy to follow.
		args := &BinpackArgs{
			Enabled:         true,
			Weight:          1,
			ResourceWeights: map[string]int64{NPUExtendedResource: 5},
		}
		score := scoreNodeForPod(args,
			makePodWithRequests(0, 0, 4), // request 4 NPU
			makeNodeInfo(0, 0, 8))         // allocatable 8 NPU
		// ratio = 4/8 = 0.5 → contribution = 5 * 0.5 * 100 = 250
		// total_weight = 5 → score = 250/5 = 50
		if score != 50 {
			t.Fatalf("expected score=50 (single NPU @ 50%% util), got %d", score)
		}
	})

	t.Run("multi-resource default weights: 50% util across cpu+memory+npu → 50", func(t *testing.T) {
		args := defaultArgs()
		args.Enabled = true
		score := scoreNodeForPod(args,
			makePodWithRequests(4000, 4<<30, 4),  // request 4 cores / 4 GiB / 4 NPU
			makeNodeInfo(8000, 8<<30, 8))          // allocatable 8/8/8
		// Each resource ratio = 0.5; default weights {cpu:1, memory:1, npu:5}
		// contributions = 1*50 + 1*50 + 5*50 = 350
		// total_weight = 7 → score = 350/7 = 50
		if score != 50 {
			t.Fatalf("expected score=50 (3 resources @ 50%% util), got %d", score)
		}
	})

	t.Run("empty allocatable (skip node) → 0", func(t *testing.T) {
		args := defaultArgs()
		args.Enabled = true
		// Node has zero allocatable for every weighted resource.
		score := scoreNodeForPod(args,
			makePodWithRequests(1000, 1<<30, 1),
			makeNodeInfo(0, 0, 0))
		if score != 0 {
			t.Fatalf("empty allocatable should yield score=0, got %d", score)
		}
	})

	t.Run("Pod requests nothing → 0 (no weighted contribution)", func(t *testing.T) {
		args := defaultArgs()
		args.Enabled = true
		score := scoreNodeForPod(args,
			makePodWithRequests(0, 0, 0),
			makeNodeInfo(8000, 8<<30, 8))
		if score != 0 {
			t.Fatalf("zero-request Pod should yield score=0, got %d", score)
		}
	})

	t.Run("clamp ratio at 1: Pod over-requests → score = full weight contribution", func(t *testing.T) {
		args := &BinpackArgs{
			Enabled:         true,
			Weight:          1,
			ResourceWeights: map[string]int64{NPUExtendedResource: 5},
		}
		score := scoreNodeForPod(args,
			makePodWithRequests(0, 0, 100), // request 100 NPU
			makeNodeInfo(0, 0, 8))           // allocatable 8 NPU
		// ratio clamped to 1.0 → contribution = 5 * 1.0 * 100 = 500
		// score = 500 / 5 = 100
		if score != 100 {
			t.Fatalf("expected score=100 (clamped), got %d", score)
		}
	})
}

// TestParseArgs validates default substitution + Weight bounds.
func TestParseArgs(t *testing.T) {
	t.Run("nil → defaults", func(t *testing.T) {
		args, err := parseArgs(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if args.Weight != DefaultWeight {
			t.Fatalf("Weight = %d, want %d", args.Weight, DefaultWeight)
		}
		if args.Enabled != DefaultEnabled {
			t.Fatalf("Enabled = %v, want %v", args.Enabled, DefaultEnabled)
		}
		if args.ResourceWeights[NPUExtendedResource] != 5 {
			t.Fatalf("NPU weight should default to 5, got %d", args.ResourceWeights[NPUExtendedResource])
		}
	})

	t.Run("Weight out of range rejected", func(t *testing.T) {
		_, err := parseArgs(&BinpackArgs{Weight: 999})
		if err == nil {
			t.Fatal("expected error for Weight=999")
		}
	})

	t.Run("Enabled true preserved", func(t *testing.T) {
		args, err := parseArgs(&BinpackArgs{Enabled: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !args.Enabled {
			t.Fatal("Enabled should preserve true")
		}
	})
}
