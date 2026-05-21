# P8-T-004 · vllm-ascend ProxyImage chart default flip — doc-only fallback (re-deferred to Phase 10)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: plan 0.3-0.5d / actual ~0.2d (doc-only fallback path)

## Intent

Plan §3-T004 原 intent:check vllm-ascend image-pull access + tag convention · 若验证 pass → chart `defaults.proxyImage` 翻到 pinned `quay.io/vllm-project/vllm-ascend:v0.18.0` · 否则 doc-only carry to Phase 10。

实际落地形态:**doc-only fallback**(同 Phase 7 P7-T-102 outcome 一致 · refresh + 新 blocker 记录 + Phase 10 carry)。

## Path adaptations

- Plan §3 P8-T-004 doc-only fallback Allowed Paths covered:
  - `operators/inference-operator/DESIGN.md` (small edit) — 加新 §5.0.2 "Phase 8 P8-T-004 vllm-ascend ProxyImage chart flip re-deferred to Phase 10 (2026-05-21)" 段 · 含 Phase 8 W1 entry re-verify outcome 表 + 4 维度 reason aggregation + Phase 10 forward path + cross-refs ✅
  - `deploy/helm-charts/inference-operator/values.yaml`(no change · 确认 default 仍 empty)— grep confirm chart default `defaults.proxyImage: ""` 维持 ✅
  - `docs/devlog/phase-8-t004.md`(本 file)✅
  - `docs/known-issues.md`(small edit — add new entry #13 "ProxyImage chart default re-deferred Phase 7 → Phase 8 → Phase 10")✅
- 不动 source code、不动 chart templates、不动 deployment_builder.go、不动 tests
- `effectiveProxyImage` fallback chain(Phase 6 T105)保持不变 · operators 显式 set `ms.Spec.PDPair.ProxyImage` 路径继续工作

## Debugging trail

- 无 false start。Phase 7 T102 devlog 提供完整的 outcome 模板 · 本 task 主要是 re-verify upstream state + 记录新 blocker。
- WebFetch attempts:
  - `quay.io/repository/vllm-project/vllm-ascend?tab=tags` → Quay 公共 web UI read-only error · 无 tag 数据可读
  - `https://api.github.com/repos/vllm-project/vllm-ascend/releases` → HTTP 403(API rate limit / auth-required)
  - `https://github.com/vllm-project/vllm-ascend/releases` → 成功;v0.18.0 (2024-04-30) 仍是 "Latest" · 无 v0.18+ stable 发布(只有 v0.19.1rc1 2024-04-30 pre-release)
- 关键 finding:**vllm-ascend stable line 14 个月静默**(v0.18.0 2024-04-30 → 2026-05-21 现在) · chart default 翻到一个 14 个月没更新的 image 价值低
- 本地 docker pull 验证 image-pull access:**Blocked by C: 盘 100% full**(Phase 8 P8-T-002 同样的 disk constraint · `go clean -modcache` 待 user 手动跑)
- CI image-pull 验证 from GHA runners:**未实测**(同 Phase 7 T102 状态)

## Key decisions

- **Doc-only fallback over full flip**(同 Phase 7 P7-T-102 保守 posture):
  - 价值:operators 看 DESIGN.md §5.0.2 + known-issues #13 立即明白 P8-T-004 re-verify outcome + 怎么手动 opt in
  - 风险:零(无 chart values 改动 · 无 default 行为变化 · 既有 buildPDPairContainers tests + effectiveSchedulerName tests 全 PASS at Phase 7 baseline)
  - Phase 10 demo polish 时 chart default 翻转 = 1 行改动 + 实测 image pull · 该工作量 dispatch to Phase 10
- **Phase 8 conservative inheritance**:P8-T-002 用户 mandated "stay K8s 1.32" 决策的 spirit 是降低本期风险;ProxyImage flip 同样 inherits 该 posture
- **known-issues #13 新建**(not 更新 #12)— ProxyImage 与 NumaAffinity 是不同 axis 的 carry-forward · 各自独立 issue · 便于 Phase 10 demo polish 时分别 verify
- **No code change**:plan §3-T004 doc-only fallback Acceptance "No source code change; busybox fallback path preserved" 100% 兑现 · `git diff --stat` 输出仅 3 docs files

## Verification

- **新 finding**(本 task entry sanity scan):`deploy/helm-charts/inference-operator/values.yaml` 实际**无 `defaults.proxyImage` 字段**(只有 `proxyImage` 在 bundled CRD schema 内做 user-settable field doc · `crds/modelservices.yaml:140`)。这意味着 plan §3-T004 "full flip" 路径预设了 chart `defaults.proxyImage` 字段已存在 · 实际不存在。Phase 10 demo polish 真要 flip 时,**必须先 ADD `defaults.proxyImage` value field + 在 deployment_builder 或 template 内 wiring · 才能 flip default**(原计划"1 行改 + CI 实测"低估了实际工作量)。本 finding 已记入 known-issues #13 "Proposed resolution" 的 step 0 隐含工作 · 不影响 doc-only T004 本身的 outcome
- **存在性**:
  - `operators/inference-operator/DESIGN.md` §5.0.2 新段 · grep "Phase 8 P8-T-004" 命中 ✅
  - `docs/known-issues.md` #13 新条目 · grep "### #13" 命中 ✅
  - `docs/devlog/phase-8-t004.md` 本 file ✅
- **完整性**(plan §3-T004 doc-only fallback Acceptance vs 实际):
  - "Devlog enumerates: (a) verification attempt outcome (b) precise blocker (c) Phase 10 carry-forward rationale (d) chart default unchanged confirmation" → 本 devlog Debugging trail + Key decisions + Carry-forward 4 节均 cover ✅
  - "DESIGN.md §5.0.1 reflects carry-forward" → 实际加了 §5.0.2(并列段而非 §5.0.1 内追加 · Phase 7 §5.0.1 历史层级清晰)+ §5.0.2 显式 cross-ref 到 §5.0.1 ✅
  - "No source code change; busybox fallback path preserved" → `git diff --stat` 输出无 .go / values.yaml / template files ✅
  - "chart default unchanged confirmation" → DESIGN.md §5.0.2 表 + known-issues #13 显式 confirm 维持 empty ✅
- **正确性**:
  - `deploy/helm-charts/inference-operator/values.yaml` `defaults.proxyImage` 确认仍 empty(本 commit 不动 chart values 文件 · 状态 from Phase 7 P7-T-102)— 通过 file-state 隐含 verify
  - 仓库 .go / values.yaml / Deployment template 无改动 → Phase 5/6/7 tests 不需要 re-run(本 task doc-only · 不影响 binary 行为)
  - 与 Phase 7 T102 outcome 对齐:Phase 8 不"加码" · 不"reverse" · 只是 re-state + 加新 blocker · 历史轨迹清晰

## Carry-forward

- **Phase 10 demo polish** continues to own ProxyImage chart default flip:
  - 真硬件 lab 内 docker pull verify
  - GHA CI image-pull cache mount + retry policy
  - chart `defaults.proxyImage` 翻到 verified tag
  - 1 effectiveProxyImage 单元测试(同 Phase 6 T106 effectiveSchedulerName pattern)
  - Phase 10 kind smoke + lab smoke 双路径验证
- **operators 想跑 PD-pair proxy 今天**:`ms.Spec.PDPair.ProxyImage` 显式 set(deployment_builder.go::buildPDPairContainers 已 ship · Phase 6 T105 + Phase 7 T003 schedulerName auto-stamp 兼容)— 无需 chart 改动
- **upstream vllm-ascend stable release monitoring**:若 vllm-ascend 在 Phase 8 内突然发 v0.19+ stable(打破 14 月静默)· 可 mid-phase chat 启动 T004 重做(从 doc-only 升级到 full flip);default = stay doc-only
- **kind smoke phase{5,6,7,8} 继续用 FallbackImage=busybox:1.36 skip 真 pull**(Phase 6 T106 pattern · 已 ship at phase-7-complete · Phase 8 内 T103 phase8/install.sh 继承同模式)
