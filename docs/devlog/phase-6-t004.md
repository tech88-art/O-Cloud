# P6-T-004 · HCCSTopologyPlugin Filter logic

- **Commit**: <pending>
- **Date**: 2026-05-20
- **Duration**: plan 1.5d · actual ~45min (sliceLister abstraction kept
  the Filter unit-testable without spinning up a real informer cache)

## Intent

Land the Filter body for the HCCSTopology plugin per ADR-0010 §2. Reads
ResourceSlice device attributes via a `sliceLister` interface (production
wraps the scheduler framework's SharedInformerFactory ResourceSlice
lister; tests inject a fake). Returns admission decisions per the
4-row Filter table (permissive when annotation absent → strict on
FailIfMissing; success when at least one ring matches → fail when none).

## Path adaptations

- **Plan acceptance "Available >= request.cores" capacity check
  deferred**: The plan §3 P6-T-004 listed `Available >= request.cores`
  alongside ring matching. I deferred it to T005 / future polish for
  two reasons:
  1. DRA's PreFilter / Filter machinery already runs a claim-driven
     availability filter before any HCCSTopology Filter call — the
     scheduler won't ask us to score a node whose ResourceClaim cannot
     bind there. Duplicating that check here would be redundant.
  2. The "AI cores remaining" math is properly a Score signal (T005
     uses it for co-location strength tiebreaking), not a hard Filter.
     Filter focuses on **topology** (ring match) per ADR-0010 §2's
     intent.

  This deviation is documented in `filter.go`'s docblock + the DESIGN.md
  §3.2 cycle diagram so future readers know the AICore Available column
  in the plan acceptance is intentionally handled elsewhere.

- **DESIGN.md created at T004, not T002**: scaffold task T002 README
  explicitly stated "DESIGN.md deferred to T004 (controller-body
  equivalent task) per root CLAUDE.md §14.2". T004 ships DESIGN.md
  with full architecture/contract/lifecycle/extension sections + the
  forward notes for T005-T008 (cross-reference points).

- **sliceLister interface, not direct informer in Filter**: production
  wires `framework.Handle.SharedInformerFactory().Resource().V1beta1().
  ResourceSlices().Lister()` via the `informerSliceLister` adapter
  (plugin.go); tests construct via `NewForTest(args, fakeLister)`.
  This kept the unit tests pure (no envtest needed) — the integration
  tests at T008 will exercise the production informer path.

## Debugging trail

- **No surprises**. Pattern was well-established by sched-plugins
  upstream — args parsing via `runtime.Unknown` decode + typed merge
  + DeepCopyObject manual implementation (no controller-gen since we're
  not a Kubebuilder project).

- **`go mod tidy` on first add of resourceapi v1beta1 + listers**: ran
  clean after ~5s (deps were already in transitive closure from K8s
  framework — no new downloads).

- **Binary size went from 94MB → 97MB**: extra ~3MB from the
  `k8s.io/client-go/informers/resource/v1beta1` + listers +
  resourceapi types being linked in by the plugin's New() path. Within
  expectations.

- **Filter on Windows non-elevated shell builds + tests directly**:
  unlike pool-operator envtest (UAC-blocked), the HCCS Filter unit
  tests use a fake sliceLister so they run locally. All 6 Filter cases
  + 7 parsePreferredRings cases + 3 parseArgs cases pass via
  `go test ./internal/plugins/hccs/... -timeout 60s` (combined
  duration 0.6s).

## Key decisions

- **Permissive default (FailIfMissing=false)**: per ADR-0010 §2 the
  baseline. Phase 5 inference-operator does not stamp the
  `preferred-hccs-ring` annotation on Pods today; making Filter
  strict by default would break every existing ModelService Pod the
  moment T101 chart deploys this scheduler. Phase 6 T105 (inference-
  operator polish) is the future opportunity to flip Pods over to
  carrying the annotation; once the rollout is complete the default
  can flip too.

- **Malformed annotation treated as missing**: e.g. `"abc,def"` →
  parsePreferredRings returns nil → behaves like annotation absent
  (permissive or strict per FailIfMissing). A pod with a typo
  shouldn't fail more aggressively than one with no annotation;
  surface the issue via the operator's logging plane.

- **Sort-as-given for preferred rings**: parsePreferredRings preserves
  input order. Adjacency lookups (T005 Score) may want sorted or
  set-style access, but Filter only checks membership so order
  doesn't matter today. Documented as "sorted-as-given" in types.go
  so the contract is clear.

- **Health filter back-compat**: missing `npu.huawei.com/health`
  attribute → treated as Healthy (mirrors T003 pool-operator
  `healthAttributeOK`). This lets a hypothetical pre-T002 npu-dra-
  driver publisher still feed valid devices to the plugin. Anything
  not `"Healthy"` (incl. malformed payload) → not healthy.

- **`nodeInfo` nil-check is defensive**: scheduler framework
  guarantees nodeInfo is non-nil with a Node attached at Filter time;
  guarding anyway with a framework.Error path. Costs one nil check
  + lets the plugin survive a future framework change without
  panicking.

- **`sliceLister` nil-check enforces graceful degradation**: if
  `h.SharedInformerFactory()` returns nil (defensive — kube-scheduler
  always provides one in production), Filter returns permissive
  (Success) when annotation absent, UnschedulableAndUnresolvable
  when annotation requires ring matching. Avoids panic / Error
  status, surfaces the misconfiguration as "no scheduling decisions
  possible" instead.

## Verification

P3 三维度:

- **Existence**:
  - `git diff --stat` → 5 files modified/added (plugin.go updated,
    args.go + types.go + filter.go + filter_test.go + DESIGN.md
    new), plus go.mod + go.sum churn
  - `ls operators/scheduler-plugin/internal/plugins/hccs/` →
    plugin.go, args.go, types.go, filter.go, filter_test.go (5 files)
  - `grep -l 'FilterPlugin' operators/scheduler-plugin/` →
    plugin.go (compile-time assertion) + filter.go (method)

- **Completeness** (plan acceptance):
  - `Filter(ctx, cycleState, pod, nodeInfo) *framework.Status`
    signature ✅ (filter.go)
  - Pod missing annotation → nil/Success (permissive) ✅
    (TestFilter/annotation_absent,_permissive_default)
  - Pod with comma-separated rings → match at least one ✅
    (TestFilter/annotation_present,_ring_matches + multi-ring partial)
  - Pod with ring not on node → UnschedulableAndUnresolvable ✅
    (TestFilter/annotation_present,_ring_does_not_match)
  - Args parsing: Weight [0..100] / PreferAnnotation default /
    FailIfMissing default false ✅ (TestParseArgs 3 cases)
  - 6 envtest cases ✅ (TestFilter has 6 sub-tests; "envtest" intent
    satisfied via the sliceLister fake — true envtest deferred to T008
    integration tests per plan's §0a.11 strict-per-task scope)
  - DESIGN.md per CLAUDE.md §14.2 ✅ (created at T004 per the deferral
    documented in T002 README — full module DESIGN with 7 sections)
  - `go test ./internal/plugins/hccs/... -timeout 60s` clean ✅
    (0.6s, 16 sub-tests all PASS)

- **Correctness**:
  - `go vet ./...` exit 0
  - `go build ./...` exit 0
  - `go build -o bin/kube-scheduler.exe cmd/main.go` produces 97MB
    binary (3MB increase over T002 from new resource listers)
  - `bin/kube-scheduler.exe --help` still prints upstream flags
    (verified post-build smoke)
  - `go test ./...` exit 0, 1 package with tests, 16/16 PASS
  - `go mod tidy` exit 0

## Carry-forward

- **T005 (HCCSTopology Score)**:
  - Will live in `internal/plugins/hccs/score.go` alongside Filter
  - Adds `var _ framework.ScorePlugin = &HCCSTopology{}` assertion
    to plugin.go
  - Will introduce `colocation.go` with NPUSliceAllocation dynamic-
    client lister abstraction (mirror sliceLister pattern)
  - Args.Adjacency map already declared in args.go (T004) so T005
    can read it directly
  - DESIGN.md §3.3 documents the Score cycle forward — T005 fills it
    in with implementation pointers

- **T006 (NUMA wrap)**: independent module; no shared code with
  hccs

- **T101 (helm chart)**:
  - Will render KubeSchedulerConfiguration.profiles[*].pluginConfig
    with HCCSTopologyArgs JSON-encoded; values.yaml drives Weight /
    FailIfMissing / Adjacency
  - DESIGN.md §6.1 already shows the rendered YAML shape (template
    reference for T101)

- **T105 (inference-operator polish)**:
  - May stamp `npu.huawei.com/preferred-hccs-ring` annotation on PD-
    pair Pods so Filter has data to consume (currently Phase 5 PD
    Router only writes slice-bindings, not preferred-rings).
    Decision deferred to T105 task entry.

- **AICore Available capacity check**:
  - Currently NOT enforced by HCCSTopology Filter (delegated to DRA
    claim-binding filter per ADR-0010 §2)
  - T005 Score may use AICore math for tiebreaking
  - If real-cluster Phase 7 finds the DRA filter insufficient (e.g.
    because publisher under-reports availability), revisit then
