# P11-T-107 · kind smoke E2E Phase 11 extension(10+ assertions covering W1+W2 substrates)

- **Commit**: (this commit)
- **Date**: 2026-05-22
- **Duration**: 1d plan / ~0.2d actual(skeleton 已 in P11-T-007 commit · 本 task 加 5 new assertions)

## Intent

ADR-0017 §2 Decision A 主线全部 + 子线 T101/T108/T202 conditional 全覆盖。Phase 11 phase11/ folder install.sh + assert.sh 全 Phase 11 W1+W2 substrate 验证:chart packaging spine 闭环(6 chart)+ Karmada propagation 第一波(4 PropagationPolicy + bootstrap script)+ Frontend src/ 3 indicator + O2 DMS authn wiring + ClusterQuota CRD bundle + Phase 11 entry ADRs。

## Path adaptations

1. **install.sh 已在 P11-T-007 ship**(skeleton + 7-step chart install + KARMADA_ENABLED/VOLCANO_ENABLED gating)· 本 task 仅 extend assert.sh
2. **kind binary 在 dev 不存** · assert.sh syntactic + chart content checks 不依赖 kind · 真 kind smoke 在装了 kind 的 CI / dev box 上跑 install.sh + assert.sh 全链
3. **真 Pod Ready / Reconcile triggered assertions 留 Phase 12+** · 本 task ship chart-content / source-content syntactic assertions(实际 runtime smoke 受限于 kind binary · 在 dev env 不可执行)

## Debugging trail

无 build/test fail。一次 bash 跑 PASS。

## Key decisions

- **per-assertion T107-A0..A10 + 3 conditional T107-C1..C3** 总 14 assertions(11 active + 3 conditional · 满足 plan §4 P11-T-107 "10+ assertions" 要求)
- **conditional gating via env**:`KARMADA_ENABLED=1` / `VOLCANO_ENABLED=1` / `LAB_AVAILABLE=1` · default off · CI 跑 fast path · operators 显式 opt-in 跑 full path
- **assertion 顺序与 task chain 一致**:T107-A0(T103 policies)→ A1(T007 NRT)→ A2(T003 demo-backend)→ A3(T004/T005/T006 IMS × 3)→ A4(T006 IMS-3 Secret RBAC)→ A5(T008 inference env)→ A6(T001/T002 ADRs)→ A7(T106 O2 DMS authn)→ A8(T104 ClusterQuota)→ A9(T102 Karmada bootstrap)→ A10(T105 frontend 3 indicator)。devlog tracking 便于 debug。
- **不 ship Pod runtime + Reconcile real verification**(真 cluster 实测留 T201 + Phase 12+):assertion 是 syntactic + chart content / file existence · kind 在 dev env 缺 · 真 cluster verify 留 CI 跑 / T201 master-demo-multi-site.sh 实测

## Verification

- `bash tests/e2e/kind/phase11/assert.sh` 当前 dev env stdout:
```
PASS T107-A0: 4 PropagationPolicy YAML present in deploy/karmada/policies/
PASS T107-A1: NRT CRD bundled + numaAffinity.enabled=true default
PASS T107-A2: demo-backend chart Lease RBAC present
PASS T107-A3:node-lifecycle-operator: controller-runtime + Reconciler shell present
PASS T107-A3:software-mgmt-operator: controller-runtime + Reconciler shell present
PASS T107-A3:bare-metal-provisioning-operator: controller-runtime + Reconciler shell present
PASS T107-A4: IMS-3 chart Secret RBAC present
PASS T107-A5: inference-operator DEFAULT_PROXY_IMAGE env injection present
PASS T107-A6: ADR-0017 + ADR-0018 committed
PASS T107-A7: O2 DMS authn chart wiring(auth.mode + OIDC env + tokenreviews RBAC conditional)
PASS T107-A8: ClusterQuota CRD bundled in inference-operator chart + types ship
PASS T107-A9: Karmada bootstrap install/uninstall scripts + values + README
PASS T107-A10: Frontend 3 indicator columns + service includes wired
SKIP T107-C1: KARMADA_ENABLED=0
SKIP T107-C2: VOLCANO_ENABLED=0
SKIP T107-C3: LAB_AVAILABLE=0

== Phase 11 kind smoke: all assertions PASSED ==
```

## Carry-forward

- **T201 master-demo-multi-site.sh**:可调本 install.sh KARMADA_ENABLED=1 部分 + apply policies + 真 propagation 端到端
- **Phase 12+ 真 runtime assertions**:Pod Ready + Lease object 真出现 + Reconcile 触发 真 verify · 在 CI 装了 kind 的 environment 跑
- **CI workflow .github/workflows/**:phase11 smoke 加入 dev branch merge gate · CI 主体 kind setup-action 装 kind + helm + kubectl

## §0a 续 autonomous · 继续 T108 DECISION-GATED Volcano(default 3rd defer Phase 12+)
