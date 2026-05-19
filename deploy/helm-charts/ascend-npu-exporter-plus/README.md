# ascend-npu-exporter-plus helm chart (P3-T-103)

Self-built Prometheus exporter for Ascend 910B NPU devices. Emits NPU
device-level (`ascend_npu_*`), vNPU slice-level (`ascend_slice_*`), and
optional PID-level workload (`ascend_pid_*`) metrics.

This chart **replaces** two Phase 2 artifacts:

- `deploy/helm-charts/ascend-npu-exporter/` — community v6.0.0 chart
  (closed `known-issues #7` — helm lint now runs in CI).
- `deploy/dev/ascend-exporter-stub/` — nginx-served canned `/metrics`
  fake (closed `known-issues #9` — the dev compose service now runs
  the real exporter-plus binary in simulator mode).

## Quick start

```bash
# Build the image first — chart references `ascend-npu-exporter-plus:dev`.
make -C exporters/ascend-npu-exporter-plus docker-build

helm upgrade --install ascend-npu-exporter-plus \
  deploy/helm-charts/ascend-npu-exporter-plus \
  --namespace monitoring \
  --create-namespace
```

The chart deploys:

| Resource | Purpose |
|---|---|
| `DaemonSet` | one pod per `huawei.com/Ascend910B=true` node |
| `Service` (ClusterIP) | exposes `/metrics` on `service.port` (default 9100) |
| `ServiceMonitor` | kube-prometheus-stack auto-scrape (toggleable) |
| `helm test` hook | scrapes `/metrics`, asserts `exporter_build_info` + at least one `ascend_npu_*` series |

## Key values

| Key | Default | Purpose |
|---|---|---|
| `image.repository` | `ascend-npu-exporter-plus` | Image registry/repo |
| `image.tag` | `dev` | Image tag (CI bumps to commit SHA) |
| `service.port` | `9100` | exporter `/metrics` port; matches `--listen` and Dockerfile EXPOSE |
| `simulator.enabled` | `true` | Phase 3 default — wires `--simulator` |
| `simulator.configMap` | `""` | Name of ConfigMap holding `sim.json`; empty falls back to in-image testdata path or DCMI-stub mode |
| `simulator.mountPath` | `/etc/ascend-exporter-plus` | Where `sim.json` is mounted inside the container |
| `workloadCorrelation.enabled` | `false` | Opt-in for `ascend_pid_*` PID series (see exporters/CLAUDE.md §8) |
| `workloadCorrelation.simRoot` | `""` | Simulator cgroup fake-fs root (set when both `workloadCorrelation.enabled` and `simulator.enabled` are true) |
| `serviceMonitor.enabled` | `true` | Disable when no Prometheus Operator |
| `serviceMonitor.namespace` | `monitoring` | KPS default; override per cluster |

## Verify

```bash
# 1. The DaemonSet rolled out and at least one pod is Ready.
kubectl -n monitoring get pods -l app.kubernetes.io/name=ascend-npu-exporter-plus

# 2. Direct scrape.
kubectl -n monitoring port-forward svc/ascend-npu-exporter-plus 9100:9100
curl -sf localhost:9100/metrics | grep -E '^exporter_build_info|^ascend_npu_' | head

# 3. helm test (runs the Pod under templates/tests/).
helm test ascend-npu-exporter-plus -n monitoring --logs
```

## Phase 4+ upgrade path

When the DCMI / npu-smi backend lands (P4-T-2xx):

```yaml
simulator:
  enabled: false   # exporter switches to real /dev/davinci* + npu-smi
```

The chart's flag wiring already supports this — only the exporter-plus
binary's source implementation needs to ship. No chart shape change.

## Out of scope

- TLS for `/metrics` — Phase 9 production hardening.
- HostNetwork mode — single-port ClusterIP is sufficient for KPS scrape.
- Multi-arch images — exporter-plus is amd64-only per ADR-0001 §7.

## References

- [exporters/CLAUDE.md](../../../exporters/CLAUDE.md) — exporter-plus
  module responsibilities + metric prefixes.
- [exporters/ascend-npu-exporter-plus/README.md](../../../exporters/ascend-npu-exporter-plus/README.md)
  — binary CLI + collector internals.
- [docs/known-issues.md](../../../docs/known-issues.md) — `#7` + `#9`
  history, both closed by P3-T-103.
- [docs/phase3-plan.md](../../../docs/phase3-plan.md) — task index
  (P3-T-103 entry).
