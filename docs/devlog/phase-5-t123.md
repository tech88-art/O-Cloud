# Phase 5 · T-123 · inference-operator RBAC: resourceclaimtemplates LIST/WATCH

**Date**: 2026-05-20
**Trigger**: kind smoke CI iteration after T122

## Failure observed

T122 bundled the ModelService CRD; CI advanced past `kubectl apply -f
modelservice-sample.yaml`. The next failure surfaced at the assert step
(32s wall-clock — much longer than T122's 1s, meaning the apply
succeeded but the controller never reached steady state):

```
2026-05-20T09:23:04Z ERROR controller-runtime.cache.UnhandledError
  Failed to watch
  {"reflector":   "tools/cache/reflector.go:289",
   "type":       "*v1beta1.ResourceClaimTemplate",
   "error":      "failed to list *v1beta1.ResourceClaimTemplate:
                  resourceclaimtemplates.resource.k8s.io is forbidden:
                  User \"system:serviceaccount:ocloud-system:inference-operator\"
                  cannot list resource \"resourceclaimtemplates\"
                  in API group \"resource.k8s.io\" at the cluster scope"}
```

The backoff loop ran for ~30s before the assert harness gave up.

## Root cause

Same shape as T121 (pool-operator missing RBAC for resourceslices), one
controller over: the inference-operator's hand-written ClusterRole in
`deploy/helm-charts/inference-operator/templates/rbac.yaml` granted
`resourceclaims` and `resourceclaims/status` but not
`resourceclaimtemplates`.

Why controller-runtime needs the watch:
`internal/controller/modelservice_controller.go:273` does
`r.Client.Get(ctx, key, &resourceapi.ResourceClaimTemplate{})`. Under
controller-runtime's cached client, the first Get on a kind triggers
informer setup (LIST → WATCH cycle), and the LIST is what got rejected.
The Reconcile sees the LIST error and the cache never syncs, so
`reconcileResourceClaimTemplate` never gets to Create — the per-side
ResourceClaimTemplate is never created, the Pod template stays
unsatisfied, ModelService.status.phase stays Pending, and the assert
times out.

## Why the marker pipeline didn't catch it

Unlike pool-operator (which uses kubebuilder:rbac markers +
`controller-gen rbac` → `config/rbac/role.yaml`), inference-operator was
scaffolded in Phase 4 W2 (P4-T-103) without an RBAC generation pipeline
— zero kubebuilder:rbac markers, no `config/rbac/`, the helm chart's
rbac.yaml IS the source of truth. So no generator validates code-vs-RBAC
drift here; mistakes are caught only at runtime.

`grep -rn "kubebuilder:rbac" operators/inference-operator/ --include="*.go"`
returns nothing. Decision (consistent with T121 minimal-scope rule):
fix the chart directly; do not retrofit a controller-gen pipeline in
this hot-fix iteration. Phase 6+ chart-vs-code RBAC parity audit task
should add the markers.

## Fix

Add one block to the ClusterRole between the `resourceclaims/status`
rule and the `Deployments` rule:

```yaml
# ResourceClaimTemplate — T007 reconciles one per side via
# reconcileResourceClaimTemplate; controller-runtime sets up a
# cache informer (LIST+WATCH) when the controller does Get/Create/
# Update against this kind. Missing this rule blocks the cache
# sync (T123 CI fix · 2026-05-20).
- apiGroups: ["resource.k8s.io"]
  resources: ["resourceclaimtemplates"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

Same verb set as resourceclaims — the controller does CRUD on the
template (`reconcileResourceClaimTemplate` Creates and Updates; on
ModelService delete the owner reference cascades the delete).

## Verify

- `helm lint deploy/helm-charts/inference-operator` → clean
- `helm template` rendered output confirms the new rule sits between the
  resourceclaims and Deployments stanzas, indented under `rules:` of the
  ClusterRole
- Code audit: only Client operation needing additional RBAC was
  pd_router.go's NPUSliceAllocation List (already covered)

## Expected next-CI flow

Cache informer for ResourceClaimTemplate syncs cleanly → Reconcile
proceeds → per-side ResourceClaimTemplate Created → Pod template
becomes satisfiable → Prefill+Decode Deployments roll out →
ResourceClaim status.allocation populated by npu-dra-driver → phase
machine advances to Bound/Ready → assert step passes.

If a subsequent step fails, it will be downstream of the cache sync
(e.g. webhook cert injection or phase machine).
