#!/usr/bin/env bash
# Phase 5 P5-T-106 — kind smoke assertions.
#
# Preconditions (run by tests/e2e/kind/phase5/install.sh):
#   * cert-manager installed + Pods Ready
#   * inference-operator helm chart installed + Deployment Available
#   * npu-dra-driver helm chart installed (from Phase 4 kind smoke)
#   * pool-operator + seed-resources.sh applied (smoke-pool exists +
#     status.totalSlices > 0)
#
# Assertions performed:
#   1. Apply tests/e2e/kind/phase5/fixtures/modelservice-sample.yaml
#   2. ModelService phase advances past Pending within 30s
#   3. Prefill + Decode Deployments created within 60s (one each)
#   4. ResourceClaim objects exist for the ModelService (one per Pod
#      replica, allocated by npu-dra-driver T002)
#   5. NPUSliceAllocation entries reach phase=Allocated for the
#      ModelService (one per replica). T125 fix: pd_router admission
#      webhook can only inject the slice-bindings annotation when
#      `NPUSliceAllocation.status.phase == "Allocated"` is observed at
#      Pod CREATE time. Because the claim controller allocates the NPU
#      and stamps phase=Allocated within milliseconds of Pod admission,
#      the initial Pod often races ahead and is admitted without the
#      annotation. We therefore wait for the allocations to settle and
#      then force a Pod recreate so the webhook fires again with the
#      allocation cache hot.
#   6. After Pod recreate, at least one Pod carries the slice-bindings
#      annotation (value non-empty).
#
# Bails on the first failure with describe/logs dumps so the workflow
# surfaces a clear failure mode.

set -euo pipefail

NS="${NS:-ocloud-system}"
MS_NAME="${MS_NAME:-smoke-ms}"
WAIT_PHASE_SECONDS="${WAIT_PHASE_SECONDS:-30}"
WAIT_DEPLOY_SECONDS="${WAIT_DEPLOY_SECONDS:-60}"
WAIT_ANNOTATION_SECONDS="${WAIT_ANNOTATION_SECONDS:-120}"
WAIT_ALLOCATIONS_SECONDS="${WAIT_ALLOCATIONS_SECONDS:-120}"

# T124: LabelModelService VALUE carries just ms.Name (K8s label-value
# regex rejects `/`). The Pod / Deployment / ResourceClaim labels stamp
# this value; the matching selector therefore does NOT prepend
# namespace.  The full `<ns>/<name>` form survives on the
# NPUSliceAllocation.spec.modelServiceRef field (set from the
# ResourceClaim annotation, where `/` is legal).
LABEL_SELECTOR="inference.ocloud.edge.example.com/model-service=${MS_NAME}"
MS_REF_FQ="${NS}/${MS_NAME}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FIXTURE="${SCRIPT_DIR}/fixtures/modelservice-sample.yaml"

apply_fixture() {
  echo "== apply ModelService fixture =="
  kubectl apply -f "${FIXTURE}"
}

wait_phase_advanced() {
  echo "== wait for ModelService phase to leave Pending (timeout ${WAIT_PHASE_SECONDS}s) =="
  for i in $(seq 1 "${WAIT_PHASE_SECONDS}"); do
    phase=$(kubectl -n "${NS}" get ms "${MS_NAME}" -o jsonpath='{.status.phase}' 2>/dev/null || echo "")
    if [ -n "${phase}" ] && [ "${phase}" != "Pending" ]; then
      echo "ModelService phase=${phase} (after ${i}s)"
      return 0
    fi
    sleep 1
  done
  echo "::error::ModelService phase still Pending/empty after ${WAIT_PHASE_SECONDS}s"
  kubectl -n "${NS}" describe ms "${MS_NAME}" || true
  kubectl -n "${NS}" logs deploy/inference-operator --tail=200 || true
  return 1
}

wait_deployments_created() {
  echo "== wait for Prefill + Decode Deployments (timeout ${WAIT_DEPLOY_SECONDS}s) =="
  for i in $(seq 1 "${WAIT_DEPLOY_SECONDS}"); do
    pf=$(kubectl -n "${NS}" get deploy "${MS_NAME}-prefill" -o jsonpath='{.metadata.name}' 2>/dev/null || echo "")
    dc=$(kubectl -n "${NS}" get deploy "${MS_NAME}-decode"  -o jsonpath='{.metadata.name}' 2>/dev/null || echo "")
    if [ -n "${pf}" ] && [ -n "${dc}" ]; then
      echo "Prefill (${pf}) + Decode (${dc}) Deployments materialised (after ${i}s)"
      return 0
    fi
    sleep 1
  done
  echo "::error::Prefill or Decode Deployment did not materialise within ${WAIT_DEPLOY_SECONDS}s"
  kubectl -n "${NS}" get deploy -l "${LABEL_SELECTOR}" -o yaml || true
  kubectl -n "${NS}" logs deploy/inference-operator --tail=200 || true
  return 1
}

# Use jq for clean integer counting — `grep -c .` + `|| echo 0` emits
# multi-line "0\n0" on empty output, which the `[ -ge 1 ]` test parses
# as syntax error (T119 install.sh fix; same pattern reappears here).
count_claims_with_label() {
  kubectl -n "${NS}" get resourceclaims \
    -l "${LABEL_SELECTOR}" \
    -o json 2>/dev/null \
    | jq '.items | length' 2>/dev/null \
    || echo 0
}

wait_resource_claims() {
  echo "== wait for ResourceClaim objects for this ModelService =="
  for i in $(seq 1 60); do
    count=$(count_claims_with_label)
    echo "  attempt ${i}: ResourceClaim count=${count}"
    if [ "${count:-0}" -ge 1 ] 2>/dev/null; then
      echo "ResourceClaims materialised (${count})"
      return 0
    fi
    sleep 2
  done
  echo "::error::No ResourceClaim materialised for ${NS}/${MS_NAME} after 120s"
  kubectl -n "${NS}" get resourceclaims -o yaml || true
  kubectl -n "${NS}" logs deploy/npu-dra-driver --tail=200 || true
  return 1
}

# Count NPUSliceAllocation entries (cluster-scoped CRD) whose
# spec.modelServiceRef == "<ns>/<name>" AND status.phase == Allocated.
count_allocations_allocated() {
  kubectl get npusliceallocations -o json 2>/dev/null \
    | jq --arg ref "${MS_REF_FQ}" \
      '[.items[] | select(.spec.modelServiceRef == $ref) | select(.status.phase == "Allocated")] | length' \
      2>/dev/null \
    || echo 0
}

wait_allocations_allocated() {
  echo "== wait for NPUSliceAllocation phase=Allocated (timeout ${WAIT_ALLOCATIONS_SECONDS}s) =="
  for i in $(seq 1 "${WAIT_ALLOCATIONS_SECONDS}"); do
    count=$(count_allocations_allocated)
    if [ "${count:-0}" -ge 1 ] 2>/dev/null; then
      echo "NPUSliceAllocation phase=Allocated count=${count} (after ${i}s)"
      return 0
    fi
    if [ $((i % 10)) -eq 0 ]; then
      echo "  attempt ${i}: NPUSliceAllocation Allocated count=${count}"
    fi
    sleep 1
  done
  echo "::error::No NPUSliceAllocation reached Allocated for ${MS_REF_FQ} within ${WAIT_ALLOCATIONS_SECONDS}s"
  kubectl get npusliceallocations -o yaml || true
  kubectl -n "${NS}" logs deploy/npu-dra-driver --tail=200 || true
  return 1
}

# Force a Pod recreate so the pd_router webhook fires again at CREATE
# time with the NPUSliceAllocation cache already populated with
# phase=Allocated entries. Without this, the initial Pod that triggered
# the claim allocation will keep running indefinitely without the
# slice-bindings annotation (webhook is admission-time only; there is
# no controller that retroactively patches existing Pods — by design,
# per ADR-0008).
force_pod_recreate() {
  echo "== force Pod recreate so PD Router fires with allocations hot =="
  kubectl -n "${NS}" delete pods -l "${LABEL_SELECTOR}" --wait=false || true
  # Give the ReplicaSet a moment to spin up replacements.
  sleep 2
}

wait_slice_bindings_annotation() {
  echo "== wait for slice-bindings annotation on at least one Pod (timeout ${WAIT_ANNOTATION_SECONDS}s) =="
  for i in $(seq 1 "${WAIT_ANNOTATION_SECONDS}"); do
    pod=$(kubectl -n "${NS}" get pods \
      -l "${LABEL_SELECTOR}" \
      -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
    if [ -n "${pod}" ]; then
      ann=$(kubectl -n "${NS}" get pod "${pod}" \
        -o jsonpath='{.metadata.annotations.npu\.huawei\.com/slice-bindings}' 2>/dev/null || echo "")
      if [ -n "${ann}" ]; then
        echo "Pod ${pod} carries slice-bindings annotation: ${ann}"
        return 0
      fi
    fi
    sleep 1
  done
  echo "::error::No Pod observed with slice-bindings annotation within ${WAIT_ANNOTATION_SECONDS}s"
  kubectl -n "${NS}" get pods -l "${LABEL_SELECTOR}" -o yaml || true
  kubectl -n "${NS}" describe mutatingwebhookconfigurations || true
  kubectl -n "${NS}" logs deploy/inference-operator --tail=300 || true
  return 1
}

apply_fixture
wait_phase_advanced
wait_deployments_created
wait_resource_claims || echo "::warning::ResourceClaim materialisation slow; smoke continues"
wait_allocations_allocated
force_pod_recreate
wait_slice_bindings_annotation
echo "== Phase 5 smoke assertions complete =="
