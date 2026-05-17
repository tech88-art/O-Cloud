# ADR-0001: Phase 0 关键架构决策

- **状态**：Accepted
- **日期**：2025-XX-XX（Phase 0 评审后填）
- **决策者**：协调者
- **相关**：`docs/architecture.md`, `docs/phase1-plan.md`

---

## 上下文

O-Cloud 边缘云平台样机启动期。基于昇腾 910B（amd64-only），需在 4 周内交付可演示原型，再分阶段补齐池化、动态切分、亲和调度、真实推理。

执行模式为**多个 AI Agent 并行**协作（非人类团队）。

---

## 决策

### 1. 后端语言：Go

**原因**：与 K8s 生态原生集成（client-go、controller-runtime、Kubebuilder），Operator 与 backend 共享类型生态，单二进制部署友好。

**替代方案**：Python（生态丰富但 K8s 集成弱）、Rust（学习曲线陡、生态尚未成熟）。

---

### 2. 前端框架：React + TypeScript + Vite

**原因**：生态最广（AntD / G6 / Recharts），TS 与 OpenAPI 类型生成顺畅，Vite 启动快。

**替代方案**：Vue（生态稍弱）、Svelte（团队学习成本）。

---

### 3. 拓扑可视化：AntV G6（暂选，POC 后定）

**原因**：节点扩展性 / 自定义布局 / 大规模渲染。

**待 POC（P1-T-009）**：与 ReactFlow 对比后由协调者定。

---

### 4. CRD 框架：Kubebuilder v4

**原因**：社区主流，与 controller-runtime 配套，CRD YAML / RBAC / Webhook 自动生成。

---

### 5. K8s 1.31+ 与 DRA

**原因**：DRA 在 K8s 1.31 进入 **Beta(非 GA)**,GA 目标 K8s 1.32(2026-06 RC)。是 NPU 动态切分(Phase 7)的核心机制。

**风险**：
- DRA 仍是 Beta,1.31.x 边缘场景兼容性需验证。已在 P1-T-012 调研。
- 部分 controller / device-plugin 生态对 DRA Beta 跟进不足。

**2026-05-17 修订(Phase 0 评审)**:
- **主路径调整**:Phase 4 默认走 **Ascend Device Plugin v1(非 DRA)**;DRA 作 Phase 4 后段或 Phase 7 增强引入
- **依据**:DRA Beta → GA 时间窗与本项目 Phase 4 实施期重叠,风险不可控;Ascend Device Plugin 是当前生产路径
- **回滚条件**:若 K8s 1.32 GA 推迟到 2026-08+,Phase 4 完全跳过 DRA
- **P1-T-012 子任务追加**:测 K8s 1.31.x DRA Beta 在单节点 KubeEdge 拓扑的稳定性(crash / leak / device hotplug)

---

### 6. 边缘 / 多站点：KubeEdge + Karmada

**原因**：KubeEdge 边缘端 + Karmada 多站点联邦是当前国内主流组合。Phase 1 单节点先不引入，Phase 9 启用 Karmada。

---

### 7. 自研 NPU DRA Driver（**不**用 MindCluster / MindX DL）

**原因**：MindCluster 黑盒、不可扩展，无法支持自研动态切分；自研 DRA Driver 基于 kubernetes-sigs/dra-example-driver，可控可演进。

**代价**：开发量增加（Phase 4 / Phase 7 范围）。

---

### 8. 演示后端无状态（无数据库）

**原因**：演示前端核心需求是聚合 K8s + Prometheus + CRD，无需自己持久化。引入 DB 增加复杂度与一致性负担。

**短期缓存**：内存 LRU，不跨实例同步。

**未来**：如 Phase 9+ 需要事件审计 / 操作历史，再引入数据库（候选 PostgreSQL）。

---

### 9. 监控前端：Grafana iframe 嵌入

**原因**：Grafana 是工业标准，社区 dashboard 丰富，重写得不偿失。前端只需做嵌入与导航。

**代价**：iframe 跨域 / 鉴权稍麻烦（P1-T-010 POC 解决）。

---

### 10. 数据源抽象：DataSource 接口

**原因**：演示后端需支持 mock / k8s / crd / prometheus / configmap 多种来源，配置文件 mapping 驱动切换，便于 Phase 1（全 mock）平滑过渡到 Phase 2（真实集群）。

**实现**：见 `backend/CLAUDE.md §4.1`。

---

### 11. 工程协作：AI Agent 并行模式

**原因**：用户明确选择 AI Agent 并行而非人类团队。

**配套**：
- 严格单模块绑定（避免冲突）
- 共享契约 W1 D1-D2 冻结（OpenAPI / CRD / Mock schema）
- 任务包驱动（机器可验证的 Acceptance Criteria）
- RFC 流程修改共享契约
- 集成 Agent 跑 E2E / 报告问题

详见 `docs/agent-coordination.md`。

---

### 12. CI/CD：GitHub Actions + Phase 1 GHCR / Phase 2+ Harbor

**原因**：用户选择 GitHub Actions + 自建镜像仓库。Phase 1 期间用 GHCR 临时落地，Phase 2 完成 Harbor 部署后迁移。

---

### 13. 不支持 ARM / 鲲鹏

**原因**：用户明确仅支持 amd64 / x86_64 + 昇腾 910B。

**影响**：所有镜像构建仅生成 amd64 layer；mock 数据 `arch: amd64` 硬约束。

---

## 后果

### 正面
- 架构决策清晰，新 Agent 上手快
- 模块边界 + 共享契约模式适合并行开发
- 技术栈选择都是社区主流，招人 / 引入工具方便
- Phase 1 全 mock 模式可独立于硬件就绪

### 负面 / 风险
- 自研 NPU DRA Driver 工作量大（Phase 4 / 7 关键路径）
- AI Agent 模式对契约纪律要求高，违规即冲突
- K8s 1.31 较新，部分 chart / operator 可能未跟进
- Harbor 自建 Phase 2 启用，Phase 1 临时方案需评估迁移成本

---

## 推翻条件

下列情形可启动新 ADR 修订本决策：

| 决策项 | 推翻条件 |
|---|---|
| 后端 Go | 出现致命性能或维护问题（未预期） |
| 前端 React | （几乎不可能） |
| G6 | POC 结果显著不利 / 大规模渲染问题 |
| 自研 DRA Driver | 上游出现等价开源方案且质量过关 |
| 演示后端无状态 | 出现强一致性 / 审计需求 |
| 不引入 MindCluster | 用户明确转向商业支持路线 |

---

## 引用

- `docs/architecture.md`
- `docs/phase1-plan.md`
- `docs/agent-coordination.md`
- 用户在 Phase 0 会话中给出的需求与约束
