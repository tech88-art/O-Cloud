# software-mgmt-operator · DESIGN.md(Phase 10 P10-T-008 controller body)

> Per CLAUDE.md §14.2. ADR cross-ref: [ADR-0003 v2 §IMS-2](../../docs/adr/0003-ims-services-phasing.md).

## §1. 架构概览

`software-mgmt-operator` 实现 O-Cloud IMS-2(Software Management)能力 — 节点级软件版本清单 + rollout 策略调度 · 适配 StarlingX sw-deployment pattern。

## §2. 接口契约

- **CRD GVK**:`software.ocloud.edge.example.com/v1alpha1/SoftwareBundle`(Cluster-scoped · `swbundle` short name)
- **Spec**:`Version` + `Patches[]` + `RolloutPolicy{Strategy, MaxUnavailable}` + optional `NodeSelector`
- **Status**:`AppliedVersion` + 3 counts(Target/Applied/Failed)+ `Conditions[]`(Progressing/Completed/Failed/Degraded)
- **3 rollout strategies**:RollingUpdate(MaxUnavailable cap)/ Parallel / Sequential

## §3. 生命周期

- **Reconcile 触发**:SoftwareBundle 变化 / Node 变化(NodeSelector match)/ 周期 requeue(steady 30s · progressing 5s)
- **Phase 11+**:cmd/main.go controller-runtime manager + chart packaging

## §4. 错误处理

- 节点 rollout fail → 进 FailedNodes set · Degraded condition emit · 不阻塞其他节点(per strategy)
- 未知 strategy → ReconcileOnce 返 error · controller-runtime requeue

## §5. 扩展点

- **Phase 10**:`ReconcileOnce` pure-Go + `rollout.NextBatch` 3 strategies + 8 controller tests + 8 rollout tests
- **Phase 11+ chart packaging**:cmd/main.go orchestration + helm chart + Dockerfile + envtest
- **Phase 11+ rollback**:SoftwareBundle.Status.AppliedVersion 是 source-of-truth · downgrade 由新 Spec.Version + RolloutPolicy 触发

## §6. 集成示例

```go
import "github.com/tech88-art/O-Cloud/operators/software-mgmt-operator/internal/controller"

in := controller.ReconcileInput{
    Current: bundleObj,
    AllTargetNodes: targetNodeNames,
    AppliedNodes: bundleObj.Status.AppliedNodes(),
    InProgressNodes: bundleObj.Status.InProgressNodes(),
    Now: time.Now(),
}
out := controller.ReconcileOnce(in)
bundleObj.Status = out.NextStatus
// dispatch out.Plan.NodesToTarget to per-node patch worker pool
// return ctrl.Result{RequeueAfter: out.RequeueAfter}, nil
```

## §7. 参考

- ADR-0003 v2 §IMS-2 software-mgmt-operator
- arch §5.10 software-mgmt-operator
- StarlingX sw-deployment 模型(reference)
- CLAUDE.md §14.2
- `docs/devlog/phase-10-t008.md`
