# O-Cloud Edge Cloud Platform — 边缘云平台样机

> 基于 O-Cloud 形态的边缘云平台样机，具备**异构算力（昇腾 910B）基础设施管理**与 **AI 推理服务编排部署**两大能力。

**当前阶段**：Phase 0（架构设计与协作协议）— **完成**
**下一阶段**：Phase 1（核心样机 + Mock 数据）— **即将启动**

**协作模式**：协调者 + N 个并行 AI Agent + 集成 Agent（详见 `docs/agent-coordination.md`）

---

## 1. 文档地图（按必读顺序）

| 顺序 | 文档 | 何时读 |
|---|---|---|
| 1 | `README.md`（本文件） | 第一份 |
| 2 | `CLAUDE.md` | 所有 Agent 启动会话必读 |
| 3 | `docs/architecture.md` | 改设计前必读 |
| 4 | `docs/phase1-plan.md` | 想知道任务全貌时读 |
| 5 | `docs/agent-coordination.md` | **★ Agent 必读**，协作协议 |
| 6 | `<module>/CLAUDE.md` | 在自己模块工作时必读 |
| 7 | `docs/api-contract.yaml` | 改 API 前必读 |
| 8 | `configs/mock-data/schema.json` | 改 Mock 数据前必读 |
| 9 | `docs/adr/0001-phase0-key-decisions.md` | 想了解为什么这样设计 |

---

## 2. 仓库结构

```
ocloud-phase0/
├── README.md                           本文件
├── CLAUDE.md                           根级协作指南（必读）
├── Makefile                            顶层快捷命令
├── .gitignore
├── .github/workflows/ci.yml            CI 框架
│
├── docs/
│   ├── architecture.md                 ★ 架构设计（14 章）
│   ├── phase1-plan.md                  ★ Phase 1 任务包总表（31 个）
│   ├── agent-coordination.md           ★ Agent 协作协议
│   ├── api-contract.yaml               ★ OpenAPI 契约（共享）
│   ├── adr/                            架构决策记录
│   │   └── 0001-phase0-key-decisions.md
│   ├── tasks/                          Phase 1 任务包（W1 启动时创建）
│   └── research/                       调研（Phase 1 P1-T-012 产出）
│
├── backend/                            演示后端（Go）
│   └── CLAUDE.md                       后端模块指南
│
├── frontend/                           演示前端（React）
│   └── CLAUDE.md                       前端模块指南
│
├── operators/                          Kubebuilder CRDs
│   ├── CLAUDE.md                       Operators 模块指南
│   ├── pool-operator/                  4 级池化 CRD（Phase 1: 类型定义）
│   └── inference-operator/             推理服务 CRD（Phase 5+）
│
├── configs/                            Mock 数据与配置
│   ├── CLAUDE.md                       Configs 模块指南
│   └── mock-data/
│       └── schema.json                 ★ Mock 数据 JSON Schema（共享）
│
├── deploy/                             部署与 DevOps
│   ├── CLAUDE.md                       Deploy 模块指南
│   ├── dev/                            docker-compose 开发栈
│   ├── single-node/                    K3s + KubeEdge 单节点
│   ├── grafana-dashboards/             Grafana JSON
│   └── helm-charts/                    Helm chart
│
├── scripts/                            一键脚本（install.sh 等）
├── hack/                               开发辅助
└── tests/e2e/                          Playwright
```

---

## 3. 关键约束

### 硬件
- **昇腾 910B**（amd64 / x86_64）
- **不支持** ARM / 鲲鹏

### 形态
- 边缘单节点
- 小集群（3-5 节点）
- 多站点（Phase 9+）

### 技术栈
- 后端：Go 1.22+ / Gin / client-go / Kubebuilder
- 前端：React 18 + TS 5 + Vite / AntD 5 / G6 / Zustand / react-query
- K8s：1.31+ / KubeEdge / K3s / Karmada / Volcano
- NPU：Ascend Device Plugin + **自研 NPU DRA Driver**
- 监控：Prometheus + Grafana（iframe 嵌入）

详见 `CLAUDE.md §5` 和 `docs/adr/0001-phase0-key-decisions.md`。

---

## 4. 快速开始

### 4.1 给协调者（Phase 1 启动者）

```bash
git clone <repo>
cd ocloud-phase0

# 1. 评审 Phase 0 文档
ls docs/

# 2. 评审通过后打 baseline tag
git tag phase-0-baseline
git push --tags

# 3. 创建 dev 分支
git checkout -b dev
git push origin dev

# 4. 按 docs/phase1-plan.md §6 启动顺序分发 W1 任务包
#    第一步派 P1-T-001 给一个 deploy agent
```

### 4.2 给 Module Agent（任务执行者）

```
启动新会话时：
1. 读 README.md（本文件）
2. 读 CLAUDE.md
3. 读 docs/agent-coordination.md
4. 读 <你的模块>/CLAUDE.md
5. 读分配给你的任务包
6. git fetch && git status
7. 检查依赖：任务包 Depends on 列表已 merge 到 dev
8. git checkout -b <type>/p1-t-<id>-<desc>
9. 开干
10. 完成后开 PR（PR description 用 docs/agent-coordination.md §5.3 模板）
```

### 4.3 给集成 Agent

```
1. 监听 dev 分支
2. 每次 merge 后跑 E2E
3. 失败 → 开 issue 指给嫌疑模块 + at 协调者
4. 不主动改业务代码
```

---

## 5. Phase 0 交付清单

- [x] 架构设计（`docs/architecture.md`，14 章 + 2 附录）
- [x] OpenAPI 契约（`docs/api-contract.yaml`，覆盖所有 endpoint）
- [x] CRD 字段草案（架构 §6.3，操作 `operators/pool-operator/` 由 P1-T-003 落实）
- [x] Mock 数据 Schema（`configs/mock-data/schema.json`）
- [x] Phase 1 任务包总表（`docs/phase1-plan.md`，31 个任务）
- [x] Agent 协作协议（`docs/agent-coordination.md`）
- [x] 根级协作指南（`CLAUDE.md`）
- [x] 5 个模块级 CLAUDE.md（backend / frontend / operators / configs / deploy）
- [x] CI 框架（`.github/workflows/ci.yml`，待 P1-T-007 填）
- [x] 根 Makefile / .gitignore
- [x] ADR-0001（关键决策记录）

---

## 6. Phase 1 概览

**周期**：4 周

**任务包**：31 个，分布：
- W1（Foundation）：12 个 — 仓库骨架、契约冻结、脚手架
- W2（Topology E2E）：10 个 — 后端 + 前端拓扑页打通
- W3（Workload + Deploy + Metrics）：9 个 — 五页全联通
- W4（Logs + Polish）：7 个 — install.sh / E2E / 文档

**DoD**：五页全演示 + 数据源配置切换 + install.sh 一键部署 + E2E 通过 + 演示视频。

详见 `docs/phase1-plan.md`。

---

## 7. 路线图

| 里程碑 | 内容 | Phase |
|---|---|---|
| **M1** 可演示原型 | 演示后端 + 前端 + Mock + 真实 K8s + Prom | Phase 1-2 |
| **M2** 池化与发现 | 4 级 Pool CRD + NPU 自动发现 + DRA driver | Phase 3-4 |
| **M3** 服务编排 | 预置应用真实部署 + NUMA/HCCS 亲和 + 动态切分 + 垂直伸缩 | Phase 5-8 |
| **M4** 工程化对外 | O2 DMS 接口 + 真实硬件 + 多站点 Karmada | Phase 9-10 |

总周期估计 6-9 个月。

---

## 8. 当前未决事项

| # | 事项 | 等待方 |
|---|---|---|
| 1 | 真机 910B 的 HCCS 拓扑详情 | 用户提供（影响 Mock 数据真实度 + Phase 6 调度插件） |
| 2 | Phase 0 评审签字 + tag phase-0-baseline | 协调者 |
| 3 | Phase 1 W1 启动派单（从 P1-T-001 开始） | 协调者 |

---

## 9. 联系与反馈

- 架构问题 / RFC：在仓库开 issue，label `rfc`
- 任务包不清晰：开 issue at 协调者
- Bug：开 issue label `bug` + 模块 label

---

**License**:Apache License 2.0 — 见 `LICENSE` 文件
**Copyright**:2026 ai-edge contributors
