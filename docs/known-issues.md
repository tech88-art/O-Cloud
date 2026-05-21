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

### #10 — inference-operator helm install order: cert-manager first

Severity: low · Status: **OPEN** (2026-05-19, P5-T-101).

The inference-operator helm chart's `certManager.enabled=true` default
renders a cert-manager `Certificate` + `Issuer`. The chart assumes
cert-manager (v1.16+) is **pre-installed** in the cluster — it is NOT
bundled as a subchart dependency (the alternative was rejected to keep
version pinning out of this chart's responsibility).

**Required ordering** for `helm install` in a fresh cluster:

```bash
# 1. Install cert-manager (any supported method)
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.0/cert-manager.yaml

# 2. Wait for cert-manager pods Ready (typically 30-90s in kind)
kubectl wait --for=condition=Available --timeout=120s -n cert-manager deploy --all

# 3. Install inference-operator
helm install inference-operator deploy/helm-charts/inference-operator \
  --namespace ocloud-system --create-namespace
```

If step 1 is skipped or step 2 doesn't complete before step 3, the
inference-operator Certificate creation will fail with `no matches
for kind "Certificate" in version "cert-manager.io/v1"`.

**Workaround**: set `--set certManager.enabled=false` and bring your
own TLS cert plumbing for the PD Router webhook (T102+); the webhook
defaults to failurePolicy=Fail so disabling cert wiring effectively
disables the webhook until you wire up the secret manually.

**Proposed resolution**: Phase 5 T106 kind smoke includes the
cert-manager install step in `tests/e2e/kind/phase5/install.sh`. A
future Phase 6+ task may introduce a chart-level dependency or a
helmfile orchestrator if cluster operators report friction.

---

### #11 — scheduler-plugin runs as a SECOND scheduler — Pods must opt in via schedulerName

Severity: low · Status: **RESOLVED** (2026-05-20, P7-T-003) — inference-operator deployment_builder auto-stamps `spec.schedulerName=npu-scheduler` on every PD-pair Pod template; operators opt out via `ms.Spec.SchedulerOverride`. The OPEN-state narrative below is retained for the historical record (it remains accurate for Pods created OUTSIDE the inference-operator path · e.g. raw `kubectl apply` of a Deployment).

The `scheduler-plugin` helm chart (`deploy/helm-charts/scheduler-plugin/`)
deploys a custom kube-scheduler binary as a SECOND scheduler in the
cluster, registered under profile name `npu-scheduler` (configurable
via `profileName` in values.yaml). The default-scheduler is
intentionally unaffected — Pods must EXPLICITLY set
`spec.schedulerName: npu-scheduler` to get HCCS-aware placement.

This is per ADR-0010 §1 (multi-scheduler form, not default-scheduler
patch) to keep blast radius bounded — a bug in HCCS scoring cannot
break the rest of the cluster's scheduling.

**Observable symptom** when forgotten: Pods schedule successfully but
WITHOUT HCCSTopology Filter/Score evaluation. No error surfaces —
just sub-optimal placement. Operators check by inspecting Pod
events: messages from `Scheduler` source with `Component=
default-scheduler` mean the wrong scheduler picked the node.

**Required action** for HCCS-aware placement:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-vllm-ascend-pod
spec:
  schedulerName: npu-scheduler  # ← required
  # ... rest of Pod spec
```

For inference-operator-managed Deployments, set this in the Pod template
inside `ms.spec.template.spec.schedulerName` (Phase 6 T105 inference-
operator polish may stamp this automatically; gated on T105 decision).

**Proposed resolution**: P6-T-105 evaluates whether inference-operator
deployment_builder should default `spec.schedulerName=npu-scheduler`
when the chart is enabled. **P7-T-003** lands the
inference-operator deployment_builder auto-stamp (entry pending);
known-issues #11 will flip RESOLVED at that commit.

---

### #12 — NumaAffinity plugin wrap re-deferred from Phase 7 → Phase 8 → Phase 9 (K8s baseline bump prerequisite)

Severity: low · Status: **OPEN** (2026-05-21 update · P8-T-002 user decision: stay K8s 1.32 baseline · re-evaluated Phase 9 W1 entry).

> 🆕 **2026-05-21 update (P8-T-002 · K8s baseline stay decision)**:Phase 8 W1 entry P8-T-002 re-WebFetch findings(upstream snapshot 2026-05-21):
> - sched-plugins latest = **v0.34.7**(2026-04-20)· v0.35.x / v0.36.x **未发布**
> - kindest/node **v1.36 镜像不存在** in kind v0.31(2025-12-18 最新)· kindest/node 最高 prebuilt = v1.35.0
> - K8s 1.36.1 GA upstream(2026-05-12)but cluster-tooling lag
>
> **用户决策**(2026-05-21 chat):Phase 8 不主动 bump K8s baseline · **stay K8s 1.32**(降低本期风险)。
>
> 影响:NumaAffinity wrap upgrade **deferred from Phase 8 → Phase 9**(known-issues #12 maintains OPEN · severity low)。下次评估在 **Phase 9 W1 entry**(届时 sched-plugins v0.35+ / kindest/node v1.36+ 若 GA · 重新评估)。
>
> 用户可在 Phase 8 内任意 chat 启动 mid-phase bump 决策(本 known-issues 政策 default = defer)。

---

**历史背景**:

The NumaAffinity scheduler plugin (`operators/scheduler-plugin/internal/plugins/numa/`)
ships a Name()-only placeholder since Phase 6 T006 because at Phase 6
entry the upstream `sigs.k8s.io/scheduler-plugins/pkg/noderesourcetopology`
release line referenced `framework.GVK` — a symbol K8s 1.32 removed
(per ADR-0010 §3 T006 落地状态 note).

**Phase 7 P7-T-002 re-attempt outcome (2026-05-20)**: doc-only
fallback. v0.32.7 (2024-08-06) is GA along with v0.33.5 / v0.34.7;
upstream's `framework.GVK` reference IS removed in v0.32+. However,
attempting `go get sigs.k8s.io/scheduler-plugins@v0.32.7` + bumping
the replace block uniformly to v0.32.7 then `go mod tidy` surfaces
`k8s.io/apimachinery v0.32.7` missing newer `pkg/api/{safe,operation,validate}`
packages along the import chain
`cmd/main.go → kube-scheduler/app → pkg/scheduler → pkg/apis/core/{validation,v1}`.
These apimachinery packages are post-v0.32 additions (likely v1.33+);
our K8s 1.32 baseline pin (ADR-0010 §1) cannot absorb the transitive
expectation without a broader baseline bump.

Revert is clean (single `git checkout go.mod go.sum` discards the
attempted upgrade with no commit on the upgrade path).

**Observable symptom** when forgotten: `kube-scheduler --help`
lists `NumaAffinity` factory but the chart's KubeSchedulerConfiguration
intentionally OMITS it from filter/score enabled lists —
no NUMA-aware scoring occurs for `npu-scheduler` profile Pods.

**Workaround** for operators wanting NUMA-aware scheduling today:
run upstream sched-plugins binary as a SECOND second-scheduler alongside
our HCCSTopology+Binpack binary (functional today, more operational
overhead — two extra schedulers running side-by-side).

**Proposed resolution** (Phase 9 candidate · was Phase 8 before P8-T-002 user decision):
1. Bump K8s baseline from 1.32 → 1.33 / 1.34 / 1.35 / 1.36 — coordinate with
   kind smoke `kindest/node` baseline (currently `v1.32.x` per
   P5-T-114)· Phase 9 W1 entry re-WebFetch sched-plugins / kindest/node
   release tracker · 拍 target minor
2. Bump sched-plugins to matching minor — at 2026-05-21 上游最新 v0.34.7
3. Replace `plugin.go` placeholder body with `return nrt.New(ctx,
   args, h)` wrap pattern (mirror hccs/args.go parseArgs structure
   for NumaAffinityArgs · pre-construct upstream
   `NodeResourceTopologyMatchArgs` with `LeastAllocated` strategy +
   cpu/memory weight=1 defaults)
4. Add 3 sanity tests per phase7-plan §3 T002 acceptance pattern
5. Update chart KubeSchedulerConfiguration to enable
   NumaAffinity in profile + `numaAffinity.enabled=true` chart default

Cross-references: `operators/scheduler-plugin/DESIGN.md` §5.2 +
ADR-0010 §1 §3 T006 + P7-T-002 attempt notes + phase7-plan.md §3 T002
+ phase8-plan.md §3 T002 + `docs/devlog/phase-7-t002.md` + `docs/devlog/phase-8-t002.md`.

---

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
