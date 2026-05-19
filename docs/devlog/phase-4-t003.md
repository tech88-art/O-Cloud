# P4-T-003 · npu-dra-driver Kubebuilder v4 scaffold

- **Commit**: 1f77243
- **Date**: 2026-05-19
- **Duration**: plan 1d · actual ~1h (scaffolding is faster than the plan estimates allow because pool-operator's existing Makefile + cmd/main.go layout is the template)

## Intent

Create the `operators/npu-dra-driver/` project skeleton (PROJECT + Makefile + Dockerfile + cmd/main.go + go.mod + hack/boilerplate.go.txt + .gitignore + README.md) mirroring `operators/pool-operator/` and `kubernetes-sigs/dra-example-driver` layouts. Manager binary should build, start, and idle cleanly (no controllers registered) so T004/T005/T006 can fill in api/ + internal/ packages without rewiring main.

## Path adaptations

**Plan vs reality — go.mod version drift**:
- Plan literal: `k8s.io/api v0.30.x` + `controller-runtime v0.18.x`
- pool-operator reality: `k8s.io/api v0.35.0` + `controller-runtime v0.23.3` + `go 1.25.7`
- Resolution: matched pool-operator's actual versions, not the plan's parenthetical. The plan's spirit was "match pool-operator versions" — the specific numbers in parens were stale guidance from when the plan was drafted. Documented this in the commit footer.

**Plan vs reality — repo path**:
- Plan literal: `github.com/tech88-art/O-Cloud/operators/npu-dra-driver`
- pool-operator existing: `github.com/example/ocloud-edge/operators/pool-operator` (placeholder from initial scaffold)
- Resolution: followed plan literal (matches actual deployed GitHub repo per memory). This means npu-dra-driver's go.mod path differs from pool-operator's — fine because Go modules are independent unless cross-importing (T102 confirms no cross-import needed).

## Debugging trail

- Initial PROJECT had `cliVersion: 4.14.0` (matched pool-operator's exact value, not the plan's "v4" string).
- cmd/main.go: had to decide on `--metrics-bind-address` default. pool-operator uses `"0"` (disabled); plan said `:8082`. Followed plan (npu-dra-driver is a new project; default-on metrics is reasonable + matches what T101 helm chart expects).
- **Reserved flags pattern**: Phase 4 plan demands `--enable-publisher` / `--enable-claim-controller` / `--mock-data-path` flags declared in T003 even though their bodies arrive in T005/T006. Resolved by registering the flags + emitting a "set but not yet wired" log if user sets them, then importing `internal/publisher/` + `internal/controller/` from T005/T006 onward. Avoids API churn between T003 and the wiring-up tasks.

## Key decisions

- **No internal/controller package yet** at T003 commit: main.go must NOT import `internal/controller` because it doesn't exist; if I had stubbed an empty package, T006 would have to delete + recreate. Instead I left a "kubebuilder:scaffold:builder" marker comment and ran with reserved flags only.
- **Apache 2.0 boilerplate**: matched pool-operator/hack/boilerplate.go.txt verbatim (Copyright 2026 placeholder). Required by controller-gen for generated files in T004+.
- **operators/CLAUDE.md update**: registered npu-dra-driver as the SECOND Kubebuilder sub-project at T003 commit. T103 will register inference-operator as the third.

## Verification

P3 三维度:
- Existence: `git ls-files operators/npu-dra-driver/` → 9 files + .gitignore
- Completeness: `go vet ./... → exit 0` · `go build → bin/manager 58 MB` · all 6 plan acceptance bullets checked
- Correctness: `./bin/manager --help` shows reserved flags with correct descriptions (publisher reserved P4-T-005 / claim-controller reserved P4-T-006 / mock-data-path reserved P4-T-005) and plan-literal defaults (:8081 health probe, :8082 metrics)

## Carry-forward

- T004 must add `utilruntime.Must(npuv1alpha1.AddToScheme(scheme))` to init() — done at T004 (api/v1alpha1 contains no CRD root types, but the scheme registration is still required if Phase 5+ ever adds one).
- T005 + T006 wire the previously-reserved flags from no-op to real. The flag descriptions update on each commit so `--help` output evolves task-by-task.
- T101 Dockerfile is a "scaffold stub" in T003; T101 updates header text and adds OCI labels but the build logic is already correct.
