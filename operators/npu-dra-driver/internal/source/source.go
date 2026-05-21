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

// Package source abstracts where the npu-dra-driver gets its device
// inventory + topology.
//
// Phase 7 P7-T-004 (ADR-0011 §2) introduces this package, lifting the
// Source interface and NodeDevices/Event types OUT of internal/publisher/
// (where Phase 4 P4-T-005 first prototyped them) so multiple source
// implementations can coexist:
//
//   - mockjson.MockJSONSource (Phase 4-6 behavior preserved bit-for-bit)
//     reads configs/mock-data/set-a-small/{nodes,npupools,slices}.json
//   - realascend.RealAscendSource ships as a W1 stub returning
//     ErrNotImplemented; Phase 7 P7-T-101 lab-conditional lights up the
//     real-silicon body (npu-smi info -t topo parsing + DCMI health poll)
//
// The factory.NewSource constructor selects an impl by source-type tag
// (default "mock-json"). cmd/main.go wires the factory output into
// publisher.Publisher; publisher behavior is unchanged from Phase 4-6
// when sourceType="mock-json".
//
// **References**:
//   - ADR-0011 §2 — Source interface contract + factory pattern + lab gating
//   - ADR-0009 §4 — Partitionable Devices forward note (long-term path that
//     coexists with this Source abstraction)
//   - phase7-plan.md §3 P7-T-004 — task acceptance
//   - operators/npu-dra-driver/DESIGN.md §"Source interface architecture"
//   - Phase 4 P4-T-005 — original prototype in internal/publisher/
package source

import (
	"context"
	"errors"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/npu-dra-driver/api/v1alpha1"
)

// Source produces the Ocloud Ascend device inventory the publisher emits
// as ResourceSlices + answers per-node topology queries for downstream
// consumers (pool-operator HCCS aggregation, Phase 10 demo dashboards).
//
// ADR-0011 §2 fixes this contract:
//
//	List         — full inventory snapshot; publisher reconcile loop caller
//	Watch        — event stream for change-driven reconcile; nil ok
//	QueryTopology — per-node HCCS ring + NUMA view; pool-operator caller
//	               (Phase 7 W1: ResourceSlice attributes still primary;
//	                this method is opt-in for Phase 10 cross-validation)
//
// Implementations:
//
//   - mockjson.MockJSONSource — Phase 4-6 behavior preserved (reads
//     configs/mock-data/set-a-small/* JSON); QueryTopology builds the
//     HCCSTopology from the same parsed data
//   - realascend.RealAscendSource — Phase 7 W1 stub (ErrNotImplemented);
//     Phase 7 T101 lab-conditional body
type Source interface {
	// List returns the current device inventory grouped by node. Called
	// every reconcile tick (publisher default 30s) and on startup.
	// Implementations should be cheap to call repeatedly; the publisher
	// handles diff / upsert / delete-stale.
	List(ctx context.Context) ([]NodeDevices, error)

	// Watch returns an event channel signalling "inventory MAY have
	// changed, please reconcile". Implementations may return nil if they
	// have no efficient change notification (the publisher then relies on
	// the timer-driven tick alone). The channel is closed when ctx is
	// cancelled.
	Watch(ctx context.Context) <-chan Event

	// QueryTopology returns the HCCS ring + NUMA topology view for a
	// single node. Caller is pool-operator NPUPool.status.hccsTopology
	// aggregator (Phase 6 P6-T-003) and Phase 10 demo dashboards. Phase 7
	// pool-operator continues to read topology via ResourceSlice
	// attributes (primary path); this method is a forward hook for
	// real-silicon cross-validation in Phase 10.
	//
	// Returns ErrNotImplemented when the source backend cannot answer
	// (e.g. realascend stub in W1). Returns (nil, nil) when the node is
	// unknown to this source — caller distinguishes via the error value.
	QueryTopology(ctx context.Context, nodeName string) (*HCCSTopology, error)
}

// ErrNotImplemented is the sentinel error returned by Source method
// implementations that haven't shipped a real body yet (per ADR-0011 §2
// Source interface stub policy). Callers should errors.Is(err,
// ErrNotImplemented) before treating an error as fatal.
var ErrNotImplemented = errors.New("source: not implemented")

// NodeDevices groups Ocloud Ascend devices by host. The publisher emits
// one upstream resource.k8s.io/v1beta1.ResourceSlice per NodeDevices
// entry — pool name and slice name both derive from NodeName.
type NodeDevices struct {
	// NodeName is the Kubernetes Node hostname. mockjson reads this
	// from npus.json's `nodeName` field; realascend reads from
	// $NODE_NAME / kubelet downward API.
	NodeName string

	// Devices is the typed-view list of Ocloud Ascend devices on this
	// node. One entry per physical NPU. Order is stable across reconcile
	// passes to make ResourceSlice diffs idempotent.
	Devices []v1alpha1.AscendDevice
}

// Event is a change notification emitted by Source.Watch. Phase 4
// (and Phase 7 — schema preserved) only distinguishes "the inventory
// might have changed, please reconcile" — the publisher itself does the
// diff. Phase 5+ may extend with per-device granularity if hotplug or
// partial updates become important.
type Event struct {
	// Source is a free-form tag identifying which Source produced this
	// event ("mock-json" / "real-ascend") for log correlation.
	Source string

	// Reason is a short machine-readable hint about why this event fired
	// ("file-modified" / "hotplug" / "tick"). Best-effort; the publisher
	// treats all events as "reconcile now".
	Reason string
}

// HCCSTopology is the per-node ring + NUMA view returned by
// Source.QueryTopology (Phase 7 P7-T-004 / ADR-0011 §2).
//
// Phase 7 W1 mockjson populates this from the parsed JSON (npus.json's
// hccsRing + numaNode fields per device). Phase 7 T101 realascend
// populates it from `npu-smi info -t topo` parsing (per ADR-0010 §7
// forward note).
//
// Schema is intentionally narrow — the canonical HCCS topology
// representation in this project is the ResourceSlice attribute
// (npu.huawei.com/hccs_ring + npu.huawei.com/numa_node) per ADR-0010 §5.
// This struct is the QUERY-side counterpart for callers that want a
// pre-aggregated view without re-walking ResourceSlices.
type HCCSTopology struct {
	// NodeName echoes the QueryTopology input for caller convenience.
	NodeName string

	// Rings maps HCCS ring ID → device names (in stable order).
	// Empty map = no HCCS-aware devices on this node (single-card NUMA
	// node etc.). nil = source backend doesn't expose topology
	// (callers should treat as "no information" not "no rings").
	Rings map[int32][]string

	// NUMA maps NUMA node ID → device names (in stable order). Same
	// nil/empty semantics as Rings.
	NUMA map[int32][]string
}
