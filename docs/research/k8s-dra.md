# K8s DRA — Phase 4 後段 / Phase 7 Pre-research

> 调研日期:2026-05-17
> 任务包:P1-T-012(子任务 2 / 3)

## TL;DR

DRA core 已于 **K8s 1.34(2025-09-01 GA)** 真正达到 GA(`resource.k8s.io/v1` 默认启用),
ADR-0001 §5 关于"1.32 GA / 2026-06 RC"的判断与实际不符 — DRA 在 1.32(2024-12)仅
为 v1beta1 Beta,GA 已比预想提前 9 个月落地。截至 1.36(2026-05-07 blog 发布),
Partitionable Devices 处于 **Beta**(KEP-4815,1.35 Alpha → 1.36 Beta),NVIDIA GPU
DRA driver 已捐赠至 CNCF(KubeCon EU 2026),HAMi-DRA(v0.2.1, 2026-05-14)初步可用
但仅 NVIDIA。**Ascend 暂无官方 DRA driver**,HAMi 主线对 910B 的虚拟化仍走传统
Device Plugin。**KubeEdge 1.22(2026-04)显式未集成 DRA,依赖 K8s 1.31.12** — 是
我们边缘场景最大阻塞。建议:Phase 4 主路径维持 Ascend Device Plugin v1,Phase 4 後段
在小集群(标准 K8s 1.34+)做 DRA spike,Phase 7 动态切分若需突破硬切分,以 DRA
Partitionable Devices(预计 1.37 GA, `[D · estimate]`)为目标,但保留 Device Plugin
fallback 直至 KubeEdge 支持 DRA 明确化。

## 1. Status (2026-05) — GA 时间窗

| 版本 | 发布 | DRA 状态 | 关键 API | 来源 |
|---|---|---|---|---|
| 1.31 | 2024-08 | Beta(redesigned, structured params) | `resource.k8s.io/v1alpha3` | `[A · k8s.io releases]` |
| 1.32 | 2024-12 | Beta(`v1beta1`)| `v1beta1` 引入 | `[A · k8s.io releases]` |
| 1.33 | 2025-04 | Beta(`v1beta2`)| 字段调整 | `[B · refs]` |
| **1.34** | **2025-09-01** | **GA(core)** | **`resource.k8s.io/v1` 默认启用** | `[A · k8s.io/blog/2025/09/01/kubernetes-v1-34-dra-updates/]` |
| 1.35 | 2026-01 | GA + Partitionable Devices Alpha | KEP-4815 引入 | `[B · KEP-4815]` |
| 1.36 | 2026-04(blog 2026-05-07) | GA + Partitionable Devices Beta + Prioritized List GA | 多项 Beta | `[A · k8s.io/blog/2026/05/07/]` |

**ADR-0001 §5 需更正**:不是"1.32 GA / 2026-06 RC",实际 **1.34 GA / 2025-09-01**,
GA 已落地 ~8 个月。

## 2. API 模型现状

`resource.k8s.io/v1` 稳定四件套:

- **DeviceClass**(cluster-scoped):定义设备类别 + 选择属性(如 `device.type=ascend-910b`)。
- **ResourceClaim**(namespaced):Pod 申请的具体资源声明,带 status 与 allocation。
- **ResourceClaimTemplate**:Pod 自动生成 per-pod ResourceClaim 的模板。
- **ResourceSlice**(由 driver 节点侧创建):节点资源清单,publishes structured attributes;
  字段含 `devices[]`(每个设备的 name、basic capacity、attributes、counters)、`nodeName`、
  `driver`、`pool`。

**1.31 Beta → 1.34 GA 差异**:
- v1alpha3 → v1beta1(1.32)→ v1beta2(1.33)→ v1(1.34)逐版字段重排
- 1.34 起 **`v1` 默认存储版本**,旧版本仅 served-for-compat
- "Structured Parameters" 模型在 1.31 redesign 后稳定,后续主要是 alpha 扩展叠加
  `[A · k8s.io/blog/2025/09/01/]`

**字段示例**(简化):
```yaml
apiVersion: resource.k8s.io/v1
kind: ResourceSlice
spec:
  driver: ascend.npu.k8s.example.com
  nodeName: edge-node-01
  pool: { name: ascend-910b-pool, generation: 7 }
  devices:
    - name: npu0
      basic:
        attributes: { productName: { string: "910B3" } }
        capacity: { memory: { value: "32Gi" } }
```

## 3. 生态进展

- **NVIDIA**:`k8s-dra-driver-gpu`(原 NVIDIA 仓)2026-03 KubeCon EU **捐赠 CNCF**,
  转为 `kubernetes-sigs/dra-driver-nvidia-gpu`,v25.3.0-rc.3(2026-Q1);GPU Operator
  集成完成;支持 MIG + ComputeDomain `[A · NVIDIA blog + canonical.com]`。
- **AKS / GKE / EKS** 均已支持 DRA(2026-03 通用)`[B · vendor blogs]`。
- **AWS** 2026-05 发布 EFA DRA driver(网络设备亦走 DRA)`[A · aws.amazon.com 2026-05]`。
- **dra-example-driver**(kubernetes-sigs):v0.2.1 = **2026-01-09**,277 commits,
  127 stars,active maintained,目标 K8s 1.34+,Helm chart 完整,**可作 Ascend driver
  起点** `[A · github]`。
- **HAMi-DRA**(Project-HAMi):v0.2.1 = 2026-05-14,mutating webhook 模式,把
  传统 GPU 请求转为 ResourceClaim;**目前仅 NVIDIA,Ascend 未列入** `[A · github
  /Project-HAMi/HAMi-DRA]`。HAMi 主线 ascend-device-plugin 仍走 Device Plugin 路径。
- **Ascend 官方**:Huawei 文档(Ascend Data Center Solution V100R020C30 等)仅描述
  Kubernetes Device Plugin,**未见官方 DRA driver 发布** `[A · support.huawei.com]`。

## 4. 与 KubeEdge / Edge 场景兼容性

**KubeEdge v1.22(2026-04-12 发布)release notes 显式未提 DRA / ResourceClaim /
resource.k8s.io**,且 K8s 依赖仍 `v1.31.12` `[A · kubeedge.io/blog/release-v1.22]`。
v1.22 新增的是 Pod Resources Server、CSI Plugin feature gates、C 语言 mapper-framework
— 都不是 DRA。EdgeCore 的 lightweight kubelet 默认不带 DRA kubelet plugin manager。

**结论**:KubeEdge 边缘节点 **当前不支持 DRA driver 运行**。我们的 KubeEdge + K3s
边缘形态在 Phase 4 必须走 Device Plugin。
**[D · estimate]**:KubeEdge 跟 K8s 1.34+ 依赖需 ~6-12 个月,DRA 在边缘可用
最快 2026-Q4 ~ 2027-Q1。

K3s 本身依赖 upstream K8s,1.34+ 版本可直接用 DRA(单机 / 小集群形态可行)。

## 5. 对项目 CRD 设计的反向影响

架构 §6.7 `NPUSlicePool` ↔ `ResourceSlice` 映射:

| 项目 CRD | DRA 对应 | 备注 |
|---|---|---|
| `NPUSlicePool.spec.templates`(vir01/vir02/vir04)| `ResourceSlice.spec.devices[]` + Partitionable `counters` | KEP-4815 Beta(1.36),GA `[D · 1.37 ~ 2026-Q3]` |
| `NPUSlice` allocation | `ResourceClaim` allocation status | 1:1 概念吻合 |
| `NPUDevice.attributes` | `ResourceSlice.devices[].basic.attributes` | 直接映射 |

**动态切分(Phase 7,突破硬切分)**:DRA `partitionable devices` 用 `CounterSet`
模型,driver 在 ResourceSlice 中声明"counters: { copies: 8, memGB: 64 }",每个
partition template 消耗一定 counter — 这正是动态切分需要的"非互斥共用底层资源"
模型。**结论:DRA Partitionable Devices 在表达力上覆盖 Phase 7,需求等其 GA**。

CRD 设计建议:**保留 NPUSlicePool 作为 stable 抽象层**,内部 reconciler 在 Phase 4
对 Device Plugin、Phase 7 对 DRA 都能产出。避免直接把 ResourceSlice 暴露给上层。

## 6. 风险 / Gotchas

1. **`v1beta1` → `v1` breaking 字段**:KEP changelog 显示字段重排,Phase 4 後段 spike
   必须直接用 v1,不要 invest 在 beta API `[A · KEP-4381]`。
2. **KubeEdge DRA 真空**(主风险):见 §4。
3. **Vendor 跟进不足 / 国产芯片**:Ascend / 寒武纪 / 海光均无官方 DRA driver(2026-05)
   `[B · 检索结果]`。我们若自研 Ascend DRA driver,等于 vendor 角色,维护成本高。
4. **Partitionable Devices 仅 Beta**:Phase 7(预计 ~6 个月後启动)若 1.37 GA 未到,
   仍要 fallback。
5. **生产采用率**:可获取的二手数据仅"已 GA、ecosystem 起来",**实际生产 DRA % 未有
   权威数字** `[待核实]`。`[D · estimate]` 大型 GPU 集群(NVIDIA)2026 内 DRA 采用 ≥30%,
   边缘 / 国产芯片场景 <5%。
6. **Allocation latency**:官方未公布 controller overhead benchmark `[待核实]`。
   `[D · estimate]` 单 ResourceClaim 调度增加 ~50-200ms vs Device Plugin(scheduler
   plus structured params filter)— 对训练 / 长任务 inference 可忽略,对秒级 burst
   推理需评估。

## 7. 与 Phase 4/7 时间线对齐

| 阶段 | 推荐路径 | 触发 DRA 切换条件 |
|---|---|---|
| Phase 4 主路径(now ~ +3m) | **Ascend Device Plugin v1** | — |
| Phase 4 後段(+3m ~ +6m) | 小集群(标准 K8s 1.34+)上 **DRA spike**:fork `dra-example-driver`,改造为 Ascend mock,验证 ResourceClaim 走通 | 不阻塞 Phase 5 |
| Phase 7 动态切分(+6m ~ +12m) | 若 K8s 1.37 Partitionable Devices GA `[D]` + KubeEdge 跟进 → 全面切 DRA;否则 Device Plugin + 自研切分 controller | 1.37 GA 推迟 / KubeEdge 仍无 DRA → 推 Phase 8 |
| Phase 9+ Karmada 多站点 | DRA 跨集群 federation 状态 `[待核实]`,可能仍 Device Plugin | 需独立调研 |

**Fallback(ADR-0001 §5 修订)**:
- 若 1.34 GA(实际)→ Device Plugin v1 + DRA spike 并行 ✓
- 若 KubeEdge 2026-Q4 仍无 DRA → Phase 7 边缘形态走 Device Plugin + 自研动态切分
- 若 Ascend 官方 2026-Q3 前发 DRA driver → 直接接入,跳过自研 dra-example-driver fork

## 8. Sources

- [Kubernetes v1.34: DRA has graduated to GA](https://kubernetes.io/blog/2025/09/01/kubernetes-v1-34-dra-updates/) — accessed 2026-05-17
- [Kubernetes v1.36: More Drivers, New Features, and the Next Era of DRA](https://kubernetes.io/blog/2026/05/07/kubernetes-v1-36-dra-136-updates/) — accessed 2026-05-17
- [Dynamic Resource Allocation | kubernetes.io docs](https://kubernetes.io/docs/concepts/scheduling-eviction/dynamic-resource-allocation/) — accessed 2026-05-17
- [KEP-4381 DRA Structured Parameters](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/4381-dra-structured-parameters/README.md) — accessed 2026-05-17
- [KEP-4815 DRA Partitionable Devices](https://github.com/kubernetes/enhancements/issues/4815) — accessed 2026-05-17
- [kubernetes-sigs/dra-example-driver](https://github.com/kubernetes-sigs/dra-example-driver) v0.2.1 (2026-01-09) — accessed 2026-05-17
- [kubernetes-sigs/dra-driver-nvidia-gpu](https://github.com/kubernetes-sigs/dra-driver-nvidia-gpu) — accessed 2026-05-17
- [NVIDIA donates DRA Driver for GPUs to CNCF](https://blogs.nvidia.com/blog/nvidia-at-kubecon-2026/) — accessed 2026-05-17
- [Project-HAMi/HAMi-DRA](https://github.com/Project-HAMi/HAMi-DRA) v0.2.1 (2026-05-14) — accessed 2026-05-17
- [Project-HAMi/k8s-dra-driver](https://github.com/Project-HAMi/k8s-dra-driver) — accessed 2026-05-17
- [HAMi: Enable Ascend sharing](https://project-hami.io/docs/userguide/ascend-device/enable-ascend-sharing) — accessed 2026-05-17
- [KubeEdge v1.22 release notes](https://kubeedge.io/blog/release-v1.22/) (2026-04-12) — accessed 2026-05-17
- [Huawei Ascend Kubernetes Device Plugin docs](https://support.huawei.com/enterprise/en/doc/EDOC1100192457/eeadb129/kubernetes-device-plugin) — accessed 2026-05-17
- [AWS: DRA for Elastic Fabric Adapter](https://aws.amazon.com/about-aws/whats-new/2026/05/kubernetes-dra-elastic-fabric-adapter/) — accessed 2026-05-17
- [AKS Engineering Blog: MIG with DRA on AKS](https://blog.aks.azure.com/2026/03/03/multi-instance-gpu-with-dra-on-aks) — accessed 2026-05-17
