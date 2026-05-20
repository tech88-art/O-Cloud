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

package hccs

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Default plugin args (ADR-0010 §4).
const (
	DefaultPreferAnnotation = "npu.huawei.com/preferred-hccs-ring"
	DefaultWeight           = 5
)

// HCCSTopologyArgs configures the HCCSTopology plugin per ADR-0010 §4.
//
// JSON-encoded inside KubeSchedulerConfiguration.profiles[*].pluginConfig.
// Operators tune Weight / FailIfMissing / Adjacency via the chart's
// values.yaml (P6-T-101).
//
// Default-substitution happens in parseArgs — fields left at zero in the
// passed object take DefaultXxx values from this file.
type HCCSTopologyArgs struct {
	metav1.TypeMeta `json:",inline"`

	// Weight is the priority weight applied by the framework to this
	// plugin's Score output. Validated to [0..100]. Ignored by Filter.
	Weight int32 `json:"weight,omitempty"`

	// PreferAnnotation is the Pod annotation key Filter / Score read for
	// the requested HCCS ring set. Default
	// "npu.huawei.com/preferred-hccs-ring".
	PreferAnnotation string `json:"preferAnnotation,omitempty"`

	// FailIfMissing makes Filter return UnschedulableAndUnresolvable when
	// a Pod lacks the PreferAnnotation. Default false (permissive — Pods
	// without the annotation still schedule, just without HCCS guidance).
	FailIfMissing bool `json:"failIfMissing,omitempty"`

	// Adjacency configures Score: maps a ring ID (string-keyed to allow
	// JSON encoding) to the list of "adjacent" rings that count as a
	// near-miss when computing co-location score. Default empty = no
	// adjacency (strict same-ring scoring only).
	Adjacency map[string][]int32 `json:"adjacency,omitempty"`
}

// DeepCopyObject satisfies runtime.Object so kube-scheduler can pass
// HCCSTopologyArgs through its codec. Implemented manually (no
// controller-gen).
func (a *HCCSTopologyArgs) DeepCopyObject() runtime.Object {
	if a == nil {
		return nil
	}
	cp := *a
	if a.Adjacency != nil {
		cp.Adjacency = make(map[string][]int32, len(a.Adjacency))
		for k, v := range a.Adjacency {
			cp.Adjacency[k] = append([]int32(nil), v...)
		}
	}
	return &cp
}

// defaultArgs returns HCCSTopologyArgs with ADR-0010 §4 defaults applied.
func defaultArgs() *HCCSTopologyArgs {
	return &HCCSTopologyArgs{
		Weight:           DefaultWeight,
		PreferAnnotation: DefaultPreferAnnotation,
	}
}

// parseArgs merges scheduler-passed args over default values. Accepts:
//   - nil → defaults
//   - *HCCSTopologyArgs → typed merge (zero fields fall back to defaults)
//   - *runtime.Unknown → JSON-decode .Raw into a fresh defaultArgs() then return
//
// Returns an error for any other type, or for Weight out of [0..100].
func parseArgs(obj runtime.Object) (*HCCSTopologyArgs, error) {
	out := defaultArgs()
	if obj == nil {
		return out, nil
	}
	switch v := obj.(type) {
	case *HCCSTopologyArgs:
		if v.Weight != 0 {
			out.Weight = v.Weight
		}
		if v.PreferAnnotation != "" {
			out.PreferAnnotation = v.PreferAnnotation
		}
		out.FailIfMissing = v.FailIfMissing
		out.Adjacency = v.Adjacency
	case *runtime.Unknown:
		if len(v.Raw) > 0 {
			if err := json.Unmarshal(v.Raw, out); err != nil {
				return nil, fmt.Errorf("parse HCCSTopologyArgs JSON: %w", err)
			}
			// JSON might have left these zero; re-apply non-zero defaults.
			if out.Weight == 0 {
				out.Weight = DefaultWeight
			}
			if out.PreferAnnotation == "" {
				out.PreferAnnotation = DefaultPreferAnnotation
			}
		}
	default:
		return nil, fmt.Errorf("HCCSTopologyArgs: unexpected runtime.Object type %T", obj)
	}
	if out.Weight < 0 || out.Weight > 100 {
		return nil, fmt.Errorf("HCCSTopologyArgs.Weight=%d out of range [0..100]", out.Weight)
	}
	return out, nil
}
