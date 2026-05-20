# P6-T-006 · NumaAffinityPlugin (placeholder · upstream wrap deferred)

- **Commit**: <pending>
- **Date**: 2026-05-20
- **Duration**: plan 0.5d · actual ~30min (most spent diagnosing
  upstream API drift)

## Intent

Land a thin wrap of `sigs.k8s.io/scheduler-plugins/pkg/
noderesourcetopology` registered under our local Name "NumaAffinity"
per ADR-0010 §3 ("wrap, don't fork"). Operator-readability win:
chart KubeSchedulerConfiguration references "NumaAffinity" not
upstream's "NodeResourceTopologyMatch".

## Path adaptation: upstream wrap DEFERRED

**Outcome**: T006 ships a Name()-only placeholder, NOT the upstream
wrap originally planned. Reason: sched-plugins v0.31.8 (the latest
release at T006 entry) imports `framework.GVK` from K8s 1.31's
`pkg/scheduler/framework` — that symbol was removed in K8s 1.32.
Our `go.mod` replace block pins K8s libraries to v0.32.0 (matches
e2e-kind smoke per P5-T-114 baseline), so any direct import of
sched-plugins v0.31.x fails to build:

```
sigs.k8s.io/scheduler-plugins@v0.31.8/pkg/noderesourcetopology/plugin.go:133:54:
  undefined: framework.GVK
```

API drift table:

| sched-plugins | K8s target | framework.GVK | vs K8s 1.32 baseline       |
|---------------|-----------|---------------|------------------------------|
| v0.30.x       | 1.30      | present       | INCOMPATIBLE                 |
| v0.31.x       | 1.31      | present       | INCOMPATIBLE — observed fail |
| v0.32.x       | 1.32      | (removed)     | COMPATIBLE — not yet released |

Three options considered:

1. **Downgrade K8s to v0.31.0** — would unblock the wrap but break
   the rest of the scheduler-plugin module (DRA v1beta1 stable since
   K8s 1.32; downgrading to 1.31 means falling back to v1beta1-on-
   alpha-feature-gate). Net loss.
2. **Fork upstream noderesourcetopology** — adapt to K8s 1.32 API
   changes ourselves. Significant scope creep (~500 LOC + ongoing
   maintenance debt). ADR-0010 §3 explicitly chose "wrap, don't
   fork" because the upstream is mature; forking inverts that.
3. **Defer the wrap, ship placeholder** — T006 placeholder mirrors
   the T002 scaffold pattern. Document the deferral in DESIGN.md +
   ADR + devlog. Chart KubeSchedulerConfiguration omits
   NumaAffinity from default enabled lists. Operators wanting NUMA-
   aware scheduling can run sched-plugins binary as a second
   scheduler alongside ours (functional today, more operational
   overhead).

Selected: option 3. Rationale aligns with ADR §3's "Phase 6 first
wants to observe HCCS+NUMA behavior" — without NumaAffinity
enabled, the impact is limited to NUMA not being part of the score
mix, but HCCS scoring + Binpack still function. NUMA can be added
later via a v0.32.x bump without breaking the chart contract.

Updates landed:
- `internal/plugins/numa/plugin.go`: package doc fully documents
  the deferral (3 sections: drift table, operative state, forward
  path) + Name() constant + UpstreamName const (for future log-
  correlation when wrap lands)
- `internal/plugins/numa/plugin_test.go`: `TestNameConstants` +
  `TestNewProducesPlugin` (placeholder factory sanity)
- `DESIGN.md §5.2`: T006 status replaced from "wrap NumaAffinity"
  to "placeholder; upstream wrap deferred" + 3-section deferral doc
- `DESIGN.md §6.1`: KubeSchedulerConfiguration example updated to
  comment out NumaAffinity in filter/score enabled lists
- `docs/adr/0010-scheduler-plugin.md` §3 (NumaAffinity reuse):
  appended "T006 落地状态(2026-05-20 update)" paragraph documenting
  the deferral
- `go.mod`: sched-plugins direct dep added at experiment start,
  then removed via `go mod edit -droprequire=sigs.k8s.io/scheduler-
  plugins` + `go mod tidy` (transitive `k8s.io/apimachinery@latest`
  v0.36.1 also vanished, restoring v0.32.0 baseline)

## Debugging trail

- **`go get sigs.k8s.io/scheduler-plugins@v0.31.8`** initially
  succeeded; deps downloaded clean.
- **First `go vet ./...` after import**: `framework.GVK` undefined
  error in upstream's plugin.go line 133. K8s 1.32 source confirms
  the symbol was moved (probably to a typed event-handler API).
- **Tried v0.30.0 fallback**: same fail (K8s 1.30 framework wasn't
  module-compatible with our v0.32.0 replace block).
- **Considered patching upstream**: would require submitting
  upstream PR or maintaining a local fork. Beyond T006 scope.
- **Rolled back via `go mod edit -droprequire=...` + `go mod tidy`**:
  worked cleanly. go.mod is back to T005 baseline.
- **Test rewrite**: `New == nil` is always-false (function value
  is a typed nil sentinel, not literal nil); rewrote
  TestNewSignatureSanity → TestNewProducesPlugin which actually
  invokes the placeholder factory.

## Key decisions

- **Defer not block**: Phase 6 has 10+ remaining tasks; blocking
  on an upstream version is not the right tradeoff. Document the
  forward path so a future task can flip NUMA on with minimal
  churn (3 file edits: go.mod bump, plugin.go body, chart values).

- **Placeholder must satisfy framework.Plugin**: even as a no-op,
  registering nil from a factory crashes kube-scheduler at
  startup. The placeholder struct + Name() + factory keep
  registration safe; the framework simply never invokes Filter/
  Score because we don't implement those interfaces.

- **Chart change deferred to T101**: don't pre-emptively edit chart
  files in T006 — T101 owns the chart and will read DESIGN.md §6.1
  + values.yaml at chart authoring time. T006 only updates the
  EXAMPLE in DESIGN.md.

- **ADR-0010 updated, not superseded**: a partial deviation from
  ADR §3 (which assumed the wrap could land "without modification")
  warrants an inline update marker, not a new ADR revision. The
  v0.32.x wrap landing will be a code change, not a design change
  — same ADR §3 still applies.

## Verification

P3 三维度:

- **Existence**:
  - `git status --short` →
    M operators/scheduler-plugin/DESIGN.md
    M operators/scheduler-plugin/go.mod  (sched-plugins entry removed)
    M operators/scheduler-plugin/go.sum   (transitive cleanup)
    M operators/scheduler-plugin/internal/plugins/numa/plugin.go
    M docs/adr/0010-scheduler-plugin.md
    ?? operators/scheduler-plugin/internal/plugins/numa/plugin_test.go
    ?? docs/devlog/phase-6-t006.md
  - `grep -c 'sigs.k8s.io/scheduler-plugins' operators/scheduler-plugin/go.mod`
    → 0 (cleanly removed)

- **Completeness** (plan §3 P6-T-006 acceptance, adapted for deferral):
  - Upstream NodeResourceTopology Filter+Score reused without modification
    ⏳ DEFERRED to v0.32.x bump task (forward path documented)
  - Plugin factory registered as `NumaAffinity` ✅
    (placeholder factory + WithPlugin in cmd/main.go already in T002)
  - Args Weight default 2 per ADR-0010 §4 ⏳ DEFERRED (no Args struct
    until wrap lands)
  - Sanity test asserts plugin name + factory registration ✅
    (TestNameConstants + TestNewProducesPlugin)
  - `go test ./internal/plugins/numa/...` clean ✅ (2/2 PASS)

- **Correctness**:
  - `go vet ./...` exit 0
  - `go mod tidy` exit 0 (after the rollback)
  - `go test ./...` exit 0, all 28 sub-tests PASS (HCCS 26 + NUMA 2)
  - `go build ./...` exit 0

## Carry-forward

- **v0.32.x wrap landing**: future task (not in current Phase 6 plan)
  flips the placeholder body in `internal/plugins/numa/plugin.go`
  from `return &NumaAffinity{}, nil` to `return nrt.New(ctx, args, h)`.
  Three files touch: go.mod (require sched-plugins), plugin.go (body
  line), DESIGN.md §5.2 (status → operative) + §6.1 (uncomment
  NumaAffinity in KubeSchedulerConfiguration).

- **Plugin contract unchanged**: `Name` const ("NumaAffinity") stays —
  the wrap landing is a transparent body change. Chart values.yaml
  written by T101 can include a `numaAffinity.enabled` toggle
  defaulting to false; flip default to true after wrap lands.

- **Operator escape hatch documented**: DESIGN.md §5.2 + ADR-0010 §3
  update tell operators they can run sched-plugins binary as a second
  scheduler alongside ours. Suitable for clusters that need NUMA-
  aware scheduling NOW and can tolerate the operational overhead.

- **T007 (Binpack)**: unaffected — internal 50-LOC impl, no
  sched-plugins dependency.

- **T008 (integration tests)**: NUMA tier will be skipped in the
  integration matrix until wrap lands. Document in T008 test
  fixture.
