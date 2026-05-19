# inference-operator

> O-Cloud inference-operator. Phase 4 scaffold — ships the ModelService CRD
> types only. The controller body lands Phase 5 per ADR-0008 (PD Router
> webhook impl) + ADR-0009 (npu-dra-driver allocation logic).

## Status

**Phase 4 (P4-T-103) — scaffold only.** Manager binary builds, starts, and
idles cleanly. No Reconcilers registered. `kubectl apply` on the CRD YAML
creates the ModelService API type; in-cluster objects stay at
`status.phase=Pending` forever until Phase 5 lands the controller.

## Why a custom operator (not KServe)

Per `docs/adr/0002-no-kserve.md`: spec line 47-48 prohibits KServe. The
inference serving topology is vllm-ascend Deployments + a Phase 5 PD Router
mutating admission webhook (ADR-0008). The inference-operator owns the
ModelService CRD that captures both sides of the PD-pair plus the
NPUSlicePoolRef binding.

## Getting started

```bash
cd operators/inference-operator

# Phase 4 scaffold-time loop:
make build              # produces bin/manager
make run                # runs manager on host kubeconfig (no controllers registered)

# CRD apply (Phase 5 controller will reconcile these once it lands):
kubectl apply -f config/crd/bases/

# Phase 5 will add: make docker-build IMG=inference-operator:v0.1.0
```

## ModelService CRD shape

The Phase 4 spec captures the minimum surface the Phase 5 controller
needs to reconcile, no more. Field-by-field semantics live in
`api/v1alpha1/modelservice_types.go` godoc.

```yaml
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
status:
  phase: Pending          # Phase 4 stays Pending forever (no controller)
  conditions: []
  observedGeneration: 0
```

## Phase 5 controller plan

1. Watch ModelService; on create, set `phase=Provisioning`.
2. Resolve `npuSlicePoolRef` → list available slices → create N
   ResourceClaims via npu-dra-driver (one per pod across both PD sides).
3. Create two Deployments (Prefill + Decode), label each with
   `routerLabel: prefill|decode`, mount the model image.
4. ADR-0008 PD Router webhook stamps `npu.huawei.com/slice-bindings` on
   accepted pods so the scheduler-plugin (Phase 6) routes them to the
   correct NUMA + HCCS group.
5. `phase=Ready` when all Pods report Ready.

## Directory layout

```
operators/inference-operator/
├── PROJECT                       Kubebuilder v4 metadata
├── Makefile                      mirrors npu-dra-driver
├── Dockerfile                    scaffold; real chart build Phase 5
├── cmd/main.go                   manager wiring (no controllers)
├── go.mod / go.sum               k8s.io/api v0.35.0 · controller-runtime v0.23.3
├── hack/boilerplate.go.txt       Apache 2.0 header
├── api/v1alpha1/                 ModelService types (T103)
│   ├── groupversion_info.go
│   ├── modelservice_types.go
│   └── zz_generated.deepcopy.go
├── config/crd/bases/             generated CRD YAML
└── README.md                     this file
```

## References

- `docs/adr/0002-no-kserve.md` — why not KServe
- `docs/adr/0008-pd-router-webhook.md` — PD Router admission webhook design
- `docs/adr/0009-npu-dra-driver.md` — slice ↔ ResourceClaim semantic mapping (lands at P4-T-105)
- `docs/architecture.md` §5.4 — inference-operator module placement
- `docs/phase4-plan.md` §3 P4-T-103 — task acceptance
- `operators/npu-dra-driver/` — Phase 5 will allocate via this DRA driver
- `operators/pool-operator/` — Phase 5 ModelService binds to NPUSlicePool here

## License

Apache 2.0 — see project root `LICENSE`.
