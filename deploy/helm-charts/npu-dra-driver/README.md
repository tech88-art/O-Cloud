# npu-dra-driver Helm chart

> Deploy the O-Cloud Ascend NPU DRA driver (Phase 4 simulator-first) into
> a standard Kubernetes 1.30+ cluster. Real Ascend hardware integration
> lands Phase 5+ per ADR-0001 v3 §5 + ADR-0009.

## Status

**Phase 4 (P4-T-101)** — chart skeleton + ConfigMap-backed simulator
mode. The manager binary already supports `--enable-publisher` and
`--enable-claim-controller`; this chart wires both behind values toggles.

**Phase 5 (P5-T-001)** — DeviceClass registration. The chart now
renders one or more `resource.k8s.io/v1beta1.DeviceClass` objects so
ResourceClaim authors can request NPU devices by class name (per
ADR-0009 §5 step 1).

## Prerequisites

- Kubernetes **1.30+** with `resource.k8s.io/v1beta1` API available
  (DRA is GA in K8s 1.34; v1beta1 is enabled by default in 1.31+; the
  manager works against both v1beta1 and v1 — see ADR-0001 v3 §5)
- A built `npu-dra-driver` image (see `operators/npu-dra-driver/`)
- helm 3.16+

## Quick start

```bash
# Build the image locally (kind or docker desktop).
cd operators/npu-dra-driver
make docker-build IMG=npu-dra-driver:v0.1.0

# Load into kind (skip if pushing to a real registry).
kind load docker-image npu-dra-driver:v0.1.0 --name ocloud-e2e

# Install the chart.
helm install npu-dra-driver deploy/helm-charts/npu-dra-driver \
  --namespace ocloud-system --create-namespace \
  --set image.tag=v0.1.0

# Verify ResourceSlices were published.
kubectl get resourceslices
# Expected (set-a-small/npus.json: 3 nodes × 8 NPUs = 24 device entries
# across 3 slices, driver=npu.ocloud.edge.example.com).
```

## Values

| Key                                | Default                                          | Notes                                                                                                  |
| ---------------------------------- | ------------------------------------------------ | ------------------------------------------------------------------------------------------------------ |
| `image.repository`                 | `npu-dra-driver`                                 | Set to your registry path                                                                              |
| `image.tag`                        | `v0.1.0`                                         | Match the binary's PROJECT version                                                                     |
| `publisher.enabled`                | `true`                                           | Phase 4 — simulator-first ResourceSlice publisher                                                      |
| `publisher.mockDataPath`           | `/etc/npu-dra-driver/mock/npus.json`             | Matches the ConfigMap mount path                                                                       |
| `mockConfigMap.create`             | `true`                                           | Chart embeds `configs/mock-data/set-a-small/npus.json` into a ConfigMap; set to false to bring your own |
| `mockConfigMap.name`               | `""` (auto)                                       | Use when bringing your own ConfigMap                                                                   |
| `claimController.enabled`          | `true`                                           | Phase 4 skeleton — records AllocationDeferred annotations only                                         |
| `deviceClass.create`               | `true`                                           | Phase 5 — render bare `npu.ocloud.edge.example.com` DeviceClass                                        |
| `deviceClass.subClasses.whole`     | `true`                                           | Phase 5 — render `npu.ocloud.edge.example.com/whole` sub-class (FixedTemplate selector)                |
| `deviceClass.subClasses.dynamic`   | `true`                                           | Phase 5 — render `npu.ocloud.edge.example.com.dynamic` sub-class (Dynamic selector)                    |
| `leaderElect`                      | `true`                                           | Required when `replicaCount > 1`                                                                       |
| `replicaCount`                     | `1`                                              | Phase 4 default; HA arrives Phase 5+                                                                   |
| `serviceAccount.create` / `rbac.create` | `true` / `true`                              | ClusterRole scoped to resource.k8s.io + coordination + events                                          |
| `resources.requests`               | `cpu: 50m / mem: 64Mi`                           | Manager is small; Phase 4 simulator runs comfortably                                                   |

## How the simulator works

When `publisher.enabled: true`, the chart:

1. Creates a ConfigMap named `<release>-npu-dra-driver-mock` with key
   `npus.json` whose content is the embedded
   `configs/mock-data/set-a-small/npus.json`.
2. Mounts the ConfigMap at `/etc/npu-dra-driver/mock/` (read-only).
3. Passes `--mock-data-path=/etc/npu-dra-driver/mock/npus.json` to the
   manager.

The publisher reads this JSON on startup and republishes whenever the
file mtime changes (which inside a ConfigMap means whenever you
`kubectl edit configmap` and the kubelet projects the new content).

## DeviceClass registration

When `deviceClass.create: true` (default), the chart renders one
`resource.k8s.io/v1beta1.DeviceClass` per enabled sub-class:

| Class name                                  | Toggle                              | Selector                                                                                       |
| ------------------------------------------- | ----------------------------------- | ---------------------------------------------------------------------------------------------- |
| `npu.ocloud.edge.example.com`               | `deviceClass.create`                | `device.driver == "npu.ocloud.edge.example.com"` (driver-name only — matches all NPU devices) |
| `npu.ocloud.edge.example.com.whole`         | `deviceClass.subClasses.whole`      | `... && device.attributes["npu.huawei.com/slice-strategy"].string == "FixedTemplate"`         |
| `npu.ocloud.edge.example.com.dynamic`       | `deviceClass.subClasses.dynamic`    | `... && device.attributes["npu.huawei.com/slice-strategy"].string == "Dynamic"`               |

ResourceClaim authors then write either:

```yaml
spec:
  devices:
    requests:
      - name: req-0
        deviceClassName: npu.ocloud.edge.example.com         # any NPU
        # or: npu.ocloud.edge.example.com.whole              # FixedTemplate only
        # or: npu.ocloud.edge.example.com.dynamic            # Dynamic only
```

Sub-class names use the `.<suffix>` form because K8s metadata.name must
be an RFC 1123 subdomain (a `/<suffix>` shape is rejected by API server
validation even though the claim controller's `isOurClass()` matches
both forms for forward compatibility). Phase 7 Partitionable Devices
may extend with a `.partition` sub-class — set
`deviceClass.subClasses.whole=false` / `dynamic=false` to opt into a
custom set without re-rendering the bare class.

## Phase 5+ migration path

1. Disable the simulator: `--set publisher.enabled=false`.
2. Replace the publisher's Source impl with the real Ascend driver
   loader (Phase 5 task — adds `operators/npu-dra-driver/internal/publisher/source_ascend.go` etc.).
3. Enable real claim allocation in `claimController` (Phase 5 task —
   removes the AllocationDeferred annotation path and writes
   `status.devices[]` instead).

## References

- `operators/npu-dra-driver/README.md` — binary getting-started
- `docs/adr/0001-phase0-key-decisions.md` §5 v3 — dual-path roadmap
- `docs/adr/0009-npu-dra-driver.md` — slice ↔ ResourceClaim semantics (lands at P4-T-105)
- `docs/cann-driver-matrix.md` — host kernel / driver / CANN compatibility (Phase 7 entry gate)
- `docs/phase4-plan.md` §3 P4-T-101 — task acceptance

## License

Apache 2.0 — see project root `LICENSE`.
