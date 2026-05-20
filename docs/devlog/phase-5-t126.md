# Phase 5 · T-126 · kind-smoke Playwright API tests: 2 latent defects

**Date**: 2026-05-20
**Trigger**: kind smoke CI iteration after T125

## Context

T125 unblocked the Phase 5 assert.sh (label selector + bash counting +
webhook race force-recreate). CI advanced PAST the Phase 5 assertion
into the workflow steps that previously never ran because earlier
steps were failing:

```
✓ Phase 5 — apply ModelService + assert PD pair + annotation
↳ install Playwright deps
↳ run kind smoke spec  ← NEW SURFACE: 2 of 3 tests fail
```

Two latent defects in the kind smoke Playwright spec were unmasked:

```
1) clusters list smoke — backend boots and routes are mounted
   Error: clusters list non-OK: 500
   (331ms, fast — handler returning 500 immediately)

2) workloads list — seeded smoke-workload Pod surfaces through k8s.Source
   Error: smoke-workload not surfaced by /api/v1/workloads within 30s
   Received: null
   (27.9s — full timeout polling)

3) exporter-plus emits ascend_npu_utilization_percent > 0
   ✓ passes
```

Both defects predate Phase 5 — they entered the codebase at P3-T-104
(commit 7134098, the original kind-smoke spec). They have been hidden
behind earlier-step failures in every CI run since; T125 was the first
push to let the workflow reach the Playwright step.

## Defect 1: `/api/v1/clusters` 500 — mock-data missing from container

backend/Dockerfile copies into the runtime stage only:

```dockerfile
COPY configs/config.example.yaml /app/configs/config.yaml
```

Nothing else from configs/. The runtime image therefore has no
`/app/configs/mock-data/` directory. The kind-smoke ConfigMap config
points `mapping.clusters: mock` and the mock source's loadClusters
helper does:

```go
path := filepath.Join(s.fixturesPath, "clusters.json")
raw, err := os.ReadFile(path)  // fails: no such file or directory
```

→ handler returns 500.

This is correct production behavior — production images shouldn't ship
test fixtures. The kind smoke must provision them separately.

### Fix

`tests/e2e/kind/install.sh` creates a ConfigMap from the actual
mock-data files immediately before applying demo-backend.yaml:

```bash
kubectl -n "${NS}" create configmap demo-backend-mockdata \
  --from-file="${REPO_ROOT}/configs/mock-data/set-a-small/" \
  --dry-run=client -o yaml | kubectl apply -f -
```

`tests/e2e/kind/manifests/demo-backend.yaml` mounts the ConfigMap at
the path the backend config expects:

```yaml
volumeMounts:
  - name: mockdata
    mountPath: /app/configs/mock-data/set-a-small
    readOnly: true
volumes:
  - name: mockdata
    configMap:
      name: demo-backend-mockdata
```

ConfigMap with --from-file= emits one key per filename (clusters.json,
nodes.json, …); mounted, each key becomes a file under the mount path.
Total fixture size ~116 KB — well under the 1 MB ConfigMap limit.

## Defect 2: `/api/v1/workloads` never sees smoke-workload

`backend/pkg/datasource/k8s/workload.go:51 ListWorkloads` aggregates
**only** Deployments + StatefulSets + Jobs. Bare Pods do not appear.

`tests/e2e/kind/manifests/mock-workload.yaml` was a bare Pod. The file
even had a "Why no Deployment" comment claiming "a single Pod is
enough" — the author didn't realize ListWorkloads filters by workload
controller kind. So the seeded Pod was invisible to /api/v1/workloads
and the test was destined to time out.

### Fix

Convert `mock-workload.yaml` from a bare Pod to a 1-replica Deployment
with the same scheduling constraints (huawei.com/Ascend910 request,
site=site-a nodeSelector, Ascend910B toleration), the same labels
(workload-type, ocloud.edge.example.com/managed-by), and the same
slice-bindings annotation on the Pod template. The Deployment surfaces
through `projectDeployment` in workload.go and the test's
`list.find(w => w.name === 'smoke-workload' && w.namespace ===
'ocloud-system')` matches.

The Status.Replicas-flakiness concern from the original comment
doesn't materialize: pause container has no readiness probe; Available
goes 1 within seconds; the test asserts on workload presence + name +
namespace only, not on replica counts.

## Files touched

- `tests/e2e/kind/install.sh` — add ConfigMap creation before backend apply
- `tests/e2e/kind/manifests/demo-backend.yaml` — add mockdata volume + volumeMount
- `tests/e2e/kind/manifests/mock-workload.yaml` — Pod → Deployment

## Verify

Locally:
- `bash -n tests/e2e/kind/install.sh` → syntax clean
- `python -c "import yaml; list(yaml.safe_load_all(open(...,encoding='utf-8')))"`
  for both manifests → parse clean

CI verification (next run): both failing Playwright tests should
flip to ✓; exporter test continues to pass.

## Why now (not earlier)

The kind-smoke Playwright spec was added at P3-T-104 (2026-05-19) and
has likely never passed in CI — every preceding step had been failing
through T108-T125 (Go toolchain → kind/K8s version → DRA enablement →
CEL syntax → Dockerfile/Makefile → CRD drift → bash bug → label
format → label value → RBAC × 2 → bash bug again × webhook race). T125
was the first commit to let the workflow reach the Playwright step.

The "exporter-plus" test passes because it bypasses the backend
entirely — direct scrape of the exporter's NodePort. So the smoke
spec's 3rd of 3 tests has always been the one most likely to work,
and the other 2 represent buried test-fixture defects.

## Expected next-CI flow

```
✓ Phase 5 — apply ModelService + assert PD pair + annotation
✓ install Playwright deps
✓ run kind smoke spec:
  ✓ clusters list smoke — mock-data ConfigMap mounted, mock.Source reads OK
  ✓ workloads list — Deployment surfaces through ListWorkloads
  ✓ exporter-plus emits ascend_npu_utilization_percent > 0
```

If a subsequent step fails, it will be downstream (e.g. the failure
dump steps or workflow finalisation).
