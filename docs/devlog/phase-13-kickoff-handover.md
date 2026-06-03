# Phase 13 Kickoff — 交接(for next session)

> 本 session 收工于 `phase-12-complete` 之后的一批 post-tag 工作。Phase 13
> (生产硬化 + 真硬件验证)**新开 session** 起步。本文件 = 新 session 的入口。

## 上一程(本 session · 已全绿落库)

- **拓扑视觉 polish ×10**(`fix-004…013`):节点内 HCCS 环 / active 连线流动 /
  折叠 chevron + Obsidian 图标分置两端 / Splitter 全屏拉伸 + 侧栏封顶 / Fabric·
  Workloads 开关上下文化(→详情面板)/ workloads 按 worker 分组多列状态点 /
  下钻 NPU 分层视图 / 语言切换纯图标 / MIG 式切片分区 + 负载列头。
- **demo/real 两交付版本架构分离**(`3765f5c`):config + deploy profile,不 fork
  代码。`backend/configs/config.real.yaml` + `deploy/profiles/{demo,real}/` +
  `install.sh --profile`。
- **e2e-kind CI flake 硬化**(`f0ce285`):kind/cert-manager 拉取重试 + 构建前
  释放磁盘。
- **dev HEAD = `f0ce285` · CI + e2e-kind 全绿 · 全部已 push。**

## Phase 13 范围(canonical 出处 · 勿重写,直接读)

- `docs/checkpoint-phase12.md` **§6 Phase 13+ handoff brief**(权威清单)
- `docs/build-and-production-validation.md` **§4.4 真机特有验证 + §5 生产硬化验收清单**
- `deploy/profiles/real/README.md`(real 版边界 + 前置 + mock-data 缺口)
- real-Ascend stub:`operators/npu-dra-driver/internal/source/realascend/realascend.go`
  (`ErrNotImplemented`)· `npusmi/parse.go`(npu-smi topo 解析器已写+测)

### 主要功能两桶
- **A · 生产硬化(纯软件,可立即做)**:OIDC/RBAC(替换 O2 DMS 静态 token)·
  ClusterQuota admission webhook B + 跨集群 usage reconcile · Karmada control-plane
  HA + 多站点 PropagationPolicy + cross-cluster RBAC · Vault Secret · vLLM PD P99 SLA。
- **B · 真硬件验证(lab-gated · 无真机做不了)**:real-Ascend body(npu-smi/DCMI)·
  真拓扑聚合(`GetTopology`)· 真 `Deploy()` · 真 PCIE/HCCS/network telemetry 替换
  fixture · 真 CANN + vllm-ascend 推理 · real 版 topology/deploy 解锁。
- **carries**:Track B Volcano · Track C Partitionable Devices(KEP-4815 Beta)·
  Go v1 ResourceSlice schema 迁移(随 K8s 1.36)· 前端 G6 5.x 拓扑重写。

## 建议第一步(新 session · 守 plan/execute 分离)

1. **先开 plan-only session 起草 `docs/phase13-plan.md`** —— 定 Phase 13 IN-scope
   (建议:A 桶纯软件先行;B 桶 lab-gated 标 carry,真机就位再点亮)+ 任务拆分 +
   `ADR-0023` Phase 13 entry decisions。**写完 commit 即停**,执行再新开 session
   (per memory `feedback_plan_vs_execute_session_split`)。
2. demo/real 双版已就位 → Phase 13 的真硬件功能填的是 **real 版的 body**,demo 版
   不动(回归基线)。

## 启动 checklist(新 session 必读)
`README.md` · 根 `CLAUDE.md` · `docs/agent-coordination.md` §0a · `docs/architecture.md`
§1.3 路线图 · `docs/checkpoint-phase12.md` §6 · 本文件。
