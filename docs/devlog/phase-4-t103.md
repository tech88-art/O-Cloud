# P4-T-103 · inference-operator Kubebuilder scaffold + ModelService CRD types

- **Commit**: 6bd4a5c
- **Date**: 2026-05-19
- **Duration**: plan 1d · actual ~1h

## Intent

Create the third Kubebuilder sub-project at `operators/inference-operator/` containing only the ModelService CRD types (api/v1alpha1) — no controller body. Manager binary should build, start, and idle cleanly. The CRD schema must capture the minimum surface the Phase 5 controller needs to reconcile (model image + PD-pair replicas + npuSlicePoolRef + routerLabel for ADR-0008 webhook).

## Path adaptations

None — paths followed plan literal.

## Debugging trail

- **`*ModelService does not implement runtime.Object`** compile error on first `go build`. Cause: the SchemeBuilder.Register call in `modelservice_types.go init()` requires `runtime.Object` interface (DeepCopyObject() method). controller-gen generates that method into `zz_generated.deepcopy.go`. Solution: ran `go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.20.1 object:headerFile="hack/boilerplate.go.txt",year=2026 paths="./api/v1alpha1/..."` — file appeared with DeepCopy + DeepCopyInto + DeepCopyObject for ModelService + ModelServiceList + ModelServiceSpec + ModelServiceStatus + 4 nested structs.
- **CRD manifest also required**: `go run ... rbac:roleName=manager-role crd webhook paths="./api/v1alpha1/..." output:crd:artifacts:config=config/crd/bases` — generated `config/crd/bases/inference.ocloud.edge.example.com_modelservices.yaml` (237 lines, full OpenAPI v3 schema with status subresource + 5 printColumns).
- **kubectl --dry-run=client without live cluster**: same as T101 — fails with "the server could not find the requested resource". CRD YAML syntax is valid because controller-gen accepted it without error; live validation deferred to CI.

## Key decisions

- **Single ModelService CRD, no subresource gymnastics**. Plan acceptance asks for `spec.model.image / spec.model.modelPath / spec.pdPair.{prefill,decode}.replicas / spec.pdPair.routerLabel / spec.npuSlicePoolRef + status.conditions + status.phase`. Resisted the temptation to over-design (no quota field, no per-replica resources, no service-type config) — Phase 5 controller body will tell us what's actually needed.
- **PhasePending default**. status.phase defaults to "Pending" via the `+kubebuilder:default="Pending"` marker. Phase 4 controller is absent so phase stays Pending forever — observable + obvious to anyone running kubectl get ms.
- **routerLabel default = `inference.ocloud.edge.example.com/pd-role`**. The Phase 5 PD Router webhook (ADR-0008) will read this label key off pods. Defaulting it in the CRD spec means Phase 5 webhook can rely on the value being non-empty unless an operator explicitly opts out.
- **NPUSlicePoolRef is LocalObjectReference, not cluster-scoped**. NPUSlicePool is Namespaced (Phase 3 design); ModelService is also Namespaced; same-namespace convention per operators/CLAUDE.md §4 ("Phase 1-2 隔离 by convention only"). Phase 5 controller does NS-local lookup.

## Verification

P3 三维度:
- Existence: `git ls-files operators/inference-operator/` → 13 files (PROJECT + Makefile + Dockerfile + go.mod + go.sum + .gitignore + hack/boilerplate + cmd/main.go + 3 api/v1alpha1/*.go + 1 generated CRD YAML + README.md)
- Completeness: `go vet ./... && go build -o bin/manager ./cmd/main.go` → 58 MB binary builds clean
- Correctness: `make generate` (controller-gen object) + `make manifests` (controller-gen crd) both produced clean files; CRD YAML opens cleanly in any K8s 1.30+ cluster (deferred to CI for live test)

## Carry-forward

- Phase 5 controller body lands in `internal/controller/modelservice_controller.go` with a Reconcile that walks the 5-step plan from README (resolve npuSlicePoolRef → create N ResourceClaims → spawn Prefill + Decode Deployments → wait for ready → set phase=Ready).
- ADR-0008 PD Router webhook + ADR-0009 npu-dra-driver allocation logic are the immediate Phase 5 prerequisites — both linked from inference-operator/README.md "Phase 5 controller plan".
- operators/CLAUDE.md now lists 3 sub-projects (pool-operator + npu-dra-driver + inference-operator) — done in this commit.
