# P11-T-103 · Karmada PropagationPolicy 第一波 + cross-cluster informer aggregation

- **Commit**: (this commit)
- **Date**: 2026-05-22
- **Duration**: 1-1.5d plan / ~0.4d actual

## Intent

ADR-0018 §2 Decision B PropagationPolicy templates + Decision D lifted-informer pattern 实质 land。4 个 YAML 模板 + O2 DMS Adapter cross-cluster informer helper · ADR-0017 §2 Decision D 8th 优先级 close · ADR-0013 §6 forward note + ADR-0014 §7 forward note 的 implementation contract 起步。

## Path adaptations

1. **`deploy/karmada/policies/` 目录新建**(per ADR-0018 §2 Decision B literal · 4 YAML 路径 plan 已定)。
2. **O2 DMS Adapter cross-cluster client helper 放 `internal/inventory/karmada_aggregated.go`**:不放 `internal/types/`(types 不该有 client logic)· 不放 `internal/api/`(本仓库 O2 DMS Adapter 走 chi 而非 ctrl.Reconciler · 没有 controller pkg)· `internal/inventory/` 是当前 inventory aggregation 的 owner · 加 helper 合适。
3. **Env-var driven enablement**(`O2DMS_KARMADA_AGGREGATED_ENABLED` + `O2DMS_KARMADA_KUBECONFIG`):不强制 O2 DMS Adapter 在所有部署里走 aggregated path · operators 在 Karmada install.sh 跑完后 flip env 启 · Phase 10 single-cluster posture 继续 work。

## Debugging trail

无 build/test fail。一次 go build clean · 5 unit tests pass。

## Key decisions

- **per-CRD PropagationPolicy granularity 选项 a**(per ADR-0018 §4 (b))· 简单 · 4 YAML 一组 · 维护成本低。per-resource override 留 Phase 12+ if per-tenant placement signal materialises。
- **`replicaSchedulingType: Duplicated` for NPUSlicePool + Quota + ClusterQuota** · 每 member 各自 enforce/maintain · 不分片(分片只 ModelService 的 Deployment 走 Divided weighted)
- **`replicaSchedulingType: Divided weighted 1:1` for ModelService**:Deployment replicas 在 2 member 平均分(`weight: 1` each)· 与 ADR-0008 PD Router placement 兼容(Pod 在哪 member 由 Karmada 决定 · Pod 内 prefer-hccs-ring annotation 仍由本地 scheduler-plugin 处理)
- **ClusterQuota 必走 ClusterPropagationPolicy**(cluster-scope CRD · per Karmada doc · namespace-scope PropagationPolicy 不能 propagate cluster-scope CRD)
- **Cross-cluster informer 路径 = file-based kubeconfig**(Phase 11 ship `/tmp/karmada-apiserver.conf` from install.sh)· Phase 12+ in-cluster SA + RBAC binding 留 production hardening cohort
- **`O2DMS_KARMADA_AGGREGATED_ENABLED` opt-in**:不破 Phase 10 single-cluster default · operators 显式 flip 启

## Verification

- 存在性:
  - `ls deploy/karmada/policies/` → 4 YAML + 1 README ✓
  - `ls operators/o2-dms-adapter/internal/inventory/karmada_aggregated.go karmada_aggregated_test.go` ✓
- 完整性:
  - 4 PropagationPolicy/ClusterPropagationPolicy YAML 各 含 apiVersion: policy.karmada.io/v1alpha1 + resourceSelectors + placement.clusterAffinity.clusterNames
  - `go build ./...` exit 0
  - `go test -vet=off -run TestNewAggregatedRestConfig -run TestFromEnv ./internal/inventory/` → ok(5 unit tests)
- 正确性:
  - propagation-modelservice.yaml 含 ModelService + Deployment + Service(linked resources · labelSelector match `inference.ocloud.edge.example.com/managed-by`)
  - cluster-propagation-clusterquota.yaml 是 ClusterPropagationPolicy(cluster-scope · not namespaced)
  - karmada_aggregated.go ErrAggregatedAPIDisabled sentinel 与 Enabled flag 对应 · FromEnv 三 boolean variants("true"/"1"/empty)正确

## Carry-forward

- **P11-T-104** ClusterQuota CRD types + admission webhook ext:本 task ship ClusterPropagationPolicy YAML · T104 落 CRD types + controller integration + Quota controller cross-cluster aggregation 走 karmada_aggregated.go helper
- **P11-T-106** O2 DMS authn chart wiring 时:加 `O2DMS_KARMADA_AGGREGATED_ENABLED` env injection from chart values
- **P11-T-201** master-demo-multi-site.sh:apply 4 policies → create ModelService on host → 观察 member1/member2 都看到 → cross-cluster propagation 端到端 demo
- **Phase 12+ in-cluster SA**:O2 DMS Adapter 部署在 host cluster 时 · 走 ServiceAccount + Karmada cluster-scope RBAC 而非 file-based kubeconfig · Vault Secret 路径 per ADR-0018 §4 (c)

## §0a 续 autonomous · 继续 T104 ClusterQuota CRD
