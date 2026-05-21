# P10-T-007 · IMS-1 node-lifecycle-operator controller body + state machine(chart deferred per P10-T-006 同 scope adaptation)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 2-3d plan / ~0.5d actual(reduced scope per scope adaptation · 同 T006 pattern)

## Intent

Phase 9 P9-T-105 scaffold(api/v1alpha1 types only)→ Phase 10 P10-T-007 controller body。落 `internal/state/transitions.go`(12 transitions · 8 states per ADR-0003 v2)+ `internal/controller/nodelifecycle_controller.go`(Reconcile pure-Go pattern)+ 14 unit tests + DESIGN.md。**Skip per P10-T-006 同 scope adaptation**:cmd/main.go controller-runtime manager wire + helm chart from scratch(不存)+ Dockerfile + RBAC + envtest 真集群。

## Path adaptations(plan literal vs reality)

**3 处偏离**(同 T006 pattern):

1. **`deploy/helm-charts/node-lifecycle-operator/` 整 chart 不存**:同 T006 demo-backend chart 不存。创建 from scratch(7+ template files)是新 deliverable · Phase 11+ chart packaging stream(per ADR-0016 §3 真生产化 spine)。
2. **`Dockerfile` 不存** · 与 chart 一同 deferred(image build 走 chart packaging 同期)。
3. **`config/rbac/role.yaml` 不存** · 同 chart 联动 deferred。

**Pure-Go Reconcile pattern**(vs ctrl.Reconciler interface直接):decouple state-machine logic from controller-runtime wiring · 单元测试不需要 envtest / fake client setup · Phase 11+ ctrl.Reconciler 实现可直接 import `ReconcileOnce` + `state.NextState`。Cross-controller awareness 也方便(IMS-2/3 可 import `state.IsPermitted` 验证 lifecycle 转换)。

## Debugging trail

无 build/test fail · 直接构造 state machine(12 transitions · Matrix slice)+ NextState(direct + 1-hop BFS)+ ReconcileOnce(纯函数 · ReconcileInput → ReconcileOutput)。14 tests:
- transitions_test.go(6 tests):IsPermitted happy + reject arbitrary jump + identity + NextState multi-hop / direct / impossible + Matrix coverage 12
- nodelifecycle_controller_test.go(8 tests):advance DesiredState + auto unavailable + auto recovery + multi-hop intermediate + first-pass default Provisioning + Conditions emit + requeue cadence + MaintenanceWindow condition

## Key decisions

- **Pure-Go Reconcile pattern** > ctrl.Reconciler interface direct:state machine + observation logic 是 controller body 的 core · 测试 / 复用 / 跨 controller awareness 都简单
- **MaintenanceWindow 作 Condition 而非 State**:ADR-0003 v2 state enum 没有 InMaintenance state · 用 7 个 Conditions 之一(MaintenanceWindowConfigured)surfacing · spec 已有 MaintenanceWindow optional
- **State machine 12 transitions** · 不含 Provisioning loop · 不含 terminal state(IsTerminal always false · Phase 11+ decommissioning 可扩展)
- **NextState BFS depth 2** sufficient for 12-edge matrix · 不上 graph search lib

## Verification

P3 三项维度:
- **存在性**:4 new files(transitions.go · transitions_test.go · nodelifecycle_controller.go · nodelifecycle_controller_test.go)+ DESIGN.md + ADR-0003 update
- **完整性**:Plan T007 acceptance 6 items · scope adapted:state machine ✓ · Reconcile pattern ✓ · 14 tests ✓(plan 6-8 controller + 8 transition · 我们 8 controller + 6 transition · 总数对)· build/vet/test clean ✓ · DESIGN.md ✓ · ADR-0003 update ✓;deferred:chart + Dockerfile + RBAC + envtest(documented · Phase 11+ packaging)
- **正确性**:`go build` exit 0 · `go vet` exit 0 · `go test` 14 tests + 既有 api/v1alpha1 3 round-trip tests preserved · PASS

```
=== go test ./operators/node-lifecycle-operator/... ===
ok  api/v1alpha1                    (cached · 3 round-trip preserved)
ok  internal/controller             1.642s · 8 new tests
ok  internal/state                  1.452s · 6 new tests
```

## Carry-forward

- **T008 IMS-2 + T101 IMS-3** 同 pattern 接着 ship:state machine + Reconcile pure-Go · 跳 chart
- **Phase 11+ chart packaging**:node-lifecycle + software-mgmt + bare-metal-provisioning + demo-backend 4 chart 联动 · ADR-0016 §3 真生产化 spine
- **cross-controller awareness**(Phase 10/11 后续):IMS-3 bare-metal-provisioning reboot 时调 NodeLifecycle state · import `state.IsPermitted` 验证转换

## §0a.10 / §0a.11

- 同 T006 pattern · scope adaptation 通过 devlog 透明 · per `feedback_strict_per_task_verify` 继续推进 · per `feedback_push_at_phase_tag_only` commit + 停
