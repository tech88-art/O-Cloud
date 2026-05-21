# Phase 7 checkpoint — NPU 动态切分 (多模板组合 fallback) + Source 接口抽象 + lab-conditional 政策

> **Date**: 2026-05-20 · **Tag**: `phase-7-complete` (lands at this commit chain head) · **Branch**: `dev`
>
> Phase 7 lands the M3 milestone headline **NPU 动态切分** as the
> multi-template combination fallback per arch §13 + ADR-0011, plus the
> Source interface abstraction unblocking real-hardware integration in
> Phase 10. NumaAffinity upgrade re-deferred (transitive dep chain on
> K8s 1.32 baseline); ProxyImage flip deferred (GA confirmed but CI
> image-pull access unverified); Source.RealAscend body deferred to
> Phase 10 (lab access not signaled in W2 entry).
>
> **Lab-gating outcome**: T101 deferred to Phase 10 per ADR-0011 §3
> default policy (no lab access signal in Phase 7 W2 entry meeting).

## 1. Deliverables (15 / 15 statuses · 13 net-new + 2 doc-only + 1 deferred)

```
W1 Foundation (8 tasks · ALL LANDED)
├── ✅ P7-T-001  ADR-0011 NPU 动态切分 + Source 接口 + lab gating  (bd6f223)
├── ⏳ P7-T-002  NumaAffinity upgrade re-deferred → doc-only       (1334644)
├── ✅ P7-T-003  schedulerName=npu-scheduler auto-stamp (#11 RESOLVED) (f164c6f)
├── ✅ P7-T-004  Source 接口 refactor (mockjson + realascend + factory) (5b5c183)
├── ✅ P7-T-005  npu-smi / DCMI Go binding scaffold                 (a82fab7)
├── ✅ P7-T-006  NPUSliceTemplate CRD types + samples + chart bundle (07bda4b)
├── ✅ P7-T-007  Template engine + reconciler                       (b260884)
└── ✅ P7-T-008  HCCS Adjacency 910B 8-card default (ADR-0010 §256) (020400a)

W2 Polish + lab-conditional + checkpoint (7 tasks)
├── ⏸ P7-T-101  [LAB-GATED] Source.RealAscend impl → DEFERRED Phase 10 (no lab signal)
├── ⏳ P7-T-102  vllm-ascend v0.12+ ProxyImage flip → doc-only refresh (27fd1f5)
├── ✅ P7-T-103+T104  kind smoke ext + synthetic multi-ring fixture  (bf0c03c)
├── 🟡 P7-T-105  Allocator AllocateBundle (controller wiring → Phase 10) (2058f37)
├── ✅ P7-T-106  Partitionable Devices (KEP-4815) spike + Phase 8/10 cost matrix (8762d43)
└── 🟢 P7-T-107  this checkpoint + tag phase-7-complete             (this commit)
```

**Legend**: ✅ landed clean · ⏳ doc-only fallback per plan-allowed gating · ⏸ lab-deferred · 🟡 partial scope reduction · 🟢 this commit.

**Phase 7 W1 = 100% landed** (all 8 tasks ship code or docs per plan
acceptance, including T002's doc-only branch when sched-plugins
v0.32.x couldn't absorb transitive apimachinery deps under K8s 1.32
pin · per known-issues #12).

**Phase 7 W2 = 6 / 7 landed** + T101 deferred per ADR-0011 §3 lab
gating default policy. T105 controller wiring (Pod label → NPUSliceTemplate
lookup → AllocateBundle dispatch) deferred to Phase 10 demo polish
under M4 value-focus scope reduction (capability landed via
`AllocateBundle` free function · 4 unit tests · controller wiring has
no Phase 7 W1 caller to exercise).

## 2. What's wired

```
                  Standard-K8s 1.32+ cluster
                        │
   ┌────────────────────┴───────────────────────────────────────────┐
   │                                                                │
   │  scheduler-plugin (Phase 6 binary · Phase 7 T008 default flip)  │
   │  - HCCSTopology Filter + Score · chart default adjacency       │
   │    NOW renders ring-of-rings {0:[1,3], 1:[0,2], 2:[1,3], 3:[2,0]} │
   │    (vs Phase 6 empty default · ADR-0010 §256 closed)           │
   │  - DefaultAdjacency910B8Card() + BuildAdjacency() validator    │
   │    (chart pre-flight reject self-loop + negative + non-int)     │
   │  - NumaAffinity STAYS placeholder (T002 re-deferred per #12)    │
   │  - Score now 4-tier 100/70/30/0 (Phase 6 was binary 100/30)    │
   │                                                                │
   │  inference-operator (Phase 5+6+7)                              │
   │  - PD-pair Pod template now AUTO-stamped                       │
   │    spec.schedulerName="npu-scheduler" (P7-T-003 · #11 RESOLVED) │
   │  - SchedulerOverride *string opt-out field on ModelServiceSpec  │
   │  - kind smoke T103 HARD FAIL on absence (closes known-issues #11) │
   │  - ProxyImage chart default STAYS empty (P7-T-102 doc-only ·    │
   │    operators set explicit image tag like v0.18.0 · CI fallback  │
   │    busybox preserved)                                          │
   │                                                                │
   │  npu-dra-driver (Phase 4-7)                                    │
   │  - Source interface lifted into internal/source/ (P7-T-004)    │
   │      Source { List · Watch · QueryTopology }                   │
   │      + NodeDevices · Event · HCCSTopology · ErrNotImplemented  │
   │  - MockJSONSource (renamed Phase 4 SimulatorSource)             │
   │      bit-for-bit Phase 4-6 behavior preserved                  │
   │  - RealAscendSource Phase 7 W1 stub                            │
   │      List/QueryTopology return source.ErrNotImplemented        │
   │      lab-conditional Phase 10 body lights up via               │
   │      sourceType=real-ascend chart value                        │
   │  - Factory dispatch in cmd/main.go::selectSource                │
   │      flag --source-type (default mock-json)                    │
   │  - npu-smi parser scaffold (internal/source/realascend/npusmi/)│
   │      Client interface + FakeClient (embed testdata) + ExecClient│
   │      ParseTopoMatrix (BFS connected components on HCCS edges)   │
   │      8-card + 16-card testdata fixtures (5 parser test cases)  │
   │  - NPUSliceTemplate CRD (P7-T-006)                              │
   │      cluster-scoped · Composition + FallbackStrategy            │
   │      5 PartType enum (whole/vir04/vir08/vir16/dynamic-shard)    │
   │      2 FallbackStrategy enum (fixed-template-combination/refuse)│
   │      ConditionType Validated + Allocatable                      │
   │  - Template engine + reconciler (P7-T-007)                      │
   │      Engine.Validate (4 reasons) + Engine.Decompose             │
   │      Duplicate-Type merging + sorted FallbackAppliedReason      │
   │      NPUSliceTemplateReconciler stamps status conditions        │
   │      6 engine cases + 3 controller cases passing                │
   │  - Allocator AllocateBundle (P7-T-105 free function)            │
   │      nil bundle → Phase 5 whole-NPU path (zero regression)      │
   │      bundle → loops items × Count with cumulative AllocatedSet  │
   │      All-or-nothing rollback on any item failure                │
   │      4 unit tests covering nil / single / multi / over-cap     │
   │      Controller wiring deferred to Phase 10                     │
   │                                                                │
   │  kind smoke (Phase 6 T106 + Phase 7 T103+T104)                 │
   │  - reseed mock data with set-b-multi-ring (4 HCCS rings)       │
   │  - NPUSliceTemplate apply + wait Validated=True / Allocatable=True │
   │  - ModelService apply + assert schedulerName auto-stamp        │
   │  - HCCS adjacency chart default rendered in ConfigMap          │
   │  - ResourceSlices span 4 rings (T104 fixture)                  │
   │  - PD-pair placement soft warning (kind no real HCCS · Phase 10 lab) │
   └─────────────────────────────────────────────────────────────────┘
```

## 3. Test posture (per surface)

| Surface | New tests landed Phase 7 | Phase 4-6 tests preserved |
|---|---|---|
| operators/npu-dra-driver/api/v1alpha1 (NPUSliceTemplate) | 4 cases (round-trip / DeepCopy / omitempty / enum-pin) | NPUSliceAllocation 4 cases + AscendDevice round-trip preserved |
| operators/npu-dra-driver/internal/source/ | (interface package · no test files) | — |
| operators/npu-dra-driver/internal/source/mockjson/ | 4 source cases + 1 set-b-multi-ring case (5 total) | (SimulatorSource tests moved + preserved bit-for-bit behavior) |
| operators/npu-dra-driver/internal/source/realascend/ | 1 stub assertion | — |
| operators/npu-dra-driver/internal/source/realascend/npusmi/ | 5 parser cases (7 sub-cases with sub-tests) | — |
| operators/npu-dra-driver/internal/template/ | 6 engine cases | — |
| operators/npu-dra-driver/internal/controller/ (NPUSliceTemplate) | 3 reconciler fake-client cases | Phase 5+6 controller tests preserved |
| operators/npu-dra-driver/internal/allocator/ (AllocateBundle) | 4 bundle cases | Phase 5 Greedy + BestFit + Available tests preserved (Phase 5 5+ cases) |
| operators/inference-operator/internal/controller/ (effectiveSchedulerName) | 4 test cases | Phase 5+6 builder + controller + webhook tests preserved (18 + 18 + 4) |
| operators/scheduler-plugin/internal/plugins/hccs/ (adjacency) | 4 adjacency cases + 2 score-with-adjacency = 6 new | Phase 6 26 hccs cases preserved (32 total per plan T008 acceptance) |
| tests/e2e/kind/phase7/ | bash + yaml syntax clean · run via CI on next push | Phase 6 phase6/ assert.sh + install.sh preserved |
| operators/scheduler-plugin/ (T002 NumaAffinity) | (no change · doc-only · placeholder retained) | Phase 6 2 numa cases preserved |

**Local-env honesty (P3 disclosure)**:
- envtest binaries deferred to CI per existing Windows UAC limitation
  (consistent with Phase 5/6 checkpoints)
- kind smoke workflow NOT run locally; bash + yaml + json lint clean
- vllm-ascend image pull NOT verified (P7-T-102 doc-only outcome)
- npu-smi binary NOT exercised (Phase 7 W1 stub · P7-T-101 lab path
  deferred · ExecClient compiles + interface-asserts but doesn't run)
- AllocateBundle controller wiring NOT exercised (M4 scope reduction ·
  Phase 10 ships the wiring + 3 envtest cases per plan §3-T105 acceptance)

## 4. DoD reconciliation

Per `docs/phase7-plan.md §5 DoD`:

### W1 Foundation
- [x] ADR-0011 landed (T001 / bd6f223) — NPU 动态切分 fallback + Source 接口 + lab gating + 推翻条件 + cross-refs
- [⏳] T002 NumaAffinity — DOC-ONLY FALLBACK landed (1334644) · sched-plugins v0.32.7 IS GA but K8s 1.32 baseline pin cannot absorb transitive apimachinery v0.32.7+ packages along import chain · documented in DESIGN.md §5.2 + new known-issues #12 + ADR-0010 §3 status refresh · Phase 8 baseline bump candidate
- [x] T003 schedulerName auto-stamp (f164c6f) — `ModelServiceSpec.SchedulerOverride *string` + `SchedulerNameDefault const "npu-scheduler"` + effectiveSchedulerName resolution helper + 4 sub-tests covering 4-row table; known-issues #11 RESOLVED with historical narrative retained for raw-Pod outsiders
- [x] T004 Source interface refactor (5b5c183) — `internal/source/source.go` + `mockjson/` + `realascend/` + `factory.go` + cmd/main.go selectSource dispatch + 5 mockjson tests + 1 realascend stub test; Phase 4-6 publisher_test.go 5 cases pass unchanged
- [x] T005 npu-smi/DCMI binding scaffold (a82fab7) — `internal/source/realascend/npusmi/` Client interface + FakeClient (embed testdata) + ExecClient + ParseTopoMatrix + 5 cases / 7 sub-cases + 8-card + 16-card testdata fixtures; zero dep on real npu-smi binary
- [x] T006 NPUSliceTemplate CRD (07bda4b) — `api/v1alpha1/npuslicetemplate_types.go` + 4 round-trip tests + 2 samples (Qwen-PD + DeepSeek strict) + chart bundle in `deploy/helm-charts/npu-dra-driver/crds/`
- [x] T007 Template engine + reconciler (b260884) — `internal/template/` engine.go (Validate + Decompose + 4 ValidationError reasons) + types.go (FixedTemplateBundle) + 6 engine cases + `internal/controller/npuslicetemplate_controller.go` reconciler + 3 fake-client cases; cmd/main.go `--enable-template-controller` flag (default true)
- [x] T008 HCCS Adjacency 910B 8-card default (020400a) — `internal/plugins/hccs/adjacency.go` DefaultAdjacency910B8Card + BuildAdjacency strict validator + ErrMalformedAdjacency sentinel + 4 adjacency cases + 2 score-with-adjacency cases; chart values default flipped `{}` → 4-ring map; ADR-0010 §256 risk row status closed

### W2 Polish + lab-conditional
- [⏳] T102 ProxyImage flip — DOC-ONLY REFRESH landed (27fd1f5) · vllm-ascend v0.12+ IS GA (latest v0.18.0 2024-04-30) but CI image-pull access from GHA runners (~5GB · disk budget concern) + `quay.io/vllm-project/vllm-ascend` exact tag convention unverified; chart `defaults.proxyImage` STAYS empty; operators opt in via explicit `ms.Spec.PDPair.ProxyImage`. DESIGN.md §5.0.1 + godoc + ADR cross-ref refreshed
- [x] T103 kind smoke E2E Phase 7 sub-job (bf0c03c) — install.sh + assert.sh + 3 fixtures + workflow step; T103-1 NPUSliceTemplate status hard assert · T103-2 schedulerName auto-stamp hard fail · T103-3 chart adjacency rendered
- [x] T104 HCCS placement (bf0c03c · combined with T103) — set-b-multi-ring fixture (16 NPUs × 4 rings) + ResourceSlice 4-ring assert; PD-pair placement T104-2 ships as soft warning (kind has no real HCCS · Phase 10 lab smoke verifies real silicon · P3 honesty disclosure in devlog)
- [🟡] T105 AllocateBundle (2058f37) — `internal/allocator/bundle.go` free function + 4 unit tests + DESIGN.md §3.10. Phase 7 W1 ship of allocator API; **controller wiring (Pod label → NPUSliceTemplate Get → Engine.Decompose → AllocateBundle → N allocations write) deferred to Phase 10 / T105-v2 per M4 value-focus scope reduction** (Phase 7 has no real Pods with slice-template label · wiring has no caller to exercise · Phase 10 demo polish ships wiring + 3 envtest cases per plan §3 T105 deferred acceptance)
- [⏸] T101 Source.RealAscend lab body — DEFERRED to Phase 10 per ADR-0011 §3 default policy (no lab access signal received during W2 entry · default = defer). chart `sourceType=real-ascend` continues to emit warning-not-error if accidentally selected — Phase 10 light up clean without chart rollback
- [x] T106 Partitionable Devices spike (8762d43) — `docs/research/k8s-partitionable-devices-spike.md` 7 sections + status drift confirmed (1.36 Beta confirmed · GA timing unconfirmed · ADR-0009 v1 "est. 1.37 GA" stale) + Phase 8/10 cost matrix (~13-18 calendar days) + Phase 8 baseline-bump triangle (unblocks NumaAffinity #12 + ProxyImage #T102 + Partitionable Devices Beta in single task)
- [x] phase-7-complete tag lands on T107 (this commit)
- [x] checkpoint-phase7.md (this file) documents every commit SHA + tests + lab-gating outcome + known issues + Phase 8 seed

### Out of scope (carried forward)
- [ ] Busy-idle vertical scaling controller — Phase 8 (reads NPUSliceTemplate substrate per ADR-0011 §1)
- [ ] Partitionable Devices (KEP-4815) actual migration — Phase 8 (1.36+ baseline bump · IF GA available) or Phase 10 (if GA delayed)
- [ ] Volcano gang-scheduling integration — Phase 8+ training-job scenario per ADR-0010 §231
- [ ] Karmada multi-site federation + multi-tenant quota controller — Phase 9
- [ ] O2 DMS adapter — Phase 9
- [ ] Live migration of HCCL ranks / per-Pod RDMA bandwidth quota — CNI-level gaps per `docs/cni-hccl-research.md` §5
- [ ] 真实硬件对接 + 演示打磨 (full real-hardware demo polish) — Phase 10 per arch §1.3
- [ ] Fabric discovery via LLDP / SONiC API — defer per ADR-0007 + ADR-0010 §229 to Phase 7+ on real switching gear
- [ ] NumaAffinity wrap (re-attempt in Phase 8 baseline bump) — known-issues #12

## 5. Known issues (rollup)

Phase 7 net-new: **1 entry added** (`docs/known-issues.md` #12 —
NumaAffinity wrap re-deferred from Phase 7 to Phase 8 baseline bump
prerequisite). Phase 6 entry #11 (scheduler-plugin opt-in) RESOLVED
via P7-T-003.

Carry-forward items documented inline:
- T101 Source.RealAscend body deferred to Phase 10 (ADR-0011 §3 default
  policy · no lab signal)
- T102 ProxyImage chart default flip deferred to Phase 10 demo polish
  (devlog phase-7-t102.md · CI image-pull verification)
- T105 controller wiring deferred to Phase 10 / T105-v2 (devlog
  phase-7-t105.md · Phase 7 W1 has no Pod-with-slice-template-label
  caller to exercise)
- T104 hard-fail placement assertion downgraded to soft warning
  pending Phase 10 lab smoke + controller wiring (devlog
  phase-7-t103-t104.md P3 disclosure)

## 6. Phase 7 → Phase 8 handoff brief

```
项目: D:/code/ai-edge
分支: dev (HEAD @ phase-7-complete tag · this commit)
Tags: phase-0-baseline · phase-1-complete · w1..w3-complete ·
      phase-2-complete · phase-3-complete · phase-4-complete ·
      phase-5-complete · phase-6-complete · phase-7-complete

Phase 7 landed the M3 milestone NPU 动态切分 substrate:
NPUSliceTemplate CRD + template engine + reconciler + allocator
AllocateBundle free function (controller wiring deferred Phase 10).
Source interface unblocks Phase 10 real-hardware integration via
mockjson + realascend stub + factory dispatch. HCCS adjacency
chart default flipped to 8-card ring-of-rings (ADR-0010 §256 closed).
schedulerName auto-stamp resolves known-issues #11. Phase 7 plan §6
top-3 risks all landed cleanly: lab access timing (T101 deferred
per default policy) · driver-layer breakthrough absence (NPUSliceTemplate
fallback path delivered) · sched-plugins v0.32.x release timing
(doc-only fallback per T002 — re-deferred under apimachinery
transitive dep chain on K8s 1.32 baseline).

Phase 8 candidate scope (recommended):

1. **K8s baseline bump 1.32 → 1.36 (or 1.37 if GA available)** —
   single task that unblocks 3 Phase 7 deferred items:
   (a) NumaAffinity wrap (known-issues #12)
   (b) ProxyImage chart default flip (devlog phase-7-t102.md)
   (c) Partitionable Devices Beta enablement (KEP-4815 confirmed
       Beta · per devlog phase-7-t106.md + docs/research/k8s-partitionable-
       devices-spike.md)
   Estimated 1-2 days (could surface new API drift; per known-issues
   #12 same baseline bump unblocks both).

2. **Busy-idle vertical scaling controller** — Phase 8 W1 candidate
   per arch §13. Reads NPUSliceTemplate.status (Phase 7 P7-T-007
   substrate · ADR-0011 §1). Stamps replicas via "重启切片" rollout
   pattern.

3. **AllocateBundle controller wiring (T105-v2)** — wire the deferred
   Phase 7 piece per devlog phase-7-t105.md: claim_controller reads
   Pod label `npu.huawei.com/slice-template=<name>` → client.Get
   NPUSliceTemplate → template.Engine.Decompose → AllocateBundle →
   write N allocations into ResourceClaim.Status / N NPUSliceAllocation
   audit objects. Ships 3 envtest cases per plan §3-T105 deferred
   acceptance.

4. **Partition-aware Allocator** (if KEP-4815 GAs in Phase 8 window)
   — per spike doc §3-§4 cost matrix. HIGH risk row · 3-5d allocator
   work + integration tests.

Carry-over from Phase 7:
- **T101 Source.RealAscend lab body** — when lab access materializes
  during Phase 8/10 window · RealAscendSource methods replace
  ErrNotImplemented stubs · npusmi.ExecClient bodies harden.
- **T102 vllm-ascend ProxyImage chart default flip** — when CI
  image-pull access verified.
- **T104 placement hard-fail assertion** — when controller wiring
  (T105-v2) stamps `preferred-hccs-ring` annotation on PD-pair Pods.

Recommended Phase 8 first session: K8s baseline bump task (item #1
above) — small but high-leverage unblock for 3 deferred Phase 7 items.

Tools needed in addition to Phase 7 toolchain:
- (No new tools required for Phase 8 baseline bump · existing
  Go 1.25 + controller-gen v0.21.0 + Helm + kind sufficient)
- Real-cluster lab access for T101 / T104 hard-fail (Phase 10 demo
  polish)

Estimated Phase 8 total: 2-3 weeks depending on baseline-bump task
breadth + KEP-4815 GA timing. Phase 8 is moderately uncertain (less
than Phase 7's "lab + 3 release dependencies" but baseline bump can
surface unforeseen dep drift).
```

This brief is the seed for the Phase 8 plan. The brief refines into
concrete task packages once the Phase 8 entry session reads the
P7-T-106 spike + decides K8s baseline target minor.

---

**END of Phase 7 checkpoint**
