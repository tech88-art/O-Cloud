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

### 3.3 Score cycle (T005 forward note)

Per ADR-0010 §2 Score table:
- No model-service label → 50 (neutral)
- Sibling Pod on this ring → 100
- Sibling Pod on adjacent ring (per Args.Adjacency) → 70
- Sibling Pod only on disjoint rings → 30
- No candidate devices → 0

Reads NPUSliceAllocation via dynamic client (similar lister abstraction).

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

### 5.1 T005 Score body (immediate next-phase work)

- `score.go` adds `Score(ctx, cycleState, pod, nodeName) (int64, *framework.Status)`
- `colocation.go` adds NPUSliceAllocation lister abstraction (mirror sliceLister pattern)
- `score_test.go` adds 5 envtest cases per plan §3 P6-T-005

### 5.2 T006 NumaAffinity wrap

- separate `internal/plugins/numa/` package, no shared code with hccs
- thin wrapper around `sigs.k8s.io/scheduler-plugins/pkg/noderesourcetopology`

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

```yaml
apiVersion: kubescheduler.config.k8s.io/v1
kind: KubeSchedulerConfiguration
profiles:
  - schedulerName: npu-scheduler
    plugins:
      filter:
        enabled:
          - name: HCCSTopology
          - name: NumaAffinity
      score:
        enabled:
          - name: HCCSTopology
            weight: 5
          - name: NumaAffinity
            weight: 2
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
      - name: NumaAffinity
        args:
          weight: 2
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
