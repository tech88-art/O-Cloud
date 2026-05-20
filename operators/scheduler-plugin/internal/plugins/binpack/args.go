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

package binpack

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// NPUExtendedResource is the K8s extended-resource name Ocloud Ascend NPUs
// expose via the device plugin / DRA driver. Mirrors arch §5.5 + Ascend
// Device Plugin v1 convention. Default ResourceWeights below give this
// resource the heaviest weight so Binpack prefers consolidating Pods onto
// already-loaded NPU nodes.
const NPUExtendedResource = "npu.ocloud.edge.example.com/devices"

// Default plugin args (ADR-0010 §4 — Binpack default disabled).
const (
	DefaultWeight  = 1
	DefaultEnabled = false
)

// BinpackArgs configures the Binpack plugin per ADR-0010 §4.
//
// Default ResourceWeights when none provided: {cpu: 1, memory: 1,
// npu.ocloud.edge.example.com/devices: 5}.
type BinpackArgs struct {
	metav1.TypeMeta `json:",inline"`

	// Weight is the priority weight applied by the framework when this
	// plugin's score participates in the profile's score sum. Default 1.
	Weight int32 `json:"weight,omitempty"`

	// Enabled toggles Binpack as a no-op when false. Default false so the
	// chart can ship without binpack effects until operators opt in via
	// values.yaml.
	Enabled bool `json:"enabled,omitempty"`

	// ResourceWeights maps K8s resource names to per-resource weights.
	// Higher weight = more sensitive to that resource's demand/capacity
	// ratio. Resources not in the map are skipped during scoring.
	ResourceWeights map[string]int64 `json:"resourceWeights,omitempty"`
}

// DeepCopyObject implements runtime.Object so kube-scheduler can pass
// BinpackArgs through its codec. Manual (no controller-gen).
func (a *BinpackArgs) DeepCopyObject() runtime.Object {
	if a == nil {
		return nil
	}
	cp := *a
	if a.ResourceWeights != nil {
		cp.ResourceWeights = make(map[string]int64, len(a.ResourceWeights))
		for k, v := range a.ResourceWeights {
			cp.ResourceWeights[k] = v
		}
	}
	return &cp
}

// defaultArgs returns BinpackArgs with ADR-0010 §4 defaults applied.
func defaultArgs() *BinpackArgs {
	return &BinpackArgs{
		Weight:  DefaultWeight,
		Enabled: DefaultEnabled,
		ResourceWeights: map[string]int64{
			"cpu":              1,
			"memory":           1,
			NPUExtendedResource: 5,
		},
	}
}

// parseArgs merges scheduler-passed args over default values. Accepts:
//   - nil → defaults
//   - *BinpackArgs → typed merge (zero / nil fields fall back to defaults)
//   - *runtime.Unknown → JSON-decode .Raw into a fresh defaultArgs() then return
//
// Validates Weight in [0..100] and rejects unexpected runtime.Object types.
func parseArgs(obj runtime.Object) (*BinpackArgs, error) {
	out := defaultArgs()
	if obj == nil {
		return out, nil
	}
	switch v := obj.(type) {
	case *BinpackArgs:
		if v.Weight != 0 {
			out.Weight = v.Weight
		}
		out.Enabled = v.Enabled
		if v.ResourceWeights != nil {
			out.ResourceWeights = v.ResourceWeights
		}
	case *runtime.Unknown:
		if len(v.Raw) > 0 {
			if err := json.Unmarshal(v.Raw, out); err != nil {
				return nil, fmt.Errorf("parse BinpackArgs JSON: %w", err)
			}
			if out.Weight == 0 {
				out.Weight = DefaultWeight
			}
			if out.ResourceWeights == nil {
				out.ResourceWeights = defaultArgs().ResourceWeights
			}
		}
	default:
		return nil, fmt.Errorf("BinpackArgs: unexpected runtime.Object type %T", obj)
	}
	if out.Weight < 0 || out.Weight > 100 {
		return nil, fmt.Errorf("BinpackArgs.Weight=%d out of range [0..100]", out.Weight)
	}
	return out, nil
}
