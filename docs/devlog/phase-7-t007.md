# P7-T-007 · Template engine + composition decomposition + reconciler

- **Commit**: (this commit)
- **Date**: 2026-05-20
- **Duration**: plan 1.5d / actual ~1.5d (main agent direct · 4 new files in template pkg + 2 new files in controller pkg + suite_test edit + cmd/main.go register + DESIGN.md §3.9)

## Intent

Per ADR-0011 §1 §4 + phase7-plan.md §3 P7-T-007:落地 template engine(Validate + Decompose)+ NPUSliceTemplate reconciler(stamps Validated + Allocatable conditions + FallbackAppliedReason)。Engine 把 user-declared composition mapped 到 FixedTemplateBundle (allocator T105 input contract);reconciler 让用户在 `kubectl describe npust qwen-8b-pd-pair` 看到 Validated 状态 + 决策 reason。

## Path adaptations

- 计划 §3-T007 Allowed Paths 全部 covered:
  - `internal/template/engine.go` + types.go + engine_test.go ✅
  - `internal/controller/npuslicetemplate_controller.go` + _test.go ✅
  - `cmd/main.go` (register controller + add --enable-template-controller flag) ✅
  - `internal/controller/suite_test.go` (add NPUSliceTemplate to WithStatusSubresource + helper newStatusSubresourceClientBuilder)
  - `DESIGN.md` §3.9 ✅
- 计划列了 3 envtest cases · 实际落地 3 fake-client cases(per P3 local-env honesty + 既有 suite_test.go 已用 fake client 不用 envtest)。Test acceptance 不变 — case 数 + 验证 condition 行为完全 match。
- 增加 `template.AsValidationError` + `template.IsValidationError` helpers(plan 没列但代码可读性 + reconciler 调用方便)。
- 增加 `statusEqual` helper:计划没列但避免无限 reconcile churn(LastTransitionTime 每次 time.Now() 都不同 → 不做 shallow compare 会一直 update)。

## Debugging trail

- 无 false start。engine + reconciler 单次写成 · 单次 PASS。
- 第一次 `go test`:6 engine cases PASS · 3 reconciler cases PASS · 全模块 10 packages OK。
- 第一次反思:`computeStatus` 是不是应该 return error 而不是吞下?决定保持 swallow + 把 error 编码进 conditions[Validated]Status=False — 让 Reconcile 永远不报错 requeue(只有 client.Get / Status().Update 失败才 requeue)· 这是 controller-runtime 推荐模式 for "validation-driven" reconcilers。
- 第二次反思:Phase 7 W1 把 Allocatable 写"always True when Validated=True"是不是太乐观?是 placeholder(已在 condition.Message 明示)· T105 allocator 真做 pool-availability check 时替换。如果 W1 写更复杂的 placeholder(比如读 NPUPool.status.totalSlices vs bundle.TotalSlices),那是 T105 工作的一部分 · 不在 T007 范围。

## Key decisions

- **Engine.Decompose calls Engine.Validate internally**:让 reconciler 只调一次 Decompose;Validate 单独暴露给 future T105 allocator(allocator 想先 validate 再 read bundle)。
- **FallbackAppliedReason 字符串排序**(items by Template name alphabetically):防止 map iteration order 不确定 → status 抖动 → `kubectl get -o yaml` diff 不 idempotent。
- **`dynamic-shard` 被拒绝 regardless of FallbackStrategy** in Phase 7 W1:ADR-0011 §後果 + DESIGN.md §3.8 已明示 · 无 driver-layer 突破或 KEP-4815 GA 之前 · refuse 与 fixed-template-combination 行为相同(都 reject)· 减少 case 数 + 简化 mental model。
- **`statusEqual` 做 shallow compare 忽略 LastTransitionTime**:防止 metav1.Condition 默认 transition time 字段每次 reconcile 都更新 → infinite churn。但保留 Type + Status + Reason 比较 · 真状态变化时 status update 仍 fire。
- **Reconciler 自动 inject `template.New()` if Engine nil**:让 SetupWithManager 调用 site 不必每次显式构造;tests 显式注入(避免测试间共享状态;Engine 是 stateless 但好 practice)。
- **Recorder Eventf with ValidationError.Reason as Type**:operators `kubectl describe npust qwen-8b-pd-pair` 看到 `Warning DynamicShardNotSupported ...` event · 直接定位问题原因。

## Verification

- 存在性:
  - `internal/template/types.go` — FixedTemplateBundle + FixedTemplateItem + IsEmpty + TotalSlices ✅
  - `internal/template/engine.go` — Engine + Validate + Decompose + ValidationError + 4 Reason consts + 2 helpers (IsValidationError / AsValidationError) ✅
  - `internal/template/engine_test.go` — 6 test functions ✅
  - `internal/controller/npuslicetemplate_controller.go` — Reconciler + SetupWithManager + Reconcile + computeStatus + statusEqual ✅
  - `internal/controller/npuslicetemplate_controller_test.go` — 3 test functions ✅
  - `internal/controller/suite_test.go` — `NPUSliceTemplate` added to default WithStatusSubresource + new `newStatusSubresourceClientBuilder` helper ✅
  - `cmd/main.go` — `--enable-template-controller` flag (default true) + reconciler registration ✅
  - `DESIGN.md` §3.9 Template engine + composition decomposition section ✅
- 完整性(verified by execution):
  - `go build ./...` clean ✅
  - `go test ./...` — 10 packages 全 PASS:api + cmd + allocator + controller + publisher + source + source/mockjson + source/realascend + source/realascend/npusmi + **template** ✅
  - `go test ./internal/template/... -v` 6 sub-cases PASS · 2.994s ✅
  - `go test ./internal/controller/... -run NPUSliceTemplate -v` 3 sub-cases PASS · 0.32s + 0.02s + 0.02s ✅
- 正确性:
  - Qwen-PD sample (`Spec.Composition: [vir04, vir08]`) → reconciler 写 Status.FallbackAppliedReason = "decomposed into 1×vir04 + 1×vir08" + Validated=True + Allocatable=True ✅
  - DeepSeek strict (`refuse + dynamic-shard`) → reconciler 写 Validated=False reason=DynamicShardNotSupported · FallbackAppliedReason 留空 ✅
  - 决策 sortedness:`TestDecomposeFixedTemplateCombination` 验证 sorted reason "3×vir04 + 1×whole"(v 在 w 前 · alphabetical)✅
  - statusEqual 防 churn:N/A 本 commit 不跑 Reconcile 多次 · 但代码 shallow compare 路径 + condition by Type lookup verified

## Carry-forward

- **T105** (allocator dynamic-slice extension) — 读 NPUSliceTemplate via `npu.huawei.com/slice-template=<name>` Pod label · 调 `engine.Decompose` 拿 bundle · pass to allocator · 替换本 controller W1 "always True Allocatable" 占位为 real bundle-vs-pool availability check
- **T103** (kind smoke ext) — fixture apply `npuslicetemplate_qwen_pd.yaml` · wait + assert `kubectl get npust qwen-8b-pd-pair -o jsonpath={.status.conditions[?(@.type=="Validated")].status}` = "True" + Allocatable=True + FallbackAppliedReason="decomposed into 1×vir04 + 1×vir08"
- **Phase 8 vertical scaling controller** — 读 `NPUSliceTemplate.status.fallbackAppliedReason` + 自己的 busy-idle metric · 决定是否 trigger "重启切片" rollout(arch §13 row · ADR-0011 §1 fallback substrate)
- **Phase 8+ dynamic-shard 解禁** — 当 driver-layer 暴露 partition API 或 KEP-4815 GA 时:Engine.Validate 删 `DynamicShardNotSupported` early-reject + Engine.Decompose 增 dynamic-shard 处理路径(Bundle 加 DynamicShard-type FixedTemplateItem · allocator T105 / T105-v2 处理)
- **Phase 9 multi-tenancy** — NPUSliceTemplate 改 namespaced(per ADR-0011 §推翻条件 row 4)· reconciler 也要从 cluster scope 改 namespace scope (For 时加 NamespacedFilter)
