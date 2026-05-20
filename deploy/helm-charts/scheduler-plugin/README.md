# scheduler-plugin Helm chart

> O-Cloud custom **kube-scheduler** with HCCSTopology (Filter+Score) +
> NumaAffinity (placeholder; deferred per T006) + Binpack (Score, default
> disabled). Phase 6 W2 T101.

## Install

```bash
helm install npu-scheduler deploy/helm-charts/scheduler-plugin \
  --namespace kube-system \
  --create-namespace \
  --set image.repository=scheduler-plugin \
  --set image.tag=v0.1.0
```

After install, Pods opt in via `spec.schedulerName: npu-scheduler`.
The default-scheduler is unaffected for all other workloads.

## Values reference

| Key | Type | Default | Description |
|---|---|---|---|
| `image.repository` | string | `scheduler-plugin` | Container image repository |
| `image.tag` | string | `"v0.1.0"` | Container image tag (matches `appVersion`) |
| `image.pullPolicy` | string | `IfNotPresent` | Image pull policy |
| `profileName` | string | `npu-scheduler` | KubeSchedulerConfiguration profile name; Pods consume via `spec.schedulerName` |
| `replicaCount` | int | `1` | Number of scheduler replicas (leader election ensures only one active) |
| `leaderElect` | bool | `true` | Enable kube-scheduler leader election |
| `leaderElectionNamespace` | string | `kube-system` | Namespace for the leader-election Lease |
| `securePort` | int | `10259` | HTTPS port for /healthz + /metrics |
| `hccsTopology.weight` | int | `5` | HCCSTopology Score weight [0..100] |
| `hccsTopology.preferAnnotation` | string | `npu.huawei.com/preferred-hccs-ring` | Pod annotation Filter / Score reads |
| `hccsTopology.failIfMissing` | bool | `false` | Strict Filter when Pod lacks the annotation |
| `hccsTopology.adjacency` | map | `{}` | Ring-to-adjacent-rings map (string keys) for Score tier 70 (adjacent) |
| `numaAffinity.enabled` | bool | `false` | **DEFERRED** — set true ONLY after upstream sched-plugins v0.32.x wrap lands per T006 |
| `numaAffinity.weight` | int | `2` | NumaAffinity Score weight; ignored when disabled |
| `binpack.enabled` | bool | `false` | Enable Binpack Score (opt-in) |
| `binpack.weight` | int | `1` | Binpack Score weight; ignored when disabled |
| `binpack.resourceWeights` | map | `{}` | Override per-resource weights (default in args.go: cpu=1, memory=1, NPU=5) |
| `serviceAccount.create` | bool | `true` | Create chart-managed ServiceAccount |
| `rbac.create` | bool | `true` | Create chart-managed ClusterRoles + Bindings |
| `rbac.bindToUpstream` | bool | `true` | Bind ServiceAccount to upstream `system:kube-scheduler` + `system:volume-scheduler` ClusterRoles |

## How the chart maps to operators/scheduler-plugin

| Chart template | Source contract |
|---|---|
| `templates/deployment.yaml` | `operators/scheduler-plugin/cmd/main.go` (binary entrypoint) + `Dockerfile` (image) |
| `templates/configmap.yaml` | KubeSchedulerConfiguration profile rendered against `hccsTopology` + `binpack` + `numaAffinity` values; matches ADR-0010 §1 / §4 args schemas |
| `templates/rbac.yaml` (upstream binding) | covers kube-scheduler operational reads (nodes/pods/bindings/leases/events/storageclasses) via `system:kube-scheduler` + `system:volume-scheduler` |
| `templates/rbac.yaml` (ocloud ClusterRole) | adds `resource.k8s.io/v1beta1` (ResourceClaim/Slice/DeviceClass) + `npu.ocloud.edge.example.com/v1alpha1.NPUSliceAllocation` reads |

## NumaAffinity deferral

Per ADR-0010 §3 + operators/scheduler-plugin/DESIGN.md §5.2, the
NumaAffinity plugin is currently a placeholder. The chart OMITS it from
the rendered KubeSchedulerConfiguration filter/score enabled lists when
`numaAffinity.enabled=false` (the default). Operators flip the toggle
once the upstream sched-plugins v0.32.x wrap lands AND
`operators/scheduler-plugin/internal/plugins/numa/plugin.go` body is
upgraded from placeholder to real wrap.

## Known issues

See `docs/known-issues.md` entry **#11** (Phase 6 T101 add): running
this chart as a SECOND scheduler alongside the default-scheduler means
Pods MUST explicitly set `spec.schedulerName: npu-scheduler` to consume
HCCS-aware placement. inference-operator deployment_builder polish at
P6-T-105 may stamp this automatically; until then, application teams
opt in manually.

## References

- `docs/adr/0010-scheduler-plugin.md` — design contract (CNI selection,
  args schema, Phase 7 forward notes)
- `operators/scheduler-plugin/DESIGN.md` — module detailed design
- `docs/phase6-plan.md` §4 P6-T-101 — task acceptance
- `docs/cni-hccl-research.md` §4 — Phase 6 entry CNI recommendation
- upstream: [sigs.k8s.io/scheduler-plugins](https://github.com/kubernetes-sigs/scheduler-plugins)
