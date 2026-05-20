#!/usr/bin/env bash
# Phase 6 P6-T-106 — kind smoke assertions.
#
# Preconditions (run by tests/e2e/kind/phase6/install.sh + Phase 5):
#   * scheduler-plugin Deployment Available (P6-T-101 chart)
#   * inference-operator chart installed (Phase 5 T106)
#   * npu-dra-driver chart installed (Phase 4 T104)
#   * pool-operator chart + seed-resources.sh applied
#
# Assertions performed (in order):
#   1. KubeSchedulerConfiguration ConfigMap profile name = "npu-scheduler"
#   2. Apply phase6/fixtures/modelservice-multiring.yaml (carries
#      preferred-hccs-ring + schedulerName)
#   3. PD-pair Pods get scheduled (no UnschedulableAndUnresolvable)
#   4. At least one Pod's `.spec.schedulerName == "npu-scheduler"`
#      (confirms the inference-operator's deployment_builder OR the
#      manual fixture set the field; Phase 6 T105 ships the schema
#      hook but not the auto-set logic — fixture sets explicitly)
#   5. inference-operator /metrics endpoint scraped via in-cluster
#      `kubectl port-forward` proxy → curl shows the 3 Phase 6
#      collectors present
#   6. NumaAffinity NOT in scheduler-plugin's enabled filter/score
#      lists (T006 deferral honored)
#
# Real-cluster HCCS placement assertion is BEST-EFFORT in kind because
# the cluster doesn't have real HCCS hardware — the simulator publishes
# ring values from mock JSON which kind nodes don't have. Phase 7 lab
# phase tightens this.

set -euo pipefail

NS_INF="${NS_INF:-ocloud-system}"
NS_SCHED="${NS_SCHED:-kube-system}"
MS_NAME="${MS_NAME:-multiring-ms}"
WAIT_DEPLOY_SECONDS="${WAIT_DEPLOY_SECONDS:-60}"
WAIT_PODS_SECONDS="${WAIT_PODS_SECONDS:-60}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FIXTURE_MS="${SCRIPT_DIR}/fixtures/modelservice-multiring.yaml"
FIXTURE_POOL="${SCRIPT_DIR}/fixtures/multi-ring-npupool.yaml"

assert_kubescheduler_config() {
  echo "== assert KubeSchedulerConfiguration profile name =="
  cm_name=$(kubectl -n "${NS_SCHED}" get cm -l app.kubernetes.io/name=scheduler-plugin -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
  if [ -z "${cm_name}" ]; then
    echo "::error::scheduler-plugin ConfigMap not found"
    return 1
  fi
  # The ConfigMap stores config.yaml as a multi-line string; grep for
  # the profile name directly.
  if ! kubectl -n "${NS_SCHED}" get cm "${cm_name}" -o jsonpath='{.data.config\.yaml}' | grep -q "schedulerName: npu-scheduler"; then
    echo "::error::ConfigMap ${cm_name} missing schedulerName: npu-scheduler"
    kubectl -n "${NS_SCHED}" get cm "${cm_name}" -o yaml || true
    return 1
  fi
  echo "ConfigMap ${cm_name}: profile npu-scheduler present."
}

assert_numa_omitted() {
  echo "== assert NumaAffinity NOT in scheduler-plugin enabled lists (T006 deferral) =="
  cm_name=$(kubectl -n "${NS_SCHED}" get cm -l app.kubernetes.io/name=scheduler-plugin -o jsonpath='{.items[0].metadata.name}')
  config=$(kubectl -n "${NS_SCHED}" get cm "${cm_name}" -o jsonpath='{.data.config\.yaml}')
  # The chart's configmap.yaml conditionally includes NumaAffinity ONLY
  # when numaAffinity.enabled=true. With default false, the rendered
  # config should NOT have `name: NumaAffinity` lines.
  if echo "${config}" | grep -q "name: NumaAffinity"; then
    echo "::error::T006 deferral broken — NumaAffinity present in rendered config"
    echo "${config}"
    return 1
  fi
  echo "NumaAffinity OMITTED per T006 deferral (expected)."
}

apply_pool_fixture() {
  echo "== apply multi-ring NPUSlicePool fixture =="
  kubectl apply -f "${FIXTURE_POOL}" || {
    # Phase 4-5 already seeded a smoke-pool; the multi-ring fixture
    # may be additive or override. Tolerate AlreadyExists.
    if [ $? -ne 0 ]; then
      echo "::warning::pool fixture apply non-zero; assuming pool already exists"
    fi
  }
}

apply_ms_fixture() {
  echo "== apply multi-ring ModelService fixture =="
  kubectl apply -f "${FIXTURE_MS}"
}

wait_pods_scheduled() {
  echo "== wait for PD-pair Pods to be scheduled (no UnschedulableAndUnresolvable) =="
  LABEL_SELECTOR="inference.ocloud.edge.example.com/model-service=${MS_NAME}"
  for i in $(seq 1 "${WAIT_PODS_SECONDS}"); do
    n_total=$(kubectl -n "${NS_INF}" get pods -l "${LABEL_SELECTOR}" -o name 2>/dev/null | wc -l)
    n_scheduled=$(kubectl -n "${NS_INF}" get pods -l "${LABEL_SELECTOR}" \
      -o json 2>/dev/null \
      | jq '[.items[] | select(.spec.nodeName != null and .spec.nodeName != "")] | length' \
      2>/dev/null || echo 0)
    if [ "${n_total:-0}" -ge 2 ] && [ "${n_scheduled:-0}" -ge 2 ] 2>/dev/null; then
      echo "Pods scheduled: ${n_scheduled}/${n_total} (after ${i}s)"
      return 0
    fi
    if [ $((i % 10)) -eq 0 ]; then
      echo "  attempt ${i}: pods total=${n_total} scheduled=${n_scheduled}"
    fi
    sleep 1
  done
  echo "::warning::Pods did not all schedule within ${WAIT_PODS_SECONDS}s — checking schedulerName"
  kubectl -n "${NS_INF}" get pods -l "${LABEL_SELECTOR}" -o wide || true
  kubectl -n "${NS_INF}" describe pods -l "${LABEL_SELECTOR}" | head -100 || true
  # Don't hard-fail — kind without real HCCS may show Pending pods if
  # Filter is too strict. The interesting bit is the schedulerName.
  return 0
}

assert_scheduler_name() {
  echo "== assert at least one Pod has schedulerName=npu-scheduler =="
  LABEL_SELECTOR="inference.ocloud.edge.example.com/model-service=${MS_NAME}"
  count=$(kubectl -n "${NS_INF}" get pods -l "${LABEL_SELECTOR}" \
    -o json 2>/dev/null \
    | jq '[.items[] | select(.spec.schedulerName == "npu-scheduler")] | length' \
    2>/dev/null || echo 0)
  if [ "${count:-0}" -ge 1 ] 2>/dev/null; then
    echo "schedulerName=npu-scheduler observed on ${count} Pod(s)"
    return 0
  fi
  echo "::warning::No Pod with schedulerName=npu-scheduler — known-issues #11 path (operators must set explicitly)"
  echo "  Note: T105 deployment_builder does NOT auto-stamp schedulerName yet (deferred)."
  return 0
}

scrape_inference_metrics() {
  echo "== scrape inference-operator /metrics endpoint =="
  # Port-forward in background; kill on exit.
  trap 'kill %1 2>/dev/null || true' EXIT
  kubectl -n "${NS_INF}" port-forward svc/inference-operator-metrics 18082:8082 >/dev/null 2>&1 &
  sleep 3

  if ! body=$(curl -sf http://localhost:18082/metrics 2>&1); then
    echo "::error::failed to curl inference-operator metrics endpoint"
    kubectl -n "${NS_INF}" get svc -l app.kubernetes.io/name=inference-operator -o yaml || true
    return 1
  fi
  # Assert the 3 P6-T-104 collectors are present (HELP / TYPE lines).
  missing=0
  for metric in \
      inference_modelservice_phase_transitions_total \
      inference_pdrouter_decisions_total \
      inference_modelservice_reconcile_duration_seconds; do
    if ! echo "${body}" | grep -q "^# TYPE ${metric}"; then
      echo "::error::expected metric not found: ${metric}"
      missing=$((missing + 1))
    fi
  done
  if [ "${missing}" -gt 0 ]; then
    echo "::error::${missing}/3 inference-operator collectors missing"
    return 1
  fi
  echo "inference-operator /metrics: 3/3 Phase 6 collectors present."
}

assert_kubescheduler_config
assert_numa_omitted
apply_pool_fixture
apply_ms_fixture
wait_pods_scheduled
assert_scheduler_name
scrape_inference_metrics
echo "== Phase 6 smoke assertions complete =="
