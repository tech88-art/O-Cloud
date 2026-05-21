# P7-T-006 · NPUSliceTemplate CRD types + scheme + round-trip tests + 2 samples

- **Commit**: (this commit)
- **Date**: 2026-05-20
- **Duration**: plan 1d / actual ~1d (main agent direct · types + tests + samples + chart bundle + DESIGN.md)

## Intent

Per ADR-0011 §1 §4 + phase7-plan.md §3 P7-T-006:落地 `NPUSliceTemplate` CRD 类型(api/v1alpha1/npuslicetemplate_types.go)+ 4 round-trip tests + Qwen / DeepSeek 2 个 sample YAMLs + chart CRD 包裹 + DESIGN.md cross-ref。CRD 用于表达"用户期望的复合切分"(如 1× vir04 prefill + 1× vir08 decode),Phase 7 T007 template engine + T105 allocator 消费。

## Path adaptations

- 计划 §3-T006 Allowed Paths 全部 covered:
  - `api/v1alpha1/npuslicetemplate_types.go` ✅
  - `api/v1alpha1/npuslicetemplate_types_test.go` (4 cases) ✅
  - `api/v1alpha1/zz_generated.deepcopy.go` (regen via `controller-gen object`) ✅
  - `config/crd/bases/npu.ocloud.edge.example.com_npuslicetemplates.yaml` (regen via `controller-gen crd`) ✅
  - `config/samples/npuslicetemplate_qwen_pd.yaml` (new dir + file) ✅
  - `config/samples/npuslicetemplate_deepseek_20b.yaml` ✅
  - `DESIGN.md` §3.8 NPUSliceTemplate CRD ✅
  - `deploy/helm-charts/npu-dra-driver/crds/npuslicetemplates.yaml` (copy of generated CRD per Phase 5 chart-bundles-CRD pattern) ✅
- ADR-0011 §4 Go types template 全部落地 + 增 ConditionType const (Validated + Allocatable) 供 P7-T-007 controller body 用。
- 第一次跑 `controller-gen object paths=./...` 不行(Windows + paths=./ wildcard 问题)。用 `controller-gen object paths=./api/v1alpha1/` 显式路径(同 P7-T-003 经验)成功。

## Debugging trail

- 无 false start。types + tests 单次写成 · 单次 PASS。
- controller-gen v0.21.0 path issue 已知 · 显式 `paths=./api/v1alpha1/` 一次到位(P7-T-003 devlog 也提到了此 quirk)。
- 唯一犹豫:`PartTypeDynamicShard` 加不加。ADR-0011 §4 列了 5 个 PartType enum 含 dynamic-shard,但 dynamic-shard 在 Phase 7 fallback path 无法 decompose(没有对应固定模板)。决定:**加进 enum 但 Phase 7 template engine validate 时 reject** · 让 enum 反映 "ADR-0011 §4 schema 长期愿景" 而不是仅 "Phase 7 W1 可处理"。Reject 在 P7-T-007 template engine 落地。Validated condition reason="DynamicShardNotSupported" 让 operator 看出原因。

## Key decisions

- **Cluster-scoped** per ADR-0011 §3:Phase 7 W1 全集群单一 template registry · Phase 9 multi-tenancy 改 namespaced(ADR-0011 §推翻条件 row 4 已 reserve)
- **`MinItems=1` on Composition + `Minimum=1` on Count**:防止空 composition / 0-count part 落库 · kubebuilder validation marker 在 admission 阶段就 reject(无需 controller body 检查)
- **`ConditionType` const 放在 types.go**(不放 controller_test):let P7-T-007 controller body 引用同一常量 · 避免字符串拼写漂移
- **`omitempty` on Status fields**:`TestEmptyStatusOmitted` 锁定 · `kubectl get -o yaml` readability + status-subresource semantics(`kubectl apply -f sample.yaml` 时 Status 不被发布 · 仅 controller 后续 write Subresource API)
- **`shortName=npust`** (类似 npua for NPUSliceAllocation):let `kubectl get npust` work · 减少 typing
- **Chart CRD bundle**:Phase 5 已 ship npusliceallocations.yaml at `deploy/helm-charts/npu-dra-driver/crds/`;Phase 7 沿用相同 convention · CRD apply before helm install templates render
- **Sample YAML 用 PD-pair vs strict-mode 2 个 demo**:符合 ADR-0011 §4 sample compositions 表 · qwen-8b-pd-pair 体现 fallback strategy (decomposed into 1×vir04 + 1×vir08) · deepseek-20b-whole 体现 refuse mode

## Verification

- 存在性:
  - `api/v1alpha1/npuslicetemplate_types.go` — NPUSliceTemplate + Spec + Status + TemplatePart + 4 enum types + 2 ConditionType const + init() Register ✅
  - `api/v1alpha1/npuslicetemplate_types_test.go` — 4 test functions ✅
  - `api/v1alpha1/zz_generated.deepcopy.go` (regen) — added DeepCopy methods for new types ✅
  - `config/crd/bases/npu.ocloud.edge.example.com_npuslicetemplates.yaml` (regen) — 含 spec.composition + spec.fallbackStrategy + status.conditions + status.fallbackAppliedReason + status.observedGeneration + 3 printer columns(Parts + Fallback + Age)✅
  - `config/samples/npuslicetemplate_qwen_pd.yaml` ✅
  - `config/samples/npuslicetemplate_deepseek_20b.yaml` ✅
  - `deploy/helm-charts/npu-dra-driver/crds/npuslicetemplates.yaml` ✅
  - `DESIGN.md` §3.8 — 详细介绍 Go types + enum const + 验证规则 + 样例 + 测试 gate ✅
- 完整性(verified by execution):
  - `controller-gen object paths=./api/v1alpha1/` regen clean ✅
  - `controller-gen crd paths=./api/v1alpha1/ output:crd:artifacts:config=config/crd/bases` regen clean ✅
  - `go build ./...` clean ✅
  - `go test ./api/v1alpha1/... -v` — 4 new TestXxx 全 PASS + 既有 NPUSliceAllocation + TestRoundTrip / TestSchemeRegistration 全 PASS ✅
  - `helm lint --strict deploy/helm-charts/npu-dra-driver/` clean(1 INFO 不是 fail)✅
  - `helm template` 渲染 10 K8s 对象(chart templates · CRDs apply 独立)✅
  - python yaml.safe_load 两个 sample 都 OK · spec keys = [composition, fallbackStrategy] ✅
- 正确性:
  - `TestEnumValuesArePinned` 锁定 5 PartType + 2 FallbackStrategy + 2 ConditionType 字符串值 = ADR-0011 §4 schema 一致 ✅
  - `TestEmptyStatusOmitted` 锁定 omitempty 行为 ✅
  - `TestDeepCopyPreservesComposition` 锁定 detached deep copy ✅
  - CRD YAML 含 `+kubebuilder:default=fixed-template-combination` 渲染为默认值 ✅

## Carry-forward

- **T007** (template engine + composition decomposition) — 消费 NPUSliceTemplate types:Engine.Validate (基于 spec / kubebuilder validation 之后的 logic) + Engine.Decompose(基于 composition 输出 FixedTemplateBundle)+ template_controller stamping Validated + Allocatable conditions + FallbackAppliedReason
- **T105** (allocator dynamic-slice extension) — 读 Pod label `npu.huawei.com/slice-template=<name>` + lookup NPUSliceTemplate + invoke Engine.Decompose + pass bundle to Allocator + all-or-nothing semantics
- **T103** (kind smoke ext) — fixture 用 `npuslicetemplate_qwen_pd.yaml` 样例 · assert `kubectl get npuslicetemplate qwen-8b-pd-pair -o jsonpath` 后 Validated=True + Allocatable=True
- **Phase 8 vertical scaling** — 读 `NPUSliceTemplate.status` 判断"重启切片"触发(arch §13 Phase 8 row + ADR-0011 §後果)· 给 Phase 8 busy-idle controller 一个 substrate hook
- **Phase 9 multi-tenancy** — namespaced scoping reconsider · 加 TenantRef field on Spec · ADR-0011 §推翻条件 row 4 已 reserve
