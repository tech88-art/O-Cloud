# ADR-0002: 从技术栈基线移除 KServe

- **状态**：Proposed
- **日期**：2026-05-17
- **决策者**：协调者（待批准）
- **相关**：
  - `spec/OR-requirements.md` 第 47-48 行（甲方原始要求）
  - `docs/architecture.md` §3.4 推理框架选型
  - `CLAUDE.md` §5 技术栈基线 + 重大决策
  - `docs/research/vllm-pd-disaggregation.md` §6 KServe 集成可行性分析
  - `ADR-0001` Phase 0 关键架构决策（推理框架基线由本 ADR 修订）

---

## 上下文

2026-05-17 项目根目录新增 `spec/OR需求`（甲方对 O-Cloud 边缘云样机的原始需求文档）。文档第 47-48 行明确：

> 备注：
> 1、推理服务框架采用 MindIE+vLLM，**不用 KServe**

而项目现有文档与该要求**直接冲突**：

- `docs/architecture.md` 9 处把 `KServe + MindIE Backend` 列为推理框架基线（line 75 / 119 / 212 / 262 / 273 / 370 / 684 / 806 / 854）
- `CLAUDE.md:103` 技术栈基线同样写 `KServe + MindIE / vLLM 或 llm-d`
- `docs/research/vllm-pd-disaggregation.md` §6 早已识别 KServe 在 Ascend 路径的可用性问题，但主设计文档尚未跟随修正

进一步分析 KServe 在 910B Ascend 路径的实际可用性：

1. **KServe `LLMInferenceService` CRD 不可用**：底座是 llm-d，而 llm-d 在 2026-05 公开文档中**不支持 Ascend**（[unverified — 未在 llm-d v0.7.0 文档检索到 Ascend 词条]，见 `vllm-pd-disaggregation.md` §5）。
2. **KServe 标准 `InferenceService` 不能表达 PD pair**：PD 拓扑只能靠"两个独立 InferenceService + 自研 router 手工拼装"，KServe 提供的 LWS / 亲和调度便利完全失效。
3. **KServe 抽象价值缩水**：在 Ascend 路径上既无法用 `LLMInferenceService`，又无法直接表达 PD pair，自研 router Operator 必不可少 → KServe 仅剩"标准 CRD 包装"这一项价值，而 Phase 5 阶段并不需要对接 KServe 生态（模型仓库 / Explainer 等）。

综上：spec 要求 + Ascend 工程现实 + 调研结论三方一致指向移除 KServe。

---

## 决策

**从项目技术栈基线移除 KServe**。具体：

1. **推理服务全部基于 vllm-ascend Deployment**
   - 锁定 `vllm-ascend v0.11.0+`（已含 Mooncake + LLMDataDist 两条 KV transfer，Qwen 系列官方教程支持）[A · vllm-ascend docs v0.11.0]
   - 容器镜像 SHA256 digest pin 到 Helm chart（见 `vllm-pd-disaggregation.md` §8）

2. **自研 inference-operator**（Phase 5-6 实现）
   - 管理 vllm-ascend Deployment 生命周期
   - 内置 PD Router（基于 vllm-ascend `disaggregated_prefill_v1/proxy_server.py` 改造为 K8s Service + Operator）
   - 集成 NUMA / HCCS 亲和调度（Phase 6 注入）
   - 集成自动伸缩（Phase 8 + VPA / 自定义 controller）
   - 对上提供 `ModelService` 高阶 CRD（替代原本由 KServe `InferenceService` 提供的标准接口）

3. **MindIE 仍可作为 vllm-ascend 的可选加速 backend**
   - 通过 vllm-ascend `MindIE Turbo` 集成（已用于 DeepSeek-V3/R1、Qwen2 系列加速）[B · vllm-ascend release notes]
   - 不再有独立的「KServe + MindIE Service」栈

4. **单实例小模型不再有 KServe 包装层**
   - Pi 3B / DeepSeek 20B 等单实例模型也走 vllm-ascend Deployment
   - Qwen 8B PD 分离走 vllm-ascend 的 1P1D 拓扑（Phase 5 起步）

5. **Fallback 路径**
   - 若 Phase 5-6 实测 vllm-ascend 阻塞（Qwen3-8B / 1P1D 真实 trace 下 TTFT 提升 < 30%，或 vllm-ascend 在 6 周锁定窗口内发布破坏 KV connector 协议的 minor release），回退路径是「单实例 MindIE Service + 自研 router」
   - **不**回到 KServe（已识别在 Ascend 路径价值缩水）

---

## 后果（Consequences）

### 正向
- ✅ **架构精简**：5 个组件（KServe + MindIE Backend + vLLM + Inference Operator + Router）→ 3 个（vllm-ascend Deployment + inference-operator + 内置 PD Router）
- ✅ **与 spec 字面一致**：移除甲方文档与项目文档间的直接冲突
- ✅ **与调研结论一致**：`vllm-pd-disaggregation.md` §6 已识别的 KServe 在 Ascend 路径价值缩水问题得到落地
- ✅ **路径单一**：避免两栈并存（KV cache 格式 / tokenizer / Sampler 不互通的复杂度，见 `vllm-pd-disaggregation.md` §7.5）

### 负向
- ❌ **失去 KServe 标准 InferenceService CRD** → 由自研 `ModelService` CRD 替代（inference-operator 提供），需自研 schema 与 webhook 校验
- ❌ **失去 KServe 自动伸缩（Knative Serving）开箱即用能力** → 需自研 VPA + 自定义伸缩 controller（已规划 Phase 8）
- ❌ **失去 KServe 生态对接**（模型仓库、Explainer、KFServing transformer）→ Phase 5-6 不需要，Phase 9+ 若需要再 RFC

### 中性
- 🔄 **inference-operator 工程量略有增加**：但 PD Router 在保留 KServe 方案中本就需要自研，差异主要是去掉 `InferenceService` 包装层 → 净工程量增量约 0.5-1 FTE-周（Phase 5）

---

## 影响范围

### 文档层（必改 13 处，详见 `~/.claude/plans/spec-prancy-hickey.md` §3.1-§3.2）
- `docs/architecture.md`：9 处文字修改 + §3.6 关键决策追加 1 行
- `CLAUDE.md`：§5 技术栈基线 1 处 + §5 重大决策块追加 1 行
- `docs/research/vllm-pd-disaggregation.md`：§6 / §7.5 / §8 / 文件头修订记录

### 代码层
- 无影响。Phase 1 之前 backend / frontend / operators 源代码均不引用 KServe。

### 契约层
- 无影响。`docs/api-contract.yaml` 不引用 KServe；runtime enum 已含 `vllm`。

### Phase 1 任务包
- 无影响。Phase 1 是 Mock 阶段，与推理框架实现无关。

### Phase 5+ 任务包
- 未来拆出 Phase 5 任务时，按本 ADR 基线设计 inference-operator 任务；当前任务包不存在，所以不改。

---

## 已拒绝的替代方案

### 方案 A：保留 KServe 当 API 抽象层，backend 换 vllm-ascend
- 与 spec 第 48 行「不用 KServe」字面冲突
- KServe 在 Ascend 路径上的核心能力（LLMInferenceService）依赖 llm-d，llm-d 不支持 Ascend
- 标准 InferenceService 不能表达 PD pair，必须自研 router——KServe 抽象价值已被掏空

### 方案 B：暂不决，进 Phase 5 前再 RFC
- 推迟决策不解决当下文档冲突
- Phase 1-4 文档将继续以 KServe 为基线产出，未来回切成本更高
- spec 已明确，无须等待

---

## 验证

参见 `~/.claude/plans/spec-prancy-hickey.md` §5。核心检查项：

```bash
# 期望：docs/architecture.md 与 CLAUDE.md 不再出现 KServe（除已声明的历史引用）
grep -rn "KServe" docs/architecture.md CLAUDE.md
# → 0 matches

# 期望：源代码无 KServe 依赖
grep -rn "kserve\|KServe\|InferenceService" backend/ frontend/ operators/
# → 0 matches
```

---

## 修订历史

- 2026-05-17：初版 Proposed（由 spec/OR-requirements.md 引入触发）
