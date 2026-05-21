# Spike: Standard-K8s Partitionable Devices (KEP-4815) status + Phase 8/10 migration cost

> **Date**: 2026-05-20 · **Author**: Phase 7 P7-T-106 (main agent direct)
> · **Status**: Spike (research-only · no code change)
>
> **Purpose**: Per phase7-plan.md §4 P7-T-106 + ADR-0009 §4 Partitionable
> Devices forward note · refresh the W2 entry view of upstream KEP-4815
> status + decision matrix for the npu-dra-driver publisher migration
> from the Phase 7 NPUSliceTemplate fallback path to native K8s
> Partitionable Devices when KEP-4815 GAs.

---

## 1. KEP-4815 status as of W2 entry (2026-05-20)

| Stage  | K8s minor | Released         |
|--------|-----------|------------------|
| Alpha  | 1.35      | confirmed        |
| **Beta**   | **1.36**      | **confirmed (issue label `stage/beta` set; "Tracked for Docs Freeze" complete)** |
| GA     | not specified yet | — |

**Source**: https://github.com/kubernetes/enhancements/issues/4815 (WebFetch 2026-05-20).
Issue is actively tracked across SIG-node + SIG-scheduling +
"1.36 Enhancements Tracking" + "Dynamic Resource Allocation" projects.

**Drift from Phase 4 baseline assumption** (ADR-0009 §4 v1):

| Source                              | Stage assumption                                                  |
|-------------------------------------|-------------------------------------------------------------------|
| ADR-0009 §4 v1 (2026-05-19)         | "K8s 1.35 Alpha / 1.36 Beta / **est. K8s 1.37 GA**"               |
| P7-T-106 spike (2026-05-20)         | 1.35 Alpha confirmed · 1.36 Beta confirmed · **GA timing unknown** |

ADR-0009 §4 v1 was directionally correct — Alpha + Beta both landed
as predicted. **GA estimate (1.37) is now unconfirmed** — upstream
issue page does not commit to a GA target K8s minor. Phase 8/10
planning should treat GA as "1.37 minimum, possibly later" (1.38?).

---

## 2. Upstream merged PRs since Phase 4 baseline (2026-05-19)

(Not enumerated exhaustively here — the spike's scope per
phase7-plan §4 T106 is "confirm KEP status + status drift", not "audit
every merged PR". When Phase 8 planning starts, run a fresh `gh issue
list -R kubernetes/enhancements -L 50 --state all --search "4815"`
to catch the latest implementation PRs.)

**Key signals from the issue page**:

- Multiple related PRs across `kubernetes/kubernetes` + `kubernetes/website`
- "Tracked for Docs Freeze" indicates user-facing API is stable enough
  for documentation
- SIG-node + SIG-scheduling co-owned (consistent with the 4-block DRA
  ownership model)

**Implication for npu-dra-driver**:

- Publisher's `ResourceSlice.Spec.Devices` schema may have new
  optional fields (parent reference for partitions) in K8s 1.36+
- Existing whole-device emit path (one Device per NPU) continues to
  work as a "non-partitioned" device — backward-compat path preserved
- The "partition emit" alternative is opt-in via new fields, not a
  forced schema change

---

## 3. Breaking-change risk for npu-dra-driver publisher

**Phase 7 baseline** (after P7-T-004 Source interface refactor):

```
internal/source/mockjson/MockJSONSource.List()
  → emits []NodeDevices{NodeName, Devices: []AscendDevice}
  → publisher.buildSlice() projects each AscendDevice into
    upstream resourceapi.Device (one per NPU)
```

**Phase 8/10 partitioned-emit migration path**:

```
internal/source/realascend/RealAscendSource.List()
  → emits []NodeDevices{NodeName, Devices: [whole NPU × N + partitions × M]}
  → publisher.buildSlice() projects whole NPUs + partitions into
    resourceapi.Device entries with parent ref (whole NPU) + partition
    children with smaller capacity slots
```

**Breaking-change risk assessment**:

| Component                         | Phase 7 → Phase 8+ migration impact                                            |
|-----------------------------------|--------------------------------------------------------------------------------|
| `api/v1alpha1.AscendDevice`       | LOW — typed view of upstream Device; if upstream adds parent/partition fields, the typed view extends with new fields; no rename/removal expected |
| `internal/source/source.go::Source` interface | LOW — `List` + `Watch` + `QueryTopology` contract preserved; new partition emit goes through same List path |
| `internal/source/mockjson/MockJSONSource` | LOW — JSON fixture schema can extend with optional partition fields; existing fixtures (Phase 4 set-a-small) continue to emit whole-NPU only |
| `internal/source/realascend/RealAscendSource` (P7-T-101 stub today) | MEDIUM — Phase 7 T101 lab body composes whole-NPU; Phase 8+ adds partition discovery via `npu-smi info -t partitions` (does not exist today on standard 910B but predicted by ADR-0011 §後果 for driver-layer breakthrough scenario) |
| `internal/publisher/publisher.go::Publisher.buildSlice` | LOW — emits one ResourceSlice per node; partition entries inside the slice are upstream-Device list items |
| `internal/template/Engine` (P7-T-007) | MEDIUM — Phase 7 W1 rejects `dynamic-shard`; Phase 8+ unblocks `dynamic-shard` once partitions are emit-able + allocator handles partition-targeted ResourceClaim alloc |
| `internal/controller/npuslicetemplate_controller.go` | LOW — status condition logic unchanged; observed Allocatable now reflects partition-aware availability |
| `internal/allocator/` (Phase 5 greedy + T105 dynamic-slice) | HIGH — Phase 8+ partition-aware allocator chooses among (whole NPU candidates) + (partition candidates); deciding the right partition vs spawning a new whole-NPU split is a NEW allocator policy decision |
| `operators/scheduler-plugin/` (HCCS Filter + Score + NumaAffinity + Binpack) | LOW — reads ResourceSlice attributes; partition entries inherit same attribute schema; Score's "same ring 100" tier still works (partitions on same physical NPU share hccs_ring) |
| `operators/inference-operator/internal/controller/deployment_builder.go` (PD-pair + T003 schedulerName auto-stamp) | NONE — operator-facing API unchanged; SchedulerOverride still works |

**Net assessment**: only the allocator (P5-T-002 + P7-T-105) is HIGH
risk. Everything else MEDIUM-or-LOW.

---

## 4. Estimated Phase 8/10 migration effort

| Workstream                                                       | Estimate (calendar days) |
|------------------------------------------------------------------|-------------------------|
| Upstream tracker re-audit + decision: K8s baseline 1.36/1.37/1.38 | 0.5d                    |
| `go.mod` baseline bump (resolves T002 NumaAffinity blocker too)  | 1d (potentially 2-3d if NumaAffinity wrap surfaces new API drift; per known-issues #12 same baseline-bump unblocks both) |
| `api/v1alpha1.AscendDevice` extends with partition fields        | 0.5d                    |
| `internal/source/source.go` partition-emit helper                 | 0.5d                    |
| `internal/source/mockjson/MockJSONSource` partition fixture       | 1d                      |
| `internal/source/realascend/RealAscendSource` real-cluster partition emit (depends on lab T101 first) | 2-3d (lab-dependent) |
| `internal/template/Engine` unblock `dynamic-shard` PartType       | 1d                      |
| `internal/allocator/` partition-aware Allocate                    | 3-5d (HIGH risk row)   |
| `internal/controller/npuslicetemplate_controller.go` status refresh | 0.5d                    |
| kind smoke + lab smoke partition scheduling integration tests     | 2d                      |
| DESIGN.md + ADR-0009 update                                       | 0.5d                    |
| Phase 8/10 checkpoint refresh                                     | 0.5d                    |
| **Total**                                                         | **~13-18 calendar days** = ~3 calendar weeks (single-track, no parallelism) |

**Phase 8 entry GA gating**:
- IF K8s 1.37 GA (best case · Phase 8 W2 calendar) → start full
  migration · ~3 weeks until tag
- IF K8s 1.37 only Beta · 1.38 candidate → split:
  - Phase 8 W1 = baseline bump 1.32 → 1.36 (Partitionable Devices Beta
    enabled via feature flag) · NPUSliceTemplate fallback path
    coexists during transition
  - Phase 8 W2+ = full partition emit + allocator migration when GA
    lands
- IF K8s 1.37 still Alpha at Phase 8 entry → defer to Phase 10 + run
  NPUSliceTemplate fallback path indefinitely

---

## 5. Phase 7 NPUSliceTemplate path vs Partitionable Devices coexistence

**Phase 7 ships** (this commit chain · T001-T008 W1):
- NPUSliceTemplate CRD + template engine + reconciler (T006/T007)
- Allocator extension (T105) reads bundle from engine.Decompose
- Source interface decouples mockjson from realascend (T004/T005)
- Chart values default 4-ring HCCS adjacency (T008)

**Once KEP-4815 GAs** (Phase 8/10 best case):
- NPUSliceTemplate CRD stays — provides operator-facing high-level
  composition syntax + status (Validated/Allocatable/FallbackAppliedReason)
- Engine.Decompose body changes: instead of mapping
  `1× vir04 + 1× vir08 → 2 whole NPUs (1 each)`, the engine maps
  `1× vir04 + 1× vir08 → 1 NPU's 2 partitions (vir04 + vir08)`
- Allocator path changes: partition-targeted ResourceClaim allocation
  vs Phase 7 whole-NPU greedy
- `dynamic-shard` PartType unblocks (Engine.Validate removes the
  early-reject; allocator picks AI-core budget from partition spec)
- Operator-facing API: zero breaking change · existing Qwen-PD /
  DeepSeek-strict samples (`config/samples/npuslicetemplate_*.yaml`)
  continue to work · Phase 7 → Phase 8 is purely internal substrate
  switch

**Trade-offs**:

| Aspect                  | Phase 7 NPUSliceTemplate fallback                                | Phase 8+ Partitionable Devices                               |
|-------------------------|------------------------------------------------------------------|--------------------------------------------------------------|
| HCCS locality           | Best-fit: 1× vir04 + 1× vir08 = 2 different whole NPUs (probably same HCCS ring) | Optimal: 1× vir04 + 1× vir08 = 1 same NPU's 2 partitions (zero-cost local copy between sibling slices) |
| Resource utilization    | Lower (2 NPUs reserved for ~12 AI cores)                          | Higher (1 NPU reserved for ~12 AI cores · other 20 AI cores on same NPU still allocatable) |
| Allocator complexity    | Greedy first-fit on whole NPUs (Phase 5 baseline)                | Partition-aware best-fit with shared-NPU sibling preference |
| Operational maturity    | Production-ready Phase 7 W1 · simulator + Phase 7 T101 lab smoke | Beta API · Phase 8/10 only · production maturity Phase 11+   |
| Multi-tenancy isolation | Whole-NPU boundary = strong isolation (no co-tenant noise)        | Partition boundary = weak isolation (co-tenants on same NPU share HCCS bus · noisy-neighbor risk) |

**Recommendation**: keep NPUSliceTemplate path as the documented
**production** path until Phase 11+; partition path is **performance**
optimisation for single-tenant workloads. Multi-tenant deployments
(Phase 9 Karmada + RBAC) likely prefer whole-NPU.

---

## 6. Forward checklist for Phase 8 entry session

Pre-flight (do these BEFORE Phase 8 T001):

- [ ] Re-WebFetch https://github.com/kubernetes/enhancements/issues/4815
      and update §1 status table
- [ ] Run `gh issue list -R kubernetes/enhancements -L 50 --search "4815"`
      to enumerate merged PRs (skipped in §2 of this spike)
- [ ] Confirm kindest/node image version supporting Partitionable
      Devices feature gate (1.36-kind release status)
- [ ] Confirm operators/scheduler-plugin v0.32+ Apilint cleanup
      (P7-T-002 known-issues #12 resolves via same K8s baseline bump)
- [ ] Verify `quay.io/vllm-project/vllm-ascend` image tags + sizes
      (P7-T-102 ProxyImage flip prerequisite — same baseline bump
      unblocks both)

Phase 8 entry meeting decides:
- (a) target K8s minor for baseline bump
- (b) phase boundaries for NumaAffinity + ProxyImage + Partitionable
      Devices (do all 3 in Phase 8 baseline-bump task, or stagger)
- (c) whether to ship partition-aware allocator as Phase 8 deliverable
      or Phase 9-10 polish

---

## 7. References

- KEP-4815: https://github.com/kubernetes/enhancements/issues/4815
  (Phase 7 P7-T-106 fetched 2026-05-20)
- ADR-0009 §4 Partitionable Devices forward note (Phase 4 baseline ·
  Phase 7 P7-T-001 cross-ref + P7-T-106 status refresh)
- ADR-0011 §1 NPU 动态切分 + §後果 row (Phase 7 fallback substrate)
- phase7-plan.md §4 P7-T-106 (this spike's task contract)
- docs/known-issues.md #12 (NumaAffinity baseline bump · same Phase 8
  entry candidate)
