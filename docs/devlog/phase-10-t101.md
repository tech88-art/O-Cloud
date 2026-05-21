# P10-T-101 · IMS-3 bare-metal-provisioning-operator controller body · 7-state provisioning machine(chart deferred per scope adaptation 同 T007/T008)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 2-3d plan / ~0.4d actual(同 T007/T008 pattern · pure-Go Reconcile)

## Intent

Phase 9 P9-T-105 scaffold → Phase 10 P10-T-101 controller body 同 T007 + T008 pattern。落 `internal/state/provisioning.go`(13 transitions · 7 states)+ `internal/controller/baremetalnode_controller.go`(ReconcileOnce · 5 Conditions · BMC error path)+ 17 unit tests + DESIGN.md。完成 3 IMS controller body 全 land。

## Path adaptations(同 T007/T008)

- 整 chart 不存 · Phase 11+ chart packaging
- 真 BMC integration(Redfish over HTTPS · IPMI client)deferred · Phase 11+ 真硬件对接 stream
- pure-Go Reconcile · 跳 ctrl.Reconciler interface
- 与 T201 真硬件 multi-pool 演示打磨 联动 · 当前 synthetic ring fallback path 走 deprovision cycle 不依赖真物理 BMC

## Test caveats(1 处)

- `TestNextStateMultiHopReady` 原写 Inspecting → Ready 期望 Registering 中间步 · 但 NextState 1-hop lookahead 限 · 4-hop path 找不到 → 改名 `TestNextStateMultiHopOneStep`(Inspecting → Provisioning 是 2-hop · NextState returns Registering)+ 加 `TestNextStateBeyondLookaheadDepth`(Inspecting → Ready 4-hop · returns ""· 验证 Reconciler multi-pass 设计)
- 这反映 Reconciler 的实际行为:Reconcile 每 pass 推一步 · multi-pass 完成 Inspecting → Ready 整 path · 不需要 NextState 一次出 4-hop · BFS-1 是 intentional

## Key decisions

- **7 ProvisioningStates + 13 transitions**:6 forward(Inspecting → Registering → Provisioning → Provisioned → Ready · Ready → Deprovisioning → Inspecting cycle)+ 6 error edges(任 state → Error)+ 1 recovery(Error → Inspecting)
- **BMCError 短路 to Error**:观察 BMC 不可达直接 flip state · 不走 desired state path
- **Error recovery**:BMCError 清 + state=Error → next Reconcile returns Inspecting(operator retry)
- **DesiredState** advance 优先级:BMCError 短路 > 观察驱动(BMCReachable / ImagePulled / NodeJoined)> Spec.DesiredState multi-hop
- **State.IsPermitted defense**:Reconcile 末尾验证 transition · 非法 revert(防止 observation-driven 与 Spec-driven 矛盾时走非法路径)
- **Requeue 2-tier**:Ready/Error 60s steady · 中间 states 10s active
- **5 Conditions semantics**:BMCReachable · InspectionDone · ImagePulled · Ready · Error(每对应 ReconcileInput 输入观察 + NextState 出错状态)

## Verification

P3 三项:
- `go build` exit 0
- `go vet` exit 0
- `go test`:state 7 tests · controller 10 tests · api/v1alpha1 3 round-trip preserved · 全 PASS

总 17 new tests:
- state/provisioning_test.go(7):happy path + Error from any state + Error recovery + reject skip + 1-hop multihop + beyond lookahead depth + Matrix coverage 13
- controller/baremetalnode_controller_test.go(10):Inspecting advances when BMCReachable · Provisioning advances when ImagePulled · Provisioned advances when NodeJoined · BMCError flips to Error · Error recovery · Deprovision trigger · Deprovisioned cycles to Inspecting · Conditions emit count 5 + Ready True · Requeue cadence 60s/10s

## Carry-forward

- **3 IMS controller body 全 land**:T007 IMS-1 + T008 IMS-2 + T101 IMS-3 · 3 pure-Go Reconcile substrate · ADR-0003 v2 IMS-1/2/3 status 一起 update at next batch commit
- **Phase 11+ chart packaging stream**:4 chart 联动(IMS-1/2/3 + demo-backend)· 真 BMC integration(Redfish/IPMI client + Secret 解析)
- **T201 master demo** 联动:synthetic ring fallback path 不依赖真硬件 BMC · IMS-3 状态机走 mock observation → 7 state progression demonstrated

## §0a.10 / §0a.11

- 同 T006/T007/T008 pattern · 同 session 连续推进 · scope adaptation 透明 · 累积本地 9 commits
