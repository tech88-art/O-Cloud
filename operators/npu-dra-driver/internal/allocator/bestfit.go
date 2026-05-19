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

package allocator

import (
	"fmt"
	"sort"

	resourceapi "k8s.io/api/resource/v1beta1"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/npu-dra-driver/api/v1alpha1"
)

// BestFit is the optional feature-flag Allocator variant called out in
// ADR-0009 §6 outcome (b). Among the eligible candidates (same filters
// Greedy applies — driver, allocated set, health, sub-class strategy),
// it picks the device with the smallest SliceAICoreCapacity. Rationale:
// on a heterogeneous fleet (mixed vir04 + vir08 + whole), allocating
// the smallest sufficient device first leaves larger devices free for
// larger future claims — minimum fragmentation.
//
// On the Phase 5 simulator (set-a-small/npus.json) all devices have
// identical capacity, so BestFit collapses to lexicographic tie-break
// on (capacity, slice name, device name) and is observationally equal
// to Greedy. The implementation matters once Phase 7 publishes
// per-partition Device entries with varying capacity.
//
// Phase 5 wiring: not enabled by default; T002 ships the Allocator
// field on ClaimReconciler so production users can flip to BestFit via
// a future flag (or controller-runtime option setter) without code
// churn here. Phase 6 scheduler-plugin supersedes both with
// topology-aware scoring (ADR-0009 §6 outcome (c)).
type BestFit struct{}

// Allocate implements Allocator.Allocate. Deterministic under tie:
// candidates with the same capacity are ordered first by slice name,
// then by device name (same lexicographic tiebreaker Greedy uses).
func (b *BestFit) Allocate(claim resourceapi.ResourceClaim, slices []resourceapi.ResourceSlice, allocated AllocatedSet) (*Allocation, error) {
	if len(claim.Spec.Devices.Requests) == 0 {
		return nil, fmt.Errorf("allocator: claim %s/%s has no device requests", claim.Namespace, claim.Name)
	}
	req := claim.Spec.Devices.Requests[0]
	if !isOurClassName(req.DeviceClassName) {
		return nil, fmt.Errorf("allocator: claim %s/%s request %q is not for npu-dra-driver", claim.Namespace, claim.Name, req.DeviceClassName)
	}

	type candidate struct {
		sliceIdx int
		devIdx   int
		capacity int64
		slice    *resourceapi.ResourceSlice
		device   resourceapi.Device
		ad       v1alpha1.AscendDevice
	}
	var cands []candidate

	for i := range slices {
		slice := &slices[i]
		if slice.Spec.Driver != v1alpha1.DriverName {
			continue
		}
		poolName := slice.Spec.Pool.Name
		for j, dev := range slice.Spec.Devices {
			if allocated.Has(poolName, dev.Name) {
				continue
			}
			ad, err := v1alpha1.AscendDeviceFromUpstream(dev)
			if err != nil {
				continue
			}
			if ad.Health != v1alpha1.HealthHealthy {
				continue
			}
			if !matchesSubClass(req.DeviceClassName, ad.SliceStrategy) {
				continue
			}
			cap := int64(0)
			if v, ok := ad.SliceAICoreCapacity.AsInt64(); ok {
				cap = v
			}
			cands = append(cands, candidate{
				sliceIdx: i,
				devIdx:   j,
				capacity: cap,
				slice:    slice,
				device:   dev,
				ad:       ad,
			})
		}
	}

	if len(cands) == 0 {
		return nil, ErrNoAvailableDevice
	}

	sort.SliceStable(cands, func(i, j int) bool {
		ci, cj := cands[i], cands[j]
		if ci.capacity != cj.capacity {
			return ci.capacity < cj.capacity
		}
		if ci.slice.Name != cj.slice.Name {
			return ci.slice.Name < cj.slice.Name
		}
		return ci.device.Name < cj.device.Name
	})

	pick := cands[0]
	return &Allocation{
		Driver:   v1alpha1.DriverName,
		Pool:     pick.slice.Spec.Pool.Name,
		Device:   pick.device.Name,
		Request:  req.Name,
		AICores:  pick.ad.AICores,
		Strategy: pick.ad.SliceStrategy,
		NodeName: pick.slice.Spec.NodeName,
	}, nil
}
