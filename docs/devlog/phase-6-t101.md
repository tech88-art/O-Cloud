# P6-T-101 · scheduler-plugin Helm chart + KubeSchedulerConfiguration

- **Commit**: <pending>
- **Date**: 2026-05-20
- **Duration**: plan 1d · actual ~30min

## Intent

Land a Helm chart that deploys the Phase 6 scheduler-plugin binary as
a second kube-scheduler in the cluster. KubeSchedulerConfiguration is
embedded in a ConfigMap rendered from `hccsTopology` / `binpack` /
`numaAffinity` values. Pods opt in via `spec.schedulerName:
npu-scheduler` per ADR-0010 §1.

## Path adaptations

- **NumaAffinity OMITTED from default chart enable lists**: per T006
  deferral (upstream sched-plugins v0.31.8 references K8s 1.31's
  `framework.GVK` removed in K8s 1.32). Chart values.yaml has
  `numaAffinity.enabled: false` default; both the filter and score
  rendered lists conditionally include NumaAffinity. Future task
  flips this when the upstream wrap lands (one values.yaml edit).

- **Two ClusterRoleBindings to upstream system: roles**: instead of
  re-implementing the full kube-scheduler RBAC surface from scratch,
  the chart binds the ServiceAccount to both `system:kube-scheduler`
  and `system:volume-scheduler` (both ship with every K8s cluster).
  This pattern is recommended by kube-scheduler upstream docs for
  out-of-tree scheduler deployments. A separate chart-managed
  ClusterRole (`<release>-ocloud`) adds the Ocloud-specific reads
  for resource.k8s.io + npu.ocloud.edge.example.com/v1alpha1.

- **`rbac.bindToUpstream` toggle**: lets operators with custom
  ClusterRole replacements opt out of the upstream bindings while
  keeping the chart-managed ocloud-extension binding. Default true.

- **Known-issue #11 added**: documents the "must set schedulerName"
  requirement so operators don't silently get default-scheduler
  placement when expecting HCCS-aware. T105 inference-operator
  polish is the proposed long-term fix (stamp schedulerName in the
  deployment_builder).

- **leaderElectionNamespace defaults to kube-system**: matches the
  upstream kube-scheduler default. Operators deploying in a custom
  namespace can override via values.yaml.

- **`--authentication-kubeconfig=` / `--authorization-kubeconfig=`
  empty**: empty strings tell kube-scheduler to use the in-cluster
  service account credentials (mounted at
  `/var/run/secrets/kubernetes.io/serviceaccount/`) instead of
  external kubeconfig files. This is the standard pattern for
  in-cluster scheduler deployments.

## Debugging trail

- **`helm lint --strict` initially flagged `icon is recommended`**:
  Chart.yaml doesn't have an `icon:` field. This is INFO-level
  (not an error); ignored.

- **YAML parse via stdin failed on Windows console encoding**:
  Python's yaml.safe_load_all choked on a Unicode character at
  position 3034 when piped through Windows shell stdin. Rerouted
  via local temp file (`> rendered.yaml`) — parsed cleanly. 7 K8s
  objects.

- **Verified two rendering paths**:
  - Default (binpack disabled, numa disabled): only HCCSTopology
    appears in filter/preScore/score plus its pluginConfig
  - `--set binpack.enabled=true`: adds Binpack to score chain +
    disables NodeResourcesFit + appends Binpack pluginConfig
  - `--set hccsTopology.adjacency."0"='{1,2}'`: renders adjacency
    map under HCCSTopology args correctly

## Key decisions

- **Multi-doc YAML with `---` separators in rbac.yaml**: cleaner
  than splitting into 4 files (one per ClusterRoleBinding +
  ClusterRole). Helm tolerates multi-doc YAML in template files.

- **checksum/config annotation on Deployment pod template**:
  forces pod rollout when the KubeSchedulerConfiguration ConfigMap
  changes (helm upgrade rebakes the config; without this the pod
  keeps reading the old config until manually restarted). Standard
  Helm pattern.

- **No Service object**: kube-scheduler exposes /healthz + /metrics
  on the secure port, but client-side use is by readiness probes
  (which the Deployment pod self-handles) and Prometheus scraping
  (configured separately via ServiceMonitor in a future task).
  Adding a Service preemptively bloats the chart for no current
  consumer.

- **Distroless image runs as nonroot (UID 65532)**: matches the
  upstream sched-plugins / Kubernetes pattern. SecurityContext is
  explicit in values.yaml; chart consumers can override for
  pod-security-standards Restricted compliance.

- **PreScore extension point explicitly in configmap**: framework
  auto-detects PreScorePlugin implementations, but listing PreScore
  in `plugins.preScore.enabled` makes the activation explicit + lets
  future operators know HCCSTopology participates here.

## Verification

P3 三维度:

- **Existence**:
  - `git status --short` → 8 new files under
    deploy/helm-charts/scheduler-plugin/ + known-issues.md
    modified + devlog
  - `ls deploy/helm-charts/scheduler-plugin/templates/` →
    _helpers.tpl, configmap.yaml, deployment.yaml, rbac.yaml,
    serviceaccount.yaml (5 templates)

- **Completeness** (plan §4 P6-T-101 acceptance):
  - `helm lint --strict` passes ✅ (1 chart, 0 failed; INFO about
    Chart.yaml icon is non-blocking)
  - `helm template` renders ServiceAccount + ClusterRole(s) +
    ClusterRoleBinding(s) + ConfigMap + Deployment ✅ (7 objects)
  - ConfigMap embeds valid KubeSchedulerConfiguration with profile
    `npu-scheduler` ✅
  - HCCSTopology Filter + Score enabled with weight 5 ✅
  - NumaAffinity OMITTED by default (T006 deferral) ✅
  - Binpack ENABLED only when values.binpack.enabled=true ✅
  - NodeResourcesFit disabled when Binpack enabled ✅
  - Pods opt in via `spec.schedulerName=npu-scheduler` (documented
    in README + known-issue #11) ✅
  - Phase 5 inference-operator deployment_builder NOT changed ✅
    (T105 will revisit)

- **Correctness**:
  - All 7 K8s objects parsed cleanly by Python yaml.safe_load_all
  - Adjacency map renders as nested object correctly
  - Binpack toggle path renders both Score enabled + disabled
    NodeResourcesFit correctly
  - RBAC binds to both upstream system: roles (kube-scheduler +
    volume-scheduler) by default; ocloud-specific role separate

## Carry-forward

- **T106 kind smoke** will:
  - install this chart with `image.repository=scheduler-plugin
    image.tag=v0.1.0` against a locally-built image
  - create a ModelService that triggers Prefill+Decode Pods
    carrying `spec.schedulerName=npu-scheduler`
  - assert Pods land on HCCS-co-located nodes per the fixture

- **T105 inference-operator polish**: deployment_builder may stamp
  `spec.schedulerName=npu-scheduler` automatically. Pending
  decision at T105 entry; known-issue #11 documents the current
  manual opt-in.

- **T107 checkpoint** will:
  - reference this chart's `phase-6-complete` tag as the chart
    version pin
  - cross-link known-issue #11

- **NumaAffinity flip**: when sched-plugins v0.32.x lands AND
  `operators/scheduler-plugin/internal/plugins/numa/plugin.go`
  body is upgraded:
  1. Flip `numaAffinity.enabled` default to true in values.yaml
  2. Verify `helm template` renders NumaAffinity in filter/score
  3. Update known-issues / DESIGN.md to mark T006 resolved
