# vLLM PD-Disaggregation — Phase 5 Path Research

> 调研日期:2026-05-17
> 任务包:P1-T-012(子任务 3 / 3)
> 关联:ADR-0001 §1.3、ADR-0002(去 KServe)、architecture.md §3.4 / §13、Phase 5 Qwen 8B PD 真实部署
>
> **修订记录**:
> - 2026-05-17:对齐 ADR-0002(从技术栈基线移除 KServe)。§6 标题改为「vllm-ascend 集成方案」并重写;§7.5 同步更新;§8 推荐表删 KServe 集成行,新增 K8s 表达行。spec/OR-requirements.md 第 47-48 行甲方明确要求「不用 KServe」是本次重写的根本动因。

## TL;DR

vLLM PD 在 2026-05 仍标 `experimental`,但 API 已经稳定到可生产(Meta、LinkedIn、Mistral、HuggingFace 已上线 [B])。Ascend 适配走 `vllm-ascend` 项目,v0.10.1rc1 起支持 PD,v0.11.0 已含 Mooncake + LLMDataDist 两套 connector,910B(Atlas 800T A2)拿到一等公民待遇 [B]。**Phase 5 推荐主路径**(经 ADR-0002 拍板,**已从基线移除 KServe**):两个独立的 `vllm-ascend v0.11.0` Deployment(prefill / decode)+ 自研 PD Router(基于 vllm-ascend `disaggregated_prefill_v1/proxy_server.py` 改造为 K8s Service + Controller)+ 自研 `inference-operator` 提供的 `ModelService` CRD。锁定 `vllm-ascend` tag `v0.11.0` 而非 vLLM 上游 commit。**Fallback 触发条件**:Qwen3-8B 在 1P1D 上 TTFT 改善 <30% 或 vllm-ascend 主线在 6 周内发生破坏性 API 变更 → 退回 MindIE Service 单实例(**不**回 KServe;详见 §6.2-C),PD 推迟至 Phase 6 后。

---

## 1. vLLM PD 现状(2026-05)

- **上游主线版本**:vLLM `v0.21.0`(2026-05-15 release)[A · GitHub Releases]。disagg-prefill 入口在 v0.7.x 引入,v0.10.x 接入 V1 engine,RFC #10818 仍 open(KV connector roadmap)[A]。
- **稳定度标签**:官方仍标 *experimental* — 标签反映 API 迭代速度,不代表运行时不稳定 [B · docs.vllm.ai/features/disagg_prefill]。
- **生产部署**:Meta(自有 RAN/agentic workload)、LinkedIn、Mistral、HuggingFace 已通过 vLLM 或 wrapper(llm-d / NVIDIA Dynamo)跑 PD 流量 [B]。
- **API 入口**:`--kv-transfer-config` 选 connector(NIXL / LMCache / Mooncake / P2pNccl / MooncakeConnectorV1 等)+ `kv_role: kv_producer | kv_consumer`,外加 proxy router。XpYd 拓扑(X 个 prefill,Y 个 decode,TP/PP 可不同)在 v0.10+ 主线可用 [B · DeepWiki §9.4]。

## 2. Ascend 910B 兼容性

- **vllm-ascend 项目**:vLLM Ascend backend 是独立 repo(`vllm-project/vllm-ascend`),并非 vLLM 主线分支。最新 stable `v0.18.0`(2026-04-30),pre-release `v0.19.1rc1`(2026-04-30),基于 vLLM v0.18.0 / v0.19.1 [A · GitHub Releases]。
- **PD 支持节点**:`v0.10.1rc1` 起初始 PD(基于 V1 engine),`v0.11.0` 含 Mooncake connector + LLMDataDist 两条路径,Qwen 系列在官方教程中给出 1P2D / 2P1D 两套样例 [A · vllm-ascend docs v0.11.0]。
- **硬件**:Atlas A2 系列(Ascend-cann-kernels-910b)、Atlas A3、Atlas 300I/310P 都列入支持矩阵 [A · vllm-ascend FAQs]。
- **MindIE 关系**:MindIE Turbo 已被集成进 vllm-ascend 用于 DeepSeek-V3/R1、Qwen2 系列加速 [B · release notes];但 **MindIE Service** 是 Huawei 自家的独立推理服务框架,有官方文档《Deploying the Prefill-decode Disaggregation Service using Qwen1.5-14B based on the MindIE Inference Framework》[A · support.huawei.cn]。两条栈不互通:走 vllm-ascend 则用 vLLM OpenAI API,走 MindIE Service 则用 MindIE 自己的 RESTful API。

**结论**:vllm-ascend 可以原生跑 910B PD,不需要 fork。**原冲突已通过 ADR-0002 解决**(项目从基线移除 KServe,见 §6 重写)。

## 3. KV cache 传输机制

| Connector | 协议 | Ascend 支持 | 备注 |
|---|---|---|---|
| NIXL | RDMA(IB / RoCE via UCX)/ TCP / NVMe-oF / S3 | 间接(无 native NPU 后端)[unverified] | NVIDIA 主推,GTC 2025 开源 [B] |
| Mooncake | TCP / RDMA / **AscendDirectTransport** | **原生** | kvcache-ai 项目,vllm-ascend 默认推荐 [A] |
| LLMDataDist | 华为 NPU 间通信(HCCL 之上) | **原生** | vllm-ascend 自带,1P2D 样例使用 [A] |
| LMCache | NIXL 之上 + 持久化层 | 间接 | 企业级 KV 多级缓存 [B] |
| P2pNcclConnector | NCCL | 否(NCCL 无 NPU) | 仅 NVIDIA |

- **节点内**:HCCS(910B 节点内 NPU 互联,带宽参数 [待核实 — 不要凭记忆])
- **节点间**:RoCE / RDMA via Mooncake AscendDirectTransport [A · vllm-ascend tutorial]
- **性能样例**:Mooncake KV pool 在 vLLM(NVIDIA 侧)agentic trace 上声称 *3.8× throughput / 46× lower TTFT / 8.6× lower end-to-end* [B · vllm.ai blog 2026-05-06];Ascend 侧的等价数字目前 vllm-ascend 文档未公开 [unverified]。

## 4. PD 真实收益

- **PyTorch + vLLM 联合 blog**:Llama4 Maverick / H100 1P1D 下,同 QPS TTIT 曲线更平滑;具体数字未在公开摘要中列出 [B · pytorch.org/blog]。
- **AMD MORI-IO 案例(Qwen3-235B-A22B-FP8,2026)**:8 GPU、8 req/s、2k 输入 / 1k 输出,disaggregated 比 colocated **2.5× goodput**,ITL 稳定 [B · vllm-project blog 2026-04-07]。
- **llm-d v0.5 (2026-02)**:B200 上声称 ~3.1k tok/s/GPU decode + "Up to 70% higher tokens/sec with PD vs standard vLLM" [B · llm-d.ai release notes]。
- **8B class 直接数据**:morphllm 2026 报告"8B model on H100 sub-80ms TTFT, ITL 11–21ms" [B] — 但**未明确是否启用 PD**,作为 colocated baseline 解读更安全 [D · 推导]。Qwen 3-8B / Ascend 910B 的 PD A/B 数据,公开渠道暂无 [unverified]。
- **适用规模**:disaggregation 的甜区是 prefill-bound 的大 prompt(>2k token)或高 concurrency 场景;8B 模型在短 prompt(<512 token)+ 低并发 (<8 QPS) 下,Mooncake 团队自己也承认收益不显著 [B · vllm.ai 2026-05-06]。

## 5. llm-d 备选(已拒,见 §6.2-B;保留作历史参考)

- **里程碑**:CNCF Sandbox(2026-03-24)[A];v0.5(2026-02)hierarchical KV offloading、UCCL transport;v0.7.0(2026-05)kustomize-first guides、扩展 nightly CI(OpenShift / GKE / CoreWeave)[A · llm-d/llm-d README]。
- **创始阵营**:Red Hat + Google Cloud + IBM Research + CoreWeave + NVIDIA。生产引用:Tesla、Google、AWS、Oracle [B]。
- **加速器矩阵**:H100 / H200 / B200、MI300X、Intel XPU、Google TPU。**Ascend 不在已测试列表**(2026-05 检索)[unverified — 未在 llm-d v0.7.0 公开文档中找到 Ascend 词条]。
- **K8s 优势**:LeaderWorkerSet 多节点拓扑、prefix-cache-aware 路由、SLA-based scheduler、自动 prefill/decode pool 伸缩;KServe 的 `LLMInferenceService` 直接以 llm-d 为底座 [A · kserve docs]。

## 6. vllm-ascend 集成方案(去 KServe)

**决策**:经 ADR-0002 评审,从项目技术栈基线移除 KServe(根因:spec/OR-requirements.md 第 47-48 行甲方明确要求)。推理服务全部基于 vllm-ascend Deployment + 自研 inference-operator。

### 6.1 推荐集成路径

- **单实例模型**(Pi 3B / DeepSeek 20B / Qwen 14B 等):一个 `Deployment` + `Service`,容器镜像基于 `vllm-ascend v0.11.0+`,启动参数走标准 vLLM CLI(`--model` / `--tensor-parallel-size` / `--max-num-seqs` / `--kv-transfer-config`)
- **PD 分离**(Qwen 8B PD):两个独立 `Deployment`(prefill / decode),通过 vllm-ascend 内置 `disaggregated_prefill_v1` 接口 + KV transfer connector(Mooncake 节点间 / LLMDataDist 节点内)
- **PD Router**:基于 vllm-ascend 官方 `disaggregated_prefill_v1/proxy_server.py` 改造为 K8s `Service` + Controller,由自研 `inference-operator` 管理
- **ModelService CRD**:自研 inference-operator 提供的高阶抽象,替代原计划的 KServe `InferenceService`,封装单实例 / PD 两种拓扑差异

### 6.2 已拒方案

**(A) KServe 标准 `InferenceService` CRD**
- 只支持单节点 LLM,**不能**表达 PD pair [A · kserve docs]
- PD 拓扑必须两个独立 `InferenceService` + 自研 router → KServe 抽象价值缩水
- 与 spec 第 48 行「不用 KServe」字面冲突 → ADR-0002 拒绝

**(B) KServe `LLMInferenceService` CRD**(原本是 KServe 唯一能原生表达 PD 的路径)
- KServe 新增,purpose-built for GenAI,显式支持 prefill-decode disaggregation、多节点 LWS、TP/DP/EP 并行 [A · kserve.github.io llmisvc-overview]
- **底座是 llm-d**,而 llm-d **不支持 Ascend** [unverified — 未在 llm-d v0.7.0 文档检索到 Ascend 词条]
- 在 910B 上启用 `LLMInferenceService` 是 **未验证路径** [D · 推断]
- 双重否定(spec + 工程现实)→ ADR-0002 拒绝

**(C) MindIE Service 单栈**(走 Huawei 官方 MindIE Service,不经 vllm-ascend)
- 优点:Huawei 官方支持完整,有 Qwen1.5-14B PD 部署官方文档 [A · support.huawei.cn];RESTful API
- 缺点:与 vLLM OpenAI API 不互通;tokenizer / KV cache 格式自成体系;社区生态有限
- **作为 Fallback 保留**(见 §8 fallback 列)— 若 vllm-ascend 在 Phase 5-6 阻塞,退到此栈而**不**回 KServe

## 7. 风险 / Gotchas

1. **API churn**:vLLM 上游 `--kv-transfer-config` 字段、connector 类名在 v0.7 → v0.10 → v0.21 之间动过 schema [B];vllm-ascend 紧跟上游,v0.18 → v0.19.1rc1 跨度内已经替换 attention 算子和 ADXL 默认后端 [A]。**对策**:Phase 5 锁 `vllm-ascend v0.11.0`(已有官方 Qwen PD 教程的最低稳定版),容器镜像 digest pin 到 Helm chart。
2. **Ascend 适配延迟**:vllm-ascend 主线发布比 vLLM 上游晚 2–6 周 [D · 推导自 release 时间差]。新特性(如 fused GDN kernel)在 NVIDIA 侧先到,Ascend 侧排队。
3. **Qwen3.x 准确度问题**:vllm-ascend 已知 Qwen3.x 在 PD 场景下有 accuracy issues(release notes 提及),需要 A/B 验证 [A]。
4. **双角色 NUMA/HCCS 亲和(Phase 6)**:prefill 节点偏重 compute,decode 节点偏重 HBM 带宽 + 长存活 KV;NUMA bind 策略需要分两套 profile,不能共用一份 device-plugin 配置。Phase 6 的 HCCS 亲和必须把 prefill→decode 的 KV 传输 path 当成一类拓扑约束(同 super-node 内、跨 super-node 用 RoCE)。
5. **vllm-ascend 单栈,不与 MindIE Service 并存**:KV cache 格式、tokenizer、Sampler 不互通,选定一栈后不再混用。但 **MindIE Turbo 作为 vllm-ascend 内置加速 backend 不冲突**——同栈调用,见 §2 [B · vllm-ascend release notes]。

## 8. Phase 5 推荐

| 维度 | 主路径 | Fallback |
|---|---|---|
| Runtime | **`vllm-ascend v0.11.0`**(锁 tag) | MindIE Service(若 vllm-ascend 阻塞;**不**回 KServe) |
| KV transfer | **Mooncake + AscendDirectTransport**(节点间)、LLMDataDist(节点内备选) | TCP(仅冒烟) |
| K8s 表达 | 两个独立 `Deployment`(prefill / decode)+ 自研 PD Router `Service` + ModelService CRD(inference-operator) | 单实例 `Deployment`(colocated) |
| 拓扑 | **1P1D** 起步,Qwen 8B 实测后再决定 2P1D 或 1P2D | Colocated 单实例 |
| 调度 | Phase 6 NUMA/HCCS 亲和注入 | 仅 NUMA bind,不做 HCCS 拓扑 |

**何时切换到 llm-d**:
- llm-d 官方公布 Ascend 后端(检索 llm-d 0.8+ release notes),并通过 vllm-ascend runtime 验证
- 项目自研的 prefill/decode router Operator 维护成本超过 1 FTE-周/月

**何时退回 colocated(放弃 PD)**:
- Qwen3-8B / 1P1D / 真实 trace 下 TTFT 提升 < 30%(本判断阈值为团队约定 [D]),或
- vllm-ascend 在 Phase 5 锁定窗口(2026-06 起 6 周)内发布破坏 KV connector 协议的 minor release

**commit 锁定建议**:
- **runtime**:`vllm-project/vllm-ascend@v0.11.0` 容器镜像 SHA256 digest(从官方 ghcr.io 拉,本地 mirror 仓固化)
- **router**:自研,基于 vllm-ascend 官方 `disaggregated_prefill_v1/proxy_server.py` 改造为 K8s Service + Operator
- **不**直接锁 vllm 上游 commit — 通过 vllm-ascend 间接固化

## 9. Sources

- [vLLM repo](https://github.com/vllm-project/vllm) — accessed 2026-05-17
- [vLLM Releases (v0.21.0, 2026-05-15)](https://github.com/vllm-project/vllm/releases) — accessed 2026-05-17
- [vLLM Disaggregated Prefilling docs](https://docs.vllm.ai/en/latest/features/disagg_prefill/) — accessed 2026-05-17
- [vLLM KV Transfer & Disaggregated Serving (DeepWiki §9.4)](https://deepwiki.com/vllm-project/vllm/9.4-kv-cache-transfer-and-disaggregated-serving) — accessed 2026-05-17
- [vLLM RFC #10818 disagg roadmap](https://github.com/vllm-project/vllm/issues/10818) — accessed 2026-05-17
- [vllm-ascend repo](https://github.com/vllm-project/vllm-ascend) — accessed 2026-05-17
- [vllm-ascend Releases](https://github.com/vllm-project/vllm-ascend/releases) — accessed 2026-05-17
- [vllm-ascend release notes (main)](https://docs.vllm.ai/projects/ascend/en/main/user_guide/release_notes.html) — accessed 2026-05-17
- [vllm-ascend P/D RFC #841](https://github.com/vllm-project/vllm-ascend/issues/841) — accessed 2026-05-17
- [vllm-ascend Mooncake connector deployment guide](https://github.com/vllm-project/vllm-ascend/blob/main/examples/disaggregated_prefill_v1/mooncake_connector_deployment_guide.md) — accessed 2026-05-17
- [vllm-ascend PD Qwen Mooncake tutorial v0.11.0](https://docs.vllm.ai/projects/ascend/en/v0.11.0/tutorials/multi_node_pd_disaggregation_mooncake.html) — accessed 2026-05-17
- [vllm-ascend PD Qwen LLMDataDist tutorial v0.11.0](https://docs.vllm.ai/projects/ascend/en/v0.11.0/tutorials/multi_node_pd_disaggregation_llmdatadist.html) — accessed 2026-05-17
- [llm-d repo](https://github.com/llm-d/llm-d) — accessed 2026-05-17
- [llm-d homepage / v0.7.0 release](https://llm-d.ai/) — accessed 2026-05-17
- [llm-d on K8s deployment guide 2026](https://www.spheron.network/blog/llm-d-kubernetes-disaggregated-inference-guide/) — accessed 2026-05-17
- [KServe LLMInferenceService overview](https://kserve.github.io/website/docs/model-serving/generative-inference/llmisvc/llmisvc-overview) — accessed 2026-05-17
- [KServe x llm-d blog (2026-03-05)](https://kserve.github.io/website/blog/cloud-native-ai-inference-kserve-llm-d) — accessed 2026-05-17
- [Mooncake Store + vLLM agentic blog (2026-05-06)](https://vllm.ai/blog/2026-05-06-mooncake-store) — accessed 2026-05-17
- [MORI-IO PD blog (2026-04-07)](https://github.com/vllm-project/vllm-project.github.io/blob/main/_posts/2026-04-07-moriio-kv-connector.md) — accessed 2026-05-17
- [PyTorch + vLLM Disaggregated Inference at Scale](https://pytorch.org/blog/disaggregated-inference-at-scale-with-pytorch-vllm/) — accessed 2026-05-17
- [Huawei MindIE PD Qwen1.5-14B deployment doc](https://support.huawei.cn/enterprise/en/doc/EDOC1100455615/f241845d/) — accessed 2026-05-17
- [NIXL connector usage guide](https://docs.vllm.ai/en/stable/features/nixl_connector_usage/) — accessed 2026-05-17
- [vLLM benchmarks 2026 (morphllm)](https://www.morphllm.com/vllm-benchmarks) — accessed 2026-05-17
