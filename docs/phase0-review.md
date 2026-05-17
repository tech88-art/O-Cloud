# Phase 0 评审记录

> **评审时间**:2026-05-17
> **执行模型**:single main agent + 3 ephemeral Explore subagent + 用户(coordinator)
> **评审范围**:Claude(Chat)提供的 Phase 0 全套交付物
> **输出**:本文档 + 6 项 must-fix 已应用 + 7 项 should-fix 在文档加 TODO 标注 + 6 项 flag-to-phase 录入 §13 roadmap

---

## 1. 评审方法

3 个 Explore subagent 并行评审 + main agent 同步审 + 综合:

| Subagent | 评审角度 | 阅读文件 |
|---|---|---|
| A | 架构 & 技术选型 & 2026-05 生态状态 | `docs/architecture.md`, `docs/adr/0001-phase0-key-decisions.md`, `operators/CLAUDE.md` |
| B | Phase 1 任务包经济学 & 单 agent 现实 | `docs/phase1-plan.md`, `docs/agent-coordination.md`, `README.md` |
| C | 协作协议 vs 单 agent 现实 | `docs/agent-coordination.md`, `CLAUDE.md`, 5 个 module CLAUDE.md, `.github/workflows/ci.yml` |

Main agent 同步审:5 个 module CLAUDE.md / Makefile / ci.yml / README §8 未决事项 / 共享契约一致性。

---

## 2. 总评

**设计 80% 扎实**。模块边界、共享契约管控、CI gate 设计、Mock 真实度约定都是顶尖水平。

主要 gap:
- 协议为多 agent 并行设计,需为 **单 main agent + ephemeral subagents** 现实瘦身
- 部分技术决策(K8s DRA / Volcano / CRD scope)未跟上 2026-05 生态
- 部分 deliverable(演示视频)AI agent 能力外
- 部分任务粒度需调(T108 = 2d 过粗)

---

## 3. MUST-FIX(baseline 前已应用)

### 3.1 协作协议为单 agent 现实瘦身

**问题**:`docs/agent-coordination.md` 设计了 "Coordinator + N persistent Module Agent + Integration Agent + Review Agent"。实际只有 1 个 main agent + ephemeral subagents。

**应用**:
- `docs/agent-coordination.md` 加 §0a "单 agent 执行模型(operative)",涵盖 subagent 派生条件 / 角色绑定 / 失败处理 / RFC 简化(chat+ADR)/ CI auto E2E(替代 Integration Agent)/ PR approval / 看板跳过
- 更新 §0 角色定义表,加 "实际承担" 列
- §4 RFC mermaid 加 cross-ref §0a.5
- §8 集成 Agent 改为 "CI auto E2E"
- §11 status board 标 operative skip
- `CLAUDE.md` 顶部加三层规则架构说明 + 协作模型 update;§11 协调者约束适配单 agent

### 3.2 Phase 1 任务包微调

**问题**(by Subagent B):
- T108(前端 Overview 拓扑页, 2d)过粗
- T108→T009 假依赖(可用 stub 并行)
- T002/T003/T004 "冻结" 措辞过强(D1-D2 难真冻结)
- mermaid 图缺 T008→T010 边
- AC 模糊(T002/T004/T009/T108)
- T001 缺 LICENSE AC

**应用**(`docs/phase1-plan.md`):
- T108 拆 → T108a(拓扑骨架+左树, 1d)+ T108b(详情面板+WS 同步, 1d)
- T108→T009 改 `-.advisory.->` (mermaid)
- T002 / T004 改 "sealed-for-review"(可经 RFC 修订,非绝对冻结)
- mermaid 加 T008 → T010 边
- AC 机器可验证化(T002 加 swagger-cli + redoc-cli + 响应示例完备性 + 字段预对齐;T004 加 ajv compile + happy-path 样例 validation;T009 加 side-by-side 对比表 6 维度;T108a/b 各加具体可测点)
- T001 加 LICENSE = Apache 2.0 AC + .gitignore Go/Node/IDE
- T110 deps 改为 T108b

### 3.3 演示视频 deliverable

**问题**(by Subagent B):AI agent 无屏幕录制能力,DoD 列 "演示视频" 不可执行。

**应用**:`docs/phase1-plan.md §9` DoD Must Have 项改为 "Agent 产 `demo-script.md` + 关键路径 `screenshots/`,视频录制由用户负责"。

### 3.4 AC 模糊处硬化

**问题**(by Subagent B):
- T002 "协调者评审签字冻结" — fuzzy
- T004 "协调者签字冻结" — fuzzy
- T009 "协调者拍板(推荐 G6)" — human decision,缺过程要求
- T108 "三种状态" — 描述性而非检测性

**应用**:见 §3.2 上面 AC 改写。

### 3.5 LICENSE 选定

**问题**:README §9 "License: 待定"。

**应用**:T001 Acceptance Criteria 加 "`LICENSE` 文件存在,内容为 Apache 2.0(K8s 生态主流)"。

### 3.6 CRD scope 多租户警告

**问题**(by Subagent A):
- `NPUSlicePool: Namespaced` + 父池 `Cluster` → 多 namespace 可对同一物理 NPU 定义切片,无 RBAC/Admission 强制隔离
- §6.6 DRA 标准对象映射只一行,Phase 4 integration chaos 风险
- Pool ↔ Pod 间缺 NPUSliceAllocation / Quota 抽象

**应用**(spec 不动 baseline,但加显式 warning + Phase 入口检查):
- `docs/architecture.md` §6.6 加 DRA mapping TODO(Phase 4 启动前补 §6.7 表)
- §6.7 新建 "Multi-tenancy 与 scope 警告":Phase 1-2 隔离 by convention(`ocloud-system` ns),Phase 3 加 ValidatingAdmissionPolicy,Phase 9 完整 multi-tenancy
- §6.8 新建 "Allocation / Quota 模型占位":Phase 5 启动前补
- `operators/CLAUDE.md` §4 CRD relationship 表后加 ⚠️ warning block

---

## 4. SHOULD-FIX(已在文档加 TODO 标注,Phase 1 W1-W2 处理或 phase 入口)

| # | 项 | 文档落点 | 处理 phase |
|---|---|---|---|
| 1 | K8s DRA Beta vs GA(K8s 1.31 DRA 是 Beta,非 GA) | `docs/adr/0001-...md §5` 加 "2026-05-17 修订" 段;`docs/architecture.md §3.4` 加 TODO note | Phase 4 启动前 |
| 2 | CRD Strategy union 强类型化(stringly-typed → polymorphic / CEL) | `docs/architecture.md §6.7` 占位 + 待 ADR-0002 | Phase 3 启动前 |
| 3 | CRD Allocation / Quota 模型缺口 | `docs/architecture.md §6.8` 占位 | Phase 5 启动前 |
| 4 | DRA standard objects 映射详表 | `docs/architecture.md §6.6` 加 TODO | Phase 4 启动前 |
| 5 | Volcano vs Kueue 2026 复审 | `docs/architecture.md §3.3` 加 TODO | Phase 6 启动前;P1-T-012 加子任务 |
| 6 | Grafana iframe SSO/auth AC | P1-T-010 AC hardening(W1 任务中) | Phase 2 前 |
| 7 | KubeEdge + K3s 拓扑明确(职责重叠) | `docs/architecture.md §3.3` 加 TODO,§9.1 待补图 | Phase 2 启动前 |

---

## 5. FLAG-to-Phase(`docs/architecture.md §13` 已录入)

各 Phase 启动前作为 entry gate 检查:

| Phase | flag | 检查动作 |
|---|---|---|
| Phase 4 | DRA mapping 详表 | 补 `NPUSlicePool ↔ ResourceSlice/ResourceClaim/DeviceClass` 字段映射表 |
| Phase 5 | `NPUSliceAllocation` / `Quota` 对象设计 | 补 CRD 设计 + inference-operator 集成方案 |
| Phase 7 | 动态切分若 fallback 多模板组合 | ADR 明确触发 fallback 的条件 |
| Phase 9 | 多站点 demo backend 缓存重构 | ADR + 重构路径(LRU 进程内 → Redis/singleton/stateless) |
| Phase 9 | 安全模型(authn/z + multi-tenancy RBAC + NPUSlicePool admission) | 完整安全设计 + Karmada RBAC 联动 |
| Phase 5+ | NPU pod 网络考量 | CNI + HCCL RDMA/RoCE/IPoIB 兼容性调研 |

---

## 6. ACCEPT-AS-IS(无需优化)

1. 五层架构(修订 Layer 5 边界说明后)— Subagent A 确认
2. 后端 datasource interface 设计(`backend/CLAUDE.md §4.1`)— 干净
3. 前端 i18n / react-query / Zustand 约定 — 标准且一致
4. 模块边界(5 个 CLAUDE.md 互不冲突)— Subagent C 核实
5. CRD 4 级 hierarchy(scope 修订后)— 自然分解,与 O-Cloud topology 对齐
6. Mock 数据真实度约定(`configs/CLAUDE.md §3.3`)— 工业级详尽
7. CI gates 框架设计(`.github/workflows/ci.yml` + `deploy/CLAUDE.md §3.1`)— P1-T-007 待填实现,框架对
8. ADR-0001 决策 1-4, 6-9(除 #5 DRA,已修订)
9. 任务包格式 + Allowed Paths 体系 + PR template — main agent 自检 + CI 兜底,适用单 agent

---

## 7. 协议瘦身:single-agent operative model(已写入 §0a)

`docs/agent-coordination.md §0a` 新增章节,operative 规则:

```
0a.1 Main agent(1)+ ephemeral subagents(完成即退,无 persistent state)
0a.2 Subagent 派生条件:互不依赖 + Allowed Paths 互不重叠 + 单 session 完成 + ≥30% 并行收益
0a.3 Subagent 角色绑定:per task 单一模块;不能再派生;共享契约只 main agent 改
0a.4 Subagent 失败处理:异常退 → main agent 接管 + 不自动重试
0a.5 RFC 简化:chat + ADR 替代 GitHub issue 仪式
0a.6 集成 Agent → CI auto post-merge E2E + github-actions bot 自动开 issue
0a.7 PR approval:用户 final approver;高风险 PR 派 ephemeral review subagent
0a.8 GitHub Projects 看板 → skip
```

---

## 8. 三个 reviewer 一致信号(高确定度)

3 个 subagent 都点到的问题(独立得到,信号强):
- **单 agent 适配缺口**(C 主审,A/B 间接)— 已通过 §0a 解决
- **W1 共享契约 freeze 风险**(B 详,A/C 暗示)— 通过 "sealed-for-review" 措辞 + chat+ADR RFC 解决
- **Phase 1 deliverable 实现度**(B 详,A 提 "hand-waving")— 通过演示视频调整 + AC 硬化解决

---

## 9. 评审产出文件清单

| 文件 | 变更 |
|---|---|
| `docs/agent-coordination.md` | 加 §0a 单 agent 执行模型(8 个子条款);§0/§4/§8/§11 加 operative cross-ref |
| `docs/architecture.md` | §2.2 加 Layer 5 边界澄清;§3.3 加 Volcano/KubeEdge TODO;§3.4 加 DRA TODO;§6.6 加 DRA mapping TODO;§6.7 新增 multi-tenancy 警告;§6.8 新增 Allocation 占位;§13 加 flag-to-phase 6 项 |
| `docs/phase1-plan.md` | T001 加 LICENSE AC;T002/T004 改 sealed-for-review + AC 硬化;T009 加 side-by-side 对比表 AC;T108 拆 T108a+T108b;T110 deps 改 T108b;mermaid 加 T008→T010 边 + T009 改 advisory;§9 DoD 演示视频改物料 |
| `docs/adr/0001-phase0-key-decisions.md` | §5 K8s DRA 加 2026-05-17 修订段(主路径改 Ascend Device Plugin v1) |
| `operators/CLAUDE.md` | §4 CRD relationship 表后加 ⚠️ multi-tenancy warning |
| `CLAUDE.md` (根) | 顶部加三层规则架构;§11 协调者约束适配单 agent |
| `.claude/skills/grill-me/SKILL.md` | reframe RAN → O-Cloud(module 消歧 + 域术语 NPU/Pool/Slice/PD/O2 + Allowed Paths 检查) |
| `.claude/skills/caveman/SKILL.md` | RAN 例换 O-Cloud 例 |
| `.claude/skills/task-package/SKILL.md` | **新建** — 任务包执行纪律 skill |
| `docs/phase0-review.md` | **本文件** |

---

## 10. 后续动作

1. **本评审记录(本文件)** + 全部 must-fix 已应用 → 提交为 phase-0-baseline tag
2. 在每个 Phase 入口跑一次 §5 flag-to-phase 检查
3. should-fix 项在对应 Phase 启动 W1 前转化为任务包

---

**END of Phase 0 Review**
