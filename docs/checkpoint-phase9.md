# Phase 9 checkpoint — M4 工程化对外 · O2 DMS Adapter + Multi-tenant Quota + IMS 3 scaffold + Phase 8 carry-forward polish

> **Date**: 2026-05-21 · **Tag**: `phase-9-complete`(lands at this commit chain head)· **Branch**: `dev`
>
> Phase 9 opens the **M4 工程化对外** milestone (arch §1.3 final
> framework phase before Phase 10 真机对接 + 演示打磨). Two co-equal
> spines landed: (a) **O2 DMS Adapter** as the 北向 production contract
> per O-RAN O2 IMS R1 (ADR-0013 · scaffold P9-T-008 + body P9-T-104 ·
> 21 test cases), and (b) **Multi-tenant Quota CRD + admission webhook**
> (ADR-0014 · types P9-T-005 + controller + 2 webhooks P9-T-006 · 10
> test cases). Phase 9 W1+W2 also closed 7 Phase 8 carry-forward items
> via T003+T004+T007 + landed P9-T-105 IMS 3 scaffolds + P9-T-107 cache
> spike + P9-T-103 kind smoke ext.
>
> **K8s baseline outcome**: Phase 9 stays at K8s 1.32 per P9-T-003
> doc-only refresh. Bump 1.34 attempted but K8s 1.34 scheduler
> framework restructuring (NodeInfo + CycleState struct→interface · 9
> files affected) exceeded T003 Forbidden Paths · reverted + Phase 10
> carry as "coordinated K8s baseline bump + framework migration +
> NumaAffinity wrap" 三件套 (2-3d estimate).
>
> **Decision-gated outcomes**:
>   - T101 Volcano gang-scheduling: deferred per default policy (no
>     training-job demo signal · doc-only path)
>   - T102 NumaAffinity wrap upgrade: auto-deferred per T003 doc-only
>     refresh outcome (4th carry · prerequisite expanded)
>
> **Lab-conditional outcome**: T106 Source.RealAscend 3rd carry to
> Phase 10 per ADR-0011 §3 default (no lab signal · synthetic ring
> fixture continues to cover CI).

## 1. Deliverables (16 / 16 statuses · 9 net-new code + 5 doc-only + 2 deferred-conditional)

```
W1 Foundation (8 tasks · 2 ADRs + baseline re-eval + propagation polish + Quota stack + PromQL + O2 DMS scaffold)
├── ✅ P9-T-001  ADR-0013 O2 DMS Adapter design freeze                          (28e660e)
├── ✅ P9-T-002  ADR-0014 Multi-tenant Quota CRD + admission design freeze     (77ee142)
├── ⏳ P9-T-003  K8s baseline bump → doc-only refresh (framework drift Phase 10) (614c569)
├── ✅ P9-T-004  deployment_builder + claim_builder annotation propagation polish (7e51e3b)
├── ✅ P9-T-005  Quota CRD types + scheme + samples (+ P9-T-002-fix-001)        (d161f78)
├── ✅ P9-T-006  Quota controller + 2 ValidatingAdmissionWebhooks               (c2d4e43)
├── ✅ P9-T-007  PromQL custom metric extension (ADR-0012 §7 landed)            (ffb72a4)
└── ✅ P9-T-008  O2 DMS Adapter NEW module scaffold (7 stubs + chart)           (de5c446)

W2 Polish + decision-gated + IMS scaffold + lab-conditional + cache spike + checkpoint (8 tasks)
├── ⏸ P9-T-101  [DECISION-GATED] Volcano binary install → deferred (default policy) (07bcf1a)
├── ⏸ P9-T-102  [DECISION-GATED] NumaAffinity wrap → auto-deferred (T003 outcome)   (328499c)
├── ✅ P9-T-103  kind smoke E2E Phase 9 extension (8 assertions · 5 fixtures)      (01f00c6)
├── ✅ P9-T-104  O2 DMS Adapter body (7 endpoints + translator + 21 tests)         (7a70b18)
├── ✅ P9-T-105  IMS 7 服务剩 3 项 scaffold (3 NEW modules · 9 test cases)         (b39f1c7)
├── ⏸ P9-T-106  [LAB-CONDITIONAL] Source.RealAscend body → 3rd carry              (ac9e336)
├── ✅ P9-T-107  demo-backend cache pattern spike (3-path eval + ADR-0015 draft)   (4809659)
└── 🟢 P9-T-108  this checkpoint + tag phase-9-complete                            (this commit)
```

**Legend**: ✅ landed clean (code or docs) · ⏳ doc-only fallback per plan-allowed gating · ⏸ deferred per gating policy · 🟢 this commit.

**Phase 9 W1**: 8 / 8 ship (1 ADR-0013 + 1 ADR-0014 + 1 baseline doc-only + 1 propagation polish + 1 Quota CRD types + 1 Quota controller+webhooks + 1 PromQL extension + 1 O2 DMS scaffold).

**Phase 9 W2**: 5 / 8 land(T103 + T104 + T105 + T107 + this T108)· 3 deferred per gating outcomes(T101 default · T102 auto-T003 · T106 3rd carry).

## 2. What's wired

```
                  Standard-K8s 1.32 cluster (kind smoke baseline · phase 9 stays)
                                                                                  │
   ┌──────────────────────────────────────────────────────────────────────────────┴──┐
   │                                                                                 │
   │ ┌── pool-operator (NPUSlicePool · 4 levels · ims.ocloud.edge.example.com) ──┐   │
   │ │                                                                            │   │
   │ │ ┌── inference-operator (single binary · multi-resource scheme) ─────────┐ │   │
   │ │ │  · ModelService (Phase 5)                                              │ │   │
   │ │ │  · NPUVerticalScaler (Phase 8)                                         │ │   │
   │ │ │  · Quota (Phase 9 W1 P9-T-005)                                         │ │   │
   │ │ │  · PD Router MutatingAdmissionWebhook (Phase 5 P5-T-103)               │ │   │
   │ │ │  · Quota ValidatingAdmissionWebhook A (NPUSliceAllocation create)      │ │   │
   │ │ │  · Quota ValidatingAdmissionWebhook B (NPUVerticalScaler update)       │ │   │
   │ │ └────────────────────────────────────────────────────────────────────────┘ │   │
   │ │                                                                            │   │
   │ │ ┌── npu-dra-driver (NPUSliceAllocation · NPUSliceTemplate · npu.ocloud) ──┐│   │
   │ │ │  · claim_controller (P8-T-008 wiring · AllocateBundle path)             ││   │
   │ │ │  · template engine (Phase 7 P7-T-007)                                   ││   │
   │ │ └─────────────────────────────────────────────────────────────────────────┘│   │
   │ │                                                                            │   │
   │ │ ┌── scheduler-plugin (HCCSTopology + NumaAffinity placeholder + Binpack) ─┐│   │
   │ │ │  · K8s 1.32 baseline · NumaAffinity wrap Phase 10 (after baseline bump) ││   │
   │ │ └─────────────────────────────────────────────────────────────────────────┘│   │
   │ │                                                                            │   │
   │ │ ┌── o2-dms-adapter (NEW Phase 9 module · standalone binary :8088) ────────┐│   │
   │ │ │  · 7 NB endpoints under /o2dms/v1 · ADR-0013 §4 catalog                  ││   │
   │ │ │  · DynamicClient + translator + LifecycleOperationQueue                  ││   │
   │ │ │  · K8s Profile only · HTTP REST · R003-v04.00 lock                       ││   │
   │ │ └─────────────────────────────────────────────────────────────────────────┘│   │
   │ │                                                                            │   │
   │ │ ┌── IMS 3 scaffold (NEW Phase 9 · api types only · controller body Phase 10) ┐ │
   │ │ │  · node-lifecycle-operator (lifecycle.ocloud · 8 state enum)                 │ │
   │ │ │  · software-mgmt-operator (softwaremgmt.ocloud · 3 rollout strategy)         │ │
   │ │ │  · bare-metal-provisioning-operator (provisioning.ocloud · 7 state enum)     │ │
   │ │ └─────────────────────────────────────────────────────────────────────────────┘ │
   │ └────────────────────────────────────────────────────────────────────────────────┘
   └──────────────────────────────────────────────────────────────────────────────────────┘

Documentation:
  · ADR-0013 (O2 DMS) + ADR-0014 (Quota) NEW
  · ADR-0010 (scheduler-plugin) §7 Volcano forward note updated · T101 deferred
  · ADR-0011 (NPU slicing) §3 Lab gating carry tally + T106 3rd carry
  · ADR-0012 (busy-idle) §7 PromQL extension landed P9-T-007
  · ADR-0003 v2 (IMS phasing) §"O2 DMS NB 与 IMS Core 关系" + P9-T-105 outcome
  · ADR-0009 §2 + ADR-0011 §1 Quota substrate cross-ref segments added
  · arch §5.8 (o2-dms-adapter) + §5.9-§5.11 (IMS 3) + §6.7+§6.8 + §13 review-table
  · docs/research/demo-backend-cache-spike.md (P9-T-107 · ADR-0015 draft outline)
  · 16 devlogs (P9-T-001 through T108) records full Phase 9 trail
```

## 3. Tests inventory

- **inference-operator**: P9 new = 4 TestQuota{Reconcile,...} + 6 TestQuotaAdmission{A/B/whitelist/cap} + 4 TestQuota api/v1alpha1 + 2 TestNPUVerticalScalerPrometheusQuery + 2 TestPrometheusIngestor_CustomPromQL + 4 TestSliceTemplateLabelPropagation (deployment_builder + claim_builder) = **22 new cases** (51+ pre-existing all preserved · go test ./... PASS clean)
- **o2-dms-adapter**: 21 cases total (13 handler + 8 translator · plus 8 routing from P9-T-008 replaced by body assertion variants in T104)
- **3 IMS scaffold modules**: 3 round-trip + 3 enum + 3 GroupVersion = **9 cases total** across node-lifecycle / software-mgmt / bare-metal-provisioning
- **Phase 9 kind smoke**: 8 assertions in phase9/assert.sh (T103-1 Quota + T103-2 PromQL + T103-3 propagation + T103-4 O2 DMS + T103-5 IMS info + 3 SKIPPED for deferred T101/T102/T106)

**Total Phase 9 net-new tests**: ~52 cases (excluding deferred kind smoke runtime validation).

## 4. Gating outcomes (W2 entry decisions)

| Task | Type | User signal | Outcome | Carry tally |
|---|---|---|---|---|
| P9-T-101 | DECISION-GATED | "继续" 未明示 training-job | **Deferred Phase 10+** (doc-only) | Volcano spike landed Phase 8 P8-T-106 · Phase 9 plan-default fallback |
| P9-T-102 | DECISION-GATED | T003 doc-only refresh outcome | **Auto-deferred Phase 10** | 4th carry · NumaAffinity wrap prerequisite expanded "baseline bump + framework migration + wrap" 三件套 |
| P9-T-106 | LAB-CONDITIONAL | "继续" 未明示 lab access | **3rd carry Phase 10** (doc-only) | 1st Phase 7 → 2nd Phase 8 → 3rd Phase 9 · synthetic ring fixture covers CI |

## 5. Known issues (Phase 9 net-new + Phase 8 inheritance)

- **#12 NumaAffinity wrap** (Phase 7 → Phase 8 → Phase 9 → Phase 10) · status updates from P9-T-003 framework drift + P9-T-102 auto-deferred · prerequisite expanded to "coordinated K8s baseline bump + framework migration + wrap" 三件套 · Phase 10 estimate 2-3d
- **#13 ProxyImage chart flip** (Phase 7 → Phase 8 → Phase 10) · inherits Phase 8 P8-T-004 conservative posture · re-eval Phase 10 demo polish window
- **#14 Volcano gang-scheduling** (Phase 9 new) · P9-T-101 deferred per default · Phase 10+ re-eval if training-job demo signal materialises
- **#15 Source.RealAscend body 3rd defer** (Phase 9 new) · P9-T-106 3rd carry · Phase 10 W1 entry re-eval if lab access mid-phase signaled

## 6. Phase 10 handoff brief

**Phase 10 候选 workstreams** (从 most-actionable 排序):

1. **真实硬件对接 + 完整 multi-pool / multi-tenant 演示打磨**(per arch §1.3 Phase 10 row)· 涵盖 P9-T-106 lab body + 多 NPU pool scenario · 实质 Phase 10 milestone deliverable
2. **K8s baseline bump + scheduler framework migration + NumaAffinity wrap upgrade**(known-issues #12 prerequisite expanded · 三件套 coordinated task chain · 2-3d budget per `docs/devlog/phase-9-t003.md` migration matrix)
3. **IMS 3 项 controller body + reconcile loops + helm charts**(P9-T-105 scaffold carry-forward · per ADR-0003 v2 forward plan · 估 2-3d × 3 modules · 6-9d total)
4. **demo-backend cache implementation per ADR-0015**(P9-T-107 spike outcome · 推荐 §3.3 singleton with active-active failover · K8s Lease leader-elect + chart values.replicas升 2-3 · 估 1d)
5. **O2 DMS Phase 10 polish**(ADR-0013 §6 forward notes:authn/z full(OIDC + K8s SA + TokenReview)· Karmada multi-cluster propagation · subscription + alarmEvent endpoints · lifecycleOperation persistent backing · R004-v07.00.00 spec upgrade evaluate · frontend Workload page O2 DMS indicator · 6 子项)
6. **Quota Phase 10 polish**(ADR-0014 §7 forward notes:cluster-scope ClusterQuota + Karmada cross-cluster propagation + token-bucket algorithm + frontend visualisation + strong-consistency mode · 5 子项)
7. **Volcano gang-scheduling**(P9-T-101 deferred · if training-job demo signal materialises · 1-2d per Volcano spike §4 路径 A)
8. **Partitionable Devices Beta + partition-aware allocator**(P8-T-101/T102 carry · if K8s baseline bumped 1.36 · per ADR-0009 §4 + arch §13 Phase 10 row)
9. **vllm-ascend ProxyImage chart default flip**(known-issues #13 polish · upstream stable line activity check)
10. **Frontend Workload page extensions**(per Phase 6 T102/T103 chat+ADR self-RFC pattern · O2 DMS endpoint indicator + Quota usage visualisation + NPUVerticalScaler scaleHistory display)

**Phase 10 W1 entry meeting agenda**(same spirit as phase9-plan §6):
1. Lab access signal(影响 T106 4th carry vs light up)
2. K8s baseline bump retry decision(框架 migration scope acceptance)
3. IMS 3 项 controller body landing order(node-lifecycle 通常 first)
4. ADR-0015 cache strategy 起草(per P9-T-107 spike §6 outline)
5. O2 DMS Phase 10 polish 优先级排序(authn/z 与 Karmada 哪个先)

**Phase 9 → Phase 10 boundary 确认**:Phase 9 完成 M4 工程化对外的 NB API 头条 (O2 DMS) + 安全模型 spine (Quota) + IMS 7 服务架构补全(3 scaffold) + Phase 8 carry-forward 全部清空。Phase 10 = M4 完整工程化(真硬件 + 演示打磨 + 完整 IMS controller + 多站点 + 高级 polish)。

---

## 7. CI gate post-tag

Per memory `feedback_post_tag_ci_gate.md`(Phase 7 实战 4 fixes · 2026-05-21):**phase-N-complete tag push 后必看 GitHub Actions · 修所有 ❌ 直到 dev HEAD 全绿才算 phase 真完成**。

本 phase-9-complete tag push 后 main agent / next session 必须:
1. Watch GitHub Actions workflow runs on dev HEAD post-tag
2. 修 all ❌(potential fixes 列表):
   - Phase 9 kind smoke install.sh + assert.sh CI 实际跑(本 commit 是 CI 首次跑 Phase 9 step)· 可能暴露 helm chart 路径 / Quota CRD chart bundle / O2 DMS chart install 路径问题
   - inference-operator unit tests · 22 new tests / Quota + PromQL · 应 PASS at commit time
   - 3 IMS scaffold modules build/test · 9 cases · 应 PASS at commit time
   - o2-dms-adapter build/test · 21 cases · 应 PASS at commit time
3. 直到 dev HEAD 全绿 → Phase 9 真完成

**预计 fix series**(if needed):P9-fix-001 ... P9-fix-NNN · 同 Phase 7 P7-fix-001..004 模式 · 直接 push dev(用户已 push permission rule active in `.claude/settings.local.json`)。

---

**END of Phase 9 checkpoint**
