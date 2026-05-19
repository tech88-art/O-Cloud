# O-Cloud Edge Cloud — Phase 1 Demo Script

> 5-page walkthrough of the demo backend + frontend running against the
> `set-a-small` mock fixture. Target audience: project reviewers seeing
> the platform for the first time. Run-time: ~10 minutes.

## Pre-flight

```bash
# From a clean repo checkout
./scripts/install.sh                       # ~5 min on fresh Ubuntu 22.04
# Or, if already built:
cd backend && ./bin/demo-backend.exe -c configs/config.dev.yaml &
cd frontend && pnpm dev --host 0.0.0.0 &
```

Open `http://localhost:3000`.

## Story arc

The events.json fixture replays an 250s narrative aligned to the demo
script below. The first 180s narrate the 5-page tour; the last 70s
showcase the **D6 NUMA+HCCS affinity** comparison (spec §D6).

---

## Page 1 — Overview · Topology (0-30s)

**Route**: `/overview`

**What to show**:

1. Three-pane layout: resource tree (left) / ReactFlow topology
   (center) / DetailPanel (right).
2. The tree autocompletes with **1 cluster (`cluster-prod-a-01`) → 3
   nodes (`worker-site-a-01..03`) → 24 NPUs (Ascend 910B)**. The
   degraded NPU on a-02-npu-5 surfaces with a warning tone — pre-seeded
   by the t=0s event.
3. **Click a node** in the left tree → ReactFlow highlights the same
   id, DetailPanel flips to the node card (CPU / Memory / NPU count /
   NUMA layout).
4. **Double-click an NPU node** → its slice subtree appears under it
   (T-108b behavior; collapsed by default to keep the canvas legible).
5. **Toggle "Include Fabric"** in the header → switch nodes + inter-node
   fabric-link edges appear (ADR-0004 / T-212).
6. **Toggle "Include Workloads"** → workload (gray ⚙) + pod (compact)
   nodes appear, with a dashed orange "PD" edge between
   `qwen-8b-pd-prefill-0` and `qwen-8b-pd-decode-0` (ADR-0005 / T-214).
   Both toggles ON: 125 nodes / 108 edges.

**Live data**: the WebSocket status chip shows `● open` and the
`lastEventAt` ticks as events.json events replay (`topology.update`,
`npu.statusChanged`, etc.).

---

## Page 2 — Workloads (30-60s)

**Route**: `/workloads`

**What to show**:

1. The AntD Table lists **12 workloads** spanning 4 types (inference /
   benchmark / training / inference) and 5 statuses (running /
   succeeded / pending / failed / unknown).
2. **Filter by status = "running"** — table shrinks; clear the filter
   to restore.
3. **Click the `qwen-8b-pd` row** → Drawer slides in. Show:
   - Pods section: `prefill-0` on worker-site-a-01, `decode-0` on
     worker-site-a-02 (inter-node PD pair).
   - Containers: image `mindie/vllm-ascend:0.11.0` + start command
     `vllm serve qwen/Qwen2.5-8B-Instruct --role prefill ...`.
   - Relations: a magenta Tag reading
     `qwen-8b-pd-prefill-0 → qwen-8b-pd-decode-0 (pd-pair)`.
   - Metrics link → routes to Metrics page with the workload pre-selected.

---

## Page 3 — Deploy (60-90s)

**Route**: `/deploy`

**What to show**:

1. **Four preset cards**: pi-3b / qwen-8b-pd / deepseek-20b / qwen-14b.
   Each card shows the resource budget (NPU count, vRAM, CPU/memory).
2. **Click the `pi-3b` card** → a 2-step wizard Modal opens:
   - Step 1 (mode): radio between `auto` (scheduler picks placement)
     and `manual` (operator picks node + NPU slice).
   - Step 2 (config): replicas + namespace inputs.
3. **Submit auto-mode** → POST `/api/v1/deploy` returns 201; the toast
   shows the deploy id; the page navigates to `/workloads` and the new
   `pi-3b-demo` workload appears (mock backend creates it inline).
4. (Optional) **Manual mode** — pick a specific NPU slice via the
   cascader → submit → same 201 path.

---

## Page 4 — Metrics (90-120s)

**Route**: `/metrics`

**What to show**:

1. **Five dashboard tabs**: cluster-overview / node-detail / npu-detail
   / workload-business / workload-resource.
2. **Cluster overview** is the default landing → Grafana iframe loads
   the dashboard with the cluster pre-selected.
3. **Switch to node-detail** → the `var-node` selector appears next to
   the tab strip; picking `worker-site-a-01` reloads the iframe with
   the variable in the URL.
4. **Switch to npu-detail** → cascader for node → NPU → slice surfaces.
   Picking any leaf injects `var-node` + `var-npu` + `var-slice` into
   the iframe (RFC-003 var-slice spec §F4a — slice-level granularity).
5. **Switch to workload-resource** → workload selector; pick
   `ai-inference/qwen-8b-pd` → dashboard shows TTFT / ITL / Throughput
   panels for the PD pair.

Note: if Grafana isn't running locally the iframe will show its blank
fallback — the toolbar and URL-construction logic are still
demoable.

---

## Page 5 — Logs (120-150s)

**Route**: `/logs`

**What to show**:

1. **Empty state** until a workload is picked.
2. Pick `ai-inference/qwen-8b-pd` from the dropdown → the LogViewer
   fills with the default 200-line tail (monospace, level-coloured).
   Lines come from the procedural generator
   (`backend/pkg/datasource/mock/logs.go`) so the content reads as a
   live inference workload (kv-cache eviction warnings, allreduce
   ring numbers, prefill batch sizes …).
3. Pick a specific **container** (prefill / decode) → tail re-fetches
   with the filter; only the chosen container's lines render.
4. Flip the **Live stream toggle** ON → WebSocket
   `/ws/logs/:ns/:name` opens, a new line lands every ~1s and
   appends to the bottom. Auto-scroll follows; manually scroll up to
   freeze.
5. Toggle a **failed workload** (`oom-test-model`) → the line mix
   shifts to ~30% ERROR + 20% WARN (the generator is
   workload-status-aware).

---

## D6 — NUMA+HCCS affinity comparison (150-220s)

**What to show**:

`set-a-small/events.json` after t=180s replays the D6 spec narrative
with two new Qwen-8B PD workloads:

| Workload                | Placement                              | TTFT (ms) | ITL (ms) | TPS (tok/s) |
|-------------------------|----------------------------------------|-----------|----------|-------------|
| `qwen-8b-pd-affinity`   | a-03-npu-0 + a-03-npu-1 (HCCS-0/NUMA-0)| ~50       | ~14      | ~1050       |
| `qwen-8b-pd-cross-numa` | a-01-npu-0 (NUMA-0) + a-01-npu-4 (NUMA-1)| ~89     | ~25      | ~630        |

The events.json carries 4 `workload.statusChanged` envelopes with
`metrics-tick` reason that surface the numbers above. The WS feed
drives the values into the UI's `lastEventAt` indicator; future
PHASE-2 work will wire them into a dedicated affinity-comparison
Grafana panel (T209 / T306 follow-up).

**Talking points**:
- Same model, same node, same NPU generation — only NUMA placement
  differs.
- ~60% TTFT regression + ~75% throughput regression when prefill and
  decode straddle the NUMA boundary.
- HCCS group co-location matters because the prefill→decode KV-cache
  hop uses the HCCS ring; cross-HCCS forces a UB-fabric detour.

---

## Wrap-up

Reviewer takeaways:

1. Five pages cover the demo end-to-end against mock data.
2. Same binary swaps datasets via config alone — `datasources.mock.path:
   set-b-small` would have run a different fixture without recompile
   (P1-T-303, set-b dataset deferred to Phase 2 expansion).
3. Phase 2 replaces the mock source with real K8s / Prometheus / CRD
   sources without touching the frontend.

Phase 1 final tag: `phase-1-complete`. Recent merge: `9abf56a` (T307).

---

## Phase 2 appendix — real-K8s demo (added 2026-05-18, P2-T-107)

The Phase 1 demo runs against mock JSON; Phase 2 flips the same UI
onto live K8s + Prometheus + CRD + ConfigMap data without rebuilding
the frontend. Use this appendix when the demo machine has a real (or
`kind`/K3s) cluster reachable via kubeconfig.

### Prereqs

| Component | Why |
|---|---|
| `kubectl` + valid `KUBECONFIG` | k8s.Source talks to the apiserver |
| `helm` v3 | only required for `--with-prometheus` |
| K3s ≥ 1.28 / `kind` ≥ 0.22 | tested matrix |
| (optional) Ascend node | the dev nginx stub fills in if absent |

### One-liner installer

```bash
./scripts/install.sh --with-prometheus
```

This:

1. Builds backend + frontend (or `--image-only` to skip).
2. Brings up the docker-compose stack (frontend on 3000, demo backend
   on 8080, prometheus on 9090, grafana on 3001 — Phase 1 path).
3. Helm-installs `kube-prometheus-stack` into the current kubeconfig
   context (P2-T-106) — Grafana NodePort 30001, anon viewer + iframe
   embedding allowed.
4. Helm-installs `ascend-npu-exporter` (community v6.0.0) — DaemonSet
   that picks up nodes labelled `huawei.com/Ascend910B=true`.

Re-runnable: helm `upgrade --install` is idempotent.

### Config swap (Phase 1 promise honoured)

`backend/configs/config.yaml` mapping block:

```yaml
mapping:
  clusters: k8s          # was mock — now reads from apiserver
  workloads: k8s         # Deployments + StatefulSets + Jobs
  metrics: prometheus    # PromQL templates against kps Prometheus
  pools: crd             # NPUSlicePool + ResolveSlicesForNPUs
  presets: configmap     # ocloud-system/ocloud-presets
  topology: aggregator   # k8s + crd + fabric joined server-side
datasources:
  k8s:
    enabled: true
    kubeconfig: ""       # empty = in-cluster / KUBECONFIG env
  prometheus:
    enabled: true
    url: "http://kube-prometheus-stack-prometheus.monitoring.svc.cluster.local:9090"
  crd:
    enabled: true
    namespace: "ocloud-system"
  configmap:
    enabled: true
    namespace: "ocloud-system"
    name: "ocloud-presets"
  fabric:
    enabled: true
    path: "/etc/ocloud/fabric.yaml"
  mock:
    enabled: false       # retained for E2E / dev — flip on if you
                         # want the demo dataset alongside
```

Restart the backend pod (no rebuild required).

### Quick smoke

```bash
# Cluster source
curl -s http://localhost:8080/api/v1/clusters | jq '.[0].id'
#  → "kubernetes" (or your cluster-info ConfigMap's id)

# NPU listing (will be empty until ascend-device-plugin labels nodes)
curl -s http://localhost:8080/api/v1/nodes/<node>/npus | jq '.[].model'

# Prometheus template
curl -s 'http://localhost:8080/api/v1/metrics/query?template=npu_utilization&node=<node>' | jq

# CRD-backed slice pools
curl -s http://localhost:8080/api/v1/pools/npuslicepools | jq '.[].name'

# Preset catalog (driven by ocloud-system/ocloud-presets ConfigMap)
curl -s http://localhost:8080/api/v1/presets | jq '.[].id'
```

### Watch the live stream

The WebSocket feed switches from `events.json` replay (mock) to live
Kubernetes Watch (`k8s.Source.StreamEvents`) automatically when
`mapping.events: k8s`. Visit the Workloads page and create a
Deployment via `kubectl create`; the new row appears within ~1s with
the `workload.created` envelope.

### Limitations (Phase 2 cap)

- Logs follow a single pod — multi-replica log multiplexing is
  Phase 3+.
- Fabric data still comes from a static YAML (ADR-0007); LLDP / SONiC
  discovery is Phase 3+.
- Cache eviction is per-request; LRU / TTL tuning is Phase 3+ (see
  known-issues #8 for the real-cluster E2E gap).

Phase 2 final tag: `phase-2-complete`. Recent merge: see
`docs/checkpoint-phase2.md` for the per-task commit map.

---

## Phase 3 appendix — pool-operator + exporter-plus on kind (added 2026-05-19, P3-T-107)

> **Why a kind walkthrough**: Phase 3 added the first three control
> loops (pool-operator Reconcile × 4 + ascend-npu-exporter-plus +
> ValidatingAdmissionPolicy). They each need a real K8s API server to
> exercise — envtest covers unit-level Reconcile but doesn't speak
> Helm or ServiceMonitor. This appendix walks through the same flow
> the `.github/workflows/e2e-kind.yml` job runs on every PR (T104).

### Pre-flight (host)

```bash
# install kind v0.24+ + helm v3.16+ + kubectl 1.30+ (any K8s 1.30+
# release works; e2e-kind.yml pins kindest/node:v1.30.4)
kind version
helm version
kubectl version --client
```

### Bring up the cluster

```bash
# from repo root
bash tests/e2e/kind/install.sh build-images
bash tests/e2e/kind/install.sh up
```

`install.sh up` does, in order:

1. `kind create cluster --config tests/e2e/kind/kind-config.yaml`
   (1 control-plane + 2 workers, worker labels `huawei.com/Ascend910B=true`
   + `Ascend910-Health=Healthy` + `site=site-a`/`role=edge`)
2. `kubectl patch node --subresource=status` injects fake
   `huawei.com/Ascend910=8` capacity into each worker (kind can't set
   extended resources via config, only labels)
3. `helm install cert-manager jetstack/cert-manager` (Phase 5 PD Router
   will need it; pre-installing here exercises the dependency path)
4. `make deploy IMG=ocloud/pool-operator:e2e` inside
   `operators/pool-operator/` — `kubectl apply -k config/default`
   under the hood; lands the 4 CRDs + VAP + RBAC + manager Deployment
5. `helm install ascend-npu-exporter-plus deploy/helm-charts/ascend-npu-exporter-plus/`
   into `monitoring` namespace, simulator JSON mounted via ConfigMap
6. `kubectl apply -f tests/e2e/kind/manifests/demo-backend.yaml`
   (Deployment + Service + RBAC; backend wired with
   `mapping.pools=crd`, `mapping.nodes=k8s`, `mapping.workloads=k8s`)
7. Extra `NodePort` Service for the exporter on `:30090` so the host
   can scrape `/metrics` directly (the chart's Service is ClusterIP)

### Seed the pool hierarchy and a workload

```bash
bash tests/e2e/kind/seed-resources.sh
```

Applies:
- `NodePool smoke-nodepool` (selector `site=site-a, role=edge`)
- `NPUPool smoke-npupool` (parents NodePool, model `Ascend910B`)
- `NPUSlicePool smoke-pool` in `ocloud-system` (strategy
  `FixedTemplate`, single `vir04` template, AICoreCount=4)
- 1 mock `smoke-workload` Pod requesting `huawei.com/Ascend910=1`
  with the `npu.huawei.com/slice-bindings` annotation that Phase 5
  PD Router (ADR-0008) will eventually write

### Watch Reconcile populate status (~5-10s)

```bash
# NPUPool status (T003: TotalNPUs + HealthyNPUs + AllocatedNPUs)
kubectl get npupool smoke-npupool -o yaml | yq '.status'
# Expected: totalNPUs: 16 (2 workers × 8 fake NPUs), healthyNPUs: 16,
# allocatedNPUs: 1 (from smoke-workload's request), HCCSDiscovered=False
# Reason=PendingPhase6 condition

# NPUSlicePool status (T002: TotalSlices from template × NPU count)
kubectl get -n ocloud-system npuslicepool smoke-pool -o yaml | yq '.status'
# Expected: totalSlices: 128 (32 aiCores/NPU ÷ 4 template aiCores × 16
# NPUs), availableSlices: 128, Ready=True Reason=Reconciled

# NodePool status (T004: Nodes list + total CPU + total Memory)
kubectl get nodepool smoke-nodepool -o yaml | yq '.status.nodes'

# ClusterPool placeholder (T105: PhaseDeferred=True)
kubectl apply -f - <<'EOF'
apiVersion: ims.ocloud.edge.example.com/v1alpha1
kind: ClusterPool
metadata: { name: smoke-cp }
spec:
  clusters: []
  syncPolicy: Push
EOF
kubectl get clusterpool smoke-cp -o yaml | yq '.status.conditions'
# Expected: PhaseDeferred=True, Reason=WaitingForKarmada
```

### Verify the VAP (T005)

```bash
# Reject: NPUSlicePool in default namespace (no bypass label) is denied
kubectl apply -f - <<'EOF' || echo "rejected as expected"
apiVersion: ims.ocloud.edge.example.com/v1alpha1
kind: NPUSlicePool
metadata: { name: rejected-pool }
spec:
  npuPoolRef: { name: smoke-npupool }
  strategy: FixedTemplate
  fixedTemplates: [ { name: vir04, aiCoreCount: 4, memoryMiB: 16384 } ]
EOF
# Expected: "NPUSlicePool must be created in ocloud-system namespace
# ... npu.huawei.com/multi-tenancy-bypass=true to opt out"

# Bypass label opts out
kubectl apply -f - <<'EOF'
apiVersion: ims.ocloud.edge.example.com/v1alpha1
kind: NPUSlicePool
metadata:
  name: bypass-pool
  labels: { npu.huawei.com/multi-tenancy-bypass: "true" }
spec:
  npuPoolRef: { name: smoke-npupool }
  strategy: FixedTemplate
  fixedTemplates: [ { name: vir04, aiCoreCount: 4, memoryMiB: 16384 } ]
EOF
```

### See live metrics from the exporter

```bash
# Direct scrape (NodePort 30090, set by install.sh)
curl -s http://localhost:30090/metrics | grep -E '^(ascend_npu|ascend_slice|exporter_)' | head -20
# Expected (24 NPU samples drawn from
# exporters/.../testdata/simulator-set-a-small.json with sine-wave
# perturbation across scrapes):
#   ascend_npu_utilization_percent{npu_id="...",node="...",model="Ascend910B"} 65.4
#   ascend_npu_memory_used_bytes{npu_id="...",node="..."} 12884901888
#   ascend_npu_hbm_bandwidth_bytes_per_second{npu_id="...",node="..."} 5.49e11
#   ascend_slice_aicore_count{slice_id="...",npu_id="...",template="vir04"} 4
#   ascend_slice_allocated_to_pod{slice_id="...",namespace="ocloud-system",pod="qwen-8b-prefill-0"} 1
#   exporter_build_info{version="...",commit="...",go_version="..."} 1
```

### Curl the backend (NodePort 30080)

```bash
curl -s http://localhost:30080/api/v1/clusters | jq '.[0].id'
curl -s http://localhost:30080/api/v1/workloads | jq 'map(.name) | contains(["smoke-workload"])'
curl -s http://localhost:30080/api/v1/pools/npuslicepools | jq '.[] | {name, totalSlices: .status.totalSlices}'
```

### Limitations (Phase 3 cap)

- ClusterPool is a passive placeholder (PhaseDeferred condition only);
  Karmada multi-cluster sync lands Phase 9 per architecture.md §13.
- Backend has no `/metrics` Prometheus self-endpoint yet (T008 scoped
  to Must-Have; T008b / T103 follow-up will add `prometheus/client_golang`
  + mount `/metrics` outside `/api/v1`).
- PD Router admission webhook is **design only** (ADR-0008); impl
  lands Phase 5 alongside inference-operator.
- DRA driver path is Phase 4 (architecture.md §1.3 / ADR-0001 v2 —
  K8s 1.34 GA'd 2025-09; gating factor is KubeEdge ≤ v1.22 DRA support
  + no official Ascend DRA driver).

### Teardown

```bash
bash tests/e2e/kind/install.sh down
```

Phase 3 final tag: `phase-3-complete`. Per-task commit map:
`docs/checkpoint-phase3.md`. The same flow runs unattended in
`.github/workflows/e2e-kind.yml` on every PR.

---

## Phase 4 demo appendix — npu-dra-driver + backend metrics

> Adds the **simulator-first NPU DRA driver** and the **demo-backend
> `/metrics` Prometheus self-endpoint** on top of the Phase 3 kind
> smoke. Same simulator-first stance — no Ascend silicon required.

### What's new in Phase 4 (vs Phase 3 demo)

| Surface                                  | Phase 3                       | Phase 4                                                                            |
| ---------------------------------------- | ----------------------------- | ---------------------------------------------------------------------------------- |
| `operators/npu-dra-driver/`              | does not exist                | scaffold + simulator publisher + claim controller skeleton + Helm chart            |
| `resource.k8s.io/v1beta1` ResourceSlices | none                          | 3 slices × 8 devices (set-a-small/npus.json) under driver `npu.ocloud.edge.example.com` |
| pool-operator NPUSlicePool.status        | TotalSlices / AvailableSlices | + ResourceSlicesObserved (cross-watch — counts slices owned by npu-dra-driver)     |
| Backend `/metrics`                       | absent                        | gin engine root (outside `/api/v1`) + 3 ocloud_backend_* counters + go_/process_*  |
| `operators/inference-operator/`          | does not exist                | scaffold + ModelService CRD types (Phase 5 controller body)                        |
| Phase docs                               | ADR-0001 v2                   | + ADR-0001 v3 (dual-path) + ADR-0009 (npu-dra-driver design) + cann-driver-matrix  |

### Bring up

```bash
# Same install.sh up flow as Phase 3, now with npu-dra-driver added.
bash tests/e2e/kind/install.sh up
bash tests/e2e/kind/install.sh build-images
```

### Verify NPU DRA publication (P4-T-104 assertion)

```bash
kubectl get resourceslices
# expected: 3 ResourceSlices, driver=npu.ocloud.edge.example.com, ≥8 devices each

# CI assertion script (also runs in .github/workflows/e2e-kind.yml):
bash tests/e2e/kind/dra_publish_test.sh
```

### Verify pool-operator ↔ npu-dra-driver cross-observation (P4-T-102)

```bash
kubectl -n ocloud-system get npuslicepool smoke-pool -o jsonpath='{.status.resourceSlicesObserved}'
# expected: 3 (one ResourceSlice per simulated node)
```

### Verify backend /metrics + ocloud_backend_* counters (P4-T-106)

```bash
# Warm the dispatch counter.
for ep in clusters nodes presets workloads; do
  curl -fsS "http://localhost:30080/api/v1/$ep" >/dev/null
done

curl -fsS http://localhost:30080/metrics | grep ocloud_backend_
# expected: at least 4 lines like
# ocloud_backend_dispatch_calls_total{datasource="mock",endpoint="clusters"} 1
# ocloud_backend_dispatch_calls_total{datasource="mock",endpoint="nodes"} 1
# ...

# CI assertion script:
BACKEND_URL=http://localhost:30080 bash tests/e2e/kind/backend_metrics_test.sh
```

### What's NOT demonstrated in Phase 4

- Real Ascend silicon (Phase 7 — see `docs/cann-driver-matrix.md` §4)
- Real ResourceClaim allocation (Phase 5 per ADR-0009 §5)
- inference-operator ModelService Reconcile (Phase 5)
- PD Router mutating admission webhook (Phase 5 per ADR-0008)
- HCCS-ring topology-aware scheduling (Phase 6 scheduler-plugin)
- Karmada multi-cluster ModelService sync (Phase 9)

### Teardown

```bash
bash tests/e2e/kind/install.sh down
```

Phase 4 final tag: `phase-4-complete`. Per-task commit map:
`docs/checkpoint-phase4.md`. The same flow runs unattended in
`.github/workflows/e2e-kind.yml` on every PR with assertions for
ResourceSlice publication (P4-T-104) and backend metrics (P4-T-106).
