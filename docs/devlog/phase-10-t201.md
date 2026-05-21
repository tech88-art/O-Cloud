# P10-T-201 · master demo script · synthetic ring fallback path(T102 5th carry · arch §1.3 Phase 10 row 80% landed)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 多日 plan / ~0.2d actual(orchestrator script + devlog · 真硬件 path P deferred per T102 5th carry)

## Intent

arch §1.3 Phase 10 row "真实硬件对接 + 演示打磨" 实质 deliverable per ADR-0016 §2 Decision C 路径 F(synthetic ring fallback)。`tests/e2e/kind/master-demo.sh` orchestrator script 跑 Phase 5-10 install.sh + assert.sh 12 steps · 模拟 multi-pool / multi-tenant / multi-modelservice end-to-end flow。

## Path adaptations

- **Path P 真硬件 100% deliverable**:T102 5th carry → deferred Phase 11+(per ADR-0016 §2 Decision B trigger (1) Phase 11+ entry meeting 或 trigger (3) M5 milestone reset)
- **Path F synthetic ring fallback**:本 script ship · 80% landed(20% 缺真硬件 stamp)· checkpoint § Phase 10 row 标 "with synthetic ring fallback"
- **Recording fixture**:plan 写 "video vs YAML+kubectl script" 选项 · 实际选 shell script(可 piped to `tee` 或 `script(1)` capture · 不需要 video format · 同时是 CI runnable)
- **演示视频长度 SLA**:plan 写 "30s 滚动 vs 2-3min full flow" · 实际本 script run time ~5-10 min on local kind cluster · 不是 30s 短演示 · 适合 full-flow demonstration · 30s 截选版可 Phase 11+ 后续 work

## Master demo coverage

12 steps:
1. kind cluster up · K8s 1.34 baseline + DRA + scheduler plugins
2. Phase 5 install · npu-dra-driver + inference-operator + cert-manager
3. Phase 5 assert · ResourceClaim + NPUSliceAllocation lifecycle
4. Phase 6 install · scheduler-plugin HCCSTopology + Binpack
5. Phase 6 assert · HCCS ring filter + Binpack score + scheduler-plugin profile
6. Phase 7 install · NPUSliceTemplate + multi-template AllocateBundle
7. Phase 7 assert · NPUSliceTemplate reconciler + 多模板 fallback
8. Phase 8 install · NPUVerticalScaler busy-idle annotation patch
9. Phase 8 assert · scaler reconcile loop + scaleHistory ring buffer
10. Phase 9 install · Quota CRD + admission webhook + O2 DMS Adapter
11. Phase 9 assert · Quota over-cap rejection + O2 DMS NB endpoint
12. Phase 10 install + assert · NumaAffinity wrap + ProxyImage chart surface(SKIPPED conditionals echo · per T107)

## Carry-forward

- **Phase 11+ chart packaging stream**(per ADR-0016 §3 真生产化 spine)将 4 chart 联动(IMS-1/2/3 + demo-backend)· master-demo.sh 可扩 IMS controller body + cache singleton + token-bucket webhook 真集群 verify
- **T102 5th carry path**:Phase 11+ entry meeting trigger · 真硬件 stamp 可加 step 13 跑 `tests/lab/phase11/install.sh` + assert
