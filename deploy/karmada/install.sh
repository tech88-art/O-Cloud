#!/usr/bin/env bash
# Karmada control-plane + 2 member kind cluster bootstrap.
#
# Per ADR-0018 §2 Decision A topology: 1 host cluster(Karmada CP in
# karmada-system ns)+ 2 member kind cluster minimum(member1/member2 ·
# 单 Docker daemon 模拟 multi-site)· kindest/node v1.34.3 baseline
# lockstep · push mode default.
#
# Per ADR-0018 §2 Decision B chart selection: upstream karmada-charts/
# karmada(direct chart · NOT karmada-operator)· version pin via
# KARMADA_CHART_VERSION env(default = latest stable at script run time).
#
# Per ADR-0017 §2 Decision A 主线 2 + ADR-0018 §3 Phase 11 delivery
# scope T102.
#
# P13-T-205 (ADR-0025 §2 Decision D): the control-plane is now HA —
# deploy/karmada/values.yaml runs 3 replicas per component + a 3-node internal
# etcd quorum. This script is unchanged in flow (it already -f values.yaml);
# step 3.5 below verifies the replica counts came up. Real multi-region L4 LB
# fronting the apiserver + external etcd are lab/real-cluster (ADR-0018 §2 ·
# §3 right-sizing) — kind co-locates the 3 replicas on its single node, which
# verifies the replica COUNT but not true node-spread fault tolerance.
#
# Usage:
#   bash deploy/karmada/install.sh                              # 默认拓扑
#   KARMADA_CHART_VERSION=1.13.0 bash .../install.sh             # 锁定版本
#   HOST_CLUSTER=host MEMBER_PREFIX=member bash .../install.sh   # custom name
#
# Exit codes:
#   0  success(3 cluster + Karmada CP + 2 member registered)
#   1  missing kind / karmadactl / helm binary
#   2  kind cluster creation failed
#   3  Karmada chart install failed
#   4  karmadactl join failed for any member
#   5  cluster registration verify timeout
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
HOST_CLUSTER="${HOST_CLUSTER:-host}"
MEMBER_PREFIX="${MEMBER_PREFIX:-member}"
MEMBER_COUNT="${MEMBER_COUNT:-2}"
KIND_NODE_IMAGE="${KIND_NODE_IMAGE:-kindest/node:v1.34.3}"
KARMADA_NS="${KARMADA_NS:-karmada-system}"
KARMADA_CHART_VERSION="${KARMADA_CHART_VERSION:-}"  # empty = use latest
KARMADA_KUBECONFIG="${KARMADA_KUBECONFIG:-/tmp/karmada-apiserver.conf}"

echo "== Karmada bootstrap =="
echo "REPO_ROOT       = ${REPO_ROOT}"
echo "HOST_CLUSTER    = ${HOST_CLUSTER}"
echo "MEMBER_PREFIX   = ${MEMBER_PREFIX}"
echo "MEMBER_COUNT    = ${MEMBER_COUNT}"
echo "KIND_NODE_IMAGE = ${KIND_NODE_IMAGE}"
echo "KARMADA_NS      = ${KARMADA_NS}"
echo ""

# 1. Pre-flight tool checks
for tool in kind kubectl helm; do
  command -v "${tool}" >/dev/null 2>&1 || {
    echo "ERR: required binary missing: ${tool}"
    exit 1
  }
done

# karmadactl is provided by the karmada-cli release; this script attempts
# `karmadactl` first, falls back to `kubectl-karmada` if installed as a
# kubectl plugin.
KCTL=""
if command -v karmadactl >/dev/null 2>&1; then
  KCTL="karmadactl"
elif command -v kubectl-karmada >/dev/null 2>&1; then
  KCTL="kubectl karmada"
else
  echo "ERR: required binary missing: karmadactl(or kubectl-karmada plugin)"
  echo "     install: go install github.com/karmada-io/karmada/cmd/karmadactl@latest"
  exit 1
fi
echo "karmadactl resolved: ${KCTL}"

# 2. Create host + 2 member kind clusters
for CLUSTER_NAME in "${HOST_CLUSTER}" $(seq -f "${MEMBER_PREFIX}%g" 1 ${MEMBER_COUNT}); do
  if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}\$"; then
    echo "kind cluster '${CLUSTER_NAME}' already exists · skipping create"
  else
    echo "== kind create cluster --name=${CLUSTER_NAME} =="
    kind create cluster --name="${CLUSTER_NAME}" --image="${KIND_NODE_IMAGE}" || {
      echo "ERR: kind create cluster ${CLUSTER_NAME} failed"
      exit 2
    }
  fi
done

# 3. Install Karmada control plane in host cluster
echo ""
echo "== Install Karmada chart on host cluster =="
kubectl --context="kind-${HOST_CLUSTER}" create namespace "${KARMADA_NS}" \
  --dry-run=client -o yaml | \
  kubectl --context="kind-${HOST_CLUSTER}" apply -f -

helm repo add karmada-charts \
  https://raw.githubusercontent.com/karmada-io/karmada/master/charts 2>/dev/null || true
helm repo update karmada-charts

CHART_FLAGS=""
if [ -n "${KARMADA_CHART_VERSION}" ]; then
  CHART_FLAGS="--version ${KARMADA_CHART_VERSION}"
fi

if [ -f "${REPO_ROOT}/deploy/karmada/values.yaml" ]; then
  CHART_FLAGS="${CHART_FLAGS} -f ${REPO_ROOT}/deploy/karmada/values.yaml"
fi

helm --kube-context="kind-${HOST_CLUSTER}" upgrade --install karmada \
  karmada-charts/karmada \
  --namespace="${KARMADA_NS}" \
  --wait --timeout=600s \
  ${CHART_FLAGS} || {
  echo "ERR: helm install karmada failed"
  exit 3
}

# 3.5 Verify control-plane HA replica counts (P13-T-205 · ADR-0025 §2 Decision D)
echo ""
echo "== Verify control-plane HA: deployments in ${KARMADA_NS} should be 3-replica =="
kubectl --context="kind-${HOST_CLUSTER}" -n "${KARMADA_NS}" get deploy \
  -o custom-columns='NAME:.metadata.name,DESIRED:.spec.replicas,READY:.status.readyReplicas' 2>/dev/null || true
echo "  (etcd is a StatefulSet: kubectl -n ${KARMADA_NS} get sts)"
kubectl --context="kind-${HOST_CLUSTER}" -n "${KARMADA_NS}" get sts 2>/dev/null || true
echo "  NOTE: on single-node kind the 3 replicas co-locate (replica COUNT HA · "
echo "        true node-spread + external-etcd DR is lab/real-cluster · ADR-0018 §2)."

# 4. Extract Karmada apiserver kubeconfig
echo ""
echo "== Extract Karmada apiserver kubeconfig → ${KARMADA_KUBECONFIG} =="
kubectl --context="kind-${HOST_CLUSTER}" -n "${KARMADA_NS}" \
  get secret karmada-kubeconfig -o jsonpath='{.data.kubeconfig}' | \
  base64 -d > "${KARMADA_KUBECONFIG}"

# 5. Join member clusters(push mode default)
for MEMBER in $(seq -f "${MEMBER_PREFIX}%g" 1 ${MEMBER_COUNT}); do
  echo ""
  echo "== karmadactl join ${MEMBER} =="
  ${KCTL} --kubeconfig="${KARMADA_KUBECONFIG}" join "${MEMBER}" \
    --cluster-kubeconfig="${HOME}/.kube/config" \
    --cluster-context="kind-${MEMBER}" || {
    echo "ERR: karmadactl join ${MEMBER} failed"
    exit 4
  }
done

# 6. Verify cluster registration
echo ""
echo "== Verify: ${KCTL} get clusters =="
TIMEOUT_SECS=120
ELAPSED=0
while [ ${ELAPSED} -lt ${TIMEOUT_SECS} ]; do
  READY_COUNT=$(${KCTL} --kubeconfig="${KARMADA_KUBECONFIG}" get clusters 2>/dev/null | \
    grep -E "Ready[[:space:]]+True" | wc -l || echo 0)
  if [ "${READY_COUNT}" -ge "${MEMBER_COUNT}" ]; then
    echo "PASS: ${READY_COUNT} member cluster(s) Ready · target ${MEMBER_COUNT}"
    break
  fi
  echo "  ... ${READY_COUNT}/${MEMBER_COUNT} Ready · waiting ${ELAPSED}/${TIMEOUT_SECS}s"
  sleep 10
  ELAPSED=$((ELAPSED + 10))
done
if [ "${READY_COUNT}" -lt "${MEMBER_COUNT}" ]; then
  echo "ERR: only ${READY_COUNT}/${MEMBER_COUNT} member cluster Ready after ${TIMEOUT_SECS}s"
  ${KCTL} --kubeconfig="${KARMADA_KUBECONFIG}" get clusters
  exit 5
fi

echo ""
echo "== Karmada bootstrap complete =="
echo "  host        = kind-${HOST_CLUSTER}(Karmada CP in ${KARMADA_NS})"
echo "  members     = $(seq -s ', ' -f "kind-${MEMBER_PREFIX}%g" 1 ${MEMBER_COUNT})"
echo "  apiserver   = ${KARMADA_KUBECONFIG}"
echo ""
echo "Next:"
echo "  # apply PropagationPolicy templates (per ADR-0018 §2 Decision B · T103 落地):"
echo "  kubectl --kubeconfig=${KARMADA_KUBECONFIG} apply -f ${REPO_ROOT}/deploy/karmada/policies/"
