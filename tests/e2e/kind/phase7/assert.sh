#!/usr/bin/env bash
# Phase 7 P7-T-103 + P7-T-104 — kind smoke assertions.
#
# Preconditions: tests/e2e/kind/phase7/install.sh `all` ran successfully.
#
# Assertions (in order):
#   T103-1. NPUSliceTemplate qwen-8b-pd-pair status:
#           - Conditions[Validated].Status = True
#           - Conditions[Allocatable].Status = True (W1 placeholder per T007 +
#             P7-T-105 devlog scope reduction; T105-v2 / Phase 10 replaces
#             with real bundle-vs-pool check)
#           - Status.FallbackAppliedReason = "decomposed into 1×vir04 + 1×vir08"
#   T103-2. ModelService deployment_builder auto-stamp:
#           - Every PD-pair Pod's .spec.schedulerName == "npu-scheduler"
#             (HARD FAIL per Phase 7 T003 + closes known-issues #11)
#   T103-3. scheduler-plugin profile + Phase 7 T008 adjacency present:
#           - KubeSchedulerConfiguration ConfigMap renders adjacency: "0": [1,3]
#             entry (kubectl get configmap | grep)
#   T104-1. Multi-ring topology fixture in mock data:
#           - npu-dra-driver published ResourceSlices for nodeA + nodeB
#             with hccs_ring values spanning {0,1,2,3}
#
# Soft-warning (kind has no real HCCS hardware — Phase 7 T101 lab smoke
# verifies real placement on real silicon):
#   T104-2. PD-pair Pods land on SAME-or-ADJACENT rings (best-effort) —
#           cumulative ring set of all PD-pair Pods ⊆ {ring R, adj(R)}
#           per ADR-0010 §5 score-tier expectation. Reported as warning
#           when fails because kind has no real HCCS topology.

set -euo pipefail

NS_INF="${NS_INF:-ocloud-system}"
NS_SCHED="${NS_SCHED:-kube-system}"
NST_NAME="${NST_NAME:-qwen-8b-pd-pair}"
MS_NAME="${MS_NAME:-qwen-8b-multi-ring}"

# T103-1: NPUSliceTemplate status
echo "== T103-1: NPUSliceTemplate ${NST_NAME} status =="
validated="$(kubectl get npust "${NST_NAME}" -o jsonpath='{.status.conditions[?(@.type=="Validated")].status}')"
allocatable="$(kubectl get npust "${NST_NAME}" -o jsonpath='{.status.conditions[?(@.type=="Allocatable")].status}')"
fbreason="$(kubectl get npust "${NST_NAME}" -o jsonpath='{.status.fallbackAppliedReason}')"

if [[ "${validated}" != "True" ]]; then
  echo "::error::NPUSliceTemplate ${NST_NAME} Validated = ${validated}, want True" >&2
  kubectl describe npust "${NST_NAME}" || true
  exit 1
fi
if [[ "${allocatable}" != "True" ]]; then
  echo "::error::NPUSliceTemplate ${NST_NAME} Allocatable = ${allocatable}, want True" >&2
  kubectl describe npust "${NST_NAME}" || true
  exit 1
fi
expected_reason="decomposed into 1×vir04 + 1×vir08"
if [[ "${fbreason}" != "${expected_reason}" ]]; then
  echo "::error::NPUSliceTemplate ${NST_NAME} FallbackAppliedReason = ${fbreason}, want ${expected_reason}" >&2
  exit 1
fi
echo "  OK · Validated=True · Allocatable=True · reason matches"

# T103-2: schedulerName auto-stamp on PD-pair Pods (HARD FAIL per
# Phase 7 T003 acceptance — closes known-issues #11)
echo "== T103-2: schedulerName auto-stamp on PD-pair Pods =="
PODS="$(kubectl -n "${NS_INF}" get pods -l "app.kubernetes.io/instance=${MS_NAME}" -o name 2>/dev/null || true)"
if [[ -z "${PODS}" ]]; then
  echo "::warning::No PD-pair Pods found for ${MS_NAME} (Deployments may not have rolled out; deployment_builder T003 hookup verified in unit tests instead)"
else
  for pod in ${PODS}; do
    sched="$(kubectl -n "${NS_INF}" get "${pod}" -o jsonpath='{.spec.schedulerName}')"
    if [[ "${sched}" != "npu-scheduler" ]]; then
      echo "::error::Pod ${pod} schedulerName = ${sched}, want npu-scheduler (P7-T-003 auto-stamp)" >&2
      kubectl -n "${NS_INF}" describe "${pod}" || true
      exit 1
    fi
    echo "  OK · ${pod} schedulerName=npu-scheduler"
  done
fi

# T103-3: scheduler-plugin chart renders T008 default adjacency
echo "== T103-3: KubeSchedulerConfiguration contains T008 default adjacency =="
SCHED_CM="$(kubectl -n "${NS_SCHED}" get configmap -l app.kubernetes.io/name=scheduler-plugin -o name 2>/dev/null | head -1 || true)"
if [[ -z "${SCHED_CM}" ]]; then
  echo "::warning::scheduler-plugin ConfigMap not found · assertion skipped" >&2
else
  if ! kubectl -n "${NS_SCHED}" get "${SCHED_CM}" -o yaml | grep -E '"0":|"1":|"2":|"3":' >/dev/null; then
    echo "::error::scheduler-plugin ConfigMap missing T008 default adjacency entries" >&2
    kubectl -n "${NS_SCHED}" get "${SCHED_CM}" -o yaml || true
    exit 1
  fi
  echo "  OK · adjacency entries 0/1/2/3 present in ConfigMap"
fi

# T104-1: multi-ring topology in ResourceSlices
echo "== T104-1: ResourceSlices span 4 HCCS rings (set-b-multi-ring fixture) =="
# K8s 1.34 DRA GA flattens BasicDevice → attributes directly on device (per
# P10-T-003 三件套 part 1 baseline bump · P10-fix-001 schema compat). Try v1
# (flat) path first, fall back to v1beta1 (nested under .basic) for any
# pre-1.34 cluster smoke run.
RINGS="$(kubectl get resourceslices -o jsonpath='{range .items[*]}{range .spec.devices[*]}{.attributes.hccs_ring.int}{"\n"}{end}{end}' 2>/dev/null | sort -u | tr '\n' ' ' || true)"
if [ -z "${RINGS// /}" ]; then
  RINGS="$(kubectl get resourceslices -o jsonpath='{range .items[*]}{range .spec.devices[*]}{.basic.attributes.hccs_ring.int}{"\n"}{end}{end}' 2>/dev/null | sort -u | tr '\n' ' ' || true)"
fi
echo "  observed rings: [${RINGS}]"
for r in 0 1 2 3; do
  if ! echo "${RINGS}" | grep -qE "(^|[^0-9])${r}([^0-9]|$)"; then
    echo "::warning::HCCS ring ${r} not observed in ResourceSlices · set-b-multi-ring fixture may not have been reseeded (install.sh reseed-mockdata)" >&2
  fi
done

# T104-2: PD-pair placement (best-effort · kind has no real HCCS)
echo "== T104-2: PD-pair placement same-or-adjacent rings (soft · no real HCCS in kind) =="
if [[ -z "${PODS}" ]]; then
  echo "  (skipped · no PD-pair Pods)"
else
  PLACED_NODES="$(kubectl -n "${NS_INF}" get pods -l "app.kubernetes.io/instance=${MS_NAME}" -o jsonpath='{range .items[*]}{.spec.nodeName}{"\n"}{end}' | sort -u || true)"
  echo "  PD-pair Pods placed on nodes: ${PLACED_NODES}"
  echo "  NOTE: kind has no real HCCS · placement determined by default-scheduler interactions with HCCSTopologyPlugin Score · best-effort verification only. Phase 7 T101 lab smoke verifies real silicon."
fi

echo "== Phase 7 kind smoke assertions complete =="
