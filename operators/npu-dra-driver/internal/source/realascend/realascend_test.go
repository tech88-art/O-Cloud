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

package realascend

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/npu-dra-driver/api/v1alpha1"
	"github.com/tech88-art/O-Cloud/operators/npu-dra-driver/internal/source"
	"github.com/tech88-art/O-Cloud/operators/npu-dra-driver/internal/source/realascend/npusmi"
)

// fakeSource builds a RealAscendSource backed by the npusmi FakeClient for
// the given fixture + node — the offline (no-lab) verification path per
// ADR-0024 §3 (functional correctness on captured fixtures; real-silicon
// "对接" stamp is lab-gated).
func fakeSource(fixture, node string) *RealAscendSource {
	fc := &npusmi.FakeClient{Fixture: fixture, NodeID: node}
	return newWithClient(Config{Mode: "exec"}, fc, node)
}

// TestRealAscendList asserts List no longer returns ErrNotImplemented and
// projects the npu-smi fixture into AscendDevice values grouped by node.
func TestRealAscendList(t *testing.T) {
	cases := []struct {
		name      string
		fixture   string
		node      string
		wantCount int
		ringOf    func(i int) int64
		numaOf    func(i int) int64
	}{
		{
			name: "8card", fixture: "8card", node: "node-a", wantCount: 8,
			ringOf: func(i int) int64 {
				if i >= 4 {
					return 1
				}
				return 0
			},
			numaOf: func(i int) int64 {
				if i >= 4 {
					return 1
				}
				return 0
			},
		},
		{
			// 16card fixture: 4 HCCS rings of 4 (ring=i/4) on a 2-socket
			// host (8 NPUs per NUMA node → numa=i/8).
			name: "16card", fixture: "16card", node: "node-pro", wantCount: 16,
			ringOf: func(i int) int64 { return int64(i / 4) },
			numaOf: func(i int) int64 { return int64(i / 8) },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := fakeSource(tc.fixture, tc.node)
			nds, err := src.List(context.Background())
			if err != nil {
				t.Fatalf("List: unexpected error %v", err)
			}
			if errors.Is(err, source.ErrNotImplemented) {
				t.Fatal("List still returns ErrNotImplemented — body not wired")
			}
			if len(nds) != 1 {
				t.Fatalf("len(NodeDevices) = %d, want 1 (single node per DaemonSet pod)", len(nds))
			}
			nd := nds[0]
			if nd.NodeName != tc.node {
				t.Fatalf("NodeName = %q, want %q", nd.NodeName, tc.node)
			}
			if len(nd.Devices) != tc.wantCount {
				t.Fatalf("len(Devices) = %d, want %d", len(nd.Devices), tc.wantCount)
			}
			for i, d := range nd.Devices {
				if d.Index != int64(i) {
					t.Fatalf("Devices[%d].Index = %d, want %d (sorted ascending)", i, d.Index, i)
				}
				wantName := tc.node + "-npu-" + strconv.Itoa(i)
				if d.Name != wantName {
					t.Fatalf("Devices[%d].Name = %q, want %q", i, d.Name, wantName)
				}
				if d.HCCSRing != tc.ringOf(i) {
					t.Fatalf("Devices[%d].HCCSRing = %d, want %d", i, d.HCCSRing, tc.ringOf(i))
				}
				if d.NUMANode != tc.numaOf(i) {
					t.Fatalf("Devices[%d].NUMANode = %d, want %d", i, d.NUMANode, tc.numaOf(i))
				}
				if d.SliceStrategy != v1alpha1.SliceStrategyFixedTemplate {
					t.Fatalf("Devices[%d].SliceStrategy = %q, want FixedTemplate", i, d.SliceStrategy)
				}
				if d.AICores != 0 {
					t.Fatalf("Devices[%d].AICores = %d, want 0 (FixedTemplate)", i, d.AICores)
				}
				// FakeClient.QueryDeviceInfo reports AICores=32 → capacity 32.
				if got := d.SliceAICoreCapacity.Value(); got != 32 {
					t.Fatalf("Devices[%d].SliceAICoreCapacity = %d, want 32 (from board info)", i, got)
				}
				if d.Health != v1alpha1.HealthHealthy && d.Health != v1alpha1.HealthUnhealthy && d.Health != v1alpha1.HealthUnknown {
					t.Fatalf("Devices[%d].Health = %q, want a valid Health enum", i, d.Health)
				}
			}
		})
	}
}

// TestRealAscendQueryTopology asserts QueryTopology builds HCCS ring +
// NUMA maps for the source's node and returns (nil,nil) for unknown nodes.
func TestRealAscendQueryTopology(t *testing.T) {
	src := fakeSource("8card", "node-a")

	topo, err := src.QueryTopology(context.Background(), "node-a")
	if err != nil {
		t.Fatalf("QueryTopology: unexpected error %v", err)
	}
	if errors.Is(err, source.ErrNotImplemented) {
		t.Fatal("QueryTopology still returns ErrNotImplemented — body not wired")
	}
	if topo == nil {
		t.Fatal("QueryTopology returned nil topo for a known node")
	}
	if topo.NodeName != "node-a" {
		t.Fatalf("topo.NodeName = %q, want node-a", topo.NodeName)
	}
	// 8card → ring 0 = {npu0..3}, ring 1 = {npu4..7}.
	if len(topo.Rings) != 2 {
		t.Fatalf("len(Rings) = %d, want 2", len(topo.Rings))
	}
	if got := len(topo.Rings[0]); got != 4 {
		t.Fatalf("Rings[0] has %d devices, want 4", got)
	}
	if got := len(topo.Rings[1]); got != 4 {
		t.Fatalf("Rings[1] has %d devices, want 4", got)
	}
	if topo.Rings[0][0] != "node-a-npu-0" {
		t.Fatalf("Rings[0][0] = %q, want node-a-npu-0 (stable sort)", topo.Rings[0][0])
	}
	if len(topo.NUMA) != 2 {
		t.Fatalf("len(NUMA) = %d, want 2", len(topo.NUMA))
	}

	// Unknown node → (nil, nil) per ADR-0011 §2 semantics.
	other, err := src.QueryTopology(context.Background(), "not-this-node")
	if err != nil {
		t.Fatalf("QueryTopology(unknown): unexpected error %v", err)
	}
	if other != nil {
		t.Fatalf("QueryTopology(unknown) = %+v, want nil", other)
	}
}

// TestRealAscendWatchClosesOnCancel asserts Watch's channel closes when
// the context is cancelled (no goroutine leak).
func TestRealAscendWatchClosesOnCancel(t *testing.T) {
	src := fakeSource("8card", "node-a")
	src.pollInterval = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	ch := src.Watch(ctx)
	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			// An event may legitimately arrive (Unknown→fixture health)
			// before close; drain until closed.
			for range ch {
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Watch channel did not close within 2s of ctx cancel")
	}
}

// TestRealAscendWatchDegradesOnHealthError asserts Watch closes its
// channel (degrades to publisher-tick) when the driver does not expose a
// health query (ADR-0024 §2 Decision B fallback).
func TestRealAscendWatchDegradesOnHealthError(t *testing.T) {
	c := &healthErrClient{FakeClient: &npusmi.FakeClient{Fixture: "8card", NodeID: "node-a"}}
	src := newWithClient(Config{Mode: "exec"}, c, "node-a")
	src.pollInterval = 5 * time.Millisecond

	ch := src.Watch(context.Background())
	select {
	case _, ok := <-ch:
		// Drain to closed — the degrade path returns (closes ch) on the
		// first QueryHealth error after a tick.
		if ok {
			for range ch {
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not degrade to a closed channel on health error within 2s")
	}
}

// TestRealAscendListPropagatesTopoError asserts a QueryTopo failure (e.g.
// npu-smi not installed / ErrNoCommand) surfaces from List rather than
// silently returning an empty inventory.
func TestRealAscendListPropagatesTopoError(t *testing.T) {
	c := &topoErrClient{}
	src := newWithClient(Config{Mode: "exec"}, c, "node-a")
	if _, err := src.List(context.Background()); err == nil {
		t.Fatal("List should propagate the QueryTopo error, got nil")
	}
}

// TestNewResolvesNodeFromEnv asserts New falls back to $NODE_NAME when
// Config.NodeID is empty (the DaemonSet downward-API convention).
func TestNewResolvesNodeFromEnv(t *testing.T) {
	t.Setenv("NODE_NAME", "env-node")
	src := New(Config{Mode: "exec"})
	if src.nodeID != "env-node" {
		t.Fatalf("nodeID = %q, want env-node (from NODE_NAME)", src.nodeID)
	}
	if src.pollInterval != defaultWatchPollInterval {
		t.Fatalf("pollInterval = %v, want default %v", src.pollInterval, defaultWatchPollInterval)
	}
}

// --- test client fakes -----------------------------------------------------

// healthErrClient wraps FakeClient but fails QueryHealth, exercising the
// Watch degrade-to-closed-channel path.
type healthErrClient struct {
	*npusmi.FakeClient
}

func (c *healthErrClient) QueryHealth(ctx context.Context, devID int) (npusmi.HealthState, error) {
	return npusmi.HealthUnknown, npusmi.ErrNoCommand
}

// topoErrClient fails QueryTopo, exercising List error propagation.
type topoErrClient struct{}

func (c *topoErrClient) QueryTopo(ctx context.Context) ([]npusmi.TopoEntry, error) {
	return nil, npusmi.ErrNoCommand
}
func (c *topoErrClient) QueryDeviceInfo(ctx context.Context, devID int) (*npusmi.DeviceInfo, error) {
	return nil, npusmi.ErrNoCommand
}
func (c *topoErrClient) QueryHealth(ctx context.Context, devID int) (npusmi.HealthState, error) {
	return npusmi.HealthUnknown, npusmi.ErrNoCommand
}
