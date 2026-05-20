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
    Adjacency        map[string][]int32 // T005 Score; default empty
}
```

Encoded inside `KubeSchedulerConfiguration.profiles[*].pluginConfig`
when the chart (T101) renders the ConfigMap. Defaults applied by
`parseArgs` when args fields are zero / nil.

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

Status: **T006 placeholder landed**; upstream wrap deferred.

Per ADR-0010 §3 this plugin should wrap upstream
`sigs.k8s.io/scheduler-plugins/pkg/noderesourcetopology` without
modification. At T006 entry the direct `nrt.New(...)` delegation FAILED
to build because upstream v0.31.8 references `framework.GVK` — a symbol
that exists in K8s 1.31's `pkg/scheduler/framework` but was removed in
K8s 1.32 (our pinned baseline per `go.mod` replace block / ADR-0010 §1).

API drift table (observed 2026-05-20):

| sched-plugins | K8s target | framework.GVK | vs our K8s 1.32 baseline   |
|---------------|-----------|---------------|------------------------------|
| v0.30.x       | 1.30      | present       | INCOMPATIBLE                 |
| v0.31.x       | 1.31      | present       | INCOMPATIBLE — observed fail |
| v0.32.x       | 1.32      | removed       | COMPATIBLE — not yet released |

**Operative**: T006 ships a Name()-only placeholder; kube-scheduler
registers the plugin under `NumaAffinity` but invokes no Filter/Score
because we don't implement the extension-point interfaces. T101 chart
will OMIT NumaAffinity from its KubeSchedulerConfiguration default
until the wrap lands.

**Forward path**: once sched-plugins v0.32.x ships (or a downstream
v0.31.y backport with K8s 1.32 compatibility), revisit by:
1. Bumping the dep in `go.mod`
2. Replacing `plugin.go` placeholder body with `return nrt.New(...)`
3. Updating T101 chart's KubeSchedulerConfiguration to enable
   NumaAffinity in profile

Until then, operators wanting NUMA-aware scheduling can run upstream
sched-plugins binary as a second scheduler alongside our HCCSTopology+
Binpack binary (functional today, more operational overhead).

### 5.3 T007 Binpack

- separate `internal/plugins/binpack/` package
- ~50 LOC internal Score impl per ADR-0010 §4

### 5.4 T008 Integration tests

- `internal/integration/integration_test.go` exercises all 3 plugins
  end-to-end against a multi-ring, 2-node fixture
- Uses `setup-envtest` for real kube-scheduler bootstrap (no fake)
- Plan §3 P6-T-008 acceptance: 5 cases

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
