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
	"strings"

	resourceapi "k8s.io/api/resource/v1beta1"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/npu-dra-driver/api/v1alpha1"
)

// Greedy is the Phase 5 default Allocator. It scans ResourceSlices in
// lexicographic order by name; within each slice it scans devices in
// lexicographic order by name; the first device that:
//
//  1. Belongs to the npu-dra-driver (v1alpha1.DriverName),
//  2. Is not already in the AllocatedSet,
//  3. Reports Health = Healthy via AscendDeviceFromUpstream,
//  4. Matches the claim request's DeviceClassName sub-class filter
//     (bare class matches everything; ".whole" / ".dynamic" — and
//     forward-compat "/whole" / "/dynamic" — gate on slice-strategy),
//
// is returned as the Allocation. ErrNoAvailableDevice when nothing fits.
//
// Determinism contract (allocator.go): same inputs always produce the
// same output. Sort happens on a defensive copy so callers retain their
// input slice order.
type Greedy struct{}

// Allocate implements Allocator.Allocate.
func (g *Greedy) Allocate(claim resourceapi.ResourceClaim, slices []resourceapi.ResourceSlice, allocated AllocatedSet) (*Allocation, error) {
	if len(claim.Spec.Devices.Requests) == 0 {
		return nil, fmt.Errorf("allocator: claim %s/%s has no device requests", claim.Namespace, claim.Name)
	}
	req := claim.Spec.Devices.Requests[0]
	if !isOurClassName(req.DeviceClassName) {
		return nil, fmt.Errorf("allocator: claim %s/%s request %q is not for npu-dra-driver", claim.Namespace, claim.Name, req.DeviceClassName)
	}

	sorted := append([]resourceapi.ResourceSlice(nil), slices...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	for i := range sorted {
		slice := &sorted[i]
		if slice.Spec.Driver != v1alpha1.DriverName {
			continue
		}
		poolName := slice.Spec.Pool.Name

		devOrder := make([]int, len(slice.Spec.Devices))
		for k := range devOrder {
			devOrder[k] = k
		}
		sort.SliceStable(devOrder, func(a, b int) bool {
			return slice.Spec.Devices[devOrder[a]].Name < slice.Spec.Devices[devOrder[b]].Name
		})

		for _, idx := range devOrder {
			dev := slice.Spec.Devices[idx]
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
			return &Allocation{
				Driver:   v1alpha1.DriverName,
				Pool:     poolName,
				Device:   dev.Name,
				Request:  req.Name,
				AICores:  ad.AICores,
				Strategy: ad.SliceStrategy,
				NodeName: slice.Spec.NodeName,
			}, nil
		}
	}

	return nil, ErrNoAvailableDevice
}

// isOurClassName mirrors claim_controller.isOurClass so the allocator can
// short-circuit foreign claims that slipped past the controller's pre-
// filter (defense-in-depth). Kept package-local to avoid an
// allocator → controller import cycle.
func isOurClassName(name string) bool {
	if name == "" {
		return false
	}
	return name == v1alpha1.DriverName ||
		strings.HasPrefix(name, v1alpha1.DriverName+"/") ||
		strings.HasPrefix(name, v1alpha1.DriverName+".")
}

// matchesSubClass returns true when a device with the given slice-strategy
// satisfies the implicit filter of the claim's DeviceClassName sub-class.
//
//   - Bare class (`npu.ocloud.edge.example.com`) matches all strategies.
//   - `.whole` / `/whole` sub-class requires SliceStrategyFixedTemplate.
//   - `.dynamic` / `/dynamic` sub-class requires SliceStrategyDynamic.
//   - Any other suffix is rejected (defensive: an unknown sub-class is
//     better treated as "no match" than as "everything matches").
//
// The `/` shapes are accepted for forward compatibility with claims
// authored by external consumers even though Phase 5 cannot CREATE a
// DeviceClass with `/` in the name (T001 devlog explains the RFC 1123
// constraint).
func matchesSubClass(className, strategy string) bool {
	if className == v1alpha1.DriverName {
		return true
	}
	suffix := strings.TrimPrefix(className, v1alpha1.DriverName)
	switch suffix {
	case ".whole", "/whole":
		return strategy == v1alpha1.SliceStrategyFixedTemplate
	case ".dynamic", "/dynamic":
		return strategy == v1alpha1.SliceStrategyDynamic
	}
	return false
}
