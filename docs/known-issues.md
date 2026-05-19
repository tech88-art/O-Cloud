# Known Issues — Phase 1

> Issues identified during Phase 1 that are documented rather than fixed
> in-phase. Each entry lists the symptom, the root cause, the workaround
> (if any), and the proposed resolution path.

---

## 1. GrafanaPanel `react-refresh/only-export-components` lint warning

**Symptom**: `pnpm lint` emits one warning:

```
frontend/src/components/GrafanaPanel/index.tsx:33:17
warning  Fast refresh only works when a file only exports components.
react-refresh/only-export-components
```

**Root cause**: `GrafanaPanel/index.tsx` exports both the component
and a small lookup constant. Vite's React-refresh plugin disables HMR
on files that export non-components.

**Workaround**: HMR for this file silently falls back to a full reload
(the page works correctly — only dev ergonomics suffer).

**Resolution path**: split the constant out into a sibling file
(`GrafanaPanel/constants.ts`). Single-file change, ~5 minutes. Tracked
since T010 (W1) — left for a dedicated fix-up commit because every
T2xx-T3xx frontend agent noticed it but it sits outside their Allowed
Paths.

**Lands**: opportunistic; not blocking Phase 2.

---

## 2. ~~`docs/api-contract.yaml` enum drift on TopologyNode / TopologyEdge~~ ✓ RESOLVED

**Resolved**: 2026-05-18 via ADR-0006 (commit landed on dev, contract
regenerated + frontend types refreshed + widening casts dropped +
`model.Pod.Bindings` added + Pod.bindings surfaces over REST). The
schema mismatch noted below is the historical record.

**Original symptom**: The OpenAPI spec's `TopologyNode.type` enum was
`[cluster, nodepool, node, npu, slice, network]` and
`TopologyEdge.type` was `[contains, hccs, network, allocated]`. The
backend emitted additional runtime values not in either enum:

- nodes: `switch` (ADR-0004), `workload` / `pod` (ADR-0005)
- edges: `fabric-link` (ADR-0004), `binds-to` / `pd-pair` (ADR-0005)

**Original root cause**: each ADR explicitly chose to ship as
"runtime-only strings" rather than block its task on a contract regen.
The frontend's `TopologyGraph` widened the generated union via cast at
the render boundary, so the components stayed forward-compatible with a
later regen.

**Resolution**: ADR-0006 added the 6 missing enum values + Pod.bindings
+ relaxed `TopologyNode.status` to a free-form string (its value space
is polymorphic by node type — see ADR-0006 §"Status enum relaxation"
note in the YAML). Frontend cast widening sites removed in
`TopologyGraph.tsx` and `Overview/index.tsx`'s `buildTreeData`. Test
fixtures (`tests/Overview.test.tsx`) dropped the corresponding
`as unknown as 'contains'` workarounds.

---

## 3. ~~T013 generator drift~~ ✓ RESOLVED

**Resolved**: 2026-05-18. Generator extended to emit fabric
(`networkSwitches.json` + `networkLinks.json`), pod `bindings`, and
the two T307 D6 affinity workloads. Schema-validated parity at the
headline level (1 cluster / 3 nodes / 24 NPUs / 12 workloads / 1
switch / 3 fabric-links). Capability-vs-content split documented in
`configs/mock-data/generator/README.md` §"Capability vs content".

**Historical**: Symptom was `make gen-small` producing a set-a-small
without the fabric files or pod bindings — those fields were
hand-patched into the on-disk fixture by T013 / T307 but the Go
generator was never updated. Resolution preserved the canonical
hand-tuned narrative (events.json with T307 D6 metric-tick payloads,
denser per-workload labels) while enabling `make gen-small` to
reproduce a schema-equivalent fixture against `--output` for testing
and Phase 2 multi-site / stress preset development.

---

## 4. ~~set-c-stress (800 NPU) FPS test deferred~~ ✓ FIXTURE DELIVERED (FPS run pending)

**Resolved (fixture)**: 2026-05-18. `preset_stress.go` now emits a
schema-valid `configs/mock-data/set-c-stress/` (`make gen-stress`):

- 1 cluster / 100 nodes / 800 NPUs / 0 slices (whole-NPU mode)
- 20 workloads (1 PD pair with 20 pods + 4 medium × 3 pods + 15 small
  × 1 pod = ~47 pods) with bindings
- 1 spine + 4 leaf switches + 104 fabric links
- Schema-valid against `configs/mock-data/schema.json`

Backend smoke against `set-c-stress` with `?depth=npu&includeFabric=
true&includeWorkloads=true` returns:

  - **973 nodes / 1014 edges** (1 cluster + 100 nodes + 800 NPUs +
    20 workloads + 47 pods + 5 switches; 900 contains + 104 fabric-link
    + 10 pd-pair edges)
  - Note: binds-to edges drop because the stress preset uses whole-NPU
    mode (no `slice` nodes for the aggregator's orphan guard to match
    against). This is documented behavior — Phase 2 fabric work may
    revisit if we want binds-to → NPU edges in whole-NPU mode.

**Open (FPS measurement)**: the actual ADR-0005 推翻条件 check
("FPS ≥ 15 with all toggles ON") needs a human-run benchmark — open
`http://localhost:3000/overview` against a backend loaded with
set-c-stress, toggle both fabric + workloads ON, scroll for 30s, read
DevTools Performance panel. If FPS < 15, fall back to ADR-0005's
per-workload mini-graph.

This step is intrinsically manual (no headless FPS counter in Phase 1
E2E), so it stays on the operator's plate.

---

## 5. ~~WS event-driven E2E test skipped~~ ✓ RESOLVED

**Resolved**: 2026-05-18. Added `?fastforward=<N>` query parameter to
`/ws/topology`; `parseStreamOpts` in `backend/pkg/api/ws.go` plumbs
it into `model.StreamEventsOptions.FastForward`. Frontend
`useTopologyWS` accepts a matching `fastforward` option and appends
the param when set; `Overview/index.tsx` reads `?ffwd=<N>` from the
page URL and forwards it to the hook. The previously-skipped E2E
case now passes deterministically against `/overview?ffwd=100` (1.8s
replay vs 180s real-time). `WsStatusChip` also gained a
`data-last-event-at` data attribute as the stable assertion target.

---

## 6. ~~`data:` URL Grafana fallback when Grafana isn't running~~ ✓ RESOLVED

**Resolved**: 2026-05-18. `GrafanaPanel` now runs a reachability
probe (Image() against `/public/img/fav32.png` with 3s timeout) before
mounting the iframe. Outcomes:

- probing → AntD `<Spin>` placeholder
- reachable → iframe renders as before
- unreachable → AntD warning `Alert` with a clear "Grafana not
  reachable" message and the docker-compose start hint, both i18n'd
  in zh-CN and en-US

New `data-testid="grafana-panel-unreachable"` lets tests pin the
state directly. Vitest gained a `FakeImage` stub (in
`src/components/GrafanaPanel/index.test.tsx` and `tests/Metrics.test.tsx`)
so jsdom-driven test environments default to "reachable" (matching
the docker-compose flow); the new "renders the unreachable Alert when
the probe fails" case in GrafanaPanel/index.test.tsx exercises the
negative branch.

---

## Severity ranking

| # | Severity | Blocking demo? | Blocking Phase 2? | Status |
|---|---|---|---|---|
| 1 | trivial | no | no | **resolved** (split helpers, lint 0 warnings) |
| 2 | medium  | no | yes(was) → **resolved** (ADR-0006 contract regen) |
| 3 | medium  | no | yes(was) → **resolved** (generator emits fabric + bindings + D6 workloads) |
| 4 | medium  | no | maybe | **fixture delivered**; FPS run remains operator-side |
| 5 | trivial | no | no | **resolved** (WS `?fastforward=N` + frontend `?ffwd=` opt-in; E2E unskipped) |
| 6 | trivial | no | no | **resolved** (reachability probe + Unreachable Alert) |
| 7 | trivial | no | no | **RESOLVED** (2026-05-19, P3-T-103 — `.github/workflows/helm-lint.yml` runs `helm lint --strict` + `helm template` on every PR touching `deploy/helm-charts/**`) |
| 8 | medium  | no | yes(was) → **RESOLVED** (2026-05-19, P3-T-104 — `.github/workflows/e2e-kind.yml` spins up kind + cert-manager + pool-operator + ascend-npu-exporter-plus + demo-backend, seeds the pool hierarchy, waits for `NPUSlicePool.status.totalSlices > 0` and runs three Playwright API smokes) |
| 9 | trivial | no | no | **RESOLVED** (2026-05-19, P3-T-103 — dev stub retired; `deploy/dev/docker-compose.yaml`'s `ascend-npu-exporter-plus` service now runs the self-built exporter binary in simulator mode against the checked-in `testdata/simulator-set-a-small.json` seed) |

After ADR-0006, generator parity (#3), WS fastforward (#5),
GrafanaPanel unreachable UX (#6), and set-c-stress fixture (#4), all
six Phase 1 known issues have engineering resolutions. Phase 2 added
three new entries (#7-#9); P3-T-103 (2026-05-19) closes #7 (helm lint
CI) and #9 (dev stub retired in favour of `ascend-npu-exporter-plus`
in simulator mode), and P3-T-104 (2026-05-19) closes #8 (kind-based
real-cluster smoke workflow). All Phase 2 known issues now carry
engineering resolutions.

---

## Phase 2 entries — details

### #7 — helm lint not run on the Windows dev host

Severity: trivial · Status: **RESOLVED** (2026-05-19, P3-T-103).

`deploy/helm-charts/ascend-npu-exporter/` was developed without `helm`
on the Windows authoring box. Phase 2 authoring path:

1. `python -c "import yaml; yaml.safe_load(open(...))"` on `Chart.yaml`
   and `values.yaml`.
2. Hand inspection of `templates/*.yaml` against the helm template
   helpers in `_helpers.tpl`.

**Resolution**: `.github/workflows/helm-lint.yml` (lands with P3-T-103)
runs `helm lint --strict` + `helm template ci-render` on every chart
under `deploy/helm-charts/*/` whenever a PR touches that path or the
workflow itself. The community chart it was originally written against
was simultaneously retired in favour of the self-built
`deploy/helm-charts/ascend-npu-exporter-plus/`; the new workflow lints
the new chart from its first commit.

### #8 — Real-cluster E2E not yet automated

Severity: medium · Status: **RESOLVED** (2026-05-19, P3-T-104).

Phase 2 unit tests covered the K8s/Prometheus/CRD/ConfigMap sources
via `k8s.io/client-go/kubernetes/fake` + `httptest.NewServer` +
dynamic fake. Playwright ran against the mock-backed binary only.

**Resolution**: `.github/workflows/e2e-kind.yml` (lands with P3-T-104)
runs on every PR + dev push. The `e2e-kind` job:

1. spins up a kind v0.24 cluster (1 control-plane + 2 workers,
   `kindest/node:v1.30.4` for ValidatingAdmissionPolicy GA) via
   `tests/e2e/kind/kind-config.yaml`;
2. patches fake `huawei.com/Ascend910=8` capacity onto each worker
   (`kubectl patch node --subresource=status`) since kind config
   cannot set extended-resource capacity directly;
3. builds the pool-operator, demo-backend, and
   ascend-npu-exporter-plus images and `kind load`s them;
4. installs cert-manager v1.16 (helm), pool-operator (kustomize
   via `make deploy`), exporter-plus (helm with simulator JSON as
   a ConfigMap), and demo-backend (manifest with
   `mapping.pools=crd` + `mapping.workloads=k8s`);
5. seeds NodePool + NPUPool + NPUSlicePool (in `ocloud-system` per
   the T005 admission policy) + a mock workload Pod requesting
   `huawei.com/Ascend910=1`;
6. waits up to 150s for `NPUSlicePool.status.totalSlices > 0`
   (proves Reconcile loops T002-T004 fired);
7. runs three Playwright API smokes
   (`tests/e2e/specs/kind-smoke.spec.ts`):
   - `/api/v1/clusters` returns >= 1 entry (backend boot + router);
   - `/api/v1/workloads` surfaces the seeded `smoke-workload` Pod
     via k8s.Source;
   - direct scrape of `ascend-npu-exporter-plus:9100/metrics`
     (NodePort 30090) returns
     `ascend_npu_utilization_percent > 0` from the simulator seed.

The separate `e2e-mock-regression` job in the same workflow rebuilds
the demo-backend in mock mode and reruns the existing
`playwright.config.ts` mock suite to prove Phase 1 paths continue to
hold while the new real-cluster job lands.

Frontend rendering inside kind is intentionally not exercised — the
React build + nginx serve pipeline would add a node image build step
without coverage that the existing mock-backed Playwright suite
doesn't already provide. The kind smoke's unique value is proving the
*server-side* chain (Reconcile -> exporter -> backend datasources ->
JSON), which is what the API-level smokes assert. See
`tests/e2e/specs/kind-smoke.spec.ts` header comment for the full
trade-off.

### #9 — ascend exporter dev stub serves static metrics only

Severity: trivial · Status: **RESOLVED** (2026-05-19, P3-T-103).

The Phase 2 dev stub (`deploy/dev/ascend-exporter-stub/`) was an nginx
container that served a canned `/metrics` body — flat lines on Grafana.

**Resolution**: P3-T-103 retired both the stub directory and the
community helm chart. The `ascend-npu-exporter-plus` service in
`deploy/dev/docker-compose.yaml` (profile `ascend`) now runs the
self-built exporter binary in simulator mode against
`exporters/ascend-npu-exporter-plus/testdata/simulator-set-a-small.json`,
which mirrors `configs/mock-data/set-a-small/` (3 nodes × 8 NPUs = 24
devices, 12 slices). Numbers vary across NPUs/slices but are still
time-static (no synthetic load curves yet — that lands in Phase 4
when DCMI sources replace simulator mode).

For live numbers from real silicon, deploy the new helm chart
(`deploy/helm-charts/ascend-npu-exporter-plus/`) onto a cluster with
`huawei.com/Ascend910B`-labeled nodes; the chart's `simulator.enabled:
false` switch flips the exporter to its DCMI/npu-smi backend once
P4-T-2xx lands those sources.
