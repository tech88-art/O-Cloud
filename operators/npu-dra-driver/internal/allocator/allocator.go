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

// Package allocator picks an NPU device for a v1beta1.ResourceClaim that
// requests the npu-dra-driver class family.
//
// The Allocator interface is intentionally small: take a claim, the live
// ResourceSlices (filtered to this driver), and the set of already-
// allocated devices; return a single Allocation describing the chosen
// device or ErrNoAvailableDevice when nothing fits. T002 (Phase 5)
// ships the greedy first-fit implementation; T003 adds the best-fit
// variant + table-driven tests; Phase 6 introduces a topology-aware
// scoring layer (NUMA + HCCS — see ADR-0009 §6 outcome c).
//
// Determinism is a contract requirement: identical inputs must produce
// identical outputs across runs. Greedy enforces this via lexicographic
// sort on slice + device names before scanning. Without determinism, the
// kind smoke (T106) and the per-claim allocation audit log
// (NPUSliceAllocation in T005) cannot be diffed reliably across CI runs.
package allocator

import (
	"errors"

	resourceapi "k8s.io/api/resource/v1beta1"
)

// ErrNoAvailableDevice is returned when no slice carries a device that
// matches the claim's request constraints and is not already allocated.
// Callers (the claim controller) treat this as a soft failure: requeue
// the claim on the workqueue and try again on the next tick (publisher
// re-emits, other claims release, etc.).
var ErrNoAvailableDevice = errors.New("allocator: no available device matches claim request")

// Allocation describes one allocator decision. The fields map 1:1 onto
// resource.k8s.io/v1beta1.DeviceRequestAllocationResult so the claim
// controller can copy them into ResourceClaim.Status.Allocation.Devices
// .Results without further translation.
type Allocation struct {
	// Driver is the DRA driver name. Always v1alpha1.DriverName for this
	// allocator; carried explicitly so future per-driver federation work
	// (multi-driver claim consumers) does not need to re-derive it.
	Driver string

	// Pool is the ResourceSlice's ResourcePool.Name — by convention the
	// node hostname for the simulator publisher (see
	// publisher.sliceNameForNode + publisher.buildSlice).
	Pool string

	// Device is the per-slice device name (Device.Name on the upstream
	// type). DNS label.
	Device string

	// Request is the claim's request name (DeviceRequest.Name) that this
	// allocation satisfies. Phase 5 supports single-request claims; the
	// allocator picks the first request and only fills that one.
	Request string

	// AICores carries the Ascend-specific AI-core count attribute of the
	// chosen device. Zero when the device's slice-strategy is
	// FixedTemplate (per-template count looked up from CRD). Useful for
	// the NPUSliceAllocation controller (T005) and the PD Router
	// webhook (T103) without re-parsing the ResourceSlice attributes.
	AICores int64

	// Strategy is the chosen device's slice-strategy attribute
	// (FixedTemplate / Dynamic). Same rationale as AICores — propagated
	// for downstream consumers.
	Strategy string

	// NodeName is the slice's NodeName field (publisher sets it from the
	// device's node-affine pool). Phase 6 scheduler-plugin uses this for
	// pod-to-NPU co-location scoring; Phase 5 propagates it into the
	// NPUSliceAllocation status for audit-log purposes.
	NodeName string
}

// AllocatedSet is the deduplicated set of (pool, device) tuples that the
// allocator must not pick. Built from the live ResourceClaim list (see
// ComputeAllocatedFromClaims) — Phase 5 single source of truth. T005
// adds NPUSliceAllocation entries as a parallel index but the canonical
// "is this device taken" answer always lives in
// ResourceClaim.Status.Allocation.
type AllocatedSet map[allocatedKey]struct{}

type allocatedKey struct {
	Pool   string
	Device string
}

// NewAllocatedSet returns an empty AllocatedSet ready to receive Add calls.
func NewAllocatedSet() AllocatedSet {
	return make(AllocatedSet)
}

// Add records (pool, device) as already-allocated. Idempotent.
func (a AllocatedSet) Add(pool, device string) {
	a[allocatedKey{Pool: pool, Device: device}] = struct{}{}
}

// Has reports whether (pool, device) is in the set.
func (a AllocatedSet) Has(pool, device string) bool {
	_, ok := a[allocatedKey{Pool: pool, Device: device}]
	return ok
}

// Len returns the number of (pool, device) tuples in the set.
func (a AllocatedSet) Len() int {
	return len(a)
}

// Allocator picks a single device for the claim from the candidate
// ResourceSlices, excluding everything already in the AllocatedSet. The
// returned Allocation refers to the chosen (pool, device) — caller is
// responsible for actually writing the status.allocation field on the
// claim.
//
// Implementations must be:
//   - Pure: no side effects, no client calls. Inputs fully drive output.
//   - Deterministic: same input → same output across processes.
//   - Free of panics on bad inputs; return ErrNoAvailableDevice or a
//     wrapped error instead.
type Allocator interface {
	Allocate(claim resourceapi.ResourceClaim, slices []resourceapi.ResourceSlice, allocated AllocatedSet) (*Allocation, error)
}
