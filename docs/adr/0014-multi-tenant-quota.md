# ADR-0014: Multi-tenant Quota CRD + fair scaling admission (Phase 9 安全模型 spine)

- **状态**:Accepted (design freeze; implementation P9-T-005 + P9-T-006)(2026-05-21 — Phase 9 P9-T-002)
- **日期**:2026-05-21
- **决策者**:协调者(用户)
- **相关**:ADR-0001 v3 §5(K8s baseline · K8s-native admission 路径)/ ADR-0008(PD Router mutating webhook · 与 Quota validating webhook 共存于 inference-operator binary · cert-manager 依赖复用 P5-T-101)/ ADR-0009 §2(slice ↔ ResourceClaim operative 表 · NPUSliceAllocation create 是 Webhook A 拦截点)/ ADR-0011 §1(NPUSliceTemplate substrate · Quota.spec.enforcement.maxNPUSliceTemplateRefs whitelist 引用 NPUSliceTemplate name)/ ADR-0012 §5(NPUVerticalScaler mutation model — patches `ModelService.metadata.annotations[npu.huawei.com/slice-template]` · 这是 Webhook B 拦截点)/ ADR-0013(O2 DMS Adapter · O2 NB 注入 ModelService 同样过 Quota webhook chain · Quota enforcement 不需要 NB special path)/ `docs/checkpoint-phase8.md` §6 Phase 9 candidate workstream #2 / `docs/phase9-plan.md` §3 P9-T-002 + §3 P9-T-005 CRD + §3 P9-T-006 controller + webhook / `docs/architecture.md` §6.7(Multi-tenancy 与 scope 警告)§6.8(Allocation / Quota 模型占位 · Phase 5 NPUSliceAllocation landed · 本 ADR 升 Quota 行)§13 Phase 9 review-table 安全模型 row

---

## 上下文

Phase 8 (`phase-8-complete` @ e24f365 + P8-fix-001 CI gate dev HEAD 全绿)合上 M3 服务编排 milestone — NPUVerticalScaler controller(P8-T-007 / 45775bc)8-step Reconcile + status.scaleHistory(rolling 10-entry window)+ status.lastScaleTime 为 multi-tenant fair scaling 提供已就绪 data feed。Phase 5 P5-T-004(8173e83)落地 NPUSliceAllocation CRD types + scheme + 3 round-trip tests · P5-T-005(c283e94)落 controller + audit lifecycle — Quota 本 ADR §2 Decision C Webhook A 拦截 NPUSliceAllocation create 即消费此 substrate。Phase 9 开 **M4 工程化对外** milestone(arch §1.3),两大并列脊柱:(a) **O2 DMS Adapter**(ADR-0013 · 北向生产契约),(b) **Multi-tenant Quota CRD + admission webhook**(本 ADR · 安全模型 spine)。两 spine 同 W1 起草 + 同 phase ship · 互不阻塞。

`docs/checkpoint-phase8.md` §6 Phase 9 candidate workstream #2 列 "**Multi-tenant fair scaling policy**(读 NPUVerticalScaler.status + 新 Quota CRD):Phase 8 NPUVerticalScaler.status.scaleHistory + status.lastScaleTime 已为 multi-tenant 提供数据 · Phase 9 加 Quota CRD + admission webhook 限定 per-namespace scaling rate / template ref"。`docs/architecture.md` §6.7 Multi-tenancy 与 scope 警告(2026-05-17 评审追加)记录"`NPUSlicePool` 是 `scope: Namespaced`,父池是 `scope: Cluster`,后果:多 namespace 可对同一物理 NPU 定义切片,无 RBAC / Admission 强制隔离"+ "Phase 1-2 by-convention 落 `ocloud-system` ns" + "Phase 3 修复 skeleton landed P3-T-005 CEL ValidatingAdmissionPolicy" + "**Phase 9 完整**:multi-tenancy + Karmada RBAC 联动"。`docs/architecture.md` §6.8 Allocation / Quota 模型占位 — Phase 5 NPUSliceAllocation 已 ship · Quota 占位仍 TODO · 本 ADR 升 status。`docs/architecture.md` §13 review-table Phase 9 行 "安全模型(authn/z + multi-tenancy RBAC + NPUSlicePool admission policy)" 检查动作 "Phase 9 启动前完整安全设计 + Karmada RBAC 联动" — 本 ADR 落 multi-tenancy + NPU-aware Quota admission 那一部分;authn/z full + Karmada RBAC 联动 Phase 10 polish(per ADR-0013 §6 forward note + 本 ADR §7)。

**与 K8s ResourceQuota 关系**:K8s 原生 `core/v1.ResourceQuota` 是 generic resource quota(CPU / memory / pod count / PVC等)· **不**感知 NPU 切片语义(`NPUSliceAllocation` count · `NPUVerticalScaler` scale event rate · `NPUSliceTemplate` ref whitelist)。本 ADR `Quota` CRD 是 **NPU-aware** orthogonal layer — 与 K8s ResourceQuota 并存 · 运维同 namespace 可既设 ResourceQuota(CPU/mem limits)又设 Quota(NPU slice allocation limits)· 两端独立 enforce · 互不替代。**命名空间冲突避免**:本 CRD `apiVersion: inference.ocloud.edge.example.com/v1alpha1, kind: Quota` — 与 K8s `core/v1, kind: ResourceQuota` 完全独立 GVK,无 RBAC 或 selector 重叠。

**Phase 8 → Phase 9 boundary 视角**:Phase 8 单租户 busy-idle scaling · Phase 9 多租户 fair scaling — 本 ADR 把"单租户"约束升"namespace-as-tenant"(per ADR-0012 §6 推翻条件 "命名空间作用域 · Phase 9 multi-tenancy 引入 → namespace = tenant boundary 已经满足");scaler.spec 路径不变 · 通过 Quota CRD 加上 admission layer 拦截 over-limit operations。

**本 ADR 不涉及**:
- authn/z full implementation(K8s API server identity + OIDC + ServiceAccount RBAC fine-grained perms)— Phase 10 polish 同 ADR-0013 §6 forward note 协同走
- Karmada cross-cluster Quota aggregation — Phase 10 polish(cross-cluster Quota propagation 走 PropagationPolicy · 累计 usage 走 Karmada control-plane aggregator)
- frontend Quota usage 可视化 / dashboard panel — Phase 10 polish per Phase 6 T102/T103 chat+ADR self-RFC pattern if needed
- 真硬件 lab smoke 验证 Quota enforcement — Phase 10 lab smoke 走 ADR-0011 §3 default policy carry-forward
- O2 DMS NB 路径上 Quota enforcement special handling — ADR-0013 §3 Consequences 已说明 O2 NB 注入 ModelService 走同一 K8s API server admission chain · 本 Quota webhook 自然 enforce,无需 NB special path

---

## 决策

### 1. Scope unit(Decision A):namespace-scope only Phase 9

**承诺范围**:

- Phase 9 `Quota` CRD 落 **namespace-scoped** — 匹配 K8s ResourceQuota mental model(同 namespace 一个 Quota 对象 own 全 namespace 的 NPU 切片 / scale event 限额)
- **namespace = tenant boundary** — 默认假定 1 namespace = 1 tenant · per ADR-0012 §6 推翻条件 "命名空间作用域 · Phase 9 multi-tenancy 引入 → namespace = tenant boundary 已经满足"
- **cluster-scope Phase 10**:cluster-level cap(跨 namespace 总和限制 · e.g. "整个 cluster 最多 32 NPUSliceAllocation")推 Phase 10 polish · 路径详 §6 Open question (b)
- **多 Quota per namespace**:Phase 9 default 假定 1 namespace 1 Quota · 多 Quota per namespace(分类型限 / 时间窗口分)Phase 11+ 评估 · 现阶段若 namespace 出现 2 个 Quota → Webhook A/B 应用 AND 语义(同时满足才允许)+ emit Event "MultipleQuotasFoundUsingAnd" · 不是 hard error(避免 footgun)

**为什么不是 cluster-scope first**:cluster-scope Quota 需要 cross-namespace aggregation pattern · Phase 9 W1 scope 内增加复杂度不必要;namespace-scope 与 K8s ResourceQuota 同步骤更易理解 · Phase 10 polish 时加 ClusterQuota 是 additive 不破。

### 2. Quota CRD shape(Decision B)

**API group**:`inference.ocloud.edge.example.com/v1alpha1`(与 ModelService + NPUVerticalScaler 同 group · 同 inference-operator binary scheme · per P9-T-005 task entry self-correction — 原 v1 写 `ocloud.edge.example.com/v1alpha1` 错:NPUSlicePool 在 `ims.ocloud.edge.example.com`,NPUSliceAllocation 在 `npu.ocloud.edge.example.com`,**无 CRD** 用 bare `ocloud.edge.example.com` group · P9-T-002-fix-001 自我纠正 2026-05-21)

**Kind**:`Quota`(短 · clear · 不与 K8s `ResourceQuota` 冲突 GVK)

**Scope**:**namespace-scoped**

**Spec fields**:
- `spec.enforcement.maxSliceAllocations int32` — 该 namespace 同时存在的 NPUSliceAllocation 上限 · default 0 = 无限(Quota disabled for this dimension)
- `spec.enforcement.maxScaleEventsPerWindow.count int32` — 该 namespace `windowSeconds` 时窗内允许的 NPUVerticalScaler scale event 次数上限 · default 0 = 无限
- `spec.enforcement.maxScaleEventsPerWindow.windowSeconds int32` — 滑动窗口长度 · default 3600(1 小时)· min 60
- `spec.enforcement.maxNPUSliceTemplateRefs []string` — NPUSliceTemplate name whitelist(optional)· default empty `[]` = 允许任意 NPUSliceTemplate ref · 非空时 NPUVerticalScaler.spec.scaleSlice.{busy,idle}TemplateName 必须在列表内

**Status fields**:
- `status.usage.currentSliceAllocations int32` — 当前该 namespace 内 NPUSliceAllocation 个数(由 controller 60s tick 同步)
- `status.usage.scaleEventsInWindow int32` — 当前 windowSeconds 内 NPUVerticalScaler scale event 累计计数
- `status.lastSyncTime *metav1.Time` — controller 上次同步 status.usage 的时间(供 admission webhook 评估 cache 新鲜度)
- `status.conditions []metav1.Condition` — `Active` / `EnforcementOK` 两态(controller 写入 · Active=False 表示 controller 暂停 · EnforcementOK=False 表示 webhook 注册失败 · 详 §5)

**Printer columns**(kubebuilder markers):
- `MaxAllocations` ← `.spec.enforcement.maxSliceAllocations`
- `Used` ← `.status.usage.currentSliceAllocations`
- `MaxScaleEvents` ← `.spec.enforcement.maxScaleEventsPerWindow.count`
- `ScaleEventsWindow` ← `.spec.enforcement.maxScaleEventsPerWindow.windowSeconds`
- `Status` ← `.status.conditions[?(@.type=='Active')].status`

**YAML form sample**(P9-T-005 落地 in `operators/inference-operator/config/samples/`):

```yaml
apiVersion: inference.ocloud.edge.example.com/v1alpha1
kind: Quota
metadata:
  name: ai-edge-demo-quota
  namespace: ai-edge-demo
spec:
  enforcement:
    maxSliceAllocations: 8              # 同时最多 8 个 NPUSliceAllocation in namespace
    maxScaleEventsPerWindow:
      count: 5                          # 同 windowSeconds 最多 5 次 NPUVerticalScaler scale 事件
      windowSeconds: 3600
    maxNPUSliceTemplateRefs:            # 仅允许引用以下两个 NPUSliceTemplate
      - qwen-pd-busy
      - qwen-pd-idle
status:
  conditions:
    - type: Active
      status: "True"
      reason: ControllerReady
    - type: EnforcementOK
      status: "True"
      reason: WebhooksRegistered
  usage:
    currentSliceAllocations: 3
    scaleEventsInWindow: 1
  lastSyncTime: "2026-05-21T14:32:17Z"
```

**Go types 详 P9-T-005 落地**:`operators/inference-operator/api/v1alpha1/quota_types.go`。

### 3. Enforcement points(Decision C):TWO ValidatingAdmissionWebhooks colocated with inference-operator binary

**承诺范围**:

- 部署形态决策 = **colocated with inference-operator binary** for Phase 9 simplicity
  - 理由 1:cert-manager 依赖已存在(Phase 5 P5-T-101 / c3452d3)· 重用同 cert-manager Issuer + Certificate · 不引新 cert bootstrap path
  - 理由 2:Quota controller + Quota webhook + 现有 ModelService controller + NPUVerticalScaler controller + PD Router webhook(ADR-0008)同 binary · leader-elect single replica · 避免多 binary 复杂度
  - 理由 3:Phase 10 polish 若 Quota webhook 维护成本爆炸(实质化 webhook reject 性能瓶颈 / cross-controller race)→ split 到 new binary `operators/quota-controller/` · re-eval at T006 implementation per §6 Open question (a)
  - **alternative considered + rejected**:`operators/quota-controller/` 独立 binary 起手 — 拒因 Phase 9 W1 scope 内 cert-manager 重新 bootstrap 复杂 + leader-elect 多 binary 管理复杂 + 故障域隔离收益 < 部署复杂度成本(Phase 9 demo scale)
- **Webhook A**:`ValidatingAdmissionWebhook` on `NPUSliceAllocation` create
  - **Match**:apiGroup `npu.ocloud.edge.example.com`(NPUSliceAllocation 实际 group · per `operators/npu-dra-driver/api/v1alpha1/types.go` `+groupName=npu.ocloud.edge.example.com`),apiVersion `v1alpha1`,resources `npusliceallocations`,operations `CREATE`(Phase 9 不拦 UPDATE — `NPUSliceAllocation.spec` 不变)
  - **Decision logic**:
    1. Get Quota in same namespace as incoming NPUSliceAllocation(in-memory cache · 5s TTL · fallback Get)
    2. If Quota not found(无 quota set in namespace)→ admit · fail-open(无限语义)
    3. If `quota.spec.enforcement.maxSliceAllocations == 0` → admit · 0=unbounded 语义
    4. Compute `effectiveUsage = quota.status.usage.currentSliceAllocations`(可能 stale,详 §5)
    5. If `effectiveUsage + 1 > quota.spec.enforcement.maxSliceAllocations` → **Reject** · reason `QuotaExceeded` · message `"namespace <ns> at NPUSliceAllocation cap <N>; current <M>"`
    6. Else → admit
- **Webhook B**:`ValidatingAdmissionWebhook` on `NPUVerticalScaler` update(specifically patch on `spec.scaleSlice.busyTemplateName` or `spec.scaleSlice.idleTemplateName` per ADR-0012 §5 mutation model · 这是 controller patch ModelService.spec.template.sliceTemplate 之前的"上游 patch chain")
  - **Match**:apiGroup `inference.ocloud.edge.example.com`,apiVersion `v1alpha1`,resources `npuverticalscalers`,operations `UPDATE` only;Phase 9 不拦 CREATE(创建 scaler 时不算 scale event)+ 不拦 DELETE
  - **Decision logic**(per ADR-0012 §5 — NPUVerticalScaler controller patches `ModelService.metadata.annotations[npu.huawei.com/slice-template]` · Webhook B 拦截的是 **NPUVerticalScaler.spec patch** 此即 controller 内部决定要 scale 的入口 · 不是 ModelService patch 本身):
    1. Get Quota in same namespace as NPUVerticalScaler
    2. If Quota not found OR maxScaleEventsPerWindow.count == 0 → admit · fail-open
    3. Diff incoming `.spec.scaleSlice.{busy,idle}TemplateName` vs current — 若 NOT changed → admit(纯 status update etc.)
    4. Compute `effectiveScaleEvents = quota.status.usage.scaleEventsInWindow`(per controller 60s tick)
    5. If `effectiveScaleEvents + 1 > quota.spec.enforcement.maxScaleEventsPerWindow.count` (within `windowSeconds`)→ **Reject** · reason `ScaleRateExceeded` · message `"namespace <ns> scale event rate cap <N>/<windowSec>s; current <M>"`
    6. If `quota.spec.enforcement.maxNPUSliceTemplateRefs` non-empty AND new `busy/idleTemplateName` NOT IN whitelist → **Reject** · reason `TemplateRefNotAllowed`
    7. Else → admit

**为什么 Webhook B 拦 NPUVerticalScaler.spec patch 而非 ModelService.spec.template.sliceTemplate patch**:
- NPUVerticalScaler controller(P8-T-007)patch `ModelService.spec.template.sliceTemplate`(per ADR-0012 §5)是**已 scaled 的下游动作** · Quota 应**前置**拦截在 NPUVerticalScaler 决定 scale 时(spec change)
- 若直接拦 ModelService patch · 会与 ADR-0008 PD Router mutating webhook chain 顺序冲突 + 与用户直接 kubectl edit ModelService 路径混淆(那是 ModelService spec patch 而非 NPUVerticalScaler-driven scale)· 误拒率高
- NPUVerticalScaler.spec patch 拦截点专一 = scaler-driven scale event · 与运维直接 ModelService spec change 不冲突

### 4. Status sync(Decision D):60s tick + 5s cache TTL fallback

**承诺范围**:

- **Quota controller reconcile loop**(Phase 9 P9-T-006 落地 in `operators/inference-operator/internal/controller/quota_controller.go`):
  1. List NPUSliceAllocation in target namespace · count → `status.usage.currentSliceAllocations`
  2. Get NPUVerticalScaler list · sum scaleHistory entries(ADR-0012 §4 status.scaleHistory · rolling 10-entry FIFO)within `windowSeconds` → `status.usage.scaleEventsInWindow`
  3. Update Quota.status.usage atomically · set `lastSyncTime = now`
  4. Update Quota.status.conditions[Active] = True
  5. Requeue 60s
- **Admission webhook cache layer**(Phase 9 P9-T-006 落地 in `operators/inference-operator/internal/webhook/quota_admission.go`):
  - In-memory map keyed by `<namespace>` · value = `{quotaSnapshot, fetchedAt}`
  - TTL = 5s · webhook 决策时:`if now - fetchedAt < 5s use cache; else Get from API server + refresh cache`
  - 目的:burst create N NPUSliceAllocation 时,5s 内 N 次 webhook 不重复 hammer API server · 减负载
  - **fail-open on Get NotFound**:webhook Get Quota 返回 NotFound → admit(无 quota set = unbounded · 见 Decision C step 2)· **不**因 cache miss 报错
- **Why 60s tick + 5s TTL not eventual consistency**:
  - 60s tick 是 status.usage 真值的更新 cadence — 用户在 dashboard 看到的 usage 滞后 1 min OK(monitoring scope)
  - 5s TTL 是 webhook 决策的"stale tolerance" — burst N create scenario 内,N 次 webhook 用同 cache snapshot · Phase 9 demo 接受短暂 +N 误差(实际 over-limit by 1-N 个 allocation)· production-grade strict 走 Phase 10 polish strong-consistency mode(每次 webhook strict Get · 性能成本接受)
  - Phase 9 demo budget 内 cache TTL 5s 是 sweet spot

---

## 3. 后果

### 正面

- **arch §6.7 Multi-tenancy 警告 row 完整化**:Phase 1-2 by-convention → Phase 3 admission policy skeleton → **Phase 9 Quota CRD + admission webhook 完成 "Phase 9 完整:multi-tenancy + Karmada RBAC 联动" 的 multi-tenancy 那一部分**(Karmada 部分 Phase 10 polish 走 ADR-0013 §6 forward note)
- **arch §6.8 Allocation / Quota 模型占位 row 解锁**:Phase 5 NPUSliceAllocation landed P5-T-004/T005 · Quota Phase 9 landed P9-T-005/T006 · arch §6.8 TODO 关闭
- **arch §13 review-table Phase 9 安全模型 row 部分推进**:"Phase 9 启动前完整安全设计" → "in flight via ADR-0014(multi-tenancy + NPU-aware Quota admission)" + Phase 10 polish(authn/z + Karmada RBAC 联动)走 ADR-0013 §6 forward note + 本 ADR §7
- **inference-operator binary 单 leader-elect 收敛**:Quota controller + Quota webhook + ModelService controller + NPUVerticalScaler controller + PD Router webhook(ADR-0008)共 binary · cert-manager 重用 · 部署复杂度增量小
- **ADR-0013 协同清晰**:O2 NB 注入 ModelService 走 K8s API server admission chain · 自然过 Quota webhook B(若 NB Create 触发 NPUVerticalScaler.spec update)+ 本 Quota webhook A(若 NB Create 直接触发 NPUSliceAllocation create chain — Phase 9 NB shape 通常不直接 emit allocation · 经 inference-operator → claim_controller chain · Webhook A 同样拦截)· **不需要** O2 DMS Adapter 内特殊路径
- **NPUSliceTemplate ref whitelist 反向锁住 Phase 7 NPUSliceTemplate substrate**(ADR-0011 §1)— `maxNPUSliceTemplateRefs` 可以由 cluster admin 显式限定 namespace 能用哪些 template · 防止 namespace 越权用其他 tenant 的 template
- **测试性**:Webhook A/B 都可以走标准 envtest pattern · 6 cases 设计(P9-T-006 acceptance)覆盖正/负路径 + 缓存 fallback + whitelist allow/deny
- **K8s ResourceQuota 互补不替代**:运维想同时管控 CPU/mem 用 K8s ResourceQuota · 管控 NPU slice 数 + scale event rate 用本 ADR Quota · 两端独立 enforce · 心智模型清晰

### 负面 / 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| **5s cache TTL** 在 burst create 内允许 +N 误差(Decision D 描述) | 实际 over-limit by 1-N 个 allocation(burst 内 N 次 webhook 用同 cache snapshot) | Phase 9 demo 接受 · production-grade strict 走 Phase 10 polish strong-consistency mode(每 webhook strict Get)· §6 Open question (d) 留路径 |
| **60s sync tick** 让 dashboard usage 滞后 1 min | UI 显示 stale usage 误导操作判断 | Phase 9 dashboard 可视化是 Phase 10 polish · Phase 9 仅 kubectl printer columns · 60s 滞后接受;production 路径 Phase 10 polish 评估 informer event-driven sync(替换 60s tick) |
| **multi-Quota per namespace** AND 语义可能 footgun(运维误配 2 个互相覆盖的 Quota) | 比预期更严格 enforce · 触发"无端 reject" | webhook 检测多 Quota 时 emit Event "MultipleQuotasFoundUsingAnd" 提示运维 · 不 hard error · Phase 10 polish 评估单 Quota per namespace constraint(validation webhook on Quota create) |
| **webhook reject 透传到 NPUVerticalScaler spec patch** — controller 自身在 reconcile 时 patch 自己的 spec 也会被 reject? | Phase 8 ADR-0012 §5 NPUVerticalScaler controller patch 的是 **ModelService.spec.template.sliceTemplate**(via annotation per Phase 8 mutation adaptation),**不**是 NPUVerticalScaler.spec 自身 patch — 故 controller 自身 patch chain 不被 Webhook B 拦截 | ADR-0012 §5 实际 mutation 路径是 controller patches ModelService annotation(per phase8 mutation adaptation note in arch §13 Phase 8 row "annotation vs spec.template.sliceTemplate") · Webhook B 拦截的是 **用户或 O2 NB 触发的 NPUVerticalScaler.spec.scaleSlice patch**(外部触发 scale rule change) · controller 内部 reconcile 不触发此路径 · 二者解耦 |
| **fail-open on Quota NotFound** 让 namespace 无 Quota 时无限制 | 运维忘配 Quota → namespace 可耗尽 cluster NPU | Phase 9 接受 — fail-open 是 admission webhook 通行 default(避免 webhook 故障锁死所有 namespace);Phase 10 polish 可加 cluster-level default Quota fallback(走 ClusterQuota 路径 · §6 Open question (b)) |
| **NPUSliceAllocation create 路径** 是 claim_controller(npu-dra-driver)owner-ref 创建 — Webhook A 拦截 controller 内部 create? | Phase 5 P5-T-005 audit lifecycle path NPUSliceAllocation create 是由 claim controller 内部 emit · Webhook A reject 会阻塞 allocation 流程 | Webhook A 设计 = 拦截**所有** create source(用户 / O2 NB / claim_controller)— 这是预期行为(若 namespace at-cap · 任何来源的 create 都应被拒)· 关键是 claim_controller 必须**预先 check Quota 状态**(post-reject 走 Pod scheduling backoff 路径 · 不卡 reconcile)· P9-T-006 acceptance 覆盖此场景;Phase 10 polish 评估 claim_controller pre-check Quota 避免 admission round-trip |
| **scaleHistory sum 与实际 scale event 数偏差**(ADR-0012 §4 status.scaleHistory rolling 10-entry FIFO · oldest entries evicted) | quota usage 计数低估 — 实际 scale event > 10 次时只看到 10 | Phase 9 接受 — windowSeconds default 3600 + maxScaleEventsPerWindow default 推荐 ≤ 5/hour 业务 reasonable · scaleHistory 10-entry 在合理 cap 范围内已足 · windowSeconds 长 + cap 高场景(>10 events/window)走 Phase 10 polish:event-driven counter(K8s Event + counter sum)替换 scaleHistory-based sum |
| **cert-manager 重用 inference-operator webhook cert** 与 ADR-0008 PD Router cert + ADR-0012 NPUVerticalScaler cert 同 ca · 单点故障域 | inference-operator binary cert 错全 webhook 都挂 | 接受 — Phase 5 P5-T-101 cert-manager 已是 inference-operator binary 生命线 · 单一 cert source 反而减少 cert sprawl;Phase 10 polish 评估 webhook cert split(每 webhook 独立 cert · 故障域隔离)若实质需求 |
| **Phase 9 namespace-scope only · cluster-scope cap 缺位** | 多 namespace 各自 within-quota 但累计 over-cluster · cluster 资源耗尽 | Phase 9 接受 — single-cluster demo + namespace boundary 已是 tenant 主隔离;Phase 10 polish 加 ClusterQuota(§6 Open question (b)) |
| **token-bucket vs sliding-window count** rate cap 算法 | sliding-window count 在 windowSeconds 边界附近 allows burst — N at edge of window-1 + N at edge of window-2 = 2N within 2 windowSeconds | Phase 9 接受 — count 简单 demo 友好;Phase 10 polish 评估 token-bucket(rate-limit smooth) · §6 Open question (e)留路径 |

---

## 4. CRD schema

YAML form 见 §2 Decision B sample · Go types 详 P9-T-005 落地 in `operators/inference-operator/api/v1alpha1/quota_types.go` per 下列大纲:

```go
// +kubebuilder:resource:scope=Namespaced,shortName=quota
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="MaxAllocations",type=integer,JSONPath=`.spec.enforcement.maxSliceAllocations`
// +kubebuilder:printcolumn:name="Used",type=integer,JSONPath=`.status.usage.currentSliceAllocations`
// +kubebuilder:printcolumn:name="MaxScaleEvents",type=integer,JSONPath=`.spec.enforcement.maxScaleEventsPerWindow.count`
// +kubebuilder:printcolumn:name="ScaleEventsWindow",type=integer,JSONPath=`.spec.enforcement.maxScaleEventsPerWindow.windowSeconds`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=='Active')].status`
type Quota struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec              QuotaSpec   `json:"spec,omitempty"`
    Status            QuotaStatus `json:"status,omitempty"`
}

type QuotaSpec struct {
    Enforcement QuotaEnforcement `json:"enforcement"`
}

type QuotaEnforcement struct {
    // MaxSliceAllocations is the namespace-wide cap on concurrent NPUSliceAllocation
    // objects. 0 means unbounded.
    // +kubebuilder:default=0
    // +kubebuilder:validation:Minimum=0
    MaxSliceAllocations int32 `json:"maxSliceAllocations,omitempty"`

    // MaxScaleEventsPerWindow caps the rate of NPUVerticalScaler.spec mutations
    // (scale events) within a sliding window. count=0 means unbounded.
    MaxScaleEventsPerWindow ScaleEventRateCap `json:"maxScaleEventsPerWindow,omitempty"`

    // MaxNPUSliceTemplateRefs is an optional whitelist of NPUSliceTemplate names
    // that NPUVerticalScaler.spec.scaleSlice.{busy,idle}TemplateName may reference.
    // Empty (default) allows any template.
    MaxNPUSliceTemplateRefs []string `json:"maxNPUSliceTemplateRefs,omitempty"`
}

type ScaleEventRateCap struct {
    // Count is the maximum number of scale events allowed within WindowSeconds. 0 = unbounded.
    // +kubebuilder:default=0
    // +kubebuilder:validation:Minimum=0
    Count int32 `json:"count,omitempty"`
    // WindowSeconds is the sliding-window length in seconds.
    // +kubebuilder:default=3600
    // +kubebuilder:validation:Minimum=60
    WindowSeconds int32 `json:"windowSeconds,omitempty"`
}

type QuotaStatus struct {
    Conditions []metav1.Condition `json:"conditions,omitempty"`
    Usage      QuotaUsage         `json:"usage,omitempty"`
    LastSyncTime *metav1.Time     `json:"lastSyncTime,omitempty"`
}

type QuotaUsage struct {
    CurrentSliceAllocations int32 `json:"currentSliceAllocations"`
    ScaleEventsInWindow     int32 `json:"scaleEventsInWindow"`
}

// +kubebuilder:object:root=true
type QuotaList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []Quota `json:"items"`
}

const (
    ConditionQuotaActive         = "Active"
    ConditionQuotaEnforcementOK  = "EnforcementOK"
)
```

---

## 5. Enforcement contract

**Webhook ManifestEvent**(P9-T-006 helm template `deploy/helm-charts/inference-operator/templates/webhook.yaml` 加入):

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: quota-admission.inference.ocloud.edge.example.com
  annotations:
    cert-manager.io/inject-ca-from: "{{ .Release.Namespace }}/inference-operator-webhook-cert"
webhooks:
- name: npusliceallocation.quota.inference.ocloud.edge.example.com
  clientConfig:
    service:
      name: inference-operator-webhook
      namespace: "{{ .Release.Namespace }}"
      path: /validate-npu-ocloud-edge-example-com-v1alpha1-npusliceallocation-quota
  rules:
  - apiGroups: ["npu.ocloud.edge.example.com"]
    apiVersions: ["v1alpha1"]
    resources: ["npusliceallocations"]
    operations: ["CREATE"]
  failurePolicy: Fail
  matchPolicy: Equivalent
  sideEffects: None
  admissionReviewVersions: ["v1"]
  timeoutSeconds: 5
- name: npuverticalscaler.quota.ocloud.edge.example.com
  clientConfig:
    service:
      name: inference-operator-webhook
      namespace: "{{ .Release.Namespace }}"
      path: /validate-inference-ocloud-edge-example-com-v1alpha1-npuverticalscaler-quota
  rules:
  - apiGroups: ["inference.ocloud.edge.example.com"]
    apiVersions: ["v1alpha1"]
    resources: ["npuverticalscalers"]
    operations: ["UPDATE"]
  failurePolicy: Fail
  matchPolicy: Equivalent
  sideEffects: None
  admissionReviewVersions: ["v1"]
  timeoutSeconds: 5
```

**`failurePolicy: Fail` rationale**:Quota enforcement 是 security boundary · webhook 故障 ≠ admit(那是 fail-open 的反义)· 故障时拒绝 API call 比让 quota 失效更安全(运维必须修 webhook · 不会"静默放过")。

**`timeoutSeconds: 5` rationale**:webhook decision logic + cache layer 典型耗时 < 10ms · 5s 是 K8s API server 单次 admission 上限 sensible default · 大于则评估 webhook 性能瓶颈(typically cache miss + slow API server Get)。

**`sideEffects: None` rationale**:Webhook A/B 是 pure validating · 不 mutate object · 不写其他 K8s resource(仅读 in-memory cache + 偶尔 Get Quota)· `None` 是正确表达。

**`matchPolicy: Equivalent` rationale**:防止未来 CRD API version bump(v1alpha1 → v1beta1)时 webhook 自动 match 新 version(否则旧 version 行 admit · 新 version 行 by-pass · 不一致)。

**Cert reuse path**:Phase 5 P5-T-101(`c3452d3`)落地 cert-manager Issuer `inference-operator-selfsigned-issuer` + Certificate `inference-operator-webhook-cert` · 全 inference-operator binary webhook(PD Router · NPUVerticalScaler future · Quota A/B)共用此 cert。Phase 10 polish 评估 split(每 webhook 独立 cert · §3 risk row 倒数第 2)。

---

## 6. Open questions

### (a) Colocated vs new binary `operators/quota-controller/`(Phase 9 default · Phase 10 re-eval)

- **现状**:Phase 9 default = colocated with inference-operator binary(§2 Decision C)
- **决策点**:Phase 10 polish 时是否 split 出独立 binary `operators/quota-controller/`
- **决策分支**:
  - **Stay colocated**(if webhook reject 性能 < 100ms · controller reconcile race 与现有 controller 无冲突):保持 single inference-operator binary · cert-manager 重用 · 部署简单
  - **Split new binary**(if 出现 实质性能 / race 问题):新建 `operators/quota-controller/` · 独立 cert · 独立 leader-elect · 故障域隔离;成本 = 新模块开销 + cert bootstrap 复杂
- **Default Phase 9**:colocated · P9-T-006 implementation 时收集 performance data + race log · Phase 10 evaluate

### (b) Cluster-scope cap Phase 10 path

- **现状**:Phase 9 namespace-scope only(§2 Decision A)
- **决策点**:Phase 10 polish 时如何加 cluster-level cap
- **决策分支**:
  - **新 CRD `kind: ClusterQuota`**:scope=Cluster · spec/status 与 Quota 同 shape · 独立 admission webhook · 与 namespace Quota AND 语义(同时满足 cluster 和 namespace 才允许)
  - **现 Quota CRD 加 `spec.clusterScope bool` flag**:同 CRD 表达 namespace + cluster · 但 Quota.metadata.namespace 与 clusterScope flag 互斥语义复杂(Quota.metadata.namespace 非空时 clusterScope 必须 false · 否则 admission policy validate-reject)
- **Default Phase 10**:新 CRD `kind: ClusterQuota`(additive · 不破 namespace Quota · 心智模型清晰)
- **Karmada 协同**:cross-cluster aggregator + PropagationPolicy 一起设计(Open question (c)耦合)

### (c) Karmada cross-cluster Quota aggregation(Phase 10 polish)

- **现状**:Phase 9 single-cluster
- **决策点**:Phase 10 polish 时如何 propagate Quota + aggregate cross-cluster usage
- **决策分支**:
  - **Karmada-native**:Quota 是 Karmada PropagationPolicy target · 同 namespace Quota 同步到所有 member cluster · cross-cluster usage aggregator 走 Karmada control plane 累计
  - **Per-cluster Quota independent**:每 cluster 独立 Quota · cross-cluster 不 aggregate · 适用"namespace 在每个 cluster 物理隔离"模型
- **Default Phase 10**:Karmada-native(per ADR-0001 §6 + ADR-0013 §6 forward note Karmada multi-cluster 路径基线)

### (d) Strong-consistency mode for webhook decision(替代 5s cache TTL)

- **现状**:Phase 9 5s cache TTL(§2 Decision D · webhook 决策可能 stale 5s)
- **决策点**:Phase 10 polish 时是否提供 strong-consistency 模式(每 webhook strict Get · 无 cache)
- **决策分支**:
  - **Configurable per Quota**:`spec.enforcement.strictMode bool` · True = 每 webhook strict Get · False(default)= 5s cache · production 用户根据负载 trade off
  - **Cluster-level config**:`inference-operator` chart values 加 `quotaWebhookCacheTTLSeconds` · 全 cluster 统一
- **Default Phase 10**:per-Quota strictMode flag(更细粒度 · production 不同 namespace 不同 SLA)

### (e) Token-bucket vs sliding-window count for scale rate cap

- **现状**:Phase 9 sliding-window count(§3 Consequences)
- **决策点**:Phase 10 polish 时是否升级为 token-bucket 算法
- **决策分支**:
  - **Stay sliding-window count**:简单 · demo 友好 · windowSeconds 边界 burst 是已知接受
  - **Token-bucket**:rate-limit smooth · 边界 burst 抑制 · 算法复杂度+ · 需 status 额外字段(bucket tokens · refill rate)
- **Default Phase 10**:per-Quota algorithm flag(`spec.enforcement.maxScaleEventsPerWindow.algorithm: countSlidingWindow | tokenBucket`)· additive
- **Phase 11+**:基于 production 实际 burst pattern 决定 default

### (f) Frontend Quota usage 可视化

- **现状**:Phase 9 仅 kubectl printer columns + ADR §2 Decision B sample · 无 dashboard panel
- **决策点**:Phase 10 polish frontend Workload page 是否加 Quota usage 显示
- **决策分支**:per Phase 6 T102/T103 chat+ADR self-RFC pattern · 走 demo-backend new API + frontend new component
- **Default Phase 10**:加 Workload page 顶部 Quota usage bar(progress bar by namespace · 红 / 橙 / 绿 thresholds)+ NPUVerticalScaler scale event timeline 与 Quota cap overlay · 与 ADR-0013 §6 forward note frontend extension 同期评估

---

## 7. Forward notes(Phase 10+)

> 🆕 **Phase 10 polish**:cluster-scope Quota(`kind: ClusterQuota`)+ Karmada cross-cluster propagation + aggregator — per §6 Open questions (b) + (c) · 与 ADR-0013 §6 Karmada multi-cluster 同期。

> 🆕 **Phase 10 polish**:strong-consistency webhook 模式(per-Quota strictMode flag · §6 Open question (d))· production-grade strict enforcement · cache miss penalty 接受。

> 🆕 **Phase 10 polish**:token-bucket scale rate cap algorithm(per §6 Open question (e))· additive algorithm flag · backward-compat sliding-window count default。

> 🆕 **Phase 10 polish**:frontend Workload page Quota usage 可视化(§6 Open question (f))· progress bar + scale event timeline overlay · 与 ADR-0013 §6 frontend forward note 同期。

> 🆕 **Phase 10 polish**:event-driven status.usage sync(替换 60s tick)· informer event → quota controller 增量更新 status.usage · 减少 sync lag · dashboard real-time。

> 🆕 **Phase 10 polish**:`scaleHistory rolling 10-entry FIFO` 替换为 event-driven counter(K8s Event + counter sum · 不依赖 ADR-0012 NPUVerticalScaler.status.scaleHistory window 限额)· 解决 risk row "scaleHistory sum 与实际 scale event 数偏差" · 配 Open question (d) strict mode 一起。

> 🆕 **Phase 11+ 候选**:Quota CRD 升级 v1beta1 → v1 promotion(per K8s API maturity convention)· `apiVersion: inference.ocloud.edge.example.com/v1` · v1alpha1 backward-compat 保留 N phases。

> 🆕 **Phase 11+ 候选**:Webhook cert split(per Quota A · Quota B · ADR-0008 PD Router · NPUVerticalScaler future webhook 独立 cert)· 故障域隔离 · cert-manager 多 Certificate 管理 · 当 cert reuse 出 incident 时触发。

> 🆕 **Phase 11+ 候选**:claim_controller pre-check Quota(在 controller 内部 emit NPUSliceAllocation create 前先 read Quota state)· 减少 admission round-trip + Pod scheduling backoff(per §3 risk row claim_controller path)· 若 webhook reject 实质性能瓶颈出现触发。

> 🆕 **Phase 11+ 候选**:NPU-aware K8s ResourceQuota extension — 若 K8s 上游引入 NPU device quota extension(类似 GPU device quota)→ 评估 deprecate 本 Quota CRD · 升级到 upstream ResourceQuota · backward-compat 路径 N phases。

---

## 推翻条件

| 决策项 | 推翻条件 |
|---|---|
| Quota CRD namespace-scope only Phase 9 | Phase 10 Karmada federation 落地 → cluster-scope ClusterQuota 加 · namespace Quota baseline 保留 |
| Colocated with inference-operator binary | webhook reject 性能 > 100ms · controller reconcile race 与现有 controller 冲突 · Phase 10 split 到 `operators/quota-controller/` |
| Webhook A/B `failurePolicy: Fail` | 出现 webhook故障导致全 cluster API 锁死 incident · 评估降级到 Ignore + alarm + 运维快速修复 SLA |
| Sliding-window count rate algorithm | windowSeconds 边界 burst 实际造成 production incident → 升级 token-bucket · §7 forward note 路径 |
| 5s cache TTL | Phase 9 demo 内 quota over-limit by 1-N · 单 namespace burst pattern 频繁 → 升级 strong-consistency mode · §6 Open question (d) |
| scaleHistory sum 作为 scaleEventsInWindow 数据源 | windowSeconds > 1h + maxScaleEventsPerWindow > 10 production scenario · 替换 event-driven counter · §7 forward note 路径 |
| K8s ResourceQuota 互补独立(orthogonal) | K8s 上游引入 NPU device ResourceQuota extension · 评估 upstream 替代 · §7 forward note 路径 |
| fail-open on Quota NotFound | 运维忘配 Quota 实质成为 production gap · 评估 cluster-level default Quota fallback · §6 Open question (b) ClusterQuota 路径同步 |

---

## 引用

- `docs/adr/0001-phase0-key-decisions.md` v3 §5(K8s baseline · K8s-native admission webhook 路径)
- `docs/adr/0008-pd-router-webhook.md` — PD Router mutating webhook 与 Quota validating webhook 共存于 inference-operator binary · cert-manager 重用(本 ADR §5 Cert reuse path 强调)
- `docs/adr/0009-npu-dra-driver.md` §2(slice ↔ ResourceClaim operative 表 · NPUSliceAllocation create 是 Webhook A 拦截点 row · 本 ADR cross-ref · ADR-0009 小编辑加 Phase 9 Quota admission validates row)§5(Phase 5 实施要点 · claim_controller path · NPUSliceAllocation owner-ref 创建路径)
- `docs/adr/0011-npu-dynamic-slicing-and-source-interface.md` §1(NPUSliceTemplate substrate · Quota.spec.enforcement.maxNPUSliceTemplateRefs whitelist 引用 NPUSliceTemplate name · ADR-0011 §1 後果 row 加 Quota CRD substrate interaction cross-ref · 本 ADR §2 Decision B `maxNPUSliceTemplateRefs` 字段)
- `docs/adr/0012-busy-idle-vertical-scaler.md` §3(scaling decision rules · Phase 9 forward note "if Quota.ScalingAllowed=False → stay + reason QuotaExhausted" · 本 ADR Webhook B Reject 后 NPUVerticalScaler controller 路径走 stay)§5(mutation model · NPUVerticalScaler controller patches `ModelService.metadata.annotations[npu.huawei.com/slice-template]` · Webhook B 拦截 NPUVerticalScaler.spec 而非 ModelService.spec)§7(forward notes · Phase 9 multi-tenant fair scaling + PromQL custom metric 双 candidate · 本 ADR 落 multi-tenant fair scaling · ADR-0012 §3 + §7 status flip "Phase 9 fair scaling policy" 行 · ADR-0012 小编辑反向 cross-ref)
- `docs/adr/0013-o2-dms-adapter.md` — O2 NB 注入 ModelService 走同一 K8s API server admission chain · 自然过 Quota webhook · ADR-0013 §3 Consequences ADR-0014 协同段已说明 · 本 ADR §1 Context 强调 "**本 ADR 不涉及** O2 DMS NB 路径上 Quota enforcement special handling"
- `docs/checkpoint-phase8.md` §6 Phase 9 candidate workstream #2 "Multi-tenant fair scaling policy"
- `docs/phase9-plan.md` §1 Scope summary "Multi-tenant Quota + fair scaling(arch §13 Phase 9 安全模型 spine)" 行 · §2 Task package overview T002 + T005 + T006 · §3 P9-T-002 acceptance criteria 1:1 mapping · §5 DoD W1 Foundation T005 + T006 / Out of scope rows · §6 Phase 9 risks "lab access remains a soft 3rd per ADR-0011 §3 default policy carry-forward"
- `docs/architecture.md` §6.7(Multi-tenancy 与 scope 警告 · 2026-05-17 评审追加 · 本 ADR 升 "Phase 9 完整" 那一部分 multi-tenancy + NPU-aware admission)§6.8(Allocation / Quota 模型占位 · Phase 5 NPUSliceAllocation landed · 本 ADR Quota 落 · §6.8 TODO 关闭)§13 review-table Phase 9 安全模型 row(从 "启动前完整安全设计" → "in flight via ADR-0014" 本 ADR 小编辑)
- Phase 5 P5-T-004 commit 8173e83 — NPUSliceAllocation CRD types + scheme + 3 round-trip tests(Webhook A 拦截点)
- Phase 5 P5-T-005 commit c283e94 — NPUSliceAllocation controller + audit lifecycle
- Phase 5 P5-T-101 commit c3452d3 — cert-manager Issuer + Certificate(本 ADR §5 Cert reuse path)
- Phase 8 P8-T-007 commit 45775bc — NPUVerticalScaler controller 8-step Reconcile + status.scaleHistory(Webhook B 拦截点 input data source)
- upstream:K8s `admissionregistration.k8s.io/v1.ValidatingWebhookConfiguration`(本 ADR §5 Webhook ManifestEvent)
- upstream:K8s `core/v1.ResourceQuota`(orthogonal — Quota CRD 是 NPU-aware · K8s ResourceQuota 是 generic resource quota · 本 ADR §1 Context 强调互补不替代)
- upstream:`sigs.k8s.io/controller-runtime` admission webhook builder(本 ADR §2 Decision C + §5 实现路径)

---

**END of ADR-0014**
