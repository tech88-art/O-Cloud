# P6-T-005 · HCCSTopologyPlugin Score logic

- **Commit**: <pending>
- **Date**: 2026-05-20
- **Duration**: plan 1d · actual ~40min (allocationLister abstraction
  mirrored the sliceLister pattern from T004 → fast to implement)

## Intent

Land the Score body for HCCSTopology per ADR-0010 §2 Score table.
Implements PreScore + Score + ScoreExtensions on the existing
HCCSTopology struct. NPUSliceAllocation reverse-lookup happens once per
Pod cycle in PreScore (PreScorePlugin path); Score reads precomputed
ring-occupation state from CycleState for O(1) per-node decisions.

## Path adaptations

- **NewForTest signature broken (T004 → T005)**: T004 had
  `NewForTest(args, sliceLister)` (2 args); T005 adds allocationLister
  → now `NewForTest(args, sLister, aLister)` (3 args). All 6 filter_test
  call sites updated (`nil` passed for aLister since Filter doesn't
  need it). This is a deliberate test API break — none of the test
  callers are stable contracts; production goes through `New()`.

- **NPUSliceAllocation listed once per Pod, not cached via informer**:
  Plan §3 P6-T-005 didn't specify caching strategy. For Phase 6
  simulator scope (< 100 NPUSliceAllocations expected, single
  cluster), a direct `dynamic.Resource.List()` per PreScore call is
  acceptable. PreScore runs once per Pod scheduling cycle so the
  cost is bounded. Phase 9 multi-tenancy will likely need a
  `dynamicinformer.NewDynamicSharedInformerFactory` cache —
  documented inline in `colocation.go` doc comment + ADR-0010 §3
  performance risk row.

- **Self-allocation skip is heuristic**: PreScore reads NPUSliceAllocations
  that match the Pod's ModelService — some of them might be claims this
  Pod already owns. We skip when `alloc.Name == pod.Name`, which is the
  simplest signal. Imperfect (claims may have distinct names from Pods)
  but acceptable for Phase 6 — the false positives just count a Pod as
  "its own sibling" giving it ScoreSame on its own current node, which
  is harmless first-pass.

- **No `Adjacency` JSON-round-trip yet**: HCCSTopologyArgs.Adjacency is
  `map[string][]int32` (string keys for JSON-encodability) per ADR-0010
  §4. T005 `buildAdjacency` converts to `map[int64]map[int64]struct{}`
  for O(1) Score lookups. Malformed string keys silently dropped — same
  fail-soft posture as T004's `parsePreferredRings`.

## Debugging trail

- **No surprises**. Pattern was well-established by T004 + the upstream
  ScorePlugin / PreScorePlugin interfaces are simple.

- **CycleState `Read` returns error not (val, bool)**: minor friction —
  built a `readHCCSState(cs)` helper that wraps the error-returning API
  in a (val, ok) idiomatic pattern.

- **All 13 sub-tests pass in 0.67s**: combined Filter (6) + Score (7) +
  ParsePreferredRings (7) + ParseArgs (3) + BuildAdjacency (3) = 26.
  Local test run on Windows non-elevated shell — sliceLister/
  allocationLister fakes avoid envtest entirely.

## Key decisions

- **PreScorePlugin not InvariantInformerLister**: K8s scheduler offers
  multiple ways to do per-Pod data prep. PreScorePlugin is the
  canonical "compute once, share across all Score calls" hook. Used
  it explicitly so the framework knows to call PreScore before any
  Score on a given cycle.

- **ScoreExtensions returns nil**: framework auto-normalises to
  [0..100]. Our 5 tier constants (0/30/50/70/100) already live in that
  range, so NormalizeScore is a no-op. Returning nil saves an
  extension-point hop and clarifies intent.

- **Adjacency precompute in PreScore, not Score**: parsing the
  string→int adjacency map is O(|adjacency|). Doing it once per cycle
  (PreScore) is strictly cheaper than once per node (Score). The
  precomputed result stashes in CycleState alongside ringsOccupied.

- **No-MS-label and no-siblings both return 50**: tested separately
  for clarity but the implementation collapses them — both indicate
  "no co-location preference". `pod.Labels[ModelServiceLabel]` is
  re-checked in Score for the rare case PreScore was skipped (which
  shouldn't happen because PreScore is unconditional for our plugin,
  but defensive coding).

- **dynamic.Interface built per Filter/Score plugin construction**:
  scheduler creates one HCCSTopology instance per profile. The
  dynamic client closes over a single rest.Config — no
  reconstruction cost per scheduling cycle.

- **dynamic-client construction error silently swallowed in New**:
  if `dynamic.NewForConfig(kc)` fails (rare — would mean an invalid
  rest.Config), the plugin still constructs with `allocationLister=
  nil`. Score gracefully degrades to ScoreNeutral (50) on every
  node. Better than refusing to register the plugin, which would
  block the entire scheduler profile.

## Verification

P3 三维度:

- **Existence**:
  - `git status --short` → 5 files changed (plugin.go, filter_test.go,
    DESIGN.md, go.mod, go.sum) + 3 new (colocation.go, score.go,
    score_test.go) + devlog
  - `grep -c 'ScorePlugin\|PreScorePlugin' operators/scheduler-plugin/`
    → 4 (2 interface refs + 2 compile-time assertions)
  - `ls operators/scheduler-plugin/internal/plugins/hccs/` → plugin.go,
    args.go, types.go, filter.go, filter_test.go, score.go,
    score_test.go, colocation.go (8 files)

- **Completeness** (plan §3 P6-T-005 acceptance):
  - `Score(ctx, cycleState, pod, nodeName) (int64, *framework.Status)`
    signature ✅
  - Pod NOT carrying model-service label → 50 ✅
    (TestScore/no_model-service_label)
  - Sibling Pod on ring → 100 ✅
    (TestScore/MS_label_with_sibling_on_ring_0)
  - Sibling Pod on adjacent ring → 70 ✅
    (TestScore/MS_label_with_sibling_on_adjacent_ring)
  - Sibling on disjoint ring → 30 ✅
    (TestScore/MS_label_with_sibling_on_disjoint_ring)
  - No candidate devices → 0 ✅
    (TestScore/MS_label_with_sibling_but_node_has_no_candidate_device)
  - 5+ envtest cases ✅ (7 sub-tests covering all 5 tiers + multi-ring +
    no-siblings)
  - `go test ./internal/plugins/hccs/... -timeout 60s` clean (combined
    T004+T005) ✅ (26 sub-tests, 0.67s)

- **Correctness**:
  - `go vet ./...` exit 0
  - `go mod tidy` exit 0
  - `go test -v ./internal/plugins/hccs/...` → 26/26 PASS
  - `go build ./...` exit 0
  - DESIGN.md §3.3 Score cycle pseudocode + §5.1 status updated to
    operative

## Carry-forward

- **T006 (NUMA wrap)**: independent module; no shared code with hccs

- **T007 (Binpack)**: separate `internal/plugins/binpack/` package;
  same NewForTest pattern (args + handle) but no slice/allocation
  listers

- **T008 (integration tests)**: will exercise PreScore + Score via the
  real kube-scheduler binary against a setup-envtest cluster. Cases:
  - Pod 1 (no MS label) → permissive Filter, Score 50 on every node
  - Pod 2 (MS label, no siblings) → Score 50 on every node
  - Pod 3 (MS label, sibling on worker-a:ring=0) → Score 100 for
    worker-a, lower for worker-b
  - Pod 4 (preferred-hccs-ring=5 annotation) → all nodes Filtered out

- **T101 (helm chart)**: KubeSchedulerConfiguration ConfigMap must
  include the PreScore extension-point enable line. K8s framework
  defaults to enabling PreScore for any plugin that declares
  framework.PreScorePlugin — chart author can rely on the default,
  no explicit `preScore.enabled` list needed.

- **T105 (inference-operator)**: Pods need both the
  `inference.ocloud.edge.example.com/model-service` LABEL (Score
  grouping key) and the `npu.huawei.com/preferred-hccs-ring`
  ANNOTATION (Filter ring matching). Phase 5 PD Router stamps the
  label already; preferred-rings annotation is a T105 candidate add.

- **NPUSliceAllocation cache upgrade**: Phase 9 multi-tenancy entry
  point — dynamicinformer.NewDynamicSharedInformerFactory + lister
  cache. Currently per-cycle List() suffices.
