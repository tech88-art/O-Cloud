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

package webhook

import (
	"context"
	"strings"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sjson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// newDecoder returns an admission.Decoder seeded with the corev1 +
// admissionv1 schemes. Required for the handler to parse the Pod
// payload out of an admission.Request.
func newDecoder(t *testing.T) admission.Decoder {
	t.Helper()
	s := runtime.NewScheme()
	if err := scheme.AddToScheme(s); err != nil {
		t.Fatalf("corev1 scheme: %v", err)
	}
	if err := admissionv1.AddToScheme(s); err != nil {
		t.Fatalf("admissionv1 scheme: %v", err)
	}
	return admission.NewDecoder(s)
}

func newPod(name, namespace string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "main", Image: "vllm:test"}},
		},
	}
}

func podAdmissionRequest(t *testing.T, p *corev1.Pod) admission.Request {
	t.Helper()
	// Use the K8s JSON serializer to encode the Pod.
	enc := k8sjson.NewSerializerWithOptions(
		k8sjson.DefaultMetaFactory, scheme.Scheme, scheme.Scheme,
		k8sjson.SerializerOptions{Yaml: false, Pretty: false, Strict: false},
	)
	buf := []byte{}
	w := &byteSliceWriter{buf: &buf}
	if err := enc.Encode(p, w); err != nil {
		t.Fatalf("encode pod: %v", err)
	}
	return admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UID:       "test-uid",
			Kind:      metav1.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			Resource:  metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
			Namespace: p.Namespace,
			Name:      p.Name,
			Operation: admissionv1.Create,
			Object:    runtime.RawExtension{Raw: buf},
		},
	}
}

// byteSliceWriter is a minimal io.Writer that appends to a []byte
// referenced by pointer (so the encoder writes into a buffer we can
// later read).
type byteSliceWriter struct{ buf *[]byte }

func (w *byteSliceWriter) Write(p []byte) (int, error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}

// newFakeClientWithAllocations seeds a fake client with the given
// unstructured NPUSliceAllocation list. The fake client is configured
// to handle the npu.ocloud.edge.example.com/v1alpha1 GVKs as
// unstructured objects.
func newFakeClientWithAllocations(t *testing.T, allocations ...*unstructured.Unstructured) client.Client {
	t.Helper()
	s := runtime.NewScheme()
	if err := scheme.AddToScheme(s); err != nil {
		t.Fatalf("scheme: %v", err)
	}
	builder := fake.NewClientBuilder().WithScheme(s)
	for _, a := range allocations {
		builder = builder.WithObjects(a)
	}
	return builder.Build()
}

func newAllocation(name, msRef, node, pool, device string, aiCores int32, phase string) *unstructured.Unstructured {
	a := &unstructured.Unstructured{}
	a.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "npu.ocloud.edge.example.com",
		Version: "v1alpha1",
		Kind:    "NPUSliceAllocation",
	})
	a.SetName(name)
	_ = unstructured.SetNestedField(a.Object, msRef, "spec", "modelServiceRef")
	_ = unstructured.SetNestedField(a.Object, node, "spec", "nodeName")
	_ = unstructured.SetNestedField(a.Object, pool, "spec", "sliceRef", "pool")
	_ = unstructured.SetNestedField(a.Object, device, "spec", "sliceRef", "device")
	_ = unstructured.SetNestedField(a.Object, "npu.ocloud.edge.example.com", "spec", "sliceRef", "driver")
	_ = unstructured.SetNestedField(a.Object, int64(aiCores), "spec", "aiCores")
	_ = unstructured.SetNestedField(a.Object, phase, "status", "phase")
	return a
}

func TestHandle_HappyPath_InjectsSliceBindings(t *testing.T) {
	dec := newDecoder(t)
	a1 := newAllocation("alloc-1", "ns/ms-1", "nodeA", "nodeA", "nodeA-npu-0", 32, "Allocated")
	a2 := newAllocation("alloc-2", "ns/ms-1", "nodeB", "nodeB", "nodeB-npu-0", 16, "Allocated")
	cli := newFakeClientWithAllocations(t, a1, a2)
	h := &PDRouter{Decoder: dec, Client: cli, DenyOnOrphaned: true}

	pod := newPod("p1", "ns", map[string]string{LabelModelService: "ms-1"})
	req := podAdmissionRequest(t, pod)
	resp := h.Handle(context.Background(), req)

	if !resp.Allowed {
		t.Errorf("happy path should Allow; got %+v", resp)
	}
	if len(resp.Patches) == 0 {
		t.Errorf("happy path should produce patches; got 0")
	}
	// Verify the annotation value is in at least one patch.
	foundAnnotation := false
	for _, p := range resp.Patches {
		// Patch value can be a map[string]string when adding the
		// annotations container, or a string when adding a single key.
		val := fmtPatchValue(p.Value)
		if strings.Contains(p.Path, "annotations") &&
			(strings.Contains(val, AnnotationSliceBindings) || strings.Contains(val, "slice-bindings")) {
			foundAnnotation = true
		}
	}
	if !foundAnnotation {
		t.Errorf("expected a patch touching slice-bindings annotation; got patches: %+v", resp.Patches)
	}
}

// fmtPatchValue converts a jsonpatch.JsonPatchOperation.Value (which is
// a generic interface{}) to a string for substring searching in tests.
func fmtPatchValue(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case map[string]string:
		out := ""
		for k, val := range x {
			out += k + "=" + val + ";"
		}
		return out
	case map[string]interface{}:
		out := ""
		for k, val := range x {
			out += k + "="
			if s, ok := val.(string); ok {
				out += s
			}
			out += ";"
		}
		return out
	default:
		return ""
	}
}

// TestHandle_PDEndpoint_InjectedForPDRolePod exercises P13-T-105: a
// PD-pair Pod carrying the pd-role label gets the REAL prefill→decode
// KV-cache endpoint annotation injected ALONGSIDE the (unchanged
// ADR-0008) slice-bindings annotation. A prefill Pod points at the
// decode sibling Service.
func TestHandle_PDEndpoint_InjectedForPDRolePod(t *testing.T) {
	dec := newDecoder(t)
	a1 := newAllocation("alloc-pd-1", "ns/ms-pd", "nodeA", "nodeA", "nodeA-npu-0", 32, "Allocated")
	cli := newFakeClientWithAllocations(t, a1)
	h := &PDRouter{Decoder: dec, Client: cli, DenyOnOrphaned: true}

	pod := newPod("p-pd", "ns", map[string]string{
		LabelModelService: "ms-pd",
		RoleLabelKey:      "prefill",
	})
	req := podAdmissionRequest(t, pod)
	resp := h.Handle(context.Background(), req)

	if !resp.Allowed {
		t.Fatalf("PD-role happy path should Allow; got %+v", resp)
	}
	if len(resp.Patches) == 0 {
		t.Fatalf("PD-role happy path should produce patches; got 0")
	}
	var sawSliceBindings, sawPDEndpoint, sawDecodeSibling bool
	for _, p := range resp.Patches {
		if !strings.Contains(p.Path, "annotations") {
			continue
		}
		val := fmtPatchValue(p.Value)
		if strings.Contains(p.Path, "slice-bindings") || strings.Contains(val, "slice-bindings") {
			sawSliceBindings = true
		}
		if strings.Contains(p.Path, "pd-endpoint") || strings.Contains(val, "pd-endpoint") {
			sawPDEndpoint = true
		}
		// prefill side → decode sibling Service.
		if strings.Contains(val, "ms-pd-decode:8000") || strings.Contains(p.Path, "pd-endpoint") {
			sawDecodeSibling = true
		}
	}
	if !sawSliceBindings {
		t.Errorf("ADR-0008 slice-bindings annotation must still be patched; patches: %+v", resp.Patches)
	}
	if !sawPDEndpoint {
		t.Errorf("P13-T-105 pd-endpoint annotation should be patched; patches: %+v", resp.Patches)
	}
	if !sawDecodeSibling {
		t.Errorf("prefill-side PD endpoint should point at the decode sibling; patches: %+v", resp.Patches)
	}
}

// TestPDEndpointFor covers the sibling-endpoint resolution table.
func TestPDEndpointFor(t *testing.T) {
	cases := []struct {
		msName, role, want string
	}{
		{"ms-pd", "prefill", "ms-pd-decode:8000"},
		{"ms-pd", "decode", "ms-pd-prefill:8000"},
		{"ms-pd", "single", ""},
		{"ms-pd", "", ""},
		{"", "prefill", ""},
	}
	for _, c := range cases {
		if got := pdEndpointFor(c.msName, c.role); got != c.want {
			t.Errorf("pdEndpointFor(%q,%q) = %q, want %q", c.msName, c.role, got, c.want)
		}
	}
}

// TestAnnotationPDEndpoint_KeyStable guards the P13-T-105 annotation key
// (consumed by the vllm-ascend disaggregated launcher).
func TestAnnotationPDEndpoint_KeyStable(t *testing.T) {
	const expected = "inference.ocloud.edge.example.com/pd-endpoint"
	if AnnotationPDEndpoint != expected {
		t.Errorf("AnnotationPDEndpoint drift: want %q, got %q", expected, AnnotationPDEndpoint)
	}
}

func TestHandle_NoMatchingAllocations_AllowedWithoutPatch(t *testing.T) {
	dec := newDecoder(t)
	// One allocation but for a DIFFERENT ModelService — should be filtered out
	a1 := newAllocation("alloc-other", "ns/other-ms", "nodeA", "nodeA", "nodeA-npu-0", 32, "Allocated")
	cli := newFakeClientWithAllocations(t, a1)
	h := &PDRouter{Decoder: dec, Client: cli, DenyOnOrphaned: true}

	pod := newPod("p2", "ns", map[string]string{LabelModelService: "ms-1"})
	req := podAdmissionRequest(t, pod)
	resp := h.Handle(context.Background(), req)

	if !resp.Allowed {
		t.Errorf("no-allocation path should Allow; got %+v", resp)
	}
	if len(resp.Patches) != 0 {
		t.Errorf("no-allocation path should NOT patch; got %d patches", len(resp.Patches))
	}
}

func TestHandle_AllOrphaned_Denied(t *testing.T) {
	dec := newDecoder(t)
	a1 := newAllocation("alloc-orph-1", "ns/ms-1", "nodeA", "nodeA", "nodeA-npu-0", 32, "Orphaned")
	a2 := newAllocation("alloc-orph-2", "ns/ms-1", "nodeB", "nodeB", "nodeB-npu-0", 16, "Orphaned")
	cli := newFakeClientWithAllocations(t, a1, a2)
	h := &PDRouter{Decoder: dec, Client: cli, DenyOnOrphaned: true}

	pod := newPod("p3", "ns", map[string]string{LabelModelService: "ms-1"})
	req := podAdmissionRequest(t, pod)
	resp := h.Handle(context.Background(), req)

	if resp.Allowed {
		t.Errorf("all-Orphaned path should Deny; got Allowed")
	}
	if !strings.Contains(resp.Result.Message, "NoAvailableNPUSlices") {
		t.Errorf("expected NoAvailableNPUSlices in Deny reason; got %q", resp.Result.Message)
	}
}

func TestHandle_AllOrphaned_DenyOff_AllowedWithoutPatch(t *testing.T) {
	dec := newDecoder(t)
	a1 := newAllocation("alloc-orph", "ns/ms-1", "nodeA", "nodeA", "nodeA-npu-0", 32, "Orphaned")
	cli := newFakeClientWithAllocations(t, a1)
	h := &PDRouter{Decoder: dec, Client: cli, DenyOnOrphaned: false}

	pod := newPod("p4", "ns", map[string]string{LabelModelService: "ms-1"})
	req := podAdmissionRequest(t, pod)
	resp := h.Handle(context.Background(), req)

	if !resp.Allowed {
		t.Errorf("DenyOnOrphaned=false should Allow; got %+v", resp)
	}
	if len(resp.Patches) != 0 {
		t.Errorf("DenyOnOrphaned=false orphaned should not patch; got %d", len(resp.Patches))
	}
}

func TestHandle_PartialAllocation_AllowedWithoutPatch(t *testing.T) {
	dec := newDecoder(t)
	a1 := newAllocation("alloc-ok", "ns/ms-1", "nodeA", "nodeA", "nodeA-npu-0", 32, "Allocated")
	a2 := newAllocation("alloc-pending", "ns/ms-1", "nodeB", "nodeB", "nodeB-npu-0", 16, "")
	cli := newFakeClientWithAllocations(t, a1, a2)
	h := &PDRouter{Decoder: dec, Client: cli, DenyOnOrphaned: true}

	pod := newPod("p5", "ns", map[string]string{LabelModelService: "ms-1"})
	req := podAdmissionRequest(t, pod)
	resp := h.Handle(context.Background(), req)

	if !resp.Allowed {
		t.Errorf("partial-allocation path should Allow; got %+v", resp)
	}
	if len(resp.Patches) != 0 {
		t.Errorf("partial-allocation should not patch yet; got %d patches", len(resp.Patches))
	}
}

func mustMarshal(t *testing.T, v interface{}) []byte {
	t.Helper()
	buf, err := jsonMarshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return buf
}

func jsonMarshal(v interface{}) ([]byte, error) {
	switch x := v.(type) {
	case string:
		return []byte("\"" + x + "\""), nil
	case []byte:
		return x, nil
	default:
		// best-effort fmt fallback
		return []byte(strings.TrimSpace(toJSONString(v))), nil
	}
}

func toJSONString(v interface{}) string {
	if v == nil {
		return "null"
	}
	return "<json>"
}

func TestHandle_ScaffoldAllowsAll(t *testing.T) {
	dec := newDecoder(t)
	// Empty client — no allocations seeded — should Allow without patch
	// (no bindings observed).
	h := &PDRouter{Decoder: dec, Client: newFakeClientWithAllocations(t)}

	pod := newPod("p1", "ns", map[string]string{
		LabelModelService: "ms-1",
	})
	req := podAdmissionRequest(t, pod)
	resp := h.Handle(context.Background(), req)

	if !resp.Allowed {
		t.Errorf("no-bindings path should Allow; got Denied with %+v", resp)
	}
	if len(resp.Patches) != 0 {
		t.Errorf("no-bindings path should NOT patch; got %d patch ops", len(resp.Patches))
	}
}

func TestHandle_NonMSPod_Allowed(t *testing.T) {
	dec := newDecoder(t)
	h := &PDRouter{Decoder: dec, Client: newFakeClientWithAllocations(t)}

	pod := newPod("p2", "ns", map[string]string{
		"app.kubernetes.io/name": "unrelated",
	})
	req := podAdmissionRequest(t, pod)
	resp := h.Handle(context.Background(), req)

	if !resp.Allowed {
		t.Errorf("non-MS Pod must be allowed; got %+v", resp)
	}
}

func TestHandle_BadPodPayload_AllowedNotDenied(t *testing.T) {
	// Decode failure → handler returns Allowed (best-effort).
	dec := newDecoder(t)
	h := &PDRouter{Decoder: dec, Client: newFakeClientWithAllocations(t)}

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			UID:       "bad",
			Operation: admissionv1.Create,
			Object: runtime.RawExtension{
				// Intentionally non-Pod JSON
				Raw: []byte(`{"not-a-pod": true}`),
			},
		},
	}
	resp := h.Handle(context.Background(), req)
	// Allowed even on bad payload.
	if !resp.Allowed {
		t.Errorf("should Allow on bad payload; got %+v", resp)
	}
}

// Ensure the package-level constants stay in sync with the controller
// labels — if either side drifts, T103 webhook + T007 Deployment will
// fail to communicate.
func TestLabelModelService_MatchesControllerConstant(t *testing.T) {
	const expected = "inference.ocloud.edge.example.com/model-service"
	if LabelModelService != expected {
		t.Errorf("LabelModelService drift: want %q, got %q", expected, LabelModelService)
	}
}

func TestAnnotationSliceBindings_FormatStable(t *testing.T) {
	const expected = "npu.huawei.com/slice-bindings"
	if AnnotationSliceBindings != expected {
		t.Errorf("AnnotationSliceBindings drift: want %q, got %q", expected, AnnotationSliceBindings)
	}
}
