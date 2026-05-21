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

// Package mockjson provides MockJSONSource — a Source impl reading a
// static JSON file shaped like configs/mock-data/set-a-small/npus.json.
//
// Phase 4-6 prototype lived in operators/npu-dra-driver/internal/publisher/
// as SimulatorSource. Phase 7 P7-T-004 (ADR-0011 §2) lifted it here so
// multiple source impls (mockjson + realascend) coexist behind the
// Source interface in internal/source/. **Behavior is bit-for-bit
// preserved from the Phase 4 prototype** — existing publisher tests pass
// unchanged when wired through the new mockjson path.
//
// The Phase 7 W1 addition is the QueryTopology method (ADR-0011 §2 third
// contract method): builds an HCCSTopology view from the same parsed
// data list returns.
package mockjson

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
	"github.com/tech88-art/O-Cloud/operators/npu-dra-driver/internal/source"
)

// Config captures MockJSONSource construction parameters. Set by
// source.NewSource factory + cmd/main.go from chart values; tests
// construct directly.
type Config struct {
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
}

// MockJSONSource is the Phase 4-6 SimulatorSource rehoused under the
// Source interface (ADR-0011 §2). All behavior preserved bit-for-bit
// EXCEPT the new QueryTopology method (Phase 7 addition).
//
// Threading: List is safe to call from multiple goroutines (each call
// re-reads the file via os.ReadFile under a read lock). Watch starts a
// single goroutine that polls the file mtime every Config.WatchPollInterval
// and emits an Event when the mtime changes (handles atomic-rename file
// updates per P4-T-005 acceptance).
type MockJSONSource struct {
	cfg Config

	mu sync.RWMutex
}

// New constructs a MockJSONSource from a Config. The Config struct's
// validation is delegated to List/Watch/QueryTopology — calling New with
// an empty Config returns a usable struct that errors at first use
// (matches the Phase 4 lazy-validation pattern).
func New(cfg Config) *MockJSONSource {
	return &MockJSONSource{cfg: cfg}
}

// simulatorNPU is the subset of npus.json fields the simulator needs.
// We intentionally do NOT unmarshal the rest of the schema (status,
// usage, vramMiB, etc.) — those belong to other Ocloud subsystems
// (exporter-plus, frontend telemetry). Source.List only projects what
// npu-dra-driver publishes as ResourceSlice attributes / capacity.
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
func (s *MockJSONSource) List(ctx context.Context) ([]source.NodeDevices, error) {
	payload, err := s.readPayload()
	if err != nil {
		return nil, err
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

	out := make([]source.NodeDevices, 0, len(byNode))
	for node, devs := range byNode {
		sort.Slice(devs, func(i, j int) bool { return devs[i].Index < devs[j].Index })
		out = append(out, source.NodeDevices{NodeName: node, Devices: devs})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NodeName < out[j].NodeName })
	return out, nil
}

// Watch polls Path's mtime at WatchPollInterval and emits an Event each
// time the mtime moves forward. Phase 4 shipped only mtime polling;
// Phase 5+ may add fsnotify if the real-Ascend source uses an
// event-driven discovery path. Channel is closed when ctx is cancelled.
func (s *MockJSONSource) Watch(ctx context.Context) <-chan source.Event {
	interval := s.cfg.WatchPollInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ch := make(chan source.Event, 1)
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
				info, err := os.Stat(s.cfg.Path)
				if err != nil {
					// Soft-fail Watch: the next List call will surface
					// the real error to the publisher.
					continue
				}
				if mod := info.ModTime(); mod.After(lastMod) {
					if !lastMod.IsZero() {
						select {
						case ch <- source.Event{Source: "mock-json", Reason: "file-modified"}:
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

// QueryTopology returns the HCCS ring + NUMA view for one node, built
// from the same JSON the List path reads (ADR-0011 §2 QueryTopology
// contract). Returns (nil, nil) if the node is unknown — caller
// distinguishes via the error value.
//
// Phase 7 W1: pool-operator continues to aggregate via ResourceSlice
// attributes (Phase 6 P6-T-003); this method is the forward hook for
// Phase 10 demo dashboards / lab-conditional cross-validation against
// the realascend body.
func (s *MockJSONSource) QueryTopology(ctx context.Context, nodeName string) (*source.HCCSTopology, error) {
	payload, err := s.readPayload()
	if err != nil {
		return nil, err
	}

	rings := make(map[int32][]string)
	numa := make(map[int32][]string)
	found := false

	for _, npu := range payload.NPUs {
		if npu.NodeName != nodeName {
			continue
		}
		found = true
		devName := deviceName(npu.NodeName, npu.Index)
		ring := int32(npu.HccsRing)
		nn := int32(npu.NUMANode)
		rings[ring] = append(rings[ring], devName)
		numa[nn] = append(numa[nn], devName)
	}

	if !found {
		return nil, nil
	}

	// Stable order within each ring / numa node for diff idempotence.
	for k := range rings {
		sort.Strings(rings[k])
	}
	for k := range numa {
		sort.Strings(numa[k])
	}

	return &source.HCCSTopology{
		NodeName: nodeName,
		Rings:    rings,
		NUMA:     numa,
	}, nil
}

// readPayload centralises the file-read + unmarshal path so List and
// QueryTopology share the same code (and the same error handling /
// concurrency guarantees).
func (s *MockJSONSource) readPayload() (*simulatorPayload, error) {
	if s.cfg.Path == "" {
		return nil, fmt.Errorf("mockjson source: Path is empty")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	raw, err := os.ReadFile(s.cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("mockjson source: read %q: %w", s.cfg.Path, err)
	}

	var payload simulatorPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("mockjson source: parse %q: %w", s.cfg.Path, err)
	}
	return &payload, nil
}

func (s *MockJSONSource) capacityOr(total int64) resource.Quantity {
	if total > 0 {
		return *resource.NewQuantity(total, resource.DecimalSI)
	}
	fallback := s.cfg.SliceAICoreCapacityFallback
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
// AscendDevice schema accepts Healthy / Unhealthy / Unknown. "degraded"
// is projected as Unhealthy (treat-as-degraded-equals-unhealthy is the
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

// aiCoresForStrategy returns the AICores attribute for a device. Per
// the AscendDevice schema, AICores > 0 is required when
// SliceStrategy=Dynamic, and unset (0) when SliceStrategy=FixedTemplate.
func aiCoresForStrategy(sliceMode string, aiCoreTotal int64) int64 {
	if strategyFromSliceMode(sliceMode) == v1alpha1.SliceStrategyDynamic {
		// Dynamic slice publishes the device's full AI-core total as the
		// allocatable slice budget; the Phase 5 allocator chops this
		// into per-claim sub-allocations.
		if aiCoreTotal > 0 {
			return aiCoreTotal
		}
		return 32
	}
	return 0
}

// Compile-time assertion that MockJSONSource satisfies source.Source.
var _ source.Source = &MockJSONSource{}
