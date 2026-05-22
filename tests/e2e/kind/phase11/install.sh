#!/usr/bin/env bash
# Phase 11 P11-T-107 — kind smoke installer for Phase 11 chart packaging
# spine + Karmada propagation 第一波 + Frontend src/ + Volcano/Partitionable
# Devices conditionals(per ADR-0017 §2 Decision A + ADR-0018 §3 Phase 11
# delivery scope + plan §4 P11-T-107 spec)。
#
# Phase 11 task chain landed substrates(at install.sh write time · T107
# extends as W2/W3 tasks land):
#   - W1: T002 ADR-0018 + T003 demo-backend chart + T004 IMS-1 chart +
#         T005 IMS-2 chart + T006 IMS-3 chart + T007 NRT CRD bundle +
#         T008 inference-operator chart ProxyImage env wire
#   - W2: T101 LAB-CONDITIONAL(default-deferred path · ADR-0011 §3 carry
#         tally 5th entry)+ T102 Karmada chart deploy + T103 Karmada
#         PropagationPolicy + T104 ClusterQuota + T105 frontend src/ +
#         T106 O2 DMS authn chart
#   - W3: T201 master-demo-multi-site.sh + T203 docs + T204 checkpoint
#
# Conditional sections per Phase 11 W2 + W3 entry outcomes:
#   - LAB-CONDITIONAL(T101): 5th attempt outcome = default-deferred · SKIPPED
#   - Volcano gang(T108): 3rd defer Phase 12+(per ADR-0017 §2 Decision A
#     不在 Phase 11 spine)· SKIPPED
#   - Partitionable Devices(T202): KEP-4815 GA + K8s 1.36+ baseline 未达
#     · SKIPPED
#   - Karmada propagation 第一波(T102/T103/T104): gated on
#     KARMADA_ENABLED=1 env flag · default off to keep CI fast
#
# Usage:
#   bash tests/e2e/kind/phase11/install.sh         # default · skip Karmada
#   KARMADA_ENABLED=1 bash .../install.sh           # enable Karmada path
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
NS_OCLOUD="${NS_OCLOUD:-ocloud-system}"
CLUSTER_NAME="${CLUSTER_NAME:-ocloud-phase11}"

echo "== Phase 11 kind smoke installer =="
echo "REPO_ROOT  = ${REPO_ROOT}"
echo "NS_OCLOUD  = ${NS_OCLOUD}"
echo "CLUSTER    = ${CLUSTER_NAME}"
echo "KARMADA    = ${KARMADA_ENABLED:-0}"
echo ""

if ! command -v kind >/dev/null 2>&1; then
  echo "SKIP install.sh: kind binary not found · syntactic verify via assert.sh only"
  echo "(install + helm install kind smoke require kind · CI 上 kind 已 prebuilt)"
  exit 0
fi

# 1. Create kind cluster
if ! kind get clusters | grep -q "^${CLUSTER_NAME}\$"; then
  kind create cluster --name="${CLUSTER_NAME}" --image=kindest/node:v1.34.3
fi

# 2. Ensure namespace
kubectl --context="kind-${CLUSTER_NAME}" create namespace "${NS_OCLOUD}" --dry-run=client -o yaml | \
  kubectl --context="kind-${CLUSTER_NAME}" apply -f -

# 3. Install P11-T-003 demo-backend chart
helm --kube-context="kind-${CLUSTER_NAME}" upgrade --install demo-backend \
  "${REPO_ROOT}/deploy/helm-charts/demo-backend" \
  --namespace="${NS_OCLOUD}" --create-namespace --wait --timeout=120s

# 4. Install P11-T-004 IMS-1 node-lifecycle-operator chart
helm --kube-context="kind-${CLUSTER_NAME}" upgrade --install node-lifecycle-operator \
  "${REPO_ROOT}/deploy/helm-charts/node-lifecycle-operator" \
  --namespace="${NS_OCLOUD}" --wait --timeout=120s

# 5. Install P11-T-005 IMS-2 software-mgmt-operator chart
helm --kube-context="kind-${CLUSTER_NAME}" upgrade --install software-mgmt-operator \
  "${REPO_ROOT}/deploy/helm-charts/software-mgmt-operator" \
  --namespace="${NS_OCLOUD}" --wait --timeout=120s

# 6. Install P11-T-006 IMS-3 bare-metal-provisioning-operator chart
helm --kube-context="kind-${CLUSTER_NAME}" upgrade --install bare-metal-provisioning-operator \
  "${REPO_ROOT}/deploy/helm-charts/bare-metal-provisioning-operator" \
  --namespace="${NS_OCLOUD}" --wait --timeout=120s

# 7. Install P11-T-007 scheduler-plugin chart with NRT CRD bundle + NumaAffinity ON
helm --kube-context="kind-${CLUSTER_NAME}" upgrade --install scheduler-plugin \
  "${REPO_ROOT}/deploy/helm-charts/scheduler-plugin" \
  --namespace="${NS_OCLOUD}" --wait --timeout=120s

# 8. Karmada control plane (conditional)
if [ "${KARMADA_ENABLED:-0}" = "1" ]; then
  echo "== KARMADA_ENABLED=1 · invoking deploy/karmada/install.sh =="
  bash "${REPO_ROOT}/deploy/karmada/install.sh"
fi

# 9. Volcano (conditional · per P11-T-108 default 3rd defer)
if [ "${VOLCANO_ENABLED:-0}" = "1" ]; then
  echo "== VOLCANO_ENABLED=1 · installing Volcano v1.10.x via independent helm =="
  helm --kube-context="kind-${CLUSTER_NAME}" repo add volcano-sh \
    https://volcano-sh.github.io/helm-charts || true
  helm --kube-context="kind-${CLUSTER_NAME}" upgrade --install volcano volcano-sh/volcano \
    --namespace volcano-system --create-namespace --wait --timeout=180s
fi

echo ""
echo "== Phase 11 kind smoke install complete =="
