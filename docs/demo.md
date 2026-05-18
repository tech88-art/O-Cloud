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
