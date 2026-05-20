# Phase 5 · T-127 · demo-backend RBAC: apps/Deployments + batch/Jobs

**Date**: 2026-05-20
**Trigger**: kind smoke CI iteration after T126

## Progress

T126 fixed defect 1 (mock-data ConfigMap mount → `/api/v1/clusters`
passes) and converted defect 2's bare Pod fixture to a Deployment.
Latest CI:

```
✓ 1 clusters list smoke — backend boots and routes are mounted (342ms)
✘ 2 workloads list — seeded smoke-workload Pod surfaces ... (29.6s timeout)
✘ 3 workloads list — retry #1 (28.0s)
✓ 4 exporter-plus emits ascend_npu_utilization_percent > 0 (339ms)
```

Workloads test still fails. T126's Deployment conversion was necessary
but not sufficient — there's a second layer of defect.

## Root cause

`tests/e2e/kind/manifests/demo-backend.yaml` ClusterRole grants:

```yaml
rules:
  - apiGroups: [""]
    resources: ["nodes", "pods", "services", "namespaces", "events"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["ims.ocloud.edge.example.com"]
    resources: [clusterpools, nodepools, npupools, npuslicepools]
    verbs: ["get", "list", "watch"]
```

But `backend/pkg/datasource/k8s/workload.go:51 ListWorkloads` calls:

```go
deployments, _ := s.client.AppsV1().Deployments(ns).List(ctx, ...)
statefulSets, _ := s.client.AppsV1().StatefulSets(ns).List(ctx, ...)
jobs, _ := s.client.BatchV1().Jobs(ns).List(ctx, ...)
```

Three Lists across two unmapped API groups: `apps/v1` and `batch/v1`.
The first List (Deployments) hits an apiserver `forbidden` error, the
handler hits the error path:

```go
deployments, err := s.client.AppsV1().Deployments(ns).List(ctx, ...)
if err != nil {
    return nil, errFromAPIServer("list deployments", err)
}
```

→ handler logs ListWorkloads failed → 500. The test's
`if (!resp.ok()) return null;` lets the poll spin until timeout.

This is a sibling oversight to T126:
- T126 fixed the *fixture* (bare Pod was invisible to ListWorkloads
  regardless of RBAC — the kind filter dropped it).
- T127 fixes the *RBAC* (now a Deployment exists, but the handler
  couldn't even reach the projection step).

Both were buried under earlier CI failures since P3-T-104 added the
spec; neither was ever exercised through.

## Fix

Add 2 RBAC rules to the `demo-backend` ClusterRole — mirror the same
`get/list/watch` verbs the existing rules use:

```yaml
- apiGroups: ["apps"]
  resources: ["deployments", "statefulsets"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["batch"]
  resources: ["jobs"]
  verbs: ["get", "list", "watch"]
```

These exactly cover the three List calls in ListWorkloads. No verbs
beyond what the handler needs (it never Creates/Patches/Deletes).

## Why these are the right groups + resources

Per the Kubernetes API hierarchy (https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/):
- `Deployment` lives in `apps/v1`
- `StatefulSet` lives in `apps/v1`
- `Job` lives in `batch/v1`

`s.client.AppsV1()` is the controller-runtime alias for the apps/v1
group; `s.client.BatchV1()` is batch/v1. The clientset go-typed
calls translate directly to these `apiGroups:` values.

## Verify

Locally:
- `python -c "import yaml; list(yaml.safe_load_all(...))"` →
  demo-backend.yaml still parses cleanly after the edit.
- 4-document YAML structure unchanged (ConfigMap / ServiceAccount /
  ClusterRole / ClusterRoleBinding / Deployment / Service).

## Expected next-CI flow

```
✓ clusters list smoke — mock.Source via ConfigMap (T126)
✓ workloads list — k8s.Source can now List Deployments;
   smoke-workload Deployment surfaces → list.find(...) hits;
   poll returns the workload object → toBeTruthy passes
✓ exporter-plus emits ascend_npu_utilization_percent > 0
```

If another failure surfaces, it will be downstream of the workloads
test (no more kind-smoke spec tests after this point).

## Notes on completeness audit

Re-walked the demo-backend RBAC against every datasource the backend
might list:
- mock.Source: filesystem only, no apiserver calls — no RBAC needed.
- k8s.Source ListNodes / GetNodeDetail → `nodes` (core) ✓ granted.
- k8s.Source ListWorkloads → apps/Deployments + apps/StatefulSets +
  batch/Jobs ✗ MISSING (this fix).
- k8s.Source ListNPUs → derived from node labels + Pods (no extra
  groups).
- crd.Source ListNPUSlicePools etc. → ims.ocloud.edge.example.com ✓
  granted.
- crd.Source has no other CRDs in scope at Phase 5.
- prometheus.Source → external HTTP (no RBAC needed).
- configmap.Source → not currently mapped in demo-backend-config
  (Phase 2 future work).

After T127, the demo-backend SA has exactly the RBAC it needs for the
configured mapping. No over-grants.
