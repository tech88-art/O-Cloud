# Deploy Profile — REAL edition(物理数据版)

所有逻辑资源接 **真实 in-cluster 数据**(K8s API / Pool CRD / Prometheus / ConfigMap /
真 ResourceSlice 拓扑 / 真 Deploy)。**Phase-13 已把 real 版全部 stub/mock 填成真体**;
剩余仅 **真机运行验证**(lab-gated · 见 `tests/e2e/real/`),软件层完整。

## 真接现状(Phase-13 全 ✅)

| 资源 | 真版来源 | 状态 |
|---|---|---|
| clusters / nodes / npus / workloads / logs | in-cluster K8s API | ✅ 真 |
| pools | Pool CRD(crd.Source) | ✅ 真 |
| metrics | Prometheus(kube-prometheus-stack) | ✅ 真 |
| presets | ConfigMap `ocloud-system/ocloud-presets` | ✅ 真(需先建 ConfigMap) |
| **topology** | k8s `GetTopology`(ResourceSlice `hccs_ring`/`numa_node` 聚合) | ✅ 真(**P13-T-103** · 备选 crd 读 `NPUPool.status.hccsTopology`) |
| **deploy** | k8s `Deploy()/DeleteDeploy()`(client-go apply Deployment+Service) | ✅ 真(**P13-T-104** · backend SA RBAC 见 chart `rbac.deployWrite`) |
| NPU 真分配 | npu-dra-driver `real-ascend`(npu-smi/DCMI) | ✅ 真体(**P13-T-101** · 真机 lab 验对接 = `tests/e2e/real/`) |
| 真 telemetry | exporter DCMI/npu-smi(simulator off) | ✅ 真体(**P13-T-102**) |
| authz / secret / quota / Karmada HA / SLA | 见 build-doc §5(全 land) | ✅ **P13-T-201..206** |

## 前置(上真集群前必备)
1. **arm64 K8s 集群**(鲲鹏 920 + openEuler · K8s 1.34+ DRA `resource.k8s.io/v1`)。
2. **Ascend Device Plugin** 已装(节点带 `huawei.com/Ascend910B` 容量/标签)。
3. **Pool CRD** 已 apply:`make -C operators/pool-operator install`(或 chart hook)。
4. **kube-prometheus-stack** 已装(`monitoring` ns)→ exporter/inference/backend 接它。
5. **presets ConfigMap**:`kubectl -n ocloud-system create configmap ocloud-presets --from-file=...`(key=JSON PresetDetail),否则 `/api/v1/presets` 500。
6. **mock-data 缺口 = CLOSED(P13-T-301)**:topology + deploy 转真源(P13-T-103/T-104)后,demo-backend 真集群部署**不再引用任何 mock-data fixture** → 无需挂 `demo-backend-mockdata` ConfigMap。config.real.yaml 全部 `mapping.*` 接真源,mock 块仅作应急回退保留(`enabled: true` 但无 mapping 引用)。

## 怎么选这版验证
```bash
# 单组件
helm upgrade --install <chart> deploy/helm-charts/<chart> \
  -n <ns> -f deploy/profiles/real/<chart>.values.yaml
# 或经 install.sh(K8s helm 步用 real profile 值 · +secrets/authz)
scripts/install.sh --profile real --all-phase-4 --with-secrets
```

## 真机端到端验证(lab-gated · 软件层已完整)
real 版软件层 Phase-13 全部 land(real-Ascend body / 真拓扑 / 真 Deploy / 真 telemetry /
OIDC-RBAC / Vault / 配额真强制 / Karmada HA / P99 SLA)。剩余仅**真机运行对接验证**:
跑 `bash tests/e2e/real/connection-stamps.sh`(build-doc §4.4 五项对接 stamp)+
`tests/sla` P99 压测 · 结果记入 `docs/checkpoint-phase13.md`。
见 `docs/build-and-production-validation.md` §4.4/§5(全 land · 真机实测随 lab stamp)。
