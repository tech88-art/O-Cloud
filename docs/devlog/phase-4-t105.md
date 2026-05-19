# P4-T-105 · ADR-0009 npu-dra-driver design (slice ↔ ResourceClaim + KubeEdge gap + Partitionable Devices Phase 7)

- **Commit**: a93ceb6
- **Date**: 2026-05-19
- **Duration**: plan 0.5d · actual ~45min

## Intent

Lock the design contract for slice ↔ ResourceClaim semantic mapping, KubeEdge gap handling, K8s 1.37 Partitionable Devices forward note, and the Phase 5 implementation checklist (6 items) into ADR-0009 so Phase 5 entry has zero remaining design work for the npu-dra-driver path.

## Path adaptations

None — pure docs.

## Debugging trail

- Reread `docs/adr/0008-pd-router-webhook.md` for the canonical ADR format used in this repo. Followed it: Status / Date / 决策者 / 相关 → 上下文 → 决策 (numbered sub-sections) → 后果 (positive + negative) → 推翻条件 → 引用.
- The slice ↔ ResourceClaim mapping table is the load-bearing artifact. Worked through each row asking "is this the right K8s DRA v1beta1 mapping?":
  - NPUSlicePool ↔ DeviceClass: yes, cluster-scoped admission selector.
  - NPU 物理设备 ↔ Device entry: yes, with Basic.Attributes carrying Ocloud-namespaced attrs.
  - NPU slice (logical) ↔ device-level Phase 4 / Partitionable Devices entries Phase 7. The Phase 4 "allocator gives whole NPU" simplification is intentional and matters for Phase 5 first-cut algorithm.
  - claim ↔ ResourceClaim: per-Pod or per-PD-side.
  - Ocloud业务绑定 ↔ ResourceClaim.metadata.annotations: documented at T004.
  - PD-pair role ↔ Pod label + claim annotation: documented at T103.
- §6 Allocator algorithm pseudocode (greedy first-fit / best-fit / topology-aware): wrote three alternatives so Phase 5 entry meeting can pick. The greedy pseudocode is essentially what `internal/controller/claim_controller.go` Phase 5 needs to grow into.

## Key decisions

- **Status = `Accepted (design + scaffold landed; allocation logic Phase 5)`**. Differs from ADR-0008's `Accepted (design only — implementation Phase 5)` — we have actual scaffold code (T003-T006 + T101) plus the design. Spelling it out prevents future readers from thinking "Phase 4 only designed it, why isn't the code there?"
- **KubeEdge 6-month rollback judgment date is explicit (2026-11)**. If KubeEdge upstream still has no DRA support by then, Phase 9 production-rolling-up the edge path stays on Device Plugin v1 permanently. Naming the date prevents the question from becoming a perpetual "Phase 5 entry" deferral.
- **Phase 7 upgrade is API-stable**. The §4 forward note explicitly says `AscendDevice` type stays unchanged through Partitionable Devices migration — publisher emits per-partition entries internally, consumers (inference-operator) don't notice. Operators reading this section have a clear signal: "you don't need to plan for an API break."

## Verification

P3 三维度:
- Existence: `grep -n "ADR-0009" docs/adr/0001-phase0-key-decisions.md docs/architecture.md` → 6 hits (3 in ADR-0001 from T001 forward-ref + 3 in arch §3.4 callout + §13 Phase 4/Phase 5 rows)
- Completeness: §1 through §6 + 后果 + 推翻条件 + 引用 all present (mirrors ADR-0008 structure)
- Correctness: every section has concrete content, not placeholder text. The mapping table has 5 rows + 1 invariant; the Phase 5 implementation notes have 6 numbered items each with rationale.

## Carry-forward

- Phase 5 entry meeting picks allocator algorithm (a/b/c from §6); locks NPUSliceAllocation CRD schema (§5 item 3 placeholder).
- Phase 7 entry checks K8s upstream Partitionable Devices GA status (KEP-4815 page) before triggering the publisher's per-partition migration.
- All 4 推翻条件 rows are tracked — if any becomes operative, this ADR gets a v2 / superseded notice.
