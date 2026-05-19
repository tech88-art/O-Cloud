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

// Package publisher implements the ResourceSlice publication loop for the
// npu-dra-driver.
//
// Phase 4 T005 ships:
//   - Source interface (List + Watch) abstracting where Ocloud Ascend devices
//     come from (simulator, future real-Ascend driver)
//   - SimulatorSource reading configs/mock-data/set-a-small/npus.json
//   - Publisher running a reconcile loop: list → diff → upsert → delete stale
//     → requeue 30s
//
// The publisher is encapsulated inside operators/npu-dra-driver/internal/ so
// no public symbol leaks outside this module (P4-T-004 acceptance).
//
// References:
//   - docs/phase4-plan.md §3 P4-T-005 — task acceptance
//   - docs/adr/0001-phase0-key-decisions.md §5 v3 — dual-path roadmap
//   - configs/mock-data/set-a-small/npus.json — data source
//   - operators/npu-dra-driver/api/v1alpha1/ — AscendDevice typed view
package publisher

import (
	"context"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/npu-dra-driver/api/v1alpha1"
)

// NodeDevices groups Ocloud Ascend devices by host. The publisher emits one
// upstream resource.k8s.io/v1beta1.ResourceSlice per NodeDevices entry —
// pool name and slice name both derive from NodeName.
type NodeDevices struct {
	// NodeName is the Kubernetes Node hostname. The simulator reads this
	// from npus.json's `nodeName` field; the future real-Ascend source
	// reads it from $NODE_NAME / kubelet downward API.
	NodeName string

	// Devices is the typed-view list of Ocloud Ascend devices on this node.
	// One entry per physical NPU. Order is stable across reconcile passes
	// to make ResourceSlice diffs idempotent.
	Devices []v1alpha1.AscendDevice
}

// Source produces the Ocloud Ascend device inventory the publisher emits as
// ResourceSlices. Implementations:
//
//   - SimulatorSource (this file's sibling source_simulator.go) — reads
//     a static JSON file matching the configs/mock-data/set-a-small/npus.json
//     shape; the Phase 4 default.
//   - (Phase 5+) RealAscendSource — talks to npu-smi / DCMI to discover
//     local NPUs; not in scope for Phase 4.
type Source interface {
	// List returns the current device inventory grouped by node. It is
	// called every reconcile tick (30s) and on demand at startup. Implementations
	// should be cheap to call repeatedly; the publisher does the diff,
	// upsert, and delete-stale work.
	List(ctx context.Context) ([]NodeDevices, error)

	// Watch returns a channel of Event notifications. The publisher uses
	// this to short-circuit the reconcile timer when the source signals a
	// known change (file rename for the simulator; hotplug event for the
	// real driver). Implementations may return nil if they have no efficient
	// change notification — the publisher then relies on the 30s tick alone.
	Watch(ctx context.Context) <-chan Event
}

// Event is a minimal change notification emitted by Source.Watch. Phase 4
// only distinguishes "the inventory might have changed, please reconcile" —
// the publisher itself does the diff. Phase 5+ may extend with per-device
// granularity if hotplug or partial updates become important.
type Event struct {
	// Source is a free-form tag identifying which Source produced this
	// event ("simulator" / "real-ascend") for log correlation.
	Source string

	// Reason is a short machine-readable hint about why this event fired
	// ("file-modified" / "hotplug" / "tick"). Best-effort; the publisher
	// treats all events as "reconcile now".
	Reason string
}
