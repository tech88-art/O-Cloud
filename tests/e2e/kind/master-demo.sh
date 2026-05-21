#!/usr/bin/env bash
# Phase 10 P10-T-201 — master demo script · arch §1.3 Phase 10 实质
# deliverable · multi-pool / multi-tenant / multi-modelservice flow with
# synthetic ring fixture fallback path(per ADR-0016 §2 Decision C · T102
# 5th carry · 不挂 "真硬件" 标签 · checkpoint § Phase 10 row 标 "with
# synthetic ring fallback")。
#
# Path P(primary · T102 landed real hardware): synthetic ring fixture
# **不**使用 · master-demo.sh 跑真硬件 multi-pool flow · CANN X / driver Y
# verification stamp · arch §1.3 Phase 10 row 100% deliverable。
#
# Path F(fallback · T102 deferred 5th carry · default per ADR-0011 §3 +
# ADR-0016 §2 Decision C): synthetic ring fixture(set-b-multi-ring · Phase
# 7+8+9+10 累计 cover)继续覆盖 multi-pool / multi-tenant / Quota / cache /
# O2 DMS 全 flow · checkpoint § Phase 10 row 80% landed(20% 缺真硬件
# stamp)。
#
# This script orchestrates Phase 5 → Phase 6 → Phase 7 → Phase 8 → Phase 9 →
# Phase 10 install.sh + assert.sh sequence to demonstrate end-to-end
# (synthetic ring) capability set。

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "=================================================================="
echo "Phase 10 P10-T-201 · master demo script · synthetic ring fallback path"
echo "Per ADR-0016 §2 Decision C · T102 5th carry · 不挂 '真硬件' 标签"
echo "=================================================================="

echo ""
echo "[Step 1/12] kind cluster up · K8s 1.34 baseline · DRA + scheduler plugins"
bash "${SCRIPT_DIR}/install.sh" up

echo ""
echo "[Step 2/12] Phase 5 install · npu-dra-driver + inference-operator + cert-manager"
bash "${SCRIPT_DIR}/phase5/install.sh" all

echo ""
echo "[Step 3/12] Phase 5 assert · ResourceClaim + NPUSliceAllocation lifecycle"
bash "${SCRIPT_DIR}/phase5/assert.sh"

echo ""
echo "[Step 4/12] Phase 6 install · scheduler-plugin HCCSTopology + Binpack"
bash "${SCRIPT_DIR}/phase6/install.sh" all

echo ""
echo "[Step 5/12] Phase 6 assert · HCCS ring filter + Binpack score + scheduler-plugin profile"
bash "${SCRIPT_DIR}/phase6/assert.sh"

echo ""
echo "[Step 6/12] Phase 7 install · NPUSliceTemplate + multi-template AllocateBundle"
bash "${SCRIPT_DIR}/phase7/install.sh" all

echo ""
echo "[Step 7/12] Phase 7 assert · NPUSliceTemplate reconciler + 多模板 fallback"
bash "${SCRIPT_DIR}/phase7/assert.sh"

echo ""
echo "[Step 8/12] Phase 8 install · NPUVerticalScaler busy-idle annotation patch"
bash "${SCRIPT_DIR}/phase8/install.sh" all

echo ""
echo "[Step 9/12] Phase 8 assert · scaler reconcile loop + scaleHistory ring buffer"
bash "${SCRIPT_DIR}/phase8/assert.sh"

echo ""
echo "[Step 10/12] Phase 9 install · Quota CRD + admission webhook + O2 DMS Adapter"
bash "${SCRIPT_DIR}/phase9/install.sh" all

echo ""
echo "[Step 11/12] Phase 9 assert · Quota over-cap rejection + O2 DMS NB endpoint"
bash "${SCRIPT_DIR}/phase9/assert.sh"

echo ""
echo "[Step 12/12] Phase 10 install + assert · NumaAffinity wrap + ProxyImage chart surface"
bash "${SCRIPT_DIR}/phase10/install.sh"
bash "${SCRIPT_DIR}/phase10/assert.sh"

echo ""
echo "=================================================================="
echo "Phase 10 master demo · synthetic ring fallback path COMPLETE"
echo "Coverage: Phase 5-10 substrates · multi-pool / multi-tenant / multi-"
echo "modelservice / Quota / cache substrate / O2 DMS / NumaAffinity wrap /"
echo "ProxyImage chart surface · 80% Phase 10 row deliverable(20% lab"
echo "stamp deferred to Phase 11+ per ADR-0016 §2 Decision C)"
echo "=================================================================="
