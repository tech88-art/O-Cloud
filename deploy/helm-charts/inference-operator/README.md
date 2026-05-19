# inference-operator Helm chart

> Deploy the O-Cloud inference-operator (Phase 5 ModelService controller
> + future PD Router webhook) into a Kubernetes 1.30+ cluster.

## Status

**Phase 5 T006** — chart skeleton + ModelService controller. The
chart wires the binary's `--enable-modelservice-controller` flag and
ClusterRole. T101 adds cert-manager dependency + Certificate + Issuer
for the PD Router webhook (ADR-0008). T102 lands the
MutatingWebhookConfiguration template.

## Prerequisites

- Kubernetes **1.30+**
- helm 3.16+
- pool-operator installed (this chart's RBAC depends on
  `ims.ocloud.edge.example.com/v1alpha1.NPUSlicePool` being a registered
  CRD)
- npu-dra-driver installed (this chart's RBAC depends on
  `npu.ocloud.edge.example.com/v1alpha1.NPUSliceAllocation` being a
  registered CRD; npu-dra-driver also publishes ResourceClaim status
  the inference-operator T007 reads)
- **cert-manager (v1.16+) pre-installed** when `certManager.enabled=true`
  (the chart's default). The chart renders a Certificate + Issuer
  pointing at cert-manager objects but does not ship cert-manager as a
  subchart. See `docs/known-issues.md` #10 for the recommended install
  order. Disable via `--set certManager.enabled=false` if you bring
  your own TLS plumbing for the PD Router webhook (T102+).

## Quick start

```bash
# Build the image locally.
cd operators/inference-operator
make docker-build IMG=inference-operator:v0.1.0

# Load into kind (skip if pushing to a real registry).
kind load docker-image inference-operator:v0.1.0 --name ocloud-e2e

# Install the chart.
helm install inference-operator deploy/helm-charts/inference-operator \
  --namespace ocloud-system --create-namespace \
  --set image.tag=v0.1.0
```

## Values

| Key                                | Default                  | Notes                                            |
| ---------------------------------- | ------------------------ | ------------------------------------------------ |
| `image.repository`                 | `inference-operator`     | Set to your registry path                        |
| `image.tag`                        | `v0.1.0`                 | Match the binary's PROJECT version               |
| `modelServiceController.enabled`   | `true`                   | T006 ModelServiceReconciler toggle               |
| `certManager.enabled`              | `true`                   | T101 render Certificate + Issuer for webhook TLS |
| `certManager.issuer.create`        | `true`                   | T101 render self-signed Issuer (set false to BYOI)|
| `certManager.issuer.kind`          | `Issuer`                 | T101 — `ClusterIssuer` for cluster-wide CAs      |
| `certManager.issuer.name`          | `""` (auto)              | T101 — explicit issuer name when BYOI            |
| `replicaCount`                     | `1`                      | Phase 5 default; HA arrives Phase 7+             |
| `leaderElect`                      | `true`                   | Required when `replicaCount > 1`                 |
| `serviceAccount.create`/`rbac.create` | `true` / `true`       | ClusterRole scoped to ModelService + dep CRDs    |
| `resources.requests`               | `cpu: 50m / mem: 64Mi`   | Manager is small                                 |

## Phase 5 roadmap (this chart)

- **T006** (this commit) — ModelService controller skeleton (resolve
  pool, phase=Provisioning)
- **T007** — controller creates Prefill+Decode Deployments +
  ResourceClaims (no chart change)
- **T008** — phase machine to Ready/Failed (no chart change)
- **T101** — `cert-manager` subchart dependency + Certificate + Issuer
- **T102** — MutatingWebhookConfiguration + webhook port wiring on
  Deployment
- **T103** — webhook handler implementation (no chart change)

## References

- `operators/inference-operator/DESIGN.md` — module detailed design
- `operators/inference-operator/README.md` — binary getting-started
- `docs/adr/0008-pd-router-webhook.md` — PD Router webhook design
- `docs/adr/0009-npu-dra-driver.md` — slice ↔ ResourceClaim semantics
- `docs/phase5-plan.md` §3 P5-T-006..008 — task acceptance

## License

Apache 2.0 — see project root `LICENSE`.
