# P11-T-202 · [DECISION-GATED · re-eval at W3 entry] Partitionable Devices Beta — deferred Phase 12+

- **Commit**: (this commit)
- **Date**: 2026-05-22
- **Duration**: 0.5d plan / ~0.05d actual(default deferred path)

## Intent

ADR-0017 §2 Decision A 不在 Phase 11 spine + plan §4 P11-T-202 "default reject if KEP-4815 still Beta or 1.36 not GA" 命中。Phase 11 W3 entry re-eval outcome = deferred Phase 12+:
- baseline K8s 1.34.3(Phase 10 P10-T-003 三件套 lock · 未升 1.36)
- KEP-4815 仍 Beta(Phase 11 W3 entry re-WebFetch outcome 一致 with Phase 10 W3 entry · 期间未 GA)

## Path adaptations

无。default deferred path · 仅 ADR-0009 §4 forward note update + devlog short rationale。

## Debugging trail

无。

## Key decisions

- **defer Phase 12+ · 与 K8s baseline bump trigger 联动**(per ADR-0016 §3 Stream 6):
  - Phase 12+ entry 若 K8s 1.36 GA + kindest/node 1.36 available + KEP-4815 GA → 同期评估升级
  - Phase 12+ entry 若上述条件未达 → 继续 carry · 与 Stream 6 cohort 一起
- **累计 2 次推迟**(Phase 10 P10-T-202 1st · 本 P11-T-202 2nd)· 与 K8s baseline bump trigger 联动 · 不与 lab gating 5th carry(ADR-0011 §3)/ Volcano 3rd carry(ADR-0010 §未来扩展)同一 policy(各自独立 trigger 条件)
- **本 task NOT trigger ADR-0016 §2 Decision B trigger 3 M5+ milestone reset**:Partitionable Devices Beta 与 milestone naming 解耦(是 K8s upstream cadence dependent · 不是 真硬件 / 真生产化 milestone gate)

## Verification

- ADR-0009 §4 forward note 加 2026-05-22 P11-T-202 update segment · 含 P10-T-202 1st defer + 本 P11-T-202 2nd defer trail
- `docs/research/k8s-partitionable-devices-spike.md` 不动(Phase 10 P10-T-202 W3 entry re-WebFetch outcome 仍 valid)· Phase 12+ 起草时再 re-WebFetch
- partition-aware allocator + ADR §4 升级路径 第 2 行 不 land · 路径 1(NPUSliceTemplate fallback)继续是 Phase 11+ 主路径

## Carry-forward

- **P11-T-204 checkpoint stamp**:加 "T202 2nd defer Phase 12+ · K8s 1.34 baseline 不 unlock · KEP-4815 仍 Beta" 行
- **Phase 12+ entry W1**:re-WebFetch KEP-4815 status + 评估 K8s 1.36 baseline bump cohort · 与 ADR-0016 §3 Stream 6 同期

## §0a 续 autonomous · 继续 T203 docs 大整理 + Phase 12+ 前瞻
