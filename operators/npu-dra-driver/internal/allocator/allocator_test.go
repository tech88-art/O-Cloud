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
	"math/rand"
	"testing"

	resourceapi "k8s.io/api/resource/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/npu-dra-driver/api/v1alpha1"
)

func mkDevice(name string, index int64, strategy string, capacity string) resourceapi.Device {
	return v1alpha1.AscendDevice{
		Name:                name,
		Index:               index,
		Health:              v1alpha1.HealthHealthy,
		SliceStrategy:       strategy,
		AICores:             0,
		NUMANode:            0,
		HCCSRing:            0,
		SliceAICoreCapacity: resource.MustParse(capacity),
	}.ToUpstream()
}

func mkDeviceUnhealthy(name string, index int64) resourceapi.Device {
	return v1alpha1.AscendDevice{
		Name:                name,
		Index:               index,
		Health:              v1alpha1.HealthUnhealthy,
		SliceStrategy:       v1alpha1.SliceStrategyFixedTemplate,
		SliceAICoreCapacity: resource.MustParse("32"),
	}.ToUpstream()
}

func mkSlice(name, node string, devs ...resourceapi.Device) resourceapi.ResourceSlice {
	return resourceapi.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: resourceapi.ResourceSliceSpec{
			Driver: v1alpha1.DriverName,
			Pool: resourceapi.ResourcePool{
				Name:               node,
				Generation:         1,
				ResourceSliceCount: 1,
			},
			NodeName: node,
			Devices:  devs,
		},
	}
}

func mkClaim(name, ns, className string) resourceapi.ResourceClaim {
	return resourceapi.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			UID:       types.UID(name + "-uid"),
		},
		Spec: resourceapi.ResourceClaimSpec{
			Devices: resourceapi.DeviceClaim{
				Requests: []resourceapi.DeviceRequest{
					{Name: "req-0", DeviceClassName: className},
				},
			},
		},
	}
}

// TestGreedy_DeterministicAcrossShuffles verifies the determinism
// contract: identical input always yields identical output. Three
// shuffles of the same (slice + device) population must all return
// the same Allocation.
func TestGreedy_DeterministicAcrossShuffles(t *testing.T) {
	base := []resourceapi.ResourceSlice{
		mkSlice("npu-dra-nodeC", "nodeC",
			mkDevice("nodeC-npu-0", 0, v1alpha1.SliceStrategyFixedTemplate, "32"),
			mkDevice("nodeC-npu-1", 1, v1alpha1.SliceStrategyFixedTemplate, "32"),
		),
		mkSlice("npu-dra-nodeA", "nodeA",
			mkDevice("nodeA-npu-0", 0, v1alpha1.SliceStrategyFixedTemplate, "32"),
		),
		mkSlice("npu-dra-nodeB", "nodeB",
			mkDevice("nodeB-npu-0", 0, v1alpha1.SliceStrategyFixedTemplate, "32"),
		),
	}
	claim := mkClaim("c", "ns", v1alpha1.DriverName)

	var g Greedy
	want, err := g.Allocate(claim, base, NewAllocatedSet())
	if err != nil {
		t.Fatalf("baseline allocate: %v", err)
	}
	if want.Pool != "nodeA" {
		t.Fatalf("expected nodeA lexicographic winner; got %s/%s", want.Pool, want.Device)
	}

	rng := rand.New(rand.NewSource(42))
	for trial := 0; trial < 3; trial++ {
		shuf := make([]resourceapi.ResourceSlice, len(base))
		copy(shuf, base)
		rng.Shuffle(len(shuf), func(i, j int) { shuf[i], shuf[j] = shuf[j], shuf[i] })
		got, err := g.Allocate(claim, shuf, NewAllocatedSet())
		if err != nil {
			t.Fatalf("trial %d allocate: %v", trial, err)
		}
		if got.Pool != want.Pool || got.Device != want.Device {
			t.Errorf("trial %d: want %s/%s, got %s/%s", trial, want.Pool, want.Device, got.Pool, got.Device)
		}
	}
}

// TestBestFit_PicksSmallestSlack verifies BestFit prefers smaller-
// capacity devices. With a fleet mixing 16-core and 32-core capacities,
// the picker must select the 16-core device first.
func TestBestFit_PicksSmallestSlack(t *testing.T) {
	slices := []resourceapi.ResourceSlice{
		mkSlice("npu-dra-nodeA", "nodeA",
			mkDevice("nodeA-npu-0", 0, v1alpha1.SliceStrategyFixedTemplate, "32"),
		),
		mkSlice("npu-dra-nodeB", "nodeB",
			mkDevice("nodeB-npu-0", 0, v1alpha1.SliceStrategyFixedTemplate, "16"),
		),
	}
	claim := mkClaim("c", "ns", v1alpha1.DriverName)

	var b BestFit
	got, err := b.Allocate(claim, slices, NewAllocatedSet())
	if err != nil {
		t.Fatalf("BestFit allocate: %v", err)
	}
	if got.Pool != "nodeB" {
		t.Errorf("BestFit must prefer 16-core nodeB; got %s/%s", got.Pool, got.Device)
	}
}

// TestGreedy_EmptySliceList_ErrNoAvailableDevice verifies the empty
// fleet case yields the sentinel error.
func TestGreedy_EmptySliceList_ErrNoAvailableDevice(t *testing.T) {
	claim := mkClaim("c", "ns", v1alpha1.DriverName)
	var g Greedy
	_, err := g.Allocate(claim, nil, NewAllocatedSet())
	if !errors.Is(err, ErrNoAvailableDevice) {
		t.Errorf("want ErrNoAvailableDevice on empty list; got %v", err)
	}
}

// TestGreedy_AllSlicesFull_ErrNoAvailableDevice verifies that when
// every candidate is in the AllocatedSet, the picker returns the
// sentinel error rather than picking an already-taken device.
func TestGreedy_AllSlicesFull_ErrNoAvailableDevice(t *testing.T) {
	slices := []resourceapi.ResourceSlice{
		mkSlice("npu-dra-nodeA", "nodeA",
			mkDevice("nodeA-npu-0", 0, v1alpha1.SliceStrategyFixedTemplate, "32"),
			mkDevice("nodeA-npu-1", 1, v1alpha1.SliceStrategyFixedTemplate, "32"),
		),
	}
	taken := NewAllocatedSet()
	taken.Add("nodeA", "nodeA-npu-0")
	taken.Add("nodeA", "nodeA-npu-1")

	claim := mkClaim("c", "ns", v1alpha1.DriverName)
	var g Greedy
	_, err := g.Allocate(claim, slices, taken)
	if !errors.Is(err, ErrNoAvailableDevice) {
		t.Errorf("want ErrNoAvailableDevice when all taken; got %v", err)
	}
}

// TestGreedy_SingleSliceMultiDevice_PicksLowestIndex verifies the
// per-slice scan picks the lexicographically-first device when one
// slice carries multiple devices.
func TestGreedy_SingleSliceMultiDevice_PicksLowestIndex(t *testing.T) {
	slices := []resourceapi.ResourceSlice{
		mkSlice("npu-dra-nodeA", "nodeA",
			mkDevice("nodeA-npu-2", 2, v1alpha1.SliceStrategyFixedTemplate, "32"),
			mkDevice("nodeA-npu-0", 0, v1alpha1.SliceStrategyFixedTemplate, "32"),
			mkDevice("nodeA-npu-1", 1, v1alpha1.SliceStrategyFixedTemplate, "32"),
		),
	}
	claim := mkClaim("c", "ns", v1alpha1.DriverName)
	var g Greedy
	got, err := g.Allocate(claim, slices, NewAllocatedSet())
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if got.Device != "nodeA-npu-0" {
		t.Errorf("want nodeA-npu-0 lex-first; got %s", got.Device)
	}
}

// TestGreedy_HealthFilter verifies unhealthy devices are skipped.
func TestGreedy_HealthFilter(t *testing.T) {
	slices := []resourceapi.ResourceSlice{
		mkSlice("npu-dra-nodeA", "nodeA",
			mkDeviceUnhealthy("nodeA-npu-0", 0),
			mkDevice("nodeA-npu-1", 1, v1alpha1.SliceStrategyFixedTemplate, "32"),
		),
	}
	claim := mkClaim("c", "ns", v1alpha1.DriverName)
	var g Greedy
	got, err := g.Allocate(claim, slices, NewAllocatedSet())
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if got.Device != "nodeA-npu-1" {
		t.Errorf("must skip unhealthy nodeA-npu-0 and pick nodeA-npu-1; got %s", got.Device)
	}
}

// TestGreedy_SubClassWhole verifies the /whole sub-class filters out
// Dynamic-strategy devices.
func TestGreedy_SubClassWhole(t *testing.T) {
	slices := []resourceapi.ResourceSlice{
		mkSlice("npu-dra-nodeA", "nodeA",
			mkDevice("nodeA-dyn-0", 0, v1alpha1.SliceStrategyDynamic, "32"),
			mkDevice("nodeA-whole-0", 1, v1alpha1.SliceStrategyFixedTemplate, "32"),
		),
	}
	claim := mkClaim("c", "ns", v1alpha1.DriverName+".whole")
	var g Greedy
	got, err := g.Allocate(claim, slices, NewAllocatedSet())
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if got.Strategy != v1alpha1.SliceStrategyFixedTemplate {
		t.Errorf(".whole sub-class must pick FixedTemplate; got strategy=%s", got.Strategy)
	}
}

// TestGreedy_SubClassDynamic verifies the .dynamic sub-class filters
// out FixedTemplate devices.
func TestGreedy_SubClassDynamic(t *testing.T) {
	slices := []resourceapi.ResourceSlice{
		mkSlice("npu-dra-nodeA", "nodeA",
			mkDevice("nodeA-whole-0", 0, v1alpha1.SliceStrategyFixedTemplate, "32"),
			mkDevice("nodeA-dyn-0", 1, v1alpha1.SliceStrategyDynamic, "16"),
		),
	}
	claim := mkClaim("c", "ns", v1alpha1.DriverName+".dynamic")
	var g Greedy
	got, err := g.Allocate(claim, slices, NewAllocatedSet())
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if got.Strategy != v1alpha1.SliceStrategyDynamic {
		t.Errorf(".dynamic sub-class must pick Dynamic; got strategy=%s", got.Strategy)
	}
}

// TestGreedy_NoRequest verifies a claim with empty Requests yields a
// non-sentinel error (operator misconfiguration, not a transient
// "no device" condition the controller should retry on).
func TestGreedy_NoRequest(t *testing.T) {
	claim := mkClaim("c", "ns", v1alpha1.DriverName)
	claim.Spec.Devices.Requests = nil
	var g Greedy
	_, err := g.Allocate(claim, nil, NewAllocatedSet())
	if err == nil {
		t.Fatal("expected non-nil error on no-request claim")
	}
	if errors.Is(err, ErrNoAvailableDevice) {
		t.Errorf("no-request error should not be ErrNoAvailableDevice; got %v", err)
	}
}
