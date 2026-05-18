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

## 4. set-c-stress (800 NPU) FPS test deferred

**Symptom**: ADR-0005 §"推翻条件" says: "set-c-stress (800 NPU) FPS
< 15 → fall back to per-workload mini-graph". Phase 1 never built
set-c-stress — it sits as a `preset_stress.go` stub in the generator
that returns `errStressNotImplemented`.

**Root cause**: set-c-stress is large enough to require a fresh
generator pass; T307 scope kept set-a-small.

**Workaround**: hand-test against set-a-small (which has at most 125
nodes / 108 edges fully expanded — well under any sane FPS budget).

**Resolution path**: generate set-c-stress + run a 30s manual scroll
benchmark with the topology toggles all ON. If FPS drops below 15,
trigger ADR-0005's fallback (mini-graph in WorkloadDetailDrawer).

**Lands**: Phase 2 stretch, alongside the multi-site (set-b)
generator.

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

## 6. `data:` URL Grafana fallback when Grafana isn't running

**Symptom**: When the Grafana service isn't reachable, the iframe
falls through to a placeholder src and the panel renders blank.

**Root cause**: `frontend/src/components/GrafanaPanel/index.tsx`
attempts a HEAD probe, but in CORS-restricted dev environments the
probe fails silently and the iframe is given a placeholder URL.

**Workaround**: bring Grafana up via the `deploy/dev/docker-compose.yaml`
profile (it ships pre-provisioned with the 5 Phase-1 dashboards).

**Resolution path**: surface a more obvious "Grafana not reachable"
state in the panel (currently it's just blank). Cosmetic.

**Lands**: opportunistic frontend polish.

---

## Severity ranking

| # | Severity | Blocking demo? | Blocking Phase 2? | Status |
|---|---|---|---|---|
| 1 | trivial | no | no | **resolved** (split helpers, lint 0 warnings) |
| 2 | medium  | no | yes(was) → **resolved** (ADR-0006 contract regen) |
| 3 | medium  | no | yes(was) → **resolved** (generator emits fabric + bindings + D6 workloads) |
| 4 | medium  | no | maybe (depends on whether stress fidelity matters early) | open |
| 5 | trivial | no | no | **resolved** (WS `?fastforward=N` + frontend `?ffwd=` opt-in; E2E unskipped) |
| 6 | trivial | no | no | open |

After ADR-0006 + generator parity, no Phase 2 entry blockers remain.
#4-#6 are optional polish; none are demo-blocking.
