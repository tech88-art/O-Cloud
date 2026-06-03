# P13-T-103 · backend 真拓扑聚合(k8s/crd GetTopology 真体 + real profile topology→k8s)

- **Commit**: <pending — main agent verifies + commits>
- **Date**: 2026-06-03
- **Duration**: plan 2d · actual ~0.4d (offline layer only; real-machine stamp is lab-gated, deferred — see Carry-forward)

## Intent
Replace the Phase-2 `ErrCapabilityUnavailable` stub in the k8s + crd datasources' `GetTopology`/`GetTopologyWithFabric` with real bodies that assemble the cluster→node→npu HCCS graph from live cluster objects, then flip the real profile's `mapping.topology` off `mock` onto the real source. The seam-above aggregator stays untouched behaviourally (ADR-0024 §2 Decision G): both real sources feed the SAME `aggregator.BuildTopology` the mock source drives.

## Path adaptations
- **ResourceSlice typed import avoided.** Plan/ADR-0024 §2 Decision C says k8s source reads ResourceSlice `hccs_ring`/`numa_node` device attrs. The backend pins `k8s.io/api v0.31.4`, which only ships `resource/v1alpha3` — NOT the `v1beta1` the npu-dra-driver publisher writes. Rather than add a cross-module typed dep (and pin a version that ADR-0024 §4(a) says is mid-migration v1beta1→v1), I read ResourceSlices via the **dynamic client** (unstructured), exactly mirroring how `crd.Source` already reads pool CRDs to stay module-boundary-clean. The reader probes `resource.k8s.io` versions v1→v1beta1→v1beta2→v1alpha3 in order and uses the first that lists actual slices — version-agnostic across the migration.
- **k8s.Source had no dynamic client.** Added a `dyn dynamic.Interface` field, wired in `NewSource` from the same `rest.Config` (so it shares the kubeconfig/in-cluster-SA chain). `main.go` is OUT of T103 Allowed Paths, so I did NOT touch wiring there — `NewSource` builds the dynamic client itself; no main.go change needed. Added `NewSourceWithClients(clientset, dyn, opts)` as the test seam (existing `NewSourceWithClient` unchanged → existing tests intact).
- **crd.Source is pool-only** (ListNodes/ListNPUs return ErrCapabilityUnavailable). Its `GetTopology` therefore SYNTHESIZES the cluster + node + npu nodes from `NPUPool.status.hccsTopology.peerGroups` device ids (convention `<node>-npu-<index>`, parsed on the LAST `-npu-` so hyphenated hostnames like `atlas-800-01` survive). hccsGroup is node-namespaced so two nodes' "ring-0" never collapse cross-node.
- **No slice layer on the k8s/crd topology path.** depth=slice renders cluster→node→npu (k8s) — the slice projection is the separate `crd.ResolveSlicesForNPUs` primitive, joined by a future aggregator-wiring task, not in scope here.

## Debugging trail
- **Pre-existing stale test**: `k8s/source_test.go TestStubMethods_ReturnErrCapabilityUnavailable` asserted GetTopology returns the stub error. Now false. Removed the two topology sub-cases (their behaviour moved to `topology_test.go`). `source_test.go` is the test for the file I'm editing; the edit is a direct consequence of the required change.
- **Dynamic fake panic**: `dynamic/fake` PANICS (not errors) on a List against an unregistered GVR. My version-probe tries v1 first; the fake only knew v1beta1 → panic. Fix: register ALL candidate ResourceSlice list kinds in the test helper (the fake's documented contract). A real apiserver returns a discovery error the loop already tolerates — the panic was a fake-only artifact.
- **Multi-version break-on-empty bug** (caught via the fake): my first loop broke on the first *served* version even when empty. A real cluster can serve both `v1` and `v1beta1` while the publisher wrote only one → breaking on the empty newer version would miss the slices under the older one. Fixed to break only on the first NON-empty result and fall through empties (a slice-less cluster probes all 4 cheaply, then falls back to the structural layout).
- **Seam grep tripped by my own comment**: the acceptance grep `if real|== "real"|profile` initially matched the package-header note where I'd written those literal words to *describe* the invariant. Reworded the comment to "source-agnostic / source-conditional / source-name check" so the literal grep is genuinely empty — the invariant is documented without the trigger tokens.

## Key decisions
- **Chose k8s as the real topology source** in the config (`topology: k8s`), with crd as a documented swap (`topology: crd`). Rationale: clusters/nodes/npus already route to k8s in the real profile, so topology→k8s keeps node/NPU identities consistent with the rest of the real graph; the k8s source reads the real-Ascend publisher's primary output (ResourceSlice attrs, ADR-0024 §2 Decision C). crd (NPUPool.status.hccsTopology) is fully implemented + tested as the alternative.
- **Best-effort ResourceSlice overlay** (k8s): when the publisher hasn't run / RBAC forbids the list / no slices exist, the overlay is empty and the NPUs keep npu.go's static 4-per-NUMA layout → the graph still renders (just with placeholder grouping, not real silicon adjacency). A forbidden list is non-fatal, NOT a 500. This is the seam-below "real source loads real data, gracefully" half of the invariant.
- **HCCS edges ride `IncludeFabric=true`** — identical to the mock source. The real sources only populate NPU.hccsGroup/HCCSBandwidthGBps; `appendHCCS` emits the ring. Zero-regression contract preserved: `GetTopology` == `GetTopologyWithFabric(opts{})` (no fabric, structural only), verified by test.
- Datasheet HCCS bandwidth nominal (56 GB/s, ADR-0021) stamped on enriched/synthesized NPUs so edge hover attrs are populated (apiserver exposes no measured bandwidth) — identical across demo/real/k8s/crd for hover consistency.

## Verification
**Offline layer (functional — this is where correctness is decided per plan §8):**
- `go build ./...` → OK; `go vet ./pkg/datasource/k8s/... ./pkg/datasource/crd/...` → clean.
- `go test ./...` → all PASS, no regressions. New tests: 7 k8s (`k8s/topology_test.go`), 9 crd (`crd/topology_test.go`), 3 aggregator real-path (`aggregator/topology_test.go`).
- `CGO_ENABLED=0 GOARCH=arm64 GOOS=linux go build ./...` → OK (no cgo introduced; pure dynamic-client reads).
- `make lint` (golangci-lint) → clean (see Verify chapter for tail).
- **Seam invariant** (headline AC): `grep -nE 'if real|== "real"|profile' pkg/aggregator/topology.go` → **empty** (exit 1). Aggregator runs one code path for mock + real; difference is only which Source `mapping.topology` selects.
- **Stub removed**: `grep -nE "GetTopology.*ErrCapabilityUnavailable" pkg/datasource/{k8s,crd}/source.go` → none.
- **Config flipped**: `config.real.yaml` + `deploy/profiles/real/demo-backend.values.yaml` both `topology: k8s` (was `mock`); ErrCapabilityUnavailable boundary comment removed.
- **demo profile regression**: mock source untouched; full `pkg/api` + `pkg/datasource/mock` suites green → `/api/v1/topology` demo path unaffected.

**Real-machine layer (LAB-GATED — NOT run, deferred per plan §8 / ADR-0024 §3):** the §4.4(3) "真 HCCS ring 与 `npu-smi info -t topo` 一致" stamp requires a real arm64 910B lab cluster with the T101 publisher writing ResourceSlices. T103 soft-depends on T101 (real ResourceSlice attrs); developed against the expected attr schema (`npu.huawei.com/hccs_ring` + `numa_node`, mirroring v1alpha1 Attr* constants). Lab stamp is the user's lab job (T301).

## Carry-forward
- **RBAC GAP (action needed for real-machine path)**: the backend ClusterRole (`tests/e2e/kind/manifests/demo-backend.yaml` lines 98-121) grants nodes/pods/pool-CRDs but NOT `resource.k8s.io/resourceslices`. The k8s topology source needs `get,list,watch` on `resourceslices` (apiGroup `resource.k8s.io`) for the HCCS overlay to read real attrs. Without it the source silently degrades to the static layout (non-fatal). Out of T103 Allowed Paths (deploy/tests territory) → flagged here for the RBAC/T301 wiring task. Same gap applies to the production demo-backend helm chart (`deploy/helm-charts/demo-backend/templates/rbac.yaml` only has the Lease Role; cluster-wide read RBAC lives in the kind manifest / a chart addition the deploy owner must make).
- **T101 dependency**: real ring fidelity depends on the real-Ascend publisher (P13-T-101) actually writing `hccs_ring`/`numa_node` onto ResourceSlice devices. If the lab npu-smi output makes T101's parser emit different ring numbering, the overlay reflects whatever T101 publishes (this source just reads it) — no T103 change needed.
- **crd swap**: to validate the NPUPool path on real hardware instead, flip `mapping.topology: crd` — requires pool-operator P6-T-003 aggregation populating `NPUPool.status.hccsTopology` (which itself calls the real Source.QueryTopology from T101).
- **No api-contract change** (as planned): edges (hccs/network) + attrs already exist from ADR-0021; this task only produces them from real data.
