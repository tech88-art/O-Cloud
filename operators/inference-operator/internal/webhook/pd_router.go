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

// Package webhook hosts the inference-operator admission webhooks.
//
// T102 scaffold ships the PD Router boilerplate handler — every
// admission.Request is currently passed through with Allowed. T103
// lands the slice-bindings annotation injection logic per ADR-0008.
package webhook

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// PathPDRouterMutate is the URL path the webhook server serves the
// PD Router mutating handler on. The MutatingWebhookConfiguration
// references this path via `service.path`.
const PathPDRouterMutate = "/mutate-pod"

// LabelModelService matches the label key
// `inference.ocloud.edge.example.com/model-service` set by the
// inference-operator on every Pod created via a ModelService
// Deployment (see internal/controller/deployment_builder.go
// LabelModelService). The MutatingWebhookConfiguration's
// objectSelector matches this same key so most Pods skip the
// handler entirely; this constant is defense-in-depth inside the
// handler.
const LabelModelService = "inference.ocloud.edge.example.com/model-service"

// AnnotationSliceBindings is the annotation key the T103 mutating
// logic stamps onto each Prefill / Decode Pod. Value format (T103):
// comma-separated `<node>/<slice>/<device>:<aiCores>` entries.
const AnnotationSliceBindings = "npu.huawei.com/slice-bindings"

// PDRouter is the admission.Handler that injects the slice-bindings
// annotation onto Pods matching the model-service objectSelector.
//
// T102 (this commit) ships the handler skeleton: Pod decode, label
// presence check, log line. Every request returns Allowed without a
// patch. T103 replaces the early-return-without-patch with the real
// annotation injection logic.
type PDRouter struct {
	// Client is the controller-runtime client used to look up
	// ModelService + NPUSliceAllocation objects at admission time
	// (T103 wires the real lookups).
	Client client.Client

	// Decoder is the admission.Decoder injected by the webhook
	// server. T102 uses it to parse the incoming Pod from the
	// admission.Request.
	Decoder admission.Decoder
}

// Handle implements admission.Handler.
func (h *PDRouter) Handle(ctx context.Context, req admission.Request) admission.Response {
	lg := log.FromContext(ctx).WithName("pd-router").WithValues(
		"task", "P5-T-102",
		"pod-namespace", req.Namespace,
		"pod-name", req.Name,
		"operation", req.Operation,
	)

	pod := &corev1.Pod{}
	if err := h.Decoder.Decode(req, pod); err != nil {
		lg.Error(err, "decode Pod failed; allowing without mutation")
		return admission.Allowed("decode failure tolerated in T102 scaffold")
	}

	msRef, ok := pod.Labels[LabelModelService]
	if !ok || msRef == "" {
		// objectSelector should pre-filter; this branch is
		// defense-in-depth. Allow without patch.
		lg.V(1).Info("Pod missing model-service label; allowing without patch")
		return admission.Allowed("not a ModelService Pod")
	}

	lg.V(1).Info("Pod matched; T102 scaffold returns Allowed without patch — T103 lands real injection",
		"model-service-ref", msRef,
		"pd-role-label", routerLabelValues(pod))

	// T102 contract: always Allowed without patch.
	return admission.Allowed("T102 scaffold (no mutation yet)")
}

// routerLabelValues returns the values of every label that looks like
// a pd-role indicator (`*/pd-role`) — small log helper for the
// scaffold's V(1) line.
func routerLabelValues(pod *corev1.Pod) string {
	for k, v := range pod.Labels {
		if k == "inference.ocloud.edge.example.com/pd-role" {
			return v
		}
	}
	return ""
}

