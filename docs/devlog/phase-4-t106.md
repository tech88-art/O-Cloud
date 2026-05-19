# P4-T-106 · backend /metrics CI smoke + Observability docs

- **Commit**: 6b02801
- **Date**: 2026-05-19
- **Duration**: plan 0.5d · actual ~45min

## Intent

Wire a kind-smoke assertion script (`tests/e2e/kind/backend_metrics_test.sh`) that exercises GET /metrics + grep for `ocloud_backend_*` series after a warm-dispatch. Add backend README "Observability" section documenting the endpoint + 3 counter family table + Prometheus scrape config. Add `prometheus.io/scrape: "true"` annotations to the demo-backend Service so kube-prometheus-stack auto-discovery picks /metrics up.

## Path adaptations

**Plan `test/e2e/backend_metrics_test.sh` (singular) → reality `tests/e2e/kind/backend_metrics_test.sh` (plural + /kind/ subdir)**: matches T104's same adaptation.

**Plan `deploy/helm-charts/backend/values.yaml` → no backend chart exists**: plan literal said "if no backend chart exists yet, skip and note in PR description". Backend has no helm chart in this repo (only ascend-npu-exporter-plus + npu-dra-driver under deploy/helm-charts/). Resolution: annotations land on the existing `tests/e2e/kind/manifests/demo-backend.yaml` Service instead — the kind workflow's actual deployment target, reachable by Prometheus auto-discovery once kube-prometheus-stack lands.

## Debugging trail

- **Counter family registered but emits no lines without observed labelset**. First smoke attempt: `curl /metrics | grep ocloud_backend_` returned ZERO lines on a fresh `./bin/demo-backend` startup. The registered `CounterVec` doesn't emit `# HELP` / `# TYPE` / data lines until at least one labelset has been observed via `.WithLabelValues(...).Inc()`. Fix: backend_metrics_test.sh warms by curl-ing `/api/v1/clusters` 3 times first → `Registry.SourceFor("clusters")` increments `ocloud_backend_dispatch_calls_total{datasource="mock",endpoint="clusters"}` → assertion finds the series.
- **Failure-context output (P3 honesty)**. backend_metrics_test.sh on assertion failure emits `::error::` + sample body tail via `echo "${body}" | tail -30 >&2` — for GitHub Actions log triage on red builds, this is the difference between "0 chars debugging signal" and "the actual /metrics body, last 30 lines, with the failing pattern visible".

## Key decisions

- **Three-tier assertion ladder**. Test asserts in order: HTTP 200 + content-type, default Go collectors (go_memstats_), ocloud_backend_* counters. Each tier fails with a distinct ::error:: line so the failure mode is observable without reading code.
- **Annotation set on Service, NOT Deployment**. kube-prometheus-stack default config discovers via Service annotations. Setting them on Deployment is ignored by KPS auto-discovery (only by some custom Prometheus configs). Plan/best-practice align here.
- **README §Observability is co-located with /metrics design**. The new section lives between "## Layout" and "## Phase 1 scaffold (P1-T-005)" — i.e., immediately after the layout overview, where a fresh reader naturally lands. The counter family table + cache-hit-ratio PromQL formula + 5-line scrape stanza make this section self-contained.
- **architecture.md §3.5 (监控与日志 table) update, not §8**. Plan said §8; this repo's monitoring lives in §3.5. Adapted to existing structure.

## Verification

P3 三维度:
- Existence: 5 files staged (workflow + manifest + 2 docs + 1 new test script)
- Completeness: `bash -n tests/e2e/kind/backend_metrics_test.sh` clean; backend_metrics_test.sh asserts at 3 tiers (status / Go collectors / Ocloud counters)
- Correctness: live `curl /metrics | grep ocloud_backend_` after multi-endpoint warmup returned 5 dispatch series (verified at T008 + re-verified here)

## Carry-forward

- Phase 5 inference-operator's metrics endpoint will likely follow the same pattern (`/metrics` root, outside /api/v1). Backend's prom_metrics design is the template.
- kube-prometheus-stack (Phase 9 production rollout) auto-discovers the demo-backend Service via the 3 annotations added here — no extra Prometheus config needed once KPS lands.
- Phase 6+ Grafana dashboard candidate: cache hit ratio panel = `rate(ocloud_backend_cache_hits_total[5m]) / (rate(ocloud_backend_cache_hits_total[5m]) + rate(ocloud_backend_dispatch_calls_total[5m]))`. Documented in T008 carry-forward; surface in dashboard config when adding the panel.
