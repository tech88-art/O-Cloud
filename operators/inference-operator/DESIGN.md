# inference-operator — module detailed design

> Status: Phase 5 T006 — ModelService Reconciler scaffold landed
> alongside Phase 4 CRD types. T007/T008 expand the Reconcile body
> (Deployment + ResourceClaim materialisation, phase machine to Ready);
> T101-T104 add cert-manager + PD Router webhook per ADR-0008.

## 1. 架构概览

```
┌────────────────────────────────────────────────────────────────────┐
│                          K8s cluster                               │
│                                                                    │
│  ┌────────────────────────┐                                        │
│  │  user / GitOps         │                                        │
│  │  kubectl apply ms.yaml │                                        │
│  └──────────┬─────────────┘                                        │
│             │ create                                               │
│             ▼                                                      │
│  ┌────────────────────────────────────────────────────────┐        │
│  │  inference.ocloud.edge.example.com/v1alpha1            │        │
│  │  ModelService (CRD)                                    │        │
│  │  spec:                                                 │        │
│  │    model: { image, modelPath }                         │        │
│  │    pdPair: { prefill, decode, routerLabel }            │        │
│  │    npuSlicePoolRef: { name }                           │        │
│  │  status: { phase, conditions, observedGeneration }     │        │
│  └────────────────────────────────────────────────────────┘        │
│             ▲                                                      │
│             │ watch (Phase 5)                                      │
│             │                                                      │
│  ┌──────────┴──────────────────────────────────────────────┐       │
│  │            inference-operator (this module)             │       │
│  │  Phase 4: scaffold + types only (no controller)         │       │
│  │  Phase 5 controller body:                               │       │
│  │  ┌─────────────────────────────────────────────────┐   │       │
│  │  │ ModelServiceReconciler                          │   │       │
│  │  │  1. resolve npuSlicePoolRef                     │   │       │
│  │  │  2. create N ResourceClaims (via npu-dra-driver)│   │       │
│  │  │  3. create Prefill + Decode Deployments         │   │       │
│  │  │  4. track conditions + phase transitions        │   │       │
│  │  └─────────────────────────────────────────────────┘   │       │
│  │  Phase 5 PD Router (same binary, ADR-0008):             │       │
│  │  ┌─────────────────────────────────────────────────┐   │       │
│  │  │ Mutating admission webhook                       │   │       │
│  │  │  - reads pod labels for routerLabel match        │   │       │
│  │  │  - stamps npu.huawei.com/slice-bindings          │   │       │
│  │  │    annotation onto Pods                          │   │       │
│  │  └─────────────────────────────────────────────────┘   │       │
│  └─────┬──────────────────────────────────────────┬────────┘       │
│        │                                          │                │
│        ▼ creates                                  ▼ creates        │
│  ┌───────────────────┐                  ┌────────────────────┐    │
│  │ ResourceClaim     │                  │ Prefill Deployment │    │
│  │ (npu-dra-driver   │                  │ Decode Deployment  │    │
│  │  DeviceClass)     │                  │ (vllm-ascend pods) │    │
│  └─────────┬─────────┘                  └────────────────────┘    │
│            │                                                       │
│            ▼ Phase 5 allocates                                     │
│  ┌────────────────────────┐                                       │
│  │ npu-dra-driver         │                                       │
│  │ (real allocation)      │                                       │
│  └────────────────────────┘                                       │
└────────────────────────────────────────────────────────────────────┘
```

**Position**: `operators/inference-operator/` — the cluster's ModelService
controller + PD Router admission webhook. Phase 4 ships scaffold + CRD
types only; the controller body + webhook are the Phase 5 headline.

**Cross-references**:
- `docs/architecture.md` §3.4 (推理框架) + §5.4 (inference-operator module · Phase 5)
- `docs/adr/0002-no-kserve.md` — why not KServe (spec §47-48 prohibition)
- `docs/adr/0008-pd-router-webhook.md` — PD Router admission webhook design
- `docs/adr/0009-npu-dra-driver.md` §5 — Phase 5 inference-operator integration with npu-dra-driver

## 2. 数据流 (Phase 5 forward · Phase 4 ships only the CRD substrate)

### 2.1 ModelService lifecycle (Phase 5 forward)

```
User: kubectl apply -f modelservice.yaml
   │
   ▼ kube-apiserver admits (CRD schema validation)
ModelService stored
   │ status.phase = "Pending"  (default from CRD schema)
   │
   ▼ controller-runtime watch
ModelServiceReconciler.Reconcile (Phase 5)
   │
   │ 1. status.phase = "Provisioning"
   │
   │ 2. Resolve npuSlicePoolRef.name in same namespace
   │    └─ NPUSlicePool exists + Ready? continue : status.phase=Failed
   │
   │ 3. Create N ResourceClaims (one per pod across both PD sides):
   │    for i := 0; i < spec.pdPair.prefill.replicas; i++ {
   │        claim := &resourceapi.ResourceClaim{
   │            ObjectMeta: {Name: "<ms>-prefill-<i>", Namespace: ms.Namespace,
   │                         Annotations: {
   │                             ocloud.edge.example.com/model-service-ref: ns/ms,
   │                             ocloud.edge.example.com/preferred-pool: npuSlicePoolRef.Name,
   │                         }},
   │            Spec: {Devices: {Requests: [{DeviceClassName: npu.ocloud.edge.example.com}]}},
   │        }
   │        r.Create(ctx, claim)
   │    }
   │    (same for spec.pdPair.decode.replicas)
   │
   │ 4. Wait for npu-dra-driver to allocate (status.allocation != nil)
   │
   │ 5. Create Prefill Deployment + Decode Deployment:
   │    - container[0].image = spec.model.image
   │    - args include --model-path=<spec.model.modelPath>
   │    - Pod template carries spec.pdPair.routerLabel = "prefill" | "decode"
   │    - resources.claims references the N ResourceClaims (DRA scheduling)
   │
   │ 6. status.phase = "Ready" when both Deployments Available
   │    + status.conditions:
   │      - Available=True (at least one Pod ready per side)
   │      - AllocationReady=True (all N claims bound)
   │      - ProgressDeadline=True iff timed out
   ▼
Pod creation → PD Router admission webhook fires
```

### 2.2 PD Router admission webhook (Phase 5, ADR-0008)

```
kubectl creates Pod (from Prefill/Decode Deployment)
   │
   ▼ kube-apiserver receives POST /api/v1/.../pods
   │
   ▼ MutatingAdmissionWebhook configured at install time
PD Router webhook handler (Phase 5):
   │ 1. Pod has label inference.ocloud.edge.example.com/pd-role? → continue : skip
   │ 2. Look up the parent ModelService (via pod.spec.serviceAccountName +
   │    deployment owner reference walk OR via a recorded annotation)
   │ 3. Read ModelService.spec.npuSlicePoolRef → fetch live NPUSlicePool
   │ 4. Decide slice bindings (Phase 5 algorithm per ADR-0009 §6):
   │    - greedy first-fit: walk ResourceSlices for the pool
   │    - select N devices matching pod's PD-role + ai-core requirement
   │ 5. Inject annotation:
   │    npu.huawei.com/slice-bindings = json([{node, npu_index, slice_template, ai_cores}, ...])
   │
   ▼ admit pod with mutated annotations
Pod stored
   │
   ▼ scheduler-plugin (Phase 6 NUMA+HCCS) reads the annotation
   │   honors NUMA affinity + HCCS group preference in Score / Bind
```

### 2.3 Phase 4 (current) data flow

Phase 4 ships **only** the CRD types — no Reconcile, no admission, no
data flow beyond what kube-apiserver does on `kubectl apply`:

```
kubectl apply -f modelservice.yaml
   │
   ▼ CRD validation: schema + printColumns + status subresource
ModelService stored in etcd
   │ status.phase = "Pending"  (default kicks in)
   │
   X (no controller — Pending forever)
```

This is observable + obvious — anyone running `kubectl get ms` sees
`Phase: Pending` and the README directs them to Phase 5 plan for
controller body.

## 3. 接口契约

### 3.1 Manager flags (cmd/main.go, Phase 4 scaffold)

| Flag                          | Default | Purpose |
| ----------------------------- | ------- | ------- |
| `--health-probe-bind-address` | `:8081` | /healthz + /readyz |
| `--metrics-bind-address`      | `:8082` | Prometheus default scrape |
| `--leader-elect`              | `false` | HA mode |
| `--enable-http2`              | `false` | Disabled per GHSA-* |
| (Phase 5+) `--enable-controller`     | `false` | run ModelServiceReconciler |
| (Phase 5+) `--enable-webhook`        | `false` | run PD Router mutating webhook |
| (Phase 5+) `--webhook-cert-path`     | `""`    | TLS cert dir (cert-manager wires this) |

### 3.2 ModelService CRD schema (api/v1alpha1)

**API group / version**: `inference.ocloud.edge.example.com/v1alpha1`
**Kind**: `ModelService`
**Plural**: `modelservices` (short name `ms`)
**Scope**: Namespaced

```go
type ModelService struct {
    metav1.TypeMeta
    metav1.ObjectMeta

    Spec   ModelServiceSpec
    Status ModelServiceStatus
}

type ModelServiceSpec struct {
    // Image + ModelPath together identify the vllm-ascend container + model weights.
    Model ModelSpec `json:"model"`

    // Prefill + Decode replicas configuration per ADR-0008.
    PDPair PDPairSpec `json:"pdPair"`

    // NPUSlicePool to allocate slices from (same-namespace).
    NPUSlicePoolRef corev1.LocalObjectReference `json:"npuSlicePoolRef"`
}

type ModelSpec struct {
    Image     string `json:"image"`      // required, vllm-ascend image ref
    ModelPath string `json:"modelPath"`  // required, path in image or PVC
}

type PDReplicaSpec struct {
    Replicas int32 `json:"replicas"`  // >= 0, default 1
}

type PDPairSpec struct {
    Prefill     PDReplicaSpec `json:"prefill"`
    Decode      PDReplicaSpec `json:"decode"`
    RouterLabel string        `json:"routerLabel,omitempty"`  // default "inference.ocloud.edge.example.com/pd-role"
}

type ModelServiceStatus struct {
    Phase              ModelServicePhase  `json:"phase,omitempty"`               // Pending / Provisioning / Ready / Failed
    Conditions         []metav1.Condition `json:"conditions,omitempty"`          // standard K8s conditions
    ObservedGeneration int64              `json:"observedGeneration,omitempty"`
}

type ModelServicePhase string  // "Pending" | "Provisioning" | "Ready" | "Failed"
```

**Phase 5 status.conditions Types** (Phase 4 controller is absent):
- `Available` — at least one Pod ready per PD side
- `ProgressDeadline` — provisioning timed out
- `AllocationReady` — all N ResourceClaims bound via npu-dra-driver

### 3.3 printColumns (kubectl get ms)

```
NAME       IMAGE                                    PREFILL  DECODE  PHASE        AGE
llama-7b   registry.example.com/vllm-ascend:0.11.0  2        4       Pending      5m
```

## 4. 生命周期

### 4.1 Manager startup (Phase 4 scaffold)

```
main.go init()
   │ clientgoscheme + inferencev1alpha1.AddToScheme
   │
main.go main()
   │ parse flags + setup zap
   │ build controller-runtime manager
   │ (NO controllers registered)
   │ mgr.AddHealthzCheck + AddReadyzCheck
   │ "Starting manager" log with phase=4-scaffold task=P4-T-103
   │ mgr.Start(ctrl.SetupSignalHandler())
```

Manager binary starts, runs the health probes, idles forever. SIGTERM
triggers clean exit. Useful for sanity-checking scheme registration +
kubeconfig loading without controller side effects.

### 4.2 Phase 5 controller lifecycle (forward)

```
main.go main()
   │ ... same scaffold ...
   │ if --enable-controller:
   │   modelsvc.NewReconciler{Client, Scheme, Recorder}.SetupWithManager(mgr)
   │     // builder.For(ModelService).Owns(Deployment).Owns(ResourceClaim).Complete(r)
   │ if --enable-webhook:
   │   pdroutewebhook.SetupWithManager(mgr, certDir)
   │     // mgr.GetWebhookServer().Register("/mutate", &webhook.Admission{Handler: ...})
   │ mgr.Start(...)
```

### 4.2.1 P5-T-006 ModelService Reconciler scaffold (operative)

```
Reconcile flow (T006 commit):
  1. Get ModelService → if NotFound, return (deletion path)
  2. If DeletionTimestamp != nil:
        RemoveFinalizer + Update → return
     (T007/T008 expand this to drain owned Deployments + claims first)
  3. If !ContainsFinalizer(FinalizerName):
        AddFinalizer + Update → Requeue (let new RV drive next pass)
  4. Resolve spec.npuSlicePoolRef via unstructured client:
       - NotFound:
            phase=Failed
            PoolUnresolved=False reason=NPUSlicePoolNotFound
            Warning event + return
       - exists but status.totalSlices == 0:
            phase=Provisioning
            PoolUnresolved=True reason=NPUSlicePoolFound
            AllocationReady=False reason=WaitingForPool
            Normal event + return
       - exists with status.totalSlices > 0:
            phase=Provisioning
            PoolUnresolved=True
            AllocationReady=False reason=ProvisioningScaffold
            (T007 promotes AllocationReady once claims allocated)
```

Cross-module discipline: NPUSlicePool is consumed via
`unstructured.Unstructured` (operators/CLAUDE.md §1 forbids cross-
module Go imports). `poolGVK` constant pins the GroupVersionKind.

Finalizer name: `inference.ocloud.edge.example.com/modelservice-cleanup`.
Phase 5 T008 expands the deletion-drain to wait for owned Deployments
+ ResourceClaims to be GC'd before removing the finalizer.

### 4.3 ModelService reconcile lifecycle (Phase 5)

```
ModelService creation
   ▼
phase = Pending → Provisioning
   ▼
For each PD-side replica:
   - create ResourceClaim with model-service-ref + preferred-pool annotations
   - npu-dra-driver allocates → claim.status.allocation populated
   ▼
Create Prefill + Decode Deployments
   ▼
Wait for Pod readiness
   ▼
phase = Ready

(on user delete:)
   ▼ DeletionTimestamp set
   ▼ Finalizer (Phase 5 to be added): delete child Deployments + claims
   ▼ Remove finalizer → object GCed
```

## 5. 错误处理 (Phase 5 forward)

| Error                                            | Severity | Action |
| ------------------------------------------------ | -------- | ------ |
| NPUSlicePool referenced by name does NOT exist   | terminal | phase=Failed; condition AllocationReady=False reason=NPUSlicePoolMissing; do NOT requeue |
| NPUSlicePool exists but Ready=False              | retry    | requeue 30s; surface condition AllocationReady=False reason=NPUSlicePoolNotReady |
| ResourceClaim creation conflict (alreadyExists)  | tolerate | log + skip; next pass picks it up |
| ResourceClaim allocation timeout (10m)           | retry    | condition AllocationReady=False reason=AllocationTimeout; requeue 1m |
| Deployment creation conflict                     | tolerate | log + skip |
| Deployment Pods never become Ready (5m)          | terminal | condition Available=False reason=PodNotReady; phase=Failed |
| ProgressDeadlineSeconds exceeded (10m total)     | terminal | condition ProgressDeadline=True; phase=Failed |
| PD Router webhook TLS cert load failure          | startup  | exit non-zero — chart's cert-manager dependency missing |

## 6. 扩展点

### 6.1 Phase 5 controller body (immediate next-phase work)

The package layout follows kubebuilder convention:

```
operators/inference-operator/
├── api/v1alpha1/                   # T103 ✅
│   ├── groupversion_info.go
│   ├── modelservice_types.go
│   └── zz_generated.deepcopy.go
├── internal/                       # Phase 5 NEW
│   ├── controller/
│   │   ├── modelservice_controller.go    # ModelServiceReconciler
│   │   ├── modelservice_controller_test.go
│   │   ├── allocation.go                 # Phase 5 allocator (per ADR-0009 §6)
│   │   ├── deployments.go                # Prefill / Decode Deployment templates
│   │   ├── suite_test.go
│   │   └── utils.go
│   └── webhook/
│       ├── pdrouter.go                   # PD Router admission handler
│       ├── pdrouter_test.go
│       └── webhook.go                    # mgr.GetWebhookServer().Register(...)
├── config/                         # Phase 5 NEW (certmanager + webhook YAML)
│   ├── certmanager/
│   └── webhook/
└── ... existing scaffold files
```

### 6.2 Helm chart (Phase 5)

`deploy/helm-charts/inference-operator/`:
- Chart.yaml + values.yaml
- templates/{deployment,configmap,rbac,serviceaccount,_helpers.tpl}.yaml
- templates/webhook.yaml — MutatingWebhookConfiguration + Service + cert-manager Certificate
- Dependency on cert-manager (Helm chart dependency, NOT subchart — operators install cert-manager separately)

### 6.3 Failure handling refinement (Phase 5+)

- Add a `ms.spec.recreatePolicy: OnFailure | Never` field so failed
  ModelServices auto-recover on transient errors.
- Add a `ms.spec.progressDeadlineSeconds` field (default 600s) so
  caller can extend the wait for slow image pulls.

### 6.4 Multi-tenant quota (Phase 9)

- Coordinate with NPUSliceAllocation CRD (Phase 5 placeholder per arch §6.8)
  to enforce per-namespace quota.
- Add `ms.spec.priority` / `preemption` policy for hot models displacing cold ones.

### 6.5 OpenAPI contract publication (Phase 5+)

If the demo backend / frontend learns about ModelService, add a
`ModelServiceDTO` to `docs/api-contract.yaml` mirroring the Go types.
Currently the contract has no ModelService surface — Phase 1-4 demo
backend doesn't observe inference-operator.

## 7. 集成示例

### 7.1 Phase 4 (CRD-only, observable but inert)

```bash
# Apply the CRD (manager binary running as a sanity-check pod)
kubectl apply -f operators/inference-operator/config/crd/bases/inference.ocloud.edge.example.com_modelservices.yaml

# Create a ModelService
cat <<EOF | kubectl apply -f -
apiVersion: inference.ocloud.edge.example.com/v1alpha1
kind: ModelService
metadata:
  name: llama-7b
  namespace: ocloud-system
spec:
  model:
    image: registry.example.com/vllm-ascend:v0.11.0
    modelPath: /models/llama-7b
  pdPair:
    prefill:
      replicas: 2
    decode:
      replicas: 4
    routerLabel: inference.ocloud.edge.example.com/pd-role
  npuSlicePoolRef:
    name: ascend-pool-a
EOF

# Inspect
kubectl get ms -n ocloud-system
# NAME       IMAGE  ...  PREFILL  DECODE  PHASE     AGE
# llama-7b   ...    ...  2        4       Pending   5s
# (Pending forever in Phase 4 — controller body is Phase 5)
```

### 7.2 Phase 5 forward (controller body + PD Router)

After Phase 5 lands:

```bash
# Install with cert-manager dependency (chart)
helm install cert-manager jetstack/cert-manager --set crds.enabled=true
helm install inference-operator deploy/helm-charts/inference-operator \
  --namespace ocloud-system --create-namespace \
  --set controller.enabled=true \
  --set webhook.enabled=true

# After ~30s, the same ModelService transitions:
kubectl get ms -n ocloud-system -w
# NAME       IMAGE  ...  PHASE
# llama-7b   ...    ...  Pending
# llama-7b   ...    ...  Provisioning
# llama-7b   ...    ...  Ready

# Inspect the allocated ResourceClaims
kubectl get resourceclaim -n ocloud-system
# llama-7b-prefill-0   ...allocated...
# llama-7b-prefill-1   ...allocated...
# llama-7b-decode-0    ...allocated...
# ... etc.

# Inspect the materialised Deployments + Pods
kubectl get deploy,pods -n ocloud-system -l app.kubernetes.io/managed-by=inference-operator
```

## 8. 参考

- ADRs: `docs/adr/0002-no-kserve.md` (primary) / `docs/adr/0005-pod-in-topology-fusion.md` (`npu.huawei.com/slice-bindings` annotation read path) / `docs/adr/0008-pd-router-webhook.md` (webhook design) / `docs/adr/0009-npu-dra-driver.md` §5 (Phase 5 integration with npu-dra-driver)
- Architecture: `docs/architecture.md` §3.4 (推理框架) + §5.4 (inference-operator module) + §13 (Phase 5 review row)
- Plan: `docs/phase4-plan.md` §3 P4-T-103 (this scaffold) / Phase 5 plan (forthcoming, controller body)
- Devlog: `docs/devlog/phase-4-t103.md`
- Code: `operators/inference-operator/api/v1alpha1/modelservice_types.go` · `cmd/main.go` · `config/crd/bases/inference.ocloud.edge.example.com_modelservices.yaml`
- Sibling modules: `operators/pool-operator/` (NPUSlicePool consumer) · `operators/npu-dra-driver/` (Phase 5 allocator)
