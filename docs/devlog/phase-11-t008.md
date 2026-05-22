# P11-T-008 · inference-operator chart DEFAULT_PROXY_IMAGE env wire

- **Commit**: (this commit)
- **Date**: 2026-05-22
- **Duration**: 0.5d plan / ~0.2d actual(small chart edit + 5-line main.go + 1 test)

## Intent

W1 chart packaging spine 末端 short task。ADR-0017 §2 Decision D 6th 优先级 + ADR-0010 known-issues #13 5-phase carry closer。把 P10-T-106 EffectiveProxyImage substrate(已 land in deployment_builder.go)wired 到 chart values → deployment env → cmd/main.go startup hook → controller.DefaultProxyImage package var。

## Path adaptations

1. **Chart `defaults.proxyImage` field 实际未存**:plan + ADR-0015/deployment_builder.go comment 都说"chart `defaults.proxyImage` 已 land via P10-T-106 substrate"· 但 grep 实测 `deploy/helm-charts/inference-operator/values.yaml` 无 `defaults:` 或 `proxyImage` field。本 task 不仅是 wire env · 是同时 add chart field + wire env + main.go hook + test。教训 P3 verify-before-claim · 不假定 substrate 完整。
2. **`os` import 在 main.go 已存**(line 22):startup hook 不需新 import。

## Debugging trail

无 build/test fail · 一次 helm template + go test pass。

## Key decisions

- **`defaults.proxyImage` 默认空**(不预填具体 image)· operators 显式 opt-in `--set defaults.proxyImage=...` · 与 EffectiveProxyImage() 现有语义 align(per-CR > default > "")
- **`{{- if and .Values.defaults .Values.defaults.proxyImage }}` conditional**:仅当 chart 设了非空 default 才注入 env(不污染 deployment env block)
- **Test 设计**:`TestDefaultProxyImageEnvInjection` 模拟 cmd/main.go startup hook 行为(setenv → DefaultProxyImage = v)· 验证 EffectiveProxyImage 在 per-CR 无值时返 env injected value · t.Cleanup 恢复 env(避免污染其他 test)

## Verification

- **存在性**:
  - chart values.yaml `defaults.proxyImage` field ✓
  - deployment.yaml DEFAULT_PROXY_IMAGE env conditional ✓
  - cmd/main.go startup hook 12 lines(包含 setupLog.Info)✓
  - effective_proxy_image_test.go TestDefaultProxyImageEnvInjection ✓
- **完整性**:
  - `helm lint --strict` clean
  - `helm template --set defaults.proxyImage=quay.io/vllm-project/vllm-ascend:v0.18.0` → `- name: DEFAULT_PROXY_IMAGE / value: "quay.io/vllm-project/vllm-ascend:v0.18.0"` renders
  - `go test -run "TestDefaultProxyImageEnvInjection|TestEffectiveProxyImage" ./internal/controller/` → ok
  - `bash tests/e2e/kind/phase11/assert.sh` → T107-A5 PASS(改写前 SKIP "T008 carry"· 现 chart 已 ship)
- **正确性**:per-CR 优先级仍保持(TestEffectiveProxyImagePerCRWins 仍 PASS · 不破现有行为)· chart 默认空时 env 不注入(conditional 守门)

## Carry-forward

- **P11-T-107 kind smoke E2E** 可扩展:`helm install --set defaults.proxyImage=...` + `kubectl exec` 入 manager Pod + `env | grep DEFAULT_PROXY_IMAGE` 真集群 verify(本 task ship syntactic + integration test · 真 kind smoke 留 T107 body land)
- **Phase 12+ Vault Secret 路径**(per ADR-0018 §4 (c)):chart `defaults.proxyImage` 可改为从 Secret 读 · 避免 image registry private repo 凭据混入 plaintext values · 与 Vault Secret integration 同 cohort
- **W1 chart packaging spine 闭环完成**:T003(demo-backend)+ T004/T005/T006(3 IMS)+ T007(scheduler-plugin NRT)+ T008(inference-operator env)· 6 chart 全 land · ADR-0017 §2 Decision A 主线 1 完整 close

## §0a 续 autonomous · 继续 T101(LAB-CONDITIONAL 5th 默认-deferred 路径)
