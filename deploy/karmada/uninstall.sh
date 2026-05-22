#!/usr/bin/env bash
# Karmada control-plane + 2 member kind cluster teardown.
# Idempotent — safe to re-run if a previous attempt partially completed.
#
# Counterpart to install.sh. Per ADR-0018 §2 Decision C.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
HOST_CLUSTER="${HOST_CLUSTER:-host}"
MEMBER_PREFIX="${MEMBER_PREFIX:-member}"
MEMBER_COUNT="${MEMBER_COUNT:-2}"
KARMADA_NS="${KARMADA_NS:-karmada-system}"
KARMADA_KUBECONFIG="${KARMADA_KUBECONFIG:-/tmp/karmada-apiserver.conf}"

echo "== Karmada teardown =="
echo "HOST_CLUSTER  = ${HOST_CLUSTER}"
echo "MEMBER_PREFIX = ${MEMBER_PREFIX}"
echo "MEMBER_COUNT  = ${MEMBER_COUNT}"
echo ""

KCTL=""
if command -v karmadactl >/dev/null 2>&1; then
  KCTL="karmadactl"
elif command -v kubectl-karmada >/dev/null 2>&1; then
  KCTL="kubectl karmada"
fi

# 1. Unjoin members(soft · failures non-fatal)
if [ -n "${KCTL}" ] && [ -f "${KARMADA_KUBECONFIG}" ]; then
  for MEMBER in $(seq -f "${MEMBER_PREFIX}%g" 1 ${MEMBER_COUNT}); do
    echo "== karmadactl unjoin ${MEMBER} =="
    ${KCTL} --kubeconfig="${KARMADA_KUBECONFIG}" unjoin "${MEMBER}" \
      --cluster-kubeconfig="${HOME}/.kube/config" \
      --cluster-context="kind-${MEMBER}" || \
      echo "  (unjoin ${MEMBER} failed · continuing teardown)"
  done
fi

# 2. Uninstall Karmada chart
if command -v helm >/dev/null 2>&1 && kind get clusters 2>/dev/null | grep -q "^${HOST_CLUSTER}\$"; then
  echo ""
  echo "== helm uninstall karmada =="
  helm --kube-context="kind-${HOST_CLUSTER}" uninstall karmada \
    --namespace "${KARMADA_NS}" 2>/dev/null || \
    echo "  (helm uninstall failed · continuing teardown)"
fi

# 3. Delete kind clusters
for CLUSTER_NAME in $(seq -f "${MEMBER_PREFIX}%g" 1 ${MEMBER_COUNT}) "${HOST_CLUSTER}"; do
  if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}\$"; then
    echo ""
    echo "== kind delete cluster ${CLUSTER_NAME} =="
    kind delete cluster --name="${CLUSTER_NAME}" || \
      echo "  (kind delete cluster ${CLUSTER_NAME} failed · continuing teardown)"
  fi
done

# 4. Clean cached kubeconfig
[ -f "${KARMADA_KUBECONFIG}" ] && rm -f "${KARMADA_KUBECONFIG}"

echo ""
echo "== Karmada teardown complete =="
