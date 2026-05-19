# Ascend Device Plugin — Phase 4 Primary Path Research

> 调研日期:2026-05-17
> 任务包:P1-T-012(子任务 1 / 3)
> 调研者:ephemeral subagent
>
> **Phase 2 status update (2026-05-18, P2-T-008)**: 演示后端不直接调
> ADP,而是通过 `huawei.com/Ascend910B` capacity 标签由 `k8s.Source`
> 推断 NPU 数量(见 `backend/pkg/datasource/k8s/npu.go`)。配套的
> `ascend-npu-exporter` 由 `deploy/helm-charts/ascend-npu-exporter/`
> Helm chart 部署,Prometheus 通过 ServiceMonitor 抓取(K8s 路径)
> 或通过 `docker compose --profile ascend` 拉起 nginx 静态 stub
> (dev 路径,不需要 Ascend 实机)。Phase 2 暂只用 whole-card 健康+利用率
> 指标;切片/PID 级别仍按"Phase 3+"计划留待 exporter-plus 自研。

## TL;DR

Ascend Device Plugin(以下简称 ADP)是 Phase 4 整卡分配路径的**可行主路径**,确认采用。建议锁定 v6.0.0(2024-12-15 稳定版,K8s 1.25–1.32 兼容)作为生产基线,或在 7.0.RC1(2025-04-27)经内部回归后切换 [A]。在 K8s 1.31 上,**whole-card 分配 + 健康监控 + HCCS 拓扑感知**已是 Huawei Cloud CCE / ModelArts 生产环境长期跑通的能力 [B];fixed-template 切片(vir01/vir02/vir04)在 ADP 上游也支持,但需运营商提前 `npu-smi set` 静态创建 vNPU [A · 华为云手动虚拟化文档]。**Catch**:ADP 与 Ascend Docker Runtime 容器运行时强耦合,要求 NPU driver ≥24.x、配套的 containerd shim 配置;且 ADP 上游主仓库已迁移至 `gitee.com/ascend/mindxdl`(再转 Gitcode),URL/CI 来源需在 Phase 4 启动时再 verify 一次。

## 1. Current State (as of 2026-05)

- **主仓库**:`gitee.com/ascend/ascend-device-plugin` 仍可访问,但页面标注"已转移至 `gitee.com/ascend/mindxdl`",MindCluster 仓库再迁移至 Gitcode 平台 [A · gitee README]。
- **最新版本**:
  - **v6.0.0 — 2024-12-15**(stable,推荐 Phase 4 起步)[A · mindxdl README]
  - **v7.0.RC1 — 2025-04-27**(release candidate)[A · mindxdl README]
  - 知识截止前未见 v7.0.0 GA;若 2025-Q4 / 2026 出新版,Phase 4 启动时再 verify [unverified]。
- **维护方**:Huawei MindCluster 团队(原 MindX DL),官方商业支持入口为 `kangfuan2@huawei.com` [A · gitee README]。
- **生产部署**:
  - **Huawei Cloud CCE AI Suite (Ascend NPU)** add-on v2.1.63 已包装 ADP,支持 K8s 1.25–1.32,且为 ModelArts / CloudMatrix384 底层调度组件 [A · Huawei Cloud doc; B · CloudMatrix384 arXiv 2506.12708]。
  - 第三方分发:DaoCloud 企业版 kpanda、HAMi 项目(对 ADP 二次封装,加入软切片)[B]。

边界:"production-ready" 指 Huawei Cloud 自家 CCE / ModelArts(规模数千张 910B)+ DaoCloud 中国区客户;**社区(非华为)在 K8s 1.30+ 自建集群上的回归报告偏少**,本项目相当于"小规模社区生产"场景,需自行回归。

## 2. K8s 1.31+ 兼容性

- **官方矩阵**:Huawei Cloud CCE add-on 显式支持 K8s **v1.25 / 1.27 / 1.28 / 1.29 / 1.30 / 1.31 / 1.32**(2.1.63 版)[A · `support.huaweicloud.com/eu/usermanual-cce/cce_10_0239.html`]。
- **Die affinity scheduling**(910B 拓扑亚卡级感知)显式要求 K8s ≥ **v1.23.18 / 1.25.13 / 1.27.10 / 1.28.8 / 1.29.4 / 1.30.1**;1.31 / 1.32 默认满足 [A · `cce_10_0980.html`]。
- **结论**:K8s 1.31 在 Huawei 官方支持矩阵内,不存在已知阻断性不兼容。
- **未知**:K8s 1.32 以上 ADP 是否仍走 v1 Device Plugin API(无变更)— 上游未公开矩阵超过 1.32 [unverified]。

## 3. 功能覆盖

| 能力 | ADP 原生支持 | 备注 |
|---|---|---|
| **Whole-card allocation**(整卡) | ✅ | `huawei.com/Ascend910B: 1` [A · HAMi README / Huawei Cloud doc] |
| **Fixed-template 静态切片**(vir01/02/04/08/16) | ✅(需手动) | 运营商先 `npu-smi set` 创建 vNPU,ADP 重启后上报为 `huawei.com/Ascend910B-2c` 等资源 [A · `cce_10_0994.html`] |
| **Automatic 静态切片** | ✅(CCE add-on 路径) | Huawei Cloud `cce_10_1010.html` 自动虚拟化 [B] |
| **Dynamic soft-slicing**(运行时切分) | ❌(原生不支持) | 需 HAMi 二次封装,且**仅支持 ARM 平台**(libvnpu.so 拦截)[A · HAMi README] — 本项目 amd64 only,**不可用** |
| **Health monitoring** | ✅ | NPU-Exporter 配套,要求 driver ≥24.x [A · `cce_10_0239.html`] |
| **HCCS 拓扑感知** | ✅(Volcano + ADP 协同) | 8×910 分两组,4 卡 HCCS / 4 卡 PCIe;ADP 上报拓扑、Volcano 实际打分 [A · `cce_10_0980.html`] |

**Phase 4 用法**:仅使用 whole-card + health monitoring;切片留待 Phase 5+ 评估。

## 4. 与本项目集成

- **架构位置**:架构文档 §3.4(NPU 资源层)+ ADR-0001 §7。ADP 取代"自研 NPU DRA Driver"作为 Phase 4 主路径,后者降级为 Phase 5 备选(待 K8s 1.34 DRA GA 后评估)。
- **资源模型**:Pod 请求 `huawei.com/Ascend910B: 1`;Operator 通过 `nodeSelector` + Volcano scheduler 实现拓扑感知 [A · HAMi README + Huawei Cloud doc]。
- **MindCluster 关系**:ADR-0001 §7 明确不引入完整 MindCluster(其包含 ClusterD、HCCL-Controller、ascend-for-volcano 等多组件)。**仅取 ADP + NPU-Exporter 子集**,Volcano 用社区版即可。验证:gitee README 显示 ADP 可独立部署(`build/ascendplugin-volcano.yaml` 仅依赖 Volcano,非整个 MindCluster)[A · gitee build YAML]。
- **DRA 迁移路径**:
  - K8s 1.32 / 1.33:DRA 仍为 Beta,**不切换** [A · Kubernetes DRA doc]。
  - K8s **1.34 DRA GA**(2025-Q3 上游 GA;OpenShift 4.21 同步 GA)[A · Red Hat developers 文章 2026-03-25]。
  - 上游尚无 Huawei 官方 Ascend DRA driver;社区 dra-driver-nvidia-gpu 为参考实现 [unverified for Ascend]。
  - **迁移可行性**:ADP 与 DRA 在 kubelet 层都通过 CDI / Pod resource API 暴露设备,workload 侧只需改 Pod spec(`resources.claims` 替代 `resources.limits`),**不破坏容器镜像或 NPU driver 栈**。Operator 可在 reconcile 层同时支持两条路径(feature gate),实现渐进切换。

## 5. 主要风险 / Gotchas

1. **Container runtime 强耦合**:ADP 必须搭配 **Ascend Docker Runtime**(`Ascend-docker-runtime_5.0.RC2_linux-x86_64.run` 量级版本)+ 在 `/etc/containerd/config.toml` 注入 shim 配置,否则容器内 `/dev/davinci*` 不可见 [A · DaoCloud kpanda 文档 / RiseUnion blog]。这意味着**节点初始化必须由 Operator/DaemonSet 注入**,不能依赖通用 containerd 默认配置。
2. **NPU driver / firmware 版本耦合**:NPU-Exporter 要求 driver ≥24.x;ADP v6.0.0 推荐配套 driver 23.0.x+(具体表见 mindxdl README 配套矩阵)[A]。Driver 升级与 ADP 升级要协同 rollout。
3. **NUMA 感知不完整**:ADP 上报 HCCS 拓扑,但**节点级 NUMA**(NPU 与本地 CPU socket 的亲和)需依赖 K8s topology manager 单独配置,ADP 本身不强制 [B · Koordinator + huaweicloud topology doc 推断]。
4. **HCCS 仅在 die-affinity scheduling 启用时生效**,且仅 K8s 补丁版本 ≥1.30.1 / 1.29.4 等才支持 [A · `cce_10_0980.html`];生产 K8s 集群一定要锁补丁版本。
5. **NPU error 时容器行为**:ADP 上报 health 状态变 Unhealthy,但 Pod 不会自动重启;需 Operator 层加 liveness probe + 拓扑感知重调度逻辑 [D · 基于 K8s Device Plugin v1 通用机制推导,缺乏 ADP 特定文档]。
6. **仓库迁移风险**:gitee.com/ascend/ascend-device-plugin → mindxdl → Gitcode,2 次迁移意味着 CI URL、镜像 registry、文档锚点都可能在 Phase 4 启动时再变 [A · 两个 README 都标注转移]。CI/CD 要做 URL 抽象层。
7. **HAMi 软切片诱惑陷阱**:HAMi 文档显眼,但其 `hami-core` 模式**仅 ARM 平台**,amd64 only 项目用不上 [A · HAMi README]。

## 6. Phase 4 推荐

**主路径决策**:

- 锁定 ADP **v6.0.0**(stable,2024-12-15)作为 Phase 4 起始版本。
- K8s 集群锁定 ≥ **1.31.x**(满足 die-affinity 补丁阈值,且与 CCE 矩阵对齐)。
- 容器运行时:**containerd 1.7.x** + **Ascend Docker Runtime 5.0.RC2+**(具体版本由 driver 决定)。
- 资源模型:`huawei.com/Ascend910B`(整卡),Phase 4 不启用切片。
- Volcano scheduler:启用,负责 HCCS topology-aware 调度。
- **不引入完整 MindCluster**,仅 ADP + NPU-Exporter。

**配置规范初稿**(Operator 默认值):

```yaml
ascendDevicePlugin:
  version: "v6.0.0"
  image: "swr.cn-north-4.myhuaweicloud.com/ascendhub/ascend-device-plugin:v6.0.0"  # [待核实 registry URL]
  containerRuntime: "containerd"
  ascendDockerRuntime:
    version: ">=5.0.RC2"
    configPath: "/etc/containerd/config.toml"
  npuDriver:
    minVersion: "24.0.0"  # NPU-Exporter 要求
  resourceName: "huawei.com/Ascend910B"
  scheduler: "volcano"
  hccsTopology: true
  diaAffinity: true        # K8s ≥1.30.1 / 1.31 满足
featureGates:
  dynamicResourceAllocation: false  # Phase 4 不启用;K8s 1.34 GA 后再评估
```

**升级路径门槛**:
- K8s 1.34 升级 + 出现官方 Ascend DRA driver(任一条件不满足则继续 ADP)。
- 内部 staging 集群跑通 30 天回归。

## 7. Sources

- [ascend-device-plugin (gitee, 原仓库)](https://gitee.com/ascend/ascend-device-plugin) — accessed 2026-05-17
- [Ascend/mindxdl (gitee, 当前主仓)](https://gitee.com/ascend/mindxdl) — accessed 2026-05-17,显示 v6.0.0 (2024-12-15) / v7.0.RC1 (2025-04-27)
- [HAMi Ascend Device Plugin README](https://github.com/Project-HAMi/ascend-device-plugin/blob/main/README.md) — accessed 2026-05-17(注:HAMi 是上游 ADP 的二次封装,但 README 准确描述了 huawei.com/Ascend910B 资源名与 hami-core ARM 限制)
- [Huawei Cloud CCE AI Suite (Ascend NPU) 文档](https://support.huaweicloud.com/eu/usermanual-cce/cce_10_0239.html) — accessed 2026-05-17,K8s 1.25–1.32 兼容矩阵
- [NPU Topology-aware Affinity Scheduling on a Single Node](https://support.huaweicloud.com/intl/en-us/usermanual-cce/cce_10_0980.html) — accessed 2026-05-17,K8s 补丁版本要求
- [Manual NPU Virtualization (Huawei Cloud)](https://support.huaweicloud.com/intl/en-us/usermanual-cce/cce_10_0994.html) — accessed 2026-05-17,vir01/02/04 模板与 `npu-smi set` 流程
- [Automatic NPU Virtualization (Huawei Cloud)](https://support.huaweicloud.com/intl/en-us/usermanual-cce/cce_10_1010.html) — accessed 2026-05-17
- [Kubernetes Device Plugins (官方文档)](https://kubernetes.io/docs/concepts/extend-kubernetes/compute-storage-net/device-plugins/) — accessed 2026-05-17
- [Kubernetes Dynamic Resource Allocation](https://kubernetes.io/docs/concepts/scheduling-eviction/dynamic-resource-allocation/) — accessed 2026-05-17,DRA Beta on 1.32/1.33
- [Red Hat Developer — DRA GA in OpenShift 4.21 (2026-03-25)](https://developers.redhat.com/articles/2026/03/25/dynamic-resource-allocation-goes-ga-red-hat-openshift-421-smarter-gpu) — accessed 2026-05-17,确认 K8s 1.34 DRA GA
- [Serving LLMs on Huawei CloudMatrix384 (arXiv 2506.12708)](https://arxiv.org/html/2506.12708v3) — accessed 2026-05-17,生产规模 384× Ascend 910 部署案例
- [RiseUnion — Ascend NPU Virtualization Guide (910 / 310P)](https://www.theriseunion.com/en/blog/HAMi-ascend-910b-support.html) — accessed 2026-05-17(二手综述,B 级)
- [DaoCloud — Install Ascend NPU Components](https://docs.daocloud.io/en/kpanda/user-guide/gpu/ascend/ascend_driver_install/index.html) — accessed 2026-05-17,Ascend Docker Runtime + containerd shim 配置实操
