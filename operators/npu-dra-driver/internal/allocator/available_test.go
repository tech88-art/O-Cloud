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
	"testing"

	resourceapi "k8s.io/api/resource/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/npu-dra-driver/api/v1alpha1"
)

func mkAllocatedClaim(name, uid, driver, pool, device string) resourceapi.ResourceClaim {
	return resourceapi.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			UID:  types.UID(uid),
		},
		Status: resourceapi.ResourceClaimStatus{
			Allocation: &resourceapi.AllocationResult{
				Devices: resourceapi.DeviceAllocationResult{
					Results: []resourceapi.DeviceRequestAllocationResult{
						{Driver: driver, Pool: pool, Device: device, Request: "req-0"},
					},
				},
			},
		},
	}
}

func TestComputeAllocatedFromClaims_FiltersOurDriver(t *testing.T) {
	claims := []resourceapi.ResourceClaim{
		mkAllocatedClaim("c1", "u1", v1alpha1.DriverName, "nodeA", "nodeA-npu-0"),
		mkAllocatedClaim("c2", "u2", v1alpha1.DriverName, "nodeB", "nodeB-npu-0"),
		// claim allocated against a different DRA driver — must NOT be counted
		mkAllocatedClaim("c3", "u3", "nvidia.example.com", "nodeC", "gpu-0"),
		// claim with no allocation — ignored
		{ObjectMeta: metav1.ObjectMeta{Name: "c4", UID: "u4"}},
	}
	got := ComputeAllocatedFromClaims(claims, "")
	if got.Len() != 2 {
		t.Errorf("want 2 entries (our driver only), got %d (%+v)", got.Len(), got)
	}
	if !got.Has("nodeA", "nodeA-npu-0") {
		t.Errorf("missing nodeA/nodeA-npu-0")
	}
	if !got.Has("nodeB", "nodeB-npu-0") {
		t.Errorf("missing nodeB/nodeB-npu-0")
	}
	if got.Has("nodeC", "gpu-0") {
		t.Errorf("foreign-driver allocation must NOT be in set")
	}
}

func TestComputeAllocatedFromClaims_ExcludeByUID(t *testing.T) {
	claims := []resourceapi.ResourceClaim{
		mkAllocatedClaim("c1", "u1", v1alpha1.DriverName, "nodeA", "nodeA-npu-0"),
		mkAllocatedClaim("c2", "u2", v1alpha1.DriverName, "nodeB", "nodeB-npu-0"),
	}
	got := ComputeAllocatedFromClaims(claims, "u1")
	if got.Len() != 1 {
		t.Errorf("want 1 entry after excluding u1, got %d", got.Len())
	}
	if got.Has("nodeA", "nodeA-npu-0") {
		t.Errorf("excluded claim's allocation must NOT be in set")
	}
	if !got.Has("nodeB", "nodeB-npu-0") {
		t.Errorf("non-excluded claim must remain in set")
	}
}

func TestComputeAllocatedFromClaims_EmptyInput(t *testing.T) {
	if got := ComputeAllocatedFromClaims(nil, ""); got.Len() != 0 {
		t.Errorf("nil claims must produce empty set; got %+v", got)
	}
}

func TestAllocatedSet_AddIdempotent(t *testing.T) {
	s := NewAllocatedSet()
	s.Add("nodeA", "nodeA-npu-0")
	s.Add("nodeA", "nodeA-npu-0")
	if s.Len() != 1 {
		t.Errorf("Add must be idempotent; got Len=%d", s.Len())
	}
	if !s.Has("nodeA", "nodeA-npu-0") {
		t.Errorf("Has must return true for added entry")
	}
	if s.Has("nodeB", "x") {
		t.Errorf("Has must return false for missing entry")
	}
}
