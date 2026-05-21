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

package npusmi

import (
	"context"
	"embed"
	"fmt"
)

//go:embed testdata/npu-smi-topo-fixture-8card.txt testdata/npu-smi-topo-fixture-16card.txt
var fakeFixtures embed.FS

// FakeClient implements Client by parsing a baked-in testdata fixture
// file. Phase 7 W1 + T101 dev mode use FakeClient when there's no real
// npu-smi binary available (and CI never has one).
//
// Caller selects which fixture by setting Fixture to one of the known
// names ("8card" · "16card") or to "" for the default ("8card").
type FakeClient struct {
	// Fixture is the testdata fixture name. Defaults to "8card" when
	// empty.
	Fixture string

	// NodeID is stamped on every returned TopoEntry's NodeID field.
	// Defaults to "fake-node" when empty.
	NodeID string
}

// Compile-time assertion: FakeClient satisfies Client.
var _ Client = &FakeClient{}

// QueryTopo loads the embedded fixture text, runs the parser, and
// stamps NodeID on each entry. Returns a fresh slice on every call so
// callers can mutate without polluting subsequent calls.
func (f *FakeClient) QueryTopo(ctx context.Context) ([]TopoEntry, error) {
	name := f.Fixture
	if name == "" {
		name = "8card"
	}
	fname := "testdata/npu-smi-topo-fixture-" + name + ".txt"
	raw, err := fakeFixtures.ReadFile(fname)
	if err != nil {
		return nil, fmt.Errorf("npusmi FakeClient: load fixture %q: %w", fname, err)
	}
	entries, err := ParseTopoMatrix(string(raw))
	if err != nil {
		return nil, err
	}
	nodeID := f.NodeID
	if nodeID == "" {
		nodeID = "fake-node"
	}
	for i := range entries {
		entries[i].NodeID = nodeID
	}
	return entries, nil
}

// QueryDeviceInfo returns a canned DeviceInfo for the requested devID.
// Phase 7 W1: returns chip name + AI cores defaults; T101 lab body's
// ExecClient queries real DCMI. FakeClient surfaces "device not found"
// when devID is outside the fixture's bounds.
func (f *FakeClient) QueryDeviceInfo(ctx context.Context, devID int) (*DeviceInfo, error) {
	entries, err := f.QueryTopo(ctx)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.DeviceID == devID {
			return &DeviceInfo{
				DeviceID:        devID,
				ChipName:        "Ascend 910B",
				AICores:         32,
				MemorySizeMiB:   65536,
				NumaNode:        e.NumaNode,
				Health:          e.Health,
				DriverVersion:   "fake-24.x",
				FirmwareVersion: "fake-7.x",
			}, nil
		}
	}
	return nil, fmt.Errorf("npusmi FakeClient: device %d not in fixture %q", devID, f.Fixture)
}

// QueryHealth returns the per-device health from the fixture's
// TopoEntry. Cheaper than QueryDeviceInfo (no fixture re-parse — but
// Phase 7 W1 scaffold accepts the parse cost; T101 lab body caches).
func (f *FakeClient) QueryHealth(ctx context.Context, devID int) (HealthState, error) {
	entries, err := f.QueryTopo(ctx)
	if err != nil {
		return HealthUnknown, err
	}
	for _, e := range entries {
		if e.DeviceID == devID {
			return e.Health, nil
		}
	}
	return HealthUnknown, fmt.Errorf("npusmi FakeClient: device %d not in fixture %q", devID, f.Fixture)
}
