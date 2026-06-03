# P13-T-104 · [B4] backend 真 Deploy() / DeleteDeploy()

- **Commit**: <pending — main agent commits this task>
- **Date**: 2026-06-03
- **Duration**: plan 2d vs actual ~0.4d (offline layer only; real-machine stamp lab-gated)

## Intent

Replace the backend k8s source's `Deploy()` / `DeleteDeploy()` `ErrCapabilityUnavailable`
stubs with real bodies that apply / delete a `apps/v1.Deployment` + `core/v1.Service`
via the typed client-go clientset, then flip the real profile's `mapping.deploy`
from `mock` to `k8s`. Below the `datasource.Source` seam (ADR-0024 §2 Decision E/G):
the HTTP handler (`pkg/api/deploy.go`) is untouched and does not branch on profile —
it calls `Deploy` on whatever `mapping.deploy` selected. demo profile (mock deploy)
regression unaffected.

## Path adaptations

- **Separate `deploy.go` (not in source.go)**: plan §4 T104 lists `k8s/source.go` AND
  `k8s/*.go`; I put the bodies in a new `backend/pkg/datasource/k8s/deploy.go` and left
  only a pointer comment in source.go — mirrors how T103 split `topology.go` and keeps
  source.go lean. Within the Allowed `k8s/*.go` glob.
- **No preset catalog on the k8s source**: the mock `Deploy` resolves image / npuCount
  from `ListPresets`, but the k8s source's `ListPresets` is `ErrCapabilityUnavailable`
  (presets are served by the *configmap* source). So the k8s `Deploy` reads the image
  and per-replica NPU count from `req.Parameters["image"]` / `["npuCount"]` (optional,
  defaulted) — this keeps the wire `DeployRequest` schema unchanged (no api-contract.yaml
  edit) while giving a real Deployment the one or two knobs it needs.
- **`Capabilities().Deploy` left `false`**: the field is *not* consumed by factory/
  registry wiring (grep `\.Capabilities\(\)` across `pkg/` → no call sites; the registry
  routes purely by config `mapping`). T103 set the identical precedent — it implemented
  `GetTopology` but left `Capabilities().Topology = false`. To stay consistent and
  surgical I did NOT flip the literal; the `caps.Deploy == false` assertion in
  `source_test.go` therefore still holds unchanged.

## Debugging trail

- First `go test ./pkg/datasource/k8s/...` failed on the pre-existing
  `TestStubMethods_ReturnErrCapabilityUnavailable/{Deploy,DeleteDeploy}` — same stale-test
  situation T103 hit for GetTopology. Removed the two entries from that table's stub list
  and added a P13-T-104 NOTE mirroring the existing GetTopology NOTE. Same-module,
  necessary (the assertion was now false-by-construction). No other test referenced the
  Deploy stub.
- `model` import in source_test.go stayed live (still used by the QueryMetric stub's
  `model.TimeRange{}`), so no import churn.

## Key decisions

- **Ownership / delete-handle convention** (the load-bearing Deploy↔DeleteDeploy contract):
  every created object carries `app.kubernetes.io/managed-by=demo-backend` +
  `deploy.ocloud.io/id=<deployID>` (+ `app.kubernetes.io/name=<workloadName>`). Both
  labels go on the Deployment, the Service, AND the pod template, so (a) the Service
  selector resolves the pods and (b) `DeleteDeploy` does a single all-namespaces
  label-selector list (`deploy.ocloud.io/id=<id>`) + delete — no in-memory index needed
  (the apiserver is the source of truth; backend has no DB per root §5). Mirrors the mock
  source's label pair, differing only in the managed-by VALUE (`demo-backend` vs
  `mock-deploy`) so a cluster that ran both never cross-deletes.
- **deployID = sanitized workloadName**: a name unique within the namespace doubles as a
  reconstructible delete handle. `req.Name` is sanitized to a DNS-1123 label; absent →
  `<presetId>-<unixnano>`.
- **Delete semantics**: unknown deployID → `ErrResourceNotFound` (handler → 404), matching
  the mock's `ErrDeployNotFound` so the DELETE contract is profile-consistent (NOT a silent
  no-op). Service-already-gone on delete is tolerated (idempotent re-delete). Foreground
  propagation so the Deployment's ReplicaSet/Pods GC with it.
- **Partial-apply rollback**: if the Service create fails after the Deployment landed, the
  Deployment is best-effort deleted so a retry doesn't hit `ErrDeployConflict` on a
  half-applied pair.

## Verification

Offline layer (dev machine · CPU-arch-independent · ADR-0024 §8 双层 verify layer 1):

- `go build ./...` → OK (clean).
- `go test ./...` → all PASS, no regressions; `pkg/datasource/k8s` **coverage 78.4%**
  (≥70% core-pkg target). deploy_test.go: materialise (name/ns/labels/replicas/image/NPU
  request), defaults, name generation, name sanitization, validation errors, duplicate
  conflict, delete-owned, round-trip, unknown-id 404, label-selector scoping (delete alpha
  leaves beta), ctx-cancel.
- `CGO_ENABLED=0 GOARCH=arm64 GOOS=linux go build ./...` → OK (no cgo; pure client-go).
- `gofmt -l` on the two new files (LF view) → clean.
- `golangci-lint run ./...` → (see report; run from backend/).
- Seam grep (ADR-0024 §2 Decision G): `pkg/api/deploy.go` has no profile branch — it
  routes via `Registry.SourceFor("deploy")`; the real body lives only in the k8s source
  below the seam. demo config (`config.dev.yaml`) + chart default values still map
  `deploy: mock` → mock source backs the demo deploy unchanged.
- `grep ErrCapabilityUnavailable backend/pkg/datasource/k8s/*.go` → Deploy/DeleteDeploy no
  longer among the returns (ListNPUSlicePools / ListPresets / GetPreset / QueryMetric
  legitimately remain — those caps are served by other sources or unimplemented here).
- `config.real.yaml` `mapping.deploy: k8s` (≠ mock); real profile values inlined mapping
  flipped to `deploy: k8s`.

## Real-machine stamp (LAB-GATED — NOT verified, per plan §8 / ADR-0024 §3)

The real-machine "对接 stamp" for this task — lab arm64 cluster, real profile,
`POST /api/v1/deploy` → `kubectl get deploy,svc` materialise + `DELETE` cleans up (real
README deploy gap close) — is **deferred to lab access (P13-T-301 真机 E2E)**. The
offline layer (fake-clientset apply/delete) proves functional correctness; the lab only
needs to confirm the typed client writes reach a real apiserver + RBAC is in place. Marked
lab-gated; not claimed as done.

## Carry-forward

- **RBAC (deploy-module territory · MUST land before real deploy works)**: the backend
  ServiceAccount needs `create`/`delete`/`list`/`get` on `apps/v1 deployments` +
  `core/v1 services` (label-selector delete needs `list`). The base demo-backend chart
  RBAC (`deploy/helm-charts/demo-backend/templates/rbac.yaml`) currently grants only READ
  verbs. I did NOT edit the base chart (module boundary) — noted in
  `deploy/profiles/real/demo-backend.values.yaml` header + here. P13-T-202 (RBAC manifest)
  or P13-T-301 (真机 E2E) must add the write verbs, else a real `POST /api/v1/deploy` gets
  apiserver 403. **This is the #1 thing to double-check.**
- **Image source**: real deploys should pass `parameters.image` (the preset's resolved
  image). The default `quay.io/ascend/vllm-ascend:latest` is a placeholder so a minimal
  request still produces a runnable shape; not pinned.
- **mock source now unreferenced** in the real profile (all mappings real). Left
  `mock.enabled: true` for emergency fallback; could be disabled in a later cleanup once
  confirmed nothing constructs it expecting presence.
- **Status is "accepted" with empty ScheduledNodes**: real scheduling is async — nodes
  populate once pods bind, reflected via `ListWorkloads`, not synthesized at deploy time
  (unlike the mock which fabricates scheduledNodes from picked slices).
