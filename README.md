# O-Cloud Edge Cloud Platform — 边缘云平台样机

> 基于 O-Cloud 形态的边缘云平台样机，具备**异构算力（昇腾 910B）基础设施管理**与 **AI 推理服务编排部署**两大能力。

**当前阶段**:**Phase 13 complete — M7 真生产化收口(项目收官 · final milestone)**(2026-06-03 · tag `phase-13-complete` · 16/16 task · 承 Phase 12 M6)。**real(生产)版软件层完整** —— 填平 real 版剩余所有 stub/mock/ErrNotImplemented:**Bucket B 真硬件 body**(real-Ascend Source npu-smi/DCMI + 真 telemetry simulator-off + 真拓扑聚合 `GetTopology` + 真 `Deploy()` + 真 CANN/vllm-ascend PD 推理 · lab-gating 翻转 supersede ADR-0011 §3)+ **Bucket A 生产硬化**(OIDC/TokenReview validator + Dex 参考 IdP + 最小权限 RBAC + Vault/external-secrets + ClusterQuota webhook B 真强制 + 跨集群 usage + Karmada control-plane HA + vLLM PD P99 SLA harness)。demo/real 隔离闭合(隔离活在 `Source` 接口缝 · ADR-0024 §2 Decision G)· build-doc §5 生产硬化清单全 land。**验证分工**:功能由 demo + CI fixtures 持续验 · 真机验「对接」(harness `tests/e2e/real/` + `tests/sla` + Release Images arm64 push 就位 · **真机实测 lab-gated**)。**不规划 Phase 14**;4 active carry 移出项目核心交付物(信号触发可选扩展)。3 entry ADR(0023-0025)。详 `docs/checkpoint-phase13.md`。

**上一阶段**:**Phase 12 complete — M6 平台真实化 + 操作台一体化**(2026-06-01 · tag `phase-12-complete` · 16/16 task · 承 Phase 11 M5)。**3 track 全 land**:① **平台真实化** — aarch64 鲲鹏(Kunpeng 920)+ 昇腾 910B + openEuler 真实目标平台(**supersede ADR-0001 §13 amd64-only** · 华为 Atlas 800 原生配置)· 10 Dockerfile multi-arch buildx(`linux/amd64,linux/arm64`)+ CI arm64 cross-compile matrix + helm `nodeAffinity arch=arm64`(soft)+ install.sh openEuler dnf · amd64 保留本机 dev/CI/render-verify · 真鲲鹏运行验证 lab-gated 留 Phase 13+;② **拓扑数据全保真** — NPU PCIE 带宽 + node↔node network 绿边 + npu↔npu HCCS 边 + 非 NPU workload→node runs-on 边 + 带宽 hover tooltip(ADR-0021);③ **前端 one-page workspace** — 5 路由 + AntSider nav 收敛为单页操作台(AntD Splitter 可隐藏可拖拽 + 拓扑全保真渲染 + focus/isolate + 右栏 selection-dispatch 吸收 workloads/deploy/metrics/logs 四页指标/日志 + 顶栏 preset bar)。前端 80 unit + 9 e2e(one-page flow)+ 11 render-verify 截图 · 4 entry ADR(0019-0022)。production-hardening cohort 顺延 Phase 13+。详 `docs/checkpoint-phase12.md`。

**上一阶段**:**Phase 11 complete — M5 真生产化 foundation subset**(2026-05-22 · tag `phase-11-complete`)· 6 chart packaging spine 闭环 + Karmada propagation 第一波 + Frontend Workload 3 indicators + O2 DMS authn wiring · 20/20 · 详 `docs/checkpoint-phase11.md`。

**上一阶段**:Phase 9 complete — M4 工程化对外 milestone · O2 DMS Adapter + Multi-tenant Quota + PromQL ext + IMS 3 scaffold + demo-backend cache spike · 16/16 statuses · tag `phase-9-complete`。Phase 8 complete — Busy-idle 垂直伸缩 + NPUVerticalScaler CRD + 8-step Reconcile · 15/15 · tag `phase-8-complete`。
**早期阶段**:Phase 1(核心样机 + Mock 数据)44/44 · Phase 2(真实数据源切入)15/15 · Phase 3(pool controllers + ascend-npu-exporter-plus + ADR-0008 PD Router design)15/15 · Phase 4(npu-dra-driver scaffold + ADR-0001 v3 + CANN matrix)15/15 · Phase 5(real claim allocation + NPUSliceAllocation CRD + inference-operator controller body + PD Router mutating webhook + cert-manager wiring)15/15 · Phase 6(HCCS/NUMA-aware scheduler-plugin + pool-operator HCCS topology + inference-operator metrics + vllm-ascend PD proxy_server)15/15 · Phase 7(NPU 动态切分 + Source 接口 + lab gating · ADR-0011)15/15 · 全部完成
**下一阶段**:**无核心 phase — 项目已收官**(M7 = final milestone · `phase-13-complete` · per ADR-0023 §2 Decision B · **不规划 Phase 14**)。剩余仅 **信号触发的可选扩展**(不 gating 收口 · 不构成后续 phase):Volcano gang-scheduling(训练 demo signal)· Partitionable Devices KEP-4815(GA + K8s 1.36+)· Go v1 ResourceSlice migration(lab K8s ≥1.36 · v1beta1 shim 仍 work)· Frontend G6 5.x 重写(ReactFlow 渲染瓶颈)· 真机实测 stamp(lab 接入跑 `tests/e2e/real/`)· 甲方 swap point(OIDC IdP / P99 SLO / Vault backend)。详 `docs/phase12-candidate-streams.md` + `docs/checkpoint-phase13.md` §6-§7。

**上一阶段**:**Phase 10 complete — M4 工程化对外 milestone CLOSER**(2026-05-21 · tag `phase-10-complete`)· 20 task chain T001-T204。3 三件套(K8s 1.34 baseline + framework migration + NumaAffinity wrap)· 3 IMS controller body 全 land · ADR forward note polish(T103 authn substrate + T104 token-bucket rate algorithm + T106 ProxyImage chart flip)· 3 docs ADRs(ADR-0015/0016 + ADR-0017 entry decisions · ADR-0018 Karmada topology)· 3 deferred outcomes(T102 5th carry + T108 2nd defer + T202 defer)· 详 `docs/checkpoint-phase10.md` §6 Phase 11+ handoff brief。

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
- **昇腾 910B** NPU + **aarch64 鲲鹏（Kunpeng 920）host + openEuler 节点 OS**（华为 Atlas 800 原生配置 · Phase 12 ADR-0020 翻转 ADR-0001 §13 amd64-only）
- 镜像 multi-arch buildx（`linux/amd64,linux/arm64`）· **arm64 = 部署 target** · amd64 保留本机 dev/CI/render-verify
- 真鲲鹏 + 昇腾集群运行验证 lab-gated 留 Phase 13+

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

### 4.1 一键部署(干净 Ubuntu 22.04)

```bash
git clone <repo> && cd ai-edge
./scripts/install.sh
```

完成后:
- 前端 http://localhost:3000
- 后端 http://localhost:8080/api/v1/healthz
- Grafana http://localhost:3001(admin/admin)

详见 [`deploy/single-node/README.md`](deploy/single-node/README.md)。

### 4.2 本地开发(已装 Go 1.22+ / pnpm 9+)

```bash
# Backend
cd backend && make build && ./bin/demo-backend.exe -c configs/config.dev.yaml

# Frontend(另一终端)
cd frontend && pnpm install && pnpm dev --host 0.0.0.0

# 浏览器开 http://localhost:3000
```

### 4.3 数据源切换(零重编译)

`backend/configs/config.dev.yaml` 中 `datasources.mock.path` 改为不同 fixture 目录,重启 backend 即生效。详见 [`backend/configs/config.example.yaml`](backend/configs/config.example.yaml) 头部"DATASET SWAP CONTRACT"。

### 4.4 演示流程

跟 [`docs/demo.md`](docs/demo.md) 走 **one-page workspace**(单页操作台:左资源树 → 中拓扑 focus → 右栏资源/负载指标+日志 → 顶栏 preset 部署 · Phase 12 吸收原 5 页)+ D6 NUMA+HCCS 亲和对比演示。

### 4.5 给 Agent(新加入的 AI 协作者)

```
启动新会话时:
1. 读 README.md(本文件)
2. 读 CLAUDE.md(根级协作指南)
3. 读 docs/agent-coordination.md §0a(operative 模型)
4. 读 <你的模块>/CLAUDE.md
5. 读分配给你的任务包
6. git fetch && git status
7. 检查依赖:任务包 Depends on 列表已 merge 到 dev
8. git checkout -b <type>/p1-t-<id>-<desc>
9. 开干
10. 完成后 squash merge 到 dev(本仓库 local-only,无 remote PR)
```

---

## 5. Phase 1 交付状态

| Week | 任务数 | 完成 | tag |
|---|---|---|---|
| W1 Foundation | 13 | ✓ | `w1-complete` |
| W2 Topology E2E | 11 | ✓ | `w2-complete` |
| W3 Workload + Deploy + Metrics + Logs | 13 | ✓ | `w3-complete` |
| W4 Logs + Polish | 7 | ✓ | `phase-1-complete` |
| **总** | **44** | **44 ✓** | |

**DoD 验收**(phase1-plan.md §9):
- [x] 五页全演示(Overview / Workloads / Deploy / Metrics / Logs)
- [x] 数据源配置切换(`config.example.yaml` 切 fixture 路径,零重编译)
- [x] install.sh 一键部署(Ubuntu 22.04,< 30 min)
- [x] OpenAPI 契约 v1.0(`docs/api-contract.yaml`,15 REST + 4 WS endpoint)
- [x] E2E 主流程通过(Playwright,5 specs / 15 cases + 1 skip)
- [x] 演示物料:`docs/demo.md` 脚本 + `docs/screenshots/` 占位(视频录制由用户负责,见 phase0-review.md MUST-FIX #3)
- [x] CI 全绿

**Should Have**:
- [x] 中英双语完整
- [ ] manual 部署支持拖拽(deferred to Phase 2)
- [ ] 暗色主题(deferred)

---

## 6. Phase 1 实现概览

**周期**:4 周(W1-W4)

**架构层**:
- backend(Go 1.22 + Gin):无状态聚合,Source 接口抽象 mock/k8s/prometheus/crd,REST + WS
- frontend(React 18 + TS 5 + AntD 5 + ReactFlow + dagre):one-page workspace(Phase 12 吸收原 5 页)+ i18n(zh/en)
- configs:mock-data set-a-small(12 workloads incl. 3 PD variants + D6 affinity demo)
- deploy:docker-compose dev stack + install.sh single-node

**关键决策**(见 `docs/adr/`):
- ADR-0001 Phase 0 关键决策
- ADR-0002 不引入 KServe(spec L47-48 客户要求,推理用 vllm-ascend + inference-operator)
- ADR-0003 IMS 3 项推 Phase 9
- ADR-0004 inter-node fabric topology(switch + fabric-link 节点)
- ADR-0005 POD 融合主 Topology(workload + pod 节点 + pd-pair 虚线箭头)

详见 [`docs/phase1-plan.md`](docs/phase1-plan.md)。已知遗留见 [`docs/known-issues.md`](docs/known-issues.md)。

---

## 7. 路线图

| 里程碑 | 内容 | Phase |
|---|---|---|
| **M1** 可演示原型 | 演示后端 + 前端 + Mock + 真实 K8s + Prom | Phase 1-2 |
| **M2** 池化与发现 | 4 级 Pool CRD + NPU 自动发现 + DRA driver | Phase 3-4 |
| **M3** 服务编排 | 预置应用真实部署 + NUMA/HCCS 亲和 + 动态切分 + 垂直伸缩 | Phase 5-8 |
| **M4** 工程化对外 | O2 DMS 接口 + 真实硬件 + 多站点 Karmada | Phase 9-10 |
| **M5** 真生产化 foundation | chart packaging spine + Karmada propagation 第一波 + Frontend Workload extension | Phase 11 |
| **M6** 平台真实化 + 操作台一体化 | aarch64 鲲鹏 + openEuler target + 拓扑全保真 + 前端 one-page workspace | Phase 12 |
| **M7** 真生产化收口(final · **✅ landed**) | real 版完整化 + 真硬件点亮 → 项目收口:Bucket B 真硬件 body(real-Ascend + 真 telemetry + 真拓扑 + 真 Deploy + 真推理)+ Bucket A 生产硬化(OIDC/RBAC + Vault + ClusterQuota 真强制 + Karmada HA + vLLM PD P99 SLA)· 软件层完整 · 真机实测 lab-gated · 不规划 Phase 14 | Phase 13 ✅ |

总周期估计 6-9 个月。

---

## 8. 当前未决事项 / Phase 2 入口

| # | 事项 | 状态 |
|---|---|---|
| 1 | 真机 910B 的 HCCS 拓扑详情 | 等用户提供(影响 Phase 6 调度插件真实度) |
| 2 | 真实 K8s / Prometheus / CRD datasource 实现 | Phase 2 起 |
| 3 | api-contract.yaml WS 类型 + TopologyNode/Edge enum 同步 RFC | 已知遗留(known-issues.md #2) |
| 4 | set-c-stress(800 NPU)FPS 测试 | Phase 2 stretch(ADR-0005 推翻条件验证) |

---

## 9. 联系与反馈

- 架构问题 / RFC：在仓库开 issue，label `rfc`
- 任务包不清晰：开 issue at 协调者
- Bug：开 issue label `bug` + 模块 label

---

**License**:Apache License 2.0 — 见 `LICENSE` 文件
**Copyright**:2026 ai-edge contributors
