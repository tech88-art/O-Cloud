# P9-T-004 · deployment_builder annotation → Pod label + ResourceClaim annotation propagation polish

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 0.5d planned · ~0.7d actual (claim_builder.go scope expansion accounts for extra)

## Intent

Phase 8 carry-forward task: deployment_builder propagation chain closure. Previously phase8/install.sh `cmd_demo_bundle_path` directly stamped `npu.huawei.com/slice-template` annotation on ResourceClaims via `kubectl annotate resourceclaim` to demo P8-T-008 bundle path · because the propagation chain (NPUVerticalScaler → ModelService annotation → ??? → ResourceClaim annotation that claim_controller consumes) was incomplete. P9-T-004 closes this chain so claim_controller's `AnnotationSliceTemplate` dispatch triggers naturally without manual annotate step.

## Path adaptations

- **claim_builder.go scope expansion** (out of strict plan §3 T004 Allowed Paths):Plan listed `deployment_builder.go` + tests + install.sh + DESIGN.md + devlog · 但 NOT `claim_builder.go`。然而 chain endpoint claim_controller reads `npu.huawei.com/slice-template` ANNOTATION on ResourceClaim(not Pod label · per `operators/npu-dra-driver/internal/controller/claim_controller.go` line 103-122 + dispatch logic)。Pod label propagation alone不能让 bundle path 触发 · 需 ResourceClaim annotation。
  - **Plan author 假设**:DESIGN.md acceptance "propagation chain ... claim_controller label lookup" 暗示 plan author 以为 claim_controller 读 Pod LABEL · 实际读 ResourceClaim ANNOTATION
  - **K8s DRA upstream contract**:ResourceClaimTemplate.spec.metadata.annotations 自动复制到 materialised ResourceClaim · 是 propagation 自然 K8s 路径
  - **决策**:扩展 scope 添加 claim_builder.go 修改 — copy `ms.Annotations[SliceTemplateAnnotation]` 到 `ResourceClaimTemplate.Spec.ObjectMeta.Annotations`(plan acceptance "slice-template propagation chain now end-to-end natural" 实质需要此修改才能满足)
  - **路径备选(reject)**:(a) 保留 install.sh workaround = T004 partial · install.sh acceptance 不满足。(b) 修 PD Router webhook 添加 stamp 路径 = ADR-0008 scope expansion · 更违反 boundary。claim_builder.go 扩展是最 minimal + 自然 path。
- **DESIGN.md §5.0.2/§5.0.3 不存在**:Plan 列 §5.0.2 deployment_builder section + §5.0.3 propagation chain。实际 DESIGN.md §5.0.2 = ProxyImage chart flip · §5.4.x 才是 NPUVerticalScaler 部分。**Path adapt**:加新 §5.4.7 "Annotation → Pod label propagation chain (P9-T-004 · 2026-05-21)" 在 §5.4.6 RBAC 之后(§5.4.x 是 NPUVerticalScaler stack 自然延续)。
- **modelservice_controller_test.go 不动**:Plan 列 "small edit if Reconcile-level integration test needs propagation assertion · likely 1 new case"。`if needs` 条件下:deployment_builder_test.go TestSliceTemplateLabelPropagation 已覆盖 buildDeployment + buildResourceClaimTemplate 两端 unit testing · Reconcile-level integration test 额外 case 不增信息(只 cover orchestration · 已被 unit tests 覆盖)。M4 不为完整性堆 padding → 跳过。

## Debugging trail

- **Step 1 · 阅读 plan + 设置 todo**:plan §3 T004 acceptance = (a) Pod label propagation + (b) install.sh workaround removed + (c) kind smoke STILL pass assert.sh + (d) DESIGN.md propagation diagram。
- **Step 2 · deployment_builder.go 修改**:加 `SliceTemplateAnnotation` 常量 + buildDeployment 函数体加 propagation logic。`go build` clean。
- **Step 3 · 测试编写**:`TestSliceTemplateLabelPropagation` 4 sub-cases initially:present / absent / empty-string absent / decode-side。`go test ./internal/controller/...` PASS。
- **Step 4 · install.sh 编辑**:`cmd_demo_bundle_path` function deleted · `cmd_all` 移除调用 · case switch 移除 `demo-bundle-path` · usage 字符串更新 · 加 explanatory comment block 说明 propagation chain 已 close。
- **Step 5 · DESIGN.md §5.4.7 加段**:初版 描述 deployment_builder propagation only (假设 Pod label → claim_controller 读 Pod label)。
- **Step 6 · assert.sh 验证 chain**:grep assert.sh 发现 line 100 期望 ResourceClaim 上的 `npu.huawei.com/slice-template` annotation · line 113-120 hard-fail 在 preferred-hccs-ring 缺失 时 trigger · preferred-hccs-ring 仅 bundle path(claim_controller line 422)stamps。如果 ResourceClaim annotation 缺 → bundle path 不触发 → 走 Phase 5 fallback path → claim 仍 allocated → preferred-hccs-ring NOT stamped → assert.sh HARD FAIL。
- **Step 7 · claim_controller.go 代码阅读**:line 103-122 注释明示 "AnnotationSliceTemplate: when this annotation is set on a **ResourceClaim**, the claim controller dispatches the AllocateBundle path"。即 claim_controller 读的是 ResourceClaim annotation · 不是 Pod label。
- **Step 8 · 路径决策 + claim_builder.go 扩展**:plan author 的"claim_controller label lookup"表述与实际 ResourceClaim annotation read 不一致。功能上 plan acceptance "STILL passes assert.sh" 需要 ResourceClaim annotation propagation chain end-to-end。决策:扩展 claim_builder.go scope · 添加 `ms.Annotations[SliceTemplateAnnotation]` → `ResourceClaimTemplate.Spec.ObjectMeta.Annotations` copy(K8s DRA upstream contract自动复制到 materialised ResourceClaim)。
- **Step 9 · 编辑 + 测试 + 验证**:
  - claim_builder.go 修改:加 `claimAnnotations` map 初始化 + 条件 copy slice-template annotation。
  - test 加 2 sub-cases:"ms annotation set → ResourceClaim spec carries annotation"(同时验证 pre-existing AnnotationModelServiceRef + AnnotationPreferredPool 保留)+ "ms no annotation → no slice-template on spec"。
  - 首次 build FAIL:`undefined: inferencev1alpha1.NPUSlicePoolRef`(test 用错 type 名)
  - 修正:`ms.Spec.NPUSlicePoolRef.Name = "qwen-pool"`(NPUSlicePoolRef 实际是 corev1.LocalObjectReference type · 不是独立 struct)
  - `go build` clean · `go vet` clean · `go test ./internal/controller/...` PASS 全套
- **Step 10 · DESIGN.md §5.4.7 更新**:把 propagation chain 从 single-path Pod label 升 twin-propagation Pod label + ResourceClaim annotation · diagram 添加 K8s materialise ResourceClaim 路径 + claim_controller AnnotationSliceTemplate dispatch endpoint。

## Key decisions

- **claim_builder.go 扩展为 T004 scope**(scope expansion · 文档化于本 devlog):full propagation chain 包含 Pod label(human visibility · scheduling hint)+ ResourceClaim annotation(claim_controller bundle path dispatch · functional endpoint)。plan author 假设 chain endpoint 是 Pod label 但实际是 ResourceClaim annotation · scope must expand to satisfy acceptance。
- **不添加新 file**(claim_builder_test.go):测试加到 deployment_builder_test.go 同一 TestSliceTemplateLabelPropagation function 内 · 6 sub-cases 共同覆盖 Pod label + ResourceClaim annotation chain · 一组 makeTestModelService helper 复用 · file count minimised。
- **modelservice_controller_test.go 不动**:Plan 条件 "if needs" · 测试覆盖足够(unit tests on buildDeployment + buildResourceClaimTemplate)· orchestration-level test 不增信息密度。
- **保留 cmd_demo_bundle_path comment block in install.sh**:不删 100-130 行那块 comment · explain rationale for future maintainers · 防止 inadvertent re-introduction。

## Verification

P3 三项验证维度:

| 维度 | 验证方法 | 结果 |
|---|---|---|
| **存在性** | `git diff --stat HEAD` · 改 4 文件 + 新 1 文件 · 与 Allowed Paths(+ claim_builder.go expansion documented)对齐 | 5 文件 modified: deployment_builder.go + deployment_builder_test.go + claim_builder.go + install.sh + DESIGN.md · 1 新 file: devlog/phase-9-t004.md |
| **完整性** | plan §3 T004 acceptance 6 项逐项核对(go build clean / go test PASS / Pod template label propagation / kind smoke STILL passes assert.sh / DESIGN.md diagram / devlog)+ scope expansion claim_builder.go propagation tested | 4/6 直接 verifiable in dev tree(go build · go test · DESIGN.md · devlog)· 2/6(propagation observable + kind smoke assert.sh)CI runtime verify · scope expansion claim_builder.go 4 个测试 case pass · pre-existing annotations preserved test verified |
| **正确性** | `go build ./operators/inference-operator/...` clean exit 0 · `go vet ./operators/inference-operator/...` clean exit 0 · `go test ./internal/controller/...` PASS 全套(包括 P5/6/7/8 51+ pre-existing controller tests · P9 6 个新 sub-cases under TestSliceTemplateLabelPropagation) | 全部 PASS · 无 pre-existing test regression |

P4 横向 grep `SliceTemplateAnnotation` / `cmd_demo_bundle_path` 全仓库 → claim_controller.go 注释 line 108 仍提"deployment_builder (carry-forward Phase 8 polish task)"`carry-forward` 措辞 stale(应升 "P9-T-004 landed")· **但 npu-dra-driver/internal/controller/claim_controller.go 不在 T004 Allowed Paths · Phase 9 W2 polish 任务或自然在 Phase 10 framework migration 中一并更新**。

## Carry-forward

- **claim_controller.go comment refresh** (Phase 9 W2 polish 或 Phase 10):line 100-118 注释提"carry-forward Phase 8 polish task" 应升 "P9-T-004 landed" · 加 cross-ref 本 devlog · 不在 T004 Allowed Paths(npu-dra-driver/internal/controller/* 是 forbidden in T004)
- **kind smoke CI verify**:T004 push 后 CI 跑 phase-8 kind smoke assert.sh · `==T103-4 / T104==` 应从":warning:: install.sh stamps via kubectl annotate" 改为":  OK · ${claim} carries npu.huawei.com/slice-template=qwen-pd-idle" · preferred-hccs-ring hard-fail 应 PASS via bundle path 自然触发
- **T005 Quota CRD types**:Phase 9 W1 下一 task · 独立于 T004
- **T006 Quota controller + admission webhook**:Phase 9 W1 后续 · 独立于 T004
- **T007 PromQL custom metric extension**:Phase 9 W1 后续 · 独立于 T004
- **T008 O2 DMS scaffold**:Phase 9 W1 后续 · 独立于 T004

---

**END of P9-T-004 devlog**
