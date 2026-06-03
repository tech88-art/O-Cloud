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

// Package realascend is the Source impl that talks to real Ascend silicon
// via npu-smi (+ DCMI health) to discover device inventory + HCCS topology.
//
// Phase 13 status (P13-T-101 · ADR-0024 §2 Decision A/B · lab-gating FLIP):
//
//	The Phase 7 W1 stub (all methods returned ErrNotImplemented) is now a
//	real body. ADR-0011 §3 lab-gating default-defer ran 6 consecutive
//	phases (P7-P12); the 2026-06-02 lab-ready signal flipped it to
//	light-up (ADR-0016 §2 Decision B trigger 2 FIRED → ADR-0023 §2
//	Decision C → ADR-0024 §2 Decision A).
//
//	The body reads from a npusmi.Client (ExecClient shells out to npu-smi
//	in production; FakeClient drives table-tests + dev mode without a lab):
//
//	  - List         — npusmi QueryTopo enumerates devices + QueryDeviceInfo
//	    enriches per-device capacity → []source.NodeDevices (one NodeDevices
//	    per node; the real-Ascend DaemonSet runs one pod per node)
//	  - QueryTopology — npusmi QueryTopo (→ ParseTopoMatrix) → HCCS ring +
//	    NUMA maps keyed by the same device names List emits
//	  - Watch         — npusmi QueryHealth poll loop; emits an Event when a
//	    device's health changes. Degrades to a closed channel (publisher
//	    falls back to its timer-tick) when the lab driver doesn't expose
//	    a health query (ADR-0024 §2 Decision B fallback)
//
// **Decoupling-seam invariant** (ADR-0024 §2 Decision G): all real-silicon
// logic lives BELOW the source.Source seam (here + the npusmi package).
// The seam-above shared layer (publisher / pool-operator aggregation /
// backend) consumes the same source.NodeDevices / source.HCCSTopology
// shapes the mockjson source returns — there is no `if real {}` branch in
// the shared layer. demo profile (sourceType=mock-json) regression is
// unaffected.
//
// **No cgo** (ADR-0024 §4(c)): the Phase 13 body uses npu-smi via os/exec
// only (npusmi.ExecClient). It contains no libdcmi cgo binding, so the
// default CGO_ENABLED=0 GOARCH=arm64 cross-compile (ADR-0020) stays green
// without build-tag isolation. A future libdcmi binding (Config.Mode
// "library") would gate behind `//go:build dcmi` per ADR-0024 §4(c).
//
// **References**:
//   - ADR-0024 §2 Decision A/B/G (lab activation + real body + seam invariant)
//   - ADR-0011 §2 Source interface contract + §3 lab gating policy (flipped)
//   - phase13-plan.md §4 P13-T-101
//   - operators/npu-dra-driver/internal/source/realascend/npusmi/ (Client +
//     ParseTopoMatrix + board parser)
package realascend

import (
	"context"
	"os"
	"sort"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/npu-dra-driver/api/v1alpha1"
	"github.com/tech88-art/O-Cloud/operators/npu-dra-driver/internal/source"
	"github.com/tech88-art/O-Cloud/operators/npu-dra-driver/internal/source/realascend/npusmi"
)

// defaultWatchPollInterval is the health poll cadence for Watch when
// Config.WatchPollInterval is unset. Matches the mockjson watch tempo
// order-of-magnitude; the publisher's own reconcile tick (default 30s)
// is the primary refresh path, so a coarse health poll is sufficient.
const defaultWatchPollInterval = 30 * time.Second

// defaultSliceAICoreCapacity is the per-device AI-core capacity used when
// npu-smi board info does not surface an AI-core count (defensive — the
// Ascend 910B chip total). Mirrors the mockjson fallback so ResourceSlice
// capacity is identical across demo/real profiles for the same chip.
const defaultSliceAICoreCapacity int64 = 32

// Config captures RealAscendSource construction parameters.
type Config struct {
	// Mode selects how the source backs its calls. Phase 13 ships "exec"
	// (shell out to npu-smi via npusmi.ExecClient). "library" (libdcmi cgo
	// binding) is reserved for a future build-tag-gated mode (ADR-0024
	// §4(c)). Empty defaults to "exec".
	Mode string

	// NodeID is the Kubernetes Node hostname stamped on emitted devices.
	// Empty → resolved from the NODE_NAME environment variable (the
	// DaemonSet downward-API convention). Tests set it directly.
	NodeID string

	// BinaryPath is the absolute path to npu-smi. Empty → ExecClient looks
	// it up on PATH (lab deploys typically mount
	// /usr/local/Ascend/driver/tools/npu-smi via hostPath).
	BinaryPath string

	// WatchPollInterval is the health-poll cadence for Watch. Zero →
	// defaultWatchPollInterval.
	WatchPollInterval time.Duration
}

// RealAscendSource implements source.Source against real Ascend silicon.
type RealAscendSource struct {
	cfg          Config
	client       npusmi.Client
	nodeID       string
	pollInterval time.Duration
}

// New constructs a RealAscendSource backed by a npusmi.ExecClient (the
// production path). NodeID falls back to $NODE_NAME when Config.NodeID is
// empty. Construction never fails — an unreachable npu-smi binary surfaces
// as an error at first List/QueryTopology call (npusmi.ErrNoCommand), so
// the publisher reconcile loop can soft-fail loudly rather than crash-loop
// the DaemonSet on a node that is mid-driver-install.
func New(cfg Config) *RealAscendSource {
	nodeID := cfg.NodeID
	if nodeID == "" {
		nodeID = os.Getenv("NODE_NAME")
	}
	client := &npusmi.ExecClient{BinaryPath: cfg.BinaryPath, NodeID: nodeID}
	return newWithClient(cfg, client, nodeID)
}

// newWithClient is the test seam: it injects an arbitrary npusmi.Client
// (FakeClient in table-tests) so the real body can be exercised without a
// lab npu-smi binary. Production code calls New.
func newWithClient(cfg Config, client npusmi.Client, nodeID string) *RealAscendSource {
	interval := cfg.WatchPollInterval
	if interval <= 0 {
		interval = defaultWatchPollInterval
	}
	return &RealAscendSource{cfg: cfg, client: client, nodeID: nodeID, pollInterval: interval}
}

// List enumerates the node's Ascend devices via npu-smi and projects each
// into a v1alpha1.AscendDevice. QueryTopo provides the device set + HCCS
// ring + NUMA node; QueryDeviceInfo enriches per-device AI-core capacity.
// A per-device QueryDeviceInfo failure is soft (the device is still
// emitted with the fallback capacity) so a single mid-reset NPU does not
// blank the whole node's ResourceSlice.
func (s *RealAscendSource) List(ctx context.Context) ([]source.NodeDevices, error) {
	entries, err := s.client.QueryTopo(ctx)
	if err != nil {
		return nil, err
	}

	byNode := make(map[string][]v1alpha1.AscendDevice)
	for _, e := range entries {
		node := e.NodeID
		if node == "" {
			node = s.nodeID
		}
		dev := v1alpha1.AscendDevice{
			Name:                deviceName(node, int64(e.DeviceID)),
			Index:               int64(e.DeviceID),
			Health:              healthString(e.Health),
			SliceStrategy:       v1alpha1.SliceStrategyFixedTemplate,
			AICores:             0, // FixedTemplate → 0 (dynamic slicing is opt-in via NPUSliceTemplate)
			NUMANode:            int64(e.NumaNode),
			HCCSRing:            int64(e.Ring),
			SliceAICoreCapacity: s.capacityFor(ctx, e),
		}
		byNode[node] = append(byNode[node], dev)
	}

	out := make([]source.NodeDevices, 0, len(byNode))
	for node, devs := range byNode {
		sort.Slice(devs, func(i, j int) bool { return devs[i].Index < devs[j].Index })
		out = append(out, source.NodeDevices{NodeName: node, Devices: devs})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NodeName < out[j].NodeName })
	return out, nil
}

// capacityFor resolves the per-device AI-core capacity from npu-smi board
// info, falling back to defaultSliceAICoreCapacity when the board query
// fails or reports a non-positive count. Soft per-device (see List).
func (s *RealAscendSource) capacityFor(ctx context.Context, e npusmi.TopoEntry) resource.Quantity {
	if info, err := s.client.QueryDeviceInfo(ctx, e.DeviceID); err == nil && info != nil && info.AICores > 0 {
		return *resource.NewQuantity(int64(info.AICores), resource.DecimalSI)
	}
	return *resource.NewQuantity(defaultSliceAICoreCapacity, resource.DecimalSI)
}

// Watch polls per-device health via npu-smi and emits a reconcile Event
// when any device's health changes. When the lab driver does not expose a
// health query (QueryHealth errors), Watch degrades to a closed channel
// and the publisher relies on its timer-driven tick (ADR-0024 §2 Decision
// B fallback). Channel is closed when ctx is cancelled.
func (s *RealAscendSource) Watch(ctx context.Context) <-chan source.Event {
	ch := make(chan source.Event, 1)
	go func() {
		defer close(ch)

		entries, err := s.client.QueryTopo(ctx)
		if err != nil {
			// Cannot enumerate devices → no health watch; publisher tick
			// is the only refresh path. Closing ch (deferred) is the
			// non-blocking degrade.
			return
		}
		last := make(map[int]npusmi.HealthState, len(entries))
		for _, e := range entries {
			last[e.DeviceID] = npusmi.HealthUnknown
		}

		t := time.NewTicker(s.pollInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				changed := false
				for id := range last {
					h, herr := s.client.QueryHealth(ctx, id)
					if herr != nil {
						// Health query unsupported on this driver/version →
						// degrade to closed channel (ADR-0024 §2 Decision B).
						return
					}
					if h != last[id] {
						last[id] = h
						changed = true
					}
				}
				if changed {
					select {
					case ch <- source.Event{Source: "real-ascend", Reason: "health-change"}:
					default:
					}
				}
			}
		}
	}()
	return ch
}

// QueryTopology returns the HCCS ring + NUMA view for one node, built from
// `npu-smi info -t topo` (via npusmi.QueryTopo → ParseTopoMatrix). Returns
// (nil, nil) when nodeName is not this source's node — matching the
// mockjson "unknown node" semantics so callers distinguish via the error
// value (ADR-0011 §2 + ADR-0024 §2 Decision C).
func (s *RealAscendSource) QueryTopology(ctx context.Context, nodeName string) (*source.HCCSTopology, error) {
	entries, err := s.client.QueryTopo(ctx)
	if err != nil {
		return nil, err
	}

	rings := make(map[int32][]string)
	numa := make(map[int32][]string)
	found := false
	for _, e := range entries {
		node := e.NodeID
		if node == "" {
			node = s.nodeID
		}
		if node != nodeName {
			continue
		}
		found = true
		dn := deviceName(node, int64(e.DeviceID))
		rings[e.Ring] = append(rings[e.Ring], dn)
		numa[e.NumaNode] = append(numa[e.NumaNode], dn)
	}
	if !found {
		return nil, nil
	}

	for k := range rings {
		sort.Strings(rings[k])
	}
	for k := range numa {
		sort.Strings(numa[k])
	}
	return &source.HCCSTopology{NodeName: nodeName, Rings: rings, NUMA: numa}, nil
}

// deviceName mirrors mockjson.deviceName so demo/real profiles emit
// identical device names for the same (node, index) — keeps backend
// topology aggregation (T103) and ResourceSlice diffs profile-agnostic.
func deviceName(node string, index int64) string {
	return node + "-npu-" + strconv.FormatInt(index, 10)
}

// healthString maps the npusmi health enum to the v1alpha1 Health string.
// The enum values happen to share spelling (Healthy / Unhealthy /
// Unknown) but the mapping is explicit so a future divergence in either
// package is caught here rather than silently mis-projected.
func healthString(h npusmi.HealthState) string {
	switch h {
	case npusmi.HealthHealthy:
		return v1alpha1.HealthHealthy
	case npusmi.HealthUnhealthy:
		return v1alpha1.HealthUnhealthy
	default:
		return v1alpha1.HealthUnknown
	}
}

// Compile-time assertion that RealAscendSource satisfies source.Source.
var _ source.Source = &RealAscendSource{}
