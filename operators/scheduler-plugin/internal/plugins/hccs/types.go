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
	"strconv"
	"strings"

	resourceapi "k8s.io/api/resource/v1beta1"
)

// ResourceSlice attribute keys + label / value constants.
//
// Text-copied (no Go import) per operators/CLAUDE.md §1
// "module path 不交叉依赖" rule. Sources:
//   - npu.ocloud.edge.example.com/managed-by + npu-dra-driver:
//     operators/npu-dra-driver/internal/publisher/publisher.go
//   - npu.huawei.com/{hccs_ring,health,ai_cores}:
//     operators/npu-dra-driver/api/v1alpha1/resourceslice_types.go
//
// ADR-0010 §5 freezes this schema; the text copies here stay in sync via
// PR review (not Go import).
const (
	LabelManagedBy        = "npu.ocloud.edge.example.com/managed-by"
	LabelManagedByValue   = "npu-dra-driver"
	AttrHCCSRing          = "npu.huawei.com/hccs_ring"
	AttrNUMANode          = "npu.huawei.com/numa_node"
	AttrHealth            = "npu.huawei.com/health"
	AttrAICores           = "npu.huawei.com/ai_cores"
	HealthValueHealthy    = "Healthy"
)

// sliceLister abstracts ResourceSlice lookup for one node so Filter / Score
// can be tested without spinning up a real SharedInformerFactory. Production
// wires this to
// `framework.Handle.SharedInformerFactory().Resource().V1beta1().
// ResourceSlices().Lister()` (see informerSliceLister in plugin.go).
type sliceLister interface {
	// ListForNode returns ResourceSlices pinned to nodeName, filtered to
	// those labelled `npu.ocloud.edge.example.com/managed-by=npu-dra-driver`.
	// Returns an empty (not-nil) slice for "no slices for this node".
	ListForNode(nodeName string) ([]*resourceapi.ResourceSlice, error)
}

// parsePreferredRings parses the Pod annotation value into a sorted-as-given
// list of int64 ring IDs. Empty input / no valid ints → nil. Malformed
// (non-integer) entries are skipped silently so a single typo doesn't fail
// the whole pod (Filter callers still need to handle "no valid rings"
// separately via len check).
func parsePreferredRings(annotation string) []int64 {
	if annotation == "" {
		return nil
	}
	var out []int64
	for _, raw := range strings.Split(annotation, ",") {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			continue
		}
		out = append(out, v)
	}
	return out
}

// deviceMatchesRing reports whether device d is on any ring in wantedRings
// AND is healthy. Both conditions must hold. Used by Filter to decide if a
// node can satisfy a Pod's HCCS ring preference.
func deviceMatchesRing(d resourceapi.Device, wantedRings []int64) bool {
	if d.Basic == nil {
		return false
	}
	if !deviceHealthy(d) {
		return false
	}
	ringAttr, ok := d.Basic.Attributes[AttrHCCSRing]
	if !ok || ringAttr.IntValue == nil {
		return false
	}
	ring := *ringAttr.IntValue
	for _, w := range wantedRings {
		if w == ring {
			return true
		}
	}
	return false
}

// deviceHealthy returns true when the device's npu.huawei.com/health attr
// equals "Healthy", OR the attribute is absent (back-compat: pre-T002
// publishers may not emit it). Anything else (Unhealthy / Unknown / unexpected
// payload) returns false.
func deviceHealthy(d resourceapi.Device) bool {
	if d.Basic == nil {
		return false
	}
	healthAttr, ok := d.Basic.Attributes[AttrHealth]
	if !ok {
		return true
	}
	if healthAttr.StringValue == nil {
		return false
	}
	return *healthAttr.StringValue == HealthValueHealthy
}
