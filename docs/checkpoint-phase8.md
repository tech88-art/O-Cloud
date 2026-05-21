# Phase 8 checkpoint — Busy-idle 垂直伸缩 (重启切片 pattern) + NPUVerticalScaler

> **Date**: 2026-05-21 · **Tag**: `phase-8-complete`(lands at this commit chain head)· **Branch**: `dev`
>
> Phase 8 lands the M3 → M4 transition headline **busy-idle 垂直伸缩
> controller**(重启切片 pattern per ADR-0012)+ Phase 7 deferred body
> AllocateBundle controller wiring(T105-v2)+ HCCS placement hard-fail
> upgrade(T104-v2)。**3 个 Phase 7 deferred items**(NumaAffinity wrap
> + Partitionable Devices Beta + ProxyImage chart flip)inherit
> Phase 8 user-mandated conservative posture:**K8s 1.32 baseline stay**
> 决策(chat 2026-05-21)blocks all 3 unblock paths · 3 items collectively
> re-deferred to Phase 9-10 per plan §6 fallback。Volcano gang-scheduling
> spike docs-only landed for Phase 9 entry decision matrix。
>
> **K8s baseline outcome**: Phase 8 stays at K8s 1.32 per user decision
> (scheduler-plugin v0.32.0 · main modules v0.35.0 · kindest/node v1.32.0)。
> sched-plugins v0.35.x/v0.36.x 未发布 + kindest/node v1.36 不存在 双重 upstream lag.
>
> **Lab-gating outcome**: T105 deferred to Phase 10 per ADR-0011 §3
> default(no lab access signal in Phase 8 W2 entry meeting/chat)。
>
> **BETA-gating outcome**: T101+T102 deferred to Phase 10 per K8s 1.32
> stay + kindest/node v1.36 lag double-block。

## 1. Deliverables (15 / 15 statuses · 9 net-new + 4 doc-only + 2 deferred)

```
W1 Foundation (8 tasks)
├── ✅ P8-T-001  ADR-0012 Busy-idle 垂直伸缩 + NPUVerticalScaler CRD shape  (9b89ff9)
├── ⏳ P8-T-002  K8s baseline bump → doc-only refresh (user stay 1.32)     (84c5222)
├── ⏸ P8-T-003  NumaAffinity wrap upgrade → DEFERRED Phase 9 (T002 block)
├── ⏳ P8-T-004  ProxyImage chart flip → doc-only fallback Phase 10 carry  (e82819d)
├── ✅ P8-T-005  NPUVerticalScaler CRD types + scheme + samples            (646f5f9)
├── ✅ P8-T-006  Busy-idle metrics ingestor (Prometheus + Fake)            (b636133)
├── ✅ P8-T-007  NPUVerticalScaler controller (8-step reconcile)           (45775bc)
└── ✅ P8-T-008  AllocateBundle controller wiring (T105-v2 · 3 envtest)    (d0d1813)

W2 Polish + BETA-conditional + LAB-conditional + checkpoint (7 tasks)
├── ⏸ P8-T-101  [BETA] Partitionable Devices Beta → DEFERRED Phase 10 (K8s 1.32 + kindest/node lag)
├── ⏸ P8-T-102  [BETA] Partition-aware allocator → AUTO-DEFERRED Phase 10 (T101 dep)
├── ✅ P8-T-103  kind smoke E2E Phase 8 extension (4 fixtures + 6 asserts) (5bfc5ba)
├── ✅ P8-T-104  HCCS placement hard-fail upgrade (T104-v2 · ring valid)   (5cb461f)
├── ⏸ P8-T-105  [LAB] Source.RealAscend body → DEFERRED Phase 10 (no lab signal)  (988ec11)
├── ✅ P8-T-106  Volcano gang-scheduling spike (Phase 9 entry decision matrix)  (3a7d600)
└── 🟢 P8-T-107  this checkpoint + tag phase-8-complete                    (this commit)
```

**Legend**: ✅ landed clean(code or docs)· ⏳ doc-only fallback per plan-allowed gating · ⏸ deferred per gating policy · 🟢 this commit。

**Phase 8 W1**: 8 / 8 ship 文件(5 net-new code · 2 doc-only · 1 deferred per upstream lag)。

**Phase 8 W2**: 5 / 7 land(T103 + T104 + T106 net-new · T105 deferred · T101+T102 deferred per BETA gating)+ this checkpoint T107。

## 2. What's wired

```
                  Standard-K8s 1.32 cluster (kind smoke baseline preserved)
                        │
   ┌────────────────────┴───────────────────────────────────────────┐
   │                                                                │
   │  scheduler-plugin (Phase 6 binary · Phase 7 T008 chart default)  │
   │  - HCCSTopology Filter + Score · NumaAffinity placeholder · Binpack opt-in │
   │  - npu-scheduler profile (Pod opt-in via spec.schedulerName)    │
   │  - Phase 8 W2 SKIP: NumaAffinity wrap upgrade re-deferred Phase 9 │
   │                                                                │
   │  inference-operator (Phase 5+6+7 ship · Phase 8 P8-T-007 加)     │
   │  - ModelService controller (P5-T-007 + P7-T-003 schedulerName)  │
   │  - PD Router webhook (P5-T-103 · ADR-0008)                       │
   │  - **NEW Phase 8 P8-T-007**: NPUVerticalScalerReconciler       │
   │    · 8-step reconcile loop · PrometheusIngestor / FakeIngestor   │
   │    · patches ModelService.metadata.annotations[npu.huawei.com/   │
   │      slice-template] per ADR-0012 §5 mutation model              │
   │  - **NEW Phase 8 P8-T-006**: metrics ingestor (HTTP /api/v1/query)│
   │    · NoData semantics fold connection / 5xx / empty into stay   │
   │                                                                │
   │  npu-dra-driver (Phase 4-7 ship · Phase 8 P8-T-008 加)          │
   │  - claim_controller (P5-T-002 + Phase 5/6 audit)                │
   │  - **NEW Phase 8 P8-T-008**: reconcileBundlePath dispatch       │
   │    · annotation `npu.huawei.com/slice-template` → Get NPUSliceTemplate│
   │    · Engine.Decompose → AllocateBundle (Phase 7 free function) │
   │    · write N AllocationResults + N audit objects                │
   │    · stamp `npu.huawei.com/preferred-hccs-ring` annotation       │
   │  - Whole-NPU path preserved (zero regression on Phase 5/6/7)     │
   │                                                                │
   └────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
           kind smoke E2E (Phase 4-8 layered · Phase 5→6→7→8 chain)
           - Phase 8 phase8/install.sh: apply NPUSliceTemplates +
             ModelService (annotation seeded) + NPUVerticalScaler +
             stamp claim annotation workaround (T008 wiring demo)
           - Phase 8 phase8/assert.sh: 6 assertions · 4 hard + 2 SKIPPED
             (NumaAffinity per T002 · Partitionable Devices per T101)
           - T104-v2 hard-fail upgrade: ring value validation ∈ {0,1,2,3}
```

**3 schedulers + 3 operators co-existence** preserved unchanged from Phase 7:
- default-scheduler / npu-scheduler / (volcano TBD Phase 9 per T106 spike)
- pool-operator / npu-dra-driver / inference-operator(+ NPUVerticalScalerReconciler co-located)

## 3. Test posture (per surface)

| Surface                            | Phase 5/6/7 baseline | Phase 8 adds                          | Total           |
|-----------------------------------|---------------------|---------------------------------------|-----------------|
| inference-operator/api/v1alpha1   | ModelService 4 cases | NPUVerticalScaler 4 cases             | **8 cases**     |
| inference-operator/internal/metrics | metrics 4 cases    | Ingestor 7 cases (5 + 2 bonus)       | **11 cases**    |
| inference-operator/internal/controller | ModelService 45+ cases | NPUVerticalScaler 6 cases       | **51+ cases**   |
| inference-operator/internal/webhook | PDRouter cases     | unchanged                             | preserved       |
| npu-dra-driver/internal/allocator | 4 AllocateBundle + Phase 5/6 | unchanged (T008 reuses Phase 7) | preserved       |
| npu-dra-driver/internal/controller | 11+ cases          | 3 BundlePath cases (Single / Multi / Rollback) | **14+ cases**   |
| npu-dra-driver/internal/template  | Engine 6+ cases     | unchanged                             | preserved       |
| **Total new Phase 8 tests**       |                     | **4 + 7 + 6 + 3 = 20 new cases**     |                 |

All Phase 5/6/7 既有 tests **preserved no regression**(verified via
`go test ./...` PASS for inference-operator + npu-dra-driver at each
T005-T008 commit edge)。

**helm lint --strict** clean on inference-operator + npu-dra-driver charts(P8-T-006 metrics.prometheusURL env injection · P8-T-007 NPUVerticalScaler RBAC additions · 都通过)。

**kind smoke phase8/install.sh + assert.sh**:
- `bash -n` syntax check PASS
- yaml safe_load PASS on 4 fixtures + workflow yaml
- CI 实证 carry-forward at this commit push(workflow runs phase8 step in chain alongside Phase 5/6/7)

## 4. DoD reconciliation

### W1 Foundation

- [x] ADR-0012 Busy-idle 垂直伸缩(`docs/adr/0012-busy-idle-vertical-scaler.md`)— 重启切片 pattern + NPUVerticalScaler CRD shape(`inference.ocloud.edge.example.com/v1alpha1`)+ 5-step mutation model + Forward notes for Phase 9-10 · cross-refs to ADR-0009 / 0010 / 0011 / architecture.md
- [x] K8s baseline bump(T002)→ **doc-only refresh per user "stay K8s 1.32" decision**(2026-05-21 chat)· ADR-0010 §1 加 2026-05-21 P8-T-002 update segment(upstream findings + 用户决策 + Phase 8 影响)· known-issues #12 status header refresh + 顶段加 P8-T-002 update segment · Phase 9 W1 entry 重新评估 path
- [x] NumaAffinity scheduler-plugin upgrade(T003)→ **deferred to Phase 9**(T002 block · sched-plugins v0.32.x apimachinery v0.32.x `pkg/api/{safe,operation,validate}` 包缺失 + 用户 stay 1.32 双重 block)· known-issues #12 OPEN maintained
- [x] vllm-ascend ProxyImage chart flip(T004)→ **doc-only fallback Phase 10 carry**(vllm-ascend 14 个月 stable 静默 + chart values.yaml 无 defaults.proxyImage 字段 · plan 假设字段不存在 · Phase 10 真 flip 时需先 ADD value field + template wiring + verified tag pin)· DESIGN.md §5.0.2 + known-issues #13(new)
- [x] NPUVerticalScaler CRD(T005)— types + scheme + 4 round-trip tests + sample + chart bundle + `make manifests` clean(controller-gen v0.20.1)· 11 DeepCopy methods + 5 condition constants + 3 default constants(DefaultCooldownSeconds=600 · DefaultMetricWindowSeconds=300 · MaxScaleHistoryEntries=10)
- [x] Busy-idle metrics ingestor(T006)— PrometheusIngestor + FakeIngestor + WindowedAverage + 5 ingestor tests + 2 bonus(interface assertion + helper edge)· NoData semantics fold conn/5xx/empty/empty-URL · DESIGN.md §5.3
- [x] NPUVerticalScaler controller(T007)— 8-step Reconcile per ADR-0012 §1 · annotation mutation per ADR-0012 §5 + 6 controller tests(NoData / Busy / Idle / Cooldown / FIFO / TargetNotFound)+ cmd/main.go register reconciler · chart RBAC + DESIGN.md §5.4
- [x] AllocateBundle controller wiring(T008)— reconcileBundlePath dispatch via claim annotation `npu.huawei.com/slice-template` + pickPreferredRing(`v1alpha1.AttrHCCSRing` qualified key)+ stampClaimAnnotation helper · 3 envtest-fake-client cases(SingleTemplate / MultiTemplateDecompose / OverCapacityRollback)+ whole-NPU path preserved + audit emit per Phase 5 pattern + DESIGN.md §3.10.1

### W2 Polish + BETA-conditional + LAB-conditional

- [x] Partitionable Devices Beta(T101)→ **deferred Phase 10**(K8s 1.32 baseline stay + kindest/node v1.36 not in kind v0.31 双重 block · 等 Phase 9 W1 entry 重新评估)
- [x] Partition-aware allocator(T102)→ **auto-deferred Phase 10**(T101 dep)
- [x] kind smoke E2E Phase 8 sub-job(T103)— 4 fixtures(busy + idle NPUSliceTemplate · ModelService qwen-pd · NPUVerticalScaler qwen-pd-scaler)+ install.sh 4 cmd functions + assert.sh 6 assertions(4 hard + 2 SKIPPED per W1 决策)+ workflow step + cluster-dump group
- [x] HCCS placement hard-fail upgrade(T104)— assert.sh T103-4 block 升级 `::warning::` → `::error::` + `exit 1` + ring value validation `case 0|1|2|3)` from set-b-multi-ring(Phase 7 reseeded)· T008 stamp 路径已 ship · 仅 verify upgrade
- [x] [LAB-CONDITIONAL] Source.RealAscend(T105)— **deferred Phase 10**(no lab access signal in Phase 8 calendar · ADR-0011 §3 default policy · 3-line devlog 记录)
- [x] Volcano gang-scheduling spike(T106)— spike doc 6 sections(PodGroup CRD shape · inference-vs-training scope · 3-scheduler coexistence · Phase 9 cost matrix 3 paths · HCCL topology constraints · refs)+ ADR-0010 §7 Volcano forward note refresh + arch §13 Phase 9 row cross-ref
- [x] `phase-8-complete` tag(本 commit)
- [x] `docs/checkpoint-phase8.md`(本文件)

### Out of scope(carried forward to Phase 9+)

- [ ] Karmada multi-site federation + multi-tenant quota controller — Phase 9(reads NPUSliceAllocation Phase 5 substrate + NPUVerticalScaler Phase 8 status for fair scaling)
- [ ] O2 DMS adapter(K8s Profile)— Phase 9 per arch §1.3
- [ ] Volcano gang-scheduling **implementation** — Phase 9(T106 spike landed Phase 8 · implementation Phase 9 training-job scenario · 路径 A 推荐)
- [ ] Custom-metric(Prometheus PromQL)extension for NPUVerticalScaler — Phase 9(Phase 8 ships NPUUtilization built-in only · `MetricType` enum 已预占 forward-compat)
- [ ] **真实硬件对接 + 演示打磨**(full real-hardware demo polish)— Phase 10 per arch §1.3(T105 lab smoke deferred to Phase 10)
- [ ] **Partitionable Devices GA migration**(T101+T102)— Phase 10 if KEP-4815 GA confirmed in K8s 1.37+ AND kindest/node v1.36+ released
- [ ] Live migration of HCCL ranks / per-Pod RDMA bandwidth quota — CNI-level gaps per `docs/cni-hccl-research.md` §5 · Phase 8 重启切片 sidesteps this · live migration Phase 11+ if at all
- [ ] Fabric discovery via LLDP / SONiC API — defer per ADR-0007 + ADR-0010 §229 to Phase 9+ on real switching gear
- [ ] **NumaAffinity wrap upgrade** — Phase 9 W1 entry re-evaluate(known-issues #12 OPEN · 路径 = sched-plugins v0.35+/v0.36+ if released + kind v0.32+/v0.33+ if released)
- [ ] **ProxyImage chart default flip** — Phase 10 demo polish(known-issues #13 OPEN · 路径 = ADD chart `defaults.proxyImage` value field + template wiring + docker pull verify + GHA CI image-pull cache mount + 1 effectiveProxyImage 单元测试)
- [ ] **deployment_builder annotation→Pod label propagation**(T007 carry-forward · Phase 8 polish task or Phase 9):take annotation `npu.huawei.com/slice-template` from ModelService → stamp Pod label · removes install.sh `cmd_demo_bundle_path` direct claim annotation workaround in kind smoke

## 5. Known issues (rollup)

| #    | Title                                                                       | Severity | Status / Resolution                                                       |
|------|-----------------------------------------------------------------------------|----------|---------------------------------------------------------------------------|
| #11  | scheduler-plugin schedulerName opt-in                                       | low      | **RESOLVED** Phase 7 P7-T-003(deployment_builder auto-stamp)              |
| #12  | NumaAffinity plugin wrap re-deferred Phase 7 → 8 → 9                       | low      | OPEN · P8-T-002 user stay K8s 1.32 decision · Phase 9 W1 entry re-eval     |
| #13  | vllm-ascend ProxyImage chart default re-deferred Phase 7 → 8 → 10           | low      | OPEN(new in Phase 8 P8-T-004)· Phase 10 demo polish 路径 ADD chart value field + verify  |

No net-new ⊕ Phase 8 specific OPEN issues beyond #12 / #13 extensions and #13 new entry。

## 6. Phase 9 handoff brief

**Phase 9 candidate workstreams**(从 most-actionable 排序):

1. **Volcano gang-scheduling 引入**(per T106 spike recommendation · 路径 A · 1-2d):安装 Volcano binary 独立 helm · 训练 Pod opt-in via `schedulerName=volcano` + PodGroup atomicity · npu-scheduler 继续 own NPU device 级 Filter+Score
2. **Multi-tenant fair scaling policy**(读 NPUVerticalScaler.status + 新 Quota CRD):Phase 8 NPUVerticalScaler.status.scaleHistory + status.lastScaleTime 已为 multi-tenant 提供数据 · Phase 9 加 Quota CRD + admission webhook 限定 per-namespace scaling rate / template ref
3. **O2 DMS adapter**(K8s Profile · per arch §1.3 Phase 9 row):北向 O2 IMS R1 接口 · 把 ocloud 资源(NPUSlicePool / ModelService / NPUVerticalScaler)reflect 到 O-RAN 北向 API · 涉及 backend / inference-operator status reflection
4. **K8s baseline bump 1.32 → 1.34/1.35**(per P8-T-002 user decision "stay 1.32" 等 Phase 9 W1 re-evaluate):
   - 上游 condition check:sched-plugins v0.35+/v0.36+ released? · kindest/node v1.36+ released?
   - 若条件满足:NumaAffinity wrap upgrade(known-issues #12 closer)+ Partitionable Devices Beta 重新 evaluate
   - 若条件不满足:维持 1.32 一个 Phase · Phase 10 again re-eval
5. **PromQL custom metric extension**(per ADR-0012 §7 forward note):`MetricSpec.Type` 加 `PrometheusQuery` enum 值 · Ingestor 加 PromQL-mode 路径 · 不破 NPUUtilization 现有路径 · 实现 ~0.5d
6. **deployment_builder annotation propagation**(per T007/T008 carry-forward):取消 phase8/install.sh `cmd_demo_bundle_path` claim annotation workaround · 走自然 propagation chain
7. **HCCS placement Pod-side annotation**(per T008 mutation adaptation forward note):Pod annotation `preferred-hccs-ring`(vs claim-side · Phase 8 现 ship claim-side)· 若 lab smoke 显示真硬件需要 Pod-level visibility

**Phase 9 W1 entry meeting agenda**(同 phase8-plan §6 spirit):
1. K8s baseline bump 决策(stay 1.32 vs bump 1.34/1.35)· user signal + upstream condition check
2. Volcano gang-scheduling 引入决策(per T106 spike · 路径 A vs C)
3. Multi-tenant Quota CRD shape design
4. O2 DMS adapter scope
5. Lab access signal for Phase 9 lab smoke(per ADR-0011 §3)

**Phase 8 vs Phase 9 boundary 确认**:Phase 8 = 单租户 busy-idle scaling + AllocateBundle wiring + 3 doc-only deferred items · Phase 9 = 多租户 fair scaling + Karmada + O2 DMS + Volcano + PromQL custom metric。

---

## 7. CI gate post-tag

Per memory `feedback_post_tag_ci_gate.md`(Phase 7 实战 4 fixes · 2026-05-21):**phase-N-complete tag push 后必看 GitHub Actions · 修所有 ❌ 直到 dev HEAD 全绿才算 phase 真完成**。

本 phase-8-complete tag push 后 main agent / next session 必须:
1. Watch GitHub Actions workflow runs on dev HEAD post-tag
2. 修 all ❌(potential fixes 列表):
   - phase8/install.sh + assert.sh CI 实际跑(本 commit 是 CI 首次跑 Phase 8 step)· 可能暴露 helm chart 路径 / fixture cross-ref 问题
   - inference-operator unit test workflow · 5 new tests / 9 new files in inference-operator chart bundle · 可能 lint issues
   - npu-dra-driver unit test workflow · T008 wiring · 3 new envtest cases · 应已 PASS at commit time
3. 直到 dev HEAD 全绿 → Phase 8 真完成

**预计 fix series**(if needed):P8-fix-001 ... P8-fix-NNN · 同 Phase 7 P7-fix-001..004 模式 · 直接 push dev(同 P7-fix series + 本 phase 各 task commit · 用户 chat 2026-05-21 "维持现状" 确认)。

---

**END of Phase 8 checkpoint**
