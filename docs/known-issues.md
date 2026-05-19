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
| 7 | trivial | no | no | **accepted** (Phase 3 — helm lint will run in Linux CI; values.yaml + Chart.yaml validated via python-yaml on the Windows dev host) |
| 8 | medium  | no | yes(Phase 3) | **accepted** (real-cluster E2E suite is Phase 3 scope; Phase 2 verifies via fake clientset + Playwright against mock backend) |
| 9 | trivial | no | no | **accepted** (Phase 2 ascend exporter dev stub serves static metrics — real silicon required for live numbers; community v6.0.0 used in production helm chart) |

After ADR-0006, generator parity (#3), WS fastforward (#5),
GrafanaPanel unreachable UX (#6), and set-c-stress fixture (#4), all
six Phase 1 known issues have engineering resolutions. Phase 2 added
three new entries (#7-#9), all accepted with explicit owners and Phase
3 follow-ups.

---

## Phase 2 entries — details

### #7 — helm lint not run on the Windows dev host

Severity: trivial · Status: **accepted** (Phase 3 CI work).

`deploy/helm-charts/ascend-npu-exporter/` was developed without `helm`
on the Windows authoring box. Authoring path:

1. `python -c "import yaml; yaml.safe_load(open(...))"` on `Chart.yaml`
   and `values.yaml`.
2. Hand inspection of `templates/*.yaml` against the helm template
   helpers in `_helpers.tpl`.

Phase 3 work: GitHub Action job `helm-lint` runs `helm lint` + `helm
template` on every PR touching `deploy/helm-charts/**`. Tracked under
P3-T-XXX (placeholder — owner TBD at Phase 3 kickoff).

### #8 — Real-cluster E2E not yet automated

Severity: medium · Status: **accepted** (Phase 3 scope).

Phase 2 unit tests cover the K8s/Prometheus/CRD/ConfigMap sources via
`k8s.io/client-go/kubernetes/fake` + `httptest.NewServer` + dynamic
fake. Playwright still runs against the mock-backed binary.

A real-K3s E2E pipeline (`kind`-based, ascend-npu-exporter stub + the
five frontend pages) is desirable but explicitly Phase 3 scope. Phase
2 ships with the manual verification path documented in
`docs/demo.md` §Phase 2 appendix.

### #9 — ascend exporter dev stub serves static metrics only

Severity: trivial · Status: **accepted** (operator-aware).

Without an Ascend host the dev stub
(`deploy/dev/ascend-exporter-stub/`) serves a canned `/metrics`
response — flat lines on Grafana panels. The production helm chart
(`deploy/helm-charts/ascend-npu-exporter`) brings up the real
community v6.0.0 exporter and produces live numbers on hosts with
`huawei.com/Ascend910B` capacity.

Operators using the demo box for live data need either:
- A real Ascend node + the helm chart, OR
- Phase 3+ `ascend-npu-exporter-plus` (self-built) once it ships.
