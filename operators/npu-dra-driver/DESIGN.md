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
| `npu.huawei.com/slice-strategy`      | string     | FixedTemplate / Dynamic                | required |
| `npu.huawei.com/ai-cores`            | int        | 0 for FixedTemplate; 1..32 for Dynamic | optional |
| `npu.huawei.com/numa-node`           | int        | host NUMA index                        | optional |
| `npu.huawei.com/hccs-ring`           | int        | Phase 6 placeholder, default 0         | optional |

### 3.3 Ocloud Device capacity schema

Set on `Device.Basic.Capacity`:

| Key                                  | Value (resource.Quantity)                |
| ------------------------------------ | ---------------------------------------- |
| `npu.huawei.com/slice-aicore`        | per-device slice AI-core capacity (e.g. 32 for Ascend 910B) |

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

### 3.6 Source interface (internal/publisher)

```go
type Source interface {
    List(ctx context.Context) ([]NodeDevices, error)
    Watch(ctx context.Context) <-chan Event
}

type NodeDevices struct {
    NodeName string
    Devices  []v1alpha1.AscendDevice
}

type Event struct {
    Source string  // "simulator" / "real-ascend"
    Reason string  // "file-modified" / "hotplug" / "tick"
}
```

**Implementations**:
- `SimulatorSource{Path, WatchPollInterval, SliceAICoreCapacityFallback}` — Phase 4 default
- `AscendSource` (Phase 5+) — real npu-smi / DCMI discovery

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
selector expressions that filter by `device.attributes["npu.huawei.com/slice-strategy"]`.

### 6.3 Real claim allocation (Phase 5)

ClaimReconciler grows from "log + annotate" to "select device + write
status.devices[]". Per ADR-0009 §6 pseudocode:

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
