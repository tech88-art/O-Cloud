# Karmada PropagationPolicy templates(Phase 11 P11-T-103)

> 4 YAML 模板分发 ocloud CRD + 关联 K8s 资源到 member cluster · per
> [ADR-0018](../../../docs/adr/0018-karmada-deployment-topology.md) §2
> Decision B(per-CRD PropagationPolicy granularity 选项 a)+
> Decision D(lifted-informer cross-cluster aggregation).

## Templates

| File | Kind | Resources propagated | Scheduling |
|---|---|---|---|
| `propagation-modelservice.yaml` | PropagationPolicy(ns-scope) | ModelService + linked Deployment/Service | Divided weighted 1:1 |
| `propagation-npuslicepool.yaml` | PropagationPolicy(ns-scope) | NPUSlicePool | Duplicated(each member 自己 NPU 资源) |
| `propagation-quota.yaml` | PropagationPolicy(ns-scope) | Quota(namespace-scope) | Duplicated(per-member enforce) |
| `cluster-propagation-clusterquota.yaml` | ClusterPropagationPolicy | ClusterQuota(cluster-scope) | Duplicated(consistent across members) |

## Apply

```bash
kubectl --kubeconfig=/tmp/karmada-apiserver.conf apply -f deploy/karmada/policies/
```

## Verify

```bash
# All policies registered
kubectl --kubeconfig=/tmp/karmada-apiserver.conf get propagationpolicies -n ocloud-system
kubectl --kubeconfig=/tmp/karmada-apiserver.conf get clusterpropagationpolicies

# After creating a ModelService in ocloud-system:
kubectl --kubeconfig=/tmp/karmada-apiserver.conf apply -f <ms.yaml>

# Verify it appears in member1 / member2:
kubectl --context=kind-member1 -n ocloud-system get modelservices
kubectl --context=kind-member2 -n ocloud-system get modelservices
```

## Cross-cluster informer aggregation(per ADR-0018 §2 Decision D)

O2 DMS Adapter binary(deployed on host cluster in `ocloud-system` ns)
+ Quota controller(colocated with inference-operator binary) consume
the **karmada-aggregated-apiserver** path to observe member-cluster
resource state. No per-cluster client construction needed — Karmada
fans out informer watches automatically.

```go
// Example: O2 DMS Adapter aggregating ModelService cross-cluster.
// Uses karmada-aggregated-apiserver kubeconfig instead of any single
// member cluster kubeconfig.
cfg, _ := clientcmd.BuildConfigFromFlags("", "/tmp/karmada-apiserver.conf")
cli, _ := client.New(cfg, client.Options{Scheme: scheme})
var msList inferencev1alpha1.ModelServiceList
cli.List(ctx, &msList) // returns aggregated list across member1 + member2
```

## Phase 12+ refinement candidates(per ADR-0018 §4)

- **per-resource override**: ClusterPropagationPolicy + priority + tag
  selector(if per-tenant placement signal materialises · per §4 (b))
- **Vault Secret propagation**: 跨 cluster Secret 动态 inject via Vault
  Agent · 替代 install.sh static copy(per §4 (c))
- **Push/pull mode mix**: pull mode for truly autonomous member clusters
  (per §2 Decision C trailing comment)

## References

- [ADR-0018 §2 Decision B + Decision D](../../../docs/adr/0018-karmada-deployment-topology.md)
- [ADR-0013 §6 forward note](../../../docs/adr/0013-o2-dms-adapter.md): O2 DMS adapter cross-cluster
- [ADR-0014 §7 forward note](../../../docs/adr/0014-multi-tenant-quota.md): Quota cross-cluster aggregation
- Karmada PropagationPolicy spec: <https://karmada.io/docs/reference/glossary/#propagation-policy>
