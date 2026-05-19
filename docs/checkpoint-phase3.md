# Phase 3 checkpoint — pool controllers + ascend-npu-exporter-plus

> **Date**: 2026-05-19 · **Tag**: `phase-3-complete` · **Branch**: `dev`
>
> Phase 3 stands up the project's first three control loops without
> revving the OpenAPI contract or touching the frontend: pool-operator
> Reconcile (the M2 "池化与发现" headline), the self-built
> `ascend-npu-exporter-plus` (replaces community chart + dev stub),
> and ADR-0008 PD Router admission webhook **design only** (impl
> deferred to Phase 5). Plus three Phase 2 known-issues paid down:
> #7 helm-lint CI · #8 real-cluster E2E · #9 dev stub retirement.

## 1. Deliverables (15 / 15 = 100% + 3 maintenance)

```
W1 Foundation
├── ✅ P3-T-001  pool-operator controller scaffolding              (4361c28)
├── ✅ P3-T-006  ascend-npu-exporter-plus skeleton                 (1b5fd2e)
├── ✅ P3-T-002  NPUSlicePool Reconcile (status + finalizer)       (3fcd110)
├── ✅ P3-T-003  NPUPool Reconcile (selector → NPU set)            (96f9a4b)
├── ✅ P3-T-007  exporter NPU-level collector                      (51cba7f)
├── 🔧 fix       dedup npuCapacityResource + findCondition         (da6f416)
├── ✅ P3-T-004  NodePool Reconcile (selector + role → node set)   (309276c)
├── ✅ P3-T-105  ClusterPool placeholder Reconcile                 (c7c1204)
├── ✅ P3-T-008  per-resource cache eviction + TTL (Must Have)     (99c5879)
├── 🔧 collate   pool-operator controllers + regen role.yaml       (1640961)
└── ✅ P3-T-005  ValidatingAdmissionPolicy skeleton                (f62657b)

W2 Polish + control + tests
├── ✅ P3-T-101  exporter slice-level metrics                      (7d3a6dd)
├── ✅ P3-T-102  exporter PID-level workload metrics               (83b587d)
├── 🔧 collate   exporter slice + workload collector registration  (320d258)
├── ✅ P3-T-106  ADR-0008 PD Router admission webhook design       (f5f6a7b)
├── ✅ P3-T-103  Helm chart + retire community + helm-lint CI (#7) (bb23933)
├── ✅ P3-T-104  Real-cluster kind smoke E2E (#8)                  (7134098)
└── 🟢 P3-T-107  this checkpoint + tag                             (this commit)
```

15 plan tasks all landed. 3 maintenance commits (1 dedup fix surfacing
from parallel-subagent dispatch in batch 2, 2 main-agent collations
that deliberately serialise shared-file edits — `cmd/main.go` for
operator default-flip + `cmd/exporter-plus/main.go` for slice +
workload collector registration).

## 2. What's wired

```
                        K8s 1.30+ cluster
                              │
        ┌─────────────────────┼─────────────────────┐
        │                     │                     │
        ▼                     ▼                     ▼
  pool-operator       ascend-npu-exporter-plus   demo-backend
  ─────────────       ────────────────────────   ────────────
  4 Reconcile         NPU + slice + PID          mock|k8s|crd|
  loops               (3 collectors)             prometheus|...
  ────────────        ──────────────────         ─────────────
  NPUSlicePool        ascend_npu_*               GET /api/v1/...
  NPUPool             ascend_slice_*             (P1 contract
  NodePool            ascend_workload_*           unchanged)
  ClusterPool         exporter_build_info
   (placeholder)      collect_duration_seconds
  ────────────        ──────────────────
  + VAP rejects       + Helm chart
    NPUSlicePool        + retire community
    outside             + helm-lint CI on
    ocloud-system         every PR
    (per-T005 VAP)
```

ValidatingAdmissionPolicy from T005 enforces multi-tenancy by-convention
gap from `architecture.md §6.7` (Phase 9 will supersede with full
RBAC + Karmada). T106 lands ADR-0008 (PD Router mutating webhook
**design only** — impl follows inference-operator in Phase 5).

Capabilities matrix (the matrix the four Reconcile loops + the
exporter populate; consumers downstream of `phase-3-complete` see
these surfaces):

| Source                       | NPUSlicePool | NPUPool | NodePool | ClusterPool | Slice | Workload (PID) | NPU |
|------------------------------|:-:|:-:|:-:|:-:|:-:|:-:|:-:|
| pool-operator Reconcile      | ✓ | ✓ | ✓ | ⏳(placeholder) |   |   |   |
| ascend-npu-exporter-plus     |   |   |   |   | ✓ | ✓(gated) | ✓ |
| backend `pkg/datasource/k8s` |   |   |   |   |   |   |   |
| backend `pkg/datasource/crd` | ✓ | ✓ | ✓ | ✓ |   |   |   |

Pool operator's ClusterPool Reconcile is the explicit Phase 9 forward
note (`Reason=WaitingForKarmada` condition, no Karmada API watches).
Workload (PID-level) metrics are gated off by default
(`--enable-workload-correlation=false`) per `exporters/CLAUDE.md §8`.

## 3. Test posture

```
operators/pool-operator   make test (envtest 1.35 on Linux runner)
  → 17 ginkgo specs green:
    5 NPUSlicePool · 5 NPUPool · 4 NodePool · 2 ClusterPool placeholder
    · 1 scaffolding placeholder (T001 baseline)
  → 7 plain-Go utils tests green (SetCondition / RemoveCondition /
    MakeOwnerRef / RequeueAfter / IsEnabled bitmask)
  → admission e2e: 3 cases (reject default-ns no-bypass / accept
    ocloud-system / accept default-ns + bypass label)
exporters/ascend-npu-exporter-plus   go test ./...
  → registry (100% cov) · server (85.7%) · collector (≥80%) ·
    sources (≥80%) — covers NPU + slice + PID + simulator paths
backend                   go test ./pkg/cache/... + existing
  → cache 16 cases (Options/TTL/capacity/Concurrent counter ×1000
    goroutines/Catalog lazy create + concurrent lookup ×200/etc.)
  → existing handlers + datasource factory all green
tests/e2e/specs/         playwright (kind smoke)
  → 3 API-level cases against real kind cluster (T104 workflow)
tests/e2e/tests/         playwright (Phase 1 mock-backed)
  → Phase 1 suite preserved + verified non-regressing by the
    e2e-mock-regression job in e2e-kind.yml
.github/workflows/        helm-lint + e2e-kind
  → helm-lint runs on every PR touching deploy/helm-charts/**
  → e2e-kind + e2e-mock-regression run on every PR / dev push
```

Main-agent strict-verify mode (per 2026-05-19 user feedback) applied
batch 4 onward: each task gets its own subagent + worktree +
independent main-agent verify (helm lint/template, `go test` on dev
tree, functional curl, syntax checks) before squash merge to dev.
Earlier batches (2 + 3) used the loosened "batch of 3 + late
coexistence verify" pattern; that pattern produced one dedup fix
(da6f416) when two subagents independently declared the same
package-level symbol, which the strict-verify mode would have caught
earlier.

## 4. DoD reconciliation

**Must Have** (from `docs/phase3-plan.md §5`):

- [x] `kubectl apply` of NPUSlicePool / NPUPool / NodePool populates
      status fields — envtest covers the controller path (17 specs);
      kind CI (T104) covers the real-cluster path via
      `kubectl wait NPUSlicePool.status.totalSlices > 0`.
- [x] All 4 pool kinds have green controller envtest + manager binary
      starts cleanly with all four registered — `bin/manager --help`
      lists `--enable-controllers (default 15)`; envtest envelope
      passes 17/17.
- [x] `ValidatingAdmissionPolicy` rejects NPUSlicePool outside
      `ocloud-system` without bypass label — T005 e2e admission suite
      (`test/e2e/admission/`, `//go:build e2e`) verifies the 3 paths.
- [x] `ascend-npu-exporter-plus` Helm install on kind + simulated NPU
      shows non-flat metrics — T103 chart + T104 kind smoke; **caveat**:
      the smoke proves the *exporter chain* (simulator → collector →
      registry → /metrics has `ascend_npu_utilization_percent > 0`),
      not the *Grafana dashboard render*. Frontend deploy in kind was
      skipped (T104 disclosed deviation; Phase 1 mock-backed Playwright
      already covers UI regression). Dashboard panels will receive data
      when kube-prometheus-stack scrapes the ServiceMonitor in a real
      deployment.
- [x] Community `ascend-npu-exporter` chart and
      `deploy/dev/ascend-exporter-stub/` removed —
      `git diff phase-2-complete..HEAD --stat` shows 10 deletions
      from `deploy/helm-charts/ascend-npu-exporter/` + 2 from
      `deploy/dev/ascend-exporter-stub/`.
- [x] `scripts/install.sh --with-prometheus` references new chart
      only — `grep -c 'ascend-npu-exporter[^-]' scripts/install.sh = 0`.
- [x] CI: `helm-lint` job — `.github/workflows/helm-lint.yml`
      (T103).
- [x] CI: `e2e-kind` job — `.github/workflows/e2e-kind.yml` (T104,
      two jobs: kind-smoke + mock-regression).
- [x] ADR-0008 landed with options table + decision + Phase 5
      implementation notes — `docs/adr/0008-pd-router-webhook.md`
      (T106, 168 lines, follows ADR-0007 format).
- [x] `phase-3-complete` tag — this commit.

**Should Have**:

- [x] LRU cache eviction count observable — T008 ships atomic
      `EvictionCounter` + `Catalog.Snapshot()`. **Caveat**: Prometheus
      `/metrics` HTTP endpoint exposure deferred to T008b / T103
      follow-up (T008 was scoped to "Must Have" because backend has
      no `prometheus/client_golang` dep yet and adding one crosses
      several module boundaries; lands cleanly as a follow-up).
- [x] Per-resource TTL configurable via `configs/config.yaml` — T008
      `CacheConfig.EntryFor(resource)` resolver, Viper
      `StringToTimeDurationHookFunc` parses `"5m"`/`"30s"`.
- [x] PID-level workload metrics gated by
      `--enable-workload-correlation` (default OFF) — T102 + 320d258
      collation; verified end-to-end via functional `/metrics`
      curl post-merge.
- [x] `docs/known-issues.md` #7, #8, #9 all RESOLVED — `grep -E "^\| (7|8|9) .*RESOLVED"`
      matches 3 entries.

**Could Have** (deferred as expected):

- [ ] inference-operator scaffold — Phase 5 per `architecture.md §10.4`.
- [ ] PD Router webhook impl — Phase 5 per ADR-0008 §"实施".
- [ ] DRA migration first cut — Phase 4 per `architecture.md §1.3` +
      ADR-0001 v2 (DRA GA'd K8s 1.34 2025-09-01; gating factor is
      KubeEdge ≤ v1.22 having no DRA support, not K8s readiness).
- [ ] LLDP / SONiC fabric discovery — operator action on real
      switching gear per ADR-0007.

## 5. Known issues (rollup)

Phase 2 inheritance:

| #  | Severity | Status                                         |
|----|----------|------------------------------------------------|
| 7  | trivial  | **RESOLVED** (P3-T-103, helm-lint workflow)    |
| 8  | medium   | **RESOLVED** (P3-T-104, e2e-kind workflow)     |
| 9  | trivial  | **RESOLVED** (P3-T-103, dev stub retired)      |

Phase 3 net-new:

| # | Severity | Status                  | Title                                                  |
|---|----------|-------------------------|--------------------------------------------------------|
| — | trivial  | accepted (operator)     | Worktree stale dirs on Windows (NTFS file handle delay after envtest exit) |

The Windows-only NTFS lock leftover is operator-cosmetic (stale
directories under `D:/code/ai-edge-wt/p3-t-*`, git-untracked); no
runtime impact. Not filed as a numbered known-issue because it's
specific to a single developer's host and self-remediates on reboot.

## 6. Phase 3 → Phase 4 handoff brief

```
项目: D:/code/ai-edge
分支: dev (HEAD @ phase-3-complete tag)
Tags: phase-0-baseline, phase-1-complete, w1..w3-complete,
      phase-2-complete, phase-3-complete

Phase 3 landed the M2 milestone "池化与发现" headline: four pool
CRD controllers (NPUSlicePool / NPUPool / NodePool / ClusterPool
placeholder) populate status from real Node + Pod state, plus a
self-built ascend-npu-exporter-plus with NPU + slice + PID-level
collectors, plus a multi-tenancy VAP skeleton, plus ADR-0008
designing the Phase 5 PD Router webhook.

Phase 4 candidate scope (architecture.md §13 + ADR-0001 v2):

1. NPU DRA driver scaffold (npu-dra-driver/) — clone
   kubernetes-sigs/dra-example-driver, adapt for Ascend semantics
   (slice = ResourceClaim, NPUSlicePool = ResourceClass equivalent).
   DRA GA'd in K8s 1.34 (2025-09); Phase 4 gating factor is no
   official Ascend DRA driver + KubeEdge ≤ v1.22 lacks DRA support
   (edge path stays on Device Plugin v1; standard K8s small-cluster
   path can spike DRA). Update ADR-0001 to v3 tracking K8s 1.34/1.36
   GA status + KubeEdge readiness.
2. NPU device discovery + reporting — the Ascend Device Plugin v1
   already labels nodes; Phase 4 layers `ResourceSlice` publication
   so the DRA driver can advertise per-NPU + per-slice resources.
3. CANN 8.1 / Ascend driver ≥ 24.x compatibility — verify on real
   hardware matrix (architecture.md §13 review-table 2026-05-18
   row).
4. Backend `/metrics` Prometheus self-endpoint + cache eviction
   counter exposure — the T008b / T103 follow-up deferred from
   Phase 3 (backend gains prometheus/client_golang dep, mounts
   /metrics on the gin router outside /api/v1).

Recommended first Phase 4 session: scaffold npu-dra-driver/ from
dra-example-driver + a placeholder ResourceSlice publisher that
mirrors the synthetic NPU set in configs/mock-data/set-a-small/
(parallels the T006 exporter skeleton's Phase 3 thesis: simulator-
first, real-driver later). DRA driver ↔ pool-operator integration
lands once Phase 4 scaffold is green.

Tools needed in addition to Phase 3 toolchain:
- K8s 1.34+ cluster (DRA GA) for the DRA spike path
- kubebuilder v4 (already in use by pool-operator) for the new
  npu-dra-driver project
- ascend-device-plugin v6.0+ (compatible with the cluster K8s
  version) for the labels/health flow that T003 NPUPool Reconcile
  already consumes

Estimated total: ~3 weeks calendar (Phase 4 is the deepest
hardware-adjacent Phase to date; DRA driver + ResourceSlice publisher
+ pool-operator integration each carry meaningful ramp-up).
```

---

**END of Phase 3 checkpoint**
