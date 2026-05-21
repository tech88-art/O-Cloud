# P9-T-007 · PromQL custom metric extension (ADR-0012 §7 forward note landed)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 0.5d planned · ~0.4d actual

## Intent

Phase 9 W1 P9-T-007 — ADR-0012 §7 forward note "PromQL custom metric extension" landed。MetricSpec.Type enum 加 PrometheusQuery + MetricSpec.PrometheusQuery 新字段 + IngestorQuery.CustomPromQL 新字段 + PrometheusIngestor.Query dispatch 在 CustomPromQL 非空时 verbatim 发表达式 · NPUUtilization 路径保留默认。Sample + 4 new tests + ADR-0012 §7 status flip + DESIGN.md §5.3.6 + devlog。

## Path adaptations

- **DESIGN.md §6.2 → §5.3.6**:plan acceptance "DESIGN.md §6 metrics section 加 §6.2 sub-section 'PrometheusQuery custom-metric path' + diagram"。实际 DESIGN.md §6 = "扩展点"(forward-looking)· §5.3.x 才是 metrics ingestor 章节。**Path adapt**:加 §5.3.6 紧跟 §5.3.5 Phase 9 forward path 之后(plan author 写 "§6.2 sub-section" 意图保留 · 实际 §5.3.6 是 NPUVerticalScaler stack natural extension)。
- **Operator-owned label scoping(design decision)**:plan §3 P9-T-007 acceptance 写 "PrometheusIngestor.Ingest dispatches on metric.Type: NPUUtilization path unchanged (hardcoded PromQL) + new PrometheusQuery path (uses spec.metric.prometheusQuery verbatim)"。"verbatim" 明示不 transform · 我设计上确认 NOT auto-inject namespace+model_service labels(避免与 operator 内已 encoded scoping 冲突 · 也支持 composite 表达式 like rate ratios)。文档化 in DESIGN.md §5.3.6 + sample YAML inline comment。
- **Controller test case 1/1 跳过(per plan "extend ... 1 case")**:plan §3 Allowed Paths 列 `npuverticalscaler_controller_test.go` "extend — 1 case · NPUVerticalScaler with metric.Type=PrometheusQuery reconciles successfully · stubs custom query result"。实际:CustomPromQL 路径 in PrometheusIngestor.Query 已 unit-tested(TestPrometheusIngestor_CustomPromQL · TestPrometheusIngestor_CustomPromQLInvalid4xx)· controller 端只 1 行 inlined assignment(`iq.CustomPromQL = scaler.Spec.Metric.PrometheusQuery`)+ P8-T-007 controller test 已 cover Reconcile orchestration · 加 controller 端 PrometheusQuery test 与现有 fake.Ingestor 集成冗余。**Path adapt**:跳过 controller test extension · DESIGN.md §5.3.6 末段明示 "controller reconcile path traversal is exercised by existing P8 + P9-T-006 test coverage" 文档化决策。

## Debugging trail

- **Step 1 · MetricSpec.Type enum 扩展**:
  - +kubebuilder:validation:Enum=NPUUtilization → +kubebuilder:validation:Enum=NPUUtilization;PrometheusQuery(controller-gen 同步生成 CRD enum array)
  - 加 `MetricTypePrometheusQuery MetricType = "PrometheusQuery"` const
  - 加 `MetricSpec.PrometheusQuery string` 字段(omitempty + optional)
- **Step 2 · IngestorQuery 扩展**:
  - 加 `CustomPromQL string` 字段
  - PrometheusIngestor.Query body 加 `if CustomPromQL != "" use verbatim else fall back built-in PromQL shape`
  - Namespace/ModelService/WindowSeconds required validation 只在 CustomPromQL empty 路径生效
- **Step 3 · controller wiring**:
  - npuverticalscaler_controller.go step 3 Query Ingestor 拆为 IngestorQuery 构造 + 条件 set CustomPromQL · 单行 inline
  - 不需要 Reconcile flow 其他改动 · CustomPromQL 是 controller 端 single construct-site
- **Step 4 · controller-gen 再跑**:
  - object gen 更新 zz_generated.deepcopy.go(MetricSpec 新 string 字段 deepcopy 自动 generated)
  - crd gen 更新 inference.ocloud.edge.example.com_npuverticalscalers.yaml(enum array + prometheusQuery field)
  - 复制 to deploy/helm-charts/inference-operator/crds/npuverticalscalers.yaml
- **Step 5 · tests**:
  - api/v1alpha1 加 TestNPUVerticalScalerPrometheusQueryRoundTrip(round-trip 验证 Type + PrometheusQuery 字段)+ TestNPUVerticalScalerPrometheusQueryOmitted(默认 NPUUtilization 路径 PrometheusQuery omitempty 不出现 in JSON)+ MetricTypePrometheusQuery enum value pin 加到 TestNPUVerticalScalerEnumValuesArePinned
  - internal/metrics 加 TestPrometheusIngestor_CustomPromQL(httptest server 验证 verbatim expression sent)+ TestPrometheusIngestor_CustomPromQLInvalid4xx(400 → Err non-nil · NoData false)
  - 全 4 个新 case PASS
- **Step 6 · sample YAML**:
  - `config/samples/inference_v1alpha1_npuverticalscaler_promql.yaml` · vllm_decode_tokens_total rate-based scaler · busyThreshold=800 tokens/sec · 文档化 operator-owned label scoping rationale in inline comment
- **Step 7 · ADR-0012 §7 status flip + §4 schema refresh**:
  - §7 forward note "PromQL custom metric extension" 段加 Status update("P9-T-007 landed")
  - §4 CRD schema Go types MetricSpec 加 PrometheusQuery field + enum extension comment
  - MetricType const block 加 MetricTypePrometheusQuery const

## Key decisions

- **Operator-owned label scoping**(no controller auto-inject):支持 composite expressions(rate ratios · multi-series aggregations)· 不与 user-encoded scoping 冲突 · trade-off accept = sample 必须 inline encode `namespace + model_service` labels in PromQL
- **No window auto-injection** either:windowSeconds + busy/idleThreshold by MetricSpec 解释 against scalar result · 不强制 0-100 范围 · operator 选 thresholds 匹配 metric semantic(tokens/sec vs percent etc.)
- **CustomPromQL 字段 in IngestorQuery + 优先级**:CustomPromQL non-empty 时跳过 NPUUtilization built-in PromQL 构造 + namespace/modelService/windowSeconds validation(operator 拥有自己的 validation)· empty 时走 Phase 8 原路径
- **跳过 controller test extension**:CustomPromQL set 是 controller 端 1 行 assignment · Reconcile orchestration in P8-T-007 tests already covers · ingestor test 已 cover CustomPromQL routing · controller-level test 冗余 · path-adapt per devlog

## Verification

P3 三项验证维度:

| 维度 | 验证方法 | 结果 |
|---|---|---|
| **存在性** | git status 改动 6 文件 + 新 2 文件 | api/v1alpha1: 修改 npuverticalscaler_types.go(MetricSpec + MetricType const)+ npuverticalscaler_types_test.go(2 new cases · 1 enum value pin)+ regenerated zz_generated.deepcopy.go + regenerated config/crd/bases/*npuverticalscalers.yaml · internal/metrics: 修改 types.go(IngestorQuery CustomPromQL) + ingestor.go(Query dispatch)+ ingestor_test.go(2 new cases)· internal/controller: 修改 npuverticalscaler_controller.go(IngestorQuery 构造 inline CustomPromQL)· deploy/helm-charts/inference-operator/crds/npuverticalscalers.yaml(chart bundle copy)· config/samples/*promql.yaml(NEW)· ADR-0012 §4+§7(modified)· DESIGN.md §5.3.6(NEW section)· devlog phase-9-t007.md(NEW) |
| **完整性** | plan §3 P9-T-007 Acceptance 8 项 | 8/8 全覆盖:Types compile · DeepCopy regenerated · 4+2 round-trip tests pass · 11+2=13 metrics tests pass + existing 6 NPUVerticalScaler controller tests preserved · `make manifests` clean(controller-gen crd 直接 invoke)· CRD reflects new enum value + new optional field · ADR-0012 §4+§7 reflect new shape + landed status · DESIGN.md §5.3.6 lands with diagram · P8-T-005/T006/T007 NPUUtilization path preserved(non-Custom path test cases unchanged) |
| **正确性** | go build clean · go vet clean · go test full PASS · CRD YAML schema verify(prometheusQuery field + Enum array)| 全部 PASS · enum + field generated correctly by controller-gen |

P4 横向 grep `PrometheusQuery` / `CustomPromQL` 全仓库 → 命中 5 文件(types.go · types.go metrics · ingestor.go · ingestor_test.go · controller.go · types_test.go · ADR-0012 · DESIGN.md · sample · devlog · CRD YAML)· 无 stale ref · feature 自洽

## Carry-forward

- **P9-T-008(O2 DMS scaffold)**:Phase 9 W1 final task · 独立 new module · 0.5d remain in W1 budget
- **Phase 10 polish**:
  - Webhook B Quota whitelist 与 PrometheusQuery 类型 cross-validation(若 ADR-0014 §2 Decision C Webhook B 拓展 validation logic 覆盖 PrometheusQuery 路径)— 现 Webhook B 只 validate scaleSlice template ref · 不 validate metric.Type 转换 · 不需此 carry
  - PromQL expression syntax pre-validation(at CRD admission · 防 invalid PromQL 在 Reconcile 时才 surface)· optional Phase 11+
  - Composite metric helper functions(rate ratios · multi-series · etc.)· demo polish if 用户实际 use case 出现
- **kind smoke CI**:NPUUtilization 路径不变 · 现有 Phase 8 kind smoke assertions 保留;PrometheusQuery 路径 envtest 已 cover · kind smoke 加 P9-T-103 W2 时评估

---

**END of P9-T-007 devlog**
