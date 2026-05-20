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
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

// fakeSliceLister implements SliceLister for unit tests by returning a
// pre-populated per-node slice list. Use newFakeLister(...) constructor.
type fakeSliceLister struct {
	byNode map[string][]*resourceapi.ResourceSlice
	err    error
}

func newFakeLister() *fakeSliceLister {
	return &fakeSliceLister{byNode: map[string][]*resourceapi.ResourceSlice{}}
}

func (f *fakeSliceLister) ListForNode(nodeName string) ([]*resourceapi.ResourceSlice, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byNode[nodeName], nil
}

// addSlice appends one ResourceSlice with the given devices to the per-node
// list. Mirrors npu-dra-driver publisher output shape.
func (f *fakeSliceLister) addSlice(nodeName string, devices ...resourceapi.Device) {
	slice := &resourceapi.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "fake-slice-" + nodeName,
			Labels: map[string]string{
				LabelManagedBy: LabelManagedByValue,
			},
		},
		Spec: resourceapi.ResourceSliceSpec{
			Driver:   "npu.ocloud.edge.example.com",
			NodeName: nodeName,
			Devices:  devices,
		},
	}
	f.byNode[nodeName] = append(f.byNode[nodeName], slice)
}

// makeDevice builds a ResourceSlice device with the standard NPU attribute
// set (hccs_ring + health). aiCores left at zero — Filter does not enforce
// capacity (see filter.go docblock; AICore Available delegated to DRA's
// claim-driven filter).
func makeDevice(name string, ring int64, health string) resourceapi.Device {
	ringCopy := ring
	healthCopy := health
	return resourceapi.Device{
		Name: name,
		Basic: &resourceapi.BasicDevice{
			Attributes: map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{
				AttrHCCSRing: {IntValue: &ringCopy},
				AttrHealth:   {StringValue: &healthCopy},
			},
		},
	}
}

// makePod builds a minimal Pod carrying (optionally) the
// preferred-hccs-ring annotation.
func makePod(annotation string) *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}
	if annotation != "" {
		pod.Annotations = map[string]string{
			DefaultPreferAnnotation: annotation,
		}
	}
	return pod
}

// makeNodeInfo wraps a Node into a framework.NodeInfo for Filter signature
// compliance.
func makeNodeInfo(nodeName string) *framework.NodeInfo {
	ni := framework.NewNodeInfo()
	ni.SetNode(&v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: nodeName},
	})
	return ni
}

// TestFilter exercises ADR-0010 §2 Filter table (6 cases).
func TestFilter(t *testing.T) {
	t.Run("annotation present, ring matches", func(t *testing.T) {
		lister := newFakeLister()
		lister.addSlice("worker-a", makeDevice("npu-0", 0, "Healthy"))
		p := NewForTest(nil, lister, nil)

		status := p.Filter(context.Background(), nil, makePod("0"), makeNodeInfo("worker-a"))
		if !status.IsSuccess() {
			t.Fatalf("expected Success, got %v: %s", status.Code(), status.Message())
		}
	})

	t.Run("annotation present, ring does not match", func(t *testing.T) {
		lister := newFakeLister()
		// Node only carries ring 0; Pod asks for ring 5.
		lister.addSlice("worker-a", makeDevice("npu-0", 0, "Healthy"))
		p := NewForTest(nil, lister, nil)

		status := p.Filter(context.Background(), nil, makePod("5"), makeNodeInfo("worker-a"))
		if status.Code() != framework.UnschedulableAndUnresolvable {
			t.Fatalf("expected UnschedulableAndUnresolvable, got %v: %s", status.Code(), status.Message())
		}
	})

	t.Run("annotation absent, permissive default (FailIfMissing=false)", func(t *testing.T) {
		lister := newFakeLister()
		p := NewForTest(nil, lister, nil)

		status := p.Filter(context.Background(), nil, makePod(""), makeNodeInfo("worker-a"))
		if !status.IsSuccess() {
			t.Fatalf("expected Success (permissive), got %v: %s", status.Code(), status.Message())
		}
	})

	t.Run("annotation absent, FailIfMissing=true", func(t *testing.T) {
		lister := newFakeLister()
		args := defaultArgs()
		args.FailIfMissing = true
		p := NewForTest(args, lister, nil)

		status := p.Filter(context.Background(), nil, makePod(""), makeNodeInfo("worker-a"))
		if status.Code() != framework.UnschedulableAndUnresolvable {
			t.Fatalf("expected UnschedulableAndUnresolvable, got %v: %s", status.Code(), status.Message())
		}
	})

	t.Run("multi-ring annotation, partial match (only one of requested rings present)", func(t *testing.T) {
		lister := newFakeLister()
		lister.addSlice("worker-a",
			makeDevice("npu-0", 0, "Healthy"),
			makeDevice("npu-1", 0, "Healthy"),
		)
		p := NewForTest(nil, lister, nil)

		// Pod asks for {0, 7}; node has only ring 0 → still matches.
		status := p.Filter(context.Background(), nil, makePod("0,7"), makeNodeInfo("worker-a"))
		if !status.IsSuccess() {
			t.Fatalf("expected Success (partial ring match), got %v: %s", status.Code(), status.Message())
		}
	})

	t.Run("unhealthy device on requested ring → does not satisfy", func(t *testing.T) {
		lister := newFakeLister()
		// Node has a device on ring 0 but it's Unhealthy; nothing else.
		lister.addSlice("worker-a", makeDevice("npu-0", 0, "Unhealthy"))
		p := NewForTest(nil, lister, nil)

		status := p.Filter(context.Background(), nil, makePod("0"), makeNodeInfo("worker-a"))
		if status.Code() != framework.UnschedulableAndUnresolvable {
			t.Fatalf("expected UnschedulableAndUnresolvable (unhealthy filtered), got %v: %s", status.Code(), status.Message())
		}
	})
}

// TestParsePreferredRings covers the annotation tokenizer.
func TestParsePreferredRings(t *testing.T) {
	cases := []struct {
		in   string
		want []int64
	}{
		{"", nil},
		{"0", []int64{0}},
		{"0,1", []int64{0, 1}},
		{" 0 , 1 ", []int64{0, 1}},
		{"0,,1", []int64{0, 1}},
		{"0,bad,1", []int64{0, 1}}, // malformed entries skipped silently
		{"bad", nil},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := parsePreferredRings(c.in)
			if !equalInt64Slice(got, c.want) {
				t.Fatalf("parsePreferredRings(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// TestParseArgs validates default substitution + Weight bounds.
func TestParseArgs(t *testing.T) {
	t.Run("nil → defaults", func(t *testing.T) {
		args, err := parseArgs(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if args.Weight != DefaultWeight {
			t.Fatalf("Weight = %d, want %d", args.Weight, DefaultWeight)
		}
		if args.PreferAnnotation != DefaultPreferAnnotation {
			t.Fatalf("PreferAnnotation = %q, want %q", args.PreferAnnotation, DefaultPreferAnnotation)
		}
		if args.FailIfMissing {
			t.Fatal("FailIfMissing should default to false")
		}
	})

	t.Run("typed args merge", func(t *testing.T) {
		in := &HCCSTopologyArgs{Weight: 10, FailIfMissing: true}
		out, err := parseArgs(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out.Weight != 10 || !out.FailIfMissing {
			t.Fatalf("merge failed: %+v", out)
		}
		if out.PreferAnnotation != DefaultPreferAnnotation {
			t.Fatalf("zero PreferAnnotation should fall back to default, got %q", out.PreferAnnotation)
		}
	})

	t.Run("Weight out of range rejected", func(t *testing.T) {
		_, err := parseArgs(&HCCSTopologyArgs{Weight: 999})
		if err == nil {
			t.Fatal("expected error for Weight=999")
		}
	})
}

func equalInt64Slice(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
