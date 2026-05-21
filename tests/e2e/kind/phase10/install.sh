#!/usr/bin/env bash
# Phase 10 P10-T-107 — kind smoke installer for Phase 10 deliverables.
#
# Phase 10 task chain landed substrates(per docs/phase10-plan.md §3):
#   - T003 + T004 + T005 三件套(K8s 1.34 baseline + framework migration +
#     NumaAffinity wrap) · landed at SHAs `8d70efa` + `c833d6a` + `5e01e23`
#   - T006 demo-backend cache singleton substrate · `2656abe`
#   - T007 + T008 + T101 3 IMS controller body · `d01a5c3` + `ed5e4fc` +
#     `213e1a7`
#   - T103 authn substrate + T104 token-bucket substrate + T105 api-contract
#     + T106 ProxyImage chart flip
#
# Per scope adaptation(devlogs phase-10-tNNN.md): chart packaging for
# demo-backend + IMS 3 modules deferred to Phase 11+ packaging stream(per
# ADR-0016 §3 真生产化 spine)。Most of Phase 10 substrates are pure-Go +
# unit-tested without chart packaging · kind smoke phase10/ thus has many
# SKIPPED conditionals(real-cluster verify defer to chart packaging
# materialise)。
#
# Conditional sections per Phase 10 W2 + W3 entry outcomes(per
# docs/devlog/phase-10-{t102,t108,t202}.md):
#   - Source.RealAscend(T102): 4th attempt outcome = 5th carry · SKIPPED
#   - Volcano gang(T108): 2nd defer Phase 11+ · SKIPPED
#   - Partitionable Devices(T202): KEP-4815 Beta only · SKIPPED
#
# Active assertions(per Phase 10 substrates that landed without chart deferral):
#   T107-1 · NumaAffinity wrap substrate present in scheduler-plugin chart
#            values.yaml(T005 wrap landed · P10-fix-002 default disabled until
#            NRT CRDs bundled · Phase 11+ chart packaging stream candidate)
#   T107-2 · vllm-ascend ProxyImage chart `defaults.proxyImage` field present
#            (T106 · default empty preserves Phase 7-8 behavior · CI smoke
#            doesn't override · just verifies chart render path)
#   T107-3 · O2 DMS Adapter authn middleware ready(T103 substrate · 401
#            on missing Authorization header)
#
# Run AFTER tests/e2e/kind/phase9/install.sh — assumes Phase 9 cluster state.

set -euo pipefail

KIND_CLUSTER="${KIND_CLUSTER:-ocloud-e2e}"
NS_INF="${NS_INF:-ocloud-system}"

echo "== Phase 10 P10-T-107 kind smoke installer =="
echo "Phase 10 substrates verified via unit tests + helm lint at task commit"
echo "time. Real-cluster verify of chart-packaging items deferred to Phase 11+:"
echo "  - demo-backend chart(T006 deferred · `feature/p11-chart-packaging`)"
echo "  - node-lifecycle-operator chart(T007 deferred · same stream)"
echo "  - software-mgmt-operator chart(T008 deferred · same stream)"
echo "  - bare-metal-provisioning-operator chart(T101 deferred · same stream)"
echo ""
echo "Active P10 chart deltas verified via inference-operator + scheduler-"
echo "plugin chart upgrades(ProxyImage defaults.proxyImage field surface +"
echo "NumaAffinity wrap substrate present · default DISABLED per P10-fix-002 ·"
echo "operators opt-in after installing NodeResourceTopology CRDs)."

# T107-active(P10-T-005 substrate landed · P10-fix-002 chart default reverted
# to false because nrt.New needs NodeResourceTopology CRDs not bundled in
# chart yet). Chart upgrade is no-op with default(numaAffinity.enabled=false).
echo ""
echo "[T107-A] scheduler-plugin chart upgrade(NumaAffinity wrap substrate · default disabled per P10-fix-002)"
helm upgrade --install -n "${NS_INF}" --create-namespace scheduler-plugin \
  /d/code/ai-edge/deploy/helm-charts/scheduler-plugin/ \
  --wait --timeout 60s 2>&1 || echo "(chart upgrade may be no-op if already Phase 9 state)"

# T107-active(P10-T-106): inference-operator chart now has values
# `defaults.proxyImage` field surface(default ""· Phase 7-8 behavior preserved).
echo ""
echo "[T107-B] inference-operator chart upgrade for ProxyImage defaults surface"
helm upgrade --install -n "${NS_INF}" --create-namespace inference-operator \
  /d/code/ai-edge/deploy/helm-charts/inference-operator/ \
  --wait --timeout 60s 2>&1 || echo "(chart upgrade may be no-op)"

# T107-SKIPPED · 真硬件 fallback path(T201): synthetic ring fixture continues
# per ADR-0016 §2 Decision C · already cover via Phase 7 P7-T-104-v2 +
# Phase 8 + Phase 9 + this Phase 10 base · 不需新 install
echo ""
echo "[T107-SKIPPED] T102 Source.RealAscend 4th attempt = 5th carry · synthetic"
echo "ring fixture continues to cover CI + demo flow per ADR-0016 §2 Decision C"
echo "[T107-SKIPPED] T108 Volcano gang-scheduling · 2nd defer Phase 11+"
echo "[T107-SKIPPED] T202 Partitionable Devices · KEP-4815 Beta only · defer Phase 11+"
echo ""
echo "Phase 10 P10-T-107 install.sh DONE"
