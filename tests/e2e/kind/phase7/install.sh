#!/usr/bin/env bash
# Phase 7 P7-T-103 + P7-T-104 — kind smoke installer.
#
# Run AFTER tests/e2e/kind/phase6/install.sh so the cluster has:
#   * Phase 4 npu-dra-driver chart (publishing ResourceSlices from mock JSON)
#   * Phase 5 cert-manager + inference-operator (with T003 schedulerName
#     auto-stamp per Phase 7 P7-T-003)
#   * Phase 6 scheduler-plugin (HCCSTopology Filter+Score + Binpack
#     opt-in + NumaAffinity placeholder; T008 chart default adjacency
#     applied per ADR-0010 §256 closure)
#
# Phase 7 layering:
#   1. (T104) reseed npu-dra-driver mock data with set-b-multi-ring
#      (4 HCCS rings across 2 nodes) — exercises 4-tier adjacency scoring
#   2. (T104) label kind worker nodes with synthetic ring annotation for
#      `kubectl get nodes -L ...` operator readability
#   3. (T103) apply NPUSliceTemplate qwen-8b-pd-pair → wait for
#      template_controller to stamp Validated + Allocatable
#   4. (T103) apply ModelService qwen-8b-multi-ring → wait for
#      deployment_builder to create PD-pair Deployments
#
# Bails on first failure with kubectl describe / logs dump.

set -euo pipefail

KIND_CLUSTER="${KIND_CLUSTER:-ocloud-e2e}"
NS_INF="${NS_INF:-ocloud-system}"
WAIT_NST_SECONDS="${WAIT_NST_SECONDS:-30}"
WAIT_DEPLOY_SECONDS="${WAIT_DEPLOY_SECONDS:-60}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"
FIX_NST="${SCRIPT_DIR}/fixtures/npuslicetemplate-qwen-pd.yaml"
FIX_MS="${SCRIPT_DIR}/fixtures/modelservice-with-template.yaml"
SET_B_NPUS="${REPO_ROOT}/configs/mock-data/set-b-multi-ring/npus.json"

cmd_reseed_mockdata() {
  echo "== Phase 7 T104: reseed npu-dra-driver mock data with set-b-multi-ring =="
  # Replace the ConfigMap key `npus.json` with set-b-multi-ring's
  # npus.json so MockJSONSource publishes 4-ring topology.
  kubectl -n kube-system create configmap npu-dra-mock \
    --from-file=npus.json="${SET_B_NPUS}" \
    --dry-run=client -o yaml | kubectl apply -f -
  echo "== restart npu-dra-driver to reload mock data =="
  kubectl -n kube-system rollout restart deploy/npu-dra-driver
  kubectl -n kube-system rollout status deploy/npu-dra-driver --timeout="${WAIT_DEPLOY_SECONDS}s"
}

cmd_label_nodes() {
  echo "== Phase 7 T104: label kind worker nodes with synthetic HCCS rings =="
  kubectl label node ocloud-e2e-worker  ocloud.edge.example.com/synthetic-hccs-ring="0,1" --overwrite || true
  kubectl label node ocloud-e2e-worker2 ocloud.edge.example.com/synthetic-hccs-ring="2,3" --overwrite || true
}

cmd_apply_template() {
  echo "== Phase 7 T103: apply NPUSliceTemplate fixture =="
  kubectl apply -f "${FIX_NST}"
  echo "== wait for Validated=True (timeout ${WAIT_NST_SECONDS}s) =="
  kubectl wait --for=jsonpath='{.status.conditions[?(@.type=="Validated")].status}'=True \
    --timeout="${WAIT_NST_SECONDS}s" \
    npust qwen-8b-pd-pair
}

cmd_apply_modelservice() {
  echo "== Phase 7 T103: apply ModelService fixture (with slice-template label) =="
  kubectl -n "${NS_INF}" apply -f "${FIX_MS}"
  echo "== wait for prefill + decode Deployments to exist (timeout ${WAIT_DEPLOY_SECONDS}s) =="
  for side in prefill decode; do
    if ! kubectl -n "${NS_INF}" wait \
        --for=condition=Progressing \
        --timeout="${WAIT_DEPLOY_SECONDS}s" \
        deploy/qwen-8b-multi-ring-"${side}" 2>/dev/null; then
      echo "::warning::Deployment qwen-8b-multi-ring-${side} not Progressing within ${WAIT_DEPLOY_SECONDS}s — describing for debug"
      kubectl -n "${NS_INF}" describe deploy/qwen-8b-multi-ring-"${side}" || true
      kubectl -n "${NS_INF}" describe modelservice qwen-8b-multi-ring || true
      # Don't fail — assert.sh handles the strict checks
    fi
  done
}

cmd_all() {
  cmd_reseed_mockdata
  cmd_label_nodes
  cmd_apply_template
  cmd_apply_modelservice
}

case "${1:-all}" in
  reseed-mockdata)   cmd_reseed_mockdata ;;
  label-nodes)       cmd_label_nodes ;;
  apply-template)    cmd_apply_template ;;
  apply-modelservice) cmd_apply_modelservice ;;
  all)               cmd_all ;;
  *)
    echo "usage: $0 [reseed-mockdata|label-nodes|apply-template|apply-modelservice|all]" >&2
    exit 2
    ;;
esac
