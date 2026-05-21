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
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	inferencev1alpha1 "github.com/tech88-art/O-Cloud/operators/inference-operator/api/v1alpha1"
)

// Webhook paths registered on the inference-operator webhook server (port 9443).
// MUST match the helm chart ValidatingWebhookConfiguration `clientConfig.service.path`.
const (
	PathQuotaValidateNPUSliceAllocation = "/validate-npu-ocloud-edge-example-com-v1alpha1-npusliceallocation-quota"
	PathQuotaValidateNPUVerticalScaler  = "/validate-inference-ocloud-edge-example-com-v1alpha1-npuverticalscaler-quota"
)

// DefaultQuotaCacheTTL is the in-memory snapshot freshness window the
// admission handlers honour before falling back to a strict API Get.
// Per ADR-0014 §2 Decision D.
const DefaultQuotaCacheTTL = 5 * time.Second

// QuotaCache is an in-memory snapshot of Quota objects keyed by namespace.
// Read by the two webhook handlers on every decision; refreshed by Get
// when the cached entry exceeds TTL. The cache is intentionally minimal —
// no informer is registered to keep the webhook fail-open path simple.
type QuotaCache struct {
	Client client.Client
	TTL    time.Duration

	mu      sync.Mutex
	entries map[string]quotaCacheEntry
}

type quotaCacheEntry struct {
	quota *inferencev1alpha1.Quota
	at    time.Time
	// notFound captures the "no Quota set in namespace" state so we don't
	// re-Get on every webhook invocation in the unbounded path.
	notFound bool
}

// NewQuotaCache constructs a fresh cache backed by `c`. Default TTL applied
// when ttl is zero.
func NewQuotaCache(c client.Client, ttl time.Duration) *QuotaCache {
	if ttl <= 0 {
		ttl = DefaultQuotaCacheTTL
	}
	return &QuotaCache{
		Client:  c,
		TTL:     ttl,
		entries: make(map[string]quotaCacheEntry),
	}
}

// Get returns the Quota for the namespace. Returns (nil, false, nil) when
// no Quota set in the namespace (fail-open semantics). Returns (nil, false,
// err) only on transient API errors.
func (q *QuotaCache) Get(ctx context.Context, namespace string) (*inferencev1alpha1.Quota, bool, error) {
	q.mu.Lock()
	entry, ok := q.entries[namespace]
	q.mu.Unlock()

	now := time.Now()
	if ok && now.Sub(entry.at) < q.TTL {
		if entry.notFound {
			return nil, false, nil
		}
		return entry.quota, true, nil
	}

	// Cache miss / stale — Get from API. Assume conventional 1-Quota-per-namespace
	// (multi-Quota per namespace is an ADR-0014 §3 risk row · AND semantics
	// not implemented this Phase 9 — Phase 10 polish if needed).
	var list inferencev1alpha1.QuotaList
	if err := q.Client.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return nil, false, err
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	if len(list.Items) == 0 {
		q.entries[namespace] = quotaCacheEntry{notFound: true, at: now}
		return nil, false, nil
	}
	// Pick first by sort order to ensure determinism if multiple exist.
	quota := list.Items[0].DeepCopy()
	q.entries[namespace] = quotaCacheEntry{quota: quota, at: now}
	return quota, true, nil
}

// Invalidate forces the next Get for the namespace to skip cache. Used
// from tests; production controllers don't need this (TTL handles it).
func (q *QuotaCache) Invalidate(namespace string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.entries, namespace)
}

// QuotaSliceAllocationValidator enforces Webhook A per ADR-0014 §2 Decision C.
// Intercepts NPUSliceAllocation CREATE in the requesting namespace and rejects
// when `quota.status.usage.currentSliceAllocations + 1 > quota.spec.enforcement.maxSliceAllocations`.
//
// MaxSliceAllocations=0 means unbounded — admit. No-quota-in-namespace
// means unbounded — admit (fail-open semantics).
type QuotaSliceAllocationValidator struct {
	Cache *QuotaCache
}

func (v *QuotaSliceAllocationValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	lg := log.FromContext(ctx).WithName("quota-webhook-A").WithValues(
		"namespace", req.Namespace, "name", req.Name, "operation", req.Operation,
	)

	resp := v.handle(ctx, req, lg)
	defer recordDecisionMetric(resp)
	return resp
}

func (v *QuotaSliceAllocationValidator) handle(ctx context.Context, req admission.Request, lg logr.Logger) admission.Response {
	if req.Operation != admissionv1.Create {
		return admission.Allowed("not a CREATE; pass-through")
	}
	quota, ok, err := v.Cache.Get(ctx, req.Namespace)
	if err != nil {
		// fail-open per ADR-0014 §2 Decision C step 2 — admit and let
		// controller catch up on its 60s tick. NotFound is already
		// translated by Cache.Get to (nil, false, nil).
		if !apierrors.IsNotFound(err) {
			return admission.Errored(http.StatusInternalServerError, fmt.Errorf("get Quota: %w", err))
		}
	}
	if !ok || quota == nil {
		return admission.Allowed("no Quota set in namespace; unbounded")
	}
	cap := quota.Spec.Enforcement.MaxSliceAllocations
	if cap == 0 {
		return admission.Allowed("Quota.spec.enforcement.maxSliceAllocations=0; unbounded")
	}
	used := quota.Status.Usage.CurrentSliceAllocations
	if used+1 > cap {
		msg := fmt.Sprintf("namespace %s at NPUSliceAllocation cap %d; current %d", req.Namespace, cap, used)
		return admission.Denied(msg)
	}
	return admission.Allowed("under cap")
}

// QuotaScalerValidator enforces Webhook B per ADR-0014 §2 Decision C.
// Intercepts NPUVerticalScaler UPDATE on `spec.scaleSlice.{busy,idle}TemplateName`
// changes. Rejects when scale event count cap exceeded OR template ref not
// in whitelist.
//
// UPDATE that does not change scaleSlice templates is admitted (status
// updates / other spec changes pass through). CREATE / DELETE are NOT
// intercepted by this webhook.
type QuotaScalerValidator struct {
	Cache   *QuotaCache
	Decoder admission.Decoder
}

func (v *QuotaScalerValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	lg := log.FromContext(ctx).WithName("quota-webhook-B").WithValues(
		"namespace", req.Namespace, "name", req.Name, "operation", req.Operation,
	)
	resp := v.handle(ctx, req, lg)
	defer recordDecisionMetric(resp)
	return resp
}

func (v *QuotaScalerValidator) handle(ctx context.Context, req admission.Request, lg logr.Logger) admission.Response {
	if req.Operation != admissionv1.Update {
		return admission.Allowed("not an UPDATE; pass-through")
	}
	var newScaler, oldScaler inferencev1alpha1.NPUVerticalScaler
	if err := v.Decoder.Decode(req, &newScaler); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("decode new NPUVerticalScaler: %w", err))
	}
	if err := v.Decoder.DecodeRaw(req.OldObject, &oldScaler); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("decode old NPUVerticalScaler: %w", err))
	}

	// Only intercept actual scaleSlice template changes — status updates etc.
	// pass through unaffected.
	if newScaler.Spec.ScaleSlice.BusyTemplateName == oldScaler.Spec.ScaleSlice.BusyTemplateName &&
		newScaler.Spec.ScaleSlice.IdleTemplateName == oldScaler.Spec.ScaleSlice.IdleTemplateName {
		return admission.Allowed("scaleSlice template refs unchanged; not a scale event")
	}

	quota, ok, err := v.Cache.Get(ctx, req.Namespace)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return admission.Errored(http.StatusInternalServerError, fmt.Errorf("get Quota: %w", err))
		}
	}
	if !ok || quota == nil {
		return admission.Allowed("no Quota set in namespace; unbounded")
	}

	// Rate cap check: scaleEventsInWindow + 1 ≤ count.
	rateCap := quota.Spec.Enforcement.MaxScaleEventsPerWindow.Count
	if rateCap > 0 {
		used := quota.Status.Usage.ScaleEventsInWindow
		if used+1 > rateCap {
			windowSec := quota.Spec.Enforcement.MaxScaleEventsPerWindow.WindowSeconds
			msg := fmt.Sprintf("namespace %s scale event rate cap %d / %ds; current %d", req.Namespace, rateCap, windowSec, used)
			return admission.Denied(msg)
		}
	}

	// Template whitelist check: when non-empty, both new busy + idle template
	// names must appear in the whitelist.
	if wl := quota.Spec.Enforcement.MaxNPUSliceTemplateRefs; len(wl) > 0 {
		if !templateInWhitelist(newScaler.Spec.ScaleSlice.BusyTemplateName, wl) {
			return admission.Denied(fmt.Sprintf("NPUSliceTemplate %q not in namespace whitelist", newScaler.Spec.ScaleSlice.BusyTemplateName))
		}
		if !templateInWhitelist(newScaler.Spec.ScaleSlice.IdleTemplateName, wl) {
			return admission.Denied(fmt.Sprintf("NPUSliceTemplate %q not in namespace whitelist", newScaler.Spec.ScaleSlice.IdleTemplateName))
		}
	}

	return admission.Allowed("under cap and templates whitelisted")
}

func templateInWhitelist(name string, wl []string) bool {
	for _, w := range wl {
		if w == name {
			return true
		}
	}
	return false
}

