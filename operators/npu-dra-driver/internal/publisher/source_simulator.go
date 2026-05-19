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
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/npu-dra-driver/api/v1alpha1"
)

// SimulatorSource implements Source by reading a static JSON file shaped
// like configs/mock-data/set-a-small/npus.json. It is the Phase 4 default
// data source; the real-Ascend driver lands Phase 5+.
//
// Threading: List is safe to call from multiple goroutines (each call
// re-reads the file via os.ReadFile under a read lock). Watch starts a
// single goroutine that polls the file mtime every WatchPollInterval and
// emits an Event when the mtime changes (handles atomic-rename file updates
// per P4-T-005 acceptance).
type SimulatorSource struct {
	// Path is the absolute or relative path to the JSON file shaped like
	// configs/mock-data/set-a-small/npus.json. Required.
	Path string

	// WatchPollInterval controls how often Watch checks the file mtime.
	// Defaults to 5s when zero; tests override to drive reconciles
	// promptly without flakiness.
	WatchPollInterval time.Duration

	// SliceAICoreCapacityFallback is the per-device slice-aicore capacity
	// used when the JSON entry lacks aiCoreTotal. Defaults to "32" (the
	// Ascend 910B chip total). Tests can override.
	SliceAICoreCapacityFallback string

	mu sync.RWMutex
}

// simulatorNPU is the subset of npus.json fields the simulator needs.
// We intentionally do NOT unmarshal the rest of the schema (status, usage,
// vramMiB, etc.) — those belong to other Ocloud subsystems (exporter-plus,
// frontend telemetry). Source.List only projects what npu-dra-driver
// publishes as ResourceSlice attributes / capacity.
type simulatorNPU struct {
	ID          string `json:"id"`
	NodeName    string `json:"nodeName"`
	Index       int64  `json:"index"`
	AICoreTotal int64  `json:"aiCoreTotal"`
	NUMANode    int64  `json:"numaNode"`
	HccsGroup   string `json:"hccsGroup"`
	HccsRing    int64  `json:"hccsRing"`
	Status      string `json:"status"`
	SliceMode   string `json:"sliceMode"`
}

type simulatorPayload struct {
	NPUs []simulatorNPU `json:"npus"`
}

// List reads the JSON file, projects each npu entry into an AscendDevice,
// and groups by NodeName. Result is sorted (NodeName ascending, Index
// ascending) so subsequent diffs against published ResourceSlices are
// stable.
func (s *SimulatorSource) List(ctx context.Context) ([]NodeDevices, error) {
	if s.Path == "" {
		return nil, fmt.Errorf("simulator source: Path is empty")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	raw, err := os.ReadFile(s.Path)
	if err != nil {
		return nil, fmt.Errorf("simulator source: read %q: %w", s.Path, err)
	}

	var payload simulatorPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("simulator source: parse %q: %w", s.Path, err)
	}

	byNode := make(map[string][]v1alpha1.AscendDevice)
	for _, npu := range payload.NPUs {
		dev := v1alpha1.AscendDevice{
			Name:                deviceName(npu.NodeName, npu.Index),
			Index:               npu.Index,
			Health:              healthFromStatus(npu.Status),
			SliceStrategy:       strategyFromSliceMode(npu.SliceMode),
			AICores:             aiCoresForStrategy(npu.SliceMode, npu.AICoreTotal),
			NUMANode:            npu.NUMANode,
			HCCSRing:            npu.HccsRing,
			SliceAICoreCapacity: s.capacityOr(npu.AICoreTotal),
		}
		byNode[npu.NodeName] = append(byNode[npu.NodeName], dev)
	}

	out := make([]NodeDevices, 0, len(byNode))
	for node, devs := range byNode {
		sort.Slice(devs, func(i, j int) bool { return devs[i].Index < devs[j].Index })
		out = append(out, NodeDevices{NodeName: node, Devices: devs})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NodeName < out[j].NodeName })
	return out, nil
}

// Watch polls Path's mtime at WatchPollInterval and emits an Event each time
// the mtime moves forward. Phase 4 ships only mtime polling; Phase 5+ may
// add fsnotify if the real-Ascend source uses an event-driven discovery
// path. Channel is closed when ctx is cancelled.
func (s *SimulatorSource) Watch(ctx context.Context) <-chan Event {
	interval := s.WatchPollInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ch := make(chan Event, 1)
	go func() {
		defer close(ch)
		var lastMod time.Time
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				info, err := os.Stat(s.Path)
				if err != nil {
					// Soft-fail Watch: the next List call will surface
					// the real error to the publisher.
					continue
				}
				if mod := info.ModTime(); mod.After(lastMod) {
					if !lastMod.IsZero() {
						select {
						case ch <- Event{Source: "simulator", Reason: "file-modified"}:
						default:
						}
					}
					lastMod = mod
				}
			}
		}
	}()
	return ch
}

func (s *SimulatorSource) capacityOr(total int64) resource.Quantity {
	if total > 0 {
		return *resource.NewQuantity(total, resource.DecimalSI)
	}
	fallback := s.SliceAICoreCapacityFallback
	if fallback == "" {
		fallback = "32"
	}
	return resource.MustParse(fallback)
}

func deviceName(node string, index int64) string {
	return node + "-npu-" + strconv.FormatInt(index, 10)
}

// healthFromStatus maps mock-data status strings to Ocloud Health enum.
// Mock data uses "healthy" / "degraded" / "unhealthy" / "unknown";
// AscendDevice schema accepts Healthy / Unhealthy / Unknown. "degraded" is
// projected as Unhealthy (treat-as-degraded-equals-unhealthy is the
// simplest Phase 4 choice; Phase 5+ may add a third condition).
func healthFromStatus(s string) string {
	switch s {
	case "healthy":
		return v1alpha1.HealthHealthy
	case "degraded", "unhealthy":
		return v1alpha1.HealthUnhealthy
	default:
		return v1alpha1.HealthUnknown
	}
}

// strategyFromSliceMode maps mock-data sliceMode strings to Ocloud
// SliceStrategy enum. Mock uses "whole" / "fixed-template" / "dynamic" /
// (and any future). AscendDevice schema accepts FixedTemplate / Dynamic.
// "whole" is projected as FixedTemplate (the chip is undivided — a
// fixed-template of 1 vir32 slice covering the whole NPU).
func strategyFromSliceMode(m string) string {
	switch m {
	case "dynamic":
		return v1alpha1.SliceStrategyDynamic
	default:
		return v1alpha1.SliceStrategyFixedTemplate
	}
}

// aiCoresForStrategy returns the AICores attribute for a device. Per the
// AscendDevice schema, AICores > 0 is required when SliceStrategy=Dynamic,
// and unset (0) when SliceStrategy=FixedTemplate.
func aiCoresForStrategy(sliceMode string, aiCoreTotal int64) int64 {
	if strategyFromSliceMode(sliceMode) == v1alpha1.SliceStrategyDynamic {
		// Dynamic slice publishes the device's full AI-core total as the
		// allocatable slice budget; the Phase 5 allocator chops this into
		// per-claim sub-allocations.
		if aiCoreTotal > 0 {
			return aiCoreTotal
		}
		return 32
	}
	return 0
}
