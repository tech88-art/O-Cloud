# P6-T-001 · ADR-0010 scheduler-plugin design (HCCSTopology + CNI selection)

- **Commit**: <pending>
- **Date**: 2026-05-20
- **Duration**: plan 0.5d · actual ~35min

## Intent

Land the Phase 6 scheduler-plugin design ADR so T002-T008 can directly
implement without re-litigating decisions. Acceptance asks for 6
sections — framework + plugin semantics + args schema + ResourceSlice
attribute contract + CNI selection + Phase 7 forward notes. Pure docs,
no code.

## Path adaptations

- **Plan listed 6 sections (§1-§6) but ADR-0009 precedent uses
  multi-section "决策" structure** — I split into §1-§7 to give CNI
  selection its own section (was clustered into "Decision" in plan
  acceptance). Substance is the superset of plan acceptance:
  - Plan §1 Context → ADR `上下文` (cites P5 checkpoint §7 + ADR-0009
    §6.2 forward note)
  - Plan §2 Decision → ADR `决策` §1 (framework + deployment) + §2
    (HCCS Filter+Score) + §3 (NUMA reuse) + §4 (Binpack opt-in)
  - Plan §3 Consequences → ADR `决策` §1 (multi-scheduler form) + ADR
    `后果` (sections both 正面 and 负面 / 风险)
  - Plan §4 Plugin args schema → ADR `决策` §5 (full Go struct schemas)
  - Plan §5 ResourceSlice attribute contract → ADR `决策` §5 (table +
    Pod 注释契约 sub-block)
  - Plan §6 Forward notes → ADR `决策` §7 (Phase 7+ table)
  - Bonus: ADR `决策` §6 = Phase 6 CNI 选型 (Cilium primary / Calico
    fallback) — plan acceptance had this in §2 but it deserved its
    own section because the rationale matrix is 5 rows.

## Debugging trail

(N/A — pure docs)

## Key decisions

- **Plugin args weight defaults**: HCCS=5 / NUMA=2 / Binpack=1.
  Reasoning: HCCS is the new Phase 6 capability and the headline
  optimization (HCCL bandwidth dominates PD-pair throughput per
  vllm-ascend disaggregated_prefill design). NUMA is secondary
  (cache locality matters less than ring co-location once HCCL is
  the bottleneck). Binpack is off by default — Phase 6 first wants
  to observe HCCS+NUMA behavior before consolidating. These weights
  are knobs in KubeSchedulerConfiguration, operators tune per
  workload.

- **Default Filter permissive (FailIfMissing=false)**: This is the
  safe default because inference-operator Phase 5 does NOT stamp
  `npu.huawei.com/preferred-hccs-ring` on Pods. If Filter defaulted
  to strict, every existing ModelService would suddenly fail to
  schedule once the plugin landed. ADR explicitly notes future
  toggle to strict + inference-operator polish to stamp the
  annotation; meantime Score still works as a soft preference.

- **Score reads NPUSliceAllocation reverse-lookup (not spec)**:
  npu-dra-driver/DESIGN.md §6.3.4 §1 called out the reverse-lookup
  index as "Phase 6 scheduler-plugin co-location decisions" — so the
  Score logic uses ground-truth ("where are siblings already
  allocated") instead of spec ("where will siblings be allocated").
  This is the right call: spec changes don't propagate atomically,
  and Phase 5 PD Router webhook fail-closes when allocations are
  Orphaned — so reading allocations is the freshest signal.

- **Multi-scheduler form not patch default-scheduler**: K8s
  KubeSchedulerConfiguration profiles[] is the first-class extension
  point. Patching default-scheduler would force every workload onto
  custom logic. Multi-scheduler keeps blast radius bounded — Pod
  must `spec.schedulerName: npu-scheduler` to opt in.

- **Wrap upstream noderesourcetopology, don't fork**: No Ascend-
  specific NUMA logic exists in this codebase. Fork would add long-
  term maintenance debt; wrap pulls upgrades for free.

- **Binpack ~50 LOC internal, not Volcano**: Volcano's binpack ties
  into volcano-specific scheduling cycle state (PodGroup, etc.) we
  don't need. 50 LOC of `score = sum(weight * requested / allocatable)`
  is cheap to maintain and zero new external deps.

- **Cilium primary, Calico fallback in ADR §6**: Per cni-hccl-research
  §4 recommendation but with caveat — Cilium 1.16+ RoCE is still
  experimental (Confidence C · derived) per research doc. Production
  rollout teams may legitimately choose Calico for stability. ADR-0010
  §推翻条件 records this is reviewable on real-cluster data.

- **CNI-portable design isolation**: Wrote explicitly into §6 that
  the plugin reads ResourceSlice attributes + NodeResourceTopology +
  Pod annotations only — zero CNI-specific API. So the choice is a
  deployment recommendation, not a build-time lock.

## Verification

P3 三维度:

- **Existence**:
  - `git status` shows: 1 new file `docs/adr/0010-scheduler-plugin.md`,
    2 modified (`docs/architecture.md` §5.6 + §13 row, `docs/cni-hccl-
    research.md` §4 final paragraph addendum)
  - `grep -l ADR-0010 docs/` returns 4 files — the new ADR + 3 cross-
    referencing docs (arch, research, phase6-plan)

- **Completeness** (plan acceptance criteria checklist):
  - §1 Context cites Phase 5 checkpoint §7 ✓ (上下文 paragraph 4)
  - §1 Context cites ADR-0009 §6.2 forward note ✓ (上下文 paragraph 3)
  - §2 Decision: HCCSTopologyPlugin Filter+Score ✓ (决策 §2)
  - §2 Decision: NumaAffinity upstream reuse ✓ (决策 §3)
  - §2 Decision: Binpack opt-in via args ✓ (决策 §4)
  - §2 Decision: CNI Cilium primary / Calico fallback ✓ (决策 §6)
  - §3 Consequences: separate kube-scheduler binary NOT default-scheduler
    patch ✓ (决策 §1 + 后果 §正面 paragraph 2)
  - §3 Consequences: opt-in via KubeSchedulerConfiguration ✓ (决策 §1
    paragraph "Pod 显式 opt-in")
  - §3 Consequences: CNI choice does not lock plugin (CNI-portable) ✓
    (决策 §6 final paragraph + 后果 §正面 paragraph 3)
  - §4 Plugin args schema: HCCSTopologyArgs / NumaAffinityArgs /
    BinpackArgs ✓ (决策 §5 with full Go struct definitions)
  - §5 ResourceSlice attribute contract: hccs_ring + numa_node IntValue
    reads ✓ (决策 §5 contract table)
  - §5 Pod annotation `preferred-hccs-ring` gates strict filtering ✓
    (决策 §5 Pod 注释契约 sub-block)
  - §6 Forward notes: Phase 7 npu-smi for real-hw ring discovery ✓
    (决策 §7 row 1)
  - §6 Forward notes: Phase 9 per-Pod RDMA quota ✓ (决策 §7 row 4)

- **Correctness**:
  - Frontmatter status line dated 2026-05-20 (T001 run date) per ADR-0009
    convention
  - All cross-refs verified: ADR-0009 §6.2 (allocator forward note) +
    npu-dra-driver/DESIGN.md §6.3.4 (NPUSliceAllocation reverse-lookup)
    + cni-hccl-research §4 (Phase 6 entry recommendation) + arch §5.6
    (scheduler-plugin module) + arch §13 (Phase 5+ network + Phase 6 row)
  - architecture.md §5.6 edit: 3 plugin bullets enriched with concrete
    impl notes + cross-link to ADR-0010
  - architecture.md §13: CNI row updated from "selection deferred" to
    "selection landed Phase 6 ADR-0010"; new Phase 6 row added marking
    "in flight"
  - cni-hccl-research.md §4: appended ADR-0010 §6 cross-reference
    paragraph
  - All P3 uncertainty marks reused from cni-hccl-research where claims
    are inherited (Cilium 1.16 RoCE is C · derived).

## Carry-forward

- **T002 framework version pin**: ADR §1 says v0.31.x · final patch
  rev pinned at task entry per "P6-T-002 task entry decision"
- **T003 (pool-operator)**: must read ResourceSlice attribute names
  exactly as ADR §5 contract table — `npu.huawei.com/hccs_ring` not
  `hccsRing` (the qualified-name form is what's published, the bare
  Go field name on AscendDevice differs)
- **T004 (HCCS Filter)**: directly implements ADR §2 Filter table —
  unit tests should mirror the 4 input combinations
- **T005 (HCCS Score)**: directly implements ADR §2 Score table —
  unit tests should cover the 5 scoring tiers (50/100/70/30/0)
- **T006 (NUMA)**: wraps `sigs.k8s.io/scheduler-plugins/pkg/
  noderesourcetopology` — no Ascend-specific logic
- **T007 (Binpack)**: 50 LOC internal, default ResourceWeights from
  ADR §4 paragraph
- **T101 (chart)**: KubeSchedulerConfiguration ConfigMap must include
  pluginConfig for HCCSTopologyArgs + NumaAffinityArgs + BinpackArgs;
  values.yaml drives weights/enabled
- **T105 (inference-operator)**: decide whether to stamp
  `npu.huawei.com/preferred-hccs-ring` annotation on Deployment Pod
  templates; current ADR leaves Filter permissive so this is optional
  but worth doing for explicit contract
