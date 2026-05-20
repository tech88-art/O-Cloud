# scheduler-plugin

> O-Cloud custom **kube-scheduler** binary with HCCSTopology + NumaAffinity +
> Binpack plugins compiled in via `sigs.k8s.io/scheduler-plugins` framework.
> Phase 6 scaffold (P6-T-002); Filter/Score bodies arrive T004-T007.

## Status

**Phase 6 scaffold (P6-T-002)** — `bin/kube-scheduler` builds and prints upstream
flags; no plugin Filter/Score logic yet:

- **P6-T-001** (ADR-0010 lands at `cfa6260` · 2026-05-20) freezes design.
- **P6-T-002** (this commit) ships the scaffold: cmd/main.go + 3 placeholder
  plugin packages + go.mod + Makefile + Dockerfile + PROJECT + this README +
  module DESIGN.md deferred to T004 controller-body task (per root CLAUDE.md
  §14.2).
- **P6-T-003** (pool-operator) populates `NPUPool.status.hccsTopology` —
  consumed indirectly by HCCSTopology Plugin via ResourceSlice attribute
  reads.
- **P6-T-004** HCCSTopology Filter body.
- **P6-T-005** HCCSTopology Score body + NPUSliceAllocation reverse-lookup.
- **P6-T-006** NumaAffinity body (wraps upstream `noderesourcetopology`).
- **P6-T-007** Binpack body (internal ~50 LOC Score).
- **P6-T-008** Plugin unit + integration envtest.
- **P6-T-101** Helm chart wraps the binary as a Deployment with
  KubeSchedulerConfiguration ConfigMap.

## Why a separate scheduler binary

Per ADR-0010 §1 — multi-scheduler form, not default-scheduler patch:

- **Opt-in safety**: Pods explicitly set `spec.schedulerName=npu-scheduler`
  to use this scheduler. Default scheduler is unaffected for all other
  workloads.
- **Operational isolation**: a bug in HCCS scoring cannot break the rest of
  the cluster's scheduling.
- **Upgrade path**: bumping `sigs.k8s.io/scheduler-plugins` version =
  bumping this binary's go.mod. Default scheduler upgrades on its own
  cadence with the K8s control plane.

## Framework + version pinning

- `sigs.k8s.io/scheduler-plugins` v0.31.x — pinned to K8s 1.32 baseline
  (kind smoke per P5-T-114). Patch revision picked at T002 entry from
  latest release notes.
- `k8s.io/*` libraries pinned to v0.32.0 via go.mod replace block (upstream
  scheduler-plugins requires this pattern; without it `go mod download`
  resolves `k8s.io/kubernetes` to staging-tree pseudo-versions and fails).

## Getting started

```bash
cd operators/scheduler-plugin

# Scaffold-time loop:
make build        # produces bin/kube-scheduler
./bin/kube-scheduler --help   # prints upstream kube-scheduler flags

# Run tests (empty placeholder packages → 0 tests; T004-T008 fill in):
make test

# Docker build (used by P6-T-101 helm chart):
make docker-build IMG=scheduler-plugin:v0.1.0
```

## Plugin registry

| Plugin name      | Extension points    | Status (T002) | Body lands |
|------------------|---------------------|---------------|------------|
| `HCCSTopology`   | Filter, Score        | Name()-only placeholder | T004 (Filter), T005 (Score) |
| `NumaAffinity`   | Filter, Score        | Name()-only placeholder | T006 (wraps upstream)        |
| `Binpack`        | Score                | Name()-only placeholder | T007 (internal ~50 LOC)     |

T002 placeholders return a struct whose `Name()` matches the const so
`app.WithPlugin(Name, New)` registers cleanly. Without a FilterPlugin /
ScorePlugin interface implementation, kube-scheduler does not invoke the
plugin at admission — the binary builds + runs but is functionally a
no-op until T004+.

## Directory layout

```
operators/scheduler-plugin/
├── PROJECT                    sub-project metadata (not Kubebuilder)
├── Makefile                   build / vet / test / lint / docker targets
├── Dockerfile                 multi-stage distroless build
├── README.md                  this file
├── go.mod                     sched-plugins v0.31.x + k8s 1.32 replace block
├── hack/                      boilerplate.go.txt for Apache 2.0 license header
├── cmd/main.go                entrypoint: app.NewSchedulerCommand + 3 WithPlugin registers
└── internal/plugins/          one package per plugin
    ├── hccs/plugin.go         HCCSTopology placeholder (T004/T005 fill in)
    ├── numa/plugin.go         NumaAffinity placeholder  (T006 wraps upstream)
    └── binpack/plugin.go      Binpack placeholder       (T007 internal Score)
```

DESIGN.md per root CLAUDE.md §14.2 convention is **deferred to T004**
(controller-body equivalent task) — scaffold task by definition has no
body to document.

## Phase 6 simulator scope

**This scheduler does NOT depend on real Ascend hardware or specific CNI.**
Per ADR-0010 §6 — CNI-portable design: the plugins read only ResourceSlice
attributes (`npu.huawei.com/hccs_ring` / `numa_node`), NodeResourceTopology
CR (upstream), and Pod annotations (`npu.huawei.com/preferred-hccs-ring`,
`inference.ocloud.edge.example.com/model-service`). All of these are
populated upstream from the simulator path or by deployment tooling.

Real hardware verification deferred to Phase 7 lab phase (per cann-driver-
matrix §4 entry checklist).

## References

- `docs/adr/0010-scheduler-plugin.md` — design freeze (Filter+Score
  semantics, plugin args schema, CNI selection, Phase 7+ forward notes)
- `docs/adr/0009-npu-dra-driver.md` §6.2 — topology-aware scoring forward
  note that motivates this plugin
- `docs/cni-hccl-research.md` §4 — Phase 6 entry recommendation
  (Cilium + Multus + SR-IOV primary)
- `docs/architecture.md` §5.6 — scheduler-plugin module placement
- `docs/architecture.md` §6.3 — `HCCSTopologyInfo` struct (T003 source)
- `docs/phase6-plan.md` §3 P6-T-002 — task acceptance
- `operators/npu-dra-driver/DESIGN.md` §6.3.4 — NPUSliceAllocation reverse-
  lookup index that T005 consumes
- `operators/inference-operator/DESIGN.md` §4.2 — `inference.ocloud.edge.
  example.com/model-service` label objectSelector (Score grouping key)
- upstream: [sigs.k8s.io/scheduler-plugins](https://github.com/kubernetes-sigs/scheduler-plugins)
- upstream: [scheduler-plugins/pkg/noderesourcetopology](https://github.com/kubernetes-sigs/scheduler-plugins/tree/master/pkg/noderesourcetopology) (T006 wraps)
