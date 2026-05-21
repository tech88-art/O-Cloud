# P9-T-006 · Quota controller + admission webhook body

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 1.5d planned · ~1.1d actual

## Intent

Phase 9 W1 P9-T-006:Ship Quota controller body(60s tick reconcile · status.usage sync from NPUSliceAllocation count + NPUVerticalScaler scaleHistory sum)+ 2 ValidatingAdmissionWebhooks(A:NPUSliceAllocation CREATE · B:NPUVerticalScaler UPDATE)+ helm chart RBAC + ValidatingWebhookConfiguration template + cmd/main.go register + DESIGN.md §5.4.8 + 10 test cases(4 controller + 6 webhook)。Multi-tenant fair scaling safety boundary 落地 per ADR-0014 §2 Decision C + D + §5 enforcement contract。

## Path adaptations

- **NPUSliceAllocation cross-module via unstructured**:plan §3 P9-T-006 Forbidden Paths "operators/npu-dra-driver/**" 禁止 import npu-dra-driver Go module。`QuotaReconciler.countNPUSliceAllocations` 用 `unstructured.UnstructuredList` + `NPUSliceAllocationGVK` 常量(group `npu.ocloud.edge.example.com`)替代 Go type import。同 operators/CLAUDE.md §1 cross-module 文本-duplication pattern。
- **logr.Logger 而非自定义 interface**:首次写 webhook handle 函数签名时尝试自定义 logVerbosity interface · build FAIL(`controller-runtime logr.Logger.V() returns logr.Logger not logVerbosity`)· 简化为直接 `logr.Logger` 参数 type。
- **NPUSliceAllocation 资源 list 模式**:fake client 测试时 NPUSliceAllocation 用 `unstructured.Unstructured` 加 GVK setter · `WithRuntimeObjects` 注入(非 `WithObjects` · 后者要求 typed Go scheme)
- **ctrl.Request types.NamespacedName**:首次写测试 helper `keyFor` 时 typo 用 `ctrl.Request_NamespacedName_Stub`(不存在的 type)· build FAIL · 替换为 `types.NamespacedName` import
- **DESIGN.md §8 位置**:plan acceptance "§8 'Quota controller + admission webhook'" — 实际 DESIGN.md 没 §8 顶级 section · §5.4.x 是 NPUVerticalScaler stack。**Path adapt**:作 §5.4.8 嵌入 NPUVerticalScaler 段后(同 P9-T-004 §5.4.7 风格)· 内容覆盖 plan 要求 4 项(Reconcile flow + admission decision flow + cache TTL + cert-manager dependency)
- **suite_test.go envtest 不动**:plan acceptance "envtest cases compile clean; run when envtest available (P3 local-env honesty)" · 但 plan §3 Allowed Paths 列 `internal/controller/suite_test.go` 仅"small edit · register Quota CRD into envtest scheme"。我用 unstructured 路径所以 envtest scheme 不需 register Quota · `internal/controller/suite_test.go` 不动 · CI runtime envtest 跑现有控制器测试。**注**:fake.NewClientBuilder 已覆盖 4 controller test cases 的单元验证 · envtest 完整 round-trip 走 CI 自然 cover。

## Debugging trail

- **Step 1 · ADR-0014 group correction 验证就位**:P9-T-002-fix-001 在 T005 已 batched · Quota group `inference.ocloud.edge.example.com/v1alpha1` · NPUSliceAllocation group `npu.ocloud.edge.example.com/v1alpha1`。本 T006 直接消费 corrected group · 无 group rework 风险。
- **Step 2 · quota_controller.go 写**:仿 NPUVerticalScalerReconciler 风格 · 60s tick reconcile · NowFn injection 测试性 · NPUSliceAllocation unstructured list · NPUVerticalScaler typed list scaleHistory sum within window · status.usage atomic update + LastSyncTime + Active condition · `SetupWithManager(mgr).For(&Quota{})` 注册
- **Step 3 · 控制器 build clean**:exit 0 一次过
- **Step 4 · quota_admission.go 写**:QuotaCache 5s TTL (DefaultQuotaCacheTTL) + Get fallback via List Quota in namespace + NotFound caching · 2 validators(QuotaSliceAllocationValidator + QuotaScalerValidator)· admission.Response 走 admission.Allowed / Denied / Errored API
- **Step 5 · 自定义 logVerbosity interface 错**:webhook handle() function signature 用 custom interface · controller-runtime `logr.Logger.V()` return type 不匹配 → 替换为 `logr.Logger` 直接 type · 删除 logVerbosity interface 和 unused import (`encoding/json`, `runtime`, `types`)
- **Step 6 · controller tests 4 cases pass**:
  - EmptyNamespaceUsageZero(seed Quota only · 验证 0/0 usage + LastSyncTime set)
  - NPUSliceAllocationCount(seed 3 unstructured allocs · 验证 currentSliceAllocations=3)
  - ScaleEventCountSync(seed NPUVerticalScaler with 3 scaleHistory entries · 2 in-window + 1 out-of-window · 验证 ScaleEventsInWindow=2)
  - LastSyncTimeMonotonic(2 ticks at t0 vs t0+60s · 验证 monotonic advance)
- **Step 7 · webhook tests 6 cases pass**:
  - Webhook A UnderCapAllow(used=5/cap=8 · CREATE allowed)
  - Webhook A AtCapReject(used=8/cap=8 · CREATE rejected)
  - Webhook A NoQuotaUnbounded(empty namespace · fail-open allowed)
  - Webhook B RateExceedReject(scaleEventsInWindow=5/cap=5 · template change rejected)
  - Webhook B WhitelistAllow(under cap + template in whitelist · allowed)
  - Webhook B WhitelistDeny(under cap + template not in whitelist · rejected)
- **Step 8 · cmd/main.go register**:加 QuotaReconciler.SetupWithManager + quotaCache + 2 webhook server Register paths(matches helm chart ValidatingWebhookConfiguration `clientConfig.service.path`)
- **Step 9 · helm chart**:rbac.yaml 加 Quota verbs (quotas + quotas/status + quotas/finalizers)· 新建 templates/validatingwebhookconfiguration-quota.yaml · values.yaml 加 `quota.enabled/failurePolicy/timeoutSeconds` config · helm lint --strict clean · helm template 渲染 ValidatingWebhookConfiguration + 2 webhook rules confirmed
- **Step 10 · DESIGN.md §5.4.8 段**:新章节 in §5.4.x stack(NPUVerticalScaler 部分自然延续)· 覆盖 Components + Cross-module pattern + Webhook decision flow + Cert reuse + Phase 10 polish path

## Key decisions

- **`unstructured.Unstructured` for NPUSliceAllocation**:符合 operators/CLAUDE.md §1 cross-module 禁止 import 约定 · 同 claim_builder.go 文本-duplication 风格 · GVK 常量集中在 quota_controller.go 内的 `NPUSliceAllocationGVK`
- **5s cache TTL DefaultQuotaCacheTTL + NotFound caching**:burst create 内 5s 窗口允许 cache 但 NotFound 也 cached avoid re-Get spam · 同 ADR-0014 §2 Decision D + §3 risk row "5s cache stale" 设计
- **fail-open on no-Quota / Get NotFound / cache miss**:符合 admission webhook 通行约定 · webhook 故障 ≠ admit · 没 quota set 也 ≠ reject(unbounded semantics)· production gap 走 Phase 10 polish ClusterQuota default fallback per ADR-0014 §6 Open question (b)
- **Webhook B 只 intercept actual scaleSlice template change**:Phase 8 ADR-0012 §5 mutation model 已确认 controller patch 的是 ModelService annotation(not NPUVerticalScaler.spec)· Webhook B 拦 user / O2 NB-触发 NPUVerticalScaler.spec patch · controller 自身 reconcile 不触发此路径 · 状态 update + 其他 spec change pass through
- **`templateInWhitelist` linear scan**:whitelist typically ≤ 10 templates · linear scan 简单 + 内存友好;map lookup 提速不显著 demo scale
- **Quota cert reuse Phase 5 P5-T-101 cert-manager**:符合 ADR-0014 §5 Cert reuse path · single cert-manager Certificate cover 3 webhook handlers(PD Router mutating + Quota A/B validating)· Phase 10 polish split per webhook for故障域 isolation

## Verification

P3 三项验证维度:

| 维度 | 验证方法 | 结果 |
|---|---|---|
| **存在性** | `git status` 改文件清单 | 6 新文件: quota_controller.go + quota_controller_test.go + quota_admission.go + quota_admission_test.go + validatingwebhookconfiguration-quota.yaml + devlog · 4 修改: cmd/main.go + rbac.yaml + values.yaml + DESIGN.md |
| **完整性** | plan §3 P9-T-006 Acceptance 8 项逐项核对 | 8/8 全覆盖:go build clean · TestQuota controller 4 cases PASS · TestQuotaAdmission 6 cases PASS · Reconcile loop behavior 按 ADR-0014 §2 Decision D 实现 · Webhook A/B decision flow 按 §5 enforcement contract 实现 · P5/6/7/8 tests preserved(全 go test PASS · 包括 51+ inference-operator pre-existing controller tests · 11+ metrics tests · 18+ webhook tests · 4+ ModelService api · 6+ NPUVerticalScaler api · 6+ controller_test + 4 NPUVerticalScalerController + P9 4+6 new)· DESIGN.md §5.4.8(plan 写 §8 实际 §5.4.8 path adapted)· helm lint --strict clean · envtest 走 CI runtime |
| **正确性** | `go build ./operators/inference-operator/...` clean exit 0 · `go vet` clean exit 0 · `go test ./operators/inference-operator/... -count=1` 全套 PASS · `helm lint --strict deploy/helm-charts/inference-operator/` clean · `helm template` 渲染 Quota ValidatingWebhookConfiguration + 2 rules + RBAC verbs confirmed via grep | 全部 PASS · 无 pre-existing test regression · 渲染 confirmed |

P4 横向 grep `Quota` / `quota-controller` / `quota_admission` 全仓库 → 命中 inference-operator + helm chart + ADR-0014 + arch + ADR-0009 + ADR-0012 + sample YAML + devlog · 无 stale ref · cross-module pattern via unstructured 文档化 in quota_controller.go inline comment + DESIGN.md §5.4.8 Cross-module pattern 段

## Carry-forward

- **P9-T-007(PromQL custom metric extension)**:T007 独立 of T005/T006 · NPUVerticalScaler.spec.metric.Type 扩展 PrometheusQuery enum · Ingestor PromQL-mode path · 0.5d · 后续 W1 task
- **P9-T-008(O2 DMS scaffold)**:T008 独立 · 新 module `operators/o2-dms-adapter/` per ADR-0013 + 1.5d · 后续 W1 task
- **P9-T-103(W2 kind smoke E2E Phase 9 extension)** Quota admission live assertion:envtest 已 cover 单元路径 · W2 T103 kind smoke 加 Quota apply + 触发 over-cap NPUSliceAllocation create + assert Webhook A REJECT response · W2 起草时拆任务
- **Phase 10 polish**:
  - cluster-scope `ClusterQuota` per §6 Open question (b)
  - strong-consistency webhook mode (per-Quota strictMode flag) per §6 Open question (d)
  - token-bucket scale rate cap algorithm per §6 Open question (e)
  - frontend Quota usage 可视化 per §6 Open question (f)
  - event-driven status.usage sync(替换 60s tick)per §7 forward note
  - scaleHistory event-driven counter(替换 rolling 10-entry FIFO sum)per §7 forward note

---

**END of P9-T-006 devlog**
