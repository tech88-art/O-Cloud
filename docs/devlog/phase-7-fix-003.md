# P7-fix-003 · post-tag CI fix #3: npu-dra-driver chart RBAC for NPUSliceTemplate (controller cache sync unblock)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: ~20 min (CI log re-triage with auth · root cause identification · 1-file RBAC fix · helm lint verify)

## Intent

P7-fix-002(494307e)修了 chart-bundled CRD sync · ModelService apply 成功 · 但 e2e-kind 仍 fail at Phase 5 step 18:

```
::error::No NPUSliceAllocation reached Allocated for ocloud-system/smoke-ms within 120s
```

Drill into npu-dra-driver logs:**T007 NPUSliceTemplateReconciler 加 watch on NPUSliceTemplate CRD · 但 chart RBAC 没加该 CRD 的 get/list/watch verbs**:

```
ERROR controller-runtime.cache.UnhandledError Failed to watch
type: *v1alpha1.NPUSliceTemplate
error: failed to list *v1alpha1.NPUSliceTemplate:
npuslicetemplates.npu.ocloud.edge.example.com is forbidden:
User "system:serviceaccount:ocloud-system:npu-dra-driver"
cannot list resource "npuslicetemplates" in API group ...
```

informer reflector retries forever(observed: 04:56:54 · 04:56:59 · 04:57:10 · 04:57:34 · 04:58:14 · 04:58:47 — 6 retries in 2 min)· cache sync 永远不完成 → controller manager 的 WaitForCacheSync 卡住 → ClaimReconciler + AllocationReconciler 都不能 reconcile → NPUSliceAllocation 永远 0 count → assert.sh 120s timeout fail。

## Path adaptations + fix

Single-file change: `deploy/helm-charts/npu-dra-driver/templates/rbac.yaml`
- 加 `npuslicetemplates` resource verbs:get / list / watch(满足 reconciler informer cache + Get 调用)
- 加 `npuslicetemplates/status` resource verbs:get / update / patch(满足 reconciler status stamping)
- 加 inline 注释解释 root cause("Without these verbs the shared informer cache fails to list/watch the CRD → cache sync blocks → other reconcilers never start → NPUSliceAllocation never reaches Allocated")

Same chart-as-source-of-truth class as P7-fix-002 (CRD sync) — both are gaps between code changes(T007 controller registration · T003 + T006 type generation)+ chart manifest updates。

## Debugging trail

- P7-fix-002 push 后 wait 12+ min · Monitor authenticated polling 出 terminal `CI:success | e2e-kind:failure | helm-lint:success`
- 拉 e2e-kind run jobs(`/runs/{id}/jobs`)· 仍是 step 18 fail
- Fetch logs(3054 行)· grep "ModelService|::error" 定位:
  - line 1358 `== apply ModelService fixture ==` SUCCEED(P7-fix-002 工作 · CRD 接受 schedulerOverride)
  - line 1380 `::error::No NPUSliceAllocation reached Allocated within 120s` NEW failure
- grep "npu-dra-driver|claim|allocator|template_controller" 找到 6 处 NPUSliceTemplate RBAC 拒绝 stack trace · 间隔约 30s · 标志 reflector retry loop
- grep `ClaimReconciler registered` + `AllocationReconciler registered` + `NPUSliceTemplateReconciler registered` 全部 logged at 04:56:36 startup time — controllers 注册成功 · 但 cache sync 卡死
- 1816 行 NPUPool.status `16/16 NPUs healthy, 1 allocated` — 误导 · 实际是 set-b-multi-ring 16-NPU data 已经被 reseed(可能 Phase 4 install 之后的某个步骤 swap 进来了 · 但不影响 root cause 分析 · 1 allocated 反映只 1 device 通过 DRA-level allocate · NPUSliceAllocation 仍 0)
- Find npu-dra-driver chart `templates/rbac.yaml` line 34-39 既有 npusliceallocations grants · 加 npuslicetemplates grants 同 pattern
- helm lint --strict pass · helm template grep verify new rules render

## Key decisions

- **Only get/list/watch + status verbs**(不加 create/update/patch/delete on root resource):NPUSliceTemplate 是 user-created object(operators or e2e fixtures 创建)· reconciler 只读 spec + 写 status · 没必要给 create/delete · 最小权限
- **Same chart-RBAC-as-source-of-truth pattern**:Phase 5 P5-T-118 也类似 — chart RBAC 必须同步 code controller 注册。Phase 8 baseline-bump candidate "chart audit CI" 应 add `diff chart/templates/rbac.yaml vs cmd/main.go controller list`
- **不 ship 单独 verify CI rule(本 commit)**:CI rule add 是 follow-up · 本 fix 单一目标解 e2e-kind · Phase 8 polish 时一并补 CI check

## Verification

- 存在性:
  - `deploy/helm-charts/npu-dra-driver/templates/rbac.yaml` 加 NPUSliceTemplate 7 行(2 resource blocks + 注释 5 行 + 空行 1)✅
- 完整性(verified by execution):
  - `helm lint --strict deploy/helm-charts/npu-dra-driver/` 1 chart linted · 0 failed ✅
  - `helm template` rendered ClusterRole 含 `resources: ["npuslicetemplates"]` + `verbs: ["get", "list", "watch"]` + status verbs `["get", "update", "patch"]` ✅
- 正确性:
  - 与 Phase 5 npusliceallocations RBAC pattern 一致(同 apiGroup · 同 status subresource · 同最小权限分布)
  - reconciler 在 cmd/main.go 注册时 typed object 是 `&v1alpha1.NPUSliceTemplate{}` + 操作是 `For(&NPUSliceTemplate{})` + `Get(ctx, req, &NPUSliceTemplate{})` + `Status().Update(ctx, &NPUSliceTemplate{})` · 全部 require get/list/watch + status update verbs · 本 RBAC 覆盖

## Carry-forward

- **CI 验证**:next workflow run on this commit · npu-dra-driver SA 应可 list/watch NPUSliceTemplate · cache sync 通 · ClaimReconciler 启动 · NPUSliceAllocation 应在 120s 内 reach Allocated · Phase 5 step 18 应 pass
- **Phase 8 baseline-bump task 加 chart-RBAC-sync CI check**(low-priority polish · follow phase-7-fix-002 同 candidate):每个 controller 在 cmd/main.go 注册 → chart RBAC 必须覆盖其 Reconcile 路径 require 的 verbs。可写 unit test 或 CI shell script 比较两者
- **Phase 7 T007 + T006 commits 反思**:T006 (07bda4b) 加 CRD types + chart CRD bundle(crds/npuslicetemplates.yaml)· T007 (b260884) 加 reconciler · 但 RBAC update 漏 · 三者应作为单一 unit ship。Phase 8 ship 新 CRD + 新 reconciler 模板:CRD types + CRD chart bundle + chart RBAC + reconciler 同 commit · 用 checklist enforce
