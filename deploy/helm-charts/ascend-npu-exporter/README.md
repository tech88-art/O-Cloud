# ascend-npu-exporter helm chart (P2-T-008)

Phase-2 packaging of the **community** Ascend NPU exporter (v6.0.0,
maintained by Huawei MindCluster). This chart is sufficient for the
demo's `cluster_overview` / `node_detail` / `npu_detail` Grafana
dashboards — exporter-plus features land in Phase 3+.

## Install

```bash
helm upgrade --install ascend-npu-exporter \
  ./deploy/helm-charts/ascend-npu-exporter \
  --namespace ocloud-system \
  --create-namespace
```

The chart deploys:

| Resource | Purpose |
|---|---|
| `DaemonSet` | one pod per Ascend node (selected via `huawei.com/Ascend910B=true`) |
| `Service` (ClusterIP) | exposes `/metrics` on port 8082 |
| `ServiceMonitor` | kube-prometheus-stack scrape target (disable via `serviceMonitor.enabled=false`) |
| `ClusterRole`/`Binding` + `ServiceAccount` | RBAC for Node/Pod/ConfigMap read |

## Verify

```bash
kubectl -n ocloud-system get pods -l app.kubernetes.io/name=ascend-npu-exporter
kubectl -n ocloud-system port-forward svc/ascend-npu-exporter 8082:8082
curl localhost:8082/metrics | grep npu_chip_info
```

A typical exposed series (per [Huawei docs](https://support.huaweicloud.com/eu/usermanual-cce/cce_10_0239.html)):

```
npu_chip_info_aicore_current_freq{model_name="Ascend910B",npu_id="0",node_name="..."}
npu_chip_info_hbm_used_memory{...}
npu_chip_info_health_status{...}
npu_chip_info_utilization{...}
```

## Local-dev stub

Without an Ascend host, run `deploy/dev/docker-compose.yaml`'s
`--profile ascend` stub. It serves canned metrics from
`deploy/dev/ascend-exporter-stub/metrics.txt` so Grafana panels render
non-empty during demo prep — no `helm` needed:

```bash
docker compose -f deploy/dev/docker-compose.yaml --profile ascend up -d
```

Prometheus picks up the stub via the `ascend-npu-exporter` job in
`deploy/dev/prometheus/prometheus.yml`.

## Out of scope

- Slice-level metrics (vir01/vir02/vir04) — Phase 3 once vNPU
  device-plugin output stabilises.
- NPU process-level (per-PID) metrics — needs `exporter-plus` (Phase 3+).
- Multi-arch image variants — community image is amd64-only, matching
  ADR-0001 §7 platform decision.

## References

- [ADR-0001 — Phase 4 device-plugin primary path](../../../docs/adr/0001-npu-device-plugin-primary-path.md)
- [Phase 2 plan §P2-T-008](../../../docs/phase2-plan.md)
- [ascend-device-plugin research](../../../docs/research/ascend-device-plugin.md)
