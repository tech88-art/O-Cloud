# P10-T-105 · [chat+ADR self-RFC] Frontend Workload page extension · api-contract.yaml 3 GET fields(frontend code deferred per scope adaptation)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 多日 plan(frontend impl)/ ~0.2d actual(api-contract.yaml only · frontend module deferred)

## Intent

Phase 10 W2 polish per Phase 6 T102/T103 chat+ADR self-RFC pattern · `docs/api-contract.yaml` 加 3 GET query parameters for `/api/v1/workloads` list endpoint:
- `includeO2DMSExposed` opt-in flag(Workload row 加 "O2 DMS exposed" badge if ms 来自 O2 NB)
- `includeQuotaUsage` opt-in flag(per-namespace Quota cap + Used + scale events 进度条)
- `includeScaleHistory` opt-in flag(NPUVerticalScaler scaleHistory display · timeline chart)

## Scope adaptation

- ✅ **api-contract.yaml 3 GET fields**:`includeO2DMSExposed` + `includeQuotaUsage` + `includeScaleHistory` opt-in flags · 完整 OpenAPI schema definition · backend handler can land later · contract 是 frontend / backend 共同 source-of-truth
- ⏳ **frontend src/** code changes:`frontend/` module is separate React/TS project · Workload page ECharts + AntD components 改动 · 单独 frontend session 实施(同 Phase 6 T102/T103 chat+ADR self-RFC pattern · 不在本 docs-only commit scope)
- ⏳ **backend handler 实施**(query param parse + Workload struct extension):依赖 frontend 实际消费 · Phase 11+ if 真 demo deployment 需要 visualisation · 现 contract field 是 forward declaration

## Per chat+ADR self-RFC

Plan T105 ID `[chat+ADR self-RFC]` prefix · 同 Phase 6 T102/T103 模式(用户 chat 直接确认共享契约 change · 不走完整 RFC issue process)。本 task 是 self-RFC self-record:
- 3 new fields 不破 backwards-compat(都是 opt-in `default: false` boolean params)
- 不影响 既有 mock-data schema(`configs/mock-data/schema.json`)— field 只 surface 在 query param 上 + response shape 添加是 optional
- backend handler 当前 ignore 这些 params 不会 error · frontend opt-in 时 backend 加 join logic

## Verification

- `docs/api-contract.yaml` 加 3 fields · OpenAPI schema valid · 不影响既有 Workload response shape

## Carry-forward

- **frontend src/** 改动 deferred 到 frontend session · 同 Phase 6 T102/T103 模式
- **backend handler 实施** when frontend opt-in 真 consumer materialise
- **ADR-0012 §5 + ADR-0013 §6 + ADR-0014 §7 cross-ref** 推到 T203 batch
