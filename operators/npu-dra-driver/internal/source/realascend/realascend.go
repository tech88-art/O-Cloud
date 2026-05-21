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

// Package realascend is the Phase 7 W1 stub Source impl that — once Phase
// 7 P7-T-101 lab-conditional lights it up — talks to real Ascend silicon
// via npu-smi + DCMI to discover device inventory + HCCS topology.
//
// Phase 7 W1 status (this commit · ADR-0011 §2 §3 + phase7-plan §3 T004):
//
//   - All three Source methods (List / Watch / QueryTopology) return
//     ErrNotImplemented · Construction via New() succeeds + registers via
//     factory but operating the source returns the sentinel error so the
//     publisher reconcile loop can soft-fail loudly when this source is
//     accidentally selected without lab access
//   - Phase 7 T101 lab-conditional: replaces the stub body with the real
//     impl (npu-smi info -t topo parsing via internal/source/realascend/npusmi
//     package shipped by P7-T-005)
//   - Phase 7 helm chart values default sourceType="mock-json"; selecting
//     "real-ascend" without lab access surfaces ErrNotImplemented in
//     reconcile-loop logs (intentional · operators must explicitly opt in)
//
// **References**:
//   - ADR-0011 §2 Source interface contract + §3 Lab gating policy
//   - phase7-plan.md §3 P7-T-004 + §4 P7-T-101 (lab-gated body)
//   - operators/npu-dra-driver/DESIGN.md §"Source interface architecture"
//   - operators/npu-dra-driver/internal/source/realascend/npusmi/ (P7-T-005 ·
//     parser scaffold for the eventual lab body)
package realascend

import (
	"context"

	"github.com/tech88-art/O-Cloud/operators/npu-dra-driver/internal/source"
)

// Config captures RealAscendSource construction parameters. Phase 7 W1
// stub uses only the Mode field; Phase 7 T101 lab body will add hostPath
// mount + npu-smi binary path + DCMI socket path fields.
type Config struct {
	// Mode selects how the source backs its calls. Phase 7 T101 introduces
	// "exec" (shell out to npu-smi binary). W1 stub accepts any value and
	// returns ErrNotImplemented uniformly.
	Mode string
}

// RealAscendSource is the Phase 7 W1 stub. Phase 7 T101 lab-conditional
// replaces the method bodies with the real impl.
type RealAscendSource struct {
	cfg Config
}

// New constructs a RealAscendSource. Phase 7 W1 has no construction-time
// validation; the stub returns ErrNotImplemented uniformly across all
// three methods. Phase 7 T101 lab body will add config validation
// (npu-smi binary exists, hostPath mount is correct, etc.).
func New(cfg Config) *RealAscendSource {
	return &RealAscendSource{cfg: cfg}
}

// List is a Phase 7 W1 stub returning ErrNotImplemented per ADR-0011 §2.
// Phase 7 T101 lab body queries device inventory via npu-smi info.
func (s *RealAscendSource) List(ctx context.Context) ([]source.NodeDevices, error) {
	return nil, source.ErrNotImplemented
}

// Watch is a Phase 7 W1 stub. Phase 7 T101 lab body polls DCMI health
// changes. Returns a closed channel to signal "no events ever" without
// blocking callers that range over it.
func (s *RealAscendSource) Watch(ctx context.Context) <-chan source.Event {
	ch := make(chan source.Event)
	close(ch)
	return ch
}

// QueryTopology is a Phase 7 W1 stub returning ErrNotImplemented per
// ADR-0011 §2. Phase 7 T101 lab body shells out to
// `npu-smi info -t topo` and parses the HCCS ring + NUMA mapping.
func (s *RealAscendSource) QueryTopology(ctx context.Context, nodeName string) (*source.HCCSTopology, error) {
	return nil, source.ErrNotImplemented
}

// Compile-time assertion that RealAscendSource satisfies source.Source.
var _ source.Source = &RealAscendSource{}
