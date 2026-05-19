#!/usr/bin/env bash
# Phase 5 P5-T-106 — kind smoke extension installer.
#
# Run AFTER tests/e2e/kind/install.sh `up` + `build-images` so the
# cluster is up + the Phase 3/4 components are installed. This script
# layers Phase 5 on top:
#
#   1. cert-manager (v1.16+) install via upstream manifests
#   2. wait for cert-manager Deployments Ready (≤ 90s default)
#   3. build inference-operator image + `kind load`
#   4. helm install inference-operator chart (defaults: webhook ON,
#      cert-manager wiring ON)
#   5. wait for inference-operator Deployment Available + Certificate
#      issuance complete (≤ 60s)
#
# Bails on the first failure with kubectl describe / logs dumps so the
# workflow surface a clear failure mode.

set -euo pipefail

KIND_CLUSTER="${KIND_CLUSTER:-ocloud-e2e}"
NS="${NS:-ocloud-system}"
CERT_MANAGER_NS="${CERT_MANAGER_NS:-cert-manager}"
CERT_MANAGER_VERSION="${CERT_MANAGER_VERSION:-v1.16.0}"
INF_OP_IMG="${INF_OP_IMG:-ocloud/inference-operator:e2e}"
CERT_WAIT_SECONDS="${CERT_WAIT_SECONDS:-90}"
INF_OP_WAIT_SECONDS="${INF_OP_WAIT_SECONDS:-60}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"

cmd_install_cert_manager() {
  echo "== install cert-manager ${CERT_MANAGER_VERSION} =="
  kubectl apply --validate=false -f \
    "https://github.com/cert-manager/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml"
  echo "== wait for cert-manager Pods Ready (timeout ${CERT_WAIT_SECONDS}s) =="
  if ! kubectl -n "${CERT_MANAGER_NS}" wait --for=condition=Available \
      --timeout="${CERT_WAIT_SECONDS}s" deploy --all; then
    echo "::error::cert-manager Deployments did not become Available within ${CERT_WAIT_SECONDS}s — check kind cluster size or pre-pull cert-manager images"
    kubectl -n "${CERT_MANAGER_NS}" describe deploy || true
    kubectl -n "${CERT_MANAGER_NS}" logs deploy/cert-manager --tail=100 || true
    exit 1
  fi
  echo "cert-manager Ready."
}

cmd_build_inference_operator() {
  echo "== build inference-operator image =="
  (cd "${REPO_ROOT}/operators/inference-operator" && \
    docker build -t "${INF_OP_IMG}" .)
  kind load docker-image "${INF_OP_IMG}" --name "${KIND_CLUSTER}"
}

cmd_install_inference_operator() {
  echo "== helm install inference-operator =="
  helm upgrade --install inference-operator \
    "${REPO_ROOT}/deploy/helm-charts/inference-operator" \
    --namespace "${NS}" --create-namespace \
    --set image.repository=ocloud/inference-operator \
    --set image.tag=e2e \
    --set image.pullPolicy=Never \
    --wait \
    --timeout=180s

  echo "== wait for inference-operator Deployment Available (timeout ${INF_OP_WAIT_SECONDS}s) =="
  if ! kubectl -n "${NS}" wait --for=condition=Available \
      --timeout="${INF_OP_WAIT_SECONDS}s" deploy/inference-operator; then
    echo "::error::inference-operator did not become Available within ${INF_OP_WAIT_SECONDS}s"
    kubectl -n "${NS}" describe deploy inference-operator || true
    kubectl -n "${NS}" logs deploy/inference-operator --tail=200 || true
    exit 1
  fi

  echo "== verify Certificate Ready =="
  if ! kubectl -n "${NS}" wait --for=condition=Ready \
      --timeout=60s certificate/inference-operator-webhook 2>/dev/null; then
    echo "::error::inference-operator Certificate did not become Ready"
    kubectl -n "${NS}" describe certificate || true
    kubectl -n "${NS}" describe issuer || true
    exit 1
  fi
  echo "inference-operator Ready."
}

case "${1:-}" in
  install-cert-manager)
    cmd_install_cert_manager
    ;;
  build-inference-operator)
    cmd_build_inference_operator
    ;;
  install-inference-operator)
    cmd_install_inference_operator
    ;;
  all)
    cmd_install_cert_manager
    cmd_build_inference_operator
    cmd_install_inference_operator
    ;;
  *)
    cat >&2 <<EOF
usage: $0 <subcommand>

Subcommands:
  install-cert-manager        apply upstream cert-manager + wait Available
  build-inference-operator    docker build + kind load inference-operator image
  install-inference-operator  helm install chart + wait for Deployment + Certificate
  all                         run all three in sequence
EOF
    exit 2
    ;;
esac
