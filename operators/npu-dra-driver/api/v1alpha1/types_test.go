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
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
	resourceapi "k8s.io/api/resource/v1beta1"
)

// TestRoundTrip exercises AscendDevice <-> upstream Device conversion
// across the 5 cases specified in docs/phase4-plan.md §3 P4-T-004:
//   - empty
//   - single-device
//   - multi-device
//   - Dynamic strategy (AICores > 0)
//   - unknown attribute warn-not-error
func TestRoundTrip(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		// An empty AscendDevice (zero value) round-trips: ToUpstream produces a
		// Device with all attrs set to zero / empty; FromUpstream returns the
		// same zero-valued AscendDevice. The required-attribute check fires
		// because Health and SliceStrategy are empty strings.
		empty := AscendDevice{}
		up := empty.ToUpstream()
		back, err := AscendDeviceFromUpstream(up)
		if err != nil {
			t.Fatalf("empty round-trip error (allowed for zero-value Ocloud Device): %v", err)
		}
		if back.Index != 0 || back.AICores != 0 || back.NUMANode != 0 {
			t.Errorf("empty round-trip: numeric fields drifted: %+v", back)
		}
		if back.Health != "" || back.SliceStrategy != "" {
			t.Errorf("empty round-trip: string fields drifted: %+v", back)
		}
		// Empty Devices fail ValidateAttributes (Health enum invalid).
		if vErr := ValidateAttributes(up); vErr == nil {
			t.Errorf("empty Device should fail ValidateAttributes (empty Health)")
		}
	})

	t.Run("single-device", func(t *testing.T) {
		// One physical NPU, FixedTemplate strategy. Mirrors what the
		// simulator publisher (P4-T-005) emits from set-a-small/npus.json.
		in := AscendDevice{
			Name:                "node-0-npu-3",
			Index:               3,
			Health:              HealthHealthy,
			SliceStrategy:       SliceStrategyFixedTemplate,
			AICores:             0,
			NUMANode:            0,
			HCCSRing:            0,
			SliceAICoreCapacity: resource.MustParse("32"),
		}
		up := in.ToUpstream()
		out, err := AscendDeviceFromUpstream(up)
		if err != nil {
			t.Fatalf("single round-trip err: %v", err)
		}
		assertAscendDeviceEqual(t, in, out)
		if vErr := ValidateAttributes(up); vErr != nil {
			t.Errorf("single device should pass ValidateAttributes: %v", vErr)
		}
	})

	t.Run("multi-device", func(t *testing.T) {
		// Two NPUs on the same node, both FixedTemplate. We round-trip each
		// independently and assert all attributes preserved.
		set := []AscendDevice{
			{Name: "n-0-npu-0", Index: 0, Health: HealthHealthy, SliceStrategy: SliceStrategyFixedTemplate, NUMANode: 0, HCCSRing: 0, SliceAICoreCapacity: resource.MustParse("32")},
			{Name: "n-0-npu-1", Index: 1, Health: HealthUnhealthy, SliceStrategy: SliceStrategyFixedTemplate, NUMANode: 1, HCCSRing: 0, SliceAICoreCapacity: resource.MustParse("32")},
		}
		for _, in := range set {
			up := in.ToUpstream()
			out, err := AscendDeviceFromUpstream(up)
			if err != nil {
				t.Fatalf("multi round-trip err for %s: %v", in.Name, err)
			}
			assertAscendDeviceEqual(t, in, out)
		}
	})

	t.Run("Dynamic-strategy", func(t *testing.T) {
		// Dynamic strategy requires AICores > 0; ValidateAttributes enforces.
		in := AscendDevice{
			Name:                "n-1-npu-0",
			Index:               0,
			Health:              HealthHealthy,
			SliceStrategy:       SliceStrategyDynamic,
			AICores:             8,
			NUMANode:            0,
			HCCSRing:            1,
			SliceAICoreCapacity: resource.MustParse("32"),
		}
		up := in.ToUpstream()
		out, err := AscendDeviceFromUpstream(up)
		if err != nil {
			t.Fatalf("Dynamic round-trip err: %v", err)
		}
		assertAscendDeviceEqual(t, in, out)
		if vErr := ValidateAttributes(up); vErr != nil {
			t.Errorf("Dynamic with AICores=8 should validate: %v", vErr)
		}

		// Negative: Dynamic with AICores=0 must fail validation.
		bad := in
		bad.AICores = 0
		badUp := bad.ToUpstream()
		if vErr := ValidateAttributes(badUp); vErr == nil {
			t.Errorf("Dynamic with AICores=0 must fail ValidateAttributes")
		}
	})

	t.Run("unknown-attribute-warn-not-error", func(t *testing.T) {
		// Build an upstream Device carrying an Ocloud attribute we do not
		// recognise (e.g. a future Phase 5+ extension or a third-party
		// driver's attribute). FromUpstream must tolerate it — required
		// attrs still parse; the unknown attribute is silently ignored.
		base := AscendDevice{
			Name:                "n-2-npu-0",
			Index:               7,
			Health:              HealthHealthy,
			SliceStrategy:       SliceStrategyFixedTemplate,
			NUMANode:            1,
			HCCSRing:            0,
			SliceAICoreCapacity: resource.MustParse("32"),
		}
		up := base.ToUpstream()

		// Inject an unknown attribute.
		unknownKey := resourceapi.QualifiedName("npu.huawei.com/future-phase5-field")
		futureVal := "hypothetical"
		up.Basic.Attributes[unknownKey] = resourceapi.DeviceAttribute{StringValue: &futureVal}

		out, err := AscendDeviceFromUpstream(up)
		if err != nil {
			t.Fatalf("unknown-attribute round-trip must not error: %v", err)
		}
		assertAscendDeviceEqual(t, base, out)
		if vErr := ValidateAttributes(up); vErr != nil {
			t.Errorf("unknown-attribute Device must still ValidateAttributes (warn-not-error): %v", vErr)
		}
	})
}

// TestClaimAnnotationsRoundTrip exercises AscendClaimAnnotations <-> map
// conversion. Mirror of the TestRoundTrip cases but for ResourceClaim
// annotations rather than ResourceSlice Device attributes.
func TestClaimAnnotationsRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		in   AscendClaimAnnotations
	}{
		{"empty", AscendClaimAnnotations{}},
		{"model-ref-only", AscendClaimAnnotations{ModelServiceRef: "default/llama-7b"}},
		{"preferred-pool-only", AscendClaimAnnotations{PreferredPool: "edge-pool-a"}},
		{"both", AscendClaimAnnotations{ModelServiceRef: "ns/svc", PreferredPool: "p1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := c.in.ToMap()
			out := AscendClaimAnnotationsFromMap(m)
			if out != c.in {
				t.Errorf("round-trip drift: in=%+v out=%+v map=%+v", c.in, out, m)
			}
		})
	}

	// Unknown annotation keys round-trip safely.
	t.Run("unknown-key-warn-not-error", func(t *testing.T) {
		m := map[string]string{
			AnnotationModelServiceRef:                "ns/svc",
			"ocloud.edge.example.com/future-phase5": "ignored",
			"other-org.example.com/anything":         "also-ignored",
		}
		out := AscendClaimAnnotationsFromMap(m)
		if out.ModelServiceRef != "ns/svc" {
			t.Errorf("expected ModelServiceRef preserved, got %+v", out)
		}
		if out.PreferredPool != "" {
			t.Errorf("expected empty PreferredPool, got %+v", out)
		}
	})
}

func assertAscendDeviceEqual(t *testing.T, want, got AscendDevice) {
	t.Helper()
	if want.Name != got.Name ||
		want.Index != got.Index ||
		want.Health != got.Health ||
		want.SliceStrategy != got.SliceStrategy ||
		want.AICores != got.AICores ||
		want.NUMANode != got.NUMANode ||
		want.HCCSRing != got.HCCSRing {
		t.Errorf("AscendDevice mismatch:\n  want=%+v\n  got =%+v", want, got)
	}
	if want.SliceAICoreCapacity.Cmp(got.SliceAICoreCapacity) != 0 {
		t.Errorf("SliceAICoreCapacity mismatch: want=%s got=%s", want.SliceAICoreCapacity.String(), got.SliceAICoreCapacity.String())
	}
}
