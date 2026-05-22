# P11-T-108 · [DECISION-GATED · 2nd attempt] Volcano gang-scheduling — 3rd defer Phase 12+

- **Commit**: (this commit)
- **Date**: 2026-05-22
- **Duration**: 0.2d plan / ~0.05d actual(default deferred path · 仅 ADR-0010 §未来扩展 row update + devlog)

## Intent

ADR-0017 §2 Decision A 明示 Volcano gang-scheduling 不在 Phase 11 spine。Phase 11 W2 entry chat 用户 "按计划执行" 未明示 training-job demo 需求 · per plan §4 P11-T-108 default policy "no training-job demo signal → 3rd defer Phase 12+" 命中。3rd consecutive defer(P9 1st · P10 2nd · 本 P11 3rd)与 lab gating 5th carry 同 spirit · 但 Volcano 与真硬件 milestone naming 解耦 · trigger 3 M5+ milestone reset 不适用本 carry。

## Path adaptations

无。Volcano carry 走 ADR-0010 §未来扩展 row update 路径(同 P9-T-101 + P10-T-108 pattern)· 不修 policy core。

## Debugging trail

无 build/test fail · 纯 docs 状态更新。

## Key decisions

- **default 3rd defer**(per ADR-0017 §2 Decision A 不在 Phase 11 spine + plan §4 P11-T-108 default policy)
- **`tests/e2e/kind/phase11/install.sh VOLCANO_ENABLED=1` 路径已 ship 备用**:operators mid-Phase 11 / Phase 12+ 若有 training-job demo signal · `helm install volcano-sh/volcano` 即装 · 不需要重写 install.sh
- **trigger 3 M5+ milestone reset 不适用本 carry**:Volcano 是 ecosystem extension(Spine C 候选 per ADR-0016 §4 (b))· 与 Spine A 真生产化 / Spine B 真硬件-native milestone naming 解耦 · 不强制 M5+/M6 milestone reset
- **Phase 12+ entry 评估**:per ADR-0017 §2 Decision C trigger 3 评估窗口同期 evaluate(training-job demo signal 是否 materialise · 若 materialise 则 light up Volcano · 否则继续 carry)

## Verification

- ADR-0010 §未来扩展 row Volcano 段加 2026-05-22 P11-T-108 3rd defer entry · 累计 3 次推迟 statement
- `tests/e2e/kind/phase11/assert.sh` T107-C2 conditional SKIP 默认正确(VOLCANO_ENABLED=0)
- `docs/devlog/phase-11-t108.md` 本文件 4-line rationale + ADR cross-ref

## Carry-forward

- **Phase 12+ entry meeting**:if training-job demo signal materialise · light up Volcano via `helm install`(VOLCANO_ENABLED=1 路径)· 否则继续 4th carry
- **P11-T-204 checkpoint stamp**:加 "T108 3rd defer Phase 12+ · no training-job demo signal" 行

## §0a 续 autonomous · 继续 T201 真 multi-cluster / multi-site demo
