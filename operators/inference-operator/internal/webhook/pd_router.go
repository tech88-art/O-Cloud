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
// T103 ships the PD Router mutating logic: every Pod admission
// carrying the model-service label is enriched with a
// `npu.huawei.com/slice-bindings` annotation listing the
// NPUSliceAllocation entries currently associated with the
// owning ModelService.
package webhook

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/tech88-art/O-Cloud/operators/inference-operator/internal/metrics"
)

// recordDecisionMetric inspects the admission.Response and records the
// outcome into the WebhookDecisions counter. Called via deferred wrap in
// Handle below so every return path observes exactly once.
func recordDecisionMetric(resp admission.Response) {
	switch {
	case !resp.Allowed:
		metrics.RecordWebhookDecision(metrics.WebhookDecisionDenied)
	case len(resp.Patches) > 0 || resp.PatchType != nil:
		metrics.RecordWebhookDecision(metrics.WebhookDecisionPatched)
	default:
		metrics.RecordWebhookDecision(metrics.WebhookDecisionAllowedNoPatch)
	}
}

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
// logic stamps onto each Prefill / Decode Pod. Value format:
// comma-separated `<node>/<pool>/<device>:<aiCores>` entries.
const AnnotationSliceBindings = "npu.huawei.com/slice-bindings"

// AnnotationPDEndpoint is the annotation key P13-T-105 stamps onto each
// PD-pair Pod carrying the REAL prefill→decode KV-cache transfer
// endpoint (ADR-0024 §2 Decision F · ADR-0008 PD Router contract). The
// vllm-ascend disaggregated_prefill_v1 launcher reads it to learn the
// sibling side's host:port so prefilled KV-cache transfers to the
// decode replicas.
//
// Value = `<peer-service>:<kvPort>` where peer-service follows the
// inference-operator per-side Service convention `<ms.Name>-<otherSide>`
// (matching deployment_builder.go's VLLM_PD_PREFILL_HOST/DECODE_HOST
// sidecar env). A prefill Pod points at the decode Service and vice
// versa.
//
// This is ADDITIVE — the npu.huawei.com/slice-bindings annotation
// (ADR-0008 contract) key/shape is UNCHANGED; T105 only adds the real
// PD endpoint alongside it on the same admission patch.
const AnnotationPDEndpoint = "inference.ocloud.edge.example.com/pd-endpoint"

// RoleLabelKey is the pod label key carrying the PD role
// (prefill/decode). Mirrors deployment_builder.go DefaultRouterLabelKey;
// the webhook reads it to compute the sibling endpoint. Text-duplicated
// per operators/CLAUDE.md §1 (no cross-package Go import for a const).
const RoleLabelKey = "inference.ocloud.edge.example.com/pd-role"

// pdKVCachePort is the KV-cache transfer port the vllm-ascend
// disaggregated prefill server listens on for the sibling side. Matches
// the VLLM_PD_SIBLING_LOCALHOST_PORT convention in deployment_builder.go
// (proxy reaches local vllm-ascend on 8000); the cross-Pod KV-cache
// transfer uses the same serving port via the per-side Service.
const pdKVCachePort = "8000"

// npuSliceAllocationListGVK is the GVK for cluster-wide List of the
// NPUSliceAllocation audit objects. Cross-module Go imports are
// forbidden (operators/CLAUDE.md §1), so the webhook reads via
// unstructured client.
var npuSliceAllocationListGVK = schema.GroupVersionKind{
	Group:   "npu.ocloud.edge.example.com",
	Version: "v1alpha1",
	Kind:    "NPUSliceAllocationList",
}

// DenyOnOrphaned controls whether the handler returns Denied when
// every NPUSliceAllocation for the ModelService reports phase=Orphaned.
// Default true honors ADR-0008 "Fail-closed default"; operators
// override via values.yaml when phasing the webhook in.
type PDRouter struct {
	Client  client.Client
	Decoder admission.Decoder

	// DenyOnOrphaned: when true (default), an all-Orphaned binding
	// set produces admission.Denied with reason NoAvailableNPUSlices.
	// When false, the handler logs + returns Allowed without a patch.
	DenyOnOrphaned bool
}

// Handle implements admission.Handler.
//
// Decision matrix (per plan T103 acceptance):
//
//   - Pod missing model-service label: Allowed (defense-in-depth;
//     objectSelector usually pre-filters).
//   - Lookup error (list NPUSliceAllocation fails): Allowed
//     without patch (best-effort enrichment per plan; the webhook
//     is enrichment, not blocking).
//   - All claims allocated: Patched with annotation.
//   - One or more claims pending: Allowed without patch (Pod will
//     be re-evaluated when claims allocate; eventual-consistency).
//   - All claims Orphaned: Denied with NoAvailableNPUSlices when
//     DenyOnOrphaned (default). Otherwise Allowed without patch.
//   - No allocations yet observed at all (T007 templates created
//     but K8s hasn't expanded to claims yet, or npu-dra-driver
//     hasn't allocated yet): Allowed without patch.
func (h *PDRouter) Handle(ctx context.Context, req admission.Request) admission.Response {
	lg := log.FromContext(ctx).WithName("pd-router").WithValues(
		"task", "P5-T-103",
		"pod-namespace", req.Namespace,
		"pod-name", req.Name,
		"operation", req.Operation,
	)
	// P6-T-104: record webhook decision metric on EVERY return path
	// (allowed_no_patch / patched / denied).
	var resp admission.Response
	defer func() { recordDecisionMetric(resp) }()
	resp = h.handle(ctx, lg, req)
	return resp
}

// handle is the inner Handle body — exists so we can wrap the entire
// flow with the decision-metric defer above without complicating
// every return statement.
func (h *PDRouter) handle(ctx context.Context, lg logr.Logger, req admission.Request) admission.Response {

	pod := &corev1.Pod{}
	if err := h.Decoder.Decode(req, pod); err != nil {
		lg.Error(err, "decode Pod failed; allowing without mutation")
		return admission.Allowed("decode failure tolerated")
	}

	// Label value carries just ms.Name (no namespace) because K8s
	// label-value regex rejects `/`. Reconstruct the qualified
	// `<ns>/<name>` form to match NPUSliceAllocation.spec.modelServiceRef
	// (which the npu-dra-driver claim controller copies from the
	// ResourceClaim annotation, where `/` is permitted). T124 fix.
	nameVal, ok := pod.Labels[LabelModelService]
	if !ok || nameVal == "" {
		lg.V(1).Info("Pod missing model-service label; allowing without patch")
		return admission.Allowed("not a ModelService Pod")
	}
	msRef := req.Namespace + "/" + nameVal

	bindings, err := h.listBindings(ctx, msRef)
	if err != nil {
		lg.Error(err, "list NPUSliceAllocation failed; allowing without patch (best-effort enrichment)",
			"model-service-ref", msRef)
		return admission.Allowed("allocation lookup failure tolerated")
	}

	allocated, orphaned, other := CountByPhase(bindings)
	lg.V(1).Info("Bindings counted",
		"model-service-ref", msRef,
		"total", len(bindings),
		"allocated", allocated,
		"orphaned", orphaned,
		"other", other,
	)

	// All-Orphaned → fail-closed Denied per ADR-0008.
	if len(bindings) > 0 && allocated == 0 && other == 0 && orphaned == len(bindings) && h.DenyOnOrphaned {
		return admission.Denied("NoAvailableNPUSlices: all NPUSliceAllocations for ModelService are Orphaned")
	}

	// Pending claims → best-effort skip until next reconcile.
	if allocated < len(bindings) {
		lg.V(1).Info("Some bindings not yet allocated; allowing without patch",
			"allocated", allocated, "total", len(bindings))
		return admission.Allowed("waiting for full allocation")
	}

	// Nothing to inject (no allocations yet → first Pod admission
	// races with claim allocation; downstream watch event will
	// re-fire).
	if len(bindings) == 0 {
		lg.V(1).Info("No NPUSliceAllocation entries observed for ModelService; allowing without patch",
			"model-service-ref", msRef)
		return admission.Allowed("no slice bindings yet")
	}

	// All allocated → inject annotation(s).
	allocatedBindings := FilterAllocated(bindings)
	value := EncodeBindings(allocatedBindings)
	patchedPod := pod.DeepCopy()
	if patchedPod.Annotations == nil {
		patchedPod.Annotations = make(map[string]string)
	}
	// ADR-0008 contract annotation (key/shape UNCHANGED).
	patchedPod.Annotations[AnnotationSliceBindings] = value
	// P13-T-105: ADDITIVE real PD endpoint (prefill→decode KV-cache
	// transfer URL). Derived from the Pod's pd-role label + the
	// ModelService name (sibling Service convention). Only stamped when
	// the role label is present AND a sibling endpoint resolves — a
	// single-pod / role-less ModelService gets slice-bindings but no
	// PD endpoint (no sibling to transfer to).
	if ep := pdEndpointFor(nameVal, pod.Labels[RoleLabelKey]); ep != "" {
		patchedPod.Annotations[AnnotationPDEndpoint] = ep
		lg.V(1).Info("PD endpoint injected",
			"model-service-ref", msRef,
			"pd-role", pod.Labels[RoleLabelKey],
			"endpoint", ep)
	}

	marshalled, err := json.Marshal(patchedPod)
	if err != nil {
		lg.Error(err, "marshal patched Pod failed; allowing without patch")
		return admission.Allowed("marshal failure tolerated")
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshalled)
}

// listBindings lists every NPUSliceAllocation cluster-wide via
// unstructured client, filters down to entries whose
// Spec.modelServiceRef matches msRef, and returns them as
// SliceBinding rows.
//
// Phase 5 design: cluster-wide list is fine — audit count is
// O(claims) which is O(replicas) which is small. Phase 6 may
// switch to an indexed field selector once K8s supports them
// for custom resources.
func (h *PDRouter) listBindings(ctx context.Context, msRef string) ([]SliceBinding, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(npuSliceAllocationListGVK)
	if err := h.Client.List(ctx, list); err != nil {
		if apierrors.IsNotFound(err) {
			// CRD not yet registered (cluster freshly bootstrapped
			// without npu-dra-driver chart applied). Treat as zero
			// bindings.
			return nil, nil
		}
		return nil, fmt.Errorf("list NPUSliceAllocation: %w", err)
	}

	out := make([]SliceBinding, 0, len(list.Items))
	for i := range list.Items {
		a := &list.Items[i]
		ref, found, _ := unstructured.NestedString(a.Object, "spec", "modelServiceRef")
		if !found || ref != msRef {
			continue
		}
		nodeName, _, _ := unstructured.NestedString(a.Object, "spec", "nodeName")
		pool, _, _ := unstructured.NestedString(a.Object, "spec", "sliceRef", "pool")
		device, _, _ := unstructured.NestedString(a.Object, "spec", "sliceRef", "device")
		aiCores, _, _ := unstructured.NestedInt64(a.Object, "spec", "aiCores")
		phase, _, _ := unstructured.NestedString(a.Object, "status", "phase")
		out = append(out, SliceBinding{
			Node:    nodeName,
			Pool:    pool,
			Device:  device,
			AICores: int32(aiCores),
			Phase:   phase,
		})
	}
	return out, nil
}

// pdEndpointFor returns the sibling-side KV-cache transfer endpoint for
// a PD-pair Pod, or "" when the role is unknown / not a PD side.
//
// Convention (matches deployment_builder.go VLLM_PD_PREFILL_HOST /
// VLLM_PD_DECODE_HOST sidecar env): each side reaches the OTHER side's
// per-side Service `<msName>-<otherSide>` on the KV-cache port. A
// prefill Pod transfers its KV-cache TO the decode Service; a decode
// Pod's endpoint points back at prefill (symmetric so either side can
// dial the other).
//
//	role     → endpoint
//	prefill  → <msName>-decode:8000
//	decode   → <msName>-prefill:8000
//	other/"" → "" (no PD endpoint stamped)
func pdEndpointFor(msName, role string) string {
	if msName == "" {
		return ""
	}
	switch role {
	case "prefill":
		return msName + "-decode:" + pdKVCachePort
	case "decode":
		return msName + "-prefill:" + pdKVCachePort
	default:
		return ""
	}
}
