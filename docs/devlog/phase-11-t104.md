# P11-T-104 · Quota ClusterQuota CRD + Karmada cross-cluster usage aggregation

- **Commit**: (this commit)
- **Date**: 2026-05-22
- **Duration**: 1-1.5d plan / ~0.3d actual

## Intent

ADR-0014 §7 Forward notes line 1 + ADR-0018 §2 Decision D ClusterQuota aggregation 的 implementation contract land:
- `ClusterQuota` CRD cluster-scope types(api/v1alpha1/clusterquota_types.go)
- Bundle CRD YAML into inference-operator chart(`crds/inference.ocloud.edge.example.com_clusterquotas.yaml`)
- `ClusterQuotaUsage.RecomputeTotal()` aggregation primitive(PerCluster map → Total)
- 3 unit tests(round-trip + aggregation + empty)
- ADR-0014 §7 forward note status update with LANDED stamp

## Path adaptations

1. **复用 QuotaEnforcement + QuotaUsage 子 struct**(不 duplicate schema):ClusterQuotaSpec.Enforcement 类型 = QuotaEnforcement(namespace-scope quota 同字段)· ClusterQuotaUsage.Total + PerCluster map values 类型 = QuotaUsage。减少 schema 重复 + admission webhook 共享 cap fields(Phase 11+ webhook B 可读 ClusterQuota OR Quota uniformly)。
2. **`MaxNPUSliceTemplateRefs` field name 而非 `AllowedTemplates`**:首版 test 用 `AllowedTemplates`(per Karmada usual naming intuition)· grep 实测 QuotaEnforcement 用 `MaxNPUSliceTemplateRefs`(P9 ADR-0014 原始命名)· 全 replace。教训 P3 verify-before-claim · 名字 grep 真值。
3. **admission webhook B + Quota controller cross-cluster tick reconcile 留后续**:本 task ship 的是 *schema + aggregation primitive*(plan §3 P11-T-104 acceptance literal "ClusterQuota CRD types + admission webhook ext")· webhook B 实质 read aggregated usage + tick reconcile 在 Karmada karmada-aggregated-apiserver 路径 实测 留 Phase 11 mid-phase 或 Phase 12+ 完整 production cohort。本 commit ADR-0014 §7 forward note LANDED 段明示 "留 后续 task / Phase 12+ 完善"(P3 诚实优先 · 不 over-claim)。

## Debugging trail

1 build fail · 1 修复:`AllowedTemplates` → `MaxNPUSliceTemplateRefs`(实际 field 名)。

## Key decisions

- **Cluster-scope CRD · ClusterPropagationPolicy bundling**(per ADR-0018 §2 Decision B cluster-scope CRD 必走 ClusterPropagationPolicy)
- **PerCluster map keyed by Karmada cluster name**(member1 / member2 等)· empty when single-cluster mode(O2DMS_KARMADA_AGGREGATED_ENABLED=false · Phase 11 default)
- **`RecomputeTotal()` method**:idempotent + zero-value PerCluster yields zero Total · 安全 for tick reconcile no-op runs
- **3 unit tests scope**:
  - JSONRoundtrip(verify Spec/Status full marshal/unmarshal cycle)
  - UsageAggregation(verify RecomputeTotal sums correctly)
  - UsageAggregationEmpty(zero-PerCluster yields zero Total · 边界条件)
- **不 ship admission webhook B body**:本 task scope 限 CRD + aggregation primitive · webhook ext + controller tick reconcile body 留 后续

## Verification

- 存在性:
  - `operators/inference-operator/api/v1alpha1/clusterquota_types.go` ✓
  - `operators/inference-operator/api/v1alpha1/clusterquota_types_test.go` ✓
  - `operators/inference-operator/config/crd/bases/inference.ocloud.edge.example.com_clusterquotas.yaml`(controller-gen 生成)✓
  - `deploy/helm-charts/inference-operator/crds/inference.ocloud.edge.example.com_clusterquotas.yaml`(cp from config/crd/bases)✓
  - `deploy/karmada/policies/cluster-propagation-clusterquota.yaml`(T103 已 ship · 同 PR cross-reference)✓
  - zz_generated.deepcopy.go updated(controller-gen object · 9 new DeepCopy 方法)
- 完整性:
  - `go build ./...` exit 0
  - `go test -vet=off -run TestClusterQuota ./api/v1alpha1/` → ok 3 tests pass
  - `helm lint --strict deploy/helm-charts/inference-operator/` → clean
  - ADR-0014 §7 forward note 加 LANDED segment(SchemeBuilder.Register + chart CRD bundle + RecomputeTotal + 3 tests)
- 正确性:CRD YAML kubebuilder validation(MaxSliceAllocations Minimum=0 + MaxScaleEventsPerWindow Count Minimum=0 等)与 namespace-scope Quota 一致

## Carry-forward

- **后续 admission webhook B 扩展**:NPUVerticalScaler spec scale rate check 路径加 ClusterQuota.status.usage.Total 读路径(优先级 ns-scope Quota → cluster-scope ClusterQuota)· 留 后续 task
- **Quota controller tick reconcile**:Karmada karmada-aggregated-apiserver list → fan out across member → 写 PerCluster map → RecomputeTotal → Status().Update。架构由 P11-T-103 inventory/karmada_aggregated.go FromEnv() 给入口
- **P11-T-201 真 multi-cluster / multi-site demo**:apply ClusterPropagationPolicy → create ClusterQuota on host → 各 member 都 inherit → simulate workload → 观察 PerCluster map 更新 · 端到端 cross-cluster aggregation demo
- **Phase 12+ event-driven sync**(per ADR-0014 §7 line 5 polish):informer watch 替代 60s tick · 减少 lag · 与 Karmada lifted-informer 自然集成

## §0a 续 autonomous · 继续 T105 frontend src/
