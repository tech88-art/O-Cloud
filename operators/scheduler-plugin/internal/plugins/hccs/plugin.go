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

// Package hccs is the HCCSTopologyPlugin for kube-scheduler — Phase 6
// T004 Filter body; T005 will add Score.
//
// Per ADR-0010 §2 the plugin makes two decisions:
//
//   - **Filter** (T004): reads ResourceSlice device attributes
//     `npu.huawei.com/hccs_ring` and matches against the Pod's
//     `npu.huawei.com/preferred-hccs-ring` annotation. Permissive by
//     default (absent annotation passes); strict mode available via
//     Args.FailIfMissing=true.
//   - **Score** (T005): reads NPUSliceAllocation reverse-lookup to find
//     sibling Pods of the same ModelService and rewards co-location on
//     the same HCCS ring.
//
// ResourceSlice data is sourced via the scheduler framework's
// SharedInformerFactory cache (production) or a fake SliceLister (tests).
// See SliceLister interface in types.go.
package hccs

import (
	"context"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	resourceapi "k8s.io/api/resource/v1beta1"
	"k8s.io/client-go/dynamic"
	resourcelisters "k8s.io/client-go/listers/resource/v1beta1"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)
// Note: K8s 1.34 split plugin data types (NodeInfo / CycleState / Status /
// Code constants / StateKey / StateData) to `k8s.io/kube-scheduler/framework`
// (aliased `fwk` in filter.go + score.go). Plugin contract interfaces
// (Plugin / FilterPlugin / ScorePlugin / PreScorePlugin / Handle /
// ScoreExtensions) remain in `k8s.io/kubernetes/pkg/scheduler/framework`
// (aliased `framework` here). Constructors NewNodeInfo / NewCycleState also
// stay in `framework`. Per P10-T-004 三件套 part 2 migration.

// Name is the plugin name registered with kube-scheduler. KubeSchedulerConfig
// profiles[*].plugins.{filter,score}.enabled[].name MUST match this string.
const Name = "HCCSTopology"

// HCCSTopology is the plugin struct. T004 added Filter + SliceLister; T005
// adds PreScore + Score + AllocationLister. Compile-time interface
// assertions for Plugin/FilterPlugin live here; the Score side lives in
// score.go to keep file scope per-extension-point.
type HCCSTopology struct {
	args             *HCCSTopologyArgs
	SliceLister      SliceLister
	AllocationLister AllocationLister
}

// Compile-time assertions for Filter side. Score side asserted in score.go.
var (
	_ framework.Plugin       = &HCCSTopology{}
	_ framework.FilterPlugin = &HCCSTopology{}
)

// Name returns the plugin name. Required by framework.Plugin.
func (p *HCCSTopology) Name() string {
	return Name
}

// New constructs an HCCSTopology plugin instance, parsing args from the
// scheduler framework and wiring a production SliceLister backed by the
// framework's SharedInformerFactory ResourceSlice lister.
//
// Tests construct HCCSTopology directly via NewForTest with an injected
// SliceLister.
func New(_ context.Context, args runtime.Object, h framework.Handle) (framework.Plugin, error) {
	typed, err := parseArgs(args)
	if err != nil {
		return nil, err
	}
	var sLister SliceLister
	var aLister AllocationLister
	if h != nil {
		factory := h.SharedInformerFactory()
		if factory != nil {
			sLister = &informerSliceLister{
				lister: factory.Resource().V1beta1().ResourceSlices().Lister(),
			}
		}
		// NPUSliceAllocation is a CRD; built-in SharedInformerFactory does
		// not cover it. Build a dynamic client from the scheduler's
		// KubeConfig — the per-Score-cycle List() is acceptable for Phase
		// 6 simulator scope (< 100 NPUSliceAllocations). Phase 9 may want
		// to upgrade to dynamicinformer.NewDynamicSharedInformerFactory
		// + a metadata-only watch — see colocation.go.
		if kc := h.KubeConfig(); kc != nil {
			if dyn, err := dynamic.NewForConfig(kc); err == nil {
				aLister = &dynamicAllocationLister{client: dyn}
			}
			// Silently swallow dynamic-client construction error — Score
			// gracefully degrades to neutral when AllocationLister is nil.
		}
	}
	return &HCCSTopology{
		args:             typed,
		SliceLister:      sLister,
		AllocationLister: aLister,
	}, nil
}

// NewForTest constructs an HCCSTopology with caller-supplied args +
// listers. Test-only: production callers go through New().
func NewForTest(args *HCCSTopologyArgs, sLister SliceLister, aLister AllocationLister) *HCCSTopology {
	if args == nil {
		args = defaultArgs()
	}
	return &HCCSTopology{
		args:             args,
		SliceLister:      sLister,
		AllocationLister: aLister,
	}
}

// informerSliceLister adapts a `k8s.io/client-go/listers/resource/v1beta1`
// ResourceSliceLister to the local SliceLister interface. Filters by both
// the managed-by label and the requested nodeName so Filter / Score only
// see relevant slices.
type informerSliceLister struct {
	lister resourcelisters.ResourceSliceLister
}

// ListForNode lists ResourceSlices labelled managed-by=npu-dra-driver and
// pinned to nodeName.
func (l *informerSliceLister) ListForNode(nodeName string) ([]*resourceapi.ResourceSlice, error) {
	selector := labels.SelectorFromSet(labels.Set{LabelManagedBy: LabelManagedByValue})
	all, err := l.lister.List(selector)
	if err != nil {
		return nil, err
	}
	out := make([]*resourceapi.ResourceSlice, 0, len(all))
	for _, s := range all {
		if s.Spec.NodeName == nodeName {
			out = append(out, s)
		}
	}
	return out, nil
}
