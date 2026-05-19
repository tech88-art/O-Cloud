# P4-T-104 · kind smoke E2E extension (npu-dra-driver + ResourceSlice publication assert)

- **Commit**: a9c1d11
- **Date**: 2026-05-19
- **Duration**: plan 1d · actual ~1.5h

## Intent

Extend the existing Phase 3 e2e-kind workflow to install + smoke-test the npu-dra-driver chart (build+load image → helm install → rollout-status → ResourceSlice publication assertions via dra_publish_test.sh). Plus parity `--with-dra-driver` flag on the dev-side `scripts/install.sh`.

## Path adaptations

**Two distinct install.sh files in this repo**:
- Plan said: `scripts/install.sh --with-dra-driver` should be the CI install path.
- Reality: CI workflow `.github/workflows/e2e-kind.yml` calls `tests/e2e/kind/install.sh up` (kind-specific bootstrap), NOT `scripts/install.sh` (dev single-node installer). They serve different audiences.
- Resolution: touched BOTH —
  1. `tests/e2e/kind/install.sh` (actual CI driver) — extended `up` to helm install npu-dra-driver + `build-images` to docker-build + kind-load it
  2. `scripts/install.sh` (dev installer) — added `--with-dra-driver` + `--all-phase-4` flags for dev parity per plan literal

**Plan path `test/e2e/dra_publish_test.sh` (singular) → reality `tests/e2e/kind/` (plural)**: kind-specific assertions live alongside `seed-resources.sh` per Phase 3 P3-T-104 convention.

## Debugging trail

- **NPU_DRA_IMG split via cut**. The `helm upgrade --install ... --set image.repository=... --set image.tag=...` invocation needs the image reference split. Resolution:
  ```
  --set image.repository="$(echo "${NPU_DRA_IMG}" | cut -d: -f1)" \
  --set image.tag="$(echo "${NPU_DRA_IMG}" | cut -d: -f2)"
  ```
- **wait-for-ready loop tuning**. Plan acceptance: "poll `kubectl rollout status deployment/npu-dra-driver` with 120s timeout". Used `--timeout=2m` on `kubectl rollout status` after helm install. Then added a separate 60s poll-loop (12×5s sleep) to wait for at least 1 matching ResourceSlice before invoking dra_publish_test.sh — handles the case where the rollout reports ready but the publisher's first Reconcile pass hasn't fired yet.
- **Failure-dump groups**. Existing e2e-kind failure-cleanup step had groups for all-resources / pool-CRDs / operator-logs / exporter-logs / backend-logs. Added 2 new groups: `npu-dra-driver logs --tail=300` + `kubectl get resourceslices -o yaml`. Now triage on red builds shows exactly what npu-dra-driver did and what ResourceSlices ended up in etcd.

## Key decisions

- **--all-phase-4 aggregate flag** in scripts/install.sh. Aggregate of `--with-prometheus` + `--with-dra-driver`. Composable with other --with-* flags so callers can still add. Phase 5 may add `--with-inference-operator` and the aggregate flag extends.
- **Refresh deployments step extended (not new step)**. Existing Phase 3 step `kubectl rollout restart deploy/{pool-operator,demo-backend}` + `ds/ascend-npu-exporter-plus` — added 4 lines (1 restart + 1 status). Co-locates the restart pattern so a future maintainer sees all 4 deployments handled identically.
- **dra_publish_test.sh prefers jq over kubectl jsonpath**. Plan: "kubectl get resourceslices -o json | jq". Used full jq expressions for the 4 assertions because jq's error messages are richer than jsonpath empty-output traps. `jq` is preinstalled on GitHub Actions ubuntu-22.04.

## Verification

P3 三维度:
- Existence: `git ls-files .github/workflows/e2e-kind.yml scripts/install.sh tests/e2e/kind/{install,dra_publish_test}.sh`
- Completeness: `bash -n` syntax check passes on all 3 modified/new scripts
- Correctness: shellcheck not available locally (NOT-blocking); CI on push will exercise end-to-end. The new "assert ResourceSlice publication (P4-T-104)" step has a 60s pre-poll for slice presence + invokes dra_publish_test.sh.

## Carry-forward

- T106 reuses this workflow's structure for backend /metrics smoke: another assert step + `tests/e2e/kind/backend_metrics_test.sh`. Same pattern (curl + grep).
- Phase 5 inference-operator's e2e will likely add a third assert step (kubectl get modelservices + check phase transition). Workflow timeout (25 min) should still hold.
- If kind cluster startup ever exceeds 8 minutes (plan budget for the dra-driver portion specifically), revisit the helm install --timeout 3m + the wait-for-slice 60s loop to see where the budget overflows.
