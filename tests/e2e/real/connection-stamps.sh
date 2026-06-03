#!/usr/bin/env bash
# Real-machine connection-stamp harness (P13-T-301 · build-doc §4.4 · ADR-0024 §3).
#
# LAB-GATED: runs against a REAL arm64 (Kunpeng 920) + 昇腾 910B + openEuler
# cluster after `scripts/install.sh --profile real --all-phase-4 --with-secrets`.
# It does NOT run in CI (no real silicon).
#
# Per plan §8 verification split: the demo profile + CI fixtures verify FUNCTION
# (fast · CPU-only · always-on). This harness verifies the real-source
# CONNECTION — that each real Source reads the real device/telemetry/inference
# stack and returns the SAME shape the aggregator/handler already consume. It
# does NOT re-test the whole functional stack. Each "stamp" maps 1:1 to a
# build-doc §4.4 row.
#
# Usage (in the lab):
#   bash tests/e2e/real/connection-stamps.sh
# Config via env (defaults shown):
#   NS=ocloud-system  MON_NS=monitoring  MODEL_SVC=qwen-pd
#   EXPORTER_SVC=ascend-npu-exporter-plus  BACKEND_SVC=demo-backend
#   DRA_DRIVER=npu.ocloud.edge.example.com
#
# Exit 0 only if all reachable stamps PASS; non-zero on any FAIL. Stamps whose
# prerequisite is absent are SKIPPED (reported · not failed) so a partial lab
# (e.g. no served model yet) still yields a useful report rather than a hard error.
set -uo pipefail

NS="${NS:-ocloud-system}"
MON_NS="${MON_NS:-monitoring}"
MODEL_SVC="${MODEL_SVC:-qwen-pd}"
EXPORTER_SVC="${EXPORTER_SVC:-ascend-npu-exporter-plus}"
BACKEND_SVC="${BACKEND_SVC:-demo-backend}"
DRA_DRIVER="${DRA_DRIVER:-npu.ocloud.edge.example.com}"

PASS=0
FAIL=0
SKIP=0
pass() { echo "  ✅ PASS: $*"; PASS=$((PASS + 1)); }
fail() { echo "  ❌ FAIL: $*"; FAIL=$((FAIL + 1)); }
skip() { echo "  ⏭️  SKIP: $*"; SKIP=$((SKIP + 1)); }

need() { command -v "$1" >/dev/null 2>&1; }
if ! need kubectl; then
  echo "ERROR: kubectl required (point KUBECONFIG at the lab cluster)"; exit 2
fi
if ! kubectl cluster-info >/dev/null 2>&1; then
  echo "ERROR: kubectl cannot reach a cluster — check KUBECONFIG"; exit 2
fi

echo "== Real-machine connection stamps (build-doc §4.4) =="
echo "   cluster: $(kubectl config current-context 2>/dev/null)"
echo ""

# ── Stamp 1 · §4.4(1) real NPU discovery ────────────────────────────────────
# real-Ascend publisher (T101) → ResourceSlices reflect真实 910B device count +
# carry the HCCS/NUMA attrs the backend topology overlay reads.
echo "[1/5] real NPU discovery (ResourceSlice from real-ascend publisher)"
if kubectl get resourceslices >/dev/null 2>&1; then
  cnt="$(kubectl get resourceslices -o json 2>/dev/null | jq "[.items[] | select(.spec.driver==\"${DRA_DRIVER}\")] | length" 2>/dev/null || echo 0)"
  if [ "${cnt:-0}" -gt 0 ]; then
    pass "ResourceSlices from ${DRA_DRIVER}: ${cnt} (real device count)"
    # attr presence (hccs_ring / numa_node) — the topology overlay's inputs.
    if kubectl get resourceslices -o json | grep -q "hccs_ring"; then
      pass "ResourceSlice carries npu.huawei.com/hccs_ring attr"
    else
      fail "ResourceSlice missing hccs_ring attr (topology overlay would degrade)"
    fi
  else
    skip "no ${DRA_DRIVER} ResourceSlices (driver not publishing — check --enable-publisher + real NPU)"
  fi
else
  skip "resource.k8s.io not served / no ResourceSlices (K8s <1.31 or DRA off)"
fi

# ── Stamp 2 · §4.4(2) real slice ─────────────────────────────────────────────
echo "[2/5] real slice allocation (NPUSliceAllocation on real NPU)"
if kubectl get npusliceallocations -A >/dev/null 2>&1; then
  acnt="$(kubectl get npusliceallocations -A --no-headers 2>/dev/null | wc -l | tr -d ' ')"
  if [ "${acnt:-0}" -gt 0 ]; then
    pass "NPUSliceAllocation objects present: ${acnt}"
  else
    skip "no NPUSliceAllocation yet (deploy a sliced workload to exercise)"
  fi
else
  skip "NPUSliceAllocation CRD absent (npu-dra-driver claim-controller not installed)"
fi

# ── Stamp 3 · §4.4(3) real telemetry replaces fixture ───────────────────────
# Exporter serves real DCMI/npu-smi series (NOT the simulator sine waves).
echo "[3/5] real telemetry (exporter ascend_npu_* series · simulator off)"
ep="$(kubectl -n "${MON_NS}" get svc "${EXPORTER_SVC}" -o jsonpath='{.spec.clusterIP}:{.spec.ports[0].port}' 2>/dev/null)"
if [ -n "${ep}" ]; then
  if kubectl -n "${MON_NS}" run sla-curl-$$ --rm -i --restart=Never --image=curlimages/curl:8.10.1 -- \
      -s "http://${ep}/metrics" 2>/dev/null | grep -q "ascend_npu_"; then
    pass "exporter exposes ascend_npu_* series at ${ep}"
    echo "       (manually confirm values are real, not simulator: check utilization variance over time)"
  else
    fail "exporter reachable but no ascend_npu_* series (collector source mis-wired)"
  fi
else
  skip "exporter Service ${EXPORTER_SVC} not found in ${MON_NS}"
fi

# ── Stamp 4 · §4.4(4) real inference ─────────────────────────────────────────
echo "[4/5] real inference (Qwen PD Prefill/Decode Pods Ready on real 910B)"
if kubectl -n "${NS}" get modelservice "${MODEL_SVC}" >/dev/null 2>&1; then
  ready="$(kubectl -n "${NS}" get pods -l "inference.ocloud.edge.example.com/model-service=${MODEL_SVC}" \
    -o json 2>/dev/null | jq '[.items[] | select(.status.phase=="Running")] | length' 2>/dev/null || echo 0)"
  if [ "${ready:-0}" -gt 0 ]; then
    pass "ModelService ${MODEL_SVC}: ${ready} PD Pod(s) Running on real NPU"
    echo "       → run the P99 SLA harness: TARGET_URL=<pd-endpoint> go run ./tests/sla -n 500 -c 20"
  else
    fail "ModelService ${MODEL_SVC} present but 0 PD Pods Running (check CANN/vllm-ascend image + NPU request)"
  fi
else
  skip "ModelService ${MODEL_SVC} not deployed (apply a PD ModelService sample to exercise)"
fi

# ── Stamp 5 · §4.4(5) HCCS affinity placement ────────────────────────────────
echo "[5/5] HCCS affinity placement (scheduler-plugin on real HCCS topology)"
if kubectl get npupools >/dev/null 2>&1 && \
   kubectl get npupools -o json | grep -q "hccsTopology"; then
  pass "NPUPool.status.hccsTopology populated (real HCCS ring discovered)"
  echo "       (confirm PD-pair Pods co-placed within an HCCS ring — best-effort · see scheduler-plugin logs)"
else
  skip "NPUPool.status.hccsTopology absent (pool-operator HCCS aggregation needs real npu-smi topo)"
fi

echo ""
echo "== Connection-stamp summary: PASS=${PASS} FAIL=${FAIL} SKIP=${SKIP} =="
echo "   record this output in docs/checkpoint-phase13.md §真机 verify (per plan §8)."
if [ "${FAIL}" -gt 0 ]; then
  echo "RESULT: FAIL (a wired stamp did not connect — investigate above)"
  exit 1
fi
echo "RESULT: OK (all reachable stamps connected · SKIP = prerequisite absent, not a failure)"
exit 0
