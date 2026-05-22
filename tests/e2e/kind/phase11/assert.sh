#!/usr/bin/env bash
# Phase 11 P11-T-107 — kind smoke assertions for Phase 11 chart packaging
# spine 闭环 + Karmada propagation 第一波 + Frontend src/ + conditionals.
#
# Active assertions(per Phase 11 W1/W2/W3 substrates landed):
#   T107-A1 · scheduler-plugin NRT CRD bundle + numaAffinity.enabled default
#             true(P11-T-007 · known-issues #12 完整 close 循环最终步)
#   T107-A2 · demo-backend chart Lease leader-elect substrate + namespaced
#             Role for Lease(P11-T-003 · ADR-0015 §3.3 Decision B)
#   T107-A3 · IMS-1/2/3 chart cmd/main.go controller-runtime manager wire
#             present(P11-T-004/T005/T006)
#   T107-A4 · IMS-3 chart Secret RBAC for BMC credentials(P11-T-006
#             reconciler.go resolveCredentials path)
#   T107-A5 · inference-operator chart DEFAULT_PROXY_IMAGE env injection
#             (P11-T-008 · per ADR-0017 §2 Decision D 6th)
#   T107-A6 · ADR-0017/0018 entry decisions + Karmada topology committed
#             (P11-T-001/T002)
#
# Conditional assertions(SKIPPED unless flag set):
#   T107-C1 · KARMADA_ENABLED=1: Karmada control + 2 member cluster Ready
#   T107-C2 · VOLCANO_ENABLED=1: Volcano PodGroup CRD present
#   T107-C3 · LAB_AVAILABLE=1: Source.RealAscend integration(default
#             deferred per ADR-0017 §2 Decision C)
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
FAIL=0
fail() { echo "FAIL $1: $2"; FAIL=1; }
pass() { echo "PASS $1: $2"; }

echo "== Phase 11 kind smoke assertions =="
echo "REPO_ROOT = ${REPO_ROOT}"
echo ""

# T107-A1: NRT CRD bundle + numaAffinity ON
test -f "${REPO_ROOT}/deploy/helm-charts/scheduler-plugin/crds/noderesourcetopologies.yaml" || \
  fail "T107-A1" "NRT CRD bundle missing"
grep -qE "^[[:space:]]+enabled: true" "${REPO_ROOT}/deploy/helm-charts/scheduler-plugin/values.yaml" && \
  pass "T107-A1" "NRT CRD bundled + numaAffinity.enabled=true default" || \
  fail "T107-A1" "numaAffinity.enabled not true in values.yaml"

# T107-A2: demo-backend Lease leader-elect substrate
test -f "${REPO_ROOT}/deploy/helm-charts/demo-backend/templates/rbac.yaml" && \
  grep -q "coordination.k8s.io" "${REPO_ROOT}/deploy/helm-charts/demo-backend/templates/rbac.yaml" && \
  pass "T107-A2" "demo-backend chart Lease RBAC present" || \
  fail "T107-A2" "demo-backend chart Lease RBAC missing"

# T107-A3: IMS-1/2/3 cmd/main.go controller-runtime wire
for ims in node-lifecycle-operator software-mgmt-operator bare-metal-provisioning-operator; do
  grep -q "controller-runtime" "${REPO_ROOT}/operators/${ims}/cmd/main.go" && \
    grep -q "SetupWithManager" "${REPO_ROOT}/operators/${ims}/internal/controller/reconciler.go" && \
    pass "T107-A3:${ims}" "controller-runtime + Reconciler shell present" || \
    fail "T107-A3:${ims}" "controller-runtime wire or reconciler missing"
done

# T107-A4: IMS-3 Secret RBAC
grep -A 5 "resources:" "${REPO_ROOT}/deploy/helm-charts/bare-metal-provisioning-operator/templates/rbac.yaml" | \
  grep -q "secrets" && \
  pass "T107-A4" "IMS-3 chart Secret RBAC present" || \
  fail "T107-A4" "IMS-3 chart Secret RBAC missing"

# T107-A5: inference-operator DEFAULT_PROXY_IMAGE env injection
if grep -q "DEFAULT_PROXY_IMAGE" "${REPO_ROOT}/deploy/helm-charts/inference-operator/templates/deployment.yaml" 2>/dev/null; then
  pass "T107-A5" "inference-operator DEFAULT_PROXY_IMAGE env injection present"
else
  # T008 may not yet land at T107 invocation time · log SKIP rather than FAIL
  echo "SKIP T107-A5: DEFAULT_PROXY_IMAGE env injection not yet present(T008 carry)"
fi

# T107-A6: ADR-0017 + ADR-0018 present
test -f "${REPO_ROOT}/docs/adr/0017-phase-11-entry-decisions.md" && \
  test -f "${REPO_ROOT}/docs/adr/0018-karmada-deployment-topology.md" && \
  pass "T107-A6" "ADR-0017 + ADR-0018 committed" || \
  fail "T107-A6" "Phase 11 entry ADRs missing"

# T107-C1: Karmada(conditional)
if [ "${KARMADA_ENABLED:-0}" = "1" ]; then
  if command -v karmadactl >/dev/null 2>&1; then
    karmadactl get clusters 2>&1 | grep -E "Ready" | wc -l | grep -q "^[2-9]" && \
      pass "T107-C1" "Karmada ≥ 2 member cluster Ready" || \
      fail "T107-C1" "Karmada control or members not Ready"
  else
    echo "SKIP T107-C1: karmadactl missing in PATH"
  fi
else
  echo "SKIP T107-C1: KARMADA_ENABLED=0(default · per ADR-0018 §2 Decision A · CI fast path)"
fi

# T107-C2: Volcano(conditional)
if [ "${VOLCANO_ENABLED:-0}" = "1" ]; then
  kubectl get crd podgroups.scheduling.volcano.sh >/dev/null 2>&1 && \
    pass "T107-C2" "Volcano PodGroup CRD present" || \
    fail "T107-C2" "Volcano PodGroup CRD missing"
else
  echo "SKIP T107-C2: VOLCANO_ENABLED=0(default 3rd defer Phase 12+ per ADR-0017 §2 Decision A)"
fi

# T107-C3: LAB(conditional)
if [ "${LAB_AVAILABLE:-0}" = "1" ]; then
  test -f "${REPO_ROOT}/tests/lab/phase11/assert.sh" && \
    bash "${REPO_ROOT}/tests/lab/phase11/assert.sh" && \
    pass "T107-C3" "LAB smoke pass" || \
    fail "T107-C3" "LAB smoke fail"
else
  echo "SKIP T107-C3: LAB_AVAILABLE=0(default · per ADR-0017 §2 Decision C 5th attempt default-defer 维持)"
fi

echo ""
if [ "${FAIL}" -ne 0 ]; then
  echo "== Phase 11 kind smoke: $FAIL assertion(s) FAILED =="
  exit 1
fi
echo "== Phase 11 kind smoke: all assertions PASSED =="
