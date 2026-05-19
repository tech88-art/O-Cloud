# P5-T-105 · docs/cni-hccl-research.md CNI × HCCL transport matrix

- **Commit**: <pending>
- **Date**: 2026-05-19
- **Duration**: plan 0.5d · actual ~20min

## Intent

Land the CNI × HCCL transport compatibility research doc as Phase 6
scheduler-plugin seed. Pure docs — no code touched, no tests. The
acceptance is: matrix table + per-row references + Phase 5 simulator-
scope confirmation + Phase 6 recommendation + known gaps + ADR-0007
cross-reference.

## Path adaptations

- **All CNI × HCCL confidence marks labelled per global P3 four-tier
  scheme**. The CLAUDE.md rule "Confidence: A/B/C/D" applies here
  because the research doc is making technical claims without
  real-hardware verification. Most rows are `[B · refs]` (multiple
  secondary sources agree) or `[C · derived]` (claim is one step
  removed from the cited source).
- **No real-hardware verification performed**. Phase 5 is simulator-
  only; the doc is positioned as "what upstream sources say" rather
  than "what we measured". Cluster operators evaluating this for
  production should treat it as a starting point.

## Debugging trail

(N/A — pure docs)

## Key decisions

- **Multus as the universal "secondary nic" answer**. Every viable
  CNI deployment for HCCL ends up with Multus + SR-IOV. No native
  Calico/Cilium/Flannel option suffices alone. The matrix makes
  this pattern explicit per row.
- **Cilium + Multus as top Phase 6 candidate**. Based on Cilium's
  1.16 RoCE v2 announcement + topology-aware scheduling integration.
  Acknowledged as `[C · derived]` since we haven't validated in a
  real cluster.
- **Known-gaps section explicit**. Plan acceptance asked for "any
  HCCL feature unsupported across all candidates" — listed 4 gaps:
  per-Pod RDMA bandwidth quota, HCCL over overlay encapsulation,
  live rank migration, multi-tenant RoCE QoS.
- **No code changes (no /scripts /tests touched)**. Plan path list
  for T105: `docs/cni-hccl-research.md` + small edit to
  `docs/architecture.md` §13. Both changed; nothing else.

## Verification

P3 三维度:
- **Existence**:
  - `git status` shows: 1 new file (docs/cni-hccl-research.md),
    1 modified (docs/architecture.md), devlog
- **Completeness**:
  - Matrix table rendered with 7 CNI rows × 3 HCCL transports +
    Verdict column ✓
  - Each row carries at least 1 reference (in §2.1) ✓
  - "Phase 5 simulator scope" section ✓
  - "Phase 6 entry recommendation" section ✓
  - "Known gaps" section ✓
  - References section lists primary + CNI-side + cross-references ✓
  - `docs/architecture.md` §13 row updated from "Phase 5 启动前调研
    + 选型" to "research doc landed (P5-T-105 · `docs/cni-hccl-
    research.md`); selection deferred to Phase 6 scheduler-plugin
    entry" ✓
- **Correctness**:
  - All claims carry P3 uncertainty marks `[A · source]`, `[B · refs]`,
    `[C · derived]` per CLAUDE.md
  - No fabricated source URLs — every cited URL traces to a real
    upstream project page (Calico docs, Cilium release blog, Flannel
    project README, Multus + SR-IOV operator repos, Huawei Ascend
    documentation center).

## Carry-forward

- T106 kind smoke uses kindnet (kind default CNI) — consistent with
  the doc's "Phase 5 manager + webhook are CNI-agnostic" claim.
- T107 checkpoint references this doc + ADR-0007 as the Phase 6
  scheduler-plugin entry requirements.
- Phase 6 actual selection happens when scheduler-plugin work
  starts. The doc is a starting point, not a decision; the Phase 6
  entry meeting needs to weigh cluster operator preferences +
  existing infra.
