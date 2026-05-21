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

// Package numa is the NumaAffinity plugin for kube-scheduler — Phase 10
// T005 三件套 part 3 · upstream wrap body landed.
//
// **Status: wrap delegation to upstream sigs.k8s.io/scheduler-plugins/
// pkg/noderesourcetopology v0.34.7** (K8s 1.34 lockstep per ADR-0010 §1
// P10-T-003 + P10-T-004 update segments + ADR-0001 v3 §5 per-module skew
// policy).
//
// Per ADR-0010 §3 this plugin wraps upstream `noderesourcetopology.New`
// without modification ("wrap, don't fork"). The placeholder body
// (Phase 6 T006 / Phase 7 P7-T-002 / Phase 8 P8-T-003 / Phase 9 P9-T-102
// 4× carry) finally lands at Phase 10 T005 because:
//
//   - K8s baseline bumped 1.32 → 1.34 at T003 (kindest/node v1.34.3,
//     scheduler-plugin go.mod v0.34.7 cohort).
//   - Framework migration at T004 (NodeInfo / CycleState struct→interface
//     via `k8s.io/kube-scheduler/framework` plan contract).
//   - sched-plugins v0.34.7 GA in 2024-04 (K8s 1.34 lockstep) — first
//     release where `noderesourcetopology` plugin compiles cleanly against
//     K8s 1.34 framework. Earlier v0.31.8 referenced `framework.GVK`
//     (removed K8s 1.32) blocking 4 prior Phase carry windows.
//
// Why local plugin name "NumaAffinity":
//   - chart ConfigMap profile readability (vs upstream's "NodeResourceTopologyMatch")
//   - stable chart-facing identity across upstream upgrades
//
// The wrapped plugin's `Name()` returns upstream's "NodeResourceTopologyMatch"
// — only surfaces in scheduler log lines, NOT in KubeSchedulerConfiguration
// profile lookup (which uses registration key from cmd/main.go
// `app.WithPlugin(numa.Name, numa.New)`). Documented via `UpstreamName`
// constant for log correlation.
//
// Args defaulting:
//   - When chart KubeSchedulerConfiguration profile omits pluginConfig
//     block for NumaAffinity (args=nil), wrap injects sensible defaults
//     (ScoringStrategy=LeastAllocated, Resources=[cpu, memory]).
//   - Defaulting avoids requiring upstream apis/config/v1 scheme registration
//     in cmd/main.go (Forbidden Path per T005 Allowed Paths).
//   - Chart-provided pluginConfig still flows through unchanged.
//
// Forward path:
//   - sched-plugins v0.35+/v0.36+ release → re-evaluate K8s 1.35/1.36
//     baseline bump (per ADR-0001 v3 §5 Phase 11+ re-eval triggers).
//   - Pod-spec override `numa.affinity/disable=true` annotation
//     (4th plan acceptance sanity test) deferred to Phase 11+ if production
//     demand emerges; current wrap is pure passthrough.
package numa

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	schedconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	apiconfig "sigs.k8s.io/scheduler-plugins/apis/config"
	nrt "sigs.k8s.io/scheduler-plugins/pkg/noderesourcetopology"
)

// Name is the plugin name registered with kube-scheduler. T101 helm chart
// KubeSchedulerConfiguration profile filter/score `enabled[]` lists reference
// this name (lockstep with cmd/main.go `app.WithPlugin(numa.Name, numa.New)`).
const Name = "NumaAffinity"

// UpstreamName documents the upstream plugin's internal Name() for
// log-correlation. The wrap delegates to upstream nrt.New which constructs
// a NodeResourceTopologyMatch plugin instance — that instance's Name()
// method returns this string, not our "NumaAffinity". Surfaces in
// kube-scheduler log lines (e.g. "plugin NodeResourceTopologyMatch Filter
// returned ...") — registration / profile lookup uses Name (above).
const UpstreamName = "NodeResourceTopologyMatch"

// defaultArgs returns sensible NodeResourceTopologyMatchArgs when chart
// KubeSchedulerConfiguration omits a pluginConfig block for NumaAffinity.
// ScoringStrategy=LeastAllocated favors nodes with the most allocatable
// remaining (lower binpacking pressure) — aligns with Phase 6 P6-T-007
// Binpack plugin opt-in posture (Binpack handles fill-up; NumaAffinity
// handles topology preference orthogonally). Resource weights at 1:1 for
// cpu+memory keep score contribution balanced; NPU resources are deliberately
// absent here — HCCSTopology plugin (filter+score) handles NPU-specific
// placement, NumaAffinity is host-level NUMA awareness only.
func defaultArgs() *apiconfig.NodeResourceTopologyMatchArgs {
	return &apiconfig.NodeResourceTopologyMatchArgs{
		ScoringStrategy: apiconfig.ScoringStrategy{
			Type: apiconfig.LeastAllocated,
			Resources: []schedconfig.ResourceSpec{
				{Name: "cpu", Weight: 1},
				{Name: "memory", Weight: 1},
			},
		},
	}
}

// New constructs the NumaAffinity plugin as a thin delegate of upstream
// noderesourcetopology.New. Args=nil triggers defaultArgs() fallback so
// chart KubeSchedulerConfiguration profile pluginConfig block is optional.
//
// Return type framework.Plugin per upstream contract; kube-scheduler
// detects FilterPlugin / ScorePlugin / PreFilterPlugin via interface
// assertion at profile load time (the upstream NodeResourceTopologyMatch
// struct implements all three).
func New(ctx context.Context, args runtime.Object, h framework.Handle) (framework.Plugin, error) {
	if args == nil {
		args = defaultArgs()
	}
	return nrt.New(ctx, args, h)
}
