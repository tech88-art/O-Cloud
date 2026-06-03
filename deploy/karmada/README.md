# Karmada deployment(Phase 11 P11-T-102 · HA Phase 13 P13-T-205)

> 1 host cluster(Karmada CP)+ 2 member kind cluster minimum · 单 Docker
> daemon 模拟 multi-site · per [ADR-0018](../../docs/adr/0018-karmada-deployment-topology.md)
> §2 Decision A.
>
> **Phase 13 (P13-T-205 · [ADR-0025](../../docs/adr/0025-production-hardening-architecture.md)
> §2 Decision D)**: the control-plane is HA — 3 replicas per component + a 3-node
> internal etcd quorum (`values.yaml`). Real multi-region / multi-机房 deployment
> with an L4 LB fronting the apiserver + external etcd DR remains lab/real-cluster
> (ADR-0018 §2 · §3 right-sizing): on single-node kind the 3 replicas co-locate
> (replica COUNT HA, not node-spread fault tolerance).

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
| `values.yaml` | Karmada chart values overrides — **P13-T-205 HA**: 3-replica control-plane + 3-node internal etcd quorum + external-etcd swap documented |
| `policies/` | PropagationPolicy YAML templates(T103 + P13-T-205 failover/propagateDeps + cross-cluster RBAC · per ADR-0018 §2 Decision B/D) |

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

## Known limitations(Phase 13 state)

- **Control-plane HA = replica COUNT on kind**: P13-T-205 runs 3 replicas per
  component + 3-node internal etcd quorum (`values.yaml`). On single-node kind
  the replicas co-locate — true node-spread fault tolerance, an L4 LB fronting
  the apiserver, and external-etcd multi-region DR are lab/real-cluster
  (ADR-0018 §2 · ADR-0025 §3 right-sizing · external-etcd swap documented in
  `values.yaml`).
- **Static Secret 跨 cluster**: now routed through External Secrets Operator +
  Vault (P13-T-203 · `deploy/secrets/`) on the real profile; install.sh still
  does not auto-sync member-cluster certs (per ADR-0018 §4 (c) · lab).
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
