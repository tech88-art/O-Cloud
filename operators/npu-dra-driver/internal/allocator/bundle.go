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

	resourceapi "k8s.io/api/resource/v1beta1"

	"github.com/tech88-art/O-Cloud/operators/npu-dra-driver/internal/template"
)

// AllocateBundle drives the Phase 7 P7-T-105 NPUSliceTemplate-aware
// allocation path (ADR-0011 §1 §後果 row 4): treats a
// FixedTemplateBundle as an all-or-nothing unit.
//
// Phase 7 W1 semantics (per ADR-0011 §後果 + phase7-plan.md §3 P7-T-105):
//
//   - bundle == nil OR bundle.IsEmpty() → falls back to a single
//     `a.Allocate(claim, slices, allocated)` call (Phase 5 whole-NPU
//     path · zero regression for Pods without slice-template label)
//   - bundle has items → loops bundle.Items × Count, calling
//     a.Allocate once per slot with a CUMULATIVE AllocatedSet so the
//     same device isn't picked twice
//   - any single Allocate failure rolls back the whole bundle (no
//     partial allocations leak; the cumulative set is local to this
//     call and never returned on error)
//   - whole-NPU path is preserved bit-for-bit: existing
//     Allocator.Allocate signature unchanged · Phase 5 tests pass
//
// Returns the full list of Allocations (length = bundle.TotalSlices)
// on success; nil + error on any failure. The error wraps the failing
// item template + index for operator debugging.
//
// **Caller wiring (deferred to Phase 10 / T105-v2)**: a Pod-aware
// controller path that:
//   1. Reads Pod label `npu.huawei.com/slice-template=<name>`
//   2. Looks up NPUSliceTemplate via client
//   3. Invokes template.Engine.Decompose to get the bundle
//   4. Invokes AllocateBundle here
//   5. Writes N allocations into ResourceClaim.Status / N
//      NPUSliceAllocation audit objects
//
// Phase 7 W1 ships only the allocator method (this file + bundle_test.go).
// The full controller wiring is deferred to Phase 10 demo polish when
// real Pods with slice-template labels show up in the kind / lab
// cluster. The capability is testable today via unit tests + future
// callers (Webhook · CLI tool · etc.).
func AllocateBundle(
	a Allocator,
	claim resourceapi.ResourceClaim,
	bundle *template.FixedTemplateBundle,
	slices []resourceapi.ResourceSlice,
	allocated AllocatedSet,
) ([]*Allocation, error) {
	// Phase 5 whole-NPU fallback path: bundle nil or empty.
	if bundle == nil || bundle.IsEmpty() {
		single, err := a.Allocate(claim, slices, allocated)
		if err != nil {
			return nil, err
		}
		return []*Allocation{single}, nil
	}

	// Phase 7 W1 bundle path: build cumulative AllocatedSet locally
	// (preserves input `allocated` for caller on success or rollback).
	cumulative := NewAllocatedSet()
	for k := range allocated {
		cumulative[k] = struct{}{}
	}

	out := make([]*Allocation, 0, bundle.TotalSlices())
	for _, item := range bundle.Items {
		for n := int32(0); n < item.Count; n++ {
			single, err := a.Allocate(claim, slices, cumulative)
			if err != nil {
				return nil, fmt.Errorf("bundle allocate item %s [#%d / count=%d]: %w",
					item.Template, n, item.Count, err)
			}
			cumulative.Add(single.Pool, single.Device)
			out = append(out, single)
		}
	}
	return out, nil
}
