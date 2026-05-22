# node-lifecycle-operator · DESIGN.md(Phase 10 P10-T-007 controller body)

> Per CLAUDE.md §14.2. ADR cross-ref: [ADR-0003 v2 §IMS-1](../../docs/adr/0003-ims-services-phasing.md)。

## §1. 架构概览

`node-lifecycle-operator` 实现 O-Cloud IMS-1(Node Lifecycle)能力 — 跟踪集群 K8s Node 的运维生命周期(provision / bootstrap / lock / unlock / maintenance)· 与底层 `core/v1.Node` 状态联动。

```
            +-------------------------------------+
 admin -->  | NodeLifecycle CR (Cluster-scoped)   |  <-- ctrl-runtime Reconcile loop
            |  Spec.DesiredState (Unlocked etc.)  |
            |  Status.State                       |
            +------------------+------------------+
                               |
                               | watch core/v1.Node
                               v
            +-------------------------------------+
            |  observed Node.Status.Conditions    |
            |  Ready / DiskPressure / Network*    |
            +-------------------------------------+
```

数据流:
- **Read**:`api/v1alpha1.NodeLifecycle` Spec + Status + 链 `core/v1.Node.Status.Conditions`
- **Write**:`NodeLifecycle.Status.State` + 5-7 `Status.Conditions`
- **触发**:NodeLifecycle Spec 变化 + Node Status 变化 + 周期 requeue(steady 30s · transition 5s)

## §2. 接口契约

- **CRD GVK**:`lifecycle.ocloud.edge.example.com/v1alpha1/NodeLifecycle`(Cluster-scoped · `nodelc` short name)
- **Spec**(`api/v1alpha1/nodelifecycle_types.go`):
  - `NodeName string`(required · 链 K8s Node)
  - `DesiredState NodeLifecycleState`(enum 8 states · default Unlocked)
  - `MaintenanceWindow *MaintenanceWindow`(optional)
- **Status**:
  - `State NodeLifecycleState`(observed)
  - `LastTransitionTime *metav1.Time`
  - `Conditions []metav1.Condition`(per `internal/controller` ConditionXxx 7 types)
- **State enum**(8 states · `internal/state.Matrix` 12 transitions):
  - `Provisioning` → `Bootstrap` → `Available`
  - `Available` ↔ {`DegradedAvailable` / `Unavailable` / `Locked` / `RebootRequired`}
  - `Locked` ↔ `Unlocked` → `Available`
- **Unchanging invariants**:NodeLifecycle 1:1 与 Node;Cluster-scoped(NPUSlicePool 是 namespaced 但 NodeLifecycle 跨 namespace 是 Node 级别);Status.State 由 controller 写不由 admin 写。

## §3. 生命周期(P11-T-004 chart wiring LANDED)

- **启动**:cmd/main.go 走 controller-runtime manager + Reconciler 注册 + watch NodeLifecycle (primary) + Node (Owns)。Leader-election via `--leader-elect` flag(chart values.leaderElection.enabled default true)走 controller-runtime built-in Lease lock(coordination.k8s.io/v1 Lease in release namespace · LeaseID `node-lifecycle-operator.lifecycle.ocloud.edge.example.com`)。
- **Reconcile 触发**:NodeLifecycle 变化 / Node status 变化 / 周期 requeue(30s steady · 5s during transition)。`internal/controller.NodeLifecycleReconciler.Reconcile` fetch NodeLifecycle + linked Node · 调 pure-Go `ReconcileOnce` 计算 NextState + Conditions · 通过 Status().Update 写回 apiserver · 不同的 RequeueAfter 返回。
- **退出**:graceful shutdown 走 controller-runtime SIGTERM handler(`ctrl.SetupSignalHandler()`)· leader 自动 release Lease(controller-runtime 默认 ReleaseOnCancel) · follower 立即接手。
- **Health probes**:`/healthz` + `/readyz` 走 controller-runtime healthz package · chart deployment readinessProbe/livenessProbe consume。
- **Metrics**:controller-runtime metricsserver 默认 :8080 暴露 `controller_runtime_reconcile_total` 等 · chart ServiceMonitor opt-in scrape。

## §4. 错误处理

- **可恢复**:Reconcile error → ctrl.Result.Requeue=true + RequeueAfter 5s
- **不可恢复**:CRD schema 不匹配 → admission webhook reject(Phase 10 后 ADR-0014 chain 后续)
- **状态机非法转换**:`state.NextState` returns "" → caller 不更新 Status · log warning · requeue 5s(等 admin 改 Spec)

## §5. 扩展点

- **Phase 10 minimum viable**(本 P10-T-007 ship):state machine + Reconcile pure-Go function + 8 unit tests
- **Phase 11+ chart packaging**:cmd/main.go controller-runtime manager + helm chart + Dockerfile + RBAC + envtest 真集群 verify(同 T006 demo-backend chart 联动 deferred · ADR-0016 §3 真生产化 spine)
- **Cross-controller awareness**:其他 IMS-2/3 controller 可 import `internal/state.IsPermitted` 验证 lifecycle 转换;bare-metal-provisioning-operator(IMS-3)在 hardware reboot 时调 NodeLifecycle 进入 RebootRequired

## §6. 集成示例

```go
import "github.com/tech88-art/O-Cloud/operators/node-lifecycle-operator/internal/controller"

// In ctrl.Reconciler.Reconcile:
in := controller.ReconcileInput{
    Current: nodeLifecycleObj,
    NodeReady: nodeObj.Status.Conditions[ready].Status == "True",
    NodeDiskPressure: nodeObj.Status.Conditions[disk].Status == "True",
    Now: time.Now(),
}
out := controller.ReconcileOnce(in)
nodeLifecycleObj.Status.State = out.NextState
nodeLifecycleObj.Status.Conditions = out.Conditions
// k8s client Update + return ctrl.Result{RequeueAfter: out.RequeueAfter}, nil
```

## §7. 参考

- ADR-0003 v2 §IMS-1 node-lifecycle-operator
- ADR-0017 §2 Decision D 2nd chart packaging 优先级(P11-T-004)
- arch §5.9 node-lifecycle-operator
- StarlingX node lifecycle 模型(reference inspiration · adapted for K8s-native context)
- CLAUDE.md §14.2 module DESIGN.md convention
- `docs/devlog/phase-10-t007.md` controller body 实施 trail
- `docs/devlog/phase-11-t004.md` chart packaging + cmd wire 实施 trail
- `deploy/helm-charts/node-lifecycle-operator/` 8 file(Chart.yaml + values.yaml + 5 templates + .helmignore + crds/)
