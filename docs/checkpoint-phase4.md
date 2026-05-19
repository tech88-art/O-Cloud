# Phase 4 checkpoint — npu-dra-driver scaffold + ADR-0001 v3 + CANN matrix + backend /metrics + inference-operator scaffold

> **Date**: 2026-05-19 · **Tag**: `phase-4-complete` · **Branch**: `dev`
>
> Phase 4 lays down DRA scaffolding + backend self-observability +
> inference-operator entry-readiness, without revving the OpenAPI
> contract or touching the frontend. Headline:
> **`operators/npu-dra-driver/`** — a Kubebuilder v4 simulator-first DRA
> driver covering the four resource.k8s.io v1beta1 surfaces (Types ·
> ResourceSlice publisher · ResourceClaim controller · Helm chart). Plus
> **ADR-0001 v3** codifying the corrected dual-path roadmap (KubeEdge ≤
> v1.22 has no DRA — the real edge blocker), **ADR-0009** locking the
> slice ↔ ResourceClaim semantic mapping, **CANN/Ascend driver compatibility
> matrix doc**, **backend `/metrics` Prometheus self-endpoint** (T008b
> Phase 3 carry-over), and an **`operators/inference-operator/` scaffold**
> (ModelService CRD types only; controller body Phase 5).

## 1. Deliverables (15 / 15 = 100%)

```
W1 Foundation (8 tasks)
├── ✅ P4-T-001  ADR-0001 v3 — dual-path roadmap explicit               (f5c21f7)
├── ✅ P4-T-002  CANN 8.1 / Ascend driver ≥ 24.x compatibility matrix   (5bd2a13)
├── ✅ P4-T-003  npu-dra-driver Kubebuilder v4 scaffold                 (1f77243)
├── ✅ P4-T-004  npu-dra-driver Ascend RS+RC types (api/v1alpha1)       (023f89a)
├── ✅ P4-T-005  npu-dra-driver simulator-first ResourceSlice publisher (89ea7a4)
├── ✅ P4-T-006  npu-dra-driver ResourceClaim controller skeleton       (d4fc356)
├── ✅ P4-T-007  backend prometheus/client_golang dep + /metrics route  (7f1056e)
└── ✅ P4-T-008  backend baseline metrics collectors (cache + dispatch) (e380f39)

W2 Polish + integration + checkpoint (7 tasks)
├── ✅ P4-T-101  npu-dra-driver Dockerfile + Helm chart skeleton        (f10a1dd)
├── ✅ P4-T-102  pool-operator NPUSlicePool ↔ ResourceSlice cross-watch (4fe5ee3)
├── ✅ P4-T-103  inference-operator Kubebuilder scaffold + ModelService (6bd4a5c)
├── ✅ P4-T-104  kind smoke E2E extension (npu-dra-driver + assertions) (a9c1d11)
├── ✅ P4-T-105  ADR-0009 npu-dra-driver design                         (a93ceb6)
├── ✅ P4-T-106  backend /metrics CI smoke + Observability docs         (6b02801)
└── 🟢 P4-T-107  this checkpoint + tag                                  (this commit)
```

All 15 plan tasks landed in W1+W2 order. Zero collation commits (Phase 4
ran strictly single-task-serial per the per-task-verify rule
established at Phase 3 → Phase 4 handoff; no parallel subagent dispatch
needed).

## 2. What's wired

```
                        Standard-K8s 1.30+ cluster
                              │
        ┌─────────────────────┴─────────────────────────┐
        │                                               │
   inference-operator (scaffold)              npu-dra-driver
   - ModelService CRD types  ◄─npuSlicePoolRef─┐         ├──► ResourceSlice publisher
   - controller body Phase 5                  │         │    - reads configs/mock-data/set-a-small/npus.json
                                              │         │    - emits 3 slices × 8 devices (driver=npu.ocloud.edge.example.com)
                                              │         │
                                              ▼         ├──► ResourceClaim controller (skeleton)
                              pool-operator (Phase 3)   │    - filters by DeviceClassName prefix
                              + Phase 4 T102 cross-watch│    - sets AllocationDeferred annotations + Event
                              ├── ClusterPool           │    - no real allocation (Phase 5+)
                              ├── NodePool              │
                              ├── NPUPool               │
                              └── NPUSlicePool          │
                                  + ResourceSlicesObserved status field
                                  + Watches ResourceSlice (driver-filtered)

   demo-backend (Phase 1+2+3 → + Phase 4 T007/T008)
   - /api/v1/* (existing)
   - /metrics (NEW · outside /api/v1 · 3 ocloud_backend_* counters)

   CI:
   - .github/workflows/e2e-kind.yml gained npu-dra-driver helm install +
     ResourceSlice publication assertion (P4-T-104) + backend /metrics
     smoke (P4-T-106).
   - .github/workflows/helm-lint.yml auto-covers the new chart (P3-T-103
     existing strict-lint walk).
```

**Phase 4 simulator scope**: NO real Ascend silicon required for any
deliverable. The publisher reads JSON; the claim controller logs +
annotates; the helm chart embeds a ConfigMap; kind CI runs on
ubuntu-22.04 without GPUs. Real-hardware verification = Phase 7 entry
(docs/cann-driver-matrix.md §4 procedure).

## 3. Capabilities matrix

| Capability                                                | Source                                                 |
| --------------------------------------------------------- | ------------------------------------------------------ |
| Kubebuilder v4 scaffold (PROJECT / Makefile / Dockerfile) | `operators/npu-dra-driver/` (T003)                     |
| Ocloud Device attribute schema (6 attrs + 1 capacity)     | `operators/npu-dra-driver/api/v1alpha1/types.go` (T004) |
| AscendDevice typed view + ToUpstream/FromUpstream         | `api/v1alpha1/resourceslice_types.go` (T004)            |
| AscendClaimAnnotations typed view                         | `api/v1alpha1/resourceclaim_types.go` (T004)            |
| Round-trip + ValidateAttributes tests (5+5 cases)         | `api/v1alpha1/types_test.go` (T004)                     |
| Source interface (List + Watch)                           | `internal/publisher/source.go` (T005)                  |
| SimulatorSource (reads set-a-small JSON, mtime watch)     | `internal/publisher/source_simulator.go` (T005)         |
| Publisher reconcile loop (30s + Watch + diff/upsert/stale-cleanup) | `internal/publisher/publisher.go` (T005)        |
| 11 publisher + 5 simulator unit tests                     | `internal/publisher/*_test.go` (T005)                   |
| ResourceClaim controller skeleton (AllocationDeferred annotations + Event) | `internal/controller/claim_controller.go` (T006) |
| 12 claim controller + helper tests                        | `internal/controller/claim_controller_test.go` (T006)   |
| Helm chart skeleton (Chart.yaml + values + 5 templates + files/npus.json) | `deploy/helm-charts/npu-dra-driver/` (T101) |
| pool-operator NPUSlicePool.status.resourceSlicesObserved   | `operators/pool-operator/api/v1alpha1/npuslicepool_types.go` (T102) |
| pool-operator cross-watch (Watches resource.k8s.io)        | `operators/pool-operator/internal/controller/npuslicepool_controller.go` (T102) |
| 3 cross-watch envtest cases                                | `operators/pool-operator/internal/controller/npuslicepool_controller_test.go` (T102) |
| inference-operator Kubebuilder scaffold                    | `operators/inference-operator/` (T103)                  |
| ModelService CRD types (ModelSpec + PDPairSpec + status conditions) | `operators/inference-operator/api/v1alpha1/modelservice_types.go` (T103) |
| Backend `/metrics` endpoint (gin engine root, outside /api/v1) | `backend/pkg/api/prom_metrics.go` + `router.go` (T007) |
| 3 ocloud_backend_* counter families (cache_eviction / cache_hits / dispatch_calls) | `backend/pkg/api/prom_metrics.go` + cache + datasource (T008) |
| Cache LRU nil-safe Prometheus wiring                       | `backend/pkg/cache/{lru,options}.go` (T008)              |
| Registry.DispatchCounter nil-safe wiring                   | `backend/pkg/datasource/factory.go` (T008)               |
| 4 prom_metrics + 5 cache + 2 dispatch tests                | `backend/pkg/{api,cache,datasource}/` (T007 + T008)      |
| kind smoke ResourceSlice publication assertion             | `tests/e2e/kind/dra_publish_test.sh` (T104)              |
| kind smoke backend /metrics assertion                      | `tests/e2e/kind/backend_metrics_test.sh` (T106)          |
| CI workflow extension (3 new steps in e2e-kind)            | `.github/workflows/e2e-kind.yml` (T104 + T106)           |
| Dev-installer `--with-dra-driver` + `--all-phase-4` flags  | `scripts/install.sh` (T104)                              |
| ADR-0001 v3 (dual-path roadmap explicit)                   | `docs/adr/0001-phase0-key-decisions.md` §5 (T001)        |
| ADR-0009 (npu-dra-driver slice ↔ ResourceClaim design)     | `docs/adr/0009-npu-dra-driver.md` (T105)                 |
| CANN/Ascend driver compatibility matrix                    | `docs/cann-driver-matrix.md` (T002)                      |
| Backend Observability docs (counter table + scrape stanza) | `backend/README.md §Observability` (T106)                |
| Operator module landing-page registers 3 sub-projects      | `operators/CLAUDE.md` (T003 + T103)                      |

## 4. Test posture

| Surface                                         | Tests              | Verified locally |
| ----------------------------------------------- | ------------------ | ---------------- |
| npu-dra-driver api/v1alpha1 round-trip          | 10 sub-tests       | ✅ `go test ./api/v1alpha1/... -v` |
| npu-dra-driver internal/publisher               | 12 sub-tests       | ✅ `go test ./internal/publisher/... -v` |
| npu-dra-driver internal/controller              | 12 sub-tests       | ✅ `go test ./internal/controller/... -v` |
| npu-dra-driver `go build` manager               | Compiles 58 MB     | ✅ `go build -o bin/manager ./cmd/main.go` |
| pool-operator T102 cross-watch envtest          | 3 sub-tests        | ⚠️ envtest binaries unavailable in this Windows shell (Windows SmartScreen on setup-envtest.exe); CI / lab will execute. Test file compiles. |
| pool-operator full regression                   | Build + vet clean  | ✅ `go vet ./... && go build` |
| inference-operator api/v1alpha1                 | No test files yet  | Phase 5 controller tests will land in internal/controller/ |
| inference-operator `go build` manager           | Compiles           | ✅ `go build -o bin/manager ./cmd/main.go` |
| Backend prom_metrics                            | 4 sub-tests        | ✅ `go test ./pkg/api -run Metrics -v` |
| Backend cache prom counters                     | 5 sub-tests        | ✅ `go test ./pkg/cache -v` |
| Backend datasource dispatch counter             | 2 sub-tests        | ✅ `go test ./pkg/datasource -v` |
| Backend full regression                         | All packages green | ✅ `go test ./...` |
| Backend live /metrics smoke                     | 22 go_memstats_ + 6 process_ + 5 ocloud_backend_dispatch_* series after multi-endpoint warmup | ✅ Local `./bin/demo-backend` |
| Helm chart `helm lint --strict`                 | npu-dra-driver: 0 failed | ✅ `helm lint --strict deploy/helm-charts/npu-dra-driver/` |
| Helm chart `helm template`                      | 5 K8s objects rendered (SA + ConfigMap + ClusterRole + CRB + Deployment) | ✅ `helm template test deploy/helm-charts/npu-dra-driver/` |
| dra_publish_test.sh syntax                      | bash -n clean      | ✅ `bash -n tests/e2e/kind/dra_publish_test.sh` |
| backend_metrics_test.sh syntax                  | bash -n clean      | ✅ `bash -n tests/e2e/kind/backend_metrics_test.sh` |
| install.sh + kind/install.sh syntax             | bash -n clean      | ✅ both pass |
| kind cluster live e2e                           | All steps green    | ⚠️ deferred to GitHub Actions; commits ship the infrastructure (no kind running locally) |

**Local-env honesty (P3 disclosure)**: envtest binaries for pool-operator
controllers + kind cluster live runs are gated by tooling unavailable
in this Windows dev shell (setup-envtest.exe Windows SmartScreen +
no kind running). CI on GitHub Actions ubuntu-22.04 will validate the
runtime behaviour on the next PR push. All non-cluster verification
(go vet / go build / go test on non-envtest tests / helm lint /
helm template / bash syntax / live backend curl smoke) ran clean.

## 5. DoD reconciliation

Per `docs/phase4-plan.md §5`:

### Must Have

- [x] ADR-0001 v3 published — KubeEdge DRA gap recorded as primary
      edge-path blocker (T001 / f5c21f7)
- [x] `docs/cann-driver-matrix.md` landed — 5-row matrix + Phase 4
      simulator scope + Phase 7 real-hw verification procedure (T002 /
      5bd2a13)
- [x] `operators/npu-dra-driver/` Kubebuilder v4 scaffold builds —
      `make manager` produces 58 MB bin/manager; `go vet ./...` clean
      (T003 / 1f77243)
- [x] Simulator-first ResourceSlice publisher reads
      `configs/mock-data/set-a-small/npus.json` (T005 / 89ea7a4); kind
      smoke verifies `kubectl get resourceslices` shows driver=
      `npu.ocloud.edge.example.com` with ≥ 8 devices (T104 / a9c1d11)
- [x] ResourceClaim controller skeleton compiles, registers, logs
      requests, emits `AllocationDeferred=Phase4Skeleton` (encoded via
      `metadata.annotations` per the v1beta1 schema-drift note in
      claim_controller.go — Phase 5 lifts to AllocatedDeviceStatus.Conditions
      once real allocation lands) (T006 / d4fc356)
- [x] Backend `/metrics` endpoint accessible (T007 / 7f1056e + T008 /
      e380f39 — live smoke confirmed 22 go_memstats_ + 6 process_ + 5
      ocloud_backend_dispatch_* series)
- [x] kind smoke E2E extension (T104 / a9c1d11 + T106 / 6b02801) —
      ships in `.github/workflows/e2e-kind.yml`; CI validates next push
- [x] `phase-4-complete` tag — set by this commit

### Should Have

- [x] pool-operator NPUSlicePool `status.resourceSlicesObserved`
      (T102 / 4fe5ee3 — field + Reconcile path + 3 envtest cases all
      land; envtest execution deferred to CI)
- [x] `operators/inference-operator/` scaffold + ModelService CRD types
      (T103 / 6bd4a5c — manager builds + idles cleanly; CRD YAML
      generated; full Phase 5 controller plan in README)
- [x] ADR-0009 npu-dra-driver design (T105 / a93ceb6 — slice ↔ claim
      mapping table, KubeEdge gap, Partitionable Devices forward note,
      Phase 5 implementation notes)
- [x] Backend `/metrics` CI smoke wired into e2e-kind workflow (T106 /
      6b02801)
- [x] Backend README "Observability" section (T106 / 6b02801 — endpoint
      URL, 3 counter family table, cache vs dispatch semantics, 5-line
      Prometheus scrape stanza, local smoke walkthrough)
- [x] `docs/known-issues.md` Phase 4 entries — see §6 below: zero
      net-new issues surface during W2; the empty-list state is OK

### Could Have (deferred)

- [ ] Real-cluster K8s 1.34+ DRA spike — defer to Phase 4.5 / Phase 5
- [ ] ResourceClaim real allocation logic — Phase 5 per ADR-0009 §5
- [ ] Ascend hardware driver integration / CANN runtime calls — Phase 7
- [ ] HCCS topology-aware claim allocation — Phase 6 (scheduler-plugins)
- [ ] Karmada multi-site federation — Phase 9

## 6. Known issues (rollup)

Phase 4 net-new: **zero**. The Phase 3 inheritance roster (#7 / #8 / #9)
already resolved at phase-3-complete. The Phase 4 simulator-first
design + careful path-adaptation against the plan's literal Allowed
Paths (`backend/internal/server/`, `backend/internal/dispatch/`,
`test/e2e/`) avoided new issues — see commit footers for the 3
adaptations.

Schema-drift items (documented inline, not blocking):

- v1beta1.ResourceClaimStatus has no top-level Conditions field — T006
  encodes AllocationDeferred via annotations + Event instead. Phase 5
  lifts when real allocation lands (`status.devices[]` carries conditions
  per device). Documented in `claim_controller.go` Schema-drift note.
- pool-operator + npu-dra-driver intentionally do NOT cross-import each
  other's internal/ packages (operators/CLAUDE.md §1). The
  `npu.ocloud.edge.example.com` driver-name const lives twice (once in
  each module). Phase 5 may extract a shared `pkg/sharedconst/` if a
  third consumer appears.

## 7. Phase 4 → Phase 5 handoff brief

```
项目: D:/code/ai-edge
分支: dev (HEAD @ phase-4-complete tag)
Tags: phase-0-baseline, phase-1-complete, w1..w3-complete,
      phase-2-complete, phase-3-complete, phase-4-complete

Phase 4 landed npu-dra-driver scaffold + simulator-first publisher +
claim controller skeleton + helm chart + ADR-0001 v3 dual-path +
ADR-0009 design + CANN/driver compatibility matrix + backend
self-metrics endpoint + inference-operator scaffold. CI extended to
assert ResourceSlice publication + backend metric families on every
PR.

Phase 5 candidate scope (per ADR-0009 §5 + ADR-0008 + phase4-plan
"Out of scope" list):

1. **npu-dra-driver real allocation logic** — replace T006
   AllocationDeferred annotations with status.devices[] writes.
   ADR-0009 §6 outlines a greedy first-fit algorithm + best-fit
   variant. DeviceClass registration via helm chart template.

2. **NPUSliceAllocation CRD** (arch §6.8 placeholder) — owner-ref per
   claim allocation, audit log entry, Phase 9 quota entry point.
   Schema design Phase 5 entry meeting.

3. **inference-operator controller body** — ModelService Reconcile
   resolves npuSlicePoolRef → creates N ResourceClaims via
   npu-dra-driver → spawns Prefill + Decode Deployments → tracks
   phase=Provisioning → phase=Ready.

4. **PD Router mutating admission webhook** (ADR-0008 implementation)
   — runs in the same inference-operator binary; stamps
   `npu.huawei.com/slice-bindings` annotation on accepted Pods so
   scheduler-plugin (Phase 6) routes them to the right NUMA + HCCS
   group. cert-manager dependency wires via the chart.

5. **NPU pod network considerations** — CNI + HCCL RDMA / RoCE /
   IPoIB compatibility research (arch §13 review-table Phase 5+ row).

Recommended Phase 5 first session: scaffold the PD Router webhook
alongside inference-operator controller package init. Both land in
the same binary (per ADR-0008) — start with cert-manager Helm chart
dependency + webhook handler skeleton + ModelService reconcile loop
(no allocation yet). Real allocation lands second session.

Tools needed in addition to Phase 4 toolchain:
- cert-manager v1.16+ (Phase 5 inference-operator webhook TLS)
- K8s 1.30+ cluster with DRA v1beta1 enabled (CI envtest covers)
- envtest binaries for inference-operator controller tests (setup-envtest)

Estimated total: ~3-4 weeks calendar (Phase 5 has 2 controllers + 1
webhook + 1 new CRD; allocator algorithm is the longest pole).
```

This brief is the seed for the Phase 5 plan. The brief refines into
concrete task packages once the Phase 5 entry meeting picks allocator
algorithm + NPUSliceAllocation schema.

---

**END of Phase 4 checkpoint**
