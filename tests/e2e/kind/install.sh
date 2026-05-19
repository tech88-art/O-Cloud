#!/usr/bin/env bash
# Phase 3 P3-T-104 — kind smoke bootstrap.
#
# Subcommands:
#   build-images   build pool-operator + demo-backend + ascend-npu-exporter-plus
#                  images on the host, then `kind load docker-image` each one
#                  into the e2e cluster (skipped if the cluster doesn't yet
#                  exist — call `up` first to create it).
#   up             kind create cluster + patch worker nodes with fake
#                  huawei.com/Ascend910 capacity + install cert-manager +
#                  pool-operator (via `make deploy IMG=...`) + exporter-plus
#                  (helm install) + demo-backend (kubectl apply). Rolls the
#                  deployments to Ready before returning.
#   port-forward   no-op stub. The kind-config exposes both NodePort 30080
#                  (demo-backend) and 30090 (exporter-plus, created here)
#                  via `extraPortMappings`, so `localhost:30080` /
#                  `localhost:30090` already point at the services after `up`.
#                  Kept as an addressable subcommand so the workflow YAML
#                  step name stays meaningful and a developer can drop
#                  in their own port-forward block if they switch the
#                  Services to ClusterIP for local iteration.
#   down           `kind delete cluster` — used by failure-cleanup and
#                  local dev iteration.
#
# Why `make deploy` for the operator
# ----------------------------------
#   operators/pool-operator/Makefile already has the canonical kustomize
#   recipe (`cd config/manager && kustomize edit set image controller=${IMG};
#   kustomize build config/default | kubectl apply -f -`). Reusing it means
#   we don't duplicate the image-substitution logic here and we stay in
#   sync if T005's admission/ kustomize bases shift.
#
# Why an extra NodePort for the exporter
# --------------------------------------
#   The ascend-npu-exporter-plus chart's Service is ClusterIP and the
#   chart doesn't expose a `service.nodePort` knob (we don't want to edit
#   the chart from this task per Allowed Paths). The smoke spec needs
#   to reach `/metrics` from the host so Playwright can prove the
#   simulator-driven metrics chain is live. We attach a second NodePort
#   Service inline here that selects the chart's DaemonSet pods by their
#   standard selectorLabels.
set -euo pipefail

KIND_CLUSTER="${KIND_CLUSTER:-ocloud-e2e}"
NS="${NS:-ocloud-system}"
OP_NS="${OP_NS:-pool-operator-system}"
MON_NS="${MON_NS:-monitoring}"

# Image tags consumed by kind-loaded local images. Keep in sync with the
# tags `make docker-build` produces for each component (or the explicit
# tag args we pass in `cmd_build_images`).
POOL_OPERATOR_IMG="${POOL_OPERATOR_IMG:-ocloud/pool-operator:e2e}"
DEMO_BACKEND_IMG="${DEMO_BACKEND_IMG:-ocloud/demo-backend:e2e}"
EXPORTER_IMG_REPO="${EXPORTER_IMG_REPO:-ocloud/ascend-npu-exporter-plus}"
EXPORTER_IMG_TAG="${EXPORTER_IMG_TAG:-e2e}"

# Resolve repo root regardless of where this script was invoked from.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

cmd_build_images() {
  echo "== build pool-operator image =="
  (cd "${REPO_ROOT}/operators/pool-operator" && make docker-build IMG="${POOL_OPERATOR_IMG}")
  kind load docker-image "${POOL_OPERATOR_IMG}" --name "${KIND_CLUSTER}"

  echo "== build demo-backend image =="
  docker build -t "${DEMO_BACKEND_IMG}" "${REPO_ROOT}/backend"
  kind load docker-image "${DEMO_BACKEND_IMG}" --name "${KIND_CLUSTER}"

  echo "== build ascend-npu-exporter-plus image =="
  (cd "${REPO_ROOT}/exporters/ascend-npu-exporter-plus" && \
    make docker-build IMG="${EXPORTER_IMG_REPO}:${EXPORTER_IMG_TAG}" VERSION=0.1.0)
  kind load docker-image "${EXPORTER_IMG_REPO}:${EXPORTER_IMG_TAG}" --name "${KIND_CLUSTER}"
}

cmd_up() {
  echo "== kind create cluster (idempotent) =="
  if ! kind get clusters | grep -q "^${KIND_CLUSTER}\$"; then
    kind create cluster --config "${SCRIPT_DIR}/kind-config.yaml"
  fi

  echo "== patch worker nodes with fake huawei.com/Ascend910=8 capacity =="
  # kind config 'labels:' covers the label half (used by exporter-plus
  # DaemonSet nodeSelector). Extended-resource capacity has to go through
  # the status subresource. We escape the '/' in huawei.com/Ascend910 as
  # ~1 per RFC 6901 because it's a JSON Pointer segment.
  for node in $(kubectl get nodes -l '!node-role.kubernetes.io/control-plane' -o jsonpath='{.items[*].metadata.name}'); do
    kubectl patch node "${node}" --subresource=status --type=json \
      -p='[{"op":"add","path":"/status/capacity/huawei.com~1Ascend910","value":"8"}]'
    kubectl patch node "${node}" --subresource=status --type=json \
      -p='[{"op":"add","path":"/status/allocatable/huawei.com~1Ascend910","value":"8"}]'
  done

  echo "== create namespaces =="
  kubectl create namespace "${NS}" --dry-run=client -o yaml | kubectl apply -f -
  kubectl create namespace "${MON_NS}" --dry-run=client -o yaml | kubectl apply -f -

  echo "== install cert-manager =="
  helm repo add jetstack https://charts.jetstack.io --force-update >/dev/null
  helm upgrade --install cert-manager jetstack/cert-manager \
    --namespace cert-manager --create-namespace \
    --version v1.16.0 \
    --set crds.enabled=true \
    --wait --timeout 5m

  echo "== install pool-operator via make deploy =="
  (cd "${REPO_ROOT}/operators/pool-operator" && make deploy IMG="${POOL_OPERATOR_IMG}")
  kubectl -n "${OP_NS}" rollout status deploy/pool-operator-controller-manager --timeout=3m

  echo "== install ascend-npu-exporter-plus via helm =="
  # The chart wires --simulator=<mountPath>/sim.json when both
  # simulator.enabled=true and simulator.configMap is non-empty. We
  # ship the seed JSON as a ConfigMap keyed 'sim.json'.
  kubectl -n "${MON_NS}" create configmap ascend-npu-exporter-plus-sim \
    --from-file=sim.json="${REPO_ROOT}/exporters/ascend-npu-exporter-plus/testdata/simulator-set-a-small.json" \
    --dry-run=client -o yaml | kubectl apply -f -
  helm upgrade --install ascend-npu-exporter-plus \
    "${REPO_ROOT}/deploy/helm-charts/ascend-npu-exporter-plus/" \
    --namespace "${MON_NS}" \
    --set image.repository="${EXPORTER_IMG_REPO}" \
    --set image.tag="${EXPORTER_IMG_TAG}" \
    --set image.pullPolicy=IfNotPresent \
    --set simulator.enabled=true \
    --set simulator.configMap=ascend-npu-exporter-plus-sim \
    --set serviceMonitor.enabled=false \
    --wait --timeout 3m
  kubectl -n "${MON_NS}" rollout status ds/ascend-npu-exporter-plus --timeout=2m

  echo "== expose exporter-plus on NodePort 30090 (smoke-only Service) =="
  cat <<'YAML' | kubectl apply -f -
apiVersion: v1
kind: Service
metadata:
  name: ascend-npu-exporter-plus-nodeport
  namespace: monitoring
  labels:
    app.kubernetes.io/managed-by: kind-smoke
spec:
  type: NodePort
  selector:
    app.kubernetes.io/name: ascend-npu-exporter-plus
    app.kubernetes.io/instance: ascend-npu-exporter-plus
  ports:
    - name: metrics
      port: 9100
      targetPort: metrics
      nodePort: 30090
      protocol: TCP
YAML

  echo "== install demo-backend =="
  kubectl apply -f "${SCRIPT_DIR}/manifests/demo-backend.yaml"
  kubectl -n "${NS}" rollout status deploy/demo-backend --timeout=2m

  echo "== up complete =="
}

cmd_port_forward() {
  # NodePort 30080 / 30090 are already mapped to host ports via the
  # kind config's `extraPortMappings`. Nothing to do unless a future
  # variant switches to ClusterIP-only Services.
  echo "== port-forward: NodePort host mapping handled by kind-config; no-op =="
}

cmd_down() {
  kind delete cluster --name "${KIND_CLUSTER}" || true
}

case "${1:-up}" in
  build-images) cmd_build_images ;;
  up)           cmd_up ;;
  port-forward) cmd_port_forward ;;
  down)         cmd_down ;;
  *) echo "usage: $0 {build-images|up|port-forward|down}"; exit 2 ;;
esac
