# Karmada deployment(Phase 11 P11-T-102)

> 1 host cluster(Karmada CP)+ 2 member kind cluster minimum · 单 Docker
> daemon 模拟 multi-site · per [ADR-0018](../../docs/adr/0018-karmada-deployment-topology.md)
> §2 Decision A. Real multi-region / multi-机房 deployment 留 Phase 12+.

## Quick start

```bash
# 1. Bring up Karmada CP + 2 member kind cluster
bash deploy/karmada/install.sh

# 2. Apply PropagationPolicy templates(per ADR-0018 §2 Decision B · T103)
kubectl --kubeconfig=/tmp/karmada-apiserver.conf \
  apply -f deploy/karmada/policies/

# 3. Verify
karmadactl --kubeconfig=/tmp/karmada-apiserver.conf get clusters

# 4. Teardown(idempotent)
bash deploy/karmada/uninstall.sh
```

## Topology

```
┌────────────────────────────────────────────────────────────────┐
│  Single Docker daemon                                          │
│                                                                │
│  ┌──────────────────┐   ┌─────────────┐   ┌─────────────┐    │
│  │  host cluster    │   │  member1    │   │  member2    │    │
│  │  (Karmada CP)    │   │             │   │             │    │
│  │  karmada-system  │   │  ocloud-ns  │   │  ocloud-ns  │    │
│  └─────────┬────────┘   └──────┬──────┘   └──────┬──────┘    │
│            │                   │                 │            │
│            └─karmadactl join───┴─────────────────┘            │
└────────────────────────────────────────────────────────────────┘
```

## Files

| Path | Purpose |
|---|---|
| `install.sh` | 6-step bootstrap: kind create × 3 + helm install karmada + karmadactl join × 2 + verify |
| `uninstall.sh` | counterpart teardown(idempotent · safe re-run) |
| `values.yaml` | Karmada chart values overrides for Phase 11(single replica · in-cluster etcd · auto certs) |
| `policies/` | PropagationPolicy YAML templates(T103 落地 per ADR-0018 §2 Decision B) |

## Environment overrides

| Var | Default | Purpose |
|---|---|---|
| `HOST_CLUSTER` | `host` | kind cluster name hosting Karmada CP |
| `MEMBER_PREFIX` | `member` | kind cluster name prefix for members(`member1`, `member2`) |
| `MEMBER_COUNT` | `2` | Number of member clusters(2 minimum per ADR-0018 §2 Decision A) |
| `KIND_NODE_IMAGE` | `kindest/node:v1.34.3` | Per arch §3.3 baseline lockstep |
| `KARMADA_NS` | `karmada-system` | namespace hosting Karmada CP components |
| `KARMADA_CHART_VERSION` | `""` (latest) | Pin chart version for reproducibility |
| `KARMADA_KUBECONFIG` | `/tmp/karmada-apiserver.conf` | Where to write Karmada apiserver kubeconfig |

## Known limitations(Phase 11 · Phase 12+ candidates)

- **Single replica Karmada CP**: production HA(3+ replica + external etcd
  HA)留 Phase 12+ · per ADR-0018 §4 (a).
- **Static Secret 跨 cluster**: install.sh 不自动 sync OIDC client secret /
  cert-manager certs to member clusters · operators 手动 / 用 Vault
  (Phase 12+ per ADR-0018 §4 (c)).
- **Push mode only**: pull mode(member 跑 karmada-agent)留 Phase 12+
  当真 enterprise multi-org signal 出现 · per ADR-0018 §2 Decision C.
- **CI gating**: Phase 11 kind smoke(`tests/e2e/kind/phase11/`)default
  跳过 Karmada(KARMADA_ENABLED=1 启)· keep CI fast.

## Troubleshooting

### `karmadactl: command not found`

```bash
go install github.com/karmada-io/karmada/cmd/karmadactl@latest
```

Or use the kubectl plugin variant:

```bash
go install github.com/karmada-io/karmada/cmd/karmadactl@latest
mv ~/go/bin/karmadactl ~/go/bin/kubectl-karmada
```

### Karmada CP pods stuck in Pending

```bash
kubectl --context=kind-host -n karmada-system get pods
kubectl --context=kind-host -n karmada-system describe pod <pod>
# Common cause: kind PV provisioner not ready · wait 30-60s + retry
```

### `karmadactl join` fails

```bash
# Verify host can reach member kubeconfig
kubectl --context=kind-member1 get nodes
# Re-extract karmada kubeconfig if /tmp/karmada-apiserver.conf 缺失
kubectl --context=kind-host -n karmada-system get secret karmada-kubeconfig \
  -o jsonpath='{.data.kubeconfig}' | base64 -d > /tmp/karmada-apiserver.conf
```

## References

- [ADR-0018: Karmada deployment topology](../../docs/adr/0018-karmada-deployment-topology.md)
- [ADR-0017: Phase 11 entry decisions](../../docs/adr/0017-phase-11-entry-decisions.md) §2 Decision A 主线 2
- [`docs/phase11-plan.md`](../../docs/phase11-plan.md) §4 P11-T-102
- Karmada upstream: <https://karmada.io/docs/>
