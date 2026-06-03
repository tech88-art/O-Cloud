# Deploy Profile — REAL edition(物理数据版)

大部分资源接 **真实 in-cluster 数据**(K8s API / Pool CRD / Prometheus / ConfigMap)。
用于真集群上验证。**多处仍 lab-gated / Phase-13**,如实列下,避免误判。

## 真接 vs 仍 mock(诚实现状)

| 资源 | 真版来源 | 状态 |
|---|---|---|
| clusters / nodes / npus / workloads / logs | in-cluster K8s API | ✅ 可接真 |
| pools | Pool CRD(crd.Source) | ✅ 可接真 |
| metrics | Prometheus(kube-prometheus-stack) | ✅ 可接真 |
| presets | ConfigMap `ocloud-system/ocloud-presets` | ✅ 可接真(需先建 ConfigMap) |
| **topology** | — | ⚠️ **仍 mock** · `GetTopology` 返回 `ErrCapabilityUnavailable`(真拓扑聚合 = Phase-13) |
| **deploy** | — | ⚠️ **仍 mock** · 无真 `Deploy()` 体(Phase-13) |
| NPU 真分配 | npu-dra-driver `real-ascend` | ⚠️ **stub** · `ErrNotImplemented`(Phase-13 T101 lab body) |

## 前置(上真集群前必备)
1. **arm64 K8s 集群**(鲲鹏 920 + openEuler · K8s 1.34+ DRA `resource.k8s.io/v1`)。
2. **Ascend Device Plugin** 已装(节点带 `huawei.com/Ascend910B` 容量/标签)。
3. **Pool CRD** 已 apply:`make -C operators/pool-operator install`(或 chart hook)。
4. **kube-prometheus-stack** 已装(`monitoring` ns)→ exporter/inference/backend 接它。
5. **presets ConfigMap**:`kubectl -n ocloud-system create configmap ocloud-presets --from-file=...`(key=JSON PresetDetail),否则 `/api/v1/presets` 500。
6. **mock-data 缺口**:demo-backend chart 只挂 config、**不挂 mock-data fixtures**。topology/deploy 仍走 mock → 需另挂一个 mock-data ConfigMap 到 `/etc/demo-backend/mock-data`(参考 `tests/e2e/kind/install.sh` 的 `demo-backend-mockdata` 做法)。把 mock-data 卷接进 chart = 近期 follow-up。

## 怎么选这版验证
```bash
# 单组件
helm upgrade --install <chart> deploy/helm-charts/<chart> \
  -n <ns> -f deploy/profiles/real/<chart>.values.yaml
# 或经 install.sh(K8s helm 步用 real profile 值)
scripts/install.sh --profile real --all-phase-4
```

## 仍属 Phase-13(本 profile 不实现 · 新 session)
real-Ascend source body(npu-smi/DCMI)· 真拓扑聚合 · 真 Deploy() · OIDC/RBAC ·
Karmada HA · Vault · 配额 webhook 真强制 · P99 SLA · 真机端到端验证。
见 `docs/checkpoint-phase12.md` §6 + `docs/build-and-production-validation.md` §5。
