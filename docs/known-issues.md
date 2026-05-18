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

## 3. T013 generator drift

**Symptom**: `make gen-small` from `configs/mock-data/generator/`
produces a set-a-small **without** the fabric (`networkSwitches.json`
/ `networkLinks.json`) or pod `bindings` arrays — those fields were
hand-patched into the on-disk fixture by T013 but the Go generator
was never updated.

**Root cause**: T013 was scoped to schema + data, not the generator;
the canonical regeneration path drifted from the canonical fixture.

**Workaround**: treat the hand-edited set-a-small as canonical; do
NOT run `make gen-small` against it (it will overwrite). T307's two
new D6 workloads were also hand-added — same workaround applies.

**Resolution path**: extend
`configs/mock-data/generator/pkg/model/dataset.go` + the small preset
builder to emit fabric + bindings + D6 workloads. ~1 day's work,
mostly mechanical.

**Lands**: when a real fixture refresh is needed (e.g. T209 changes
NPU layout). Bundle with set-b-multi-site implementation since both
will exercise the same generator code paths.

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

## 5. WS event-driven E2E test skipped

**Symptom**: `tests/e2e/tests/topology.spec.ts` has one skipped test
("WS event drives status change") because the mock's real-time event
replay timing is sensitive to CI runner load.

**Root cause**: mock backend's event replay is real-time-scaled
(`events.json` declares offsets in seconds). On a slow CI runner the
3s event window passes before the frontend mounts its WS hook.

**Workaround**: assert WS handshake (status chip flips to `open`)
unconditionally; skip the event-arrival assertion.

**Resolution path**: expose the existing `FastForward` knob (already
present in `backend/pkg/datasource/mock/events.go` as
`model.StreamEventsOptions.FastForward`) via a `/ws/topology?fastforward=10`
query parameter. The E2E spec can then replay the 180s storyline in
~18s deterministically.

**Lands**: Phase 2 stretch; not blocking the demo.

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
| 3 | medium  | no | yes (needs cleanup before any large fixture refresh) | open |
| 4 | medium  | no | maybe (depends on whether stress fidelity matters early) | open |
| 5 | trivial | no | no | open |
| 6 | trivial | no | no | open |

After ADR-0006 only the generator drift (#3) remains as a Phase 2 entry
concern — bundle with the multi-site (set-b) generator work. #4-#6 are
optional polish; none are demo-blocking.
