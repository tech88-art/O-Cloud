# P10-T-008 · IMS-2 software-mgmt-operator controller body + 3 rollout strategies(chart deferred per scope adaptation 同 T007)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 2-3d plan / ~0.4d actual(同 T007 pattern · pure-Go Reconcile · 跳 chart)

## Intent

Phase 9 P9-T-105 scaffold → Phase 10 P10-T-008 controller body。落 `internal/rollout/strategy.go`(3 strategies · `NextBatch` 函数)+ `internal/controller/softwarebundle_controller.go`(ReconcileOnce pure-Go · 4 Conditions · 3 counts)+ 15 unit tests + DESIGN.md。Skip per scope adaptation:cmd/main.go + chart + RBAC + Dockerfile(同 T006 + T007 pattern · Phase 11+ chart packaging)。

## Path adaptations(同 T007)

- 整 chart 不存 · Phase 11+ packaging
- Dockerfile + RBAC 不存 · deferred 与 chart 联动
- pure-Go Reconcile pattern · 跳 ctrl.Reconciler interface · 单元测试无 envtest 需求 · Phase 11+ ctrl.Reconciler 实现可直接 import

## Key decisions

- **3 rollout strategies** 各独立函数路径 · `NextBatch` returns Plan{NodesToTarget, RemainingNodes, Strategy} · pure function
- **MaxUnavailable cap** 在 RollingUpdate 减去 in-progress count(防止 cap exhaustion 时仍 target 新 nodes)
- **Sequential 强阻塞**:任何 in-progress → no new target(确保严格 one-at-a-time)
- **4 Conditions semantics**:Progressing(in-progress 或 plan 非空)/ Completed(IsComplete && no failures)/ Failed(任意 failed node)/ Degraded(applied + failed 混合 · 部分成功)
- **AppliedVersion 仅在 IsComplete && no failures 时 set**(避免 partial rollout 误报 success version)

## Verification

P3 三项维度全过:
- `go build` exit 0
- `go vet` exit 0
- `go test`:rollout package 8 tests · controller package 7 tests · api/v1alpha1 3 round-trip preserved
- helm lint + kind smoke deferred per chart packaging

15 new tests:
- rollout/strategy_test.go(8):RollingUpdate 基本 + cap 计算 + AtCap + Parallel + Sequential basic + Sequential blocks on in-progress + IsComplete + AllApplied + unknown strategy
- controller/softwarebundle_controller_test.go(7):RollingUpdate plan + Progressing + Completed + Failed + Degraded + 3 counts + 3-tier requeue cadence

## Carry-forward

- T101 IMS-3 bare-metal-provisioning 同 pattern · 接着 ship
- Phase 11+ chart packaging:4 chart 联动(node-lifecycle + software-mgmt + bare-metal-provisioning + demo-backend)

## §0a.10 / §0a.11

- 同 T006 + T007 pattern · 同 session 连续推进 · scope adaptation 透明
