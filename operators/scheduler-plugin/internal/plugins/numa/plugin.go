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

// Package numa is the NumaAffinity plugin for kube-scheduler — Phase 6
// T006 placeholder body.
//
// **Status: placeholder · not a no-op wrap of upstream yet**
//
// Per ADR-0010 §3 this plugin SHOULD wrap upstream
// `sigs.k8s.io/scheduler-plugins/pkg/noderesourcetopology` without
// modification ("wrap, don't fork"). However, a direct `nrt.New(...)`
// delegation pattern at T006 entry FAILED to build because the upstream
// v0.31.8 release references `framework.GVK` — a symbol that exists in
// K8s 1.31's `pkg/scheduler/framework` but was removed in K8s 1.32
// (our pinned baseline per `go.mod` replace block / ADR-0010 §1).
//
// API drift table (observed 2026-05-20):
//
//   sched-plugins | targets K8s | framework.GVK present | our K8s 1.32 baseline
//   v0.30.x       | 1.30        | yes                    | INCOMPATIBLE (replace block K8s = v0.32.0)
//   v0.31.x       | 1.31        | yes                    | INCOMPATIBLE  ("framework.GVK undefined" at build)
//   v0.32.x       | 1.32        | (removed)              | COMPATIBLE — not yet released as of 2026-05-20
//
// **Operative**: T006 ships a Name()-only placeholder (mirrors T002
// scaffold for this package); kube-scheduler registers the plugin under
// "NumaAffinity" but invokes no Filter/Score because we don't implement
// the extension-point interfaces. T101 chart will NOT enable NumaAffinity
// in its KubeSchedulerConfiguration default until the wrap lands.
//
// **Forward path** (T006 follow-up): once sched-plugins releases a
// v0.32.x tag (or downstream releases a v0.31.y with K8s 1.32
// compatibility patch), revisit with:
//   - update `go.mod` replace block + require sched-plugins direct
//   - replace this file's placeholder body with `return nrt.New(ctx, args, h)`
//     plus the UpstreamName log-correlation hint constant
//   - chart KubeSchedulerConfiguration enables NumaAffinity in profile
//
// Until then, operators wanting NUMA-aware scheduling can:
//   - run upstream sched-plugins binary as a second scheduler alongside
//     our HCCSTopology+Binpack binary (more operational overhead but
//     functional today)
//   - OR wait for the v0.32.x wrap landing
package numa

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

// Name is the plugin name registered with kube-scheduler. T101 helm chart
// MUST omit this plugin from the KubeSchedulerConfiguration filter / score
// enabled lists until the upstream wrap lands per the doc above.
const Name = "NumaAffinity"

// UpstreamName documents the upstream plugin's internal Name() for
// log-correlation once the wrap lands. Placeholder retains the name so
// downstream chart updates can flip the wrap on by editing one import
// + one body line.
const UpstreamName = "NodeResourceTopologyMatch"

// NumaAffinity is the placeholder plugin struct. T006 ships only the Name()
// method (framework.Plugin satisfied). When the upstream wrap lands, this
// type either gets replaced by `return nrt.New(...)` directly OR becomes
// an embedded wrap struct.
type NumaAffinity struct{}

// Compile-time interface assertion (framework.Plugin only — Filter / Score
// land alongside the upstream wrap).
var _ framework.Plugin = &NumaAffinity{}

// Name returns the plugin name. Required by framework.Plugin.
func (p *NumaAffinity) Name() string {
	return Name
}

// New constructs a NumaAffinity placeholder. T006 placeholder body — see
// package doc for the upstream-wrap deferral rationale.
func New(_ context.Context, _ runtime.Object, _ framework.Handle) (framework.Plugin, error) {
	return &NumaAffinity{}, nil
}
