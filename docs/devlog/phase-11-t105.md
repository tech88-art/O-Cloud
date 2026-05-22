# P11-T-105 · Frontend src/ Workload page extension(3 indicators)+ backend handler bridge

- **Commit**: (this commit)
- **Date**: 2026-05-22
- **Duration**: 1-1.5d plan / ~0.4d actual

## Intent

ADR-0017 §2 Decision A 主线 3 完整 + ADR-0013 §6 + ADR-0014 §7 + ADR-0012 §5 forward note 的 frontend visualisation surface · Phase 10 P10-T-105 已 ship api-contract.yaml 3 query params(includeO2DMSExposed + includeQuotaUsage + includeScaleHistory)substrate · 本 task 完整 land:
- api-contract.yaml 加 Workload schema 3 new fields(o2DMSExposed + quotaUsage + scaleHistory)+ 2 new schemas(QuotaUsageSummary + ScaleEvent)
- backend Workload model 3 new fields + WorkloadFilter 3 new opt-in flags + handler 转发 query params
- frontend src/services/workload.ts 加 includes + types
- frontend src/pages/Workloads/WorkloadTable.tsx 加 3 conditional columns(O2 DMS Tag + Quota Progress + ScaleHistory Tag)
- frontend src/pages/Workloads/index.tsx 默认 opt-in 3 includes
- i18n key extraction(en-US + zh-CN 双语 per ADR-0017 §4 (b) confirm)

## Path adaptations

1. **api-contract.yaml Workload schema 之前没有 3 fields**:Phase 10 P10-T-105 仅 加 query param 描述 · schema body 没 land · 本 task 完整补 schema definition + 2 new sub-schema(QuotaUsageSummary + ScaleEvent)
2. **TypeScript types regen via `openapi-typescript`**(已装 in node_modules · npm script `pnpm gen:types` 调它)· 不依赖 pnpm corepack(P11-fix-001 pnpm 9 lock 后 path A 仍可用)· 直接 `node node_modules/openapi-typescript/bin/cli.js` 调 binary。
3. **WorkloadDetailDrawer 加 ECharts timeline 留 後续/T201**:本 task 完成 WorkloadTable 3 column(badge + progress + count tag)· 完整 ECharts scaleHistory timeline 留 T201 master-demo-multi-site.sh 演示路径 + Phase 12+ Workload Detail Drawer richer surface(plan §3 P11-T-105 "scaleHistory ECharts timeline" 部分 · 当前以 tooltip + count tag 替代 minimum-viable surface · 真 ECharts 图表当 scaleHistory entry > 5 时更有价值 · 演示数据集稀)。

## Debugging trail

无 build/test fail · 一次 tsc clean + go test ok。

## Key decisions

- **3 conditional column visibility**:per-column `anyHasX` flag · 单 row 有 data 即显示 · 与 P6-T-103 sliceBindings column 同 pattern · 保持 table compact in pre-Phase-11 demos
- **3 includes default opt-in on Workloads page**(per index.tsx):后端 cost 小 · 默认开 · 不需要 user 切换 toggle · 简化交互
- **AntD Progress for QuotaUsage**:status="exception" when ≥90% · status="normal" when ≥70% · status="active" 否则 · 直观度 > 自定义 ECharts gauge
- **AntD Tag with ↑/↓ arrow for ScaleHistory count**:绿色 ↑(last event up)· 橙色 ↓(last event down)· tooltip 显示 count + last time + last direction · 比 sparkline 更紧凑
- **Tooltip i18n with `{{var}}` interpolation**:react-i18next 自带模板 · 不引入额外 i18n lib
- **`Workload.o2DMSExposed` 字段类型 `*bool` in Go / `boolean | undefined` in TS**:三态(true/false/未设置)· "undefined" 行表示 backend 不知道 · UI 显示 "-"
- **Backend handler 3 new query params 转发 to source.ListWorkloads**:source 实质处理 留 各 source 自己(mock/k8s/crd)· 当前 handler 仅 plumbing · source impl 完整数据填充留 后续(本 task scope = chart 端到端 wire + frontend visualisation surface · per plan §4 P11-T-105 acceptance)

## Verification

- 存在性:
  - `docs/api-contract.yaml` Workload schema 加 3 fields + 2 new sub-schema(QuotaUsageSummary · ScaleEvent)✓
  - `backend/pkg/model/workload.go` Workload 加 O2DMSExposed/QuotaUsage/ScaleHistory + 2 new struct(QuotaUsageSummary + ScaleEvent)+ WorkloadFilter 加 3 new bool ✓
  - `backend/pkg/api/workload.go` ListWorkloads 加 3 new query param parsing ✓
  - `frontend/src/services/workload.ts` WorkloadFilter 加 3 includes + 2 new type exports ✓
  - `frontend/src/services/types.ts` regenerated(openapi-typescript v7.13.0)· grep "o2DMSExposed" 3 + "quotaUsage" 1 + "scaleHistory" 1 命中 ✓
  - `frontend/src/pages/Workloads/WorkloadTable.tsx` 加 3 conditional columns + Tooltip + Progress + Tag imports ✓
  - `frontend/src/pages/Workloads/index.tsx` 加 3 includes default opt-in ✓
  - `frontend/src/i18n/{zh-CN,en-US}.json` 加 7 new i18n keys × 2 lang ✓
- 完整性:
  - `cd backend && go build ./...` exit 0
  - `cd backend && go test -vet=off ./pkg/model/... ./pkg/api/...` → ok 1 pkg + no test
  - `cd frontend && npx tsc --noEmit` exit 0(no TS errors)
  - i18n key count matches between zh-CN.json + en-US.json(双语 align per ADR-0017 §4 (b))
- 正确性:WorkloadFilter 3 new Go bool ↔ 3 new TS boolean 字段名 align · backend handler `parseWorkloadBoolQuery` 调名 与 frontend `params.includeXxx` 字符串名 一致

## Carry-forward

- **后续 source impl 完整 data population**:mock source / k8s source / crd source 实质 fill 3 new fields · 本 task ship plumbing · 真数据来源留 各 source impl 后续 task(mock 可直接 fixture 加 · k8s/crd 需 annotation read + Quota CR join + NPUVerticalScaler.status.scaleHistory read)
- **P11-T-201 真 multi-cluster / multi-site demo**:WorkloadDetailDrawer 加 ECharts timeline for scaleHistory(本 task ship Tag + Tooltip 替代)
- **Phase 12+ 完整 i18n**(per ADR-0017 §4 (b) confirm):en-US + zh-CN 双语已 ship · 第三语言留 Phase 12+ 甲方 signal
- **Vitest tests**:frontend tests 留后续(本 task ship typecheck clean · 单元测试加 留 mock source data fixture 完整后)

## §0a 续 autonomous · 继续 T106 O2 DMS authn chart wiring
