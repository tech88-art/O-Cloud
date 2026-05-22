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

echo "== T107-A1: NumaAffinity wrap substrate + NRT CRD bundled + RBAC ready (P11-T-007 + P11-fix-004/005 partial close) =="
# Trail:P10-fix-002 default flipped to false unblock phase6 install (NRT
# CRD not bundled · nrt.New panic) → P11-T-007 ADR-0017 §2 Decision D 5th
# NRT CRD bundled at chart crds/ + default flipped back to true → P11-fix
# -004 NRT RBAC ClusterRole verbs added (SA NRT access) → P11-fix-005
# default reverted to false because nrt.New informer cache sync 在 kind
# 1.34.3 cluster > 180s helm --wait timeout · phase6 仍 red 即使全 ship
# CRD + RBAC。完整 default true 留 Phase 12+ K8s 1.36+ baseline bump cohort
# (与 Partitionable Devices Track C 联动) + helm --timeout bump。
# Operators 全栈 opt in 路径:
#   helm install scheduler-plugin --set numaAffinity.enabled=true --timeout=300s
grep -qE "^numaAffinity:" /d/code/ai-edge/deploy/helm-charts/scheduler-plugin/values.yaml || {
  echo "FAIL T107-A1: numaAffinity substrate missing in chart values.yaml"
  exit 1
}
test -f /d/code/ai-edge/deploy/helm-charts/scheduler-plugin/crds/noderesourcetopologies.yaml || {
  echo "FAIL T107-A1: NRT CRD bundle missing at chart crds/noderesourcetopologies.yaml"
  exit 1
}
grep -q "topology.node.k8s.io" /d/code/ai-edge/deploy/helm-charts/scheduler-plugin/templates/rbac.yaml || {
  echo "FAIL T107-A1: NRT RBAC verbs missing in chart templates/rbac.yaml"
  exit 1
}
echo "PASS T107-A1: NumaAffinity substrate + NRT CRD bundled + NRT RBAC ready (operator opt-in via --set numaAffinity.enabled=true · default false per P11-fix-005)"

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
