# P13-T-204 · [A3] ClusterQuota webhook B real enforcement + cross-cluster usage

- **Commit**: this commit (main agent)
- **Date**: 2026-06-03
- **Duration**: plan 2.5d vs actual ~1d.

## Intent

Close build-doc §5 item "多租户配额强制" per ADR-0025 §2 Decision C: make the cluster-scoped
`ClusterQuota` (schema P11-T-104) actually enforced. Before T204 it was a typed-but-dead CRD —
grep confirmed **0 ClusterQuota reads in inference-operator/internal/** (also flagged in the T202
devlog). T204 wires the three consumers: admission webhooks (cluster cap), the scaler (cluster
scale-rate gate), and a new controller that populates `status.usage` (PerCluster → RecomputeTotal).

## Design

- **Both webhooks enforce the cluster cap, not just "webhook B".** The plan text said "webhook B reads
  ClusterQuota.status.usage.Total"; the *acceptance* says "over-cap **NPUSliceAllocation** → admission
  拒". NPUSliceAllocation CREATE is Webhook A's domain, the scaler rate is Webhook B's — so the correct
  complete design adds a cluster check to **both** (P4 horizontal): A enforces the cluster-wide slice
  total cap, B enforces the cluster-wide scale-event rate cap. A CREATE/scale must be under **both** the
  namespace cap and the cluster cap.
- **`ClusterQuotaCache`** (webhook pkg) mirrors the existing `QuotaCache` but for the cluster-scoped
  singleton (List → first by order · 5s TTL · fail-open). Both validators gained a nil-safe
  `ClusterCache` field → nil = namespace-only (Phase 9 backward-compatible; existing webhook tests
  untouched + still green).
- **`clusterquota_controller.go`** mirrors `quota_controller.go` on a 60s tick but cluster-wide:
  lists NPUSliceAllocation (unstructured · all namespaces) + NPUVerticalScaler scaleHistory-in-window,
  **buckets each by source cluster**, sets `status.usage.PerCluster`, then `RecomputeTotal()` sums →
  `Total`. Source cluster = the Karmada `resource.karmada.io/cached-from-cluster-name` annotation
  (present on objects lifted by the karmada-aggregated-apiserver · ADR-0018 §2 Decision D) else the
  local cluster name (`KARMADA_CLUSTER_NAME` env · default `host`). One code path serves BOTH
  single-cluster (everything → `host`) and aggregated mode (per-member buckets) — testable without a
  live Karmada.
- **Scaler cluster-rate gate** (Step 6.5): a nil-safe `ClusterScaleCap func` field. When the cluster
  scale-rate cap is reached the scale is **deferred** (no annotation patch · no ScaleEvent · distinct
  `ClusterQuotaScaleRateExceeded` reason · requeue) even if the per-scaler cooldown + metric say "scale
  now". Defence-in-depth; the webhook + controller-tick remain authoritative. nil → no gate (Phase 8
  backward-compatible).

## Path adaptations (transparent · P3)

- **`cmd/main.go` is NOT in T204's literal Allowed Paths**, but an unregistered controller/cache is dead
  code — the feature requires it. So main.go gains: construct one shared `ClusterQuotaCache`; register
  `ClusterQuotaReconciler`; wire `ClusterCache` into both webhook validators; set the scaler's
  `ClusterScaleCap` closure off the shared cache. This is minimal same-module wiring (inference-operator
  is T204's module); documented here rather than silently bundled — mirrors how T202 handled its
  out-of-path drift.
- **No `config/rbac/` exists in this project** (the plan listed `config/rbac/*.yaml`). RBAC is managed
  in the helm chart, and **T202 already forward-provisioned** the `clusterquotas` get/list/watch/...
  + /status + /finalizers grant there (verified: chart `templates/rbac.yaml` lines 55-61). `npusliceallocations` + `npuverticalscalers` were already granted (the namespace QuotaReconciler lists
  them). So the T204 RBAC surface is satisfied with no new file — confirmed by grep, not assumed.

## Verification (offline · plan §8 layer 1 · all fake-client, no envtest needed)

- `go build ./...` + `go vet ./internal/... ./cmd/...` clean.
- `CGO_ENABLED=0 GOARCH=arm64 go build ./...` clean (ADR-0020).
- New tests (8) all PASS + full `controller` + `webhook` packages green (no regression to the Phase 9
  namespace-quota / Phase 8 scaler tests):
  - webhook: `ClusterCapReject` (ns-under + cluster-at → deny · the §5 acceptance) · `ClusterCapAllow` ·
    `NilClusterCacheBackcompat` · `ClusterScaleRateReject` · `ClusterQuotaCache_GetAndNotFound`.
  - controller: `ClusterQuotaReconcileEmptyZero` · `CrossClusterSum` (member1=2+member2=3+host=1 →
    Total=6 · RecomputeTotal) · `ScaleEventsBucketed` (in-window per cluster).
  - scaler: `ClusterCapDefers` (over-cap → annotation unchanged · no ScaleEvent · reason set) ·
    `ClusterCapUnderProceeds`.

**Real-machine (LAB-GATED · T301)**: live cross-cluster PerCluster population through a real
karmada-aggregated-apiserver (member usage lifted with the cached-from-cluster annotation) is the T301
integration stamp; the offline layer proves the bucketing + RecomputeTotal + admission/scaler gates with
synthesised objects.

## Carry-forward

- **T205**: Karmada HA control-plane is the substrate that makes the aggregated-apiserver path real
  (the controller already groups by the cached-from-cluster annotation, so no code change needed when
  T205 lands — only the operator's client target / `KARMADA_CLUSTER_NAME` env).
- **T301**: apply a ClusterQuota in the lab + create over-cap NPUSliceAllocations → confirm admission
  deny + PerCluster sum across members.
