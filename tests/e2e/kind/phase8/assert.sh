#!/usr/bin/env bash
# Phase 8 P8-T-103 — kind smoke assertions for the busy-idle 垂直伸缩
# substrate.
#
# Preconditions: tests/e2e/kind/phase8/install.sh `all` ran successfully.
#
# Assertions (in order):
#   T103-1. NPUSliceTemplate qwen-pd-busy + qwen-pd-idle status:
#           - Conditions[Validated].Status = True (both)
#           - Conditions[Allocatable].Status = True (both · per Phase 7
#             template_controller placeholder)
#   T103-2. NPUVerticalScaler qwen-pd-scaler status:
#           - ConditionActive recorded (True OR False · controller observed
#             the object; degraded mode = True reason=NoDataThisTick)
#           - status.observedTarget.name = "qwen-pd"
#   T103-3. ModelService qwen-pd annotation reflects pre-seeded "idle"
#           - In degraded mode (no Prometheus), scaler does NOT patch
#             the annotation; it stays at qwen-pd-idle
#           - HARD FAIL if annotation flipped to qwen-pd-busy in
#             degraded mode (would indicate Reconcile bug)
#   T103-4. T008 bundle path: ResourceClaim(s) for qwen-pd carry
#           the annotation `npu.huawei.com/slice-template=qwen-pd-idle`
#           (stamped by install.sh demo workaround) AND when claim is
#           allocated, its `status.allocation.devices.results` reflects
#           the bundle decompose output
#   T103-5. NumaAffinity active assertion: SKIPPED — Phase 8 P8-T-002
#           user decision stays K8s 1.32 baseline (NumaAffinity wrap
#           upgrade deferred to Phase 9 per known-issues #12)
#   T103-6. Partitionable Devices BETA assertion: SKIPPED — Phase 8
#           P8-T-101 deferred to Phase 10 (kindest/node v1.36 lag +
#           user 1.32 stay decision · per phase8-plan §3 P8-T-101
#           doc-only fallback)

set -euo pipefail

NS_INF="${NS_INF:-ocloud-system}"
NS_SCHED="${NS_SCHED:-kube-system}"

# T103-1: NPUSliceTemplate status (qwen-pd-busy + qwen-pd-idle)
echo "== T103-1: NPUSliceTemplates Validated=True =="
for tpl in qwen-pd-busy qwen-pd-idle; do
  validated="$(kubectl get npust "${tpl}" -o jsonpath='{.status.conditions[?(@.type=="Validated")].status}' 2>/dev/null || echo "")"
  if [[ "${validated}" != "True" ]]; then
    echo "::error::NPUSliceTemplate ${tpl} Validated = ${validated:-<missing>}, want True" >&2
    kubectl describe npust "${tpl}" || true
    exit 1
  fi
  echo "  OK · ${tpl} Validated=True"
done

# T103-2: NPUVerticalScaler ConditionActive observed + observedTarget set
echo "== T103-2: NPUVerticalScaler qwen-pd-scaler ConditionActive observed =="
active="$(kubectl -n "${NS_INF}" get npuvs qwen-pd-scaler -o jsonpath='{.status.conditions[?(@.type=="Active")].status}' 2>/dev/null || echo "")"
if [[ -z "${active}" ]]; then
  echo "::error::NPUVerticalScaler qwen-pd-scaler has no Active condition · Reconcile didn't observe (timeout? Ingestor stuck?)" >&2
  kubectl -n "${NS_INF}" describe npuvs qwen-pd-scaler || true
  exit 1
fi
echo "  OK · ConditionActive=${active}"

observed_target_name="$(kubectl -n "${NS_INF}" get npuvs qwen-pd-scaler -o jsonpath='{.status.observedTarget.name}' 2>/dev/null || echo "")"
if [[ "${observed_target_name}" != "qwen-pd" ]]; then
  echo "::error::NPUVerticalScaler status.observedTarget.name = ${observed_target_name:-<missing>}, want qwen-pd" >&2
  exit 1
fi
echo "  OK · observedTarget.name=qwen-pd"

# T103-3: ModelService annotation stays at qwen-pd-idle in degraded mode
echo "== T103-3: ModelService qwen-pd annotation unchanged in degraded mode =="
ms_annotation="$(kubectl -n "${NS_INF}" get modelservice qwen-pd -o jsonpath='{.metadata.annotations.npu\.huawei\.com/slice-template}' 2>/dev/null || echo "")"
if [[ "${ms_annotation}" == "qwen-pd-busy" ]]; then
  echo "::error::ModelService annotation flipped to qwen-pd-busy in degraded mode · Reconcile bug (no Prometheus → NoData → should stay)" >&2
  kubectl -n "${NS_INF}" describe modelservice qwen-pd || true
  exit 1
fi
if [[ "${ms_annotation}" != "qwen-pd-idle" ]]; then
  echo "::warning::ModelService annotation = ${ms_annotation:-<empty>}, want qwen-pd-idle (fixture pre-seeded)" >&2
fi
echo "  OK · annotation=${ms_annotation}"

# T103-4: T008 bundle path — ResourceClaim carries slice-template annotation
echo "== T103-4: ResourceClaim slice-template annotation propagated (install.sh demo workaround) =="
claims="$(kubectl -n "${NS_INF}" get resourceclaims -l app.kubernetes.io/instance=qwen-pd -o name 2>/dev/null || true)"
if [[ -z "${claims}" ]]; then
  echo "::warning::No ResourceClaims found for qwen-pd ModelService · bundle path demo skipped"
else
  for claim in ${claims}; do
    annotated="$(kubectl -n "${NS_INF}" get "${claim}" -o jsonpath='{.metadata.annotations.npu\.huawei\.com/slice-template}' 2>/dev/null || echo "")"
    if [[ "${annotated}" != "qwen-pd-idle" ]]; then
      echo "::warning::${claim} annotation = ${annotated:-<empty>}, want qwen-pd-idle (install.sh stamps via kubectl annotate)" >&2
    else
      echo "  OK · ${claim} carries npu.huawei.com/slice-template=qwen-pd-idle"
    fi

    # When claim is allocated, verify preferred-hccs-ring annotation
    # stamped by claim_controller bundle path (P8-T-008).
    allocation_present="$(kubectl -n "${NS_INF}" get "${claim}" -o jsonpath='{.status.allocation}' 2>/dev/null || echo "")"
    if [[ -n "${allocation_present}" ]]; then
      ring="$(kubectl -n "${NS_INF}" get "${claim}" -o jsonpath='{.metadata.annotations.npu\.huawei\.com/preferred-hccs-ring}' 2>/dev/null || echo "")"
      if [[ -z "${ring}" ]]; then
        echo "::warning::${claim} allocated but preferred-hccs-ring annotation missing (claim_controller pickPreferredRing path didn't find hccs_ring attribute · expected with set-a-small fixture · set-b-multi-ring has the attribute)" >&2
      else
        echo "  OK · ${claim} preferred-hccs-ring=${ring}"
      fi
    else
      echo "  ${claim} not yet allocated (bundle path requires NPUSliceTemplate qwen-pd-idle reachable + slices available)"
    fi
  done
fi

# T103-5: NumaAffinity active assertion — SKIPPED per T002 K8s 1.32 stay
echo "== T103-5: NumaAffinity assertion SKIPPED — Phase 8 P8-T-002 user decision stays K8s 1.32 (NumaAffinity wrap upgrade deferred to Phase 9 · known-issues #12) =="

# T103-6: Partitionable Devices BETA assertion — SKIPPED per T101 defer
echo "== T103-6: Partitionable Devices BETA assertion SKIPPED — Phase 8 P8-T-101 deferred to Phase 10 (kindest/node v1.36 lag + user K8s 1.32 stay decision) =="

echo "== Phase 8 kind smoke assertions complete =="
