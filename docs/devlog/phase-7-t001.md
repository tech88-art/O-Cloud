# P7-T-001 · ADR-0011 NPU 动态切分 + Source 接口 + Lab gating 政策

- **Commit**: (this commit)
- **Date**: 2026-05-20
- **Duration**: plan 0.5d / actual ~0.5d (main agent direct per §0a.11 例外 "写 ADR")

## Intent

锁定 Phase 7 的 3 个设计基石,让后续 14 个 task subagent 不需要在 task entry 时再就以下决策做澄清:

1. 动态切分:多模板组合 fallback 为 deliverable,driver-layer 突破是 opportunistic upside(arch §13 评审追加段第 8 行"Phase 7 启动前 ADR,明确触发 fallback 的条件" → 本 ADR 即该 ADR)
2. Source 接口契约 + factory + MockJSONSource 保留 / RealAscendSource stub 路径
3. Lab gating 政策:T101 + T104-real-cluster 默认 defer to Phase 10,unblock main agent 派发决策

## Path adaptations

- Plan §3-T001 Allowed Paths 列了 ADR-0009 "§6.4 migration table" cross-reference,但 ADR-0009 实际 §6 内无 §6.4 子节;按 plan intent(让 ADR-0011 在 ADR-0009 内被发现)在 §4 末 Partitionable Devices forward note 增 ADR-0011 cross-ref 段(同时也 cover plan 想要的 "Dynamic slice composition" 引用 — §4 内的 migration table 是最贴近的位置)
- ADR-0010 §7 forward notes 表的第 1 行 "Source.RealAscend.queryTopology()" 是 ADR-0011 §2 Source 接口的灵感来源 — 在 §7 表前加 update note 引向 ADR-0011 §3 Lab gating 政策(plan 原文要求引 §"Lab gating policy",这里映射到 ADR-0011 §3)
- arch §13 评审段第 8 行 ("Phase 7 动态切分若 fallback ...") 替换为 "landed 2026-05-20 P7-T-001 → ADR-0011 ..." 实际状态;同 §13 review-table Phase 7 row 加 ADR-0011 引用尾注;§3.4 NPU/AI 运行时表后加 🆕 forward note 段(与现存 v2/v3/🎯 同列样式)

## Debugging trail

- 无 false start。ADR 体量约 220 行 · 一次成稿。
- 唯一犹豫:NPUSliceTemplate cluster-scoped vs namespaced。选 cluster-scoped(Phase 7) — 推翻条件留给 Phase 9 multi-tenancy(本 ADR §推翻条件第 4 行明示)。
- 第二点犹豫:FallbackStrategy enum 是 2 值 (`fixed-template-combination` / `refuse`) 还是 3+ 值。Phase 7 决 2 值 — 推翻条件留给 Phase 8 vertical scaling 引入新值的可能(第 5 行明示)。

## Key decisions

- **arch §13 评审 row 状态从 "Phase 7 启动前 ADR" 升级 "landed 2026-05-20 → ADR-0011"**:符合 P3 verify-before-claim — ADR 真的 landed 才升级(本 commit 同时落 ADR 主体 + arch 状态更新,无 "claim before exist" 漏洞)
- **NPUSliceTemplate Pod opt-in via label**(不入侵 ResourceClaim spec):Phase 5 既有 whole-NPU 路径完全保留,零回归 — 与 ADR-0009 §5 Phase 5 实施要点 5(claim_controller annotation→真分配)长期共存
- **Source 接口的 3 个方法精确对齐 ADR-0009 §2 表 + ADR-0010 §7 forward**:`List`(对接 publisher reconcile)/ `Watch`(对接 event-driven reconcile)/ `QueryTopology`(对接 pool-operator NPUPool.status.hccsTopology 聚合 · P6-T-003)
- **Lab gating 政策的 default = defer**:对齐用户的"实际执行模型"事实(W2 entry meeting 是 chat 内对话 · 用户不主动 signal lab access → 默认 defer)。政策固化在 ADR 后,T101 派与不派完全机械化判定,避免 main agent 反复澄清

## Verification

- 存在性:`docs/adr/0011-npu-dynamic-slicing-and-source-interface.md` 写入 220+ 行(包含 §上下文 / §决策 1-3 / §CRD schema / §后果 / §推翻 / §引用 7 段)
- 完整性:ADR-0011 §1 (NPU 动态切分) + §2 (Source 接口) + §3 (Lab gating) + §4 (NPUSliceTemplate schema) + §5 后果 + §推翻 + §引用 7 段完整;3 处 cross-ref edits 全部命中:
  - `docs/architecture.md` §13 评审追加 row "Phase 7 动态切分..." → 更新为 "landed 2026-05-20 P7-T-001 → ADR-0011" ✅
  - `docs/architecture.md` §13 风险表 Phase 7 row → 加 ADR-0011 尾注 ✅
  - `docs/architecture.md` §3.4 后 → 加 🆕 forward note 段 ✅
  - `docs/adr/0009-npu-dra-driver.md` §4 头 + §4 末 (Partitionable Devices forward) → 加 ADR-0011 cross-ref ×2 ✅
  - `docs/adr/0010-scheduler-plugin.md` §7 forward notes 表前 → 加 ADR-0011 update note ✅
- 正确性:grep `ADR-0011` 命中 5 处(本 ADR 自身 + arch 2 处 + 0009 1 处 + 0010 1 处);grep `0011-npu-dynamic-slicing-and-source-interface` 全 doc cross-ref 完整

## Carry-forward

- **T002** (NumaAffinity upgrade) 入口需 check sched-plugins v0.32.x GA status — 本 ADR 已 cover doc-only fallback 路径,T002 决策表见 ADR-0011 §推翻条件 + phase7-plan §3-T002 entry decision
- **T004** (Source interface refactor) 必须按 ADR-0011 §2 Go 接口契约逐字落地;3 个方法签名 + 包路径 (`internal/source/source.go` / `internal/source/mockjson/` / `internal/source/realascend/` / `internal/source/factory.go`) 在 ADR 中固化
- **T006** (NPUSliceTemplate CRD) 按 ADR-0011 §4 Go types 区块逐字落地;cluster-scoped + +kubebuilder marker 已写明
- **T007** (template engine) 按 ADR-0011 §1 fallback 机制段落落地;Engine.Validate + Engine.Decompose 输出 `FixedTemplateBundle{Items []{Template string; Count int}}` 是 T105 allocator 的输入契约
- **T101** (lab-conditional Source.RealAscend impl) 主 agent 在 W2 entry 必须读 ADR-0011 §3 决策矩阵决定 ship vs defer;default = defer + 政策机械化 → checkpoint 单行记录 outcome
