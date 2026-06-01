# CANN 8.1 / Ascend driver ≥ 24.x 兼容矩阵

> **目标**:Phase 4 启动前对齐 NPU/AI 运行时栈版本矩阵 — host kernel × Ascend driver × CANN × MindIE Turbo × vllm-ascend × kubelet × verdict。
>
> **范围**:本文是**版本兼容性参考**,**不是真机验证报告**。Phase 4 全部 task(T003-T006 npu-dra-driver scaffold + simulator publisher + claim controller)运行在 `configs/mock-data/set-a-small/` 合成数据集上,**不依赖真实 NPU / CANN / driver 二进制**。真机验证 deferred Phase 7(real-cluster 阶段,arch §1.3)。
>
> **来源标注约定**:
> - `[ref 2026-05-19]` — URL 与版本号结构性参考(canonical 项目 URL / 公开 release schedule),写文档时本地核对 · **未** automated URL fetch
> - `[verified YYYY-MM-DD]` — Phase 7 真机验证后由 operator 补,标 npu-smi / cann install validator / mindie-turbo health-check 实际输出
>
> **相关 ADR**:ADR-0001 v3(双轨路径)· ADR-0002(no-KServe)· **ADR-0020(aarch64 鲲鹏 + openEuler 真实目标平台 · 翻转 ADR-0001 §13 amd64-only · 见 §1.1 aarch64 包矩阵)**· `docs/architecture.md` §3.4(NPU/AI 运行时)
>
> **日期**:2026-05-19 · **Task**:P4-T-002 · **状态**:Phase 4 入口落地

---

## 1. 兼容矩阵

| host kernel | Ascend driver | CANN | MindIE Turbo | vllm-ascend | kubelet | verdict | 来源 |
|---|---|---|---|---|---|---|---|
| **5.10**(LTS · Ubuntu 22.04 / openEuler 22.03) | **24.1.RC3** | **8.1.RC1** | **2.0.RC1** | **0.11.0** | **1.30** | ✅ **pass**(Phase 4 推荐基线 · Phase 7 入口) | Ascend community release notes [ref 2026-05-19] · vllm-ascend v0.11.0 release `github.com/vllm-project/vllm-ascend` [ref 2026-05-19] · CANN 8.1 release `hiascend.com/document` [ref 2026-05-19] |
| **5.10**(LTS · **openEuler 22.03 LTS SP · aarch64 鲲鹏 Kunpeng 920**)| **24.1.RC3**(aarch64 包 `*_aarch64.run`)| **8.1.RC1**(aarch64 包)| **2.0.RC1** | **0.11.0** | **1.34** | ✅ **pass**(**Phase 12 ADR-0020 真实目标平台基线** · 华为 Atlas 800 原生 Kunpeng host + 昇腾 910B + openEuler)| Ascend aarch64 release `*-aarch64.run` `hiascend.com/document` [ref 2026-06-01] · ADR-0020 · 见 §1.1 |
| **5.4**(EOL 风险 · CentOS 7.9 / Ubuntu 20.04) | **23.0.0** | **7.0** | — | — | **1.28** | ⚠️ **warn**(最低支持下限 · frontend 可渲染但 device-discovery TBD;无 PD 分离;无 vllm-ascend) | Ascend 23.0 legacy release notes [ref 2026-05-19] |
| **6.x mainline**(Ubuntu 24.04 / Fedora 40) | 24.0 | 8.0 | — | — | 1.31+ | ❌ **fail**(Ascend driver **lacks 6.x KMD 支持** as of 2026-05;npu-smi 加载内核模块失败) | Ascend community 兼容矩阵 + 用户社区 issue [ref 2026-05-19] |
| **5.15**(Ubuntu 22.04 GA · openEuler 24.03) | 24.1.RC2 | 8.0.0 | 1.0 | 0.10.x | 1.29 | ⚠️ **warn**(可运行但**非 Phase 4 推荐**;CANN 8.0 与 vllm-ascend disaggregated_prefill_v1 兼容性需验)| Ascend 24.1.RC2 release notes [ref 2026-05-19] |
| **5.10**(同基线 kernel) | 24.1.RC3 | 8.1.RC1 | 2.0.RC1 | 0.11.0 | **1.34**(DRA GA) | ⚠️ **warn**(Standard-K8s small-cluster DRA spike 路径 · ADR-0001 v3 §5 · KubeEdge 不可用 · npu-dra-driver 真机集成 Phase 5+) | ADR-0001 v3 · `kubernetes.io/blog` K8s 1.34 [ref 2026-05-19] |

**verdict 标记说明**:
- ✅ **pass** — Phase 4 推荐基线,Phase 7 真机入口预期通过
- ⚠️ **warn** — 可装可跑但**非推荐组合**;某些场景受限(无 PD 分离 / 仅 DRA spike 路径 / vllm-ascend 兼容性待验)
- ❌ **fail** — **不要使用**;已知 driver/kernel/CANN 不兼容

### 1.1 aarch64 鲲鹏 + openEuler 包矩阵(Phase 12 ADR-0020 真实目标平台)

ADR-0020(2026-06-01)把目标平台从 amd64-only 翻转为 **aarch64 鲲鹏(Kunpeng 920)+ openEuler**(华为 Atlas 800 原生配置)。CANN / Ascend driver 包**按 host CPU 架构分发**,aarch64 host 用 **aarch64 变体包**(非 x86_64):

| 组件 | x86_64 包(原 amd64) | **aarch64 包(鲲鹏 · Phase 12 target)** |
|---|---|---|
| Ascend driver | `Ascend-hdk-<ver>-npu-driver_<ver>_linux-x86_64.run` | `..._linux-aarch64.run` |
| CANN toolkit | `Ascend-cann-toolkit_<ver>_linux-x86_64.run` | `..._linux-aarch64.run` |
| CANN kernels | `Ascend-cann-kernels-910b_<ver>_linux-x86_64.run` | `..._linux-aarch64.run` |
| vllm-ascend 镜像 | `vllm-ascend:<tag>`(multi-arch manifest · 见 P12-T-101) | 同 manifest · 拉 arm64 layer |

**关键点**:
- **版本兼容性 arch-agnostic**:§1 矩阵的 driver×CANN×vllm-ascend 版本组合对 x86_64 / aarch64 **相同**(只是二进制包不同)· 故 §1 矩阵 verdict 对两架构通用 · aarch64 行只是 host 平台标注
- **节点 OS**:openEuler 22.03 LTS SP / 24.03 LTS(华为主导 · Atlas 系列原生适配 · 见 install.sh openEuler 包管理 dnf 路径)
- **真硬件验证**:真鲲鹏 920 + 昇腾 910B 集群 CANN aarch64 包安装 + `npu-smi info` 验证 = **lab-gated · Phase 13+**(ADR-0020 §4 (a) · Track A carry)· Phase 12 仅交叉编译 + helm template arch 亲和校验(不接触真 aarch64 CANN 二进制)
- **镜像**:容器镜像 multi-arch buildx(P12-T-101 · arm64 layer)· 容器内不含 CANN(CANN 在 host · 容器通过 `/dev/davinci*` + driver mount 访问 · 见 §4.3)

---

## 2. 字段语义

| 字段 | 说明 |
|---|---|
| **host kernel** | 物理节点 / VM 的 Linux kernel,Ascend driver 依赖 KMD(kernel module driver)|
| **Ascend driver** | Huawei Ascend NPU device driver(用户态 + KMD 组合 · `npu-smi info` 报告 driver version)|
| **CANN** | Compute Architecture for Neural Networks · NPU 运行时基础库(算子 / runtime / toolkit · Ascend Cann 包安装到 `/usr/local/Ascend/`)|
| **MindIE Turbo** | vllm-ascend 可选加速 backend(LLM 推理算子加速 · 与 CANN 同源生态;Phase 4 仅记录 · Phase 5 实际启用判定)|
| **vllm-ascend** | vLLM Ascend backend(Ocloud 推理服务底座 per ADR-0002 no-KServe)· v0.11.0 引入 `disaggregated_prefill_v1`(Phase 5 PD 分离前提)|
| **kubelet** | K8s kubelet 版本(影响 DevicePlugin v1 / DRA `resource.k8s.io` API 可用性 · DRA GA in 1.34)|
| **verdict** | ✅ pass / ⚠️ warn / ❌ fail · 见 §1 末尾说明 |

---

## 3. Phase 4 simulator scope(本节强制阅读)

Phase 4 全部交付物运行在**合成 NPU 数据集**上,**不接触真实 Ascend 硬件 / CANN 二进制 / driver KMD**:

| Phase 4 task | 数据来源 | 真实硬件依赖 |
|---|---|---|
| P4-T-003 npu-dra-driver Kubebuilder 骨架 | — | 无 |
| P4-T-004 ResourceSlice + ResourceClaim 类型 | upstream `resource.k8s.io/v1beta1` 类型定义 | 无 |
| P4-T-005 simulator-first ResourceSlice publisher | `configs/mock-data/set-a-small/npus.json`(合成 2 节点 × 8 NPU)| **无** · 读 JSON 发布 ResourceSlice |
| P4-T-006 ResourceClaim 控制器骨架 | envtest fixture + kind cluster | **无** · 仅记录 + emit `AllocationDeferred=Phase4Skeleton` |
| P4-T-101 Dockerfile + Helm chart | golang multi-stage build | **无** · 镜像不含 CANN(P12-T-101 起 multi-arch amd64+arm64 · CANN 在 host 非容器)|
| P4-T-104 kind smoke E2E 扩展 | GitHub Actions ubuntu-latest | **无** · ResourceSlice 可见性断言 |
| P4-T-105 ADR-0009 npu-dra-driver design | — | 无 |

**含义**:Phase 4 PR 不会因为本机无 910B / CANN 未装 / driver 未加载而失败。CI 也不依赖 self-hosted NPU runner。

**Phase 5+ 才开始真实硬件接入**(见 §4 真机验证 procedure)。

---

## 4. 真机验证 procedure(Phase 7 入口 checklist)

> Phase 4 不执行本节,只是把真机验证流程写下来,Phase 7 operator(或 lab 真机环境)直接照做。每步通过后把对应 `[ref 2026-05-19]` 升级为 `[verified YYYY-MM-DD]` + 实际输出截屏。

### 4.1 host kernel + Ascend driver

```bash
# kernel 版本
uname -r
# 期望:5.10.x(Phase 4 推荐基线)

# Ascend driver 是否加载
lsmod | grep -E "drv|davinci|npu"
# 期望:看到 drv_dsmi_host / davinci_manager / hisi_npu_acc 等模块

# npu-smi info — driver 版本 + 设备列表
npu-smi info
# 期望:8 张 910B(单机部署)· driver 版本 24.1.RC3
```

### 4.2 CANN 安装验证

```bash
# CANN 安装目录
ls -l /usr/local/Ascend/ascend-toolkit/latest/
# 期望:存在 `version.info` 文件,Version=8.1.RC1

# CANN install validator
source /usr/local/Ascend/ascend-toolkit/set_env.sh
ascend-dmi --info
# 期望:driver / firmware / aicore version 全部 OK

# 简单算子 sanity(可选 · MindStudio 提供)
# python3 -c "import torch_npu; print(torch_npu.npu.is_available())"
```

### 4.3 MindIE Turbo / vllm-ascend health-check

```bash
# vllm-ascend 容器自检
docker run --rm \
  --device /dev/davinci0 --device /dev/davinci_manager \
  --device /dev/hisi_hdc --device /dev/devmm_svm \
  -v /usr/local/Ascend/driver:/usr/local/Ascend/driver \
  vllm-project/vllm-ascend:v0.11.0 \
  python3 -c "import vllm; import vllm_ascend; print(vllm.__version__, vllm_ascend.__version__)"
# 期望:0.11.0 / 0.11.0

# MindIE Turbo backend(可选 · 加速场景)
# vllm-ascend 启动时 --quantization mindie-turbo
# 看 stdout 是否 "MindIE Turbo backend initialized OK"
```

### 4.4 kubelet DRA 支持(Standard-K8s 路径 · ADR-0001 v3 §5)

```bash
# kubelet 版本
kubectl version --client
# 期望:1.34+(DRA GA)· KubeEdge 路径不适用此节

# resource.k8s.io API 可用性
kubectl api-resources | grep resource.k8s.io
# 期望:resourceslices / resourceclaims / deviceclasses 三类资源可见

# ResourceSlice 由 npu-dra-driver 发布(Phase 5+ 真实 / Phase 4 simulator)
kubectl get resourceslices
# Phase 4 simulator 期望:driver=npu.ocloud.edge.example.com · 8 devices(set-a-small)
# Phase 5+ 真实期望:driver=npu.ocloud.edge.example.com · 真机扫描的 NPU 数量
```

---

## 5. 已知问题与回避

| 问题 | 影响 | 回避 |
|---|---|---|
| **Ascend driver 不支持 6.x mainline kernel**(as of 2026-05) | Ubuntu 24.04 / Fedora 40 默认 kernel 6.x → 装不上 driver | 用 5.10 LTS kernel · 或装 5.15 HWE backport |
| **CANN 8.1 与 PyTorch 2.x torch_npu 版本耦合** | 装错 torch_npu 版本 → import 报符号缺失 | 用 vllm-ascend release 自带的 torch_npu wheel · 不混装 |
| **KubeEdge v1.22 不支持 DRA** | edge 节点无法跑 npu-dra-driver | edge 路径 stay Ascend Device Plugin v1(ADR-0001 v3 §5)|
| **HCCS 拓扑信息无标准 K8s API** | scheduler-plugins NUMA+HCCS 需要 Ascend SDK 调 npu-smi | Phase 6 由 scheduler-plugin 模块负责 · Phase 4 不涉及 |
| **vllm-ascend disaggregated_prefill_v1 需要 v0.11.0+** | Phase 5 PD 分离最低版本约束 | Phase 4 锁 0.11.0 作为 floor · 5+ 才用 |

---

## 6. 引用

- `docs/adr/0001-phase0-key-decisions.md` §5 v3 — 双轨路径(Edge: Device Plugin v1 / Standard-K8s: DRA spike)
- `docs/adr/0002-no-kserve.md` — 推理服务底座 vllm-ascend Deployment(spec 第 47-48 行甲方要求)
- `docs/architecture.md` §3.4 — NPU/AI 运行时选型主表
- `docs/research/k8s-dra.md` — DRA 调研详档(P1-T-012 产出)
- `docs/phase4-plan.md` §3 P4-T-002 — 本文 acceptance 出处
- `configs/mock-data/set-a-small/npus.json` — Phase 4 simulator 数据源
- vllm-ascend repo:`github.com/vllm-project/vllm-ascend` [ref 2026-05-19]
- CANN 文档:`hiascend.com/document` [ref 2026-05-19]
- Ascend Cloud / driver 镜像:`github.com/Ascend` 组织 [ref 2026-05-19]
- K8s 1.34 DRA GA blog:`kubernetes.io/blog/2025/09/01/...` [ref 2026-05-19]

---

**END of cann-driver-matrix**
