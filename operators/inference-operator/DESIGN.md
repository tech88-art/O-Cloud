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

### 4.3 PD Router webhook (operative, T102 scaffold → T103 mutate)

The PD Router admission webhook lives in the SAME binary as the
controller (ADR-0008 design choice: one container, one cert lifecycle,
one helm release). Path `/mutate-pod`. controller-runtime webhook
server on port 9443.

Wiring (T102 scaffold):
- `internal/webhook/pd_router.go` ships the `PDRouter` admission
  Handler. Every request currently returns Allowed without patch;
  T103 replaces the early-return with annotation-injection logic.
- `cmd/main.go` registers the handler behind
  `--enable-pd-router-webhook` flag (default true). Webhook server
  port wired via `ctrl.Options.WebhookServer = ctrlwebhook.NewServer
  (Options{Port: 9443})`.
- Helm chart renders four objects when `pdRouter.enabled=true`:
  - `Service` — `<release>-inference-operator-webhook`, port 443 →
    targetPort 9443
  - `MutatingWebhookConfiguration` — references the Service via
    `clientConfig.service`, objectSelector keys on
    `inference.ocloud.edge.example.com/model-service Exists`,
    `failurePolicy=Fail` per ADR-0008 (override via values)
  - `Certificate` + `Issuer` (T101) — webhook-tls Secret mounted at
    `/tmp/k8s-webhook-server/serving-certs/`
- `cert-manager.io/inject-ca-from` annotation on the
  MutatingWebhookConfiguration → cert-manager populates
  `clientConfig.caBundle` at runtime. Chart YAML stays caBundle-free
  through cert rotations.

T103 replaces the T102 always-Allow body with:
- decode Pod → read model-service label → look up ModelService →
  list NPUSliceAllocation by claim-namespace + claim-name labels →
  build `npu.huawei.com/slice-bindings = <node>/<slice>/<device>:<aiCores>
  [,...]` → return admission.Patched.

### 4.2.3 P5-T-008 Phase machine (operative)

```
                    ┌─────────────┐
   first reconcile  │  Pending    │   (default in CRD; immediately
   ───────────────► │             │    advanced on first reconcile)
                    └──────┬──────┘
                           │
                  pool not found
                           │
            ┌──────────────▼──────────────┐
            │   Failed (PoolUnresolved)   │   terminal — auto-
            └──────────────────────────────┘   recovers if spec
                                               edit changes pool
                                               ref

                    ┌─────────────┐
                    │ Provisioning│   (reconcileChildren created
                    │             │    Deployments + claim templates;
                    └──────┬──────┘    waiting for readiness)
                       ┌───┴────────┐
                       │            │
              ready    │            │  image-pull error or
              + claims │            │  >10min progress
              allocated│            │  deadline exceeded
                       ▼            ▼
                    ┌────┐      ┌────────┐
                    │Ready│     │ Failed │ (auto-recover via
                    │     │     │        │  spec edit removing
                    └──┬──┘     └────────┘  the cause)
                       │
                       │  spec edit
                       │  (scale up / image change)
                       ▼
                    ┌─────────────┐
                    │ Provisioning│  ← back, waiting for new
                    └─────────────┘    replicas to become Ready
```

Inputs aggregated each reconcile (PhaseInputs in phases.go):
- `PrefillDesired` / `DecodeDesired` from spec.pdPair
- `PrefillReady` / `DecodeReady` from Deployment.Status.ReadyReplicas
- `PrefillProgressing` / `DecodeProgressing` from Deployment.Status
  .Conditions[Type=Progressing] (image-pull detection)
- `ClaimsTotal` / `ClaimsAllocated` from ResourceClaim list filtered
  by `inference.ocloud.edge.example.com/model-service` label;
  allocated means non-nil Status.Allocation with at least one entry
  whose Driver == npu.ocloud.edge.example.com

Outputs (Status.Conditions):
- `Available` — True when phase=Ready; False with reason
  `ProvisioningInProgress` / `ImagePullError` / `ProgressDeadlineExceeded`
- `ProgressDeadline` — True when stuck Provisioning > 10min; False
  during normal Provisioning / Ready
- `AllocationReady` — True when ClaimsTotal>0 AND ClaimsAllocated ==
  ClaimsTotal

Reconcile cadence: while phase=Provisioning, requeue 30s so the
controller polls Deployment + claim readiness without external watch
events. Ready / Failed leave the queue idle (next reconcile triggered
by spec edit or watch on owned resources).

ProgressDeadline default 10 minutes; injectable via the Reconciler's
`Now` field for tests. Image-pull detection reads upstream `Reason`
strings (`ImagePullBackOff` / `ErrImagePull`).

### 4.2.2 P5-T-007 Deployment + Claim materialisation (operative)

Per-side resources created/updated by `reconcileChildren()` on every
pool-found pass:

1. **ResourceClaimTemplate** (per side): named `<ms.name>-<side>-claim`
   in `ms.namespace`. Spec carries:
   - `Spec.Spec.Devices.Requests[0].DeviceClassName = "npu.ocloud.edge.example.com"`
     (T001 DeviceClass registered by npu-dra-driver chart)
   - `Spec.ObjectMeta.Annotations`:
     - `ocloud.edge.example.com/model-service-ref = <ns>/<name>`
     - `ocloud.edge.example.com/preferred-pool = <spec.npuSlicePoolRef.name>`
   These annotations propagate to each per-replica ResourceClaim that
   K8s creates from the template at Pod scheduling — npu-dra-driver
   T002 reads them during allocation.
2. **Deployment** (per side): named `<ms.name>-<side>` in
   `ms.namespace`. Spec:
   - `Replicas = ms.Spec.PDPair.<side>.Replicas`
   - Selector + Pod template labels include
     `<routerLabel> = <side>` (default key
     `inference.ocloud.edge.example.com/pd-role`) and
     `inference.ocloud.edge.example.com/model-service = <ns>/<name>`
   - `Spec.Template.Spec.ResourceClaims[0]`:
     - `Name = "npu-slice"`
     - `ResourceClaimTemplateName = <template-name>`
     - Container.Resources.Claims references the same `"npu-slice"` alias
   - Container image = `ms.Spec.Model.Image`
   - Container args: `--model-path=<ms.Spec.Model.ModelPath>` +
     `--pd-role=<side>`
   - OwnerRef → ModelService (Controller=true, BlockOwnerDeletion=true)

Drift detection (idempotent reconcile):
- Replica count change → in-place Deployment update (no recreate).
- Image change → in-place container update; rolling-update strategy
  fires automatically via Deployment defaults.
- ResourceClaimTemplate.Spec.Spec is immutable per upstream contract;
  Phase 5 T007 leaves Spec drift alone and only refreshes labels.
  T008 may delete + recreate on a model-spec change.

"Per-replica ResourceClaim" satisfied via the standard DRA pattern:
ResourceClaimTemplate produces one ResourceClaim per Pod replica at
admission time. The plan acceptance "Per-replica ResourceClaims
created" is met because K8s — not the inference-operator — owns the
expansion. The inference-operator's responsibility ends at the
template.

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

## 5.0 PD proxy_server sidecar pattern (Phase 6 T105 · operative)

Per ADR-0010 §3 forward-notes and plan §3 P6-T-105 (gated decision on
vllm-ascend v0.12+ stability), T105 ships:

**Schema additions** (`api/v1alpha1.PDPairSpec`):

```go
type PDPairSpec struct {
    // ... Phase 5 fields (Prefill, Decode, RouterLabel) ...

    // ProxyImage opts in to vllm-ascend disaggregated_prefill_v1
    // proxy_server sidecar. Empty (default) keeps the Phase 5
    // single-container behavior.
    ProxyImage    string `json:"proxyImage,omitempty"`

    // FallbackImage overrides spec.model.image on the main container
    // in CI / kind smoke environments where pulling the real ~5GB
    // vllm-ascend image is too expensive. Production deployments
    // leave empty.
    FallbackImage string `json:"fallbackImage,omitempty"`
}
```

**Container materialisation** (`internal/controller/deployment_builder.go
buildPDPairContainers`):

```
ms.Spec.Model.Image                       → vllm-ascend (main)
ms.Spec.PDPair.FallbackImage (override)   → busybox / smoke (main, when set)
ms.Spec.PDPair.ProxyImage (opt-in)        → pd-proxy (sidecar, when set)
```

The proxy sidecar's env-var contract (matches upstream vllm-ascend
disaggregated_prefill_v1/proxy_server.py contract — to be exercised
once vllm-ascend v0.12+ stabilises):

| Env var | Value |
|---|---|
| `VLLM_PD_ROLE` | "prefill" or "decode" (this Pod's side) |
| `VLLM_PD_PREFILL_HOST` | `<ms.Name>-prefill` (Service name convention) |
| `VLLM_PD_DECODE_HOST` | `<ms.Name>-decode` |
| `VLLM_PD_SIBLING_LOCALHOST_PORT` | `"8000"` (proxy_server reaches local vllm-ascend over localhost:8000) |
| `VLLM_PD_OTHER_SIDE` | "prefill" if this Pod is decode, "decode" if prefill |

**Why split FallbackImage vs ProxyImage**: FallbackImage is for CI
expedience (avoiding 5GB image pulls); ProxyImage is a production
capability (adding a sidecar to enable PD-pair traffic routing).
Distinct fields so operators don't accidentally activate the proxy by
opting into a busybox-style stand-in.

**Forward path (Phase 7+)**: once vllm-ascend v0.12+ documents the
proxy_server contract on stable, the env-var key names above may
shift. T105 chose names matching the current upstream conventions
(VLLM_PD_*); pre-emptive standardisation, may need a rename pass when
upstream finalises.

### 5.0.1 Phase 7 P7-T-102 vllm-ascend v0.12+ status (2026-05-20)

**GA status check (P7-T-102 gating decision)**:

| Version    | Date          | GA status            |
|------------|---------------|----------------------|
| v0.11.0    | (Phase 5 ref) | GA (used as Phase 4-6 baseline in PDPairSpec.Model.Image godoc)        |
| v0.13.0    | (last v0.13)  | GA (final release of v0.13.0 line)        |
| v0.18.0    | 2024-04-30    | **GA · "Latest" tag**  |
| v0.19.1rc1 | 2024-04-30    | Pre-release (release-candidate) |

**Outcome**: doc-only refresh per phase7-plan §4 P7-T-102 fallback
branch. The v0.12+ gating condition IS met (v0.13.0 + v0.18.0 both
GA), but flipping the chart `defaults.proxyImage` to a concrete tag
without verifying:
  - CI image-pull access from GHA runners (image size ~5GB · GHA disk
    budget concern · `quay.io/vllm-project/vllm-ascend` exact tag
    convention)
  - Phase 6 T106 + Phase 7 T103 kind smoke continues to use
    `fallbackImage=busybox` to skip real pulls (per Phase 6 T106
    pattern · preserved here)

… is judged risky for the chart's tested code path. **Operators
wanting v0.18.0 proxy sidecar** set
`ms.Spec.PDPair.ProxyImage="quay.io/vllm-project/vllm-ascend:v0.18.0"`
explicitly (or whatever current stable they prefer); chart default
stays empty (Phase 5 single-container behavior preserved). When real
lab cluster + image pull verified in Phase 10 demo polish, chart
default can flip cleanly.

**Cross-references**: ADR-0010 §3 forward notes + phase7-plan.md §4
P7-T-102 doc-only fallback acceptance · ADR-0011 §3 lab gating policy
(Phase 7 W2 decisions defer cleanly to Phase 10 when verification
infrastructure not in place).

T106 kind smoke fixture uses FallbackImage=busybox:1.36 to demonstrate
the CI pattern. Production deployments leave both ProxyImage and
FallbackImage empty until vllm-ascend v0.12+ stability assessment.

## 5.1 Metrics exposition (Phase 6 T104 · operative)

Three Prometheus collectors registered against controller-runtime's
shared registry, served on the manager's `--metrics-bind-address`
(default `:8082`):

| Collector | Type | Labels | Source |
|---|---|---|---|
| `inference_modelservice_phase_transitions_total` | Counter | `from`, `to` | `internal/controller/modelservice_controller.go` Reconcile — incremented only when `Status.Phase` actually changes (recompute-same-phase is a no-op) |
| `inference_pdrouter_decisions_total` | Counter | `decision` ∈ {allowed_no_patch, patched, denied} | `internal/webhook/pd_router.go` Handle — recorded via deferred wrap that inspects every return path |
| `inference_modelservice_reconcile_duration_seconds` | Histogram | (none) | `internal/controller/modelservice_controller.go` Reconcile — observed via `defer ObserveReconcileDuration(time.Since(start).Seconds())` |

Buckets for the histogram cover the expected simulator-scope range
(`{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10}` seconds).

**Plan scope reduction**: plan §4 P6-T-104 originally listed 4
collectors including "allocator picks", but that one was dropped at
task entry because inference-operator does NOT call npu-dra-driver's
allocator directly — allocation happens via DRA's ResourceClaim
machinery. Allocator metrics belong in npu-dra-driver (Phase 7+).

**Chart wiring** (`deploy/helm-charts/inference-operator/`):
- `templates/service-metrics.yaml` — separate Service (port 8082) from
  the webhook Service so operators can scrape metrics independent of
  PD Router lifecycle
- `templates/servicemonitor.yaml` — opt-in via
  `metrics.serviceMonitor.enabled=true` (requires Prometheus Operator
  CRDs in the cluster)
- `values.yaml` block:
  ```yaml
  metrics:
    enabled: true
    port: 8082
    serviceMonitor:
      enabled: false
      interval: 30s
      scrapeTimeout: 10s
      labels: {}
      relabelings: []
  ```

## 5.2 Scheduler routing (Phase 7 T003 · operative)

Phase 7 P7-T-003 (ADR-0011 §1 · closes known-issues #11) wires
`deployment_builder` to auto-stamp `spec.schedulerName="npu-scheduler"`
on every PD-pair Pod template the controller materialises. Operators
opt out via `ms.Spec.SchedulerOverride`.

**Why**:Phase 6 T101 ships scheduler-plugin (`operators/scheduler-plugin/`)
as a SECOND scheduler — Pods opt in via `spec.schedulerName=npu-scheduler`.
Without this auto-stamp, ModelService Pods schedule via default-scheduler
and bypass HCCSTopology Filter+Score / Binpack. Phase 6 known-issues #11
documented this gap; Phase 7 T003 closes it.

**Cross-reference**: ADR-0010 §6.1 (multi-scheduler design) + ADR-0011 §1
(P7-T-003 lands deployment_builder auto-stamp) + known-issues #11 (RESOLVED
via T003).

### 5.2.1 Resolution table

```
ms.Spec.SchedulerOverride         → effective spec.schedulerName
-----------------------------------+--------------------------------
nil (default)                      | "npu-scheduler"
*ptr → ""                          | "npu-scheduler"  (empty == nil)
*ptr → "default-scheduler"         | "default-scheduler"
*ptr → "volcano-scheduler"         | "volcano-scheduler"
```

The empty-string-pointer path is intentional safety: a chart-rendered
ModelService with `schedulerOverride: ""` (e.g. accidental empty default
in a Helm chart template) STILL gets `npu-scheduler` auto-stamped.
Operators wanting default-scheduler must explicitly set a non-empty
override.

### 5.2.2 Impl

- Constant `SchedulerNameDefault = "npu-scheduler"` in
  `internal/controller/deployment_builder.go` — matches the chart
  profileName per ADR-0010 §1 (changing one without the other = silent
  HCCS bypass).
- Function `effectiveSchedulerName(ms)` returns the resolution-table
  output; called inline in `buildDeployment` Pod template Spec.
- Field `ModelServiceSpec.SchedulerOverride *string` (pointer, optional)
  — kubebuilder marker `+optional`; CRD YAML generates with
  `schedulerOverride: string` at `spec.schedulerOverride` JSONPath.

### 5.2.3 Test coverage

`internal/controller/deployment_builder_test.go::TestEffectiveSchedulerName`
covers all 4 resolution-table rows plus the buildDeployment round-trip
(asserts the Pod template actually carries the stamped name + reacts to
override mutation). 4 sub-tests pass via `go test`.

### 5.2.4 kind smoke assertion (Phase 7 T103 forward)

Phase 6 T106 kind smoke had a warning-not-fail `assert_scheduler_name`
check; Phase 7 T103 flips it to HARD FAIL on absence per phase7-plan.md
§4 T103 acceptance. The chart-rendered modelservice fixture must produce
Pods carrying `spec.schedulerName=npu-scheduler` — any Pod missing it
fails the workflow.

---

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
