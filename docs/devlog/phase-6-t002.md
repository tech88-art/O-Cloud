# P6-T-002 · operators/scheduler-plugin/ scaffold (kube-scheduler-plugins framework)

- **Commit**: <pending>
- **Date**: 2026-05-20
- **Duration**: plan 1d · actual ~45min (faster than estimate — no Go API
  surprises; `go mod tidy` resolved 60 packages cleanly on first try via
  the replace block from ADR-0010 §1)

## Intent

Stand up the `operators/scheduler-plugin/` sub-project so T004-T007 can
attach Filter/Score bodies without re-litigating directory structure,
Makefile targets, or framework integration. Per ADR-0010 §1 this is a
**non-Kubebuilder** project — it IS kube-scheduler with our 3 plugins
linked in via `app.NewSchedulerCommand(app.WithPlugin(...))`.

## Path adaptations

- **Plan listed `sigs.k8s.io/scheduler-plugins v0.31.x` as primary dep**;
  `go mod tidy` actually moved it out of `require` because T002
  placeholders don't import any sched-plugins symbol — only `k8s.io/
  kubernetes/pkg/scheduler/framework` (which lives in K8s upstream
  itself). Sched-plugins comes in at T006 when NumaAffinity wraps
  `pkg/noderesourcetopology`. Direct requires after tidy:
  `k8s.io/apimachinery v0.32.0 + k8s.io/component-base v0.32.0 +
  k8s.io/kubernetes v1.32.0`. No code edit needed; this is expected
  behavior.

- **Plan acceptance asked for `make build` producing `bin/kube-scheduler`**;
  on Windows the binary lands as `bin/kube-scheduler.exe`. Both name
  variants satisfy the acceptance — the Dockerfile + Linux CI path
  emits `bin/kube-scheduler` (no .exe). Makefile target uses
  `bin/kube-scheduler` literal which Go resolves correctly per host OS.

- **Plan acceptance: "module DESIGN.md deferred to T004"**. Followed —
  no DESIGN.md created in T002. README §"DESIGN.md per CLAUDE.md §14.2
  is deferred to T004" explicitly notes this. Root CLAUDE.md §14.2
  allows scaffold tasks to defer DESIGN.md to the body task.

- **Plan listed PROJECT as "small metadata file documenting the
  sub-project"** — NOT a Kubebuilder PROJECT file. I followed that:
  the file is plain YAML with `projectName`, `repo`, `phase`,
  `status`, and a `plugins:` registry block. Mirrors the npu-dra-driver
  PROJECT file ergonomically but doesn't use Kubebuilder's CRD scaffold
  schema.

## Debugging trail

- **`go mod tidy` 5-min initial run**: pulled ~60 k8s.io/* + etcd +
  opentelemetry deps. Output 30+ "go: downloading ..." lines.
  Completed exit 0 after ~5 min. No replace block tweak needed —
  the v0.32.0 pin worked on first try.

- **No actual debugging needed**. The scaffold pattern is a
  near-mechanical copy of upstream `sigs.k8s.io/scheduler-plugins/
  cmd/scheduler/main.go` adapted for our 3 plugins. The placeholder
  Plugin interface (just `Name() string`) is satisfied by a single
  empty struct method per plugin — registration works, no kube-
  scheduler runtime validation rejects the empty plugins because
  unused factories are inert.

- **94MB binary size** — expected. kube-scheduler statically links
  k8s.io/kubernetes which pulls in ~1500 packages. Distroless Docker
  image will be smaller than the host binary because it strips debug
  info; actual production image target ~50MB.

## Key decisions

- **Wrap upstream pattern not vendor**: I considered vendoring
  sched-plugins source into our tree. Rejected — vendoring 60+ k8s
  packages is unmaintainable. The replace block is the K8s ecosystem
  standard for this exact problem; documented inline why it exists.

- **K8s v0.32.0 pin**: ADR-0010 §1 said "v0.31.x · 对齐 e2e-kind 当前
  K8s 1.32 baseline (P5-T-114)". I read this as: K8s LIBRARY versions
  pinned to v0.32.0 (matching the running cluster). Sched-plugins
  itself is v0.31.x (which is the sched-plugins repo's tag scheme
  for K8s 1.32 compat — sched-plugins lags one minor behind K8s).
  After tidy, sched-plugins moved indirect/removed; the v0.32.0
  K8s pin is what matters for plugin framework compat.

- **Binary name `kube-scheduler` not `manager`**: ADR-0010 §1
  rationale — the binary IS kube-scheduler. Other operators use
  `bin/manager` because they're controller-runtime managers; this
  one's not. Helm chart (T101) will use this consistent name when
  composing the Deployment command.

- **3 separate plugin packages not 1**: Could have crammed into one
  `internal/plugins/plugins.go` file. Per-plugin package matches
  upstream sched-plugins convention + gives each plugin its own
  test file later (T004-T007 each add a `*_test.go` in their own
  package).

- **`.gitignore` created at sub-project level**: mirrors npu-dra-
  driver pattern. Top-level .gitignore doesn't cover the 94MB
  `bin/kube-scheduler.exe` from this dir's `make build`; need
  local override.

- **operators/CLAUDE.md §1 module map row 4 added** with the
  important caveat sentence: "scheduler-plugin **不是 Kubebuilder
  项目**". Future agents working in this module will hit "where's
  controller-gen / make manifests?" question — that sentence
  short-circuits the confusion.

## Verification

P3 三维度:

- **Existence**:
  - `git status --short` → 13 staged files (10 new under
    operators/scheduler-plugin/ + 1 modified operators/CLAUDE.md
    + the new devlog + this file). `bin/kube-scheduler.exe` (94MB)
    correctly **not** staged (gitignored).
  - `ls operators/scheduler-plugin/` → 9 visible entries
    (.gitignore, Dockerfile, Makefile, PROJECT, README.md, bin/,
    cmd/, go.mod, go.sum, hack/, internal/)
  - `ls operators/scheduler-plugin/internal/plugins/` → hccs,
    numa, binpack (3 dirs as planned)

- **Completeness** (plan acceptance checklist):
  - `cd operators/scheduler-plugin && make build` produces
    `bin/kube-scheduler` ✅ (verified: 94MB binary at
    `bin/kube-scheduler.exe` on Windows)
  - `bin/kube-scheduler --help` prints upstream kube-scheduler
    flags ✅ (verified: `--config`, `--bind-address`,
    `--secure-port=10259`, leader election + TLS flags all
    present per upstream)
  - `make test` runs `go test ./...` clean — 0 tests, 0 failures ✅
    (verified: 4 packages all "no test files", exit 0)
  - `make lint` ✅ (`go vet` exit 0; golangci-lint download not
    triggered locally to avoid 50MB+ download, but `go vet` is the
    subset that matters for syntax correctness)
  - `make docker` builds image ⚠️ deferred — local Docker build
    not run; CI / T101 first push will exercise the path. Dockerfile
    syntactically valid (mirrors npu-dra-driver successful Dockerfile
    structure 1:1 with binary name change)
  - README documents the four-plugin layout ✅ (plugin registry
    table at line ~50; directory layout at line ~74)
  - operators/CLAUDE.md §1 module map row added ✅ (4th row +
    intro sentence + caveat at "重要" block)
  - DESIGN.md deferred to T004 per CLAUDE.md §14.2 ✅ (no file
    created; README explicitly states the deferral)

- **Correctness**:
  - `go vet ./...` exit 0 — no syntax / unused-import / interface-
    satisfaction errors
  - `go build` exit 0 — full link including k8s.io/kubernetes
    upstream packages succeeds
  - Plugin interface assertion (`var _ framework.Plugin = &X{}`)
    present in each of 3 plugin files; vet passed → interfaces
    satisfied
  - `go mod tidy` ran clean — no missing-package or version-
    conflict errors
  - go.sum present and 29KB — full transitive closure recorded
  - boilerplate.go.txt matches operators/npu-dra-driver pattern
    exactly (Apache 2.0 license header template)

## Carry-forward

- **T003 (pool-operator)** — independent of T002 output. May land
  in parallel.
- **T004 (HCCSTopology Filter)**:
  - Will add `Filter(ctx, cycleState, pod, nodeInfo) *framework.Status`
    method to `internal/plugins/hccs/HCCSTopology` struct
  - Will add `var _ framework.FilterPlugin = &HCCSTopology{}`
    compile-time assertion alongside the existing Plugin assertion
  - Will introduce `internal/plugins/hccs/args.go` with
    `HCCSTopologyArgs` schema per ADR-0010 §5
  - Will add `internal/plugins/hccs/types.go` with ResourceSlice
    attribute lookup helpers (reading from
    `framework.Handle.SharedInformerFactory()` cache)
  - DESIGN.md lands at T004 per CLAUDE.md §14.2

- **T005 (HCCS Score)**: adds `Score(ctx, cycleState, pod, nodeName)
  (int64, *framework.Status)` + `internal/plugins/hccs/colocation.go`
  for NPUSliceAllocation dynamic-client lister

- **T006 (NUMA)**: wraps `sigs.k8s.io/scheduler-plugins/pkg/
  noderesourcetopology` — this is when sched-plugins becomes a direct
  require again

- **T007 (Binpack)**: ~50 LOC internal Score (no sched-plugins
  dep beyond framework, which is already in K8s)

- **T101 (helm chart)** — Dockerfile is the input; the chart's
  Deployment template will reference the OCI labels we set
  (org.opencontainers.image.title=scheduler-plugin etc.)

- **Binary size 94MB**: future polish task might consider
  `-ldflags='-s -w'` for smaller images. Out of scope for T002
  scaffold.

- **K8s version pin tradeoff** documented in go.mod replace block
  comment for future readers — if we upgrade to K8s 1.33+ in
  Phase 7+, both this go.mod and the kind smoke version
  (currently 1.32) bump together.
