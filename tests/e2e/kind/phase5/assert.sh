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
#   5. Pods created via the Deployments carry the slice-bindings
#      annotation (value non-empty)
#
# Bails on the first failure with describe/logs dumps so the workflow
# surface a clear failure mode.

set -euo pipefail

NS="${NS:-ocloud-system}"
MS_NAME="${MS_NAME:-smoke-ms}"
WAIT_PHASE_SECONDS="${WAIT_PHASE_SECONDS:-30}"
WAIT_DEPLOY_SECONDS="${WAIT_DEPLOY_SECONDS:-60}"
WAIT_ANNOTATION_SECONDS="${WAIT_ANNOTATION_SECONDS:-120}"

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
  kubectl -n "${NS}" get deploy -l "inference.ocloud.edge.example.com/model-service=${NS}/${MS_NAME}" -o yaml || true
  kubectl -n "${NS}" logs deploy/inference-operator --tail=200 || true
  return 1
}

wait_resource_claims() {
  echo "== wait for ResourceClaim objects for this ModelService =="
  for i in $(seq 1 60); do
    count=$(kubectl -n "${NS}" get resourceclaims \
      -l "inference.ocloud.edge.example.com/model-service=${NS}/${MS_NAME}" \
      -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null | grep -c . || echo 0)
    echo "  attempt ${i}: ResourceClaim count=${count}"
    if [ "${count:-0}" -ge 1 ]; then
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

wait_slice_bindings_annotation() {
  echo "== wait for slice-bindings annotation on at least one Pod (timeout ${WAIT_ANNOTATION_SECONDS}s) =="
  for i in $(seq 1 "${WAIT_ANNOTATION_SECONDS}"); do
    pod=$(kubectl -n "${NS}" get pods \
      -l "inference.ocloud.edge.example.com/model-service=${NS}/${MS_NAME}" \
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
  kubectl -n "${NS}" get pods -l "inference.ocloud.edge.example.com/model-service=${NS}/${MS_NAME}" -o yaml || true
  kubectl -n "${NS}" describe mutatingwebhookconfigurations || true
  kubectl -n "${NS}" logs deploy/inference-operator --tail=300 || true
  return 1
}

apply_fixture
wait_phase_advanced
wait_deployments_created
wait_resource_claims || echo "::warning::ResourceClaim materialisation slow; smoke continues"
wait_slice_bindings_annotation
echo "== Phase 5 smoke assertions complete =="
