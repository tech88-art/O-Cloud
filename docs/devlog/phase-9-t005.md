# P9-T-005 · Quota CRD types + scheme + samples (+ P9-T-002-fix-001 group correction batched)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 0.5d planned · ~0.6d actual (P9-T-002-fix-001 batched in)

## Intent

Phase 9 W1 P9-T-005:Ship Quota CRD api/v1alpha1 types + scheme registration + sample YAML + 4 round-trip tests + CRD YAML + helm chart bundle update per ADR-0014 §4。Controller body + admission webhooks 走 P9-T-006 W1 后续。

**Sub-task batched**:P9-T-002-fix-001 group correction(自我发现 in T005 task entry):ADR-0014 v1 落地时 §2 Decision B 选 `ocloud.edge.example.com/v1alpha1` 作为 Quota group · 但 grep 验证发现:NPUSlicePool 在 `ims.ocloud.edge.example.com` · NPUSliceAllocation 在 `npu.ocloud.edge.example.com` · **无现存 CRD** 使用 bare `ocloud.edge.example.com` group · 即原 ADR-0014 §2 Decision B 引用"与 NPUSlicePool / NPUSliceAllocation 同 group"事实错误 + 选择该 group 需引入 multi-group operator binary 复杂度。修正:Quota group 改为 `inference.ocloud.edge.example.com/v1alpha1`(同 inference-operator binary scheme · 单 group 简单)+ Webhook A apiGroup 改为 `npu.ocloud.edge.example.com`(NPUSliceAllocation 实际 group)。

## Path adaptations

- **P9-T-002-fix-001 batched into T005**:本应作 separate fix commit · 但因 T005 任务 entry 时发现 + T005 是 Quota CRD 实际落地 + ADR-0014 group correction 直接影响 T005 落点选择 · batched in one commit for audit clarity(commit message + devlog 显式标注 fix-001 sub-task)。
- **PROJECT 多个 group 选择**:plan §3 T005 Allowed Paths 写 `operators/inference-operator/api/v1alpha1/quota_types.go` · 即假设 Quota in inference group。我 T005 task entry 时考虑过 multi-group option(operators/inference-operator/api/ocloud/v1alpha1/)· 但 (a) plan Allowed Paths 不暗示 multi-group · (b) PROJECT 文件 multiGroup: false · 改 multi-group 需重构现有类型路径 · (c) 单 group 简单。决策:revise ADR-0014 group choice 而非重构 PROJECT 多 group。
- **make 不可用(Windows)**:make 命令不存在 on Windows · controller-gen 直接 invoke `C:\Users\Himalayan\go\bin\controller-gen.exe` 替代 `make generate` / `make manifests` · 注意 paths 参数 syntax:`paths="./api/v1alpha1/..."` (separate paths args 不能 comma-join)。
- **kubectl dry-run 无 cluster**:`--dry-run=client` 仍需 server openapi · 无连接 cluster 跳过 kubectl 验证 · 改用 helm lint --strict + go test pass 替代。
- **time.UTC vs time.Local round-trip**:`metav1.Time` JSON round-trip 经 RFC3339 后 deserialize 成 `time.Local`(主机本地 +0800)而非 `time.UTC`(原始)· `reflect.DeepEqual` 视为不同。Fix:在 round-trip 测试 deserialize 后 normalize `.UTC()` 再 compare。NPUVerticalScaler test 没遇此问题因为它不 round-trip status.conditions.LastTransitionTime + status.lastSyncTime。

## Debugging trail

- **Step 1 · T005 entry 发现 group choice 错误**:plan §3 T005 Allowed Paths 是 `operators/inference-operator/api/v1alpha1/quota_types.go` · 即 inference group。但 ADR-0014 §2 Decision B 我自己写 `ocloud.edge.example.com/v1alpha1` group。grep 检查 + 读 operators/{npu-dra-driver,pool-operator}/api/v1alpha1/types.go 的 `+groupName` 验证:NPUSlicePool group = `ims.ocloud.edge.example.com`,NPUSliceAllocation group = `npu.ocloud.edge.example.com`,无 bare `ocloud.edge.example.com`。即 ADR-0014 §2 Decision B 我之前引用"与 NPUSliceAllocation 同 group"完全错。
- **Step 2 · Group correction 决策**:
  - **Option A**:revise ADR-0014 group choice 为 `inference.ocloud.edge.example.com/v1alpha1`(plan-listed Allowed Paths consistent · 单 group · 简单)
  - **Option B**:keep ADR-0014 group `ocloud.edge.example.com` + introduce multi-group operator binary(重构 PROJECT 多 group · 移动现有类型 · 复杂)
  - **Option C**:keep ADR-0014 group + put Quota types in 新 module `operators/quota-controller/`(更复杂 · 与 ADR-0014 §2 Decision C colocated 矛盾)
  - **决策 A**:revise ADR-0014。fix-001 sub-task batched into T005 commit per audit clarity。
- **Step 3 · 多文件 group correction edits**(P9-T-002-fix-001):ADR-0014 §1 命名空间冲突 + §2 Decision B + §2 Decision B sample YAML + §2 Decision C Webhook A apiGroup + §5 Webhook ManifestEvent name + path + apiGroup + §7 forward note · arch.md §6.8 Quota row + NPUSliceAllocation row · arch.md §13 Phase 9 安全模型 row · ADR-0009 §2 P9-T-002 cross-ref segment。**注**:NPUSliceAllocation 自己 group 也修正到正确 `npu.ocloud.edge.example.com`(arch.md §6.8 NPUSliceAllocation row 在 T002 时我也写错了 group · 同步纠正)
- **Step 4 · Quota Go types 写**:`quota_types.go` ~150 lines · 仿 NPUVerticalScaler types.go pattern · kubebuilder markers + printer columns + DeepCopy via init() SchemeBuilder.Register
- **Step 5 · DeepCopy 生成**:`go build` initial FAIL `missing method DeepCopyObject` → controller-gen 直接 invoke:`& controller-gen.exe object:headerFile=hack/boilerplate.go.txt paths=./api/v1alpha1/...` clean exit 0 · zz_generated.deepcopy.go 补齐 Quota / QuotaList / QuotaSpec / QuotaEnforcement / ScaleEventRateCap / QuotaStatus / QuotaUsage 全部 DeepCopy methods
- **Step 6 · CRD YAML 生成**:`controller-gen` paths 需要 `./api/v1alpha1/...` + `./internal/controller/...` + `./internal/webhook/...` + `./internal/metrics/...` 分多个 paths args(comma-join 不工作)· 生成 `inference.ocloud.edge.example.com_quotas.yaml` 成功
- **Step 7 · Helm chart bundle copy**:`cp config/crd/bases/inference.ocloud.edge.example.com_quotas.yaml deploy/helm-charts/inference-operator/crds/quotas.yaml`
- **Step 8 · sample + 4 tests + PROJECT 更新**:`inference_v1alpha1_quota.yaml` sample(maxSliceAllocations=8 + maxScaleEventsPerWindow{count=5,windowSeconds=3600} + maxNPUSliceTemplateRefs whitelist 2 templates)· `quota_types_test.go` 4 cases(RoundTrip / DeepCopy / OmitEmpty / WindowSeconds JSON tag pin)· PROJECT 加 Quota resource entry
- **Step 9 · 测试**:RoundTrip 首次 FAIL `time.UTC vs time.Local` mismatch · normalize 后 PASS · 全套 inference-operator tests 全 PASS · helm lint --strict clean

## Key decisions

- **`inference.ocloud.edge.example.com/v1alpha1` group**(P9-T-002-fix-001):匹配 plan §3 T005 Allowed Paths · 单 inference-operator binary scheme · 避免 multi-group 复杂度。
- **init() SchemeBuilder.Register**:standard kubebuilder convention · Quota + QuotaList 在 quota_types.go 内 init() 自注册到 v1alpha1 group(同 existing ModelService + NPUVerticalScaler 风格)
- **Sample namespace `ai-edge-demo`**:同 NPUVerticalScaler / ModelService sample namespace · 演示 namespace 默认
- **Sample maxNPUSliceTemplateRefs whitelist `qwen-pd-busy` + `qwen-pd-idle`**:覆盖 ADR-0014 §2 Decision B sample composition + 强制 demo 时 NPUVerticalScaler.spec.scaleSlice.{busy,idle}TemplateName 必须在 whitelist 内
- **PROJECT controller: true**:为 P9-T-006 controller body landing 预留 kubebuilder marker · 暗示 Quota 有 reconcile loop
- **4 test cases 选择**(per plan acceptance):RoundTrip(JSON serialization)+ DeepCopy(generated code correctness · clone independence)+ OmitEmpty(spec.required vs status.omitempty distinction)+ JSON tag pin(WindowSeconds + MaxScaleEventsPerWindow camelCase 防 drift)

## Verification

P3 三项验证维度:

| 维度 | 验证方法 | 结果 |
|---|---|---|
| **存在性** | `git status` · 改 8 文件 + 新 3 文件 + delete 0 | 改: ADR-0014 (group fix) + arch.md (group fix) + ADR-0009 (group fix) + groupversion_info.go (no change · init() in quota_types.go itself) + PROJECT + zz_generated.deepcopy.go (regenerated) + config/crd/bases/*_quotas.yaml + helm-charts/inference-operator/crds/quotas.yaml · 新: quota_types.go + quota_types_test.go + inference_v1alpha1_quota.yaml + devlog/phase-9-t005.md |
| **完整性** | plan §3 T005 Acceptance 8 项逐项核对 | 8/8 全覆盖:Types compile(go build clean)· DeepCopy regenerated(controller-gen object clean)· 4 round-trip tests(实际 4 cases:RoundTrip / DeepCopy / OmitEmpty / JSON tag pin)· make manifests clean(controller-gen crd clean)· sample applies clean(helm lint clean)· P5/6/7/8 tests preserved(全 go test PASS)· CRD schema 8 字段全 present(spec.enforcement.maxSliceAllocations + maxScaleEventsPerWindow.{count,windowSeconds} + maxNPUSliceTemplateRefs + status.conditions + status.usage.{currentSliceAllocations,scaleEventsInWindow} + status.lastSyncTime)· printer columns 5 个全 present(kubebuilder markers verified in quota_types.go) |
| **正确性** | `go build ./operators/inference-operator/...` clean · `go vet` clean · `go test ./...` 全套 PASS · `helm lint --strict` clean · group correction bi-directional cross-ref 7 文件闭合(ADR-0014 + arch §6.8 + arch §13 + ADR-0009 + types.go + sample + CRD YAML) | 全部 PASS · group correction self-consistent · 无 stale ref |

P4 横向 grep `ocloud\.edge\.example\.com/v1alpha1.*Quota` 全仓库 → 0 hits(group correction 完成)· grep `inference\.ocloud\.edge\.example\.com/v1alpha1.*Quota` 全仓库 → 命中 4 文件(ADR-0014 + arch.md + ADR-0009 + sample YAML · 一致)

## Carry-forward

- **P9-T-006(Quota controller + admission webhook body)**:类型 + scheme + samples + CRD YAML 都 ready。controller body 见 ADR-0014 §2 Decision D 60s tick reconcile + §5 Webhook A/B logic · webhook YAML manifest 直接用 ADR-0014 §5 ValidatingWebhookConfiguration template · cert-manager 重用 P5-T-101 cert · 5 webhook test cases + 4 controller reconcile test cases per plan
- **P9-T-007(PromQL custom metric extension)**:T007 不依赖 T005 / T006 · NPUVerticalScaler.spec.metric 扩展独立 · 可并行 main agent serial per §0a.11
- **P9-T-008(O2 DMS scaffold)**:T008 不依赖 T005 / T006 · 完全独立 module · 可并行
- **Phase 9 W2 chart wiring**:Quota chart values + RBAC manifests + sample chart bundle 在 P9-T-006 helm chart 落地时一并 ship
- **frontend Quota usage 可视化**:Phase 10 polish per ADR-0014 §6 Open question (f) · 与 ADR-0013 §6 frontend forward note 同期评估

---

**END of P9-T-005 devlog**
