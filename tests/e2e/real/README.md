# `tests/e2e/real/` — real-machine connection-stamp E2E (LAB-GATED)

> Runbook + assertion harness for the **real arm64 (Kunpeng 920) + 昇腾 910B +
> openEuler** cluster verification (P13-T-301 · build-doc §4.4 · ADR-0024 §3).
> **LAB-GATED** — needs real silicon; does NOT run in CI.

## Verification split (plan §8 · 2026-06-03 calibration)

- **Function** is verified by the **demo profile + CI fixtures** (fast · CPU-only ·
  always-on · the long-lived functional台). The whole logical stack is proven there.
- **This harness verifies the real-source CONNECTION only** — that each real
  `Source` reads the real device / telemetry / CANN stack and returns the SAME
  shape the profile-agnostic aggregator/handler/frontend already consume
  (ADR-0024 §2 Decision G decoupling seam). It does **not** re-test functional
  logic. Each "stamp" maps 1:1 to a build-doc §4.4 row.

## Prereqs

1. **arm64 K8s cluster** (Kunpeng 920 + openEuler · K8s 1.34+ for DRA
   `resource.k8s.io/v1`; 1.31–1.33 use `v1beta1`).
2. **Ascend Device Plugin + driver + CANN** installed (nodes expose
   `huawei.com/Ascend910B`).
3. arm64 images pulled — push them first with the **Release Images** workflow
   (`.github/workflows/release-images.yml` · manual dispatch or a `v*` tag →
   GHCR multi-arch amd64+arm64).
4. `kubectl` + `jq` on PATH; `KUBECONFIG` pointing at the lab cluster.

## Run

```bash
# 1. Stand up the real profile (real-Ascend source + real topology/deploy +
#    authz + secrets + Karmada HA per the Phase-13 bodies):
scripts/install.sh --profile real --all-phase-4 --with-secrets

# 2. Run the 5 connection stamps (build-doc §4.4):
bash tests/e2e/real/connection-stamps.sh

# 3. Drive the P99 SLA load test against the served PD endpoint:
TARGET_URL=http://qwen-pd.ocloud-system.svc.cluster.local:8000/v1/completions \
  go run ./tests/sla -n 500 -c 20 -slo-ms 2000
```

## The 5 connection stamps (→ build-doc §4.4)

| # | §4.4 row | What the stamp checks | Real Source body |
|---|---|---|---|
| 1 | (1) real NPU discovery | `ResourceSlices` from `npu.ocloud.edge.example.com` reflect real 910B count + carry `hccs_ring`/`numa_node` attrs | T101 real-Ascend publisher |
| 2 | (2) real slice | `NPUSliceAllocation` objects materialise on real NPU | T101 claim-controller |
| 3 | (3) real telemetry | exporter serves real `ascend_npu_*` series (not simulator) | T102 DCMI/npu-smi sources |
| 4 | (4) real inference | Qwen PD Prefill/Decode Pods Running on 910B + P99 via `tests/sla` | T105 CANN + vllm-ascend |
| 5 | (5) HCCS affinity | `NPUPool.status.hccsTopology` populated + PD co-placement | T101/T103 + scheduler-plugin |

`connection-stamps.sh` reports **PASS / FAIL / SKIP** (SKIP = prerequisite
absent, e.g. no served model yet — not a failure) and exits non-zero on any FAIL.

## Single-point fallback (P3 · P5 · plan §8 · ADR-0024 §3 · do NOT reopen a phase)

If one real-hardware point blocks under the lab's driver/CANN version (e.g. a
DCMI field missing, an npu-smi output variant the parser doesn't yet handle):
**deliver that body + its captured-fixture test, mark THAT one verification point
`lab-driver-gated`** in `docs/checkpoint-phase13.md` §残留 + this directory, and
the project still closes as "software-complete + best-effort real-machine". A
single surface point does NOT gate `phase-13-complete` and does NOT open Phase 14.

## Record results

Paste the `connection-stamps.sh` summary + the `tests/sla` P99 line into
`docs/checkpoint-phase13.md` (§真机 verify) and flip the matching build-doc §4.4
🔴 rows to 🟢 with the measured numbers.
