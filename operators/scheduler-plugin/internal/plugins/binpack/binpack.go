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

// Package binpack implements the Binpack ScorePlugin per ADR-0010 §4.
//
// Score formula (ADR-0010 §4):
//
//   for each requested resource r in Args.ResourceWeights:
//     ratio_r = clamp(pod.requested[r] / node.allocatable[r], 0, 1)
//     contribution_r = weight_r * ratio_r * 100
//   score = sum(contribution_r) / sum(weight_r)
//
// Higher score = Pod consumes a larger fraction of node's allocatable per
// weighted resource = node is being "filled up" more, which Binpack
// prefers. Default off (Args.Enabled=false); chart operators opt in via
// values.yaml per ADR-0010 §4.
//
// Ascend-specific bias: default ResourceWeights gives NPU 5x weight over
// CPU/Memory so binpacking favours nodes already running NPU-heavy
// workloads. Phase 8 vertical-scaling work needs this to free empty nodes.
package binpack

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

// Name is the plugin name registered with kube-scheduler. KubeSchedulerConfig
// profiles[*].plugins.score.enabled[].name MUST match this string.
const Name = "Binpack"

// Binpack is the plugin struct. Implements framework.ScorePlugin only —
// Binpack does NOT filter (per ADR-0010 §4); a Pod's bin-packing
// preference never blocks scheduling.
type Binpack struct {
	args   *BinpackArgs
	handle framework.Handle
}

// Compile-time interface assertions.
var (
	_ framework.Plugin      = &Binpack{}
	_ framework.ScorePlugin = &Binpack{}
)

// Name returns the plugin name. Required by framework.Plugin.
func (p *Binpack) Name() string {
	return Name
}

// New constructs a Binpack plugin instance. Parses args (with defaults +
// validation), stashes the framework Handle for NodeInfo lookups in Score.
func New(_ context.Context, args runtime.Object, h framework.Handle) (framework.Plugin, error) {
	typed, err := parseArgs(args)
	if err != nil {
		return nil, err
	}
	return &Binpack{args: typed, handle: h}, nil
}

// NewForTest constructs a Binpack with caller-supplied args + handle.
// Test-only: production callers go through New().
func NewForTest(args *BinpackArgs, h framework.Handle) *Binpack {
	if args == nil {
		args = defaultArgs()
	}
	return &Binpack{args: args, handle: h}
}

// Score implements framework.ScorePlugin. Looks up nodeInfo via the
// framework's snapshot lister + delegates the math to scoreNodeForPod
// (testable without a Handle).
func (p *Binpack) Score(
	_ context.Context,
	_ *framework.CycleState,
	pod *v1.Pod,
	nodeName string,
) (int64, *framework.Status) {
	if !p.args.Enabled {
		return 0, nil
	}
	if p.handle == nil {
		return 0, nil
	}
	nodeInfo, err := p.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil || nodeInfo == nil {
		return 0, framework.NewStatus(framework.Error,
			fmt.Sprintf("Binpack Score: NodeInfo for %q: %v", nodeName, err))
	}
	return scoreNodeForPod(p.args, pod, nodeInfo), nil
}

// ScoreExtensions returns nil — framework auto-normalises to [0..100],
// which scoreNodeForPod already targets.
func (p *Binpack) ScoreExtensions() framework.ScoreExtensions {
	return nil
}

// scoreNodeForPod is the testable core of Binpack scoring. Returns a
// [0..100] integer when at least one weighted resource has both a Pod
// request and node allocatable > 0; returns 0 otherwise (empty
// allocatable → effectively skip the node from binpack preference,
// OR Args.Enabled=false → plugin is a no-op).
func scoreNodeForPod(args *BinpackArgs, pod *v1.Pod, nodeInfo *framework.NodeInfo) int64 {
	if args == nil || !args.Enabled || len(args.ResourceWeights) == 0 {
		return 0
	}
	requested := podRequestedTotals(pod)
	var (
		totalContribution float64
		totalWeight       int64
	)
	for resName, weight := range args.ResourceWeights {
		if weight <= 0 {
			continue
		}
		req := requestedFor(requested, resName)
		alloc := allocatableFor(nodeInfo, resName)
		if alloc <= 0 || req <= 0 {
			continue
		}
		ratio := float64(req) / float64(alloc)
		if ratio > 1 {
			ratio = 1
		}
		totalContribution += float64(weight) * ratio * 100
		totalWeight += weight
	}
	if totalWeight == 0 {
		return 0
	}
	return int64(totalContribution / float64(totalWeight))
}

// podRequestedTotals sums per-container resource requests across a Pod.
// Returns a freshly-allocated ResourceList.
func podRequestedTotals(pod *v1.Pod) v1.ResourceList {
	out := v1.ResourceList{}
	if pod == nil {
		return out
	}
	for _, c := range pod.Spec.Containers {
		for name, q := range c.Resources.Requests {
			if existing, ok := out[name]; ok {
				existing.Add(q)
				out[name] = existing
			} else {
				out[name] = q.DeepCopy()
			}
		}
	}
	return out
}

// requestedFor returns the requested quantity for resName as int64
// (millicores for cpu, bytes for memory, count for extended). Returns 0
// when not present.
func requestedFor(req v1.ResourceList, resName string) int64 {
	q, ok := req[v1.ResourceName(resName)]
	if !ok {
		return 0
	}
	if resName == string(v1.ResourceCPU) {
		return q.MilliValue()
	}
	return q.Value()
}

// allocatableFor reads the matching scalar from framework.NodeInfo's
// typed Allocatable. CPU is MilliCPU, memory is Memory (bytes),
// everything else lives in ScalarResources.
func allocatableFor(ni *framework.NodeInfo, resName string) int64 {
	if ni == nil || ni.Allocatable == nil {
		return 0
	}
	switch resName {
	case string(v1.ResourceCPU):
		return ni.Allocatable.MilliCPU
	case string(v1.ResourceMemory):
		return ni.Allocatable.Memory
	case string(v1.ResourceEphemeralStorage):
		return ni.Allocatable.EphemeralStorage
	default:
		if ni.Allocatable.ScalarResources == nil {
			return 0
		}
		return ni.Allocatable.ScalarResources[v1.ResourceName(resName)]
	}
}
