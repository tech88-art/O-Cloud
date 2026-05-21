# scheduler-plugin — module detailed design

> Status: Phase 6 T004 — HCCSTopology Filter body landed alongside the
> Phase 6 T002 scaffold + ADR-0010 design freeze. T005 (Score) / T006
> (NUMA wrap) / T007 (Binpack) / T008 (integration tests) extend the
> module along the dimensions documented below.

## 1. 架构概览

```
┌───────────────────────────────────────────────────────────────────┐
│                       K8s cluster                                 │
│                                                                   │
│  ┌────────────────────────┐                                       │
│  │ User / inference-      │                                       │
│  │ operator creates Pod   │                                       │
│  │ with spec.schedulerName│                                       │
│  │ = "npu-scheduler"       │                                       │
│  └──────────┬─────────────┘                                       │
│             │                                                     │
│             ▼ binding cycle                                       │
│  ┌────────────────────────────────────────────────────────┐       │
│  │  kube-scheduler (npu-scheduler profile)                │       │
│  │  Plugin chain (KubeSchedulerConfiguration):            │       │
│  │  - PreFilter / Filter / PreScore / Score / Reserve     │       │
│  │  - Our profile enables:                                 │       │
│  │      HCCSTopology      (Filter + Score)                 │       │
│  │      NumaAffinity      (Filter + Score; wrap upstream) │       │
│  │      Binpack            (Score; default disabled)      │       │
│  └──────────┬──────────────────────────────────────────────┘       │
│             │ reads                                                │
│             ▼                                                     │
│  ┌────────────────────────┐    ┌────────────────────────────┐     │
│  │ ResourceSlice          │    │ NPUSliceAllocation         │     │
│  │ (npu-dra-driver pubs)  │    │ (npu-dra-driver mirrors)   │     │
│  │ - hccs_ring attr       │    │ - claimRef → sibling Pods  │     │
│  │ - numa_node attr        │    │   per ModelService label   │     │
│  │ - health attr          │    │                            │     │
│  └────────────────────────┘    └────────────────────────────┘     │
│        ▲ T004 Filter                  ▲ T005 Score                │
│        │                              │                            │
│  ┌─────┴──────────────────────────────┴──────┐                    │
│  │    HCCSTopology plugin (this module)      │                    │
│  │    args.go     parses HCCSTopologyArgs    │                    │
│  │    types.go    ResourceSlice helpers       │                    │
│  │    filter.go   Filter (T004) — ADR-0010 §2 │                    │
│  │    score.go    Score (T005 — pending)     │                    │
│  └────────────────────────────────────────────┘                    │
└───────────────────────────────────────────────────────────────────┘
```

The plugin runs inside a custom kube-scheduler binary built from
`operators/scheduler-plugin/cmd/main.go` (Phase 6 T002 scaffold). The
default scheduler is unaffected. Pods opt in via
`spec.schedulerName: npu-scheduler`.

## 2. 接口契约

### 2.1 Args schema (HCCSTopologyArgs · args.go)

```go
type HCCSTopologyArgs struct {
    metav1.TypeMeta  `json:",inline"`
    Weight           int32              // [0..100], default 5
    PreferAnnotation string             // default "npu.huawei.com/preferred-hccs-ring"
    FailIfMissing    bool               // default false (permissive)
    Adjacency        map[string][]int32 // T005 Score; default = 8-card ring (T008)
}
```

Encoded inside `KubeSchedulerConfiguration.profiles[*].pluginConfig`
when the chart (T101) renders the ConfigMap. Defaults applied by
`parseArgs` when args fields are zero / nil.

### 2.1.1 HCCS Adjacency map (Phase 7 P7-T-008 · operative)

Phase 6 T006 + T101 shipped `adjacency: {}` chart default — Score
degenerated to binary 100/30 (same vs disjoint) because no ring
adjacency was known. Phase 7 P7-T-008 (ADR-0010 §256 closure + ADR-0011
follow-on) flips the chart default to the **8-card 910B ring-of-rings
topology** `0↔1↔2↔3↔0`:

```yaml
# deploy/helm-charts/scheduler-plugin/values.yaml
hccsTopology:
  adjacency:
    "0": [1, 3]
    "1": [0, 2]
    "2": [1, 3]
    "3": [2, 0]
```

**Go helpers** (`operators/scheduler-plugin/internal/plugins/hccs/adjacency.go`):

```go
func DefaultAdjacency910B8Card() map[string][]int32    // chart default
func BuildAdjacency(spec map[string][]int32) (map[int32][]int32, error)
                                                       // chart pre-flight validation
var ErrMalformedAdjacency error                        // sentinel for non-int keys, negative
                                                       // values, self-loops
```

BuildAdjacency validates:
- Every key parses as a non-negative int32 (rejects "abc", "-1")
- Every value is a non-negative int32 (kubebuilder int32 type already
  enforces; BuildAdjacency defensively re-validates)
- No self-loops (`"3": [3, ...]` rejected; ring 3 adjacent to itself
  is a misconfiguration — Score's 100 tier already handles same-ring)

Operators with a different physical topology override `adjacency`
chart value. Setting `adjacency: {}` explicitly (or `adjacency: null`
via `--set`) returns to the Phase 6 binary 100/30 behavior — verified
by the chart `helm template --set hccsTopology.adjacency=null`
rendering (no adjacency block in ConfigMap).

**Score impact**: when adjacency is non-empty, Score grades nodes in
4 tiers (per §3.3):

| Tier | Score | Condition                                              |
|------|-------|--------------------------------------------------------|
| Same | 100   | Node has a healthy device in a ring sibling Pods occupy |
| Adjacent | 70 | Node's ring is in `adjacency[sibling-ring]`           |
| Disjoint | 30 | Node has a device but on no occupied or adjacent ring |
| NoCandidate | 0 | Node has no healthy device the plugin recognises    |

**Test gate** (4 adjacency cases per phase7-plan §3 T008 acceptance,
adjacency_test.go):

| Case                                       | Asserts                                              |
|--------------------------------------------|------------------------------------------------------|
| `TestDefaultAdjacency910B8CardRingClosure` | 4 rings × 2 neighbors each · exact `{0:[1,3],...}`   |
| `TestBuildAdjacencyCustomMapParse`         | well-formed custom map parses; dup values dedup       |
| `TestBuildAdjacencyEmptyMapNoAdjacency`    | nil + empty → nil out (binary 100/30 fallback)        |
| `TestBuildAdjacencyMalformedRejects`       | non-int key / negative / self-loop → ErrMalformedAdjacency |

Plus 2 new score_test.go cases:
- "Phase 7 T008 · default 910B 8-card adjacency → adjacent ring 70"
  — sibling on ring 0, this node on ring 1 → Score=70
- "Phase 7 T008 · explicit empty Adjacency falls back to binary 100/30"
  — same setup with explicit `args.Adjacency={}` → Score=30

**Cross-references**: ADR-0010 §256 risk row (HCCS Adjacency map
empty default · Phase 7 T008 closes) · ADR-0011 (Phase 7 W1 entry
includes T008 as a polish item alongside the Source interface refactor) ·
phase7-plan.md §3 P7-T-008.

### 2.2 ResourceSlice attribute contract (types.go)

Read from `slice.Spec.Devices[i].Basic.Attributes`:

| QualifiedName               | Type     | Used by   |
|-----------------------------|----------|-----------|
| `npu.huawei.com/hccs_ring`  | IntValue | Filter + Score (T005) |
| `npu.huawei.com/numa_node`  | IntValue | NumaAffinity (T006 — passthrough) |
| `npu.huawei.com/health`     | StringValue (`"Healthy"` / `"Unhealthy"` / `"Unknown"`) | Filter (skip non-Healthy) |
| `npu.huawei.com/ai_cores`   | IntValue | Score (T005 — capacity weighting) |

Constants live in `types.go`. Text-copied from npu-dra-driver per
operators/CLAUDE.md §1 (no cross-module Go import). ADR-0010 §5
freezes this schema.

### 2.3 Pod annotation contract

| Annotation key                          | Format                | Read by              |
|-----------------------------------------|-----------------------|----------------------|
| `npu.huawei.com/preferred-hccs-ring`    | Comma-separated int list (e.g. `"0,1"`) | Filter (per Args.PreferAnnotation) |
| `inference.ocloud.edge.example.com/model-service` | `<ns>/<name>` | Score (T005 — sibling-Pod grouping key per ADR-0010 §2) |

## 3. 生命周期

### 3.1 Plugin construction (operative · T004)

```
cmd/main.go main()
   │ app.NewSchedulerCommand
   │   + app.WithPlugin(hccs.Name, hccs.New)
   │   + app.WithPlugin(numa.Name, numa.New)
   │   + app.WithPlugin(binpack.Name, binpack.New)
   │ cli.Run(command)
   ▼
On profile init for each Pod-scheduling cycle:
   hccs.New(ctx, args runtime.Object, h framework.Handle)
     │ parseArgs(args) → *HCCSTopologyArgs with defaults
     │ h.SharedInformerFactory().Resource().V1beta1().ResourceSlices().Lister()
     │   → wrap as informerSliceLister (production sliceLister)
     ▼
   &HCCSTopology{args, sliceLister}
```

### 3.2 Filter cycle (operative · T004)

```
Filter(ctx, cycleState, pod, nodeInfo) *framework.Status
   │
   ├── if pod missing PreferAnnotation:
   │     - FailIfMissing=false → Success (permissive)
   │     - FailIfMissing=true  → UnschedulableAndUnresolvable
   │
   ├── parsePreferredRings(annotation)
   │     - empty / all-malformed → as if missing
   │     - valid list → continue
   │
   ├── sliceLister.ListForNode(nodeInfo.Node().Name)
   │     - no slices → UnschedulableAndUnresolvable (when FailIfMissing) / Success (permissive)
   │     - error → framework.Error
   │
   ├── for slice in slices: for device in slice.Spec.Devices:
   │     - deviceMatchesRing(device, preferredRings) AND deviceHealthy
   │       → return Success
   │
   └── default → UnschedulableAndUnresolvable
        "no healthy NPU device on node X matches HCCS ring set Y"
```

### 3.3 Score cycle (operative · T005)

```
PreScore(ctx, cs, pod, nodes) *framework.Status         # once per Pod cycle
   │
   ├── state := &hccsState{adjacency: buildAdjacency(p.args.Adjacency)}
   │
   ├── modelService := pod.Labels[ModelServiceLabel]
   │
   ├── if modelService=="" OR allocationLister==nil OR sliceLister==nil:
   │       cs.Write(hccsStateKey, state)   # empty ringsOccupied
   │       return Success
   │
   ├── allocs, err := allocationLister.ListByModelService(modelService)
   │       err → framework.Error
   │
   ├── for alloc in allocs:
   │       if alloc.Name == pod.Name: continue (skip self heuristic)
   │       ring, ok := lookupDeviceRing(sliceLister, alloc.NodeName, alloc.Device)
   │       if !ok: continue (device + ring not resolvable)
   │       state.ringsOccupied[ring] = struct{}{}
   │
   └── cs.Write(hccsStateKey, state); return Success


Score(ctx, cs, pod, nodeName) (int64, *framework.Status)  # per candidate node
   │
   ├── state, _ := readHCCSState(cs)
   ├── if len(ringsOccupied)==0:
   │       return ScoreNeutral (50) — covers no-MS-label + first-Pod-of-MS
   │
   ├── if sliceLister==nil:
   │       return ScoreNeutral (50) — graceful degradation
   │
   ├── nodeRings, err := nodeRingsFor(sliceLister, nodeName)
   │       err → framework.Error
   │
   ├── if len(nodeRings)==0:
   │       return ScoreNoCandidate (0)
   │
   ├── for r in nodeRings: if r in ringsOccupied → return ScoreSame (100)
   │
   ├── for r in nodeRings: for o in ringsOccupied:
   │       if o in state.adjacency[r] → return ScoreAdjacent (70)
   │
   └── return ScoreDisjoint (30)
```

Score tiers per ADR-0010 §2:
- `0`   ScoreNoCandidate — node has no NPU device the plugin recognises
- `30`  ScoreDisjoint     — node's rings disjoint from siblings'
- `50`  ScoreNeutral      — no MS label OR no siblings yet
- `70`  ScoreAdjacent     — node's rings adjacent (per Args.Adjacency)
- `100` ScoreSame         — node shares at least one ring with a sibling

`ScoreExtensions()` returns nil — framework auto-normalises to [0..100],
which our constants already inhabit. NPUSliceAllocation lookup runs **once
per Pod** in PreScore (PreScorePlugin path), not per-node, keeping the
Score hot-path O(devices_on_node × |ringsOccupied|).

## 4. 错误处理

| Error                                                       | Action |
|-------------------------------------------------------------|--------|
| `nodeInfo` or `nodeInfo.Node()` nil                         | framework.Error (defensive — should not happen in normal flow) |
| args parse / Weight out of [0..100]                         | New returns error → kube-scheduler refuses to register plugin (fail-fast at startup) |
| sliceLister wired but `ListForNode` returns error           | framework.Error — surfaces as Pod scheduling retry, not stuck |
| sliceLister nil (no SharedInformerFactory)                  | Permissive if FailIfMissing=false, else UnschedulableAndUnresolvable |
| Pod annotation present but all entries malformed            | Treated as "missing annotation" path |
| Slice device health attribute absent                        | Treated as Healthy (back-compat with pre-T002 publishers) |

## 5. 扩展点

### 5.1 T005 Score body (operative — landed alongside T004 Filter)

- `score.go` implements `PreScore` + `Score` + `ScoreExtensions`
- `colocation.go` implements the NPUSliceAllocation lister abstraction
  (allocationLister interface; dynamicAllocationLister production impl;
  fakeAllocationLister in score_test.go)
- `score_test.go` covers 7 sub-tests across the 5 scoring tiers +
  no-MS-label + multi-ring same. Plus `TestBuildAdjacency` 3-case
  sub-suite for the string→int adjacency conversion.

### 5.2 T006 NumaAffinity (placeholder; upstream wrap deferred)

Status: **T006 placeholder landed Phase 6**; upstream wrap **attempted
Phase 7 T002 · re-deferred to Phase 8 baseline bump**.

Per ADR-0010 §3 this plugin should wrap upstream
`sigs.k8s.io/scheduler-plugins/pkg/noderesourcetopology` without
modification. At T006 entry (Phase 6) the direct `nrt.New(...)` delegation
FAILED to build because upstream v0.31.8 references `framework.GVK` —
a symbol that exists in K8s 1.31's `pkg/scheduler/framework` but was
removed in K8s 1.32 (our pinned baseline per `go.mod` replace block /
ADR-0010 §1).

API drift table (refreshed 2026-05-20 P7-T-002):

| sched-plugins | K8s target | GA status (2026-05-20)    | vs our K8s 1.32 baseline pin                                                                                                                   |
|---------------|-----------|---------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------|
| v0.30.x       | 1.30      | GA                        | INCOMPATIBLE (framework.GVK present, removed in 1.32)                                                                                           |
| v0.31.x       | 1.31      | GA                        | INCOMPATIBLE — observed fail at T006 entry                                                                                                       |
| **v0.32.7**   | **1.32**  | **GA (2024-08-06)**       | **module resolves cleanly · BUT transitive deps require apimachinery v0.32.7+ packages (`pkg/api/safe` / `pkg/api/operation` / `pkg/api/validate`) missing under our replace-block pin → blocks landing without bumping replace block to v0.32.7 (uniform) AND audit downstream transitive expectations** |
| v0.33.5       | 1.33      | GA (2024-10-27)           | Untested — would force K8s baseline bump (deferred Phase 8+ scope)                                                                              |
| v0.34.7       | 1.34      | GA (2025-04-20 · latest)  | Untested — would force K8s baseline bump                                                                                                       |

**P7-T-002 attempted upgrade outcome (2026-05-20)**:
- `go get sigs.k8s.io/scheduler-plugins@v0.32.7` succeeded · all
  `k8s.io/*` indirect deps bumped to v0.32.7
- Bumping replace block from `v0.32.0` → `v0.32.7` to match aligned
- `go mod tidy` then surfaces `k8s.io/apimachinery@v0.32.7` missing
  `pkg/api/{safe,operation,validate}` packages along the import chain
  `cmd/main.go → kube-scheduler/app → pkg/scheduler → pkg/apis/core/{validation,v1}`
- These apimachinery packages are post-v0.32 additions (likely v1.33+);
  our K8s 1.32 baseline (per ADR-0010 §1) cannot absorb the transitive
  expectation without a broader baseline bump
- Revert clean (single `git checkout go.mod go.sum`) — no commit took
  the upgrade path

**Operative**: T006 placeholder unchanged — kube-scheduler registers the
plugin under `NumaAffinity` but invokes no Filter/Score because we don't
implement the extension-point interfaces. T101 chart continues to OMIT
NumaAffinity from its KubeSchedulerConfiguration default; numaAffinity.enabled
chart toggle remains `false` default.

**Forward path** (Phase 8 candidate):
1. Bump K8s baseline from 1.32 → 1.33 (or 1.34) — coordinate with kind
   smoke baseline (currently `kindest/node:v1.32.x` per P5-T-114)
2. Bump sched-plugins to matching minor (v0.33.x or v0.34.x)
3. Replace `plugin.go` placeholder body with `return nrt.New(ctx, args, h)`
   wrap pattern (mirror hccs/args.go parseArgs structure for our
   NumaAffinityArgs · pre-construct upstream `NodeResourceTopologyMatchArgs`
   with `LeastAllocated` scoring strategy + cpu/memory weight=1)
4. Add 3 sanity tests per phase7-plan §3 T002 acceptance (TestNameConstants
   already exists · TestDefaultArgsWeight · TestParseArgsAcceptsTypedAndUnknown)
5. Updating T101 chart's KubeSchedulerConfiguration to enable
   NumaAffinity in profile + `numaAffinity.enabled=true` chart default
6. Close known-issues #12 (this entry) when wrap lands

Until then, operators wanting NUMA-aware scheduling can run upstream
sched-plugins binary as a second scheduler alongside our HCCSTopology+
Binpack binary (functional today, more operational overhead).

### 5.3 T007 Binpack (operative)

- `internal/plugins/binpack/` package, ~150 LOC across binpack.go + args.go
  (slightly above the 50-LOC estimate due to manual DeepCopyObject + arg
  parsing patterns mirrored from hccs; the actual Score formula is ~25 LOC)
- ScorePlugin only (no Filter — ADR-0010 §4: Binpack only influences
  score, never blocks scheduling)
- Score formula:
  - for each `r` in `Args.ResourceWeights`:
    - `ratio_r = clamp(pod.requested[r] / node.allocatable[r], 0, 1)`
    - `contribution_r = weight_r * ratio_r * 100`
  - `score = sum(contribution_r) / sum(weight_r)` (range [0..100])
- Default ResourceWeights: `{cpu:1, memory:1, npu.ocloud.edge.example.com/devices:5}`
- Default `Enabled=false` — chart operators opt in via values.yaml
- 9 sub-tests passing: 6 Score cases (disabled / single-resource /
  multi-resource / empty allocatable / zero-request / clamp) + 3
  parseArgs cases

### 5.4 T008 Integration tests (operative)

- `internal/integration/integration_test.go` + `helpers_test.go`
  exercise HCCSTopology Filter+Score (T004+T005) on a 2-node × 2-ring
  fixture; Binpack covered by its own package tests (T007)
- **Setup-envtest NOT required**: tests use in-process fake
  SliceLister + fake AllocationLister (mirrors per-plugin abstractions
  established T004/T005). Real apiserver dispatch is covered by
  T106 kind smoke
- 5 sub-tests passing: no-MS-label → neutral, MS-label-no-siblings →
  neutral, sibling-on-ring-0 → 100/30 split, ring-5-annotation → all
  filtered, adjacency-map-kicks-in
- NUMA plugin **deferred** per T006 placeholder; integration test grows
  a 6th case when NUMA wrap lands

### 5.5 Phase 7+ (forward notes per ADR-0010 §7)

- **Real hardware ring discovery**: npu-dra-driver `Source.RealAscend`
  fetches HCCS topology via `npu-smi info -t topo`; plugin code unchanged.
- **Partitionable Devices (KEP-4815)**: per-partition device emits;
  Filter/Score read the same hccs_ring attribute at finer granularity.
- **Cross-node HCCL**: scheduler-plugin Score adds "same NIC link / same
  leaf switch" maxim via ADR-0007 fabric labels.

## 6. 集成示例

### 6.1 KubeSchedulerConfiguration (rendered by chart T101)

NumaAffinity is **omitted from filter/score enabled lists** in the
default chart values until the upstream wrap lands (see §5.2 deferral).

```yaml
apiVersion: kubescheduler.config.k8s.io/v1
kind: KubeSchedulerConfiguration
profiles:
  - schedulerName: npu-scheduler
    plugins:
      filter:
        enabled:
          - name: HCCSTopology
          # NumaAffinity placeholder — re-add once upstream wrap lands
      score:
        enabled:
          - name: HCCSTopology
            weight: 5
          - name: Binpack
            weight: 1
        disabled:
          - name: NodeResourcesFit  # replaced by Binpack when enabled
    pluginConfig:
      - name: HCCSTopology
        args:
          weight: 5
          preferAnnotation: npu.huawei.com/preferred-hccs-ring
          failIfMissing: false
          adjacency:
            "0": [1, 2]
            "1": [0, 3]
      # - name: NumaAffinity  # placeholder; restore when upstream wrap lands
      #   args:
      #     weight: 2
      - name: Binpack
        args:
          weight: 1
          enabled: false
```

### 6.2 Pod opt-in (T105 inference-operator polish may stamp this)

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: llama-7b-prefill-abc123
  annotations:
    npu.huawei.com/preferred-hccs-ring: "0,1"
    inference.ocloud.edge.example.com/model-service: ocloud-system/llama-7b
  labels:
    inference.ocloud.edge.example.com/pd-role: prefill
spec:
  schedulerName: npu-scheduler
  containers:
    - name: vllm-ascend
      image: registry.example.com/vllm-ascend:0.11.0
      resources:
        claims:
          - name: npu-slice
  resourceClaims:
    - name: npu-slice
      resourceClaimTemplateName: llama-7b-prefill-claim
```

## 7. 参考

- ADR: `docs/adr/0010-scheduler-plugin.md` — design freeze (this module's
  contract source)
- ADR: `docs/adr/0009-npu-dra-driver.md` §6.2 — topology-aware scoring
  forward note that motivated this plugin
- ADR: `docs/adr/0008-pd-router-webhook.md` — `inference.ocloud.edge.
  example.com/model-service` label written by Phase 5 PD Router (Score
  grouping key)
- Architecture: `docs/architecture.md` §5.6 (scheduler-plugin module
  placement) + §6.3 (HCCSTopologyInfo struct populated by P6-T-003)
- Plan: `docs/phase6-plan.md` §3 P6-T-002 / T004 / T005 / T006 / T007 / T008
- Devlog: `docs/devlog/phase-6-t002.md` (scaffold) /
  `docs/devlog/phase-6-t004.md` (this body)
- Code: `internal/plugins/hccs/{plugin,args,types,filter}.go` +
  `filter_test.go`
- Cross-controller:
  - `operators/pool-operator/internal/controller/npupool_controller.go`
    (P6-T-003) — same attribute schema; different consumer pattern
    (status aggregation vs. scheduler hot-path)
  - `operators/npu-dra-driver/internal/publisher/source_simulator.go` +
    `api/v1alpha1/resourceslice_types.go` — attribute publisher
