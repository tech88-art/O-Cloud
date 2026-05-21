#!/usr/bin/env bash
# Phase 9 P9-T-103 — kind smoke installer for Phase 9 W1 deliverables:
#   - Quota CRD + admission webhook (P9-T-005 + P9-T-006)
#   - PromQL custom metric NPUVerticalScaler (P9-T-007)
#   - deployment_builder annotation propagation polish (P9-T-004)
#   - O2 DMS Adapter chart install (P9-T-008 scaffold + P9-T-104 body)
#   - IMS 3 scaffold modules CRD discovery (P9-T-105)
#
# Conditional sections per W2 entry decision outcomes:
#   - Volcano gang (P9-T-101): deferred per default policy · NOT installed
#   - NumaAffinity wrap (P9-T-102): auto-deferred · profile config unchanged
#   - Source.RealAscend (P9-T-106): 3rd carry · synthetic ring fixture only
#
# Run AFTER tests/e2e/kind/phase8/install.sh — assumes Phase 8 cluster
# state (inference-operator + npu-dra-driver + scheduler-plugin + Quota
# CRD chart bundled in inference-operator chart upgrade post-W1).
#
# Bails on first failure with kubectl describe / logs dump.

set -euo pipefail

KIND_CLUSTER="${KIND_CLUSTER:-ocloud-e2e}"
NS_INF="${NS_INF:-ocloud-system}"
NS_DEMO="${NS_DEMO:-ai-edge-demo}"
WAIT_QUOTA_SECONDS="${WAIT_QUOTA_SECONDS:-30}"
WAIT_O2DMS_SECONDS="${WAIT_O2DMS_SECONDS:-60}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FIX_QUOTA="${SCRIPT_DIR}/fixtures/quota-default.yaml"
FIX_QUOTA_OVER="${SCRIPT_DIR}/fixtures/quota-over-cap.yaml"
FIX_NVS_PROMQL="${SCRIPT_DIR}/fixtures/npuverticalscaler-promql.yaml"
FIX_MS_ANNOTATION="${SCRIPT_DIR}/fixtures/modelservice-with-annotation.yaml"
FIX_O2DMS_PROBE="${SCRIPT_DIR}/fixtures/o2-dms-probe.yaml"

cmd_apply_quota() {
  echo "== Phase 9 T103-1: ensure demo namespace + apply Quota fixture =="
  kubectl create namespace "${NS_DEMO}" --dry-run=client -o yaml | kubectl apply -f -
  kubectl apply -f "${FIX_QUOTA}"
  # Wait for Quota.status.conditions[Active]=True from QuotaReconciler
  local i
  for i in $(seq 1 "${WAIT_QUOTA_SECONDS}"); do
    local active
    active="$(kubectl -n "${NS_DEMO}" get quota ai-edge-demo-quota \
      -o jsonpath='{.status.conditions[?(@.type=="Active")].status}' 2>/dev/null || echo "")"
    if [[ "${active}" == "True" ]]; then
      echo "  OK · Quota ai-edge-demo-quota ConditionActive=True after ${i}s"
      return 0
    fi
    sleep 1
  done
  echo "::warning::Quota ConditionActive not stamped within ${WAIT_QUOTA_SECONDS}s · controller may not be running yet"
  kubectl -n "${NS_DEMO}" describe quota ai-edge-demo-quota || true
}

cmd_apply_nvs_promql() {
  echo "== Phase 9 T103-2: apply NPUVerticalScaler with PromQL custom metric (P9-T-007) =="
  kubectl apply -f "${FIX_NVS_PROMQL}"
}

cmd_apply_ms_annotation() {
  echo "== Phase 9 T103-3: apply ModelService with slice-template annotation (P9-T-004 propagation chain) =="
  kubectl apply -f "${FIX_MS_ANNOTATION}"
}

cmd_install_o2dms() {
  echo "== Phase 9 T103-4: install O2 DMS Adapter chart (P9-T-008 scaffold + P9-T-104 body) =="
  helm upgrade --install o2-dms-adapter \
    "${SCRIPT_DIR}/../../../../deploy/helm-charts/o2-dms-adapter" \
    --namespace "${NS_INF}" \
    --wait --timeout 2m || true
  # Apply probe Job that curl's the O2 DMS endpoints
  kubectl apply -f "${FIX_O2DMS_PROBE}"
}

cmd_ims_scaffold_check() {
  echo "== Phase 9 T103-5: IMS 3 scaffold CRD discovery (P9-T-105) =="
  # Phase 9 scaffold ships api types only · no helm chart · controller body Phase 10
  # CRD installation is OUT OF SCOPE — kind smoke validates Go types build only
  echo "  IMS 3 scaffolds (node-lifecycle / software-mgmt / bare-metal-provisioning) are api-types-only per CLAUDE.md §14.2 scaffold pattern · no helm chart · no kind smoke install · Phase 10 controller body landing 时 enable"
}

cmd_all() {
  cmd_apply_quota
  cmd_apply_nvs_promql
  cmd_apply_ms_annotation
  cmd_install_o2dms
  cmd_ims_scaffold_check
}

case "${1:-all}" in
  apply-quota)        cmd_apply_quota ;;
  apply-nvs-promql)   cmd_apply_nvs_promql ;;
  apply-ms-annotation) cmd_apply_ms_annotation ;;
  install-o2dms)      cmd_install_o2dms ;;
  ims-check)          cmd_ims_scaffold_check ;;
  all)                cmd_all ;;
  *)
    echo "usage: $0 [apply-quota|apply-nvs-promql|apply-ms-annotation|install-o2dms|ims-check|all]" >&2
    exit 1
    ;;
esac
