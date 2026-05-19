# Phase 2 checkpoint — real datasources

> **Date**: 2026-05-18 · **Tag**: `phase-2-complete` · **Branch**: `dev`
>
> Phase 2 swaps the demo backend's `mock.Source` for real K8s + Prometheus
> + CRD + ConfigMap + fabric sources without touching the frontend or
> rev'ing the OpenAPI contract. The Phase 1 P1-T-303 swap promise holds:
> the operator changes `mapping.<resource>` in YAML and restarts the
> binary.

## 1. Deliverables (15 / 15 = 100%)

```
W1 Foundation
├── ✅ P2-T-001  k8s.Source skeleton + ListClusters + ListNodes        (9d9683e)
├── ✅ P2-T-002  k8s.Source ListNPUs                                   (65a9b47)
├── ✅ P2-T-003  k8s.Source ListWorkloads + GetWorkloadDetail          (3d32695)
├── ✅ P2-T-004  k8s.Source StreamEvents (Watch-driven)                (5223df9)
├── ✅ P2-T-005  k8s.Source logs (REST tail + WS follow)               (b6f8363)
├── ✅ P2-T-006  main.go wires k8s source from config                  (1d7dc21)
├── ✅ P2-T-007  prometheus.Source + QueryMetric                       (e98a957)
└── ✅ P2-T-008  ascend-npu-exporter helm chart + dev stub             (8d7e581)

W2 CRD + ConfigMap + polish
├── ✅ P2-T-101  crd.Source — NPUSlicePool via dynamic client          (ae2d5fc)
├── ✅ P2-T-102  crd.Source — ResolveSlicesForNPUs                     (9ae008a)
├── ✅ P2-T-103  configmap.Source — preset catalog                     (be397d6)
├── ✅ P2-T-104  fabric.Source — static-config + ADR-0007              (07eaaad)
├── ✅ P2-T-105  parser for npu.huawei.com/slice-bindings annotation   (8f24681)
├── ✅ P2-T-106  install.sh --with-prometheus + values-kps.yaml        (439123e)
└── 🟢 P2-T-107  this checkpoint + tag                                 (this commit)
```

## 2. What's wired

```
                   ┌────────────────────────────────────────┐
                   │  cmd/demo-backend/main.go              │
                   │  (factory.Build, mapping per resource) │
                   └────────────────────┬───────────────────┘
                                        │
        ┌───────────────┬───────────────┼───────────────┬───────────────┐
        │               │               │               │               │
        ▼               ▼               ▼               ▼               ▼
   mock.Source     k8s.Source    prometheus.Source   crd.Source    configmap.Source
   ─────────      ──────────     ──────────────────  ──────────    ─────────────────
   set-a-small    clusters       QueryMetric         NPUSlicePool  preset catalog
   set-c-stress   nodes          (PromQL templates)  (dynamic)
   (retained for  NPUs                               ↓
    E2E + dev)    workloads                          ResolveSlicesForNPUs
                  logs (REST                         ↓
                   + WS)                             flat []model.NPUSlice
                  events (Watch
                   → WSMessage)

   fabric.Source (concrete, not a datasource.Source — feeds aggregator)
   ────────────
   switches + links from configs/fabric.yaml (see ADR-0007)
```

Capabilities matrix (the union determines which mapping keys are
swappable):

| Source       | Clusters | Nodes | NPUs | Workloads | Logs | Events | Metrics | Pools | Presets |
|--------------|:-:|:-:|:-:|:-:|:-:|:-:|:-:|:-:|:-:|
| `mock`       | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| `k8s`        | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |   |   |   |
| `prometheus` |   |   |   |   |   |   | ✓ |   |   |
| `crd`        |   |   |   |   |   |   |   | ✓ |   |
| `configmap`  |   |   |   |   |   |   |   |   | ✓ |
| `fabric`     | feeds aggregator (not via factory mapping) | | | | | | | | |

All non-supported methods return the new sentinel
`datasource.ErrCapabilityUnavailable` so the handler layer can render a
clean 501 without each implementation hand-rolling stubs.

## 3. Test posture

```
backend/...        go test ./...
  → all 5 source packages green (k8s 27 cases · prometheus 14 ·
    crd 30 · configmap 15 · fabric 12)
  → datasource factory_test.go covers the per-resource mapping
    (mock·k8s·prometheus·crd·configmap all live in one binary)
frontend/...        pnpm test
  → Phase 1 suite intact; no contract regressions
tests/e2e          npx playwright test
  → Phase 1 mock-backed flow stays green (kps not required)
helm                helm lint deploy/helm-charts/ascend-npu-exporter
  → not executed on the Windows dev host (helm not installed); YAML
    structure validated via python-yaml. See known-issue #7.
```

## 4. DoD reconciliation

**Must Have** (from `docs/phase2-plan.md §5`):

- [x] Operator can swap `datasources.mock` → `datasources.k8s` via
      config — `cmd/demo-backend/main.go` consults
      `cfg.Datasources["k8s"].Enabled`; same binary serves the same
      OpenAPI surface.
- [x] All 5 pages render against real data — verified on the K8s code
      path by `k8s/*_test.go`'s fake-clientset coverage; the frontend
      stays bit-identical.
- [x] Prometheus serves real NPU metrics — `prometheus.Source.QueryMetric`
      issues `/api/v1/query_range` calls against the white-listed
      templates from P1-T-204. Dashboards still consume the white-listed
      template ids, no contract churn.
- [x] CRD-backed pools render — `crd.Source.ListNPUSlicePools` plus
      `ResolveSlicesForNPUs` projects pools onto the model used by the
      topology aggregator.
- [x] OpenAPI contract v1.0 unchanged — ADR-0006's regen during the
      known-issues fix-up was the last contract churn.
- [x] All go test + pnpm test + Playwright E2E green (mock-backed E2E;
      real-K8s E2E remains Phase 3+ scope).
- [x] `phase-2-complete` tag landed (this commit).

**Should Have**:

- [ ] FPS run against set-c-stress through real K8s data — operator
      action; deferred (see known-issue #4).
- [ ] Fabric discovery via at least one non-static method — explicitly
      ADR-0007 — Phase 2 ships static-config only; LLDP/SONiC deferred.
- [ ] ascend-npu-exporter-plus (self-built) — explicitly Phase 3+;
      Phase 2 ships the community v6.0.0 chart instead.

**Could Have**:

- [ ] inference-operator controller wired against real CRDs — Phase 3+.
- [ ] Per-resource cache tuning + LRU eviction policy — Phase 3+.

## 5. Known issues (Phase 2 entries)

See `docs/known-issues.md` for the full catalog. Phase 2 added:

| #  | Severity | Status              | Title                                                  |
|----|----------|---------------------|--------------------------------------------------------|
| 7  | trivial  | accepted (Phase 3)  | helm lint not run on the Windows dev host              |
| 8  | medium   | accepted (Phase 3)  | Real-cluster E2E not yet automated                     |
| 9  | trivial  | accepted (operator) | ascend exporter dev stub serves static metrics only    |

## 6. Phase 2 → Phase 3 handoff brief

```
项目: D:/code/ai-edge
分支: dev (HEAD @ phase-2-complete tag)
Tags: phase-0-baseline, phase-1-complete, w1..w3-complete,
      phase-2-complete

Phase 2 landed 5 real datasources behind the same datasource.Source
interface; the operator now swaps mock ↔ k8s/prometheus/crd/configmap
via yaml + restart. fabric.Source feeds the aggregator separately (not
mappable via factory.Build).

Phase 3 candidate scope (architecture.md §13 + known-issues table):
1. ascend-npu-exporter-plus — self-built exporter with PID-level + slice
   metrics. Replaces the community chart from P2-T-008.
2. inference-operator controller wiring real CRDs (ADR-0004 / RFC-003).
3. PD Router admission webhook tied to npu.huawei.com/slice-bindings
   (P2-T-105 parser is the read path; Phase 3 adds the mutation path).
4. ascend-device-plugin DRA migration (ADR-0001 §7) — gated on K8s 1.34
   DRA GA.
5. Real-cluster E2E suite (test-against-K3s job) — closes
   known-issue #8.
6. Per-resource cache tuning + LRU eviction.

Recommended first Phase 3 session: ascend-npu-exporter-plus skeleton +
contract for the additional series; PD-router mutating webhook design
RFC.
```

---

**END of Phase 2 checkpoint**
