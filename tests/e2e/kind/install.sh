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
# P4-T-104: npu-dra-driver image — built + loaded at build-images time,
# helm-installed during `up` so kubectl get resourceslices proves the
# simulator publisher (P4-T-005) reconciliation fired end-to-end against
# a live kube-apiserver.
NPU_DRA_IMG="${NPU_DRA_IMG:-ocloud/npu-dra-driver:e2e}"

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

  echo "== build npu-dra-driver image (P4-T-104) =="
  (cd "${REPO_ROOT}/operators/npu-dra-driver" && make docker-build IMG="${NPU_DRA_IMG}")
  kind load docker-image "${NPU_DRA_IMG}" --name "${KIND_CLUSTER}"
}

cmd_up() {
  echo "== kind create cluster (idempotent) =="
  if ! kind get clusters | grep -q "^${KIND_CLUSTER}\$"; then
    kind create cluster --config "${SCRIPT_DIR}/kind-config.yaml"
  fi

  echo "== verify DRA v1beta1 API is being served by kube-apiserver =="
  # P5-T-115 (2026-05-20): T114's `grep -q deviceclasses` was too
  # lenient — the API server might serve v1alpha2 or v1alpha3 (which
  # also list `deviceclasses` in `kubectl api-resources`) without
  # serving v1beta1, and helm install of the chart's v1beta1
  # DeviceClass would still fail.
  #
  # The reliable check is to hit the version-specific discovery
  # endpoint directly via `kubectl get --raw /apis/resource.k8s.io/v1beta1`.
  # 200 OK means kube-apiserver serves that version.
  if ! kubectl get --raw "/apis/resource.k8s.io/v1beta1" >/dev/null 2>&1; then
    echo "::error::resource.k8s.io/v1beta1 API NOT served by kube-apiserver"
    echo "::group::available api-resources in resource.k8s.io group"
    kubectl api-resources --api-group=resource.k8s.io || true
    echo "::endgroup::"
    echo "::group::discovery for resource.k8s.io"
    kubectl get --raw "/apis/resource.k8s.io" 2>&1 || true
    echo "::endgroup::"
    echo "::group::kube-apiserver pod args"
    kubectl -n kube-system describe pod -l component=kube-apiserver | grep -E '(feature-gates|runtime-config|--enable-)' || true
    echo "::endgroup::"
    exit 1
  fi
  echo "DRA v1beta1 API confirmed; resources:"
  kubectl api-resources --api-group=resource.k8s.io || true

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

  echo "== pre-pull cert-manager images into kind =="
  # P5-T-111 (2026-05-19): the in-cluster image pull from quay.io for
  # cert-manager v1.16.0 routinely hits the helm `--wait --timeout 5m`
  # deadline on GitHub Actions runners (run 26136209836 step #8 log:
  # `Error: context deadline exceeded` 5m 38s into install.sh). Pre-
  # pulling on the host + `kind load docker-image` avoids the quay.io
  # round trip from the kind cluster and reliably brings cert-manager
  # Ready inside the helm `--wait` window.
  CERT_MANAGER_VERSION="v1.16.0"
  for img in controller webhook cainjector acmesolver startupapicheck; do
    docker pull "quay.io/jetstack/cert-manager-${img}:${CERT_MANAGER_VERSION}" || true
    kind load docker-image "quay.io/jetstack/cert-manager-${img}:${CERT_MANAGER_VERSION}" --name "${KIND_CLUSTER}" || true
  done

  echo "== install cert-manager =="
  helm repo add jetstack https://charts.jetstack.io --force-update >/dev/null
  if ! helm upgrade --install cert-manager jetstack/cert-manager \
      --namespace cert-manager --create-namespace \
      --version "${CERT_MANAGER_VERSION}" \
      --set crds.enabled=true \
      --set image.pullPolicy=IfNotPresent \
      --set webhook.image.pullPolicy=IfNotPresent \
      --set cainjector.image.pullPolicy=IfNotPresent \
      --set startupapicheck.image.pullPolicy=IfNotPresent \
      --wait --timeout 10m; then
    echo "::error::cert-manager helm install failed; dumping cluster state"
    echo "::group::cert-manager namespace state"
    kubectl -n cert-manager get pods -o wide || true
    kubectl -n cert-manager describe pods || true
    kubectl -n cert-manager get events --sort-by=.lastTimestamp || true
    kubectl -n cert-manager logs --tail=200 -l app.kubernetes.io/instance=cert-manager --prefix=true --all-containers=true || true
    echo "::endgroup::"
    echo "::group::cluster nodes"
    kubectl get nodes -o wide || true
    kubectl describe nodes || true
    echo "::endgroup::"
    echo "::group::kube-system pods (scheduler / controller-manager / etc.)"
    # P5-T-112: when cert-manager Pods stay Pending with no Events,
    # the scheduler is the prime suspect. Surface kube-system pod
    # health + scheduler logs to confirm.
    kubectl -n kube-system get pods -o wide || true
    for p in $(kubectl -n kube-system get pods -o name 2>/dev/null | grep -E 'scheduler|controller-manager|apiserver' || true); do
      echo "--- describe ${p} ---"
      kubectl -n kube-system describe "${p}" || true
      echo "--- logs ${p} (last 100) ---"
      kubectl -n kube-system logs "${p}" --tail=100 || true
    done
    echo "::endgroup::"
    echo "::group::cluster-wide events"
    kubectl get events -A --sort-by=.lastTimestamp | tail -60 || true
    echo "::endgroup::"
    exit 1
  fi

  echo "== install pool-operator via make deploy =="
  # P5-T-113: do NOT wait for rollout here. The workflow runs
  # `install.sh up` BEFORE `install.sh build-images`, so the
  # `ocloud/pool-operator:e2e` image isn't loaded into kind yet;
  # the freshly-created Deployment's Pod will sit in
  # ImagePullBackOff. The workflow's "refresh deployments" step
  # runs `kubectl rollout restart` + `kubectl rollout status`
  # AFTER images are loaded — that's the canonical sync point.
  # Waiting here would always time out on first run.
  (cd "${REPO_ROOT}/operators/pool-operator" && make deploy IMG="${POOL_OPERATOR_IMG}")

  echo "== install ascend-npu-exporter-plus via helm =="
  # The chart wires --simulator=<mountPath>/sim.json when both
  # simulator.enabled=true and simulator.configMap is non-empty. We
  # ship the seed JSON as a ConfigMap keyed 'sim.json'.
  kubectl -n "${MON_NS}" create configmap ascend-npu-exporter-plus-sim \
    --from-file=sim.json="${REPO_ROOT}/exporters/ascend-npu-exporter-plus/testdata/simulator-set-a-small.json" \
    --dry-run=client -o yaml | kubectl apply -f -
  # P5-T-113: `--wait` removed for the same reason as pool-operator
  # above — the exporter image isn't loaded into kind yet.
  helm upgrade --install ascend-npu-exporter-plus \
    "${REPO_ROOT}/deploy/helm-charts/ascend-npu-exporter-plus/" \
    --namespace "${MON_NS}" \
    --set image.repository="${EXPORTER_IMG_REPO}" \
    --set image.tag="${EXPORTER_IMG_TAG}" \
    --set image.pullPolicy=IfNotPresent \
    --set simulator.enabled=true \
    --set simulator.configMap=ascend-npu-exporter-plus-sim \
    --set serviceMonitor.enabled=false

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
  # P5-T-113: no rollout-status wait; images not loaded yet. See
  # comment on pool-operator above.
  kubectl apply -f "${SCRIPT_DIR}/manifests/demo-backend.yaml"

  echo "== install npu-dra-driver via helm (P4-T-104) =="
  # Phase 4: simulator-first publisher reads the bundled
  # configs/mock-data/set-a-small/npus.json (embedded into the chart's
  # mock ConfigMap via .Files.Get at install time). Claim controller
  # records AllocationDeferred=Phase4Skeleton annotations on matching
  # ResourceClaims. Both behaviours land in the same Deployment behind
  # values toggles.
  # P5-T-113: `--wait` removed; image isn't loaded yet.
  helm upgrade --install npu-dra-driver \
    "${REPO_ROOT}/deploy/helm-charts/npu-dra-driver/" \
    --namespace "${NS}" \
    --set image.repository="$(echo "${NPU_DRA_IMG}" | cut -d: -f1)" \
    --set image.tag="$(echo "${NPU_DRA_IMG}" | cut -d: -f2)" \
    --set image.pullPolicy=IfNotPresent

  echo "== up complete (deployments applied; rollouts pending image load) =="
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
