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

	v1 "k8s.io/api/core/v1"
	fwk "k8s.io/kube-scheduler/framework"
)

// Filter implements framework.FilterPlugin per ADR-0010 §2 Filter table:
//
//   - Pod missing preferred-hccs-ring annotation + Args.FailIfMissing=false:
//     return nil (Success / permissive)
//   - Pod missing annotation + Args.FailIfMissing=true:
//     return UnschedulableAndUnresolvable
//   - Pod has annotation "0,1": node passes IFF at least one ResourceSlice
//     on the node has a healthy device whose
//     `npu.huawei.com/hccs_ring` ∈ {0,1}
//   - Pod has annotation but requested ring NOT present on node:
//     return UnschedulableAndUnresolvable
//
// AICore "Available >= request.cores" comparison from the plan acceptance is
// deferred — DRA's ResourceClaim machinery enforces device availability via
// the standard framework.PreFilterPlugin "claim binding" path; the HCCS
// Filter focuses on the *topology* selection rather than capacity (which
// would duplicate the framework's claim-driven filter). T005 score will
// observe AICore via NPUSliceAllocation reverse-lookup, where capacity
// hints become co-location signal.
func (p *HCCSTopology) Filter(
	_ context.Context,
	_ fwk.CycleState,
	pod *v1.Pod,
	nodeInfo fwk.NodeInfo,
) *fwk.Status {
	if nodeInfo == nil || nodeInfo.Node() == nil {
		return fwk.NewStatus(fwk.Error, "HCCSTopology Filter: nil nodeInfo")
	}
	if p.args == nil {
		// Defensive — New() always populates args; guards against direct struct
		// construction in tests that skipped New.
		return fwk.NewStatus(fwk.Error, "HCCSTopology Filter: args not initialised")
	}

	annotationKey := p.args.PreferAnnotation
	annotation, hasAnnotation := pod.Annotations[annotationKey]

	if !hasAnnotation || annotation == "" {
		if p.args.FailIfMissing {
			return fwk.NewStatus(fwk.UnschedulableAndUnresolvable,
				fmt.Sprintf("Pod missing required annotation %q", annotationKey))
		}
		return nil // permissive default
	}

	preferredRings := parsePreferredRings(annotation)
	if len(preferredRings) == 0 {
		if p.args.FailIfMissing {
			return fwk.NewStatus(fwk.UnschedulableAndUnresolvable,
				fmt.Sprintf("Pod annotation %q contains no valid ring IDs", annotationKey))
		}
		return nil
	}

	nodeName := nodeInfo.Node().Name
	if p.SliceLister == nil {
		// No lister wired (e.g. Handle had no SharedInformerFactory). Treat
		// as "no slices observed" — permissive when FailIfMissing=false,
		// strict otherwise.
		if p.args.FailIfMissing {
			return fwk.NewStatus(fwk.UnschedulableAndUnresolvable,
				fmt.Sprintf("HCCSTopology Filter: no ResourceSlice lister wired; cannot satisfy annotation %q", annotationKey))
		}
		return nil
	}

	slices, err := p.SliceLister.ListForNode(nodeName)
	if err != nil {
		return fwk.NewStatus(fwk.Error,
			fmt.Sprintf("HCCSTopology Filter: list ResourceSlices for %q: %v", nodeName, err))
	}

	for _, slice := range slices {
		for _, dev := range slice.Spec.Devices {
			if deviceMatchesRing(dev, preferredRings) {
				return nil
			}
		}
	}

	return fwk.NewStatus(fwk.UnschedulableAndUnresolvable,
		fmt.Sprintf("no healthy NPU device on node %q matches HCCS ring set %v", nodeName, preferredRings))
}
