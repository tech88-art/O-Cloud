# P8-T-007 · NPUVerticalScaler controller (重启切片 reconcile body)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: plan 1.5d / actual ~1d

## Intent

按 ADR-0012 §1 8-step reconcile loop + §5 mutation model 落地 NPUVerticalScaler 控制器:
- 控制器 struct `NPUVerticalScalerReconciler{Client, Scheme, Recorder, Ingestor, Now}`
- 8-step Reconcile(Get scaler → Get target → Query Ingestor → decide → cooldown → patch annotation → record ScaleEvent → writeStatus)
- decideTemplate helper(busy/idle/hysteresis 三态)
- patchModelServiceAnnotation helper(MergePatch 原子 stamp annotation + AnnotationManagedBy hint)
- appendScaleEvent FIFO eviction(MaxScaleHistoryEntries=10)
- writeStatus + setCondition + 5 condition reasons(ScalerReady / TargetNotFound / ScalingTransitioning / CooldownActive / NoDataThisTick)
- 6 controller tests covering 6 reconcile branches
- cmd/main.go 注册 reconciler · helm chart RBAC 加 npuverticalscalers verbs · DESIGN.md §5.4(6 subsections)

## Path adaptations

**关键 adaptation:plan §3-T007 假设 `ModelService.spec.template.sliceTemplate` 字段存在**(实际不存在)· ModelService schema(P4-T-103 + Phase 5/6/7)无 template 字段。Phase 7 NPUSliceTemplate 是 Pod label 路径(ADR-0011 §4 `npu.huawei.com/slice-template=<name>`)。

务实方案:T007 patch **ModelService.metadata.annotations**(不 patch spec)· annotation key 同 Pod label key(`npu.huawei.com/slice-template`)· 加 `ocloud.edge.example.com/vertical-scaler-managed=<scaler-name>` hint per ADR-0012 §5。

**Carry-forward**(超出 T007 Allowed Paths · 不在本 commit):deployment_builder.go 需要 READ annotation + STAMP Pod label 当构建 PD-pair Pods。Phase 8 内独立 polish task · 或 T103 fixture 直接 stamp Pod label 绕开 propagation gap demo end-to-end。

其他 Plan §3-T007 Allowed Paths 全部 covered:
- `internal/controller/npuverticalscaler_controller.go` (new · 366 行)✅
- `internal/controller/npuverticalscaler_controller_test.go` (new · 6 cases)✅
- `internal/controller/suite_test.go` (small edit · WithStatusSubresource 加 NPUVerticalScaler)✅
- `cmd/main.go` (small edit · Register NPUVerticalScalerReconciler with ingestor dep)✅
- `deploy/helm-charts/inference-operator/templates/rbac.yaml` (small edit · npuverticalscalers verbs + status + finalizers)✅
- `operators/inference-operator/DESIGN.md` (new §5.4 with 6 subsections)✅
- `docs/devlog/phase-8-t007.md` (this file)✅
- `internal/controller/utils.go` 没改(plan 说"small edit if needed" · 本 task 不需要 utility 复用 · 所有 helper 都 inline in controller file)

Forbidden Paths 兑现:
- api/v1alpha1/ 0 改动(types frozen by T005)
- internal/metrics/ 0 改动(ingestor frozen by T006)
- npu-dra-driver/ 0 改动(cross-module)

## Debugging trail

- 第一次 build 报 `not enough return values` at step 3 Err branch — writeStatus 返回 error 单值 · 我 `return r.writeStatus(...)` 写成 1 个返回。修:wrap as `if werr := ... ; werr != nil { return ctrl.Result{}, werr } return ctrl.Result{RequeueAfter: requeueOnNoData}, nil`
- 第一次 test 报 `cannot use corev1LocalRef(...)` — 我自创 helper 想避 corev1 import,实际 corev1 已经 imported in suite_test.go(via fake client + scheme)· 直接 `corev1.LocalObjectReference{Name: "..."}` · 删 helper
- 第三个细节:fake client 默认不持久 status 字段 · 需要 WithStatusSubresource 加 NPUVerticalScaler 才能让 `Client.Status().Update` 生效 · 否则 test 验证 status 全失败(empty)· 修 suite_test.go newFakeClient
- 6 个 tests 一次过 · 各覆盖 reconcile loop 独立 axis(NoData stay / Busy cross / Idle cross / Cooldown blocks / FIFO eviction / TargetNotFound)
- `helm lint --strict deploy/helm-charts/inference-operator/` 加 RBAC 后通过 · `helm template` 正确渲染 ClusterRole 含 npuverticalscalers verbs

## Key decisions

- **Annotation patch instead of spec patch**:plan 设的 spec field 不存在 · annotation 是 Phase 7 既有 label key 同源 · 不引入新概念。Carry-forward 限定 deployment_builder 一个 file,scope 小
- **MergePatchType 原子 stamp annotation**(非 client.Update get-then-update):避免 ResourceVersion conflict + 不竞争 ModelService 其他 controller 修改(如 ModelServiceReconciler 同时 patch status / finalizer)
- **immediate ConditionActive=False on Err**(非 N-tick counter):Phase 8 简单实现 · Phase 9 refine if real Prometheus 5xx storms 引发 noisy ConditionActive flips。Per ADR-0012 §6 risk row "Ingestor Prometheus 不可达"
- **5 condition reasons String constants**(不直接写 literal):防 typo 漂移 · 也让 T103 kind smoke 可 grep assertion("reason=ScalingTransitioning" / "reason=CooldownActive")
- **setCondition helper inline**(不引入 apimachinery util/conditions):Phase 5 既有 ModelService 控制器 inline 同样 helper(per ADR-0010 §3 T006 pattern)· 保持 module-level dep 一致 + 不增 lookup
- **fixedNow test helper**:cooldown + scaleHistory time-based 断言必须 deterministic · 每个 test fix 时间到 2026-05-21 14:00:00 UTC · cooldown remainder 算出 180s 用区间 `[170, 190]` 容忍 reconcile 内部 now() drift
- **decideTemplate 返回 `target=""` for hysteresis**:`""` 表"stay current" sentinel · 比 nil pointer 或 boolean flag 更易读 · controller 显式 check 后 short-circuit step 7

## Verification

- **存在性**:
  - `internal/controller/npuverticalscaler_controller.go` 366 行(struct + Reconcile + 8 helper methods + 5 const blocks + SetupWithManager)✅
  - `internal/controller/npuverticalscaler_controller_test.go` 6 test functions · 366 行 ✅
  - `internal/controller/suite_test.go` modified(NPUVerticalScaler 加 WithStatusSubresource)✅
  - `cmd/main.go` modified(register reconciler · replace `_ = ingestor` placeholder)✅
  - `deploy/helm-charts/inference-operator/templates/rbac.yaml` modified(npuverticalscalers 9 行新 RBAC rules · 3 块 resources)✅
  - `operators/inference-operator/DESIGN.md` modified(new §5.4 with 6 subsections · 100+ 行)✅
  - `docs/devlog/phase-8-t007.md` (this file)✅
- **完整性**(plan §3-T007 acceptance vs 实际):
  - `go build ./operators/inference-operator/...` clean ✅
  - `go test ./internal/controller/... -run TestNPUVerticalScaler` PASS — 6 cases ✅
  - Reconcile loop behaviour per ADR-0012 §5 mutation model · 8 steps · 全 verify(test case 1 NoData / 2 busy / 3 idle / 4 cooldown / 5 FIFO / 6 TargetNotFound · 各 cover 1-2 步)✅
  - Phase 5/6/7 modelservice_controller tests preserved:`go test ./internal/controller/...` 全 PASS · 既有 deployment_builder + modelservice + phases tests 0 regression ✅
  - DESIGN.md §5.4 + RBAC + chart + main.go 全 ship ✅
  - envtest cases compile clean:实际用 fake client(inference-operator suite_test.go 既有 pattern)· 不依赖 setup-envtest 二进制(per suite_test.go inline 注释 "fake client over envtest because ... setup-envtest binaries are not provisioned in the Windows dev shell")
- **正确性**(3 项独立验证):
  - 存在性:`git status` confirm 2 new files + 5 modified files
  - 完整性:`go vet ./...` clean · `go build ./...` clean · `helm lint --strict deploy/helm-charts/inference-operator/` 0 chart failed
  - 正确性:6 tests 各 cover 不同 axis · NoData test 验证"target annotation unchanged + ScaleHistory not appended" · Cooldown test 验证"annotation unchanged + remainder in [170, 190]" · FIFO test seeded 10 + 1 = expect 10(not 11) + tail Reason != "seeded"

## Carry-forward

- **deployment_builder Pod label propagation**(out-of-T007-scope · Phase 8 polish task 或 T103 fixture workaround):deployment_builder.go reconcileDeployment 内读 `ms.Annotations[npu.huawei.com/slice-template]` + 在 Pod template metadata.labels 同 key + 同 value · 同时 read AnnotationManagedBy hint 在 chart README 文档化 GitOps ignoreDifferences 约定
- **T008**(AllocateBundle controller wiring · npu-dra-driver 模块):消费 Pod label `npu.huawei.com/slice-template=<name>` · 解析 NPUSliceTemplate · Engine.Decompose · AllocateBundle · N allocations。注意 T007 patch 的是 ModelService annotation · T008 期待 Pod 上有 label · 必须等 deployment_builder propagate annotation → label · T103 kind smoke 可用 fixture 直接 stamp Pod label 验证 T008 wiring 独立
- **T103**(kind smoke ext)assertion strategy:
  - NPUVerticalScaler.status.conditions[Active]=True 在 reconcile 跑过后(需 mock Prometheus 或用 fixture metric · 或 chart `metrics.prometheusURL=""` 让 ingestor 走 NoData stay 路径)
  - ModelService annotation `npu.huawei.com/slice-template=<expected>` 在 ScalingTriggered 后(需 fixture metric injection · 或 fixture 直接 stamp 期望值)
  - ScaleEvent 计数 / lastScaleTime 检查(需 reconcile 跑至少一次)
- **operator GitOps guidance**(Phase 9 doc polish 或 README 当前补):chart README 加 "If using ArgoCD: spec.ignoreDifferences with jsonPointers: ['/metadata/annotations/npu.huawei.com~1slice-template'] on ModelServices annotated with ocloud.edge.example.com/vertical-scaler-managed=*"
- **T107 checkpoint**:本 commit 在 checkpoint 行 status table 标 "T007 landed · 6 controller tests PASS · annotation-based mutation adaptation in carry-forward 1 line"
