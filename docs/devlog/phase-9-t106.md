# P9-T-106 · [LAB-CONDITIONAL] Source.RealAscend body → 3rd carry to Phase 10 (default · no lab signal)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 0.1d planned (deferred path) · ~0.1d actual

## Intent

Phase 9 W2 entry LAB-CONDITIONAL Source.RealAscend body per ADR-0011 §3 default policy。3rd carry to Phase 10 per default policy "无明确信号 → defer Phase 10"。

## Decision

**Outcome**: **deferred to Phase 10 (3rd carry)**

Phase 9 W2 entry chat 用户 "继续" 未明示 lab access available · per ADR-0011 §3 default policy 命中 → 3rd carry → Phase 10。

## Lab gating carry tally

| Phase / Task | Outcome | Carry # |
|---|---|---|
| Phase 7 P7-T-101 (2026-05-20) | deferred Phase 8 | 1st |
| Phase 8 P8-T-105 (2026-05-21) | deferred Phase 9 | 2nd |
| Phase 9 P9-T-106 (2026-05-21) | **deferred Phase 10** | **3rd** |

## Deferred path execution

3 文件更新:
1. **ADR-0011 §3** Lab gating 政策 · 加 **Lab gating carry tally** 表 + 2026-05-21 P9-T-106 update segment(3rd carry rationale + synthetic ring fixture path 仍 cover CI + Phase 10 W1 entry re-eval trigger)
2. **docs/known-issues.md** 新 entry #15 · Source.RealAscend body 3rd defer · severity low · status OPEN · cross-refs ADR-0011 §3 + phase9-plan §4 + devlog
3. **docs/devlog/phase-9-t106.md**(本文件)· 3-line decision + rationale

0 代码 · CI no-op。**Source 接口** (Phase 7 T004 · mockjson impl + factory) 已就绪 · RealAscend impl 待 Phase 10 lab signal light up。

## Phase 10 W1 entry re-eval

If lab access mid-Phase 10 signaled:
- light up `operators/npu-dra-driver/internal/source/realascend/source.go` impl
- npu-smi parser real-binary call + ResourceSlice attribute populate
- `tests/e2e/lab/phase10/` smoke 脚本(install + assert · real silicon required)
- `cann-driver-matrix.md` verified row(lab session date + CANN/driver model)
- ADR-0011 §3 carry tally update(landed Phase 10 SHA)

If 再 default no signal → 4th carry → Phase 11+。

---

**END of P9-T-106 devlog**
