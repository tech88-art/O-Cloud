# CLAUDE.md — 仓库根协作指南（AI Agent 版）

> **每个 AI Agent 启动新会话时必读**。本文件是项目协作基线，覆盖项目愿景、共同约束、与各模块 CLAUDE.md 的关系。
>
> **三层规则架构**(2026-05-17 update):
> 1. **认识论层**:`~/.claude/CLAUDE.md` — 通用真值标准(P1-P7, M1-M3)
> 2. **代码工作流层**:`~/.claude/CODING.md` — 4 failure modes(misalignment / terminology / feedback / entropy)
> 3. **项目层**(本文件):O-Cloud 项目特定规则
>
> 本文件不重复上面两层,仅做项目特定扩展。
>
> **协作模型**:**single main agent + ephemeral subagents**(更新自原"协调者 + N 个并行 Module Agent + 集成 Agent")。详见 `docs/agent-coordination.md §0a`(operative)。

---

## 1. Agent 启动 Checklist（每次都要做）

```
1. 读 README.md（项目概览）
2. 读本文件 CLAUDE.md（通用规约）
3. 读自己模块的 <module>/CLAUDE.md（模块特定规则）
4. 读 docs/agent-coordination.md（协作协议）·
   特别 §0a operative + §0a.10-12（2026-05-19 加 · plan/execute
   session 分离 + strict per-task verify + push 协议)
5. 读 docs/architecture.md 相关章节（架构基线）
6. 读分配给你的任务包 docs/tasks/P{N}-T-XXX.md
7. git fetch && git status，确认本地 clean
8. 检查任务包的 Depends on 是否已 merge 到 dev
9. 从 dev 拉特性分支：git checkout -b <branch>
10. 开干
```

**禁止**：跳过任一步直接编码。

**项目级 memory 自动加载**（无需手动读）：`C:\Users\Himalayan\.claude\projects\D--code-ai-edge\memory\MEMORY.md`
索引指向 3 条 essence(2026-05-19 起):
- `feedback_strict_per_task_verify.md` — `docs/agent-coordination.md §0a.11` 长形式
- `feedback_plan_vs_execute_session_split.md` — `§0a.10` 长形式
- `reference_github_creds.md` — `§0a.12` 长形式

memory 是 Claude 视角持久层 · `docs/agent-coordination.md §0a.10-12` 是仓库视角持久层（PR review / 新 contributor / 跨工具均可见）· **两者同源**,memory 改了 docs 也改,反之亦然。

---

## 2. 项目快照

**O-Cloud 边缘云平台样机**，两大能力：

1. **基础设施管理（IMS）**：算力管理 / 池化 / 发现 / 异构 K8s NPU DRA
2. **服务编排部署**：NPU 动态切分 / AAL / NUMA+HCCS 亲和 / CANN+MindIE / PD 分离推理 / O2 DMS

**目标硬件**：昇腾 910B（amd64 / x86_64，**不支持** ARM / 鲲鹏）

**关键演示**：
- 概览拓扑（节点→NPU→切片）
- 工作负载
- 应用部署（auto / manual）
- 性能指标（Grafana 嵌入）
- 日志

---

## 3. 文档地图

| 文件 | 何时读 |
|---|---|
| `README.md` | 第一份读，了解项目 |
| `CLAUDE.md`（本文件） | 启动每个 agent 会话都读 |
| `docs/architecture.md` | 改设计前必读 |
| `docs/phase1-plan.md` | 任务总表，了解上下游 |
| `docs/agent-coordination.md` | **★ 协作协议**，必读 |
| `docs/api-contract.yaml` | 改后端 API 前必读 |
| `configs/mock-data/schema.json` | 改 Mock 数据前必读 |
| `<module>/CLAUDE.md` | 在自己模块工作必读 |
| `docs/adr/*.md` | 历史架构决策 |
| `docs/tasks/P1-T-XXX.md` | 你的任务定义 |

---

## 4. 模块化与路径所有权

**关键原则**：每个 Module Agent 严格绑定**单一模块**，不跨模块改动。

| 模块 | OWN 路径 | 模块文档 |
|---|---|---|
| backend | `backend/**` | `backend/CLAUDE.md` |
| frontend | `frontend/**` | `frontend/CLAUDE.md` |
| operators | `operators/**` | `operators/CLAUDE.md` |
| configs | `configs/**` | `configs/CLAUDE.md` |
| deploy | `deploy/**`, `.github/**`, `scripts/**`, `hack/**` | `deploy/CLAUDE.md` |
| docs | `docs/**`（共享契约除外） | — |

**共享契约**（任何模块都可读，修改走 RFC）：
- `docs/api-contract.yaml` — OpenAPI
- `operators/pool-operator/api/v1alpha1/*.go` — CRD Go 类型
- `configs/mock-data/schema.json` — Mock JSON Schema
- 根 `CLAUDE.md`、根 `README.md`、根 `Makefile`、`docs/architecture.md`

---

## 5. 技术栈基线（不要擅自换）

### 后端
Go 1.22+ / Gin / gorilla/websocket / client-go / controller-runtime / Viper / zap / Kubebuilder

### 前端
React 18 + TS 5 + Vite / Ant Design 5 / AntV G6 / ECharts / Zustand / react-query / react-i18next

### K8s 生态
KubeEdge / K3s / Karmada（Phase 9+）/ Volcano / scheduler-plugins（自研 NUMA+HCCS）

### NPU / AI
Ascend Device Plugin / **自研 NPU DRA Driver**（基于 kubernetes-sigs/dra-example-driver）/ **vllm-ascend** (v0.11.0+) / MindIE Turbo（vllm-ascend 可选加速 backend）/ 自研 **inference-operator**（含 PD Router + ModelService CRD）

### 监控
Prometheus + Grafana（iframe 嵌入）/ 自研 ascend-npu-exporter-plus / Loki + Promtail

**重大决策**（不可逆，记入 ADR）：
- **不引入 MindCluster / MindX DL**（保留自研动态切分与 DRA 空间）
- **演示后端无状态**（无数据库，仅聚合 + 短期缓存）
- **混合前端**（自研主壳 + Grafana iframe 指标页）
- **不引入 KServe**（spec 第 47-48 行甲方明确要求，见 ADR-0002）：推理服务全部基于 vllm-ascend Deployment + 自研 inference-operator（含 PD Router），不再有 KServe `InferenceService` 包装层

---

## 6. 跨模块禁止行为

无论哪个模块的 Agent，下列行为**严格禁止**：

| 禁止项 | 替代方案 |
|---|---|
| 直接 push 到 `main` / `dev` | 走 PR + 至少一个 approval |
| 修改共享契约（API / CRD / Schema） | 走 RFC 流程（见 agent-coordination §4） |
| 跨模块修改文件 | 开 issue 指给对应模块 |
| 引入新的核心技术栈（如换 Gin、换 React） | 提 ADR 走重大决策评审 |
| `git push --force` 到共享分支 | 永远不允许 |
| `git commit --no-verify` 跳过 hook | 永远不允许 |
| 把 secret / token / kubeconfig 写进仓库 | 用 GitHub Secrets / 部署注入 |
| 在 mock 数据里塞真实人名 / 真实公司名 | 用 fake data（gofakeit / Faker） |
| 关闭 CI 检查项 / `continue-on-error: true` | 修问题，不要绕过 |
| 提交未跑 lint / 未跑 test 的代码 | 本地必须先跑 |

---

## 7. PR 流程

1. 从 `dev` 拉分支：`<type>/p1-t-<id>-<desc>`
2. 提交按 Conventional Commits
3. 推到远端，开 PR 到 `dev`
4. PR description 用模板（见 `docs/agent-coordination.md §5.3`）
5. CI 全绿
6. 至少 1 个 approval
7. Squash merge

**禁止**：rebase / merge 到 main 分支（main 只接受 release tag）。

---

## 8. RFC 流程概要

修改共享契约必走 RFC：

```
Agent 想改契约 → 开 issue (title: RFC: xxx) → 协调者评估 → 批准/驳回
→ 协调者或指定 agent 执行变更 → 通知所有相关 agent → 各模块适配
```

详见 `docs/agent-coordination.md §4`。

---

## 9. 测试与质量

| 模块 | 测试要求 | 覆盖率目标 |
|---|---|---|
| backend | unit + handler test | 核心包 ≥ 70% |
| frontend | Vitest + RTL | ≥ 50% |
| operators | controller test（Phase 3+） | ≥ 60% |
| 全栈 | Playwright E2E（W2 起） | 主流程必过 |

**CI 强制**：
- lint 失败 → 阻止 merge
- test 失败 → 阻止 merge
- 契约不一致 → 阻止 merge
- Allowed Paths 越界 → 阻止 merge
- E2E 失败 → 不阻止 merge，但开 issue 指责

---

## 10. 异常处理

| 情况 | 行动 |
|---|---|
| 任务包描述不清 | 不猜，开 issue at 协调者 |
| 发现需要改禁改路径 | 提 RFC |
| PR 冲突在共享文件 | 提 RFC |
| 跑测试发现别人模块的 bug | 开 issue（不要替别人 fix） |
| CI 失败原因不明 | 看日志 → 开 issue → 不要无脑重跑 |
| 任务包估时严重偏差 | 完成时在 PR 反馈，让协调者修正后续 |

---

## 11. 给协调者(用户)的额外约束

> 实际执行模式下,协调者 = 用户(via chat)。Main agent 负责 subagent 派发决策(见 `docs/agent-coordination.md §0a.2-§0a.4`)。

- 任务包分发(chat 派单)前**确认**依赖 + 路径冲突
- 一次给 main agent 派一个主任务;main agent 可决定派生 subagent 并行
- **共享契约的修改只能 main agent 串行执行**(不能 subagent 改 `docs/api-contract.yaml` / `configs/mock-data/schema.json` / CRD types)
- 每周 W 末与 main agent 在 chat 同步状态盘点
- 改任务包时:chat 协商 → main agent 更新 `docs/tasks/P1-T-XXX.md` → 通知后续 subagent

---

## 12. 当前 Phase

**Phase 0**：架构设计与项目启动 — **已完成**
**Phase 1**：核心样机（演示后端 + 前端 + Mock 数据）— **即将启动**
  - 周期：4 周
  - 任务包：31 个（详见 `docs/phase1-plan.md`）

**Phase 2+** 见 `docs/architecture.md §13` 路线图。

---

## 13. 给 Agent 的友情提示

- **写代码前先读文档** —— 你不在场，不能问，只能看
- **任务包是合同** —— 在合同里干活，不越界
- **遇到歧义先开 issue** —— 猜测代价远大于澄清
- **保持单一职责** —— 不顺手"修复"其他问题（除非任务包要求）
- **PR 描述写清楚** —— 集成 Agent / 协调者会读，不要让他们猜
- **本文件、协作协议、模块 CLAUDE.md 之外不要假设任何"约定俗成"**

---

## 14. 开发日志与模块详细设计文档(2026-05-19 起 · 用户补充要求)

### 14.1 devlog 约定(per task)

**每个 task commit 必带 devlog 文件**,记录"commit message 不便携带的调试轨迹"(false starts / 错误链 / 路径偏离 / 设计决策的 why)。

落地位置:`docs/devlog/phase-<phase>-t<task-id>.md`。

每次 task commit 的 commit message 末尾(co-author 之上)加一行:
```
Devlog: docs/devlog/phase-N-tNNN.md
```

devlog 文件模板与详细约定见 `docs/devlog/README.md` §1。**每个文件 20-80 行**;不重复 commit message,只补 trail / 决策。

**Phase 4 之前已完成的 phase**:不要追溯写 devlog(Phase 1-3 已沉淀在 checkpoint 文档 + commit history,够用)。**Phase 4 是 retroactive batch**(已在 2026-05-19 phase-4-complete 后回填),**Phase 5 起严格 per-task 同步写入**。

### 14.2 模块详细设计文档约定(per module)

**每个新模块在落地"完整功能"(不仅 scaffold)时写 DESIGN.md**,放在模块根目录:
- `<module>/DESIGN.md`(operators / exporters / 其他独立模块)
- `backend/docs/<topic>.md`(backend 多主题,放在 docs/ 子目录)
- `frontend/docs/<topic>.md`(同上)

内容必含:
1. **架构概览**:模块在系统中的位置(arch §对应章节);数据流(谁 read / 谁 write / 何时触发)
2. **接口契约**:对外暴露的 API / CRD types / Go interface;每个字段语义 + 不变量
3. **生命周期**:启动 / 运行 / 关闭;Reconcile 触发 + 退出条件;依赖资源
4. **错误处理**:可恢复 vs 不可恢复;重试策略;状态机
5. **扩展点**:Phase N+ 演进路径(哪些字段预留 / 哪些组件可插拔)
6. **集成示例**:典型上游 / 下游调用示例;cross-controller awareness 模式
7. **参考**:相关 ADR / CLAUDE.md / phase plan / 代码路径

**Phase 4 新模块**(已在 2026-05-19 phase-4-complete 后回填):
- `operators/npu-dra-driver/DESIGN.md`
- `operators/inference-operator/DESIGN.md`
- `backend/docs/observability.md`

**Phase 5 起所有"完整功能"模块入 dev 分支前必带 DESIGN.md**。仅 scaffold (api/v1alpha1 types only) 可在 scaffold task commit message 注 "DESIGN.md deferred to controller-body task"。

### 14.3 验收

- PR 审阅(协调者 / 集成 agent):检查 commit footer Devlog 行 + devlog 文件存在性;新增模块检查 DESIGN.md 落地。
- Phase 入口 plan 起草(N+1 plan 文件): 草拟 checkpoint 时校验 devlog 覆盖率 = task 数(15/15 for Phase 4)。
- CI hook(Phase 5+ 候选,非阻塞):`.github/workflows/devlog-coverage.yml` 可 grep commit footer + 检查文件存在性 — 待 Phase 5 起草时定夺。

---

**END of root CLAUDE.md**
