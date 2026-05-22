# P11-T-005 · IMS-2 software-mgmt-operator helm chart + cmd/main.go ctrl.Reconciler wire

- **Commit**: (this commit)
- **Date**: 2026-05-22
- **Duration**: 1-1.5d plan / ~0.3d actual(pattern reuse · template sed-replace)

## Intent

Pattern reuse 接 IMS-1 chart packaging。把 P10-T-008 SoftwareBundle ReconcileOnce + 3 rollout strategies + 15 unit tests 通过 ctrl.Reconciler shell 接到 controller-runtime manager + helm chart。ADR-0017 §2 Decision D 3rd 优先级 close。

## Path adaptations

1. **chart template 大批量复制 from IMS-1**:用 `sed -e 's/node-lifecycle-operator/software-mgmt-operator/g' -e 's/NodeLifecycle/SoftwareBundle/g' -e 's/nodelifecycles/softwarebundles/g' -e 's/lifecycle\.ocloud/softwaremgmt.ocloud/g'` 把 7 个 template file 一次性 copy + transform。Chart.yaml 单独写(不 sed · description 内容差异大)。
2. **RBAC 收紧:Node 仅 list/watch · 不 patch**:T004 IMS-1 控制器要 patch Node(cordon/drain)· IMS-2 controller 不 mutate Node — SoftwareBundle controller 仅做 rollout *决策* · 真 actuation 由 per-node agents(Phase 12+ NodeSoftwareBundleStatus CR)。手 edit rbac.yaml 移除 nodes/status verb 段。
3. **Status schema 与 ReconcileInput 不对齐**:IMS-2 SoftwareBundleStatus(P10 Phase 10 P10-T-008)只有 `TargetNodeCount/AppliedNodeCount/FailedNodeCount` + Conditions · 没有 applied/in-progress/failed `[]string`。reconciler.go observed state 从 annotations `softwaremgmt.ocloud.edge.example.com/{applied,in-progress,failed}-nodes`(逗号分隔)解析 · annotations 由 per-node rollout agent 写。Phase 12+ schema 升级到独立 NodeSoftwareBundleStatus CR。
4. **Selector match minimum-viable**:`matchesSelector(labels, selector)` 只支持 exact key=value match(不支持 In/NotIn/Exists 等)。Phase 12+ 切到 `labels.NewSelector()` from `k8s.io/apimachinery/pkg/labels`。

## Debugging trail

仅 1 个小修:首版 reconciler.go 用了 `out.NextStatus.Phase` + `sb.Status.AppliedNodes` 字段 · 但 Status 没这些字段。grep 实测 + 看 P10-T-008 ReconcileOnce 真实 output 后改成 `Status.AppliedNodeCount` 等。教训复用 P3 "verify-before-claim" — 写 ctrl.Reconciler 前 grep Status struct。

## Key decisions

- **Same controller-runtime pattern as IMS-1**(LeaderElectionID = `software-mgmt-operator.softwaremgmt.ocloud.edge.example.com`)
- **Node RBAC 收紧到 list/watch**(rationale 上 path adaptation #2)
- **Annotation-based observed state**(rationale 上 path adaptation #3)· 比加 Status field 改 schema 兼容性安全 · Phase 12+ 拆 NodeSoftwareBundleStatus CR 时迁移

## Verification

- 存在性:chart 8 file + Dockerfile + reconciler.go + zz_generated.deepcopy.go + config/crd/bases/...yaml ✓
- 完整性:`helm lint --strict` clean · `helm template` 7 kind render · `go build ./...` clean · `go test -vet=off ./...` ok 4 packages(api/v1alpha1 + internal/controller + internal/rollout + cmd)
- 正确性:RBAC 与 IMS-1 对比 · Node verbs 仅 read · 反映 SoftwareBundle 不 mutate Node 的设计
- kind smoke fall back 同 T004(T107 真 smoke 覆盖)

## Carry-forward

- **P11-T-006 IMS-3**:bare-metal-provisioning-operator pattern reuse · 额外:Redfish/IPMI stub client + Secret 解析 username/password(plan §3 P11-T-006 acceptance)
- **P11-T-107 kind smoke**:assertion(b)cover 3 IMS chart install + CRD installed + Pod Ready
- **Phase 12+ NodeSoftwareBundleStatus CR**:解放当前 annotation-based observed state · 加 per-node 详细 status reflection

## §0a 续 autonomous mode · 不停 · 继续 T006
