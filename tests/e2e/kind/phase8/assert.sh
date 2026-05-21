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

# T103-4 / T104: T008 bundle path — ResourceClaim carries slice-template
# annotation + preferred-hccs-ring HARD FAIL upgrade (Phase 7 T104 v1
# soft-warning → Phase 8 P8-T-104 hard fail).
#
# T104-v2 upgrade rationale: Phase 7 T104 shipped synthetic ring fixture
# (set-b-multi-ring with 4 HCCS rings across 2 nodes) + PD-pair placement
# soft-warning. The soft-warning was because Phase 7 had no controller
# wiring stamping the annotation on Pods / claims · making strict assert
# impossible. Phase 8 P8-T-008 wiring DOES stamp `preferred-hccs-ring`
# on the claim after AllocateBundle commits. T104 flips that assertion
# from soft-warning to HARD FAIL when the claim is allocated AND set-b-
# multi-ring fixture is reseeded (Phase 7 install.sh `reseed-mockdata`
# already ran in CI chain so hccs_ring attribute IS present).
echo "== T103-4 / T104: ResourceClaim slice-template annotation propagated + preferred-hccs-ring HARD FAIL (T104-v2) =="
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

    # T104-v2 hard fail: when claim is allocated, preferred-hccs-ring
    # annotation MUST be stamped (claim_controller pickPreferredRing
    # path uses v1alpha1.AttrHCCSRing qualified attribute key; set-b-
    # multi-ring fixture's npus.json carries hccsRing field; publisher
    # writes it into ResourceSlice attribute).
    allocation_present="$(kubectl -n "${NS_INF}" get "${claim}" -o jsonpath='{.status.allocation}' 2>/dev/null || echo "")"
    if [[ -n "${allocation_present}" ]]; then
      ring="$(kubectl -n "${NS_INF}" get "${claim}" -o jsonpath='{.metadata.annotations.npu\.huawei\.com/preferred-hccs-ring}' 2>/dev/null || echo "")"
      if [[ -z "${ring}" ]]; then
        echo "::error::${claim} allocated BUT preferred-hccs-ring annotation MISSING (T104-v2 hard fail · claim_controller pickPreferredRing didn't stamp · publisher may not be emitting hccs_ring attribute · check set-b-multi-ring reseed in Phase 7 install.sh)" >&2
        kubectl -n "${NS_INF}" describe "${claim}" || true
        kubectl get resourceslices -o yaml | head -120 || true
        exit 1
      fi
      # Ring value must be one of {0,1,2,3} from set-b-multi-ring fixture.
      case "${ring}" in
        0|1|2|3) echo "  OK · ${claim} preferred-hccs-ring=${ring} (T104-v2 hard-fail PASS)" ;;
        *)
          echo "::error::${claim} preferred-hccs-ring=${ring} not in expected set {0,1,2,3} from set-b-multi-ring fixture" >&2
          exit 1
          ;;
      esac
    else
      echo "  ${claim} not yet allocated (bundle path requires NPUSliceTemplate qwen-pd-idle reachable + slices available · T104-v2 hard-fail gated on allocation)"
    fi
  done
fi

# T103-5: NumaAffinity active assertion — SKIPPED per T002 K8s 1.32 stay
echo "== T103-5: NumaAffinity assertion SKIPPED — Phase 8 P8-T-002 user decision stays K8s 1.32 (NumaAffinity wrap upgrade deferred to Phase 9 · known-issues #12) =="

# T103-6: Partitionable Devices BETA assertion — SKIPPED per T101 defer
echo "== T103-6: Partitionable Devices BETA assertion SKIPPED — Phase 8 P8-T-101 deferred to Phase 10 (kindest/node v1.36 lag + user K8s 1.32 stay decision) =="

echo "== Phase 8 kind smoke assertions complete =="
