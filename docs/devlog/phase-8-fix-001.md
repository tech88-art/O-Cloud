# P8-fix-001 · phase6/assert.sh metrics scrape retry + diagnostic dump

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Trigger**: e2e-kind #42(e24f365 T107)失败 · Phase 6 `assert_inference_metrics` step · `inference_modelservice_reconcile_duration_seconds` Histogram TYPE 行 missing

## Failure repro

User screenshot of e2e-kind #42 / kind smoke (real cluster) job · "Phase 6 — assert KubeSchedulerConfiguration + scrape metrics" step output:

```
== scrape inference-operator /metrics endpoint ==
tests/e2e/kind/phase6/assert.sh: line 150: echo: write error: Broken pipe
Error: expected metric not found: inference_modelservice_reconcile_duration_seconds
Error: 1/3 inference-operator collectors missing
Error: Process completed with exit code 1.
```

2/3 metrics found(`inference_modelservice_phase_transitions_total` + `inference_pdrouter_decisions_total` · 都 Counter)· 1/3 missing(`inference_modelservice_reconcile_duration_seconds` · Histogram)。

## Root cause analysis

**Phase 8 不动 metrics.go / modelservice_controller.go / prometheus client version**:
- `operators/inference-operator/internal/metrics/metrics.go`(P6-T-104)未改 · `MustRegister(PhaseTransitions, WebhookDecisions, ReconcileDuration)`仍 init()
- `modelservice_controller.go` 仍有 `defer metrics.ObserveReconcileDuration(...)`(line 119-121 · P6-T-104)
- `go.mod` prometheus/client_golang v1.23.2 不变(`git diff ac08e89..HEAD -- operators/inference-operator/go.mod` shows only logr promotion in T005 · no prometheus change)

**Phase 7 + Phase 8 T001-T106 都通过同一 assert**(e2e-kind #31-#41 all green per workflow history)· 唯独 **#42(e24f365 T107)失败** · T107 仅改 docs(checkpoint + tag · 完全无代码变化)· 同 T106 也是 docs-only commit。

**结论**:transient race / response truncation。两个可能机制:
1. **Response truncation**:Histogram 多 lines(HELP + TYPE + N bucket lines + sum + count · 共 ~13 行)· 若 collector 注册顺序是 PhaseTransitions(counter · ~3 行)→ WebhookDecisions(counter · ~3 行)→ ReconcileDuration(histogram · ~13 行)· response 在尾端被截断 → histogram missing 但 counters 完整
2. **Port-forward race**:`kubectl port-forward` + `sleep 3` + `curl` 时序 · 第一次连接可能 partial established · 后续 curl 可见 partial response

**Reconcile DID fire**(Pods scheduled 2/2 after 1s · per screenshot line 15)→ `defer ObserveReconcileDuration` 必触发 → histogram 有 ≥1 observation → TYPE 行应当 emit。所以是 transport/transient 层问题 · 不是 application 层 bug。

## Fix(本 commit)

`tests/e2e/kind/phase6/assert.sh` `scrape_inference_metrics`:

- **3-attempt retry loop**(10s sleep between)· 每 attempt fresh port-forward(避 dropped/half-open connection 毒化)
- `curl --max-time 10`(防 curl 自身 hang 永等)
- `printf '%s' "${body}"` 替换 `echo "${body}"`(consistency · `echo` 在 bash 与某些 shell 处理 `\n` 不同)
- 最终失败前 diagnostic dump:
  - 完整 `/metrics` body(最后一次 attempt)· `::group::Final /metrics response body`
  - inference-operator Pod describe · `::group::inference-operator Pod describe`
  - inference-operator Pod logs(last 200)· `::group::inference-operator Pod logs`
  - inference-operator metrics Service yaml · `::group::inference-operator metrics Service`

## Path adaptations

- 仅改 `tests/e2e/kind/phase6/assert.sh`(scrape_inference_metrics function)+ 本 devlog
- 不动 metrics.go / modelservice_controller.go / chart / cmd/main.go(无 application 层 bug)
- 同 Phase 7 P7-fix-001..004 pattern(本 phase 第一个 fix · 命名 P8-fix-001)

## Verification

- **Local syntax**:`bash -n tests/e2e/kind/phase6/assert.sh` ✅ clean
- **Logic**:
  - retry 3 attempts · 10s sleep · 总 ~30s tolerance
  - fresh port-forward 每 attempt(避 race accumulation)
  - all 3 metrics found in ANY single attempt → return 0(early exit)
  - all attempts exhausted with missing > 0 → diagnostic dump + return 1
- **No app-layer change**:metrics.go / controller / chart 全 unchanged
- **CI 实证**:本 commit push 后 CI 重跑 · 如 transient race 则 retry 内消化 PASS · 如真正 application bug 则 retry 后仍 fail · diagnostic dump 给出 body 内容用于二次诊断

## Carry-forward

- 若 P8-fix-001 push 后 e2e-kind 仍失败 + diagnostic dump 显示 metrics.go 真有 bug · 提 P8-fix-002 修代码层
- 若 P8-fix-001 push 后通过 · 视为 transient flake 已 absorbed · CI gate 通过 · Phase 8 真完成
- Phase 9 W1 entry 时可考虑彻底 stabilize:把 `scrape_inference_metrics` 改用 ServiceMonitor + Prometheus 实际 scrape(本 chart 已有 ServiceMonitor opt-in)· 取代 ad-hoc port-forward curl 路径
