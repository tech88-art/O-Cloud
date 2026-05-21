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

package mockjson

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const fixtureJSON = `{
	"npus": [
		{"id":"a-0","nodeName":"node-a","index":0,"aiCoreTotal":32,"numaNode":0,"hccsGroup":"a","hccsRing":0,"status":"healthy","sliceMode":"whole"},
		{"id":"a-1","nodeName":"node-a","index":1,"aiCoreTotal":32,"numaNode":0,"hccsGroup":"a","hccsRing":1,"status":"healthy","sliceMode":"dynamic"},
		{"id":"b-0","nodeName":"node-b","index":0,"aiCoreTotal":32,"numaNode":1,"hccsGroup":"b","hccsRing":0,"status":"degraded","sliceMode":"fixed-template"}
	]
}`

func writeFixture(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "npus.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

// TestListFromSetASmallShape covers ADR-0011 §2 List contract: the source
// projects npus.json into per-node device lists in stable order
// (NodeName asc · Index asc within node) for ResourceSlice diff
// idempotence.
//
// Phase 7 P7-T-004 acceptance gate 1/4.
func TestListFromSetASmallShape(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, fixtureJSON)

	src := New(Config{Path: path})
	got, err := src.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(NodeDevices) = %d, want 2 (node-a + node-b)", len(got))
	}
	if got[0].NodeName != "node-a" {
		t.Fatalf("got[0].NodeName = %q, want \"node-a\" (sort by NodeName asc)", got[0].NodeName)
	}
	if len(got[0].Devices) != 2 {
		t.Fatalf("got[0].Devices len = %d, want 2", len(got[0].Devices))
	}
	if got[0].Devices[0].Index != 0 || got[0].Devices[1].Index != 1 {
		t.Fatalf("got[0].Devices Index order = [%d, %d], want [0, 1] (sort by Index asc)",
			got[0].Devices[0].Index, got[0].Devices[1].Index)
	}
	if got[1].NodeName != "node-b" {
		t.Fatalf("got[1].NodeName = %q, want \"node-b\"", got[1].NodeName)
	}
}

// TestWatchEmitsOnFileModified covers ADR-0011 §2 Watch contract: when
// the JSON file mtime moves forward (atomic-rename file update per the
// P4-T-005 acceptance gate), Watch emits an Event tagged "mock-json".
//
// Phase 7 P7-T-004 acceptance gate 2/4.
func TestWatchEmitsOnFileModified(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, fixtureJSON)

	src := New(Config{
		Path:              path,
		WatchPollInterval: 50 * time.Millisecond,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch := src.Watch(ctx)

	// First tick establishes lastMod baseline (no event); wait a bit
	// past the poll interval to ensure that initial scan landed.
	time.Sleep(100 * time.Millisecond)

	// Touch the file (atomic rename via Write to bump mtime).
	if err := os.WriteFile(path, []byte(fixtureJSON), 0o644); err != nil {
		t.Fatalf("touch fixture: %v", err)
	}
	// Ensure mtime resolution advances even on systems with coarse
	// granularity (some Windows FS report 1s precision).
	now := time.Now().Add(1 * time.Second)
	_ = os.Chtimes(path, now, now)

	select {
	case ev := <-ch:
		if ev.Source != "mock-json" {
			t.Fatalf("Event.Source = %q, want \"mock-json\"", ev.Source)
		}
		if ev.Reason != "file-modified" {
			t.Fatalf("Event.Reason = %q, want \"file-modified\"", ev.Reason)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for Watch event after file modify")
	}
}

// TestQueryTopologyReturnsRings covers ADR-0011 §2 QueryTopology
// contract: returns the HCCS ring + NUMA view built from the same
// parsed JSON, with stable device-name ordering within rings/numa nodes.
//
// Phase 7 P7-T-004 acceptance gate 3/4.
func TestQueryTopologyReturnsRings(t *testing.T) {
	dir := t.TempDir()
	path := writeFixture(t, dir, fixtureJSON)

	src := New(Config{Path: path})

	topo, err := src.QueryTopology(context.Background(), "node-a")
	if err != nil {
		t.Fatalf("QueryTopology: %v", err)
	}
	if topo == nil {
		t.Fatal("topo is nil; want populated HCCSTopology")
	}
	if topo.NodeName != "node-a" {
		t.Fatalf("topo.NodeName = %q, want \"node-a\"", topo.NodeName)
	}
	// node-a has 2 NPUs: index 0 on ring 0 + index 1 on ring 1. Both on
	// numa 0.
	if len(topo.Rings) != 2 {
		t.Fatalf("len(topo.Rings) = %d, want 2 (rings 0 + 1)", len(topo.Rings))
	}
	if len(topo.Rings[0]) != 1 || topo.Rings[0][0] != "node-a-npu-0" {
		t.Fatalf("Rings[0] = %v, want [node-a-npu-0]", topo.Rings[0])
	}
	if len(topo.Rings[1]) != 1 || topo.Rings[1][0] != "node-a-npu-1" {
		t.Fatalf("Rings[1] = %v, want [node-a-npu-1]", topo.Rings[1])
	}
	if len(topo.NUMA) != 1 || len(topo.NUMA[0]) != 2 {
		t.Fatalf("NUMA[0] len = %d, want 2 (both devices on numa node 0)", len(topo.NUMA[0]))
	}

	// Unknown node → (nil, nil)
	topo2, err := src.QueryTopology(context.Background(), "node-z")
	if err != nil {
		t.Fatalf("QueryTopology(unknown): %v", err)
	}
	if topo2 != nil {
		t.Fatalf("QueryTopology(unknown) = %+v, want nil for unknown node", topo2)
	}
}

// TestEmptyConfigReturnsEmptyList covers the corner case the Phase 4
// prototype documented: a Config with Path="" surfaces an error from
// List instead of crashing; an empty `npus` array returns an empty
// inventory.
//
// Phase 7 P7-T-004 acceptance gate 4/4.
func TestEmptyConfigAndEmptyPayload(t *testing.T) {
	t.Run("empty Path errors at List", func(t *testing.T) {
		src := New(Config{Path: ""})
		_, err := src.List(context.Background())
		if err == nil {
			t.Fatal("expected error from List with empty Path, got nil")
		}
	})

	t.Run("empty npus array returns empty inventory", func(t *testing.T) {
		dir := t.TempDir()
		path := writeFixture(t, dir, `{"npus":[]}`)
		src := New(Config{Path: path})
		got, err := src.List(context.Background())
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("len(NodeDevices) = %d, want 0 for empty npus array", len(got))
		}
	})
}
