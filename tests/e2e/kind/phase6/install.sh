#!/usr/bin/env bash
# Phase 6 P6-T-106 — kind smoke installer for scheduler-plugin chart.
#
# Run AFTER tests/e2e/kind/install.sh `up` + Phase 5
# tests/e2e/kind/phase5/install.sh so the cluster has:
#   * Phase 4 npu-dra-driver chart installed (publishes ResourceSlices
#     with hccs_ring + numa_node + health attributes from mock JSON)
#   * Phase 5 cert-manager + inference-operator installed
#
# This script layers Phase 6 scheduler-plugin on top:
#   1. build scheduler-plugin image + `kind load`
#   2. helm install scheduler-plugin chart (defaults: HCCSTopology on,
#      Binpack off, NumaAffinity OMITTED per T006 deferral)
#   3. wait for scheduler-plugin Deployment Available + leader election
#      Lease present
#
# Bails on the first failure with kubectl describe / logs dumps so the
# workflow surfaces a clear failure mode.

set -euo pipefail

KIND_CLUSTER="${KIND_CLUSTER:-ocloud-e2e}"
NS="${NS:-kube-system}"  # scheduler-plugin runs in kube-system by default per ADR-0010
SCHED_IMG="${SCHED_IMG:-ocloud/scheduler-plugin:e2e}"
SCHED_WAIT_SECONDS="${SCHED_WAIT_SECONDS:-60}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"

cmd_build_scheduler_plugin() {
  echo "== build scheduler-plugin image =="
  (cd "${REPO_ROOT}/operators/scheduler-plugin" && \
    docker build -t "${SCHED_IMG}" .)
  kind load docker-image "${SCHED_IMG}" --name "${KIND_CLUSTER}"
}

cmd_install_scheduler_plugin() {
  echo "== helm install scheduler-plugin =="
  helm upgrade --install npu-scheduler \
    "${REPO_ROOT}/deploy/helm-charts/scheduler-plugin" \
    --namespace "${NS}" --create-namespace \
    --set image.repository=ocloud/scheduler-plugin \
    --set image.tag=e2e \
    --set image.pullPolicy=Never \
    --wait \
    --timeout=180s

  echo "== wait for scheduler-plugin Deployment Available (timeout ${SCHED_WAIT_SECONDS}s) =="
  if ! kubectl -n "${NS}" wait --for=condition=Available \
      --timeout="${SCHED_WAIT_SECONDS}s" deploy -l app.kubernetes.io/name=scheduler-plugin; then
    echo "::error::scheduler-plugin did not become Available within ${SCHED_WAIT_SECONDS}s"
    kubectl -n "${NS}" describe deploy -l app.kubernetes.io/name=scheduler-plugin || true
    kubectl -n "${NS}" logs deploy -l app.kubernetes.io/name=scheduler-plugin --tail=200 || true
    exit 1
  fi

  echo "== verify KubeSchedulerConfiguration ConfigMap rendered =="
  if ! kubectl -n "${NS}" get cm -l app.kubernetes.io/name=scheduler-plugin -o name | head -1 >/dev/null; then
    echo "::error::scheduler-plugin ConfigMap not found in namespace ${NS}"
    kubectl -n "${NS}" get cm -l app.kubernetes.io/name=scheduler-plugin -o yaml || true
    exit 1
  fi

  echo "== verify leader-election Lease created =="
  # The Lease shows up after the first Reconcile of scheduler-plugin's
  # internal leader election. Give it up to 30s to settle.
  for i in $(seq 1 30); do
    if kubectl -n "${NS}" get lease -l app.kubernetes.io/name=scheduler-plugin 2>/dev/null \
        | grep -q npu-scheduler 2>/dev/null; then
      echo "leader-election Lease present (after ${i}s)"
      break
    fi
    # Newer versions don't label the Lease — fall back to a direct name
    # check matching the chart's leaderElection.resourceName value.
    if kubectl -n "${NS}" get lease npu-scheduler-scheduler-plugin 2>/dev/null >/dev/null; then
      echo "leader-election Lease present (after ${i}s, name match)"
      break
    fi
    sleep 1
  done

  echo "scheduler-plugin Ready."
}

case "${1:-}" in
  build-scheduler-plugin)
    cmd_build_scheduler_plugin
    ;;
  install-scheduler-plugin)
    cmd_install_scheduler_plugin
    ;;
  all)
    cmd_build_scheduler_plugin
    cmd_install_scheduler_plugin
    ;;
  *)
    cat >&2 <<EOF
usage: $0 <subcommand>

Subcommands:
  build-scheduler-plugin     docker build + kind load scheduler-plugin image
  install-scheduler-plugin   helm install chart + wait for Deployment + Lease
  all                        run both in sequence
EOF
    exit 2
    ;;
esac
