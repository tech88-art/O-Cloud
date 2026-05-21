# P8-T-005 · NPUVerticalScaler CRD types + scheme + sample

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: plan 0.5d / actual ~0.5d

## Intent

按 ADR-0012 §4 Go types 区块逐字落地 NPUVerticalScaler CRD:
- types(`api/v1alpha1/npuverticalscaler_types.go` · 新)
- scheme 注册(自带 init() + SchemeBuilder.Register)
- DeepCopy 自动生成(controller-gen object · zz_generated.deepcopy.go regen)
- 4 round-trip tests(round-trip JSON · DeepCopy detach · empty status omitted · enum/constants pin · 完全 mirror Phase 7 NPUSliceTemplate test pattern)
- sample yaml(`config/samples/inference_v1alpha1_npuverticalscaler.yaml` · 新)
- chart 内 bundle CRD(`deploy/helm-charts/inference-operator/crds/npuverticalscalers.yaml` · 新 · 从 controller-gen 输出复制)
- PROJECT 入新 resource 行(kubebuilder convention)

ADR-0012 §4 spec/status schema **完全 verbatim** 落地 — TargetRef + MetricSpec + ScaleSliceSpec + ObservedTargetStatus + ScaleEvent + 4 个 constants(ConditionActive / ScalingInProgress / CooldownActive · AnnotationManagedBy · FinalizerName)+ 3 个 default 常量(DefaultCooldownSeconds=600 · DefaultMetricWindowSeconds=300 · MaxScaleHistoryEntries=10)。

## Path adaptations

- 计划 §3-T005 Allowed Paths 全部 covered:
  - `api/v1alpha1/npuverticalscaler_types.go` (new) ✅
  - `api/v1alpha1/groupversion_info.go` (small edit — N/A · NPUVerticalScaler 自带 init() 在 npuverticalscaler_types.go 末尾 SchemeBuilder.Register;groupversion_info.go 不需改) ✅ adaptation
  - `api/v1alpha1/zz_generated.deepcopy.go` (regenerated via controller-gen v0.20.1) ✅
  - `api/v1alpha1/npuverticalscaler_types_test.go` (new · 4 cases) ✅
  - `config/crd/bases/inference.ocloud.edge.example.com_npuverticalscalers.yaml` (generated) ✅
  - `config/samples/inference_v1alpha1_npuverticalscaler.yaml` (new · 1 sample) ✅
  - `deploy/helm-charts/inference-operator/crds/npuverticalscalers.yaml` (chart bundle · 从 controller-gen 输出 cp) ✅
  - `operators/inference-operator/PROJECT` (kubebuilder 加 NPUVerticalScaler resource block) ✅
  - `docs/devlog/phase-8-t005.md` ✅
- 不动 internal/controller(T007)+ internal/metrics(T006)— Forbidden Paths 兑现
- 不动 modelservices CRD YAML(controller-gen v0.20.1 ≠ 历史 regen v0.21.0 · annotation downgrade revert 避免不相关 cosmetic diff)
- go.mod 由 `go mod tidy` 自动把 `github.com/go-logr/logr v1.4.3` 从 indirect 提升到 direct require — 实为 pre-existing 状态修正(`internal/webhook/pd_router.go` 直接 import logr · 历史 go mod tidy 没跑彻底);本 task 保留该提升 · 不属于 T005 引入

## Debugging trail

- 无 false start。`controller-gen object paths=./api/v1alpha1/...` 一次成功 generate DeepCopy。
- 第一次 build 失败 cleanly:`*NPUVerticalScaler does not implement runtime.Object (missing method DeepCopyObject)` — 预期,等 controller-gen 运行后修
- controller-gen 安装:`go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.20.1`(Makefile pinned 版本)· `~/go/bin/controller-gen.exe` ready
- controller-gen `paths=./...` 报"no Go files in repo root" · 改 explicit `paths=./api/v1alpha1/... paths=./internal/controller/... paths=./internal/webhook/...` 通过 — 由于不像 Makefile 走 `cd` + glob expansion · 必须每个 sub-tree explicit list
- 第一次 manifests 输出 regen 了 modelservices CRD YAML(controller-gen annotation 从 v0.21.0 → v0.20.1)· 这是 pre-existing 历史 inconsistency(有人之前用 v0.21.0 regen 过)· 不属本 task scope · revert 该 annotation 改回 v0.21.0
- `go mod tidy` 自动 promote logr direct · 保留(pre-existing 状态修正)
- 测试 4 cases 一次性 PASS · 既有 inference-operator 45+ tests 同样 PASS(api / controller / metrics / webhook 4 packages 全 OK · no regression)

## Key decisions

- **NPUVerticalScaler 自带 init() in npuverticalscaler_types.go**(不写到 groupversion_info.go)— 遵循 ModelService 既有 pattern(modelservice_types.go 末尾自带 init() 调 SchemeBuilder.Register)· kubebuilder convention 是"每 CRD types 文件自带 init"。groupversion_info.go 只 own SchemeBuilder declaration
- **3 个 constants(DefaultCooldownSeconds / DefaultMetricWindowSeconds / MaxScaleHistoryEntries)在 types 文件**:T007 controller body 引用同一常量保证一致性 · 避免 magic number hardcoded 散乱在 controller 各 helper
- **4 round-trip tests mirror Phase 7 NPUSliceTemplate test pattern**:test name 用 `TestNPUVerticalScalerXxx` prefix · 避免与 Phase 7 测试同名冲突;`strings.Contains` 替换 Phase 7 `contains()` 自实现(更标准 + 不增本地辅助函数)
- **ScaleHistory `+kubebuilder:validation:MaxItems=10`**:CRD schema 层强制 max 10 entries · controller 端 FIFO eviction 不能依赖 schema 校验(因为 status updates patch · validation 不一定 round-trip)· 仍 ship marker 提示 operators API 设计意图
- **Conditions `+listType=map +listMapKey=type`**:K8s standard pattern for conditions arrays · 保证 update 时按 type 字段做 patch · 而不是整数组 replace
- **`config/samples/inference_v1alpha1_npuverticalscaler.yaml` sample 用 cooldownSeconds=300**(非 schema default 600):demo录屏 friendly · 让 reviewer 在 10-15min 录屏窗口内看到 ≥ 1 次 scaling event · production guidance ≥ 300s 防 flapping 在 sample comments 内明示

## Verification

- **存在性**:
  - `api/v1alpha1/npuverticalscaler_types.go` 248 行(含 11 个 type definitions + 2 enum constants + 5 const block + 1 init 函数)✅
  - `api/v1alpha1/npuverticalscaler_types_test.go` 4 test functions · 211 行 ✅
  - `api/v1alpha1/zz_generated.deepcopy.go` regen · 193 行 insertion(11 DeepCopy + DeepCopyInto methods for 11 types) ✅
  - `config/crd/bases/inference.ocloud.edge.example.com_npuverticalscalers.yaml` generated ✅
  - `config/samples/inference_v1alpha1_npuverticalscaler.yaml` new sample ✅
  - `deploy/helm-charts/inference-operator/crds/npuverticalscalers.yaml` chart bundle ✅
  - `PROJECT` NPUVerticalScaler resource block 已加 ✅
  - `docs/devlog/phase-8-t005.md` (this file) ✅
- **完整性 vs plan §3-T005 acceptance**:
  - Types compile: `go build ./api/...` clean ✅
  - DeepCopy regenerates clean: zz_generated.deepcopy.go regen idempotent ✅
  - Round-trip tests pass: `go test ./api/v1alpha1/... -run TestNPUVerticalScaler` **4 cases PASS** ✅
  - `make manifests` clean: `controller-gen rbac:roleName=manager-role crd webhook paths=./api/v1alpha1/... paths=./internal/controller/... paths=./internal/webhook/... output:crd:artifacts:config=config/crd/bases` 一次成功 ✅
  - Sample applies clean: schema-conformant per CRD output ✅
  - Phase 5/6/7 inference-operator tests preserved: `go test ./api/... ./internal/controller/... ./internal/metrics/... ./internal/webhook/...` 4 packages 全 OK · **no regression on existing 45+ tests** ✅
  - CRD schema fields per ADR-0012 §4: 实际生成 CRD scope=Namespaced + shortName=npuvs + 4 printer columns(Target / CurrentTemplate / LastScaleTime / Status)+ subresource:status + enum NPUUtilization + maximums 100 + minimum 30 / 0 + defaults 600/300/300 — 全 match ✅
- **正确性**(3 项独立验证):
  - 存在性:6 个新文件 + 3 个 modified 文件 · `git status` confirm ✅
  - 完整性:`go vet ./...` clean(no new lint warnings)· `go mod tidy` idempotent(再跑无变化) ✅
  - 正确性:4 test cases test 不同 axes(JSON marshal + DeepCopy detach + omitempty + enum/constants pin)· 各 axis 独立 verify · 不只机械 round-trip ✅

## Carry-forward

- **T006**(Busy-idle metrics ingestor)入手即可消费 T005 的 `MetricTypeNPUUtilization` enum + `DefaultMetricWindowSeconds` 常量;Ingestor 接口签名按 ADR-0012 §1 reconcile step 3 IngestorResult contract(`{Value: float64, NoData: bool, Err: error}`)落地
- **T007**(NPUVerticalScaler controller body)消费 T005 全 schema · 8 步 reconcile loop + cooldown gate + ScaleHistory rolling window(MaxScaleHistoryEntries=10 from T005 const)+ Mutation model(patch ModelService.spec.template.sliceTemplate + stamp AnnotationManagedBy const)
- **T008**(AllocateBundle controller wiring)与 T005 解耦 · 在 npu-dra-driver 模块 · 不读 NPUVerticalScaler
- **T103**(kind smoke E2E Phase 8 extension)需要的 `npuverticalscalers.inference.ocloud.edge.example.com` CRD `helm install inference-operator` 时 already deployed(chart bundle 内已有)· `kubectl apply -f config/samples/inference_v1alpha1_npuverticalscaler.yaml` 演示
- **CI 注意**:本 commit 触发 inference-operator unit test + helm lint workflow 跑 · `go vet` + `helm lint --strict deploy/helm-charts/inference-operator/` 应 clean(本地 dry-run 验证已 PASS)
