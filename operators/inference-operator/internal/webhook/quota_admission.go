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
//
// P13-T-204 (ADR-0025 §2 Decision C): when ClusterCache is non-nil the
// validator ALSO enforces the cluster-wide ClusterQuota cap after the
// namespace check passes — a CREATE must be under BOTH the namespace cap
// and the cluster-wide total cap. ClusterCache nil = namespace-only
// (backward-compatible with the Phase 9 wiring).
type QuotaSliceAllocationValidator struct {
	Cache        *QuotaCache
	ClusterCache *ClusterQuotaCache
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
	// P13-T-204: cluster-wide ClusterQuota total cap (after namespace check).
	if resp := v.checkClusterAllocCap(ctx, lg); resp != nil {
		return *resp
	}
	return admission.Allowed("under namespace + cluster cap")
}

// checkClusterAllocCap enforces the cluster-wide ClusterQuota slice cap.
// Returns nil to continue (no ClusterCache wired · no ClusterQuota set ·
// unbounded cap · or under cap), or a Denied/Allowed pointer to short-circuit.
// Fail-open: transient API errors continue (logged) rather than block CREATE.
func (v *QuotaSliceAllocationValidator) checkClusterAllocCap(ctx context.Context, lg logr.Logger) *admission.Response {
	if v.ClusterCache == nil {
		return nil
	}
	cq, ok, err := v.ClusterCache.Get(ctx)
	if err != nil {
		lg.V(1).Info("ClusterQuota get failed; fail-open", "err", err.Error())
		return nil
	}
	if !ok || cq == nil {
		return nil
	}
	ccap := cq.Spec.Enforcement.MaxSliceAllocations
	if ccap == 0 {
		return nil
	}
	cused := cq.Status.Usage.Total.CurrentSliceAllocations
	if cused+1 > ccap {
		resp := admission.Denied(fmt.Sprintf(
			"cluster at NPUSliceAllocation cap %d; current cluster total %d (ClusterQuota %s)",
			ccap, cused, cq.Name))
		return &resp
	}
	return nil
}

// QuotaScalerValidator enforces Webhook B per ADR-0014 §2 Decision C.
// Intercepts NPUVerticalScaler UPDATE on `spec.scaleSlice.{busy,idle}TemplateName`
// changes. Rejects when scale event count cap exceeded OR template ref not
// in whitelist.
//
// UPDATE that does not change scaleSlice templates is admitted (status
// updates / other spec changes pass through). CREATE / DELETE are NOT
// intercepted by this webhook.
//
// P13-T-204 (ADR-0025 §2 Decision C): when ClusterCache is non-nil the
// validator ALSO enforces the cluster-wide ClusterQuota scale-event rate
// cap after the namespace rate check — a scale event must be under BOTH
// the namespace rate cap and the cluster-wide rate cap. ClusterCache nil =
// namespace-only (backward-compatible).
type QuotaScalerValidator struct {
	Cache        *QuotaCache
	ClusterCache *ClusterQuotaCache
	Decoder      admission.Decoder
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

	// P13-T-204: cluster-wide ClusterQuota scale-event rate cap (after the
	// namespace rate + whitelist checks).
	if resp := v.checkClusterScaleRate(ctx, lg); resp != nil {
		return *resp
	}
	return admission.Allowed("under namespace + cluster cap and templates whitelisted")
}

// checkClusterScaleRate enforces the cluster-wide ClusterQuota scale-event
// rate cap. Same nil/unbounded/fail-open semantics as checkClusterAllocCap.
func (v *QuotaScalerValidator) checkClusterScaleRate(ctx context.Context, lg logr.Logger) *admission.Response {
	if v.ClusterCache == nil {
		return nil
	}
	cq, ok, err := v.ClusterCache.Get(ctx)
	if err != nil {
		lg.V(1).Info("ClusterQuota get failed; fail-open", "err", err.Error())
		return nil
	}
	if !ok || cq == nil {
		return nil
	}
	rateCap := cq.Spec.Enforcement.MaxScaleEventsPerWindow.Count
	if rateCap == 0 {
		return nil
	}
	used := cq.Status.Usage.Total.ScaleEventsInWindow
	if used+1 > rateCap {
		windowSec := cq.Spec.Enforcement.MaxScaleEventsPerWindow.WindowSeconds
		resp := admission.Denied(fmt.Sprintf(
			"cluster scale event rate cap %d / %ds; current cluster total %d (ClusterQuota %s)",
			rateCap, windowSec, used, cq.Name))
		return &resp
	}
	return nil
}

func templateInWhitelist(name string, wl []string) bool {
	for _, w := range wl {
		if w == name {
			return true
		}
	}
	return false
}

// ClusterQuotaCache is an in-memory snapshot of the cluster-scoped
// ClusterQuota object (P13-T-204 · ADR-0025 §2 Decision C). ClusterQuota is
// conventionally a singleton; when multiple exist the first by list order is
// used deterministically (mirrors QuotaCache's per-namespace pick). Read by
// both webhook validators on every cluster-cap decision; refreshed by List
// when the cached entry exceeds TTL. Cluster-scoped → no namespace key.
type ClusterQuotaCache struct {
	Client client.Client
	TTL    time.Duration

	mu    sync.Mutex
	entry *clusterQuotaCacheEntry
}

type clusterQuotaCacheEntry struct {
	quota    *inferencev1alpha1.ClusterQuota
	at       time.Time
	notFound bool
}

// NewClusterQuotaCache constructs a fresh cache backed by `c`. Default TTL
// (DefaultQuotaCacheTTL) applied when ttl is zero.
func NewClusterQuotaCache(c client.Client, ttl time.Duration) *ClusterQuotaCache {
	if ttl <= 0 {
		ttl = DefaultQuotaCacheTTL
	}
	return &ClusterQuotaCache{Client: c, TTL: ttl}
}

// Get returns the active ClusterQuota. Returns (nil, false, nil) when none is
// set (fail-open semantics — cluster cap unbounded). Returns (nil, false, err)
// only on transient API errors.
func (c *ClusterQuotaCache) Get(ctx context.Context) (*inferencev1alpha1.ClusterQuota, bool, error) {
	c.mu.Lock()
	entry := c.entry
	c.mu.Unlock()

	now := time.Now()
	if entry != nil && now.Sub(entry.at) < c.TTL {
		if entry.notFound {
			return nil, false, nil
		}
		return entry.quota, true, nil
	}

	var list inferencev1alpha1.ClusterQuotaList
	if err := c.Client.List(ctx, &list); err != nil {
		return nil, false, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(list.Items) == 0 {
		c.entry = &clusterQuotaCacheEntry{notFound: true, at: now}
		return nil, false, nil
	}
	quota := list.Items[0].DeepCopy()
	c.entry = &clusterQuotaCacheEntry{quota: quota, at: now}
	return quota, true, nil
}

// Invalidate forces the next Get to skip cache. Used from tests.
func (c *ClusterQuotaCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entry = nil
}
