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

package v1alpha1

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
	resourceapi "k8s.io/api/resource/v1beta1"
)

// Attribute QualifiedNames set on upstream Device.Basic.Attributes.
// Schema documented in the package doc (types.go).
//
// QualifiedName format (P5-T-120 fix · 2026-05-20): the "name" segment
// after the `/` MUST be a valid C identifier — regex
// `[A-Za-z_][A-Za-z0-9_]*`. Hyphens are NOT allowed. Earlier values
// (`slice-strategy`, `ai-cores`, `numa-node`, `hccs-ring`, `slice-
// aicore`) were rejected at K8s admission with:
//   ResourceSlice.resource.k8s.io "..." is invalid:
//   spec.devices[i].basic.attributes[npu.huawei.com/slice-strategy]:
//     Invalid value: "slice-strategy": a valid C identifier must
//     start with alphabetic character or '_', followed by a string
//     of alphanumeric characters or '_'
// Switched all hyphens to underscores. Names with no hyphens (`index`,
// `health`) are unchanged.
const (
	AttrNPUIndex       resourceapi.QualifiedName = "npu.huawei.com/index"
	AttrNPUHealth      resourceapi.QualifiedName = "npu.huawei.com/health"
	AttrSliceStrategy  resourceapi.QualifiedName = "npu.huawei.com/slice_strategy"
	AttrAICores        resourceapi.QualifiedName = "npu.huawei.com/ai_cores"
	AttrNUMANode       resourceapi.QualifiedName = "npu.huawei.com/numa_node"
	AttrHCCSRing       resourceapi.QualifiedName = "npu.huawei.com/hccs_ring"
)

// Capacity QualifiedName set on upstream Device.Basic.Capacity.
// Same C-identifier constraint as attributes (P5-T-120 fix).
const (
	CapSliceAICore resourceapi.QualifiedName = "npu.huawei.com/slice_aicore"
)

// Enum values for AttrNPUHealth (mirror Ascend Device Plugin labels).
const (
	HealthHealthy   = "Healthy"
	HealthUnhealthy = "Unhealthy"
	HealthUnknown   = "Unknown"
)

// Enum values for AttrSliceStrategy (mirror NPUSlicePool CRD).
const (
	SliceStrategyFixedTemplate = "FixedTemplate"
	SliceStrategyDynamic       = "Dynamic"
)

// AscendDevice is the Ocloud-semantic typed view of one Ascend NPU device.
// It is a structured projection of the upstream
// resource.k8s.io/v1beta1.Device.Basic.Attributes and .Capacity maps.
//
// Use ToUpstream / FromUpstream to convert between this typed view and the
// upstream Device shape. The simulator publisher (P4-T-005) constructs
// AscendDevice values from configs/mock-data/set-a-small/npus.json and emits
// upstream Devices via ToUpstream.
//
// +kubebuilder:object:generate=true
type AscendDevice struct {
	// Name is the device name within the ResourceSlice (DNS label).
	// Conventionally <node-hostname>-npu-<index>.
	Name string `json:"name"`

	// Index is the physical NPU index on the host (0..7 for Ascend 910B).
	Index int64 `json:"index"`

	// Health mirrors the Ascend Device Plugin health state.
	// One of HealthHealthy / HealthUnhealthy / HealthUnknown.
	Health string `json:"health"`

	// SliceStrategy mirrors the NPUSlicePool.spec.strategy field.
	// One of SliceStrategyFixedTemplate / SliceStrategyDynamic.
	SliceStrategy string `json:"sliceStrategy"`

	// AICores is the slice AI-core count when SliceStrategy=Dynamic.
	// Zero when SliceStrategy=FixedTemplate.
	AICores int64 `json:"aiCores"`

	// NUMANode is the host NUMA node the NPU is attached to.
	// Sourced from Ascend Device Plugin labels.
	NUMANode int64 `json:"numaNode"`

	// HCCSRing is a Phase 6 placeholder; defaults to 0 until the
	// scheduler-plugins NUMA+HCCS module lands.
	HCCSRing int64 `json:"hccsRing"`

	// SliceAICoreCapacity is the per-device slice AI-core capacity
	// (resource.Quantity). pool-operator NPUSlicePool Reconcile reads
	// this as the upper bound.
	SliceAICoreCapacity resource.Quantity `json:"sliceAiCoreCapacity"`
}

// ToUpstream renders this typed view as an upstream
// resource.k8s.io/v1beta1.Device suitable for inclusion in a
// ResourceSlice.spec.devices array.
func (a AscendDevice) ToUpstream() resourceapi.Device {
	attrs := map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{
		AttrNPUIndex:      {IntValue: int64Ptr(a.Index)},
		AttrNPUHealth:     {StringValue: stringPtr(a.Health)},
		AttrSliceStrategy: {StringValue: stringPtr(a.SliceStrategy)},
		AttrAICores:       {IntValue: int64Ptr(a.AICores)},
		AttrNUMANode:      {IntValue: int64Ptr(a.NUMANode)},
		AttrHCCSRing:      {IntValue: int64Ptr(a.HCCSRing)},
	}
	caps := map[resourceapi.QualifiedName]resourceapi.DeviceCapacity{
		CapSliceAICore: {Value: a.SliceAICoreCapacity},
	}
	return resourceapi.Device{
		Name: a.Name,
		Basic: &resourceapi.BasicDevice{
			Attributes: attrs,
			Capacity:   caps,
		},
	}
}

// AscendDeviceFromUpstream parses an upstream Device back into the typed
// Ocloud view. Returns a soft error for missing required attributes; unknown
// attributes are tolerated (warn-not-error) so a Phase 5+ upstream Device
// carrying additional Ocloud or third-party attributes round-trips cleanly.
//
// Required attributes:
//   - AttrNPUIndex, AttrNPUHealth, AttrSliceStrategy
//
// Optional attributes (default to zero value if missing):
//   - AttrAICores, AttrNUMANode, AttrHCCSRing
//
// Required capacity:
//   - CapSliceAICore (defaults to zero Quantity if missing — caller decides
//     whether that is acceptable)
func AscendDeviceFromUpstream(d resourceapi.Device) (AscendDevice, error) {
	out := AscendDevice{Name: d.Name}

	if d.Basic == nil {
		return out, fmt.Errorf("device %q has nil Basic; v1beta1 Devices must be Basic", d.Name)
	}

	var missing []string
	attrs := d.Basic.Attributes

	if v, ok := attrs[AttrNPUIndex]; ok && v.IntValue != nil {
		out.Index = *v.IntValue
	} else {
		missing = append(missing, string(AttrNPUIndex))
	}
	if v, ok := attrs[AttrNPUHealth]; ok && v.StringValue != nil {
		out.Health = *v.StringValue
	} else {
		missing = append(missing, string(AttrNPUHealth))
	}
	if v, ok := attrs[AttrSliceStrategy]; ok && v.StringValue != nil {
		out.SliceStrategy = *v.StringValue
	} else {
		missing = append(missing, string(AttrSliceStrategy))
	}

	if v, ok := attrs[AttrAICores]; ok && v.IntValue != nil {
		out.AICores = *v.IntValue
	}
	if v, ok := attrs[AttrNUMANode]; ok && v.IntValue != nil {
		out.NUMANode = *v.IntValue
	}
	if v, ok := attrs[AttrHCCSRing]; ok && v.IntValue != nil {
		out.HCCSRing = *v.IntValue
	}

	if cap, ok := d.Basic.Capacity[CapSliceAICore]; ok {
		out.SliceAICoreCapacity = cap.Value
	}

	if len(missing) > 0 {
		return out, fmt.Errorf("device %q missing required attributes: %v", d.Name, missing)
	}
	return out, nil
}

// ValidateAttributes confirms a Device carries all Ocloud-required
// attributes and rejects empty / nonsense enum values. Returns nil iff the
// Device is fully Ocloud-compliant. Unknown attributes do not produce errors
// — see the AscendDeviceFromUpstream doc for the warn-not-error contract.
func ValidateAttributes(d resourceapi.Device) error {
	a, err := AscendDeviceFromUpstream(d)
	if err != nil {
		return err
	}
	switch a.Health {
	case HealthHealthy, HealthUnhealthy, HealthUnknown:
	default:
		return fmt.Errorf("device %q has invalid health %q (must be Healthy/Unhealthy/Unknown)", a.Name, a.Health)
	}
	switch a.SliceStrategy {
	case SliceStrategyFixedTemplate, SliceStrategyDynamic:
	default:
		return fmt.Errorf("device %q has invalid sliceStrategy %q (must be FixedTemplate/Dynamic)", a.Name, a.SliceStrategy)
	}
	if a.SliceStrategy == SliceStrategyDynamic && a.AICores <= 0 {
		return errors.New("Dynamic slice strategy requires ai-cores > 0")
	}
	return nil
}

func int64Ptr(v int64) *int64    { return &v }
func stringPtr(v string) *string { return &v }
