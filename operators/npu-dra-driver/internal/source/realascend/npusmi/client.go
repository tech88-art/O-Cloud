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

// Package npusmi is the Phase 7 W1 scaffold for the npu-smi / DCMI
// query subsystem used by realascend.RealAscendSource (lab-conditional
// P7-T-101 body).
//
// Phase 7 W1 ships (this commit · ADR-0011 §2 §3 + phase7-plan §3 T005):
//
//   - Client interface (3 methods) — what callers expect from a real or
//     fake npu-smi binding
//   - FakeClient — returns canned data from testdata/*.txt; used by
//     parser tests + future T101 dev mode that doesn't have a lab cluster
//   - ExecClient — shells out to `npu-smi` binary; only consulted when
//     realascend.Config.Mode == "exec" (default in chart values when
//     sourceType=real-ascend). Phase 7 W1 ships the type-only scaffold;
//     Phase 7 T101 hardens the os/exec wrapper with context timeout +
//     non-zero exit handling
//   - parse.go — `npu-smi info -t topo` text matrix parser (5 unit
//     test cases · 8-card + 16-card fixtures · empty + unhealthy +
//     malformed edges)
//
// **Operative**: Phase 7 W1 CI does NOT exercise any real npu-smi binary —
// FakeClient is used by tests; ExecClient compiles but is unreachable
// from the W1 RealAscendSource stub (which returns ErrNotImplemented).
// Phase 7 T101 lab body wires ExecClient through realascend.RealAscendSource.
//
// **References**:
//   - ADR-0010 §7 forward note row 1 ("替换 npu-dra-driver
//     Source.RealAscend.queryTopology() · 走 npu-smi info -t topo 解析
//     HCCS group 拓扑")
//   - ADR-0011 §2 Source interface + lab gating policy
//   - phase7-plan.md §3 P7-T-005 + §4 P7-T-101
//   - upstream Huawei docs: npu-smi info -t topo output format
package npusmi

import (
	"context"
	"errors"
)

// HealthState mirrors the AscendDevice Health enum but lives at the
// npu-smi query layer for caller convenience (avoids importing
// api/v1alpha1 from this leaf package).
type HealthState string

const (
	HealthHealthy   HealthState = "Healthy"
	HealthUnhealthy HealthState = "Unhealthy"
	HealthUnknown   HealthState = "Unknown"
)

// TopoEntry is one row of the per-node Ascend topology view, as
// composed from `npu-smi info -t topo` (provides Ring) + `npu-smi info
// -t board -i <id>` (provides NumaNode + Health). The Phase 7 W1 parser
// (parse.go) populates Ring from the topo matrix; NumaNode + Health
// default to 0 / Unknown when not provided by the input.
//
// Phase 7 T101 lab body composes a complete TopoEntry per device by
// running multiple npu-smi queries and merging results.
type TopoEntry struct {
	// NodeID is the Kubernetes Node hostname this entry belongs to.
	// Real-cluster T101 reads from $NODE_NAME / kubelet downward API;
	// FakeClient + parser tests use the fixture file name as proxy.
	NodeID string

	// DeviceID is the npu-smi index (0..N-1 per node, where N is the
	// number of NPUs on the host · typically 8 for 910B server · 16 for
	// 910B-pro).
	DeviceID int

	// Ring is the HCCS ring assignment derived from the topo matrix —
	// two devices share a Ring when their topo entry is "HCCS" (per the
	// upstream output legend); ring IDs are assigned by walking
	// connected components.
	Ring int32

	// NumaNode is the host NUMA node binding (0..M-1 per host). Default
	// 0 when the input does not surface NUMA. Real-cluster T101 reads
	// from `numactl --hardware` or sysfs.
	NumaNode int32

	// Health is the device state from `npu-smi info -t board -i <id>`
	// or `dcmi_get_device_health`. Default HealthUnknown for the W1
	// parser scaffold.
	Health HealthState
}

// DeviceInfo is the per-device detail rolled up from one or more
// npu-smi queries. Phase 7 W1 ships the type only; T101 lab body
// populates the body of ExecClient.QueryDeviceInfo.
type DeviceInfo struct {
	DeviceID       int
	ChipName       string // e.g. "Ascend 910B"
	AICores        int32
	MemorySizeMiB  int64
	NumaNode       int32
	Health         HealthState
	DriverVersion  string
	FirmwareVersion string
}

// Client is the abstraction over real / fake npu-smi access. Phase 7
// P7-T-005 (this commit) ships the interface + two impls (FakeClient
// reading testdata fixtures · ExecClient shelling out at T101 time).
//
// The realascend.RealAscendSource (Phase 7 T101 lab body) consumes this
// interface to populate Source.List / Source.Watch / Source.QueryTopology
// results.
type Client interface {
	// QueryTopo returns the per-device topology view for ALL devices on
	// the current node (the Client implementation knows the node it's
	// bound to · NodeID per entry is the same value). Phase 7 W1 parser
	// builds this from `npu-smi info -t topo` matrix; NumaNode + Health
	// fields default to zero / Unknown unless populated by the caller.
	QueryTopo(ctx context.Context) ([]TopoEntry, error)

	// QueryDeviceInfo returns per-device detail (chip / AI-cores /
	// memory / numa / health / driver versions). Phase 7 W1 ships type
	// only; T101 lab body wires real exec call.
	QueryDeviceInfo(ctx context.Context, devID int) (*DeviceInfo, error)

	// QueryHealth returns the device health state. Cheaper than full
	// QueryDeviceInfo · called by Source.Watch loop at high cadence
	// (Phase 7 T101).
	QueryHealth(ctx context.Context, devID int) (HealthState, error)
}

// ErrNoCommand is the sentinel returned when ExecClient cannot find
// the npu-smi binary on the host PATH or at the configured path. Phase
// 7 W1: this is the most common failure mode for a fresh kind cluster
// that picked sourceType=real-ascend by accident.
var ErrNoCommand = errors.New("npusmi: npu-smi binary not found")

// ErrParse is returned when the topo matrix parser fails to interpret
// the input (truncated output / unexpected legend changes / 0-row
// matrix). Phase 7 T101 lab body should retry once on this error
// before surfacing — npu-smi output occasionally races with device
// hot-reset.
var ErrParse = errors.New("npusmi: parse error")
