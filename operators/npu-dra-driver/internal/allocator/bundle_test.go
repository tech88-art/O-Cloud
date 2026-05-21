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
	"errors"
	"testing"

	resourceapi "k8s.io/api/resource/v1beta1"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/npu-dra-driver/api/v1alpha1"
	"github.com/tech88-art/O-Cloud/operators/npu-dra-driver/internal/template"
)

// TestAllocateBundle_NilBundleFallsBackToWholeNPUPath covers Phase 7
// P7-T-105 acceptance case 1/4: nil bundle → exactly one allocation
// matching the existing Greedy.Allocate result (Phase 5 zero
// regression invariant).
func TestAllocateBundle_NilBundleFallsBackToWholeNPUPath(t *testing.T) {
	slices := []resourceapi.ResourceSlice{
		mkSlice("npu-dra-nodeA", "nodeA",
			mkDevice("nodeA-npu-0", 0, v1alpha1.SliceStrategyFixedTemplate, "32"),
		),
	}
	claim := mkClaim("c", "ns", v1alpha1.DriverName)
	var g Greedy

	allocs, err := AllocateBundle(&g, claim, nil, slices, NewAllocatedSet())
	if err != nil {
		t.Fatalf("AllocateBundle(nil bundle): %v", err)
	}
	if len(allocs) != 1 {
		t.Fatalf("len(allocs) = %d, want 1 (whole-NPU fallback)", len(allocs))
	}
	if allocs[0].Pool != "nodeA" || allocs[0].Device != "nodeA-npu-0" {
		t.Fatalf("allocs[0] = %+v, want nodeA / nodeA-npu-0", allocs[0])
	}
}

// TestAllocateBundle_SingleVir04 covers Phase 7 P7-T-105 acceptance
// case 2/4: a 1-item bundle (vir04 × 1) allocates 1 device from the
// available pool.
func TestAllocateBundle_SingleVir04(t *testing.T) {
	slices := []resourceapi.ResourceSlice{
		mkSlice("npu-dra-nodeA", "nodeA",
			mkDevice("nodeA-npu-0", 0, v1alpha1.SliceStrategyFixedTemplate, "32"),
		),
	}
	claim := mkClaim("c", "ns", v1alpha1.DriverName)
	bundle := &template.FixedTemplateBundle{
		Items: []template.FixedTemplateItem{
			{Template: "vir04", Count: 1},
		},
	}
	var g Greedy

	allocs, err := AllocateBundle(&g, claim, bundle, slices, NewAllocatedSet())
	if err != nil {
		t.Fatalf("AllocateBundle: %v", err)
	}
	if len(allocs) != 1 {
		t.Fatalf("len(allocs) = %d, want 1", len(allocs))
	}
}

// TestAllocateBundle_Vir04PlusVir08 covers Phase 7 P7-T-105
// acceptance case 3/4: a 2-item bundle (vir04 + vir08) allocates 2
// different devices (cumulative AllocatedSet prevents picking the same
// device twice).
func TestAllocateBundle_Vir04PlusVir08(t *testing.T) {
	slices := []resourceapi.ResourceSlice{
		mkSlice("npu-dra-nodeA", "nodeA",
			mkDevice("nodeA-npu-0", 0, v1alpha1.SliceStrategyFixedTemplate, "32"),
			mkDevice("nodeA-npu-1", 1, v1alpha1.SliceStrategyFixedTemplate, "32"),
		),
	}
	claim := mkClaim("c", "ns", v1alpha1.DriverName)
	bundle := &template.FixedTemplateBundle{
		Items: []template.FixedTemplateItem{
			{Template: "vir04", Count: 1},
			{Template: "vir08", Count: 1},
		},
	}
	var g Greedy

	allocs, err := AllocateBundle(&g, claim, bundle, slices, NewAllocatedSet())
	if err != nil {
		t.Fatalf("AllocateBundle: %v", err)
	}
	if len(allocs) != 2 {
		t.Fatalf("len(allocs) = %d, want 2 (vir04 + vir08)", len(allocs))
	}
	// Must be 2 different devices (cumulative dedup).
	if allocs[0].Device == allocs[1].Device {
		t.Fatalf("allocs[0].Device = allocs[1].Device = %q (cumulative AllocatedSet must dedup)",
			allocs[0].Device)
	}
}

// TestAllocateBundle_OverCapacityRollback covers Phase 7 P7-T-105
// acceptance case 4/4: bundle requesting more slots than the pool has
// → returns error; no partial allocations leak (caller's `allocated`
// AllocatedSet unchanged because the cumulative set is local).
func TestAllocateBundle_OverCapacityRollback(t *testing.T) {
	slices := []resourceapi.ResourceSlice{
		mkSlice("npu-dra-nodeA", "nodeA",
			mkDevice("nodeA-npu-0", 0, v1alpha1.SliceStrategyFixedTemplate, "32"),
		),
	}
	claim := mkClaim("c", "ns", v1alpha1.DriverName)
	bundle := &template.FixedTemplateBundle{
		Items: []template.FixedTemplateItem{
			{Template: "vir04", Count: 2}, // pool has 1 device, bundle wants 2
		},
	}
	var g Greedy

	callerAllocated := NewAllocatedSet()
	allocs, err := AllocateBundle(&g, claim, bundle, slices, callerAllocated)
	if err == nil {
		t.Fatalf("expected error (bundle wants 2 devices, pool has 1), got %+v", allocs)
	}
	if !errors.Is(err, ErrNoAvailableDevice) {
		t.Fatalf("expected errors.Is(err, ErrNoAvailableDevice), got %v", err)
	}
	if callerAllocated.Len() != 0 {
		t.Fatalf("caller's allocated set was mutated (len = %d, want 0) — rollback failed",
			callerAllocated.Len())
	}
}
