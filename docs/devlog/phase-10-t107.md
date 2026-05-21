# P10-T-107 · kind smoke E2E Phase 10 extension · phase10/ folder + 2 active assertions + 3 SKIPPED conditionals(chart packaging deferred items skipped)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 多日 plan / ~0.2d actual(2 active asserts only · most Phase 10 substrates 不需要 real-cluster verify because chart packaging deferred)

## Intent

Phase 10 W2 kind smoke extension per plan T107 acceptance:install.sh + assert.sh 加 10+ assertions covering cache singleton + IMS 3 controllers + O2 DMS authn + Quota cluster-scope + Karmada propagation + ProxyImage chart smoke + 真硬件 fallback + Volcano conditional + Partitionable Devices conditional。

## Scope adaptation

Plan T107 envisions 10+ assertions · 实际 Phase 10 substrates 多为 unit-tested without chart packaging(per scope adaptation across T006-T101 + T103-T106):

**Active assertions(2)**:
- **T107-A1**:scheduler-plugin ConfigMap KubeSchedulerConfiguration contains `- name: NumaAffinity` in plugins.filter.enabled + plugins.score.enabled(per T005 NumaAffinity wrap default ENABLED)
- **T107-A2**:inference-operator chart values.yaml `defaults.proxyImage` field surface(per T106 chart-side default substrate)

**SKIPPED conditionals(3 task outcomes + deferred chart items)**:
- T102 Source.RealAscend(5th carry per ADR-0016 §2 Decision B)
- T108 Volcano gang-scheduling(2nd defer Phase 11+ · no training-job demo signal)
- T202 Partitionable Devices(KEP-4815 Beta only · defer Phase 11+)
- T006 demo-backend cache singleton chart(chart 不存 · Phase 11+ chart packaging)
- T007/T008/T101 IMS-1/2/3 charts(chart 不存 · 同 T006)
- T103 O2 DMS authn(substrate ready · chart wiring deferred to Phase 11+ wiring)
- T104 token-bucket(substrate ready · webhook integration deferred)

**Net active vs SKIPPED**:2 active(chart-level field/profile surface verify)+ 3-7 SKIPPED conditional(per task outcomes documented in respective devlogs)。

## Verification

- `phase10/install.sh` 可执行 · 走 scheduler-plugin chart upgrade + inference-operator chart upgrade(idempotent · existing chart state 不破)
- `phase10/assert.sh` 2 active asserts:NumaAffinity profile + ProxyImage field surface

Real-cluster end-to-end verify for chart-packaging-deferred items(demo-backend Lease leader-elect · IMS 3 controller reconcile · O2 DMS authn middleware · Quota webhook · cache cross-instance consistency)推到 Phase 11+ chart packaging stream(per ADR-0016 §3)。

## Per `feedback_post_tag_ci_gate`

Phase 10 tag `phase-10-complete` push 后:
- kind smoke phase5+ 走 K8s 1.34 baseline + NumaAffinity active · should PASS
- phase10/install.sh + assert.sh 2 active asserts · PASS expected
- 3 SKIPPED conditionals · echo + 0 exit · 不破 CI

可能 surface CI failures(post-tag fix-001 series 候选):
- K8s 1.34 DRA GA `resource.k8s.io/v1` 兼容性(chart 仍走 `v1beta1` · `v1beta1` 在 1.34 仍 served alongside `v1` · 但 CI 可能 surface deprecation warnings)
- helm chart kubeVersion range bump 漏 chart(其他 charts 可能 still pin `kubeVersion: ~1.32` · CI 可能 reject)
- scheduler-plugin chart upgrade after Phase 9 cluster state · 可能需要 reinstall(KubeSchedulerConfiguration NumaAffinity 注册 first time)
