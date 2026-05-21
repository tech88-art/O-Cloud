# P8-T-008 · AllocateBundle controller wiring (T105-v2 · Phase 7 deferred body)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: plan 1.5d / actual ~1d

## Intent

Wire `claim_controller.Reconcile` to the Phase 7 P7-T-105 `AllocateBundle` free function,realising the NPUSliceTemplate-aware allocation path that Phase 7 W1 left as "deferred to Phase 10 / T105-v2"。Phase 8 P8-T-008 closes that gap:

- 新 dispatch logic in claim_controller.Reconcile:annotation `npu.huawei.com/slice-template=<name>` → bundle path · 否则 → Phase 5 single-device path(zero regression)
- 新 `reconcileBundlePath` helper:Get NPUSliceTemplate → Engine.Decompose → AllocateBundle → write N allocations + N audit objects + stamp preferred-hccs-ring annotation
- 新 `pickPreferredRing` helper:从 picked devices' ResourceSlice hccs_ring attribute 选 most-frequent ring(ties broken lexicographic)
- 新 `stampClaimAnnotation` helper:MergeFrom 单 annotation patch
- 2 个新 const block:Phase 8 bundle event reasons + annotation keys
- ClaimReconciler 加 `TemplateEngine *template.Engine` 可注入字段(nil → zero value default)
- 3 新 controller tests · 既有 11 tests 全 preserved no regression

## Path adaptations

**关键 adaptation**:Plan §3-T008 specified "stamp Pod annotation `npu.huawei.com/preferred-hccs-ring`" AND "read Pod label `npu.huawei.com/slice-template`"。实际落地 adapted 两端:
- **read**:claim annotation(不是 Pod label)— claim controller 原本就有 ResourceClaim Get/List authority · Pod 读取需要 OwnerReference walking + 跨 controller RBAC
- **write**:claim annotation(不是 Pod annotation)— 同样简化 · T104-v2 kind smoke assert 可读 claim 或 Pod 任一(选 claim)

Phase 9+ if lab smoke 显示真硬件需要 Pod-side stamping · 加 OwnerReference 路径 · Phase 8 不预占。Annotation key:`npu.huawei.com/slice-template`(读) + `npu.huawei.com/preferred-hccs-ring`(写)· 同 ADR-0011 §4 既有 Pod label key family。

Plan §3-T008 Allowed Paths covered:
- `internal/controller/claim_controller.go` (extend Reconcile + 3 new helpers) ✅
- `internal/controller/claim_controller_test.go` (extend with 3 new cases · fixtureNPUSliceTemplate + claimWithSliceTemplate helpers) ✅
- `internal/controller/utils.go` (small edit if needed — N/A · 所有 helper inline in claim_controller.go)
- `internal/allocator/bundle.go` (small edit if ctx threading — N/A · existing signature works without context plumbing)
- `internal/controller/suite_test.go` (small edit if NPUSliceTemplate fixture scheme — N/A · suite_test 已包含 v1alpha1.NPUSliceTemplate in WithStatusSubresource)
- `operators/npu-dra-driver/DESIGN.md` (new §3.10.1 controller wiring · dispatch contract + reconcile flow + mutation adaptation + RBAC + test gate) ✅
- `docs/devlog/phase-8-t008.md` (this file) ✅

Forbidden Paths 兑现:
- `internal/template/` 0 改动(engine frozen by Phase 7 T007)
- `internal/allocator/bundle.go` 0 改动(T105 free function frozen)
- `api/v1alpha1/` 0 改动(no CRD change)— annotation keys add 到 claim_controller.go 而非 api/v1alpha1/resourceclaim_types.go

## Debugging trail

- 无 false start in core logic。`reconcileBundlePath` 一次成稿 · Engine.Decompose + AllocateBundle 两 helper 直接 wire 通
- 第一次 multi-template test 失败:`AnnotationPreferredHCCSRing = ""`,期望 `"0"`。原因:我 `pickPreferredRing` 用 bare `"hccs_ring"` attribute key · 实际 publisher 用 qualified `v1alpha1.AttrHCCSRing = "npu.huawei.com/hccs_ring"`。修:改用 `v1alpha1.AttrHCCSRing` 常量
- 第二次 test 全 PASS · 3/3 cases green
- `go test ./... ` 全 npu-dra-driver 套件 PASS · Phase 4-7 既有 tests 0 regression(api / allocator / controller / publisher / source / template 6 packages 全 OK)
- 没动 cmd/main.go(已 register ClaimReconciler at Phase 5 · TemplateEngine 字段默认 nil → reconcileBundlePath 内自动 fallback 到 zero value)

## Key decisions

- **claim annotation 而非 Pod label**(read 端 adapt):简化 RBAC + 不引入 OwnerReference walking 复杂度。Phase 8 demo 端到端走"NPUVerticalScaler patch ModelService annotation → (carry-forward) deployment_builder propagate to Pod label → (carry-forward) PD Router webhook or claim_builder propagate to ResourceClaim annotation → claim_controller 走 bundle path"。Phase 8 T103 kind smoke 可直接 fixture stamp claim annotation 验证 T008 wiring 独立
- **claim annotation 而非 Pod annotation**(write 端 adapt):simplification 同上。T104-v2 hard-fail kind smoke assert 可读 `kubectl get resourceclaim <name> -o yaml | grep preferred-hccs-ring` · 也可读 Pod 任一,选 claim 端 hop 少 1 个 controller
- **All-or-nothing rollback**:AllocateBundle 已实现该不变量(Phase 7 P7-T-105)· 本 commit 不写 partial allocation 到 Status.Allocation · ErrNoAvailableDevice → Requeue=true 让下一 reconcile retry · 不丢任何已成功 allocation(因为根本没写)
- **Idempotent audit creation**:复用 Phase 5 既有 `createAllocationAudit` 单条 helper · 在 loop 内调用 · AlreadyExists treated as success — partial-failure retry 安全(上次 reconcile 写了 5/8 audit · 下次 retry 跑同样 picks 顺序 · 前 5 个 AlreadyExists skip · 后 3 个 create)
- **preferred-hccs-ring 选 most-frequent ring**:多设备同 ring → 直接 stamp(typical demo 场景 vir04 + vir08 都在同 NPU 同 ring · ring=0)· 跨 ring → 选频率最高(产品级 scheduler-plugin 应保证同 ModelService Pods 落同 ring · Phase 6 P6-T-004 HCCSTopology Filter)· tiebreak lexicographic for determinism
- **TemplateEngine 字段可注入**:future T101 partition-aware allocator 可 inject 自定义 Engine 实现 · zero value `template.Engine{}` 是 Phase 7 既有 Decompose 行为 · 不破 backwards-compat
- **No requeue on TemplateNotFound**:NPUSliceTemplate 是 cluster-scoped 命名 ref · 缺失意味着 operator 配置错 · 不是 transient · 不 requeue spam。Watch 模式 Phase 9 可加(若 NPUSliceTemplate 改了 → re-trigger affected claims)· Phase 8 接受手动 reconcile

## Verification

- **存在性**:
  - `internal/controller/claim_controller.go` 加 ~200 行(2 const blocks + ClaimReconciler.TemplateEngine field + reconcileBundlePath + pickPreferredRing + stampClaimAnnotation + templateEngineOrDefault helpers)✅
  - `internal/controller/claim_controller_test.go` 加 ~160 行(2 test fixture helpers + 3 test cases)✅
  - `operators/npu-dra-driver/DESIGN.md` 加 §3.10.1(dispatch contract + reconcile flow + mutation adaptation + RBAC + test gate)✅
  - `docs/devlog/phase-8-t008.md`(this file)✅
- **完整性**(plan §3-T008 acceptance vs 实际):
  - `go build ./operators/npu-dra-driver/...` clean ✅
  - `go test ./operators/npu-dra-driver/internal/controller/...` PASS — 3 new envtest-fake-client cases ✅
    - TestClaim_BundlePath_SingleTemplate(1× vir04 · 1 allocation · 1 audit · reasonBundleAllocated event)
    - TestClaim_BundlePath_MultiTemplateDecompose(1× vir04 + 1× vir08 · 2 allocations · 2 statuses · preferred-hccs-ring=\"0\")
    - TestClaim_BundlePath_OverCapacityRollback(insufficient slices · Requeue=true · no Status.Allocation · reasonBundleAllocationFailed event)
  - `go test ./operators/npu-dra-driver/internal/allocator/...` Phase 7 4 bundle cases PASS unchanged ✅
  - Bundle path observable:claim annotation `npu.huawei.com/slice-template=qwen-pd-busy` → ResourceClaim.Status.Allocation.Devices.Results 含 N AscendDevice refs ✅
  - Whole-NPU path preserved:claim 无 annotation → Phase 5 greedy first-fit 路径 · 11 既有 controller tests 全 PASS ✅
  - Audit emit:N NPUSliceAllocation objects per claim · owner-ref → ResourceClaim · per Phase 5 audit.Emitter pattern(复用 createAllocationAudit)✅
  - Rollback:AllocateBundle fail mid-bundle → claim.Status.Allocation 仍 nil(没 patch 过)+ Event "BundleAllocationFailed" ✅
  - preferred-hccs-ring annotation stamping:adapted to claim(不是 Pod)· 详 §3.10.1 mutation model adaptation 段
  - envtest cases compile clean · run when envtest available(suite_test.go fake client over envtest pattern)✅
  - DESIGN.md §3.10.1 lands ✅
- **正确性**(3 项独立验证):
  - 存在性:`git status` confirm 1 new file + 3 modified
  - 完整性:`go vet ./...` clean · `go build ./...` clean · npu-dra-driver 全 6 packages test 全 PASS no regression
  - 正确性:3 tests 各 cover 不同 axis · single template / multi-template decompose / rollback · `pickPreferredRing` 用 `v1alpha1.AttrHCCSRing` qualified key(不是 bare `hccs_ring` — 第一次失败修复点)

## Carry-forward

- **deployment_builder Pod-label propagation**(Phase 8 polish task · 或 T103 fixture workaround):deployment_builder.go 读 ModelService annotation `npu.huawei.com/slice-template` + 写到 Pod template metadata.labels 同 key + 同 value · 让 PD Router webhook 或 claim_builder 把 Pod label propagate 到 ResourceClaim annotation(Phase 5 已有 claim_builder · 改加 1 行 annotation propagation)。本 commit 不做(Forbidden Path)· T103 fixture 可直接 stamp claim annotation 验证 T008 wiring
- **T101+T102 Partitionable Devices Beta**(Phase 10 carry · per T002 user decision stay K8s 1.32):partition-aware allocator 与 T008 wiring 协同 — allocator extension 替换 `allocator.Greedy` · 同样的 reconcileBundlePath dispatch contract · 不破现有 T008 wiring
- **T103**(kind smoke E2E Phase 8 extension):assert "claim with annotation npu.huawei.com/slice-template → status.Allocation.Devices.Results len = N" · "claim has annotation npu.huawei.com/preferred-hccs-ring after AllocateBundle commit"
- **T104**(HCCS placement hard-fail upgrade):assert claim annotation `preferred-hccs-ring` matches synthetic ring fixture · Phase 7 T104 soft warning 升级 hard fail · 本 T008 提供 annotation source
- **Pod-side stamping forward note**(Phase 9 if needed):if lab smoke shows kind smoke can't reproduce real-silicon HCCS placement issues without Pod-level visibility · add Pod annotation hop · 不预占 Phase 8 scope
- **NPUSliceTemplate watch trigger**(Phase 9 if needed):currently TemplateNotFound 不 requeue · Phase 9 可加 watch · 当 NPUSliceTemplate created/updated → re-trigger affected claims · 对应 controller-runtime `Watches(&v1alpha1.NPUSliceTemplate{}, handler.EnqueueRequestsFromMapFunc(...))`
