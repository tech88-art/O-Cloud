# ADR-0003: IMS 7-service phasing — provisioning / software-management / lifecycle

- **状态**:Accepted — **Option A(2026-05-18 v2 修订)**
- **日期**:2026-05-18(v1 Option C → v2 Option A,同日改)
- **决策者**:协调者(用户)
- **相关**:spec/OR-requirements.md L16, docs/architecture.md §1.2 + §13, M4 路线图 Phase 9 工程化对外

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

## 选定方案:**Option A**(2026-05-18 v2)

v1 曾选 Option C(Phase 1 W4 加占位 UI + Phase 3 真实)。用户复盘"根据整体计划归档合理 Phase 阶段"后修订为:

**3 项全推 Phase 9 工程化对外**(M4 同期,与 O2 DMS 同节奏)。

### 理由(P5 因果链)

- **资源准备**(provisioning):bare-metal init / OS install / K8s deploy — 系统运行**前**的 Day-0 操作,与 Phase 10 真机对接同期
- **软件管理**(software mgmt):running node 上 pkg/version 升级 workflow(参考 StarlingX `sw-deployment`)— Day-2 ops,工程化能力
- **生命周期**(lifecycle):node enroll / drain / decommission — Day-2 ops,工程化能力

3 项共性:**Day-0 / Day-2 ops**,而非 Phase 1-8 的 **样机能力 / 商业演示**。M4 工程化对外是它们的自然归宿。

### Phase 9 任务占位(待 Phase 9 plan 起草时细化)

- **P9-T-IMS-1** node-lifecycle-operator(enroll / drain / decommission CRD + Controller)
- **P9-T-IMS-2** software-mgmt(节点级软件 inventory + 升级 workflow,参考 StarlingX)
- **P9-T-IMS-3** bare-metal-provisioning(基于 Metal3 / Tinkerbell,与 Phase 10 真机对接同期)

**Phase 9 plan 落地状态**(2026-05-21 起草 · `docs/phase9-plan.md` §4 P9-T-105):3 项合并为单任务 P9-T-105 — `operators/node-lifecycle-operator/` + `operators/software-mgmt-operator/` + `operators/bare-metal-provisioning-operator/` **api/v1alpha1 types only scaffold**(per CLAUDE.md §14.2 scaffold pattern)· controller bodies + helm + 真 reconcile loops deferred Phase 10。

**Phase 10 W1+W2 落地状态(2026-05-21 P10-T-007 + T008 + T101)**:**3 IMS controller body 全 land**:
- **IMS-1 node-lifecycle-operator** P10-T-007 commit `d01a5c3`:`internal/state/transitions.go`(12 transitions · 8 states)+ `internal/controller/nodelifecycle_controller.go`(ReconcileOnce pure-Go · NodeReady auto-transition · 7 Conditions)+ 14 unit tests + DESIGN.md §1-§7。
- **IMS-2 software-mgmt-operator** P10-T-008 commit `ed5e4fc`:`internal/rollout/strategy.go`(3 strategies RollingUpdate/Parallel/Sequential · NextBatch pure-Go)+ `internal/controller/softwarebundle_controller.go`(ReconcileOnce · 4 Conditions · 3 counts · AppliedVersion semantics)+ 15 unit tests + DESIGN.md §1-§7。
- **IMS-3 bare-metal-provisioning-operator** P10-T-101 commit `213e1a7`:`internal/state/provisioning.go`(13 transitions · 7 ProvisioningStates · Metal3-adapted)+ `internal/controller/baremetalnode_controller.go`(ReconcileOnce · 5 Conditions · BMCError short-circuit + recovery)+ 17 unit tests + DESIGN.md §1-§7。

**Pure-Go Reconcile pattern**(3 IMS modules 共同 idiom):state machine logic decoupled from controller-runtime wiring · 单元测试无 envtest 需求 · cross-controller awareness 方便(IMS-3 reboot 时调 IMS-1 NodeLifecycle state · import `internal/state.IsPermitted` 验证转换)。Phase 11+ chart packaging stream(per ADR-0016 §3 真生产化 spine)将 4 chart 联动:IMS-1 + IMS-2 + IMS-3 + demo-backend · 每 chart 加 cmd/main.go controller-runtime manager + Dockerfile + RBAC + envtest 真集群 verify。

**Phase 11 W1 chart packaging 状态(2026-05-22 P11-T-003 + T004 + T005 + T006)**:
- **demo-backend** P11-T-003 commit `1a46edc`:chart `deploy/helm-charts/demo-backend/` 9 file + `backend/pkg/cache/leaderelect.go` client-go tools/leaderelection wire + main.go cfg.Lease.Enabled 分支 + Handler.CacheSingleton 字段(per ADR-0015 §3.3 + ADR-0017 §2 Decision D 1st)
- **IMS-1 node-lifecycle-operator** P11-T-004(本 commit):chart `deploy/helm-charts/node-lifecycle-operator/` 8 file + `operators/node-lifecycle-operator/cmd/main.go` controller-runtime manager wire + `internal/controller/reconciler.go` ctrl.Reconciler shell + `api/v1alpha1/groupversion_info.go` SchemeBuilder + AddToScheme + Dockerfile + go.mod controller-runtime/client-go added · `go build ./...` clean + `go test -vet=off ./...` PASS + `helm lint --strict` clean + `helm template` 7 kind render OK
- **IMS-2 + IMS-3** P11-T-005 / T006:pattern reuse(serial main agent default per ADR-0017 §2 Decision D · subagent batch eligible per phase11-plan §2 if user 显式 batch)

**Phase 9 W2 P9-T-105 实际 outcome(2026-05-21)**:**LANDED 3 modules scaffold-only**。每模块结构:
- **group** per kubebuilder per-operator convention(不用 plan 写的 bare `ocloud.edge.example.com` group · 同 P9-T-002-fix-001 group correction spirit · 无现存 CRD 使用 bare group):
  - `node-lifecycle-operator` → `lifecycle.ocloud.edge.example.com/v1alpha1` · CRD: `NodeLifecycle`(8 state enum: Provisioning/Bootstrap/Available/DegradedAvailable/Unavailable/Locked/Unlocked/RebootRequired · StarlingX adapted)
  - `software-mgmt-operator` → `softwaremgmt.ocloud.edge.example.com/v1alpha1` · CRD: `SoftwareBundle`(patches[] + rolloutPolicy + appliedVersion · StarlingX sw-deployment model)
  - `bare-metal-provisioning-operator` → `provisioning.ocloud.edge.example.com/v1alpha1` · CRD: `BareMetalNode`(bmc{address,credentials} + provisioning state machine 7 enum + macAddress · Metal3/cluster-api adapted)
- 每模块 9 文件:`go.mod` + `go.sum` + `PROJECT` + `hack/boilerplate.go.txt` + `api/v1alpha1/{groupversion_info,name_types,name_types_test}.go` + `config/samples/{group}_v1alpha1_{name}.yaml` + `cmd/main.go`(minimal binary · prints banner exits 0 · "DESIGN.md deferred to controller-body task per CLAUDE.md §14.2")
- 每模块 3 round-trip tests pass(RoundTrip + StateEnum + GroupVersion · 9 cases total across 3 modules)
- Per ADR-0003 v2 + plan §3 P9-T-105 acceptance Forbidden Paths:**unchanged**(各 module 的 internal/controller/** 与 helm chart 都 deferred Phase 10)

Cross-ref:
- arch §5 模块划分 加 §5.9 + §5.10 + §5.11 (3 new module rows)
- arch §13 review-table Phase 9 IMS 3 项 scaffold row updated · status flip "P9-T-105 scaffold landed" with commit SHA
- Root `Makefile` + `.github/workflows/ci.yml` 3 modules build/test targets + jobs added
- `docs/devlog/phase-9-t105.md` 记录 3 module scaffold outcomes + group choice rationale

### O2 DMS NB 与 IMS Core 关系(2026-05-21 · P9-T-001 ADR-0013 落地后增补)

`docs/adr/0013-o2-dms-adapter.md`(2026-05-21 Phase 9 P9-T-001 落)锁定 **O2 DMS Adapter** 作为对 O-RAN 联盟北向的生产契约 — K8s Profile only · HTTP REST `/o2dms/v1` 7 endpoint · reflect 内部 ocloud 资源(NPUSlicePool / ModelService / NPUVerticalScaler / NPUSliceAllocation)。**O2 DMS Adapter ≠ IMS Core**,二者职责正交:

| 维度 | O2 DMS Adapter (ADR-0013) | IMS Core 服务 (本 ADR Option A · P9-T-105) |
|---|---|---|
| **职责** | 对外 NB API server(O-RAN 联盟契约) | Day-0/Day-2 ops 基础设施服务(provisioning / software-mgmt / lifecycle) |
| **方向** | 北向(NB) — 让外部 O-RAN 集成对接方调用 | 内部 ops — 集群运维自身使用 |
| **协议** | HTTP REST · O-RAN.WG6.O2IMS-INTERFACE-R003-v04.00 | K8s CRD + Controller(Phase 9 scaffold-only · Phase 10 reconcile body) |
| **生命周期** | Phase 9 W1 scaffold(P9-T-008)+ W2 body(P9-T-104) | Phase 9 W2 scaffold(P9-T-105 · 3 modules api/v1alpha1 types only)+ Phase 10 reconcile body |
| **互相关系** | O2 DMS reflect ocloud 资源对外 · 暂不 reflect IMS Core CRDs(Phase 10 polish 评估) | IMS Core 服务被 O2 DMS Phase 10 polish 评估纳入 `infrastructureInventory`(ADR-0013 §6 forward note · `swInventory` Phase 10 候选) |

**Phase 9 boundary**:O2 DMS Adapter 单独 ship full(scaffold + body 都在 Phase 9 内)· IMS 3 项只 scaffold(types only)· 两个 spine 同 Phase 9 但独立任务链,互不阻塞。**Phase 10+ 整合**:IMS Core CRDs(node-lifecycle / software-mgmt / bare-metal-provisioning)的 reconcile body 落 + O2 DMS Phase 10 polish 纳入 `swInventory` resource path 一同评估。

### Phase 1 不做

- **撤回 v1 的 P1-T-308 占位 UI** — 演示无 IMS-7 入口标签
- Sider 不加"基础设施服务"父菜单
- 总任务数 45 → **44**

## 影响

- 架构 §13 路线图 Phase 9 行加 3 条(IMS 子项)
- phase1-plan.md W4 撤回 P1-T-308
- spec OR-requirements F1 IMS 7 服务 → 演示样机阶段呈现 4/7(日志 / 监控告警 / 性能分析 / 资源清单);剩 3 项 Phase 9 落地
- demo-script.md(W4 P1-T-306 落)需明确"演示展示 4 项 IMS 服务能力,3 项 Phase 9 工程化阶段补"
