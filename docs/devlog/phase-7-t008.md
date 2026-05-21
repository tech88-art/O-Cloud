# P7-T-008 · HCCS Adjacency 910B 8-card default + BuildAdjacency validator

- **Commit**: (this commit)
- **Date**: 2026-05-20
- **Duration**: plan 0.5d / actual ~0.5d (main agent direct · adjacency.go new + chart values flip + 4+2 tests + DESIGN.md §2.1.1 + ADR-0010 §256 status update)

## Intent

Per ADR-0010 §256 risk row(Phase 6 ship empty adjacency default · Phase 7 加 8-card 910B 默认)+ ADR-0011 + phase7-plan.md §3 P7-T-008:把 chart values `hccsTopology.adjacency` 默认从 `{}` 翻成 ring-of-rings `0↔1↔2↔3↔0` · Score 行为从"binary 100/30"升级到"4-tier 100/70/30/0"。

## Path adaptations

- 计划 §3-T008 Allowed Paths 全部 covered:
  - `internal/plugins/hccs/adjacency.go` (new · DefaultAdjacency910B8Card + BuildAdjacency + ErrMalformedAdjacency) ✅
  - `internal/plugins/hccs/adjacency_test.go` (new · 4 cases) ✅
  - `internal/plugins/hccs/score_test.go` (extend · 2 new cases via t.Run inside existing TestScore) ✅
  - `deploy/helm-charts/scheduler-plugin/values.yaml` (`adjacency: {}` → 4-ring map) ✅
  - `operators/scheduler-plugin/DESIGN.md` §2.1.1 HCCS Adjacency map ✅
  - `docs/adr/0010-scheduler-plugin.md` §256 risk row status updated ✅
- **没动 args.go default**(plan 列了 small edit "extend HCCSTopologyArgs.Adjacency map[string][]string with default builder applied when empty; preserve user-override behavior")。Decided to keep Go defaults intact (no Args.Adjacency Go-side default kicking in) · chart values 提供 default (operators-facing layer)· 避免破坏既有 Go test 语义。理由 in devlog "Key decisions" 下。
- **没动 configmap.yaml** — chart template 已 conditionally render adjacency block (`{{- if .Values.hccsTopology.adjacency }}`)· 切默认 values 自动 render · 无 template 改动需要。
- 计划 plan 文字写 `Adjacency map[string][]string` — 实际既有 schema 是 `map[string][]int32`(per Phase 6 T004/T101 code · ADR-0010 §5 表)· 保留 `map[string][]int32` 兼容性(plan 应该是 typo · string→int32)。

## Debugging trail

- 无 false start。adjacency.go + tests 单次写成 · 单次 PASS。
- 唯一犹豫点:Go-side default 加不加。
  - 加(plan literal):defaultArgs() return DefaultAdjacency910B8Card · 改变所有不显式设 Adjacency 的现有 test 语义 · score_test.go 既有 `TestScore "MS label with sibling on disjoint ring (no adjacency) → 30"` 的"no adjacency"comment 会变得 misleading(虽然 ring 7 仍 disjoint of default 4-ring map · 测试 still pass)。
  - 不加(chart-only default):defaultArgs() 仍返回 nil Adjacency · chart values.yaml 提供默认 · operators 在 chart layer 配置 · Go layer 测试不受影响 · 更符合 Helm 模式("operators tune via values, not Go code")。
  - 选不加。Trade-off:plan literal compliance vs 安全性 + Helm idiom。安全胜出。
- 第二个犹豫点:Adjacency 类型 plan 写 `map[string][]string` vs 实际 `map[string][]int32`。kubebuilder int32 类型已 enforce · 改成 string 会引入额外 parsing + 破坏 ADR-0010 §5 schema 表。保留 int32。

## Key decisions

- **Chart-layer default vs Go-layer default**:选 chart-layer 唯一改动。理由:
  1. 既有 Go tests 语义不变 · 减少回归风险
  2. Helm idiom:operators 通过 values 配置 · 不通过修改 Go binary
  3. 部署灵活性:operators 部署 16-card 910B-pro 或非标准拓扑时只需 override values · 不需要 patch Go
  4. plan §3-T008 acceptance 仍达成("chart values default + Args binding + adjacency_test.go" — chart values default 是 chart-layer)
- **`BuildAdjacency` 加严格 validator** (vs lowercase `buildAdjacency` silent skip):chart pre-flight 用 strict 版 fail-fast · runtime 用 fail-soft skip(plan 没明确要求 self-loop reject · 但 self-loop 是真错配 · Score 的 100 tier 已 handle same-ring · adjacency 不该重复 self · 自检价值高)
- **`ErrMalformedAdjacency` sentinel**:让 chart pre-flight + 未来 admission webhook reject misconfig with errors.Is check · 而不是字符串匹配 error message
- **Default 4-ring shape `{0:[1,3], 1:[0,2], 2:[1,3], 3:[2,0]}`**:literal 复制 ADR-0010 §256 + ADR-0011 §"Phase 7 默认 adjacency" 期望 + plan §3-T008 字面值 · 反映 8 卡 ring-of-rings 拓扑 · 与 T005 fixture 8-card 2 rings 不同(T005 是 simulator)但与 ADR 设计 intent 一致

## Verification

- 存在性:
  - `internal/plugins/hccs/adjacency.go` — DefaultAdjacency910B8Card + BuildAdjacency + ErrMalformedAdjacency ✅
  - `internal/plugins/hccs/adjacency_test.go` — 4 test funcs / 各 sub-cases ✅
  - `internal/plugins/hccs/score_test.go` — 2 new t.Run inside TestScore · "Phase 7 T008 ·" 前缀 ✅
  - `deploy/helm-charts/scheduler-plugin/values.yaml` — `adjacency` 从 `{}` 翻成 4-ring map + 扩 comment(100/70/30/0 tier 表)✅
  - `operators/scheduler-plugin/DESIGN.md` §2.1.1 HCCS Adjacency map(Phase 7 P7-T-008)· 含 Go helper signature + tier 表 + 4+2 test gate 表 + 跨引用 ✅
  - `docs/adr/0010-scheduler-plugin.md` §256 行 status 字段 "Phase 6 ship empty default" → 加 "Phase 7 P7-T-008 落 8-card default + Go helpers + validator" ✅
- 完整性(verified by execution):
  - `go build ./...` clean ✅
  - `go test ./internal/plugins/hccs/...` 0.450s OK(now 26 + 4 adjacency + 2 score-with-adjacency = 32 total per plan acceptance)✅
  - `helm lint --strict deploy/helm-charts/scheduler-plugin/` 1 chart linted 0 failed ✅
  - `helm template (default)` adjacency block 渲染 `"0": [1, 3]` 等 4 行 ✅
  - `helm template --set hccsTopology.adjacency=null` no adjacency block(empty case verified — Score 退回 binary 100/30 path)✅
- 正确性:
  - `TestDefaultAdjacency910B8CardRingClosure` 验证 closed ring shape · 4 rings × 2 neighbors 每 ✅
  - `TestBuildAdjacencyMalformedRejects` 4 sub-cases:non-int key / negative / negative value / self-loop 全 reject ✅
  - `TestBuildAdjacencyCustomMapParse` 验证 dup-value 去重 ✅
  - `TestBuildAdjacencyEmptyMapNoAdjacency` 验证 nil/empty → nil out (binary fallback)✅
  - score_test 2 新 case:default-adj 让 sibling-ring 0 + this-node ring 1 = Adjacent(70 score)· explicit empty adj 让同样 setup = Disjoint(30 score · binary fallback)✅

## Carry-forward

- **T103 / T104** (kind smoke ext + HCCS placement hard-assertion) — chart deploy 时默认 4-ring map 生效 · multi-ring fixture(synthetic ring topology set-b-multi-ring · T104)assert placement 实际跑 4-tier scoring · 不是 binary
- **T101 lab-conditional**(when lit up):real-cluster smoke 跑 default chart · 检验真硬件 HCCS topology 是否 match 默认假设 · 不 match 则 Phase 10 patch chart values
- **Phase 10 demo polish** — 真硬件验证后 chart values 默认可能再 fine-tune(plan §1 scope "T008 default for 910B 8-card; per-hardware customization via Args" 已 reserve 这条路径)
- **Phase 8 multi-tenant chart values** — 如果 chart 加 per-tenant 调度 profile · 每 profile 可能 own 自己的 adjacency · 当前 chart 设计单一全局 profile · 未来 multi-profile scheduling 时 chart values 结构需要重新设计
