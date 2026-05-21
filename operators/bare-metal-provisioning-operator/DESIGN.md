# bare-metal-provisioning-operator · DESIGN.md(Phase 10 P10-T-101 controller body)

> Per CLAUDE.md §14.2. ADR cross-ref: [ADR-0003 v2 §IMS-3](../../docs/adr/0003-ims-services-phasing.md).

## §1. 架构概览

`bare-metal-provisioning-operator` 实现 O-Cloud IMS-3(Bare Metal Provisioning)能力 — BMC inspection + OS image provisioning + K8s join lifecycle · 适配 Metal3 / cluster-api BareMetalHost pattern。Phase 10 真硬件对接同期(P10-T-201 master demo 联动)。

## §2. 接口契约

- **CRD GVK**:`provisioning.ocloud.edge.example.com/v1alpha1/BareMetalNode`(Cluster-scoped · `bmnode` short name)
- **Spec**:`BMC{Address, CredentialsRef{Name, Namespace}}` + optional `Image{URL, Checksum}` + `DesiredState`(default Inspecting)
- **Status**:`ProvisioningState`(7-state enum)+ `MACAddress` + `HardwareInfo{CPU, Memory, NPU}` + `Conditions[]`
- **7 ProvisioningStates** + 13 transitions:
  - Inspecting → Registering → Provisioning → Provisioned → Ready
  - Ready → Deprovisioning → Inspecting(cycle)
  - 任意 state → Error(BMC unreachable / failure)
  - Error → Inspecting(operator retry)

## §3. 生命周期

- **Reconcile 触发**:BareMetalNode 变化 / BMC poll(controller-runtime periodic)/ 周期 requeue(Ready/Error 60s · 中间 10s)
- **Phase 11+**:cmd/main.go controller-runtime manager + chart packaging + Redfish/IPMI client integration

## §4. 错误处理

- BMC unreachable → BMCError set in ReconcileInput → 立 flip to Error state · ConditionError emit
- Error recovery → admin 清 BMCError(BMC 重新可达)→ next Reconcile returns Inspecting
- 非法 transition → state.IsPermitted false → revert NextState to current(per Reconcile defense)

## §5. 扩展点

- **Phase 10**:state machine + Reconcile pure-Go + 14 unit tests
- **Phase 11+ chart packaging**:cmd/main.go + chart + Dockerfile + envtest + 真 BMC integration(Redfish over HTTPS · IPMI fallback · Secret 解析 username/password)
- **Phase 11+ cross-controller awareness**:Ready 时 trigger NodeLifecycle 进 Provisioning state · RebootRequired 时调 NodeLifecycle state(import internal/state.IsPermitted 验证转换)
- **真硬件对接 Phase 10 T201**:T201 master demo script 模拟 BMC fixture · synthetic ring fallback path 走 deprovision cycle 不依赖真物理 BMC

## §6. 集成示例

```go
import "github.com/tech88-art/O-Cloud/operators/bare-metal-provisioning-operator/internal/controller"

// In ctrl.Reconciler:
in := controller.ReconcileInput{
    Current: bmnodeObj,
    BMCReachable: bmcClient.Ping() == nil,
    InspectionDone: bmnodeObj.Status.MACAddress != "",
    ImagePulled: imageWorker.IsDone(),
    NodeJoined: nodeReady(bmnodeObj.Status.MACAddress),
    BMCError: lastBMCErr.Error(),
    Now: time.Now(),
}
out := controller.ReconcileOnce(in)
bmnodeObj.Status.ProvisioningState = out.NextState
bmnodeObj.Status.Conditions = out.Conditions
// k8s client Update + return ctrl.Result{RequeueAfter: out.RequeueAfter}, nil
```

## §7. 参考

- ADR-0003 v2 §IMS-3 bare-metal-provisioning-operator
- arch §5.11 bare-metal-provisioning-operator
- Metal3 / cluster-api BareMetalHost(reference inspiration)
- Redfish + IPMI specs(Phase 11+ integration target)
- CLAUDE.md §14.2
- `docs/devlog/phase-10-t101.md`
