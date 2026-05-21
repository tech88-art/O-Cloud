# P8-T-006 · Busy-idle metrics ingestor

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: plan 1d / actual ~0.5d

## Intent

Ship the Phase 8 NPUVerticalScaler controller's metrics dependency:
- `Ingestor` interface(controller-facing contract)
- `PrometheusIngestor`(production impl · HTTP /api/v1/query with PromQL `avg_over_time(ascend_npu_utilization_percent{namespace,model_service}[$window])`)
- `FakeIngestor`(testing impl · canned result slice + call assertion helpers)
- 7 test cases(5 production + 2 FakeIngestor/helper bonus per acceptance · all PASS)
- cmd/main.go wiring(reads `NPUVERTICAL_SCALER_PROMETHEUS_URL` + `NPUVERTICAL_SCALER_QUERY_TIMEOUT` env)
- Helm chart values + Deployment env injection
- DESIGN.md §5.3(5 subsections · interface / PromQL / NoData semantics / chart wiring / Phase 9 forward)

ADR-0012 §1 reconcile step 3 IngestorResult contract:`{Value, NoData, Err}` — connection failures + 5xx + empty vector + empty URL **fold into** `NoData=true`(transient · stay current template + requeue);4xx + parse failures + `status != "success"` **fold into** `Err`(genuine bug surfaced)。

## Path adaptations

- Plan §3-T006 Allowed Paths 全部 covered:
  - `internal/metrics/ingestor.go` (new · PrometheusIngestor + HTTP /api/v1/query handling) ✅
  - `internal/metrics/ingestor_test.go` (new · 7 cases) ✅
  - `internal/metrics/fake_ingestor.go` (new · canned-result Ingestor impl + assertion helpers) ✅
  - `internal/metrics/types.go` (new · Ingestor + IngestorOpts + IngestorQuery + IngestorResult + MetricSample + WindowedAverage) ✅
  - `cmd/main.go` (small edit · env-driven PrometheusIngestor construction · T007 will register reconciler with this ingestor) ✅
  - `deploy/helm-charts/inference-operator/values.yaml` (small edit · `metrics.prometheusURL` + `queryTimeout` + `scrapeIntervalSeconds` defaults) ✅
  - `deploy/helm-charts/inference-operator/templates/deployment.yaml` (small edit · env-var injection from chart values) ✅
  - `operators/inference-operator/DESIGN.md` (new §5.3 with 5 subsections) ✅
  - `docs/devlog/phase-8-t006.md` (this file) ✅
- 不动 internal/controller/(T007)— Forbidden Paths 兑现
- 不动 npu-dra-driver / scheduler-plugin / pool-operator(跨模块禁止)

## Debugging trail

- 无 false start。Plan §3-T006 acceptance 列了 5 tests · 实际 ship 7 tests(5 PrometheusIngestor + 1 FakeIngestor interface guarantee + 1 WindowedAverage helper edge)· 余 2 是 acceptance bonus
- 第一次决定 prometheus HTTP client 选型时考虑 `github.com/prometheus/client_golang/api`(官方 client)vs standard `net/http`:选 **net/http** — 避免 dep 膨胀 + 单 PromQL instant query 实现简单(URL-encode + GET + JSON decode)+ T007 controller test 不需要 mock Prometheus client 库
- 关键 design choice:NoData 语义 — connection refused / DNS fail / timeout 都 fold 进 NoData=true · 不是 Err。这避免 controller 在 Prometheus 短暂 down 时 false-flip `ConditionActive=False`(per ADR-0012 §1 reconcile step 3 contract)。仅 4xx / parse failure / `status != success` 是 genuine Err
- 测试 stub server pattern:`httptest.NewServer` 接 inline handler · response body 一行 JSON · capture URL.Query() 验证 PromQL shape。已 establish 在 Phase 6 metrics_test.go 既有 pattern · 我直接套用
- `helm template` render 验证:`helm template test deploy/helm-charts/inference-operator/ --set metrics.prometheusURL=http://prom.test:9090` 输出含 `NPUVERTICAL_SCALER_PROMETHEUS_URL=http://prom.test:9090` env var · 链路 chart → values → deployment.yaml → manager 容器 env → main.go os.Getenv 完整

## Key decisions

- **Single `metrics` package**(不新建 `internal/ingester/` 或 `internal/metrics/ingestor/`):package 名虽然语义 overload(`metrics` 既 expose 又 ingest)· 但 internal/metrics 已 own Prometheus 相关代码 · 一 package 容纳 expose collectors + ingest client 是合理 grouping。Phase 9 多 metric backend 时再 split
- **PromQL 内嵌 ingestor.go**(不抽 const):Phase 8 ships 唯一一种 PromQL · 直接 fmt.Sprintf 嵌入。Phase 9 PrometheusQuery 引入时可 refactor 抽 const · 不预占
- **HTTP timeout 默认 5s**(`IngestorOpts.QueryTimeout` 零值 fallback):Prometheus instant query 通常 < 100ms · 5s 给足 quantile + retry buffer。短于 30s 的 reconcile cadence · 不会阻塞 reconcile loop
- **5xx → NoData(不是 Err)**:transient · 不应让 controller 翻 ConditionActive。Per ADR-0012 §1 reconcile step 3 + NoData 持续 N 次 tick 后才升级到 `ConditionActive=False reason=MetricsUnreachable`(T007 controller body 实现该 N)
- **`var _ Ingestor = (*FakeIngestor)(nil)` 编译时 assertion**:plan §3-T006 acceptance 明确要求;我同时给 `*PrometheusIngestor` 也加该 assertion(防御性 · 重构 interface 时编译期 catch)
- **`FakeIngestor.CalledWith()`返回 snapshot copy**(非原 slice):防 caller 修改 internal state。`Reset()` 让多 reconcile-loop assertion 在同一 test case 内 clean restart
- **chart `metrics.prometheusURL` default empty**:operator 显式 opt-in scaling decision · 不 push 全部新 inference-operator 部署立即依赖 Prometheus。Degraded mode(empty URL)Ingestor 返回 NoData=true · controller 保持 stay decision

## Verification

- **存在性**:
  - `internal/metrics/types.go` 117 行(Ingestor interface + 4 types + WindowedAverage helper)✅
  - `internal/metrics/ingestor.go` 176 行(PrometheusIngestor + Query + prometheusQueryResponse + compile-time assertion)✅
  - `internal/metrics/fake_ingestor.go` 85 行(FakeIngestor + Query + CalledWith + Reset + compile-time assertion)✅
  - `internal/metrics/ingestor_test.go` 217 行(7 cases · stub server / empty vector / empty URL / transport error / 4xx / FakeIngestor interface / WindowedAverage edge)✅
  - cmd/main.go (env-driven ingestor construction · Phase 8 P8-T-006 setupLog · `_ = ingestor` placeholder until T007 controller registration)✅
  - deploy/helm-charts/inference-operator/values.yaml (`metrics.prometheusURL` + `queryTimeout` + `scrapeIntervalSeconds`)✅
  - deploy/helm-charts/inference-operator/templates/deployment.yaml (env-var conditional injection)✅
  - DESIGN.md §5.3 (interface + PromQL + NoData semantics + chart wiring + Phase 9 forward)✅
  - docs/devlog/phase-8-t006.md (this file)✅
- **完整性**(plan §3-T006 acceptance vs 实际):
  - `go build ./internal/metrics/...` clean ✅
  - `go test ./internal/metrics/...` PASS — 5 PrometheusIngestor cases + 1 FakeIngestor + 1 WindowedAverage = 7 cases · Phase 6 既有 4 metrics tests preserved(11 total PASS · no regression)✅
  - PromQL query 形式 documented · `avg_over_time(ascend_npu_utilization_percent{namespace="$ns", model_service="$ms"}[$window s])` · captured by test assertion in `TestPrometheusIngestor_StubServerWindowAverage` ✅
  - FakeIngestor implements Ingestor interface(`var _ Ingestor = (*FakeIngestor)(nil)` at fake_ingestor.go bottom · `TestFakeIngestor_ImplementsInterface` runtime test)✅
  - chart `helm template test deploy/helm-charts/inference-operator/ --set metrics.prometheusURL=http://prom.test:9090` renders Deployment with `NPUVERTICAL_SCALER_PROMETHEUS_URL` + `NPUVERTICAL_SCALER_QUERY_TIMEOUT` env vars ✅
  - DESIGN.md §5.3 lands with PromQL query + window aggregation + NoData fallback behaviour ✅
  - `helm lint --strict deploy/helm-charts/inference-operator/` clean(0 chart failed)✅
- **正确性**(3 项独立验证):
  - 存在性:`git status` confirm 4 new files + 4 modified files
  - 完整性:`go vet ./...` clean · `go mod tidy` idempotent · `go build ./...` clean(包含 cmd/main.go 修订)
  - 正确性:test stub server + transport error simulation + 4xx response 各 cover 不同 axis · 不仅 happy path

## Carry-forward

- **T007**(NPUVerticalScaler controller body)入手即可注册 `Ingestor` dependency:
  - `cmd/main.go` `_ = ingestor` placeholder 被 T007 替换为 `controller.NPUVerticalScalerReconciler{ Client: ..., Ingestor: ingestor, ... }.SetupWithManager(mgr)`
  - test 用 `FakeIngestor` seed canned values 驱动 reconcile branches(busy threshold cross / idle threshold cross / cooldown active / NoData stay / Err surface)
- **T103**(kind smoke E2E ext)安装时 chart `--set metrics.prometheusURL=http://prometheus.observability:9090`(若 kind cluster 内有 Prometheus)· 否则保持 default empty(degraded mode · scaling 不触发 · 不影响 T103 其他断言)
- **Phase 9 forward note**(已 commit DESIGN.md §5.3.5):`MetricSpec.Type` 加 `PrometheusQuery` enum 时 · `IngestorQuery` 加 PromQL field(forward-compat interface · 不破现有 NPUUtilization 路径)
- **没有 cross-module side effect**:本 commit 不动 npu-dra-driver / scheduler-plugin / pool-operator / configs / mock-data · CI lint/test 应仅触发 inference-operator workflow
