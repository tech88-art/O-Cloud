# P8-T-105 · [LAB-CONDITIONAL] Source.RealAscend body — deferred to Phase 10

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: plan 2-3d (lab) / 0.1d (deferred) · actual ~0.1d

## Intent

Phase 7 P7-T-101 carried Source.RealAscend body to Phase 10 per ADR-0011 §3 lab gating policy(default = defer to Phase 10 when no user lab signal at W2 entry)。Phase 8 P8-T-105 is the same task re-attempted at Phase 8 W2 entry · same gating policy applies。

## Lab gating outcome(2026-05-21 W2 entry)

User did **NOT** signal lab access available in Phase 8 calendar window during this execute session(chat 2026-05-21:user only discussed K8s 1.32 stay decision + go cache cleanup · no mention of Ascend 910B lab availability)。

Per ADR-0011 §3 decision matrix:
- 无明确信号(default)→ T105 **deferred to Phase 10**
- Checkpoint 记录"T105 deferred to Phase 10(no lab signal received in Phase 8 window)"

## Action

- **No source code changes**
- **No deploy / chart changes**
- **No CI workflow changes**
- This devlog file(3-line outcome record)
- T107 checkpoint table will记 T105 = "deferred Phase 10 · no lab signal · ADR-0011 §3 default"

## Carry-forward

- Phase 10 demo polish 期再次评估 lab gating · 同样政策 default = defer unless 用户显式 signal
- 实际 implementation(若 lab 可用):extend `operators/npu-dra-driver/internal/source/realascend/source.go` stub → real npu-smi parse · `npusmi/exec.go` harden · `tests/e2e/lab/phase8/install.sh` + `assert.sh` 实硬件 smoke。这些 path 暂不 ship
- ADR-0011 §3 政策保持 default = defer · Phase 10 后续 W2 entry meeting 重新 trigger evaluation
