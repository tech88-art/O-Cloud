# Phase 10 checkpoint — M4 工程化对外 milestone CLOSER · 20-task chain · 真硬件 synthetic ring fallback path

> **Date**: 2026-05-21 · **Tag**: `phase-10-complete`(lands at this commit chain head)· **Branch**: `dev`
>
> Phase 10 closes the **M4 工程化对外** milestone(arch §1.3 final phase of project roadmap)along three co-equal主线 + 三条子线:
> - **主线 1** 完整 IMS 7 服务:Phase 9 P9-T-105 scaffold-only 3 modules → Phase 10 P10-T-007 + T008 + T101 controller body 全 land(3 pure-Go Reconcile substrates · 33 state transitions + 46 unit tests)。
> - **主线 2** 真实硬件对接:P7-T-101 + P8-T-105 + P9-T-106 lab-gating 4th attempt via T102 → **5th carry** outcome(per ADR-0011 §3 default policy · no user lab signal)· synthetic ring fixture 继续 cover CI + demo flow。
> - **主线 3** 完整 multi-pool / multi-tenant 演示打磨:T201 master-demo.sh 12-step orchestrator(synthetic ring fallback path per ADR-0016 §2 Decision C · 80% landed deliverable)。
> - **子线 1** Phase 9 carry-forward 全 close:T003+T004+T005 K8s baseline bump 三件套(known-issues #12 RESOLVED · 5-phase carry closer)+ T006 cache substrate(per ADR-0015)+ T106 ProxyImage chart flip(known-issues #13 RESOLVED · 5-phase carry closer)。
> - **子线 2** ADR-0013 / ADR-0014 forward notes 大部分 land via T103 + T104(authn substrate + token-bucket rate algorithm)· Karmada propagation 第一波 deferred Phase 11+ multi-site stream。
> - **子线 3** docs 大整理 + Phase 11+ 前瞻 via T203(arch §13 row promote + README current-phase + Phase 11+ candidate streams per ADR-0016 §3)。
>
> **3 deferred-by-default outcomes**:
> - T102 LAB-CONDITIONAL · 4th attempt outcome = **5th carry**(per ADR-0011 §3 default policy · no user signal · synthetic ring substrate covers · Phase 11+ entry meeting trigger ADR-0016 §2 Decision B re-evaluation)
> - T108 Volcano gang-scheduling · **2nd defer Phase 11+**(no training-job demo signal · default per Phase 9 P9-T-101 carry policy)
> - T202 Partitionable Devices · **defer Phase 11+**(KEP-4815 Beta only in K8s 1.36 · 1.34 baseline 不 unlock · doc-only refresh outcome)
>
> **2 RESOLVED known-issues**:#12 NumaAffinity wrap(5-phase carry closer at P10-T-005 · sched-plugins v0.34.7 nrt.New delegate)+ #13 ProxyImage chart flip(5-phase carry closer at P10-T-106 · EffectiveProxyImage helper + chart default field)。
>
> **K8s baseline outcome**:**K8s 1.32 → 1.34.3** lockstep(scheduler-plugin v0.32 → v0.34.7 · kindest/node v1.34.3 · option B per-module skew policy per ADR-0001 v3 §5)。

## 1. Deliverables(20/20 statuses · per task taxonomy)

```
W1 Foundation(8 tasks · 全 LANDED)
├── P10-T-001  ADR-0015 demo-backend cache strategy · ba26935
├── P10-T-002  ADR-0016 lab onboarding 4th attempt + Phase 11+ 前瞻 · 30a1c39
├── P10-T-003  K8s baseline bump 三件套 part 1(scheduler-plugin v0.32→v0.34.7
│              + kindest/node v1.32→v1.34.3 + option B per-module skew)· 8d70efa
├── P10-T-004  K8s baseline bump 三件套 part 2(framework migration · 9 files
│              NodeInfo/CycleState struct→interface · k8s.io/kube-scheduler/
│              framework data contract)· c833d6a
├── P10-T-005  K8s baseline bump 三件套 part 3(NumaAffinity wrap body ·
│              sched-plugins v0.34.7 nrt.New delegate · chart default ENABLED ·
│              known-issues #12 RESOLVED · 5-phase carry closer)· 5e01e23
├── P10-T-006  demo-backend cache singleton substrate per ADR-0015 §3.3(Lease
│              state machine + 3 new Prometheus metrics · chart packaging
│              deferred Phase 11+ stream)· 2656abe
├── P10-T-007  IMS-1 node-lifecycle controller body(12 transitions · 8 states ·
│              14 unit tests · pure-Go Reconcile · chart deferred)· d01a5c3
└── P10-T-008  IMS-2 software-mgmt controller body(3 rollout strategies · 15
                unit tests · chart deferred)· ed5e4fc

W2 Polish + lab-conditional + decision-gated + frontend + ProxyImage + kind smoke(8 tasks · 5 LANDED + 3 DEFERRED)
├── P10-T-101  IMS-3 bare-metal-provisioning controller body(13 transitions · 7
│              ProvisioningStates · 17 unit tests · chart deferred · 3 IMS
│              controller body 全 land)· 213e1a7
├── P10-T-102  [LAB-CONDITIONAL · 4th attempt]Source.RealAscend body
│              → **DEFERRED Phase 11+(5th carry)**· default policy(no lab
│              signal)· per ADR-0011 §3 + ADR-0016 §2 Decision B · 1338f19
├── P10-T-103  O2 DMS Phase 10 polish · authn substrate(3 Validator interface ·
│              OIDC + K8s SA + TokenReview · 10 unit tests · Karmada / subscription /
│              alarmEvent deferred Phase 11+)· 4268554
├── P10-T-104  Quota Phase 10 polish · token-bucket rate algorithm substrate
│              (6 unit tests · ClusterQuota + Karmada deferred Phase 11+)· 189fdb6
├── P10-T-105  [chat+ADR self-RFC]Frontend Workload ext · api-contract.yaml 3
│              GET fields(includeO2DMSExposed + includeQuotaUsage +
│              includeScaleHistory · frontend src/ + backend handler deferred
│              Phase 11+)· 9c27782
├── P10-T-106  vllm-ascend ProxyImage chart default flip · EffectiveProxyImage
│              helper + chart `defaults.proxyImage` field + 4 unit tests ·
│              known-issues #13 RESOLVED · 5-phase carry closer · fddcf1a
├── P10-T-107  kind smoke E2E Phase 10 ext · phase10/ folder + 2 active asserts
│              + 3+ SKIPPED conditionals(chart packaging deferred items)· 9ea7297
└── P10-T-108  [DECISION-GATED]Volcano gang-scheduling install body
                → **DEFERRED Phase 11+(2nd defer)**· default policy(no
                training-job demo signal)· per Phase 9 P9-T-101 carry policy ·
                1338f19(batch)

W3 Closer + Milestone Closer(4 tasks · 全 LANDED)
├── P10-T-201  真硬件 multi-pool / multi-tenant 演示打磨 · master-demo.sh 12-step
│              orchestrator(Phase 5-10 install + assert chain · synthetic ring
│              fallback path per ADR-0016 §2 Decision C · 80% landed · 20% 缺
│              真硬件 stamp T102 5th carry)· c7a0e1f
├── P10-T-202  [DECISION-GATED]Partitionable Devices Beta + partition-aware
│              allocator → **DEFERRED Phase 11+**· KEP-4815 Beta only in K8s 1.36
│              (not GA)· 1.34 baseline 不 unlock T202 cascade · 1338f19(batch)
├── P10-T-203  docs 大整理 + Phase 11+ 前瞻 · arch §13 Phase 10 row promote
│              "landed phase-10-complete" + README current-phase update · ADR
│              cross-ref + devlog index + timeline narrative per scope
│              adaptation(M4 价值聚焦)· 0f6dcd4
└── P10-T-204  Phase 10 checkpoint + tag phase-10-complete + M4 milestone
                CLOSER announcement(本文件 · 即 this commit)
```

**Tally**:20 / 20 statuses:13 net-new substrates(T001-T008 + T101 + T103-T107 + T201 + T203 + T204)+ 3 deferred outcomes(T102 + T108 + T202)+ 4 in-task ADR/known-issues updates(三件套 + 3 IMS + 2 RESOLVED known-issues)。

## 2. ADR forward note + status updates landed in Phase 10

- **ADR-0001 v3 §5** per-module K8s skew policy explicit(P10-T-003 update segment · 6 类 module 表 + backend KubeEdge lock + 4-point policy 解读 + Phase 11+ re-eval triggers)
- **ADR-0003 v2 IMS-1/2/3** controller body status flip "scaffold + controller body LANDED"(P10-T-007 + T008 + T101 batch update)
- **ADR-0009 §4** Partitionable Devices defer cross-ref(P10-T-202 · KEP-4815 Beta only)
- **ADR-0010 §1 三件套 part 1 + part 2 + part 3 update segments**(P10-T-003 + T004 + T005)· **§3** NumaAffinity status flip "P10-T-005 LANDED"
- **ADR-0010 §7** Volcano gang-scheduling status flip "2nd defer Phase 11+"(P10-T-108)
- **ADR-0011 §3** lab gating carry tally **4th entry**(P10-T-102 outcome · 5th carry · ADR-0016 §2 Decision B trigger cross-ref)
- **ADR-0013 §6** O2 DMS forward notes · authn substrate landed + Karmada propagation Phase 11+ defer(P10-T-103)
- **ADR-0014 §7** Quota forward notes · token-bucket substrate landed + ClusterQuota + Karmada Phase 11+ defer(P10-T-104)
- **ADR-0015** demo-backend cache strategy · Accepted + §3.1 Phase 10 W1 status update(substrate landed · chart packaging deferred Phase 11+)
- **ADR-0016** lab onboarding 4th attempt + Phase 11+ 前瞻 · Accepted · §3 Phase 11+ 8 candidate streams enumeration source-of-truth

## 3. Test posture summary

| Surface | Phase 10 tests added | 既有 baseline preserved |
|---|---|---|
| operators/scheduler-plugin | 4 NumaAffinity sanity(T005)| 既有 P6-T-002..T008 + P7-T-003 + P8-T-005 + T004 framework migration baseline · 全 PASS |
| operators/node-lifecycle-operator | 14 controller + state(T007)| 既有 3 api/v1alpha1 round-trip preserved |
| operators/software-mgmt-operator | 15 controller + rollout(T008)| 既有 3 api/v1alpha1 round-trip preserved |
| operators/bare-metal-provisioning-operator | 17 controller + state(T101)| 既有 3 api/v1alpha1 round-trip preserved |
| operators/o2-dms-adapter | 10 authn(T103)| 既有 P9-T-104 21 body tests preserved |
| operators/inference-operator | 6 token-bucket(T104)+ 4 EffectiveProxyImage(T106)| 既有 Quota + PromQL + deployment_builder + claim_builder tests preserved |
| backend | 5 cache singleton(T006)| 既有 Phase 3-4 LRU cache tests preserved |
| **Total new** | **75 unit tests** + 既有 全 preserved · `helm lint --strict` 5/5 charts clean | |

**kind smoke phase10/** ships 2 active asserts + 3+ SKIPPED conditionals · real-cluster end-to-end verify deferred to Phase 11+ chart packaging stream(per `feedback_post_tag_ci_gate` flow)。

## 4. Scope adaptations(透明 documented)

3 systemic path-vs-reality scope adaptations 跨多 task surfaced + documented in devlogs:

1. **Chart packaging deferred to Phase 11+ stream**(T006 + T007 + T008 + T101):demo-backend chart 不存 · IMS-1/2/3 charts 不存 · 创建 from scratch 是 Phase 11+ packaging spine 工作。Pure-Go Reconcile substrate ready · main.go controller-runtime manager wire deferred 同期。
2. **Plan-literal path drift**(T003 + T006):per-module K8s 版本 drift(v0.31-v0.36 across 7 modules)+ backend KubeEdge lock vs plan "lockstep no skew" 假定不符 · option B 务实 path 用 ADR-0001 v3 §5 explicit policy 替换 implicit "不动"。
3. **Pure-Go Reconcile pattern > ctrl.Reconciler interface direct**(T007 + T008 + T101):state-machine logic decoupled from controller-runtime wiring · 单元测试无 envtest 需求 · cross-controller awareness 方便 · Phase 11+ ctrl.Reconciler 实现可直接 import `ReconcileOnce` + `state.NextState`。

All 3 适配 path 在 respective task devlogs + ADR update segments 明示 · 不藏。Phase 11+ chart packaging stream(per ADR-0016 §3)将系统 close 这 3 个 deferred groups。

## 5. Verification + post-tag CI gate expectations

P3 三项验证维度 per task 全 PASS:
- **存在性**:每 task `git diff --stat` 实证(75 new tests + multiple ADR updates + 3 docs ADRs · ADR-0015 + ADR-0016 + 16 devlogs `phase-10-t001..t204.md`)
- **完整性**:Plan §3 + §4 task acceptance per-task 逐项核(完整 deliverable / deferred items documented in devlog · 不藏)
- **正确性**:`go build` / `go vet` / `go test` / `helm lint --strict` per task 真跑 · 既有 baseline tests preserved post-changes

**Post-tag `phase-10-complete` push 后 CI gate expectations**(per `feedback_post_tag_ci_gate.md`):
1. GitHub Actions workflows 全 trigger on dev HEAD + tag push
2. Phase 5-10 kind smoke 走 K8s 1.34 baseline · 主要 watch:
   - DRA `resource.k8s.io/v1beta1` 在 1.34 仍 served alongside `v1`(GA · 应兼容 · 但可能 deprecation warnings)
   - scheduler-plugin chart reinstall flow(NumaAffinity ConfigMap 注册 first time)
   - helm chart kubeVersion range bumps 漏 chart 可能 reject(逐 chart 评估)
3. 任 ❌ → P10-fix-NNN series · 同 P7-fix / P8-fix / P9-fix 模式 · 直接 push dev
4. dev HEAD 全绿 → Phase 10 真完成 → M4 milestone CLOSER

## 6. Phase 11+ handoff brief

**Phase 11+ chart packaging stream** primary work(per ADR-0016 §3 真生产化 spine):

1. **demo-backend helm chart 创建**(Chart.yaml + values + templates · 7+ files)+ cmd/main.go controller-runtime leader-elect wire(~80 lines · 调 Singleton.OnLeaseAcquired / OnLeaseLost callbacks)+ replicaCount: 2 default + Lease RBAC + readinessProbe
2. **node-lifecycle-operator chart**(同 demo-backend pattern · ~50 lines main.go + Dockerfile + RBAC + envtest)
3. **software-mgmt-operator chart**(同)
4. **bare-metal-provisioning-operator chart**(同 + Redfish/IPMI client + Secret 解析 username/password)
5. **inference-operator chart** template `DEFAULT_PROXY_IMAGE` env var injection(per T106 substrate · cmd/main.go `os.Getenv` 设 `controller.DefaultProxyImage`)
6. **O2 DMS authn chart wiring**(per T103 substrate · OIDC issuer config Secret + K8s SA TokenReview client-go wire)
7. **Karmada propagation 第一波**:Karmada control-plane 部署 + PropagationPolicy + cross-cluster informer aggregation(per ADR-0013 §6 + ADR-0014 §7 + ADR-0016 §3 Stream 1)
8. **Quota ClusterQuota CRD**:cluster-scope + Karmada cross-cluster usage 累计(per ADR-0014 §7 + ADR-0016 §3 Stream 1)
9. **Frontend src/ Workload page extension**(per T105 api-contract substrate · React/TS ECharts + AntD)

**Phase 11+ candidate streams enumeration**(per ADR-0016 §3 Stream 1-8):
1. 真实 multi-site / multi-机房 deployment(M5 主线候选)
2. Live migration of HCCL ranks + per-Pod RDMA bandwidth quota
3. Real fabric switch integration(SONiC / Cumulus / Arista API)
4. vLLM PD 分离 production-grade SLA(P99 + multi-tenant isolation)
5. OIDC IdP 完整集成(Keycloak / Dex 部署 + 真 federation)
6. KEP-4815 Partitionable Devices GA wait(if T202 deferred)
7. Volcano gang-scheduling install(if T108 2nd defer materialise)
8. O2 IMS R1 v05.00+ spec migration(if breaking · T103 defer carry)

**Phase 11+ entry meeting agenda**:
1. **Lab gating policy posture re-evaluation**(per ADR-0016 §2 Decision B trigger (1) · 5th carry 触发)· default-defer 是否翻转 · 或 M5+ milestone reset(trigger 3)将真硬件 onboarding 列为 prerequisite
2. **Phase 11+ primary spine 选择**(A 真生产化 / B 真硬件-native / C 生态扩展 · 三选一或拆 multi-phase)
3. **M5+ milestone naming**(per ADR-0016 §4 Open question (c))
4. **Chart packaging stream 优先级排序**(IMS-1/2/3 + demo-backend 4 chart 哪个先 land)

## 7. CI gate post-tag

Per memory `feedback_post_tag_ci_gate.md`(Phase 7 实战 4 fixes · 2026-05-21):**phase-N-complete tag push 后必看 GitHub Actions · 修所有 ❌ 直到 dev HEAD 全绿才算 phase 真完成**。

本 phase-10-complete tag push 后 main agent / next session 必须:
1. Watch GitHub Actions workflow runs on dev HEAD post-tag(17 commits 一次 push 触发)
2. 修 all ❌ via P10-fix-NNN series:
   - Phase 5-10 kind smoke 走新 K8s 1.34 baseline · 可能 surface DRA API deprecation / chart kubeVersion range / scheduler-plugin reinstall flow issues
   - 75 new unit tests · 应 PASS at commit time(已 strict verify pre-commit)
3. 直到 dev HEAD 全绿 → **Phase 10 真完成** → **M4 工程化对外 milestone CLOSER announcement** 真 land

---

**END of Phase 10 checkpoint**
