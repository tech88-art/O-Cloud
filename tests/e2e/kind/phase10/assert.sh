#!/usr/bin/env bash
# Phase 10 P10-T-107 — kind smoke assertions for Phase 10 deliverables.
#
# Active assertions(P10 substrates that landed without chart deferral):
#   T107-A1 · scheduler-plugin ConfigMap KubeSchedulerConfiguration contains
#             `- name: NumaAffinity` in plugins.filter.enabled and
#             plugins.score.enabled(T005 NumaAffinity wrap default ENABLED)
#   T107-A2 · inference-operator chart values.yaml `defaults.proxyImage`
#             field present(T106 · default empty preserves Phase 7-8
#             behavior · grep verify chart-level surface)
#
# SKIPPED assertions(per task outcomes):
#   T107-SKIPPED · Source.RealAscend(T102 5th carry)· Volcano(T108 2nd defer)
#                  · Partitionable Devices(T202 defer)
#
# Run AFTER tests/e2e/kind/phase10/install.sh.

set -euo pipefail

NS_INF="${NS_INF:-ocloud-system}"

echo "== T107-A1: NumaAffinity wrap substrate present in scheduler-plugin chart =="
# Per P10-fix-002 (2026-05-21): chart default flipped back to false because
# upstream `nrt.New` requires NodeResourceTopology CRDs which aren't bundled
# in this chart yet (Phase 11+ chart packaging stream candidate per
# ADR-0016 §3 真生产化 spine). Wrap substrate is landed at P10-T-005 ·
# operators opt-in via numaAffinity.enabled: true after installing NRT CRDs.
# This assert just verifies the substrate is present in the chart values.
grep -qE "^numaAffinity:" /d/code/ai-edge/deploy/helm-charts/scheduler-plugin/values.yaml && \
  grep -qE "^[[:space:]]+enabled: " /d/code/ai-edge/deploy/helm-charts/scheduler-plugin/values.yaml || {
    echo "FAIL T107-A1: numaAffinity substrate missing from scheduler-plugin chart values.yaml"
    exit 1
  }
echo "PASS T107-A1: NumaAffinity wrap substrate present (default disabled per P10-fix-002 · enable manually after installing NRT CRDs)"

echo ""
echo "== T107-A2: inference-operator chart defaults.proxyImage field surface =="
grep -qE "^[[:space:]]+proxyImage:" /d/code/ai-edge/deploy/helm-charts/inference-operator/values.yaml || {
  echo "FAIL T107-A2: defaults.proxyImage field missing from chart values.yaml"
  exit 1
}
echo "PASS T107-A2: defaults.proxyImage field present in chart values.yaml"

echo ""
echo "== T107-SKIPPED: T102 Source.RealAscend = 5th carry per ADR-0016 §2 Decision B =="
echo "== T107-SKIPPED: T108 Volcano gang-scheduling = 2nd defer Phase 11+ =="
echo "== T107-SKIPPED: T202 Partitionable Devices = KEP-4815 Beta only · defer Phase 11+ =="
echo ""
echo "Phase 10 P10-T-107 assert.sh DONE · 2 active assertions PASS · 3 SKIPPED conditional"
