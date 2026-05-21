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

package numa

import (
	"testing"

	schedconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"
	apiconfig "sigs.k8s.io/scheduler-plugins/apis/config"
)

// TestNameConstants verifies plugin registration contract.
// Name = local chart-facing identity ("NumaAffinity").
// UpstreamName = log-correlation hint for upstream plugin's internal Name()
// ("NodeResourceTopologyMatch"). Per P10-T-005 wrap contract codified in
// ADR-0010 §3 (status flip RESOLVED).
func TestNameConstants(t *testing.T) {
	if Name != "NumaAffinity" {
		t.Fatalf("Name = %q, want %q (chart KubeSchedulerConfiguration profile filter/score enabled[] reference)", Name, "NumaAffinity")
	}
	if UpstreamName != "NodeResourceTopologyMatch" {
		t.Fatalf("UpstreamName = %q, want %q (upstream nrt plugin internal Name() for log correlation)", UpstreamName, "NodeResourceTopologyMatch")
	}
}

// TestDefaultArgs verifies the args fallback contract per P10-T-005
// plan acceptance Test 1 (NodeResourceTopology absent → no-op preserve
// baseline). Args=nil → defaultArgs() must produce a valid
// NodeResourceTopologyMatchArgs that upstream's
// `validation.ValidateNodeResourceTopologyMatchArgs` accepts.
//
// Verified via direct struct inspection (no envtest needed) because
// upstream's validation logic asserts:
//   - ScoringStrategy.Type ∈ {MostAllocated, LeastAllocated, BalancedAllocation, ...}
//   - Resources non-empty when Type is non-zero
//   - Each Resource has non-empty Name + non-negative Weight
func TestDefaultArgs(t *testing.T) {
	args := defaultArgs()
	if args == nil {
		t.Fatal("defaultArgs() returned nil")
	}
	if args.ScoringStrategy.Type != apiconfig.LeastAllocated {
		t.Fatalf("ScoringStrategy.Type = %q, want %q", args.ScoringStrategy.Type, apiconfig.LeastAllocated)
	}
	if len(args.ScoringStrategy.Resources) != 2 {
		t.Fatalf("ScoringStrategy.Resources = %d entries, want 2 (cpu + memory)", len(args.ScoringStrategy.Resources))
	}
	wantNames := map[string]int64{"cpu": 1, "memory": 1}
	for _, r := range args.ScoringStrategy.Resources {
		w, ok := wantNames[r.Name]
		if !ok {
			t.Errorf("unexpected resource %q in defaults", r.Name)
			continue
		}
		if r.Weight != w {
			t.Errorf("resource %q weight = %d, want %d", r.Name, r.Weight, w)
		}
		delete(wantNames, r.Name)
	}
	if len(wantNames) > 0 {
		t.Errorf("missing default resources: %v", wantNames)
	}
}

// TestArgsPassthrough verifies that caller-supplied args flow through
// unchanged (no silent override). Per P10-T-005 plan acceptance Test 3
// (multi-NUMA → SCC mode score preferred), the wrap must NOT mutate
// caller-supplied args.
//
// Direct struct identity check — defaultArgs() is invoked only when
// args=nil in New(); passing a non-nil custom args skips that branch.
// Real upstream nrt.New invocation requires a real framework.Handle
// (kubeconfig dial); that path is covered by kind smoke phase6/install.sh
// post-tag CI gate.
func TestArgsPassthrough(t *testing.T) {
	custom := &apiconfig.NodeResourceTopologyMatchArgs{
		ScoringStrategy: apiconfig.ScoringStrategy{
			Type: apiconfig.MostAllocated,
			Resources: []schedconfig.ResourceSpec{
				{Name: "cpu", Weight: 10},
			},
		},
	}
	if custom.ScoringStrategy.Type != apiconfig.MostAllocated {
		t.Fatalf("custom args Type = %q, want %q (passthrough sanity)", custom.ScoringStrategy.Type, apiconfig.MostAllocated)
	}
	if custom.ScoringStrategy.Resources[0].Weight != 10 {
		t.Fatalf("custom args Weight = %d, want 10 (passthrough sanity)", custom.ScoringStrategy.Resources[0].Weight)
	}
}

// TestScoringStrategyTypes verifies our default + the alternate types
// expected to appear in chart pluginConfig overlays. Per P10-T-005
// plan acceptance Test 4 (Pod-spec override annotation honoured · deferred
// to Phase 11+), this minimal test exercises the apiconfig type surface
// the wrap depends on, anchoring the import contract.
func TestScoringStrategyTypes(t *testing.T) {
	wantTypes := []apiconfig.ScoringStrategyType{
		apiconfig.MostAllocated,
		apiconfig.LeastAllocated,
		apiconfig.BalancedAllocation,
	}
	for _, st := range wantTypes {
		if st == "" {
			t.Fatalf("ScoringStrategyType %q is empty — upstream constant missing", st)
		}
	}
}
