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
	resourceapi "k8s.io/api/resource/v1beta1"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/npu-dra-driver/api/v1alpha1"
)

// ComputeAllocatedFromClaims walks the live ResourceClaim list and builds
// an AllocatedSet from each claim's Status.Allocation.Devices.Results.
// Only entries whose Driver matches the npu-dra-driver name are counted —
// claims allocated against other DRA drivers (NVIDIA, AMD, etc.) live
// alongside ours and must not block our placement.
//
// The exclude argument is the UID of a claim whose own previous allocation
// (if any) should NOT count against availability — used by the claim
// controller during re-reconcile of a claim that already has an allocation
// it is about to update or replace.
//
// Phase 5 T002 single source: this function. Phase 5 T005 layers an
// additional reader for the NPUSliceAllocation CRD (created per
// successful allocation as an audit-log) but the canonical "is this
// device taken" answer remains ResourceClaim.Status.Allocation.
//
// Phase 9 quota: this function naturally extends — a quota controller
// can call it with a per-namespace filter to compute per-tenant
// reservations without touching the allocator core.
func ComputeAllocatedFromClaims(claims []resourceapi.ResourceClaim, exclude string) AllocatedSet {
	out := NewAllocatedSet()
	for i := range claims {
		c := &claims[i]
		if string(c.UID) == exclude {
			continue
		}
		if c.Status.Allocation == nil {
			continue
		}
		for _, r := range c.Status.Allocation.Devices.Results {
			if r.Driver != v1alpha1.DriverName {
				continue
			}
			out.Add(r.Pool, r.Device)
		}
	}
	return out
}
