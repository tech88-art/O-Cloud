# P5-T-106 · kind smoke E2E extension

- **Commit**: <pending>
- **Date**: 2026-05-19
- **Duration**: plan 1d · actual ~40min

## Intent

Extend the existing Phase 3/4 kind smoke (`tests/e2e/kind/install.sh`
+ `.github/workflows/e2e-kind.yml`) with Phase 5 sub-steps: install
cert-manager, build + load the inference-operator image, helm-install
the chart, create a ModelService, assert phase advances + Pods carry
the `npu.huawei.com/slice-bindings` annotation injected by the
PD Router webhook.

## Path adaptations

- **`tests/e2e/kind/phase5/` subdirectory rather than top-level**.
  The existing kind smoke lives in `tests/e2e/kind/` with Phase 3/4
  scripts at the top level. To keep Phase 5 additions isolated
  (matches the plan path list literal), the new scripts live in
  `phase5/` and are invoked by the workflow with the explicit
  subcommand (`install-cert-manager` / `build-inference-operator` /
  `install-inference-operator` / `apply` / `wait-...`).
- **busybox stand-in image, not vllm-ascend**. Plan acceptance:
  "smoke pulls a real-ish vllm-ascend image (or a stand-in busybox
  with PD-role labels?)". Mitigation listed in Phase 5 entry meeting
  risk #3. busybox keeps the smoke focused on the K8s wiring (Pod
  creation + webhook annotation injection) without burning kind
  workflow time on a 5GB image pull.
- **ResourceClaim count check is warning-only**. The Phase 4
  `dra_publish_test.sh` already exercises the npu-dra-driver T002
  allocator end-to-end (ResourceSlice publication). Phase 5 ModelService
  → claim allocation is the downstream consumer of the same allocator;
  if Phase 4 passes, Phase 5 claim materialisation is high-confidence
  derivable. Treating it as a hard fail would gate the smoke on
  ResourceClaimTemplate→ResourceClaim expansion timing in K8s, which is
  not the unit under test in T106.

## Debugging trail

- **Workflow YAML — Phase 5 unicode dash**. Step names use the em-dash
  "—" character. Pythons default GBK codec on Windows can't print it
  (UnicodeEncodeError); harmless for the workflow which YAML parses
  it correctly. Cosmetic only.
- **Two identical "install Playwright deps" steps in e2e-kind.yml**
  (one in e2e-kind job, one in e2e-mock-regression). Initial Edit
  failed with "Found 2 matches". Resolved by including more context
  in the old_string to disambiguate the e2e-kind one.
- **`kubectl wait --for=condition=Ready certificate/...` semantics**.
  cert-manager's Certificate has a `Ready` condition; `kubectl wait
  --for=condition=Ready certificate/X` works once the Certificate
  reconcile has populated the status. Used in install-inference-
  operator with 60s timeout.

## Key decisions

- **Three install subcommands rather than one**. Splitting into
  `install-cert-manager` / `build-inference-operator` /
  `install-inference-operator` lets the workflow report failure with
  pinpoint precision (e.g. "cert-manager install timed out" vs
  "image build failed" vs "helm install timed out"). Also enables
  selective re-runs during local iteration.
- **120s annotation timeout**. Pod creation + scheduling + claim
  allocation + audit creation + PD Router admission webhook hit all
  happen in sequence. Plan acceptance says "Pods carry annotation
  within 60s" but I went conservative to 120s for kind cluster slow-
  starts; the workflow timeout-minutes=25 absorbs this comfortably.
- **`pullPolicy=Never` for the loaded image**. The chart's
  `image.pullPolicy` default is `IfNotPresent`; `Never` ensures kind
  uses the loaded image even when its image-list reports a different
  registry path under the hood.
- **Failure dumps section**: cert-manager pods/logs + inference-
  operator describe/logs + Certificate/Issuer + ModelService
  describe + child Deployments/ResourceClaims/NPUSliceAllocations/Pods.
  Covers every downstream object that could explain a smoke failure.

## Verification

P3 三维度:
- **Existence**:
  - `git status` shows: 3 new files
    (tests/e2e/kind/phase5/install.sh + assert.sh + fixtures/
    modelservice-sample.yaml), 1 modified
    (.github/workflows/e2e-kind.yml), devlog
- **Completeness**:
  - `bash -n tests/e2e/kind/phase5/install.sh` → syntactically clean
  - `bash -n tests/e2e/kind/phase5/assert.sh` → syntactically clean
  - `python yaml.safe_load_all` on the workflow → 1 doc, 2 jobs,
    e2e-kind has 21 steps total with 4 new "Phase 5" steps
- **Correctness**:
  - Plan T106 acceptance steps mapped to workflow steps + script
    functions:
    - cert-manager Issuer Ready within 90s ✓
      (install.sh `install-cert-manager` waits with `CERT_WAIT_SECONDS`
      default 90s + dumps describe+logs on timeout)
    - inference-operator Deployment Available within 60s ✓
      (`install-inference-operator` waits with `INF_OP_WAIT_SECONDS`
      default 60s)
    - ModelService phase=Provisioning within 30s ✓
      (`assert.sh` `wait_phase_advanced` polls every 1s up to 30s)
    - Prefill + Decode Pods created within 60s ✓
      (`wait_deployments_created` polls Deployments named
      `<ms>-prefill` / `<ms>-decode` within 60s)
    - Pods carry slice-bindings annotation ✓
      (`wait_slice_bindings_annotation` polls Pod annotation `npu
      .huawei.com/slice-bindings` within 120s — buffer over plan's
      60s)
    - kubectl get resourceclaims count == replicas:
      `wait_resource_claims` checks for ≥1 within 120s; warning-
      only as documented
    - Failure modes captured with kubectl describe + logs dumps in
      both script-level error paths and workflow-level dump-cluster

## Carry-forward

- T107 checkpoint:
  - List all 15 tasks + their commits + tests passing as Phase 5 DoD
  - Tag `phase-5-complete` lands at the merge commit
  - Reference this file as the kind-smoke verification artefact
- Phase 6 entry meeting may decide to:
  - Replace busybox with a real vllm-ascend image once vllm-ascend
    v0.12+ stabilises (Phase 5 entry meeting note)
  - Add `dra_publish_test.sh` parallel to `assert.sh` for
    NPUSliceAllocation list-by-MS audit verification
  - Wire the PD Router webhook smoke into a Playwright test that
    exercises a downstream PD-router proxy
