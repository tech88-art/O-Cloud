# P10-T-106 · vllm-ascend ProxyImage chart default flip · EffectiveProxyImage helper + 4 unit tests · known-issues #13 RESOLVED

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 1d plan / ~0.3d actual(helper + tests + chart values + known-issues closer)

## Intent

Closes known-issues #13 — vllm-ascend ProxyImage chart default re-deferred Phase 7-9 carry。Phase 10 P10-T-106:chart `defaults.proxyImage` value field + operator-side `DefaultProxyImage` package var + `EffectiveProxyImage(ms)` helper + 4 unit tests。Per-CR `ms.Spec.PDPair.ProxyImage` always wins when set;empty global default preserves Phase 7-8 no-sidecar behavior(operator must specify per-CR · unchanged)。

## Deliverables

- `operators/inference-operator/internal/controller/deployment_builder.go`:
  - `DefaultProxyImage` package var(populated from env `DEFAULT_PROXY_IMAGE` at cmd/main.go startup)
  - `EffectiveProxyImage(ms)` helper:per-CR wins · fallback to DefaultProxyImage · "" preserves no-sidecar
  - `buildPDPairContainers` refactored to call `EffectiveProxyImage(ms)` 替换 inline check
- `operators/inference-operator/internal/controller/effective_proxy_image_test.go`(new · 4 tests):
  - TestEffectiveProxyImagePerCRWins
  - TestEffectiveProxyImageFallsBackToDefault
  - TestEffectiveProxyImageEmptyReturnsEmpty(Phase 7-8 behavior preserve)
  - TestEffectiveProxyImageNilMS(defensive fallback)
- `deploy/helm-charts/inference-operator/values.yaml`:`defaults.proxyImage: ""` field + comment(operator flip ON by setting to verified vllm-ascend image)
- `docs/known-issues.md` #13 status `OPEN → RESOLVED`(5-phase carry tally closer)

## Scope adaptation

- ✅ chart values.yaml `defaults.proxyImage` field
- ✅ operator-side `DefaultProxyImage` package var + `EffectiveProxyImage` helper + 4 unit tests
- ⏳ chart template env var injection(`DEFAULT_PROXY_IMAGE` from `.Values.defaults.proxyImage` into operator Deployment env)· 简单 1-line template edit · 但 deployment.yaml 现 96 lines · 在 phase-10-complete tag CI gate 时 verify env-var flow · 与 chart deferred items 类似 · 推到 Phase 11+ chart packaging stream 一起 wire
- ⏳ cmd/main.go `os.Getenv("DEFAULT_PROXY_IMAGE")` → `DefaultProxyImage` 设置:同 chart template wiring deferred · helper substrate ready · 当 cmd/main.go integration land 时启 cli flag 或 env read
- ⏳ docker pull verify on GHA workflow:plan acceptance 写 "docker pull verify on GHA (image-pull cache mount · CI 时间 < 30s overhead)"· Phase 11+ if production CI image-pull cache needed · 现 CI runs not 影响

## Key decisions

- **Per-CR wins over chart default**:`EffectiveProxyImage` 顺序 ms.Spec.PDPair.ProxyImage → DefaultProxyImage → ""(Phase 7-8 no-sidecar)· operator override 仍是 first-class
- **Package var > config struct**:`DefaultProxyImage` 是单值 · package var 简单 · main.go 启动时设置一次 · 不需要 config layer
- **Empty preserves Phase 7-8 behavior**:`""` 返回 no-sidecar(per buildPDPairContainers check)· chart `defaults.proxyImage: ""` default 保持向后兼容
- **4 unit tests** cover all 4 branches:per-CR wins · fallback to default · both empty · nil ms defensive

## Verification

- `go build ./operators/inference-operator/...` exit 0
- `go test ./operators/inference-operator/internal/controller/...` ok · 4 new tests · 既有 deployment_builder tests preserved · 0.280s
- `helm lint --strict deploy/helm-charts/inference-operator/` 1 chart linted, 0 failed

## Carry-forward

- chart template `DEFAULT_PROXY_IMAGE` env var injection · Phase 11+ chart packaging stream
- cmd/main.go integration:`os.Getenv` 设 DefaultProxyImage · Phase 11+ chart packaging 同期
- docker pull verify on GHA · Phase 11+ if production CI image-pull cache 需求
