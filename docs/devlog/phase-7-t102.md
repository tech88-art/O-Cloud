# P7-T-102 · vllm-ascend v0.12+ ProxyImage doc-only refresh (Phase 7 W2 gating fallback)

- **Commit**: (this commit)
- **Date**: 2026-05-20
- **Duration**: plan 0.5d (full flip) / 0.25d (doc-only) · actual ~0.25d

## Intent

Per phase7-plan.md §4 P7-T-102 gating decision:check vllm-ascend v0.12+ GA status · 若 GA + CI image-pull access available → full flip(chart default proxyImage = v0.12+ tag);若 GA but image too large for CI → doc-only(chart default empty 保留)。

## Gating decision outcome

**GA confirmed** via vllm-ascend GitHub releases:v0.13.0 final + v0.18.0 (2024-04-30 latest stable · "Latest" tag)。但 chart default 翻转风险:
- CI image-pull access from GHA runners(~5GB vllm-ascend image · GHA disk budget concern)未实测
- `quay.io/vllm-project/vllm-ascend` 确切 tag 命名规范 + 是否可拉 unverified
- Phase 6 T106 + Phase 7 T103 kind smoke 仍用 `fallbackImage=busybox` skip 真 pull · 这条 fallback 路径已在 chart 内 + 测试覆盖

**Decision**:**doc-only refresh** per phase7-plan §4-T102 fallback Allowed Paths。Chart `defaults.proxyImage` 不动 · 保持 empty Phase 5 单容器行为;operators 想要 proxy sidecar 显式设 `ms.Spec.PDPair.ProxyImage="quay.io/vllm-project/vllm-ascend:v0.18.0"`(或当前 stable 任意)。Phase 10 demo polish 时验证 CI image-pull + 真硬件 lab smoke 后 chart default 可清晰翻转。

## Path adaptations

- 计划 §4-T102 doc-only fallback Allowed Paths 全部 covered:
  - `operators/inference-operator/DESIGN.md` (small edit — refresh) — 新 §5.0.1 Phase 7 P7-T-102 vllm-ascend v0.12+ status section · 含 GA 矩阵 + outcome rationale ✅
  - `docs/adr/0010-scheduler-plugin.md` (small edit IF needed) — N/A:既有 §3 deferred 段已 mention v0.12 gating · 不需新加段 ✅
  - T102 commit message 记录 gating-decision outcome(本 commit message)✅
- 额外:`operators/inference-operator/api/v1alpha1/modelservice_types.go` ProxyImage 字段 godoc 加 "P7-T-102 gating (2026-05-20)" 段 — godoc 是面向 operator API 文档 · 让操作员直接看到字段说明就 know v0.18.0 是当前 stable + chart default 仍 empty + 自己显式 set 的方法
- `controller-gen` regen 触发 CRD YAML description 字段刷新 — schema 行为不变 · 只是 description 字串变长

## Debugging trail

- 无 false start。WebFetch 一次性确认 GA 状态 · doc-only edit 单次写成 · tests + build clean。
- 唯一犹豫点:既然 GA · 是否应"勇敢翻转"?决定**保守** — flip 需要 CI image-pull 验证 · 没验证就翻 = 风险 · 风险大于"等 Phase 10 demo polish 真硬件验证后再翻"的迟一些价值。M4 价值聚焦 · 不为 "完整翻转" 而冒不必要风险。
- 第二个犹豫点:godoc 改动是否触发 CRD schema diff?是 — description 字段更新。但 description 是 user-facing 元数据 · 不影响 admission 校验 / 现有客户端 / 现有 operators · 安全。

## Key decisions

- **Doc-only over full flip** per Phase 7 W1 conservative posture(M4 value-focus):
  - 价值:operators 看 godoc + DESIGN.md 立即明白 v0.12+ GA 状态 + 怎么手动 opt in
  - 风险:零(无 chart values 改动 · 无 default 行为变化 · 既有 5 个 buildPDPairContainers tests 全 pass)
  - Phase 10 demo polish 时 chart default 翻转 = 1 行改动 + 实测 image pull · 该工作量 dispatch to Phase 10 而非 Phase 7 W2 prematurely
- **godoc 写"latest stable v0.18.0"而非"v0.12+"含糊**:operators 看 godoc 立即知道当前推荐版本;明天 vllm-ascend 出新版 godoc 略 stale 但读者立即能从字串 know upstream 已演进很多
- **DESIGN.md 新 §5.0.1 而非改既有 §5.0**:既有 §5.0 描述 Phase 6 T105 ship 时的设计意图 + env-var 契约 · 仍正确 · 加 §5.0.1 描述"T102 时的 GA gating 决策" · 历史层级清晰

## Verification

- 存在性:
  - `operators/inference-operator/DESIGN.md` §5.0.1 — Phase 7 P7-T-102 vllm-ascend v0.12+ status · 含 GA 矩阵 + outcome rationale + Phase 10 forward path + cross-refs ✅
  - `operators/inference-operator/api/v1alpha1/modelservice_types.go` ProxyImage 字段 godoc 含 "P7-T-102 gating (2026-05-20)" 段 ✅
  - `config/crd/bases/...modelservices.yaml` (regen) description 字段 carries 新 godoc (kubectl get/describe CRD will show updated text) ✅
  - `api/v1alpha1/zz_generated.deepcopy.go` (regen · no semantic change) ✅
  - `docs/devlog/phase-7-t102.md` (this file) ✅
- 完整性(verified by execution):
  - `controller-gen object paths=./api/v1alpha1/` clean ✅
  - `controller-gen crd paths=./api/v1alpha1/ output:crd:artifacts:config=config/crd/bases` clean ✅
  - `go build ./...` clean (inference-operator package全 PASS · 既有 5 个 builder tests + 18 controller tests + 18 webhook tests + 4 metrics tests = 45 tests)✅
  - `go test ./...` 3 packages OK ✅
- 正确性:
  - WebFetch 数据 vs plan §4 T102 gating condition:v0.12 GA "v0.13.0 final release of v0.13.0 line" + v0.18.0 "Latest" → ≥ v0.12 + GA · condition 第 1 项 met
  - condition 第 2 项 "image-pull access from CI"未实测 → fall to doc-only(保守解读)
- 注释:Phase 7 T103 / T104 kind smoke 继续用 `fallbackImage=busybox` skip 真 pull · 不受本 T102 决策影响(plan §4-T102 fallback 路径明确 retain 该行为)

## Carry-forward

- **Phase 10 demo polish** — 验证 CI image-pull access · 确认 `quay.io/vllm-project/vllm-ascend:vX.Y.Z` tag 规范 + 镜像大小 vs GHA disk budget · 翻 chart `defaults.proxyImage` 默认 · 此时 1 行改 + CI 实测 = 干净
- **operators 想跑 PD-pair proxy 今天怎么做**:`ms.Spec.PDPair.ProxyImage = "quay.io/vllm-project/vllm-ascend:v0.18.0"`(或 v0.13.0 / 当前 stable)— deployment_builder.go::buildPDPairContainers 已 ship 该路径(Phase 6 T105 + Phase 7 T003 schedulerName auto-stamp 一起 ship)· 无需 chart 改动
- **Phase 7 T103 kind smoke** — multi-ring fixture 用 `fallbackImage=busybox` skip 真 pull(保持 Phase 6 T106 pattern · 已 ship)· T103 不需 ProxyImage 设置(测的是 schedulerName + NumaAffinity + NPUSliceTemplate · 不测 PD proxy 容器行为)
- **upstream vllm-ascend env-var schema 演进监控** — 若 v0.19+ 改 `VLLM_PD_*` 命名约定 · DESIGN.md §5.0 env-var contract 表需要更新 · 不在 T102 范围
