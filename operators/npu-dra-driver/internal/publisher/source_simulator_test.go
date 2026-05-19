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

package publisher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/npu-dra-driver/api/v1alpha1"
)

// minimal2NodeFixture is the smallest payload exercised by the unit tests.
// Two nodes, four NPUs each (8 total) — mirrors the plan happy path
// description but keeps the test fixture independent of the real
// configs/mock-data/set-a-small/npus.json so set-a-small can evolve
// without churning these tests.
const minimal2NodeFixture = `{
  "npus": [
    {"id":"node-a-npu-0","nodeName":"node-a","index":0,"aiCoreTotal":32,"numaNode":0,"hccsGroup":"node-a-hccs-0","hccsRing":0,"status":"healthy","sliceMode":"whole"},
    {"id":"node-a-npu-1","nodeName":"node-a","index":1,"aiCoreTotal":32,"numaNode":0,"hccsGroup":"node-a-hccs-0","hccsRing":0,"status":"healthy","sliceMode":"whole"},
    {"id":"node-a-npu-2","nodeName":"node-a","index":2,"aiCoreTotal":32,"numaNode":1,"hccsGroup":"node-a-hccs-1","hccsRing":1,"status":"degraded","sliceMode":"fixed-template"},
    {"id":"node-a-npu-3","nodeName":"node-a","index":3,"aiCoreTotal":32,"numaNode":1,"hccsGroup":"node-a-hccs-1","hccsRing":1,"status":"healthy","sliceMode":"dynamic"},
    {"id":"node-b-npu-0","nodeName":"node-b","index":0,"aiCoreTotal":32,"numaNode":0,"hccsGroup":"node-b-hccs-0","hccsRing":0,"status":"healthy","sliceMode":"whole"},
    {"id":"node-b-npu-1","nodeName":"node-b","index":1,"aiCoreTotal":32,"numaNode":0,"hccsGroup":"node-b-hccs-0","hccsRing":0,"status":"healthy","sliceMode":"whole"},
    {"id":"node-b-npu-2","nodeName":"node-b","index":2,"aiCoreTotal":32,"numaNode":1,"hccsGroup":"node-b-hccs-1","hccsRing":1,"status":"unknown","sliceMode":"whole"},
    {"id":"node-b-npu-3","nodeName":"node-b","index":3,"aiCoreTotal":32,"numaNode":1,"hccsGroup":"node-b-hccs-1","hccsRing":1,"status":"healthy","sliceMode":"whole"}
  ]
}`

const emptyNPUsFixture = `{"npus":[]}`

func writeTempFixture(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "npus.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp fixture: %v", err)
	}
	return path
}

func TestSimulatorSource_Happy(t *testing.T) {
	path := writeTempFixture(t, minimal2NodeFixture)
	src := &SimulatorSource{Path: path}

	out, err := src.List(context.Background())
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(out))
	}
	if out[0].NodeName != "node-a" || out[1].NodeName != "node-b" {
		t.Errorf("node order unstable: %+v", out)
	}
	if len(out[0].Devices) != 4 || len(out[1].Devices) != 4 {
		t.Errorf("expected 4 devices per node, got node-a=%d node-b=%d",
			len(out[0].Devices), len(out[1].Devices))
	}

	// Spot-check first device on node-a: index 0, healthy, whole slice mode.
	d0 := out[0].Devices[0]
	if d0.Name != "node-a-npu-0" {
		t.Errorf("d0.Name: want node-a-npu-0, got %s", d0.Name)
	}
	if d0.Index != 0 {
		t.Errorf("d0.Index: want 0, got %d", d0.Index)
	}
	if d0.Health != v1alpha1.HealthHealthy {
		t.Errorf("d0.Health: want %s, got %s", v1alpha1.HealthHealthy, d0.Health)
	}
	if d0.SliceStrategy != v1alpha1.SliceStrategyFixedTemplate {
		t.Errorf("d0.SliceStrategy from 'whole': want %s, got %s",
			v1alpha1.SliceStrategyFixedTemplate, d0.SliceStrategy)
	}
	if d0.AICores != 0 {
		t.Errorf("d0.AICores for FixedTemplate: want 0, got %d", d0.AICores)
	}

	// Spot-check Dynamic-mode device: index 3 on node-a.
	d3 := out[0].Devices[3]
	if d3.SliceStrategy != v1alpha1.SliceStrategyDynamic {
		t.Errorf("d3 from 'dynamic': want %s, got %s",
			v1alpha1.SliceStrategyDynamic, d3.SliceStrategy)
	}
	if d3.AICores <= 0 {
		t.Errorf("d3.AICores must be > 0 for Dynamic strategy, got %d", d3.AICores)
	}

	// Spot-check degraded → Unhealthy mapping (node-a index 2).
	d2 := out[0].Devices[2]
	if d2.Health != v1alpha1.HealthUnhealthy {
		t.Errorf("d2 from 'degraded': want %s, got %s", v1alpha1.HealthUnhealthy, d2.Health)
	}

	// Spot-check unknown status → Unknown mapping (node-b index 2).
	d2b := out[1].Devices[2]
	if d2b.Health != v1alpha1.HealthUnknown {
		t.Errorf("node-b d2 from 'unknown': want %s, got %s", v1alpha1.HealthUnknown, d2b.Health)
	}

	// Validate each emitted Device round-trips through ValidateAttributes
	// (catches regressions where the mapping forgets a required attribute).
	for _, nd := range out {
		for _, dev := range nd.Devices {
			up := dev.ToUpstream()
			if err := v1alpha1.ValidateAttributes(up); err != nil {
				t.Errorf("emitted device %q fails ValidateAttributes: %v", dev.Name, err)
			}
		}
	}
}

func TestSimulatorSource_Empty(t *testing.T) {
	path := writeTempFixture(t, emptyNPUsFixture)
	src := &SimulatorSource{Path: path}

	out, err := src.List(context.Background())
	if err != nil {
		t.Fatalf("empty fixture List error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected 0 nodes from empty fixture, got %d", len(out))
	}
}

func TestSimulatorSource_Unreadable(t *testing.T) {
	src := &SimulatorSource{Path: filepath.Join(t.TempDir(), "does-not-exist.json")}

	_, err := src.List(context.Background())
	if err == nil {
		t.Fatal("expected error on missing file, got nil")
	}
}

func TestSimulatorSource_EmptyPath(t *testing.T) {
	src := &SimulatorSource{Path: ""}
	if _, err := src.List(context.Background()); err == nil {
		t.Fatal("expected error on empty Path, got nil")
	}
}

func TestSimulatorSource_WatchPicksUpFileUpdate(t *testing.T) {
	path := writeTempFixture(t, emptyNPUsFixture)
	src := &SimulatorSource{Path: path, WatchPollInterval: 50 * time.Millisecond}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ch := src.Watch(ctx)

	// Atomically replace the file (rename pattern mirrors how ConfigMap
	// remount + atomic-rename file updates behave inside a pod).
	go func() {
		time.Sleep(150 * time.Millisecond)
		tmp := path + ".new"
		_ = os.WriteFile(tmp, []byte(minimal2NodeFixture), 0o644)
		_ = os.Rename(tmp, path)
	}()

	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("Watch channel closed before file-modified event")
		}
		if ev.Reason != "file-modified" {
			t.Errorf("expected reason file-modified, got %q", ev.Reason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not surface a file-modified event within 2s")
	}

	// After the event fires, a subsequent List must reflect the new content.
	out, err := src.List(ctx)
	if err != nil {
		t.Fatalf("List after update: %v", err)
	}
	if len(out) != 2 {
		t.Errorf("List after file update: expected 2 nodes, got %d", len(out))
	}
}

func TestSimulatorSource_UsesRealSetASmall(t *testing.T) {
	// Defensive regression: the real configs/mock-data/set-a-small/npus.json
	// must parse cleanly. This guards against future Phase 4+ schema drift
	// breaking the publisher before kind smoke surfaces it (P4-T-104).
	candidate := filepath.Join("..", "..", "..", "..", "configs", "mock-data", "set-a-small", "npus.json")
	if _, err := os.Stat(candidate); err != nil {
		t.Skipf("skipping set-a-small parse: %v", err)
	}
	src := &SimulatorSource{Path: candidate}
	out, err := src.List(context.Background())
	if err != nil {
		t.Fatalf("real set-a-small List error: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("real set-a-small must yield > 0 nodes")
	}
	total := 0
	for _, nd := range out {
		total += len(nd.Devices)
		for _, dev := range nd.Devices {
			if err := v1alpha1.ValidateAttributes(dev.ToUpstream()); err != nil {
				t.Errorf("set-a-small device %q fails ValidateAttributes: %v",
					dev.Name, err)
			}
		}
	}
	t.Logf("set-a-small produced %d nodes / %d devices", len(out), total)
}
