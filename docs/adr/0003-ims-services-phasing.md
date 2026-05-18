# ADR-0003: IMS 7-service phasing — provisioning / software-management / lifecycle

- **状态**:Accepted — Option C(用户 2026-05-18 拍板)
- **日期**:2026-05-18
- **决策者**:协调者(用户)
- **相关**:spec/OR-requirements.md L16, docs/architecture.md §1.2

---

## 上下文

spec/OR-requirements.md L16 明列 IMS 7 项服务,参考 StarlingX/OpenShift IMS:

1. **日志服务** ✓ Phase 1 P1-T-301 + Loki/Promtail
2. **监控告警** ✓ Phase 1 P1-T-204/T-209 + Prometheus/Grafana/Alertmanager
3. **资源准备**(provisioning) ❌ **无入口**
4. **软件管理**(software management) ❌ **无入口**
5. **性能分析**(perf analysis) ✓ Phase 1 metrics API + 业务指标 dashboard
6. **生命周期管理**(node/cluster lifecycle) ❌ **无入口**
7. **资源清单**(inventory) ✓ Phase 1 Topology + Workloads + Nodes APIs

→ **3/7 在 Phase 1 完全没规划**。

2026-05-18 spec 核对(grep 全 docs)证实:
- spec 是唯一定义这些需求的文档
- architecture §1.2 仅以 1 行 bullet 复述,未展开
- phase1-plan.md 零任务
- architecture §13 roadmap 也未列入 phase 入口

P3 自查:之前的 phase0-review.md 漏检此项 — **审计不周**。

---

## 决策(待拍板)

3 项缺失服务的 phase 入口,**3 个候选方案**:

### 方案 A:全部推迟到 Phase 9 工程化

- 资源准备 / 软件管理 / 生命周期管理 → **Phase 9 工程化对外**(同 O2 DMS 时机)
- Phase 1 demo 仅展示资源清单 + 监控 + 日志 + 性能
- 理由:这 3 项偏向 Day-2 ops / 商业化能力,样机阶段不必要

### 方案 B:Phase 3 起平行入口

- **Phase 3**:加 `node-lifecycle-operator`(节点 enroll / drain / decommission CRD + Controller)
- **Phase 4**:加 `software-mgmt`(节点级软件版本 inventory + 升级 workflow,参考 StarlingX `sw-deployment`)
- **Phase 4**:加 `bare-metal-provisioning`(可选)— 基于 Metal3 或 Tinkerbell
- 理由:与 NPU DRA / Operator 开发同期,人手够用

### 方案 C:Phase 1 仅"占位演示" + Phase 3+ 真实

- Phase 1 在 Overview 加"基础设施服务"侧边栏占位 tab(资源准备 / 软件管理 / 生命周期 各 1 个 Empty 页 + "Phase 3+ 实现" tooltip)
- 真实功能 Phase 3+ 落实
- 理由:样机至少要让用户看到 7 项能力的入口,即使 Phase 1 不实现

## 影响

- 方案 A:**架构 §13 路线图 Phase 9 加 3 行**;不影响样机交付
- 方案 B:**phase1-plan Phase 3-4 加 3 task**;增加 4-6 人周工作量
- 方案 C:**Phase 1 W4 加 1 task**(占位 UI,~ 0.5d);**Phase 3+ 真实实施**

## 后续动作

用户拍板方案后:
- 方案 A → 修订 ADR 状态 Accepted + 架构 §13 加 3 行
- 方案 B → 修订 ADR + 在 docs/phase-3-plan.md / phase-4-plan.md 补任务(暂不存在)
- 方案 C → 修订 ADR + phase1-plan W4 加 P1-T-308 占位 UI task

## 选定方案:Option C(2026-05-18)

- **Phase 1 W4**:加 P1-T-308 占位 UI(IMS 3 个 tab + Empty + "Phase 3+ 实现" tooltip)
- **Phase 3**:启动后真实 lifecycle / software-mgmt / provisioning(待 Phase 3 plan 起草时落任务)

## 影响

- 架构 §13 路线图 Phase 1 行加 1 条 + Phase 3 行加 3 条
- phase1-plan.md W4 新增 P1-T-308(~0.5d)
- 总 Phase 1 任务数 44 → 45
