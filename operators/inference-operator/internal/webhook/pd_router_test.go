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
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sjson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes/scheme"
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

func TestHandle_ScaffoldAllowsAll(t *testing.T) {
	dec := newDecoder(t)
	h := &PDRouter{Decoder: dec}

	pod := newPod("p1", "ns", map[string]string{
		LabelModelService: "ns/ms-1",
	})
	req := podAdmissionRequest(t, pod)
	resp := h.Handle(context.Background(), req)

	if !resp.Allowed {
		t.Errorf("T102 scaffold should always Allow; got Denied with %+v", resp)
	}
	if len(resp.Patches) != 0 {
		t.Errorf("T102 scaffold should NOT patch; got %d patch ops", len(resp.Patches))
	}
}

func TestHandle_NonMSPod_Allowed(t *testing.T) {
	dec := newDecoder(t)
	h := &PDRouter{Decoder: dec}

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
	// Decode failure → handler returns Allowed (scaffold posture).
	// T103 may revisit to Deny on bad payload (security stance).
	dec := newDecoder(t)
	h := &PDRouter{Decoder: dec}

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
	// Allowed even on bad payload (best-effort scaffold).
	if !resp.Allowed {
		t.Errorf("scaffold should Allow on bad payload; got %+v", resp)
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
