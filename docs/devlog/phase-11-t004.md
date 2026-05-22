# P11-T-004 · IMS-1 node-lifecycle-operator helm chart + cmd/main.go ctrl.Reconciler wire

- **Commit**: (this commit)
- **Date**: 2026-05-22
- **Duration**: 1-1.5d plan / ~0.6d actual

## Intent

把 IMS-1 Phase 10 P10-T-007 controller body(`internal/state/transitions.go`
+ `internal/controller/nodelifecycle_controller.go` ReconcileOnce pure-Go)
接通到 helm chart + controller-runtime manager。ADR-0017 §2 Decision D
2nd 优先级 closure(demo-backend chart pattern 1st 之后 sibling)。

## Path adaptations(plan literal vs codebase reality)

3 处:
1. **api/v1alpha1 缺 SchemeBuilder + AddToScheme**(Phase 9 P9-T-105 scaffold
   仅 GroupVersion 一行):cmd/main.go 走 controller-runtime 需要
   `v1alpha1.AddToScheme(scheme)` · 必须补 SchemeBuilder + init() Register。
   解决:`groupversion_info.go` 加 SchemeBuilder + AddToScheme + Resource
   helper + init() Register(NodeLifecycle{} + NodeLifecycleList{})。
2. **api/v1alpha1 缺 zz_generated.deepcopy.go**(controller-runtime require
   DeepCopyObject):用 controller-gen `object paths="./api/v1alpha1/..."`
   生成 134 行 deepcopy methods。
3. **config/crd/bases/ 缺 CRD YAML**(scaffold 未 run make manifests):用
   controller-gen `crd:crdVersions=v1 paths="./api/v1alpha1/..."` 生成
   `lifecycle.ocloud.edge.example.com_nodelifecycles.yaml` · cp 到 chart
   `crds/` folder(helm 自动 pre-install hook)。

## Debugging trail

3 个小决策反复:

1. **go.mod 引入 controller-runtime 时序**:首次 `go mod tidy` 删除手 add
   的 require · 因为没有 source 文件 import 它。修法:先写 cmd/main.go
   import 全 + reconciler.go import 全 · 然后 `go mod tidy` 才能根据真实
   import 加 require。最终 go.sum 加 ~50 行 indirect deps · `go.mod`
   require 块从 1 块变 2 块。
2. **reconciler.go conditionsEqual 类型 slip**:初版用 `v1alpha1Meta
   Condition` 占位 alias · build fail。修法:import `metav1
   "k8s.io/apimachinery/pkg/apis/meta/v1"` · 改 `[]metav1.Condition`
   (与 existing controller.go 一致)。
3. **go test ./... 偶发 vet.cfg 错**:Windows go test 并发跑多个包时
   build cache 临时 vet.cfg 文件被并发写 · 误报 "cannot find the path"。
   修法:`-vet=off` 跳过 vet · 真测试 PASS。

## Key decisions

- **controller-runtime over client-go raw**(为 IMS-1 与 demo-backend 走
  不同路径 rationale):IMS-1 是真 operator(CRD owner + watch + status
  update + leader-election + metrics + healthz)· controller-runtime
  封装的所有 6 项(manager + Reconciler + builder + healthz + metrics +
  signal handler)都用得上 · demo-backend 仅需 Lease loop 单点 · client-go
  够;两 module 因 *功能集合不同* 选 *不同框架* · 不是不一致 · 是合适。
- **`ctrl.Reconciler` 包装 vs 把状态机 inline 到 Reconcile**:选包装。
  `internal/controller/reconciler.go` 仅 fetch + observe + 调
  ReconcileOnce + Status.Update + Event emit · 不重复状态机逻辑。
  state machine 在 nodelifecycle_controller.go 保留 pure-Go +
  unit-testable · 这是 P10-T-007 substrate 设计目的。
- **Watches**:`For(&v1alpha1.NodeLifecycle{}).Owns(&corev1.Node{})` ·
  Owns 让 controller-runtime 自动 reconcile NodeLifecycle when linked
  Node status 变化 · 不需手工 source.Kind for Node + 自建 EventHandler。
- **LeaderElectionID**:`node-lifecycle-operator.lifecycle.ocloud.edge.
  example.com`(per inference-operator 命名 convention)
- **chart RBAC**:ClusterRole(NodeLifecycle 全 verbs + Node read/patch
  + Events create/patch)+ namespaced Role for Lease(leader-election
  necessary)· 与 controller-runtime built-in lease lock convention align
- **chart `crds/` folder**(helm 自动 pre-install hook · pre-upgrade
  仍不动 CRD by helm default · 与 inference-operator chart pattern 一致)

## Verification

P3 三项验证维度 全过:

- **存在性**:
  - `ls deploy/helm-charts/node-lifecycle-operator/{Chart,values}.yaml` ✓
  - `ls deploy/helm-charts/node-lifecycle-operator/templates/{_helpers.tpl,deployment,service,serviceaccount,rbac,servicemonitor}.yaml` ✓
  - `ls deploy/helm-charts/node-lifecycle-operator/crds/lifecycle.ocloud.edge.example.com_nodelifecycles.yaml` ✓
  - `ls operators/node-lifecycle-operator/{Dockerfile,cmd/main.go,internal/controller/reconciler.go,api/v1alpha1/zz_generated.deepcopy.go}` ✓
- **完整性**(plan §3 P11-T-004 acceptance 8 items):
  - `helm lint --strict deploy/helm-charts/node-lifecycle-operator/` → `1 chart(s) linted, 0 chart(s) failed`(only `[INFO] Chart.yaml: icon is recommended`)
  - `helm template node-lifecycle-operator deploy/helm-charts/node-lifecycle-operator/` → 7 kind render(ClusterRole + ClusterRoleBinding + Deployment + Role + RoleBinding + Service + ServiceAccount)+ ServiceMonitor opt-in
  - `helm template` ClusterRole verbs 含 `nodelifecycles get/list/watch/create/update/patch/delete` + `nodes get/list/watch/patch` + `events create/patch` ✓
  - `helm template` Lease Role verbs 含 `coordination.k8s.io/leases get/list/watch/create/update/patch/delete` + `configmaps get/list/watch` ✓
  - `cd operators/node-lifecycle-operator && go build ./...` → exit 0 · 无 unresolved · cmd/main.go + internal/controller/reconciler.go + api/v1alpha1/groupversion_info.go SchemeBuilder 全 compile
  - `go test -vet=off ./...` → ok 3 packages(api/v1alpha1 3 round-trip tests pass + internal/controller 8 ReconcileOnce tests pass + internal/state 6 NextState tests pass)
  - cmd/main.go: controller-runtime manager 启 + NodeLifecycleReconciler register + healthz/readyz + leader-elect via --leader-elect flag(chart default true)
  - ADR-0003 §IMS-1 status flip:added P11-T-004 chart packaging 状态段
  - DESIGN.md §3 生命周期 段 + §7 参考 cross-ref T004 commit + chart path
- **正确性**(诚实 kind smoke fallback):
  - kind binary missing in dev env · kind smoke fall back to helm lint + helm template syntactic verify
  - 真 kind smoke `helm install` Pod Ready 由 T107 phase11/ folder 覆盖

## Carry-forward

- **P11-T-005 IMS-2 software-mgmt-operator chart** + **P11-T-006 IMS-3 bare-metal-provisioning-operator chart**:同 pattern reuse:
  - api/v1alpha1 加 SchemeBuilder + AddToScheme + init() Register
  - controller-gen object 生 zz_generated.deepcopy.go
  - controller-gen crd 生 CRD YAML → chart crds/ folder
  - 新增 internal/controller/reconciler.go ctrl.Reconciler 包装(为 IMS-2 加 SoftwareBundleReconciler · 为 IMS-3 加 BareMetalNodeReconciler · 各 ~80 行)
  - cmd/main.go controller-runtime manager wire(IMS-2 + IMS-3 同 IMS-1 pattern · 各 ~100 行)
  - go.mod controller-runtime + client-go + go mod tidy
  - Dockerfile multi-stage(同 IMS-1 pattern · 各 ~30 行)
  - chart skeleton 同 IMS-1(Chart.yaml + values + 5 templates + .helmignore + crds/)
- **P11-T-107 kind smoke E2E** assertion(b) IMS chart install:本 chart `helm install` Pod Ready + CRD installed + Reconcile loop 触发
- **P11-T-201 真 multi-cluster / multi-site demo**:本 chart 是 multi-cluster 部署 substrate · Karmada propagation(per ADR-0018 §2 Decision B)走 PropagationPolicy 把 NodeLifecycle CRD propagate to member cluster

## §0a.10 / §0a.11 compliance

- 续 T003 autonomous mode · main agent serial · 不 batch
- strict verify = go build + go test + helm lint + helm template 各项 stdout 实证
- 不 push remote · 累积到 phase-11-complete tag
- 不停于 T004 边界 · 继续 T005
