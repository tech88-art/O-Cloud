# P11-T-F01..F04 · Frontend UX Track-2 sub-track — Carry to Phase 12+

- **Commit**: (this commit · post-tag phase-11-complete decision)
- **Date**: 2026-05-22
- **Duration**: 0d in Phase 11(carry posture activated per charter)

## Intent

Per `docs/devlog/phase-11-frontend-ux-track-charter.md` "conditional · carry to Phase 12 if not all land in Phase 11" 显式 carry posture:F01-F04 4-task sub-track 总计 12-16d frontend + 3-4d backend · 与 Phase 11 main scope T001-T204 已 5-6 weeks 工作量 + medium-high uncertainty profile 并行 capacity-dependent。本 Phase 11 main scope 已 ship 20/20 task chain + tag phase-11-complete · F01-F04 走 carry posture · 不阻塞 M5 真生产化 foundation subset milestone 关闭。

## Path adaptations

无 · 走 charter 已显式定义 carry path。

## Decision

**F01-F04 全 4 task carry to Phase 12+ first slot**(per charter "立 carry posture")。

- **F01** TopologyGraph G6 5.x 重写 + compound graph + 7 feature parity(~5-7d frontend)· `phase11-plan.md` §5.5 spec
- **F02** Deploy preset card 重设计(NPU dot + 动效)(~2d frontend)
- **F03** D6 NUMA+HCCS 对比专题 panel(~2-3d frontend + backend small)
- **F04** Backend `workload_*` histogram + counter emit · unblocks workload-business 全 panel + workload-resource 部分 panel(~3-4d backend)

## Rationale

- **Main spine 优先**:ADR-0017 §2 Decision A 三 主线(chart packaging spine + Karmada propagation 第一波 + Frontend src/ 3 indicators)已 ship · Track-2 是 UX polish 而非 spine deliverable · 与 main milestone close 解耦
- **G6 5.x 重写非 quick fix**:F01 5-7d 单 task 是 Phase 11 main 单 task(估 1-1.5d)的 4-5 倍 · 真 land 影响 Phase 11 calendar 收尾节奏
- **不阻塞 phase-11-complete tag**:per charter "不 over-commit · 4 task 都立 carry posture · phase-11-complete tag 后任一 task 仍可 carry to Phase 12 first slot"
- **充分 capacity 即可 land**:Phase 12+ entry meeting 起草时若 frontend agent 充分 capacity → F01-F04 grouped 优先入 Phase 12 first slot · 与 main M6 production hardening cohort 解耦不阻塞主线 spine 选择

## Action items

- **`docs/phase12-candidate-streams.md`** 加 F01-F04 carry section(本 task 一并 land · 见下方 commit)
- **Phase 12+ entry meeting agenda 补加**:"Frontend UX Track-2 F01-F04 land 决策(capacity-dependent · 优先入 first slot if agent 容量充分)"
- **F01-F04 spec 不动**(`docs/phase11-plan.md` §5.5 4 task spec 仍 valid · 留作 Phase 12+ first slot 起草时 reuse)

## Verification

- `docs/devlog/phase-11-frontend-ux-track-charter.md` "条件 carry" 显式 path 沿用 · 不破 charter 决策
- `phase-11-complete` tag 已 land at `8297bf7`(P11-T-204 commit)· F01-F04 carry 不在 tag 范围内 · 一致 with charter "不阻塞 milestone close"
- `docs/phase12-candidate-streams.md` 已加 F01-F04 carry-forward 表行(本 commit 同期 update)

## Carry-forward

- **Phase 12+ entry meeting**:F01-F04 land 决策 · 与 M6 production hardening cohort scope 选择联动 · 优先级 visual polish vs production hardening 取决于甲方 / 用户 input
- **如真 land in Phase 12**:F01-F04 走 `docs/phase11-plan.md` §5.5 spec(已 ship · 不需要重写 spec)· per-task Allowed Paths / Acceptance / Dependencies / Estimated effort 沿用
- **如继续 carry into Phase 13+**:不破 milestone narrative · F01-F04 是独立 UX track · 与 main spine 解耦

## §0a 续 autonomous · F01-F04 carry posture stamped · Phase 11 全 25 task chain 完成(20 main LANDED + 3 deferred + F01-F04 carry)· 等用户指明下一 step
