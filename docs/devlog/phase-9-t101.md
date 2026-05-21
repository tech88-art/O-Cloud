# P9-T-101 · [DECISION-GATED] Volcano binary install → doc-only deferred (default path)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 0.3d planned (doc-only) · ~0.1d actual

## Intent

Phase 9 W2 entry decision-gated Volcano gang-scheduling integration per P8-T-106 spike landing recommendation。Default policy(plan §4 P9-T-101)= **Deferred unless positive signal at W1 entry chat**。

## Decision

**Outcome**: **C · defer Phase 10+** (doc-only path)

**Rationale**:
- Phase 9 W1 entry chat(2026-05-21 plan execute session start):用户 "继 phase 9 plan,新开 session 执行任务" 未明示 training-job / gang 需求
- Phase 9 W2 entry chat(2026-05-21 同 session 继续):用户 "继续" 未明示 training-job / gang 需求
- Per spike doc §4 W1 entry decision matrix · "无明确信号(default) → C · defer" 路径
- Volcano upstream v1.10.x stable line 仍兼容 K8s 1.32 baseline(本 phase scheduler-plugin per P9-T-003 doc-only refresh outcome 维持)· helm install 路径 unchanged if Phase 10+ light up
- Phase 10 W1 entry 重新评估 if training-job demo signal materialises(70B model training scenario · multi-Pod HCCL collective atomicity 需要 gang)

## Re-WebFetch outcome (2026-05-21)

- Volcano upstream release tracker(github.com/volcano-sh/volcano/releases):v1.10.x stable line(同 Phase 8 W2 P8-T-106 spike findings)
- K8s 1.32 兼容:确认(P8-T-106 spike §1 PodGroup CRD shape · K8s 1.32 baseline 不 block Volcano install)
- 新增 v1.11.x or later 未发布(stable 维持 v1.10 line)

## Doc-only path execution

4 文件更新:
1. **ADR-0010 §7** Volcano forward note row · 加 "**2026-05-21 P9-T-101 deferred(doc-only path)**" 段(re-WebFetch outcome + default policy 触发 + Phase 10 re-eval 路径)
2. **docs/research/volcano-gang-scheduling-spike.md §4** cost matrix · 加 W1 entry decision matrix 表 "**P9-T-101 实际 outcome(2026-05-21)**" 行 · 推荐路径段加"**Phase 9 实际 outcome: C deferred per default policy**"
3. **docs/known-issues.md** 新 entry #14 "Volcano gang-scheduling carried to Phase 10+ (P9-T-101 doc-only deferred)" · severity low · status OPEN · cross-refs 4 文件
4. **docs/devlog/phase-9-t101.md**(本文件)· 记录 decision + rationale + Phase 10 re-eval trigger

**0 代码 · 0 helm install · CI no-op**。

## Verification

P3 三项验证维度:

| 维度 | 验证方法 | 结果 |
|---|---|---|
| **存在性** | git status · 4 文件 modified/new | ADR-0010 + spike doc + known-issues + devlog · 0 代码 modules touched |
| **完整性** | plan §4 P9-T-101 Acceptance(doc-only deferred 4 项) | 4/4 全覆盖:Devlog enumerates(a)W1 entry signal absence(b)re-WebFetch outcome(c)Phase 10 carry rationale(d)spike refresh outcome · ADR-0010 §7 status note refreshed with new deferred date · spike doc §4 cost matrix reflects deferred outcome · known-issues entry added |
| **正确性** | bi-directional cross-ref · ADR-0010 §7 ↔ spike §4 ↔ known-issues #14 ↔ devlog | 4 文件互相 cross-ref · 一致 deferred outcome |

P4 横向 grep `P9-T-101` 全仓库 → 命中本 commit 4 文件 · 一致

## Carry-forward

- **Phase 10 W1 entry re-eval**:if user signal training-job demo / gang 需求 · light up Volcano per spike doc §4 路径 A(独立 helm install 1-2d 工作量)
- **mid-phase 重启可能**:Phase 9 W2 内 if 用户 chat 突然给出 positive signal · re-open T101 with full install path · 本 doc-only commit 不阻止 mid-phase pivot
- **scheduler-plugin DESIGN.md §1 Co-existence model** 不动 · 当前仍 default-scheduler + npu-scheduler 双 scheduler · Phase 10 Volcano light up 时升级为 3 schedulers

---

**END of P9-T-101 devlog**
