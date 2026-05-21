# npu-dra-driver — module detailed design

> Status: Phase 4 simulator-first scaffold (T003-T006 + T101).
> Real allocation logic + DeviceClass registration lands Phase 5 per ADR-0009 §5.

## 1. 架构概览

```
┌────────────────────────────────────────────────────────────────────┐
│                Standard-K8s 1.30+ cluster                          │
│                                                                    │
│  ┌──────────────────────┐         ┌─────────────────────────┐      │
│  │   inference-operator │         │   pool-operator         │      │
│  │   (Phase 5 controller│ ┌──────►│   NPUSlicePool          │      │
│  │   - creates RC)      │ │watch  │   .status.              │      │
│  └─────┬────────────────┘ │       │     resourceSlices      │      │
│        │ create           │       │     Observed            │      │
│        ▼                  │       └─────────────────────────┘      │
│  ┌────────────────────────┴──────────────────────────────────┐     │
│  │  resource.k8s.io/v1beta1 (kube-apiserver)                 │     │
│  │  ┌──────────────────┐  ┌──────────────────┐               │     │
│  │  │ ResourceSlice    │  │ ResourceClaim    │               │     │
│  │  │ (one per node)   │  │ (one per Pod)    │               │     │
│  │  └────────▲─────────┘  └────────▲─────────┘               │     │
│  └───────────┼─────────────────────┼─────────────────────────┘     │
│              │ publish/upsert/del  │ watch / patch annotations     │
│  ┌───────────┴─────────────────────┴────────────────────┐          │
│  │              npu-dra-driver (this module)            │          │
│  │  ┌───────────────────┐    ┌────────────────────────┐ │          │
│  │  │ internal/publisher│    │ internal/controller    │ │          │
│  │  │ - Reconcile 30s   │    │ - ClaimReconciler      │ │          │
│  │  │ - Source.List     │    │ - AllocationDeferred   │ │          │
│  │  │ - diff/upsert/del │    │ - Phase 4: annotations │ │          │
│  │  └─────────▲─────────┘    └────────────────────────┘ │          │
│  │            │ List/Watch                              │          │
│  │  ┌─────────┴─────────┐                               │          │
│  │  │ Source interface  │                               │          │
│  │  │  ┌─────────────┐  │                               │          │
│  │  │  │ Simulator   │  │ Phase 4 default               │          │
│  │  │  │ - reads     │  │                               │          │
│  │  │  │   JSON file │  │                               │          │
│  │  │  └─────────────┘  │                               │          │
│  │  │  ┌─────────────┐  │                               │          │
│  │  │  │ Ascend (P5+)│  │ Real npu-smi / DCMI           │          │
│  │  │  └─────────────┘  │                               │          │
│  │  └───────────────────┘                               │          │
│  └──────────────────────────────────────────────────────┘          │
└────────────────────────────────────────────────────────────────────┘
```

**Position**: `operators/npu-dra-driver/` — the cluster's authoritative
publisher of `resource.k8s.io/v1beta1.ResourceSlice` objects under
DriverName=`npu.ocloud.edge.example.com`. Operates only on Standard-K8s
nodes (KubeEdge edgecore nodes excluded — see ADR-0001 v3 §5).

**Cross-references**:
- `docs/architecture.md` §3.4 (NPU/AI 运行时) + §5.5 (npu-dra-driver 模块)
- `docs/adr/0001-phase0-key-decisions.md` §5 v3 — dual-path roadmap
- `docs/adr/0009-npu-dra-driver.md` — full ADR design + Phase 5 implementation notes
- `docs/cann-driver-matrix.md` — host kernel × CANN × driver compatibility

## 2. 数据流

### 2.1 ResourceSlice publication (publisher loop)

```
configs/mock-data/set-a-small/npus.json
   │ (ConfigMap mounted at /etc/npu-dra-driver/mock/npus.json)
   ▼
SimulatorSource.List(ctx)
   │ - read + json.Unmarshal
   │ - project to AscendDevice per NPU
   │ - group by NodeName, sort by Index
   ▼
[]NodeDevices
   │
   ▼
Publisher.Reconcile(ctx)
   │ - list live ResourceSlices filtered by managed-by label
   │ - build desired ResourceSlices via AscendDevice.ToUpstream()
   │ - diff: per-name compare → Create / Update / Delete-stale
   ▼
resource.k8s.io/v1beta1.ResourceSlice writes
   │ - driver = "npu.ocloud.edge.example.com"
   │ - pool.name = NodeName
   │ - nodeName = NodeName
   │ - devices[] populated, attrs in Basic.Attributes
   │ - managed-by label set
   │ - owner refs blank (Phase 4)
   ▼
kube-apiserver (etcd)
   ▲
   │ List/Watch
   │
pool-operator NPUSlicePool Reconcile (cross-watch — T102):
   - lists ResourceSlices, filters by Spec.Driver
   - sets NPUSlicePool.status.resourceSlicesObserved = count
```

**Trigger frequency**:
- 30s ticker (DefaultRequeueInterval) — baseline reconcile cadence
- SimulatorSource.Watch event on `npus.json` mtime change (atomic-rename) — short-circuits the 30s wait
- On manager startup — initial Reconcile fires before the first tick

### 2.2 ResourceClaim observation (claim controller skeleton)

```
inference-operator (Phase 5) or kubectl
   │ create ResourceClaim with spec.devices.requests[].deviceClassName
   ▼
resource.k8s.io/v1beta1.ResourceClaim
   │
   ▼ Watches
ClaimReconciler.Reconcile(ctx, req)
   │ 1. Get claim → NotFound = deletion (return nil, no finalizer)
   │ 2. claimRequestsOurDriver(claim) → check Spec.Devices.Requests[].DeviceClassName
   │    prefix-match against DriverName "npu.ocloud.edge.example.com"
   │    (exact / "/sub" / ".sub" variants for Phase 5 sub-classes)
   │ 3. Foreign driver → return nil (skip silently)
   │ 4. Match → MergeFrom patch metadata.annotations with 4 keys:
   │      ocloud.edge.example.com/allocation-deferred         = "true"
   │      ocloud.edge.example.com/allocation-deferred-reason  = "Phase4Skeleton"
   │      ocloud.edge.example.com/allocation-deferred-message = "<full>"
   │      ocloud.edge.example.com/allocation-deferred-observed-generation = "<gen>"
   │ 5. EventRecorder.Event(claim, Normal, "Phase4Skeleton", <msg>)
   ▼
ResourceClaim annotations + Kubernetes Event observable via kubectl
```

**Why annotations, not status.conditions** (P3 honesty): the upstream
`resource.k8s.io/v1beta1.ResourceClaimStatus` type has no top-level
`Conditions` field. Conditions only exist on `AllocatedDeviceStatus`,
which presumes allocation occurred. Phase 4 has NO real allocation, so
faking `status.devices[]` entries would mislead the scheduler. The
annotations route preserves the plan intent (machine-observable
deferred state with reason + message) without faking allocation.
Phase 5 lifts to `status.devices[].Conditions` when real allocation
lands. See `internal/controller/claim_controller.go` "Schema-drift note"
comment block.

### 2.3 Cross-controller awareness

The pool-operator NPUSlicePool Reconcile (T102) carries a Watches
secondary on resource.k8s.io ResourceSlice, filtered by
`Spec.Driver == "npu.ocloud.edge.example.com"`:

```
Publisher writes ResourceSlice (driver match)
   │
   ▼ informer watch
pool-operator EnqueueRequestsFromMapFunc.mapResourceSliceToPools
   │ filter: skip if Spec.Driver != npuDraDriverName
   │ list all NPUSlicePools
   │ fan out reconcile.Request to each pool
   ▼
NPUSlicePool Reconcile (in pool-operator)
   │ countNPUDRAResourceSlices via cluster-wide List + filter
   │ writes Status.ResourceSlicesObserved
   ▼
kubectl get npuslicepool -o yaml shows the live count
```

## 3. 接口契约

### 3.1 Manager flags (cmd/main.go)

| Flag                          | Default      | Purpose |
| ----------------------------- | ------------ | ------- |
| `--health-probe-bind-address` | `:8081`      | /healthz + /readyz endpoints |
| `--metrics-bind-address`      | `:8082`      | Prometheus default scrape |
| `--leader-elect`              | `false`      | HA mode — single instance fine for Phase 4 simulator |
| `--enable-publisher`          | `false`      | Run SimulatorSource publisher loop; requires --mock-data-path |
| `--enable-claim-controller`   | `false`      | Run ResourceClaim controller skeleton |
| `--mock-data-path`            | `""`         | Path to simulator JSON file (e.g. /etc/npu-dra-driver/mock/npus.json) |
| `--enable-http2`              | `false`      | Disabled by default per GHSA-qppj-fm5r-hxr3 / GHSA-4374-p667-p6c8 |
| Plus zap flags                | various      | --zap-devel / --zap-encoder / --zap-log-level etc. |

### 3.2 Ocloud Device attribute schema (api/v1alpha1)

Set on upstream `resource.k8s.io/v1beta1.Device.Basic.Attributes`:

| Key (QualifiedName)                  | Value type | Range / enum                          | Phase   |
| ------------------------------------ | ---------- | ------------------------------------- | ------- |
| `npu.huawei.com/index`               | int        | 0..7 (Ascend 910B physical chip idx)   | required |
| `npu.huawei.com/health`              | string     | Healthy / Unhealthy / Unknown          | required |
| `npu.huawei.com/slice_strategy`      | string     | FixedTemplate / Dynamic                | required |
| `npu.huawei.com/ai_cores`            | int        | 0 for FixedTemplate; 1..32 for Dynamic | optional |
| `npu.huawei.com/numa_node`           | int        | host NUMA index                        | optional |
| `npu.huawei.com/hccs_ring`           | int        | Phase 6 placeholder, default 0         | optional |

### 3.3 Ocloud Device capacity schema

Set on `Device.Basic.Capacity`:

| Key                                  | Value (resource.Quantity)                |
| ------------------------------------ | ---------------------------------------- |
| `npu.huawei.com/slice_aicore`        | per-device slice AI-core capacity (e.g. 32 for Ascend 910B) |

### 3.4 ResourceClaim annotation schema

Set on `ResourceClaim.metadata.annotations` by Phase 4 controller:

| Key                                                       | Value | Set when |
| --------------------------------------------------------- | ----- | -------- |
| `ocloud.edge.example.com/allocation-deferred`             | `"true"` | matching claim observed |
| `ocloud.edge.example.com/allocation-deferred-reason`      | `"Phase4Skeleton"` | matching claim observed |
| `ocloud.edge.example.com/allocation-deferred-message`     | `"<full message>"` | matching claim observed |
| `ocloud.edge.example.com/allocation-deferred-observed-generation` | `"<claim.Generation>"` | matching claim observed |

Plus (set by Phase 5+ consumers — inference-operator):

| Key                                                 | Value | Set by |
| --------------------------------------------------- | ----- | ------ |
| `ocloud.edge.example.com/model-service-ref`         | `<ns>/<name>` | inference-operator on claim create |
| `ocloud.edge.example.com/preferred-pool`            | `<NPUSlicePool name>` | inference-operator on claim create |

### 3.5 Go types (api/v1alpha1)

```go
package v1alpha1

const DriverName = "npu.ocloud.edge.example.com"

// AscendDevice is the Ocloud-semantic typed view of one Ascend NPU device.
type AscendDevice struct {
    Name                string             // <node>-npu-<index>
    Index               int64              // 0..7
    Health              string             // Healthy / Unhealthy / Unknown
    SliceStrategy       string             // FixedTemplate / Dynamic
    AICores             int64              // > 0 when Dynamic
    NUMANode            int64
    HCCSRing            int64              // Phase 6 placeholder
    SliceAICoreCapacity resource.Quantity
}

func (a AscendDevice) ToUpstream() resourceapi.Device
func AscendDeviceFromUpstream(d resourceapi.Device) (AscendDevice, error)
func ValidateAttributes(d resourceapi.Device) error

// AscendClaimAnnotations is the typed view of Ocloud annotations on claims.
type AscendClaimAnnotations struct {
    ModelServiceRef string
    PreferredPool   string
}

func (a AscendClaimAnnotations) ToMap() map[string]string
func AscendClaimAnnotationsFromMap(m map[string]string) AscendClaimAnnotations
```

### 3.6 Source interface (internal/source · Phase 7 P7-T-004)

**Phase 7 update** (2026-05-20 · ADR-0011 §2): Source interface lifted
OUT of internal/publisher/ into a dedicated `internal/source/` package
so multiple impls (mockjson + realascend) coexist. Phase 4-6
SimulatorSource preserved bit-for-bit as `mockjson.MockJSONSource`.

```go
// internal/source/source.go
type Source interface {
    List(ctx context.Context) ([]NodeDevices, error)
    Watch(ctx context.Context) <-chan Event
    QueryTopology(ctx context.Context, nodeName string) (*HCCSTopology, error)
}

type NodeDevices struct {
    NodeName string
    Devices  []v1alpha1.AscendDevice
}

type Event struct {
    Source string  // "mock-json" / "real-ascend"
    Reason string  // "file-modified" / "hotplug" / "tick"
}

type HCCSTopology struct {
    NodeName string
    Rings    map[int32][]string  // ringID → device names
    NUMA     map[int32][]string  // numaNode → device names
}

var ErrNotImplemented = errors.New("source: not implemented")
```

**Implementations**:
- `mockjson.MockJSONSource{Config}` — Phase 4-6 behavior preserved
  bit-for-bit; reads `configs/mock-data/set-a-small/*.json`; **Phase 7
  default** (chart values `publisher.sourceType: mock-json`)
- `realascend.RealAscendSource{Config}` — Phase 7 W1 stub returning
  `source.ErrNotImplemented` from List/QueryTopology + closed Watch
  channel; Phase 7 T101 lab-conditional lights up the real
  `npu-smi info -t topo` + DCMI body. Operators opt in via chart
  values `publisher.sourceType: real-ascend` (chart does NOT block
  selection; reconcile-loop logs surface ErrNotImplemented warnings)

**Factory dispatch**: `cmd/main.go::selectSource(sourceType, mockDataPath,
realAscendMode)` switches on `source.ParseSourceType(--source-type
flag)`. Lives in cmd/main.go (not source pkg) to avoid an
`internal/source → internal/source/{mockjson,realascend} →
internal/source` import cycle. The source pkg owns `SourceType` enum +
`FactoryConfig` struct; cmd/main.go owns the switch + subpackage
imports.

**Phase 4-6 → Phase 7 import path migration**:

| Phase 4-6 (deleted)                              | Phase 7 (operative)                                                                                          |
|--------------------------------------------------|-------------------------------------------------------------------------------------------------------------|
| `publisher.Source` interface                     | `source.Source` (interface · 3 methods: List + Watch + QueryTopology)                                       |
| `publisher.NodeDevices` / `publisher.Event`      | `source.NodeDevices` / `source.Event`                                                                       |
| `publisher.SimulatorSource{Path, ...}`           | `mockjson.New(mockjson.Config{Path, WatchPollInterval, SliceAICoreCapacityFallback})`                       |
| `publisher.go` uses `Source` (local interface)   | `publisher.go` uses `source.Source` (imported from internal/source)                                         |
| `cmd/main.go` constructs `&publisher.SimulatorSource{...}` | `cmd/main.go` calls `selectSource(...)` which dispatches via `source.ParseSourceType` → mockjson.New OR realascend.New |

**Tests**: existing `internal/publisher/publisher_test.go` 5 cases pass
unchanged (now wired through `mockjson.New(...)` instead of inline
`&SimulatorSource{...}`). NEW `internal/source/mockjson/mockjson_test.go`
4 cases cover Source contract (List shape + Watch event + QueryTopology
rings + empty/error edge). NEW `internal/source/realascend/realascend_test.go`
1 case asserts stub returns `source.ErrNotImplemented` per ADR-0011 §2.

### 3.7 npu-smi parser contract (Phase 7 P7-T-005 · operative)

Phase 7 P7-T-005 ships the `internal/source/realascend/npusmi/`
subpackage that the lab-conditional T101 RealAscendSource body will
consume to bridge real npu-smi / DCMI calls into the Source contract.

**Package layout**:

```
operators/npu-dra-driver/internal/source/realascend/npusmi/
├── client.go             - Client interface + TopoEntry + DeviceInfo +
│                           HealthState + ErrNoCommand + ErrParse
├── fake.go               - FakeClient impl backed by embed.FS testdata
│                           fixtures (8card + 16card); used by tests +
│                           T101 dev mode without lab cluster
├── exec.go               - ExecClient impl shelling out to `npu-smi`
│                           binary; Phase 7 W1 scaffold only (QueryTopo
│                           parser wired; QueryDeviceInfo + QueryHealth
│                           bodies deferred to T101 lab landing)
├── parse.go              - ParseTopoMatrix: text-matrix → []TopoEntry
│                           with Ring populated via connected-components
│                           on HCCS edges. Sidecar comments
│                           (# numa: NPU0=0 ... + # health: NPU0=Healthy ...)
│                           populate NumaNode + Health for tests.
├── parse_test.go         - 5 cases: empty / 8-card / 16-card /
│                           unhealthy sidecar / malformed row
└── testdata/
    ├── npu-smi-topo-fixture-8card.txt   - canonical 8-card 910B
    │                                       (2 HCCS rings of 4 NPUs)
    └── npu-smi-topo-fixture-16card.txt  - synthetic 16-card 910B-pro
                                            (4 HCCS rings of 4 NPUs +
                                            cross-numa SYS links)
```

**Client interface (3 methods)**:

```go
type Client interface {
    QueryTopo(ctx context.Context) ([]TopoEntry, error)
    QueryDeviceInfo(ctx context.Context, devID int) (*DeviceInfo, error)
    QueryHealth(ctx context.Context, devID int) (HealthState, error)
}
```

- **QueryTopo** — primary path · returns ALL devices' (DeviceID +
  Ring + NumaNode + Health) for the bound node. Phase 7 T101 lab body
  composes Ring from `npu-smi info -t topo` + NumaNode from
  `numactl --hardware` or sysfs + Health from `dcmi_get_device_health`.
  Phase 7 W1 parser populates Ring only; NumaNode + Health default
  to 0 / HealthUnknown unless sidecar hints present.
- **QueryDeviceInfo** — per-device detail (chip / cores / memory /
  driver versions). Phase 7 W1 type-only; T101 wires
  `npu-smi info -t board -i <id>` parser.
- **QueryHealth** — cheaper than QueryDeviceInfo · called by Source.Watch
  loop at high cadence. Phase 7 W1 type-only.

**npu-smi topo matrix format** (per upstream Huawei docs):

```
       NPU0   NPU1   NPU2   NPU3   ...
NPU0   X      HCCS   HCCS   HCCS   ...
NPU1   HCCS   X      HCCS   HCCS   ...
...

Legend:
  X    = self
  SYS  = Connection traversing PCIe + SMP interconnect (cross-NUMA)
  HCCS = Connection traversing at most a single HCCS switch
  PIX  = Connection traversing a single PCIe switch (PHB)
```

Parser semantics:
- Build undirected graph where `HCCS` = edge between devices
- BFS connected components in DeviceID-ascending order — first
  component reached from NPU0 gets `Ring=0`; next gets `Ring=1`; etc.
- 8-card standard config: NPU0-3 = ring 0 · NPU4-7 = ring 1
- 16-card 910B-pro: 4 rings of 4 NPUs each (intra-numa HCCS · inter-numa
  SYS · inter-quadrant PIX inside the same numa)

**Test gate (5 cases per phase7-plan §3 T005 acceptance)**:

| Case                          | Asserts                                              |
|-------------------------------|------------------------------------------------------|
| `TestParseEmptyInput` × 3 sub | empty / whitespace / comment-only → `ErrParse`       |
| `TestParse8CardFixture`        | 8 entries · NPU0-3 ring 0 · NPU4-7 ring 1 · NodeID + NUMA sidecar honored |
| `TestParse16CardFixture`       | 16 entries · 4 rings of 4 each (NPU0-3=0 · NPU4-7=1 · NPU8-11=2 · NPU12-15=3) |
| `TestParseUnhealthyHint`       | 2-device minimal matrix + health sidecar → per-device Health populated |
| `TestParseMalformedRow`        | row vs column count mismatch → `ErrParse`            |

**Phase 7 W1 CI does NOT exercise any real npu-smi binary** — tests
go through FakeClient backed by embedded testdata fixtures. ExecClient
compiles + is type-asserted to satisfy Client interface but is
unreachable from the W1 RealAscendSource stub (which returns
ErrNotImplemented uniformly per ADR-0011 §2 §3).

**Cross-references**: ADR-0010 §7 forward note row 1 (real HCCS ring
discovery via npu-smi) · ADR-0011 §2 (Source interface + lab gating
policy) · phase7-plan §3 P7-T-005 (this task) + §4 P7-T-101 (lab body
that wires ExecClient).

### 3.8 NPUSliceTemplate CRD (Phase 7 P7-T-006 · operative)

Phase 7 P7-T-006 ships the `NPUSliceTemplate` CRD per ADR-0011 §1 §4 to
express user-defined complex slice compositions that the Phase 7
template engine (P7-T-007) + allocator (P7-T-105) decompose into
existing fixed-template requests.

**Go types** (`api/v1alpha1/npuslicetemplate_types.go`):

```go
type NPUSliceTemplate struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec   NPUSliceTemplateSpec   `json:"spec,omitempty"`
    Status NPUSliceTemplateStatus `json:"status,omitempty"`
}

type NPUSliceTemplateSpec struct {
    Composition       []TemplatePart    `json:"composition"`            // required · MinItems=1
    FallbackStrategy  FallbackStrategy  `json:"fallbackStrategy,omitempty"` // default fixed-template-combination
}

type TemplatePart struct {
    Type          PartType  // enum: whole | vir04 | vir08 | vir16 | dynamic-shard
    Count         int32     // required · Minimum=1
    AICoreRequest int32     // dynamic-shard only · ignored otherwise
}

type NPUSliceTemplateStatus struct {
    Conditions             []metav1.Condition  // Validated + Allocatable
    FallbackAppliedReason  string              // "decomposed into 1×vir04 + 1×vir08"
    ObservedGeneration     int64
}
```

**Enum constants** (drift here = silent migration break · TestEnumValuesArePinned guards):

| Const                                          | String value                |
|------------------------------------------------|-----------------------------|
| `PartTypeWhole`                                | `"whole"`                   |
| `PartTypeVir04`                                | `"vir04"`                   |
| `PartTypeVir08`                                | `"vir08"`                   |
| `PartTypeVir16`                                | `"vir16"`                   |
| `PartTypeDynamicShard`                         | `"dynamic-shard"`           |
| `FallbackStrategyFixedTemplateCombination`     | `"fixed-template-combination"` |
| `FallbackStrategyRefuse`                       | `"refuse"`                  |
| `ConditionTypeValidated`                       | `"Validated"`               |
| `ConditionTypeAllocatable`                     | `"Allocatable"`             |

**Pod opt-in via label** `npu.huawei.com/slice-template=<NPUSliceTemplate.metadata.name>`
on the Pod template. Absent label → Pod takes Phase 5 existing
whole-NPU allocator path (zero regression). Phase 7 T105 allocator
reads the label + looks up the NPUSliceTemplate + invokes Engine.Decompose
+ allocates as bundle (all-or-nothing semantics per ADR-0011 §後果 row 4).

**Validation rules** (template_controller T007 enforces):

| Spec violation                                                | Validated condition |
|---------------------------------------------------------------|--------------------|
| MinItems=1 on Composition (empty array)                       | False · `reason="EmptyComposition"` |
| Count < 1 on any part                                         | False · `reason="InvalidCount"` |
| Type=dynamic-shard + FallbackStrategy=fixed-template-combination | False · `reason="DynamicShardNotSupported"` (Phase 7 fallback only knows fixed templates · gated on driver-layer breakthrough OR KEP-4815 GA per ADR-0011 §後果) |
| Type=<unknown enum>                                           | Rejected at admission · kubebuilder enum marker enforces |

**Sample compositions** (`config/samples/`):

| Sample file                              | Composition                   | FallbackStrategy            | Use case                              |
|------------------------------------------|-------------------------------|-----------------------------|---------------------------------------|
| `npuslicetemplate_qwen_pd.yaml`          | 1× vir04 + 1× vir08           | fixed-template-combination  | Qwen 8B PD-pair on single NPU         |
| `npuslicetemplate_deepseek_20b.yaml`     | 1× whole                      | refuse                       | DeepSeek 20B strict mode demo         |

**Chart bundling** (Phase 5 chart-bundles-CRD convention preserved):
CRD YAML lives at `deploy/helm-charts/npu-dra-driver/crds/npuslicetemplates.yaml`
alongside `npusliceallocations.yaml`. Helm install applies both before
templates render (per chart-bundles-CRD pattern; not subject to
`helm template` rendering output count).

**Test gate** (4 round-trip cases · Phase 7 P7-T-006 acceptance):

| Case                                | Asserts                                                       |
|-------------------------------------|---------------------------------------------------------------|
| `TestRoundTripJSONMarshal`          | NPUSliceTemplate marshal → unmarshal preserves all fields     |
| `TestDeepCopyPreservesComposition`  | DeepCopy returns detached object (mutating copy doesn't leak) |
| `TestEmptyStatusOmitted`            | Empty Status fields omit from marshaled JSON (omitempty)      |
| `TestEnumValuesArePinned`           | PartType + FallbackStrategy + ConditionType strings pinned     |

**Cross-references**: ADR-0011 §1 NPU 动态切分 fallback + §4 schema
detail + §後果 row 4 (all-or-nothing bundle allocation invariant) · ADR-0009
§4 Partitionable Devices forward note (long-term replacement after KEP-4815 GA) ·
phase7-plan.md §3 P7-T-006 + §3 P7-T-007 (template engine consumer) + §4
P7-T-105 (allocator consumer).

---

### 3.9 Template engine + composition decomposition (Phase 7 P7-T-007 · operative)

Phase 7 P7-T-007 ships the template engine + NPUSliceTemplate
reconciler per ADR-0011 §1 §4 — bridge between user-declared
`NPUSliceTemplate.Spec.Composition` and the allocator (P7-T-105) input
contract.

**Package layout**:

```
operators/npu-dra-driver/
├── internal/template/
│   ├── types.go    - FixedTemplateBundle + FixedTemplateItem
│   ├── engine.go   - Engine.Validate + Engine.Decompose +
│   │                  ValidationError + 4 Reason constants
│   └── engine_test.go - 6 cases per phase7-plan §3 T007 acceptance
└── internal/controller/
    ├── npuslicetemplate_controller.go      - Reconciler
    └── npuslicetemplate_controller_test.go - 3 fake-client cases
```

**Engine API**:

```go
type Engine struct{}
func New() *Engine
func (e *Engine) Validate(spec NPUSliceTemplateSpec) error
func (e *Engine) Decompose(spec NPUSliceTemplateSpec) (*FixedTemplateBundle, string, error)
```

Decompose returns (bundle, fallbackAppliedReason, error) where:
- bundle is the FixedTemplateBundle (Items + IsEmpty + TotalSlices)
- fallbackAppliedReason is the human-readable summary (e.g.
  "decomposed into 1×vir04 + 1×vir08") stamped on
  NPUSliceTemplate.Status.FallbackAppliedReason
- error is a typed *ValidationError (IsValidationError / AsValidationError
  helpers) on schema violations; nil on success

**Validation rules** (Engine.Validate enforces beyond kubebuilder
admission):

| Violation                                     | Reason                            |
|-----------------------------------------------|-----------------------------------|
| `Spec.Composition` is empty                   | `EmptyComposition`                |
| `Composition[i].Count < 1`                    | `InvalidCount`                    |
| `Composition[i].Type` unknown enum            | `InvalidType`                     |
| `Composition[i].Type=dynamic-shard`           | `DynamicShardNotSupported`        |

`dynamic-shard` is rejected regardless of `FallbackStrategy` in Phase 7
W1 — both `fixed-template-combination` and `refuse` strategies fail
the same way because Phase 7 has no driver-layer hook for dynamic
sharding (gated on Phase 8+ per ADR-0009 §4 + ADR-0011 §後果).

**Decomposition semantics** (Phase 7 W1 mechanical):

- For each `Composition[i]` of `(Type=X, Count=N)`: emit
  `FixedTemplateItem{Template: X, Count: N}`
- Same `Type` appearing twice → merge by summing `Count` (preserves
  order of first occurrence)
- `FallbackAppliedReason` sorts items alphabetically by Template name
  for diff idempotence (e.g. `"decomposed into 1×vir04 + 1×vir08"`)

**Reconciler**:

- Watches `NPUSliceTemplate` objects
- On change: invokes `Engine.Decompose` → computes desired status
  - On `ValidationError`: stamps `Validated=False` + reason · `Allocatable=False` (gated)
  - On success: stamps `Validated=True` + `Allocatable=True` (Phase 7 W1
    placeholder · T105 allocator replaces with real bundle-vs-pool
    availability check) + `FallbackAppliedReason`
- `statusEqual` shallow check ignores `LastTransitionTime` to prevent
  infinite reconcile churn
- Emits `Warning` Events with `ValidationError.Reason` as type when
  validation fails (operators see kubectl-describable feedback)

**Test gate** (6 engine + 3 controller cases per phase7-plan §3 T007):

| Engine test                              | Case                                                       |
|------------------------------------------|------------------------------------------------------------|
| `TestDecomposeEmptyCompositionRejects`   | nil/empty Composition → ValidationError EmptyComposition   |
| `TestDecomposeWholeOnly`                 | 1× whole → 1-item bundle · TotalSlices=1                   |
| `TestDecomposeSingleVir04`               | 1× vir04 → 1-item bundle                                   |
| `TestDecomposeVir04PlusVir08`            | Qwen-PD-style 1× vir04 + 1× vir08 → 2-item · sorted reason |
| `TestDecomposeRefuseRejectsDynamicShard` | dynamic-shard + refuse → ValidationError DynamicShardNotSupported |
| `TestDecomposeFixedTemplateCombination`  | dup vir04 (×2 + ×1) + whole → merged 3×vir04 + 1×whole     |

| Controller test                                                                       | Case                                  |
|---------------------------------------------------------------------------------------|---------------------------------------|
| `TestNPUSliceTemplateReconciler_EmptyCompositionStampsValidatedFalse`                 | empty → Validated=False · Allocatable=False · empty FallbackAppliedReason |
| `TestNPUSliceTemplateReconciler_ValidCompositionStampsValidatedAndAllocatable`        | Qwen-PD → Validated=True · Allocatable=True · FallbackAppliedReason populated · ObservedGeneration synced |
| `TestNPUSliceTemplateReconciler_DynamicShardRejected`                                 | dynamic-shard + refuse → Validated=False reason=DynamicShardNotSupported  |

**`cmd/main.go` integration**: `--enable-template-controller` flag
defaults `true` — registers `NPUSliceTemplateReconciler` alongside
existing Claim + Allocation controllers. Disable only for diagnostic
builds.

**Cross-references**: ADR-0011 §1 NPU 动态切分 + §4 NPUSliceTemplate
schema + §後果 row 4 (all-or-nothing bundle invariant) · ADR-0009 §4
(Partitionable Devices long-term replacement) · phase7-plan.md §3
P7-T-007 (this task) + §4 P7-T-105 (allocator consumer reads
FixedTemplateBundle) + §4 P7-T-103 (kind smoke asserts
status.Allocatable=True for sample composition).

---

## 4. 生命周期

### 4.1 Manager startup

```
main.go init()
   │ utilruntime.Must(clientgoscheme.AddToScheme(scheme))
   │ utilruntime.Must(resourceapi.AddToScheme(scheme))   // T005
   │
main.go main()
   │ parse flags + setup zap logger
   │ build controller-runtime manager (HealthProbeBindAddress, MetricsBindAddress, LeaderElection)
   │
   │ if --enable-publisher:
   │   require --mock-data-path non-empty → os.Exit(1) otherwise
   │   construct publisher.Publisher{Client, Source: SimulatorSource{Path: mockDataPath}}
   │   publisher.SetupWithManager(mgr)  // wraps Start as a Runnable
   │
   │ if --enable-claim-controller:
   │   construct ClaimReconciler{Client, Scheme, Recorder: mgr.GetEventRecorderFor("npu-dra-claim-controller")}
   │   cr.SetupWithManager(mgr)  // controller-runtime builder + ResourceClaim watch
   │
   │ mgr.AddHealthzCheck("healthz", healthz.Ping)
   │ mgr.AddReadyzCheck("readyz",   healthz.Ping)
   │ mgr.Start(ctrl.SetupSignalHandler())
```

### 4.2 Publisher runnable lifecycle

```
Publisher.Start(ctx)
   │ initial Reconcile pass (synchronous, errors logged)
   │
   │ for {
   │   select {
   │     case <-ctx.Done():
   │       log "Publisher stopping"; return nil
   │     case <-ticker.C (every 30s):
   │       Reconcile(ctx)  // List → diff → Create/Update/Delete-stale
   │     case <-source.Watch:
   │       Reconcile(ctx)  // early reconcile on file change
   │   }
   │ }
```

**Slices NOT deleted on shutdown**: Phase 4 graceful exit leaves
published ResourceSlices in etcd. They're recreated on next startup
(deterministic sliceNameForNode keeps idempotency). Kind smoke tests
can restart the manager without the driver-name flapping out.

### 4.3 Claim controller event lifecycle

```
controller-runtime watches ResourceClaim
   │ create / update / delete events queue into workqueue
   ▼
ClaimReconciler.Reconcile(ctx, req)
   │ Get claim → NotFound = clean return (no finalizer)
   │ claimRequestsOurDriver? No → silent skip
   │ Yes → MergeFrom patch annotations + EventRecorder
   │ return ctrl.Result{} (no requeue — annotations are terminal Phase 4 state)
```

## 5. 错误处理

### 5.1 Publisher errors

| Error                              | Severity | Action |
| ---------------------------------- | -------- | ------ |
| Source.List file read failure      | retry    | log + return error; next 30s tick re-tries |
| Source.List JSON parse failure     | retry    | log + return error; same as above |
| client.List ResourceSlices failure | retry    | log + return error; next 30s tick re-tries |
| client.Create AlreadyExists race   | tolerate | log V(1) + skip; next pass picks it up |
| client.Update conflict             | retry    | controller-runtime workqueue exponential backoff |
| client.Delete NotFound             | tolerate | log V(1) + continue; the slice is already gone |
| Source.Watch channel closed        | degrade  | log V(1) + nil the channel; rely on 30s tick alone |

### 5.2 Claim controller errors

| Error                              | Severity | Action |
| ---------------------------------- | -------- | ------ |
| Get claim NotFound                 | normal   | return nil (deletion path) |
| Get claim other error              | retry    | return error; workqueue retries with backoff |
| Patch annotations failure          | retry    | return error; workqueue retries |
| EventRecorder absent (nil recorder)| degrade  | skip event emission (chart always wires it; tests sometimes pass nil) |

## 6. 扩展点 (Phase 5+ 演进)

### 6.1 Real-Ascend Source (Phase 5+)

The `Source` interface is intentionally narrow (`List` + `Watch`).
Phase 5 lands `internal/publisher/source_ascend.go` implementing the
interface against npu-smi / DCMI:

```go
type AscendSource struct {
    NpuSmiPath  string
    PollInterval time.Duration
}

func (a *AscendSource) List(ctx context.Context) ([]NodeDevices, error) {
    // exec npu-smi info / parse output / populate AscendDevice fields
}

func (a *AscendSource) Watch(ctx context.Context) <-chan Event {
    // npu-smi has no event channel; poll at PollInterval and emit on change
}
```

main.go gains `--source` flag: `simulator` (default) / `ascend`.

### 6.2 DeviceClass registration (Phase 5)

Helm chart gains `templates/deviceclass.yaml`:

```yaml
apiVersion: resource.k8s.io/v1beta1
kind: DeviceClass
metadata:
  name: npu.ocloud.edge.example.com
spec:
  selectors:
    - cel:
        expression: device.driver == "npu.ocloud.edge.example.com"
```

Phase 5 sub-classes (e.g. `/whole`, `.dynamic`) add additional
selector expressions that filter by
`device.attributes["npu.huawei.com/slice_strategy"].string`. The
`.string` accessor is mandatory — `device.attributes["..."]`
returns a `DeviceAttribute` struct (CEL type `map(string, any)`)
which cannot be `==`-compared with a string literal directly. K8s
admission rejects the expression with `compilation failed: ERROR:
found no matching overload for '_==_' applied to '(map(string, any),
string)'` if the accessor is omitted (kind smoke run on commit
88aeb38 hit this; fixed at P5-T-116).

### 6.3 Real claim allocation (Phase 5)

ClaimReconciler grows from "log + annotate" to "select device + write
status.devices[]". Per ADR-0009 §6 pseudocode:

### 6.3.1 Phase 5 allocator algorithm (P5-T-002 + P5-T-003)

The `internal/allocator/` package ships two implementations behind the
`Allocator` interface:

```go
type Allocator interface {
    Allocate(claim, slices, allocated) (*Allocation, error)
}
```

**Greedy** (default, T002): scans slices in lexicographic order by
`Name`, then devices within each slice in lex order by `Name`. The
first device that (a) belongs to the npu-dra-driver, (b) is not in
the `AllocatedSet`, (c) reports `Health == Healthy`, and (d) matches
the request's DeviceClassName sub-class filter is returned. On the
Phase 4 simulator (3 nodes × 8 NPUs / set-a-small fixture) this
deterministically picks `nodeA-npu-0` first, then `nodeA-npu-1`, etc.

Invariants:
- **Determinism**: identical input → identical output. Sort happens on
  a defensive copy so caller-supplied slices retain their order.
- **Single-request claims only in Phase 5**. The allocator picks the
  first `claim.Spec.Devices.Requests[0]` and ignores the rest. Phase 6
  iterates all requests (multi-device claims).
- **Health filter inline**. Unhealthy / Unknown devices skipped. No
  admin-override knob in Phase 5; Phase 7 may add one when real
  hardware enters the picture.

**BestFit** (T003 feature-flag, default off): ranks every eligible
candidate device by `SliceAICoreCapacity`, picks the smallest one. On
heterogeneous fleets (mixed vir04 + vir08 + whole) this minimizes
fragmentation by reserving larger devices for larger future claims.
On Phase 5 simulator (uniform capacity) BestFit is observationally
equal to Greedy; the implementation matters once Phase 7 publishes
per-partition Device entries via KEP-4815.

Phase 6 supersedes both with topology-aware scoring (NUMA + HCCS-ring
affinity) inside `kube-scheduler` via a scheduler-plugin — the
allocator package then becomes a fallback path for clusters not
running the plugin.

### 6.3.2 Phase 4 annotation path DEPRECATED

The Phase 4 `AllocationDeferred` annotation path
(`ocloud.edge.example.com/allocation-deferred*`) is no longer set by
the controller. The annotation key constants in
`claim_controller.go` carry `// Deprecated:` markers and exist only
for migration parsing: the Phase 5 controller, on first reconcile of
a Phase 4-annotated claim, **strips** the four annotations via a
metadata `Patch(MergeFrom)` and requeues. The next reconcile sees a
clean claim and runs the real allocator. Test:
`controller.TestClaim_StripsPhase4Annotations`.

### 6.3.3 ResourceClaim.Status write shape

On successful allocation the controller patches the status
subresource (`Client.Status().Patch(MergeFrom)`) with two fields:

```go
status.allocation = &AllocationResult{
    Devices: DeviceAllocationResult{
        Results: []DeviceRequestAllocationResult{{
            Request: pick.Request,
            Driver:  pick.Driver,
            Pool:    pick.Pool,
            Device:  pick.Device,
        }},
    },
}
status.devices = []AllocatedDeviceStatus{{
    Driver: pick.Driver, Pool: pick.Pool, Device: pick.Device,
    Conditions: []metav1.Condition{{
        Type: "Ready", Status: "True", Reason: "Allocated",
        Message: "Allocated by npu-dra-driver (strategy=…, aiCores=…)",
    }},
}}
```

The DRA scheduler reads `status.allocation` to bind a Pod to the
chosen device; consumers (PD Router webhook, inference-operator
phase machine) read `status.devices[].Conditions[Ready=True]` to
know when the allocation is materialised.

#### Old §6.3 reference pseudocode (kept for cross-reference)


```go
func (r *ClaimReconciler) Reconcile(ctx, req) (ctrl.Result, error) {
    claim, err := r.Get(...)
    if err != nil { ... }

    if !claimRequestsOurDriver(claim) { return ctrl.Result{}, nil }

    // NEW Phase 5:
    if claim.Status.Allocation != nil {
        // already allocated; ensure ReservedFor is correct
        return ctrl.Result{}, nil
    }

    slice, device, err := r.allocator.Allocate(claim)
    if err != nil { ... return retry }

    claim.Status.Allocation = &AllocationResult{
        Devices: AllocationResultDeviceConfig{
            // pool, device, driver name, etc.
        },
    }
    return r.Client.Status().Update(ctx, claim)
}
```

### 6.3.4 NPUSliceAllocation lifecycle (P5-T-004 + P5-T-005)

`NPUSliceAllocation` is a cluster-scoped CRD that mirrors every
successful claim allocation as an audit-log entry. It is **not** the
source of truth for "is this device allocated" — that remains
`ResourceClaim.Status.Allocation`. NPUSliceAllocation exists to provide:

1. A reverse-lookup index (device → claim) for Phase 6
   scheduler-plugin co-location decisions.
2. A per-tenant quota substrate for Phase 9 multi-tenancy (the quota
   controller lists NPUSliceAllocations by namespace label).
3. A human-readable audit trail (`kubectl get npua` shows current
   allocations with claim / device / node / aiCores / phase columns).

Lifecycle states (see api/v1alpha1.NPUSliceAllocationPhase*):

```
                                                 GC cascade deletes
                                                 the audit entry
                                                  (via owner-ref)
                                                       │
   create ─► Allocated ──► (claim deleted) ──► Released ──► (Orphaned)
   (claim_controller       within grace                    after grace
    + Reconcile)           period                          period
```

- **Allocated**: claim exists with non-empty Status.Allocation
  matching this entry's SliceRef. The `Available=True` condition
  accompanies this phase.
- **Released**: owner claim was deleted; we are within the
  OrphanGracePeriod (default 30s) waiting for K8s GC to remove the
  audit entry. `Available=False/ClaimDeleted`.
- **Orphaned**: owner claim absent for longer than the grace period.
  Indicates GC did not run as expected (resource handler error,
  network partition); a Warning event fires. Operator action: inspect
  the audit entry + delete it manually.

The cascade is driven by `metadata.ownerReferences[0]` pointing at the
ResourceClaim with `Controller=true` + `BlockOwnerDeletion=true`. The
allocation controller never deletes audit entries itself — only K8s GC
does — so the Orphaned phase is meant for human inspection.

Phase 5 chart wiring:
- `values.yaml` `allocationController.enabled` toggle (default true)
- `deployment.yaml` passes `--enable-allocation-controller` per the
  toggle
- `rbac.yaml` grants `npusliceallocations get/list/watch/create/
  update/patch/delete` + `npusliceallocations/status update/patch`
  on the ServiceAccount

### 6.4 Partitionable Devices migration (Phase 7, K8s 1.37 GA)

Publisher emits per-partition Device entries instead of one Device
per NPU. Consumer code (inference-operator) is API-stable — only the
publisher internals change. See ADR-0009 §4.

### 6.5 Cross-module shared constants

Currently `npuDraDriverName = "npu.ocloud.edge.example.com"` lives
twice (npu-dra-driver/api/v1alpha1.DriverName + pool-operator/internal/controller/npuslicepool_controller.go). When a third consumer appears
(Phase 5 inference-operator? scheduler-plugin?), extract a shared
`pkg/sharedconst/` Go module to break the duplication. Until then,
inline duplication with cross-reference comments is acceptable per
operators/CLAUDE.md §1.

## 7. 集成示例

### 7.1 Local manager startup (developer iteration)

```bash
cd operators/npu-dra-driver
make build
./bin/manager \
  --enable-publisher \
  --mock-data-path=../../configs/mock-data/set-a-small/npus.json \
  --enable-claim-controller \
  --leader-elect=false
```

### 7.2 Kind cluster integration (P4-T-104)

```bash
bash tests/e2e/kind/install.sh build-images   # docker build + kind load
bash tests/e2e/kind/install.sh up              # helm install npu-dra-driver

kubectl get resourceslices
kubectl get npuslicepool -n ocloud-system smoke-pool -o jsonpath='{.status.resourceSlicesObserved}'
bash tests/e2e/kind/dra_publish_test.sh        # 4-tier jq assertion
```

### 7.3 Standalone helm install (operator deploy)

```bash
helm install npu-dra-driver deploy/helm-charts/npu-dra-driver \
  --namespace ocloud-system --create-namespace \
  --set image.repository=registry.example.com/ocloud/npu-dra-driver \
  --set image.tag=v0.1.0
```

### 7.4 Phase 5 inference-operator integration (forward)

```go
// inference-operator ModelService Reconcile (Phase 5):
func (r *ModelServiceReconciler) Reconcile(ctx, req) (ctrl.Result, error) {
    var ms inferencev1alpha1.ModelService
    r.Get(ctx, req.NamespacedName, &ms)

    // For each Prefill + Decode replica, create a ResourceClaim
    // referencing the npu-dra-driver DeviceClass:
    for i := 0; i < int(ms.Spec.PDPair.Prefill.Replicas); i++ {
        claim := &resourceapi.ResourceClaim{
            ObjectMeta: metav1.ObjectMeta{
                Name:      fmt.Sprintf("%s-prefill-%d", ms.Name, i),
                Namespace: ms.Namespace,
                Annotations: map[string]string{
                    "ocloud.edge.example.com/model-service-ref": ms.Namespace + "/" + ms.Name,
                    "ocloud.edge.example.com/preferred-pool":    ms.Spec.NPUSlicePoolRef.Name,
                },
            },
            Spec: resourceapi.ResourceClaimSpec{
                Devices: resourceapi.DeviceClaim{
                    Requests: []resourceapi.DeviceRequest{
                        {
                            Name:            "npu",
                            DeviceClassName: v1alpha1.DriverName + "/whole",
                        },
                    },
                },
            },
        }
        r.Create(ctx, claim)
    }
    // ... npu-dra-driver Phase 5 allocator reconciles each claim ...
}
```

## 8. 参考

- ADRs: `docs/adr/0001-phase0-key-decisions.md` §5 v3 + §7 / `docs/adr/0007-fabric-discovery.md` / `docs/adr/0008-pd-router-webhook.md` / `docs/adr/0009-npu-dra-driver.md` (primary)
- Architecture: `docs/architecture.md` §3.4 (NPU/AI runtime) + §5.5 (npu-dra-driver 模块) + §6.7 (multi-tenancy) + §13 (Phase 5 review row)
- Compatibility: `docs/cann-driver-matrix.md` (Phase 7 entry gate)
- Plan: `docs/phase4-plan.md` §3 P4-T-003..006 / P4-T-101 / P4-T-105
- Devlog: `docs/devlog/phase-4-t{003..006,101,105}.md`
- Upstream: `kubernetes-sigs/dra-example-driver v0.2.1` (fork-spirit), KEP-4815 Partitionable Devices
- Code: `operators/npu-dra-driver/api/v1alpha1/` · `internal/publisher/` · `internal/controller/` · `cmd/main.go` · `deploy/helm-charts/npu-dra-driver/`
- Sibling modules: `operators/pool-operator/` (T102 cross-watch) · `operators/inference-operator/` (Phase 5 consumer)
