# ADR-0012: Busy-idle 垂直伸缩 (重启切片 pattern) + NPUVerticalScaler CRD + 集成 NPUSliceTemplate / AllocateBundle (Phase 8)

- **状态**:Accepted (design freeze; implementation P8-T-005..T008)(2026-05-21 — Phase 8 P8-T-001)
- **日期**:2026-05-21
- **决策者**:协调者(用户)
- **相关**:ADR-0001 v3(双轨路径)/ ADR-0002(不引入 KServe)/ ADR-0008(PD Router webhook · 与 NPUVerticalScaler 共存于 inference-operator binary)/ ADR-0009(npu-dra-driver · Partitionable Devices forward)/ ADR-0010(scheduler-plugin · §7 Phase 7+ forward 增 ADR-0012 cross-ref)/ ADR-0011(NPU 动态切分 + NPUSliceTemplate substrate · §1 後果 → ADR-0012 cross-ref) / `docs/checkpoint-phase7.md`(Phase 8 handoff brief)/ `docs/phase8-plan.md`(15 tasks W1+W2)/ `docs/architecture.md` §13(Phase 8 row "调研,可能只支持横向,垂直走'重启切片'")/ §3.4(NPU/AI 运行时:NPUVerticalScaler forward ref)/ §1.3(Phase 路线图 Phase 8 "忙闲时垂直伸缩")

---

## 上下文

Phase 7 (`phase-7-complete` @ 538f094 含 P7-fix-001..004 post-tag CI gate 系列) 落地了 NPU 动态切分 NPUSliceTemplate substrate(P7-T-006/T007:CRD types + template engine Validate/Decompose)+ AllocateBundle 自由函数(P7-T-105 · 4 unit tests)+ HCCS 8 卡 adjacency chart 默认(P7-T-008)+ schedulerName auto-stamp(P7-T-003)+ multi-ring fixture(P7-T-103/T104 synthetic)+ Partitionable Devices KEP-4815 spike(P7-T-106 · 1.36 Beta confirmed · GA unconfirmed)。Phase 7 checkpoint §6 列出 carry-forward 项,Phase 8 plan 选定 8 项 W1 + 7 项 W2 落地(余下推到 Phase 9-10)。本 ADR 锁定 Phase 8 关键设计基石:

1. **垂直伸缩落地形态**:arch §13 Phase 8 row 标"调研,可能只支持横向,垂直走'重启切片'(Phase 7 NPUSliceTemplate substrate · ADR-0011 §1)"。NPU vendor 栈(CANN ≤ 8.1 / Ascend driver ≤ 24.x / npu-smi / DCMI)**不暴露** 在线 re-partition 而保持 device 状态的能力 — workload state(KV cache · loaded model weights · in-flight HCCL ranks)无法 mid-flight transfer。**调研结论**:Phase 8 commit "重启切片" pattern 为 deliverable — controller 观察 busy-idle 指标 → 决策 target template → patch target `ModelService.spec.template.sliceTemplate` ref → inference-operator rolling restart 触发 Pod recreation → claim_controller(T008 wiring · Phase 7 deferred body)按新 template 重新分配。**横向伸缩**(HPA-style replica count)走 K8s 标准 HPA off-the-shelf,**不是** Phase 8 deliverable(arch §13 同 row "调研,可能只支持横向"是另一选项的描述,不是 Phase 8 deliverable;Phase 8 走"垂直 + 重启切片"路径,因为它消费 Phase 7 NPUSliceTemplate substrate 而横向 HPA 不消费)。

2. **NPUVerticalScaler CRD shape**:Phase 8 需要新 CRD 承载"目标 ModelService + 触发指标 + busy/idle template ref + cooldown + 历史记录"。本 ADR 落 CRD spec/status schema + Go types markers + printer columns + 命名空间作用域 + finalizer。

3. **集成 NPUSliceTemplate(ADR-0011)+ AllocateBundle(P7-T-105)**:Phase 8 controller patch 的是 `ModelService.spec.template.sliceTemplate` 字段(ADR-0011 NPUSliceTemplate ref);Pod recreation 后 claim_controller(P8-T-008 wiring)按 label `npu.huawei.com/slice-template=<new-template>` 读 NPUSliceTemplate → Engine.Decompose → AllocateBundle → N allocations。**ADR-0012 不引入新分配算法**,只 mutate spec 触发既有 reconcile 链。

4. **GitOps 协调冲突**:NPUVerticalScaler patch 的 ModelService 字段(sliceTemplate ref)会被 GitOps tool(ArgoCD / Flux)reconcile 回去。本 ADR 明确**annotation 提示约定** + **运维 ignoreDifferences 指南**,而**不**引入 admission webhook ownership transfer(那是 Phase 9 multi-tenancy 引入 Quota 时才考虑的复杂度)。

5. **多租户 + custom metric**:Phase 8 是**单租户** + **NPUUtilization built-in metric only**。多租户 fair scaling policy(读 NPUVerticalScaler.status + Quota CRD)+ Prometheus PromQL custom metric extension 是 **Phase 9** 范围,本 ADR §7 留 forward notes。

---

## 决策

### 1. 伸缩模式:重启切片 pattern(Decision A)

**承诺范围**:

- Phase 8 **commit** 重启切片 pattern 为 deliverable。NPUVerticalScaler controller 通过 spec patch 触发 Pod recreation,**不**尝试在线 re-partition。
- **brief outage** during rolling restart(typical ~30s for single-replica ModelService)is **accepted** for Phase 8 demo polish(单租户 · multi-tenant fair scaling 是 Phase 9)。
- Phase 8 不实现 live HCCL rank migration / KV cache transfer / loaded weights move — `docs/cni-hccl-research.md` §5 已明记 CNI-level live migration gap;重启切片 pattern **绕开** 这些 gap。

**整体流程**(reconcile loop 视角,§5 mutation model 详细):

```
NPUVerticalScaler.Reconcile()
  ├── 1. Get NPUVerticalScaler → if !Active → return
  ├── 2. Get target ModelService → if NotFound → status.Active=False reason TargetNotFound → return
  ├── 3. Query Ingestor (T006) for window-averaged metric
  │      └── if NoData → return + requeue 30s (no scaling decision this tick)
  ├── 4. Compute decision:
  │      ├── metric > busyThreshold → target = busyTemplateName
  │      ├── metric < idleThreshold → target = idleTemplateName
  │      └── else                    → stay current (no-op)
  ├── 5. If current template == decision → return (no-op)
  ├── 6. If in cooldown (now < lastScaleTime + cooldownSeconds):
  │      └── set CooldownActive=True · return + requeue cooldown remainder
  └── 7. Else (commit scaling):
         ├── patch ModelService.spec.template.sliceTemplate = decision (merge strategy)
         ├── stamp annotation ocloud.edge.example.com/vertical-scaler-managed=<scaler-name>
         ├── append ScaleEvent to status.scaleHistory (max 10 · FIFO eviction)
         ├── set lastScaleTime=now
         ├── set ScalingInProgress=True
         └── return + requeue 60s to observe rolling restart
```

**触发频率**:reconcile 默认 30s 周期(matches `metrics.scrapeIntervalSeconds` default in P8-T-006 chart values);cooldown 默认 600s 防 flapping(可通过 `spec.cooldownSeconds` 调,Phase 8 sample = 300s for demo · production recommend ≥ 300s)。

**不动**:inference-operator 现有 ModelService 控制器 reconcile 不感知 NPUVerticalScaler — 它只观察 spec change → rolling restart Deployment(Phase 5 既有路径);claim_controller(npu-dra-driver)按 label 解析 NPUSliceTemplate(P8-T-008 wiring);两端解耦。

### 2. NPUVerticalScaler CRD shape(Decision B)

**API group**:`inference.ocloud.edge.example.com/v1alpha1`(与 ModelService 同 group · 同 operator binary owns reconcile · 避免 operator 模块 proliferation)。

**Scope**:**namespace-scoped**(与 target ModelService 共 namespace)。Phase 9 multi-tenancy 引入时 namespace = tenancy boundary。

**Finalizer**:`npuverticalscaler.inference.ocloud.edge.example.com/finalizer` — 用于 stop 进行中的 scaling 决策 + 清理 scaleHistory 索引。Phase 8 实现 simple "on delete, no-op besides finalizer removal";Phase 9 quota counter dec 走此 hook。

**Printer columns** (kubebuilder markers):

| 列 | 路径 | 说明 |
|---|---|---|
| Target | `.spec.target.name` | 目标 ModelService 名 |
| CurrentTemplate | `.status.observedTarget.currentTemplate` | 当前 sliceTemplate ref(从 target ModelService 读) |
| LastScaleTime | `.status.lastScaleTime` | 最近一次 scaling 触发时间 |
| Status | `.status.conditions[?(@.type=='Active')].status` | Active condition status |

**CRD schema 详见 §4**;Go types 详见 §4 Go types 区块。

### 3. Scaling decision flow(Decision C)

**Decision inputs**:
1. NPUVerticalScaler.spec(target + metric thresholds + busy/idle template refs + cooldown)
2. 目标 ModelService current spec(读 `.spec.template.sliceTemplate` 字段)
3. Ingestor(T006)返回的 window-averaged metric value
4. NPUVerticalScaler.status(lastScaleTime · scaleHistory · conditions)

**Decision output**(每 reconcile tick):
- `{decision: "scale-up" | "scale-down" | "stay", targetTemplate?: string, reason: string}`

**Decision rules**(单租户 Phase 8):

| 当前 sliceTemplate | metric | 决策 | 备注 |
|---|---|---|---|
| any | > busyThreshold | **scale-up** to busyTemplateName | 跨 busy threshold |
| any | < idleThreshold | **scale-down** to idleTemplateName | 跨 idle threshold |
| any | idleThreshold ≤ metric ≤ busyThreshold | stay | hysteresis 带 |
| any | NoData (Ingestor 报无数据) | stay + requeue | Prometheus unreachable / metric absent |
| any | current == decision target | stay (no-op) | 已经在目标 template,不必 patch |
| any | cooldown active | stay + requeue cooldown remainder | 防 flapping |

**多租户 Phase 9 forward note**:Phase 9 引入 Quota CRD 时,decision rules 加 row "if Quota.ScalingAllowed=False → stay + reason QuotaExhausted";本 ADR 不预占该字段。

**不引入** auto-scaling 加速因子 / 预测性 scaling / 时间窗口 scheduling(cron-style ramp)— 这些都是 Phase 9+ 范围。Phase 8 = 简单 threshold + hysteresis + cooldown。

---

## 4. NPUVerticalScaler CRD schema

### YAML form(P8-T-005 落地 in `operators/inference-operator/config/samples/`)

```yaml
apiVersion: inference.ocloud.edge.example.com/v1alpha1
kind: NPUVerticalScaler
metadata:
  name: qwen-pd-scaler
  namespace: ai-edge-demo
spec:
  target:
    apiVersion: inference.ocloud.edge.example.com/v1alpha1
    kind: ModelService
    name: qwen-pd
    namespace: ai-edge-demo         # required; cross-namespace target out of Phase 8 scope
  metric:
    type: NPUUtilization            # enum: NPUUtilization (Phase 8) | PrometheusQuery (Phase 9 forward)
    busyThreshold: 75               # int32, 0-100; metric > this → scale up
    idleThreshold: 20               # int32, 0-100; metric < this → scale down
    windowSeconds: 300              # int32, default 300; sliding-window average
  scaleSlice:
    busyTemplateName: qwen-pd-busy   # NPUSliceTemplate ref (cluster-scoped per ADR-0011 §4)
    idleTemplateName: qwen-pd-idle   # NPUSliceTemplate ref
  cooldownSeconds: 600              # int32, default 600; min gap between scaling events
status:
  conditions:
    - type: Active
      status: "True"
      reason: ScalerReady
    - type: ScalingInProgress
      status: "False"
      reason: NoActiveTransition
    - type: CooldownActive
      status: "False"
      reason: CooldownExpired
  observedTarget:
    name: qwen-pd
    namespace: ai-edge-demo
    currentTemplate: qwen-pd-busy
  lastScaleTime: "2026-05-21T14:32:17Z"
  scaleHistory:
    - time: "2026-05-21T14:32:17Z"
      fromTemplate: qwen-pd-idle
      toTemplate: qwen-pd-busy
      reason: "metric 87 > busyThreshold 75"
    - time: "2026-05-21T13:45:02Z"
      fromTemplate: qwen-pd-busy
      toTemplate: qwen-pd-idle
      reason: "metric 12 < idleThreshold 20"
    # ... up to N=10 entries · FIFO eviction · oldest first
```

### Go types(P8-T-005 落地 in `operators/inference-operator/api/v1alpha1/npuverticalscaler_types.go`)

```go
// +kubebuilder:resource:scope=Namespaced,shortName=npuvs
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.spec.target.name`
// +kubebuilder:printcolumn:name="CurrentTemplate",type=string,JSONPath=`.status.observedTarget.currentTemplate`
// +kubebuilder:printcolumn:name="LastScaleTime",type=date,JSONPath=`.status.lastScaleTime`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=='Active')].status`
type NPUVerticalScaler struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec              NPUVerticalScalerSpec   `json:"spec,omitempty"`
    Status            NPUVerticalScalerStatus `json:"status,omitempty"`
}

type NPUVerticalScalerSpec struct {
    // Target is the ModelService this scaler manages.
    Target TargetRef `json:"target"`

    // Metric describes how scaling decisions are computed.
    Metric MetricSpec `json:"metric"`

    // ScaleSlice maps busy/idle states to NPUSliceTemplate refs.
    ScaleSlice ScaleSliceSpec `json:"scaleSlice"`

    // CooldownSeconds is the minimum gap between scaling events.
    // +kubebuilder:default=600
    // +kubebuilder:validation:Minimum=0
    CooldownSeconds int32 `json:"cooldownSeconds,omitempty"`
}

type TargetRef struct {
    // APIVersion of the target object.
    // +kubebuilder:default="inference.ocloud.edge.example.com/v1alpha1"
    APIVersion string `json:"apiVersion,omitempty"`
    // Kind of the target object.
    // +kubebuilder:default=ModelService
    // +kubebuilder:validation:Enum=ModelService
    Kind string `json:"kind,omitempty"`
    // Name of the target object.
    Name string `json:"name"`
    // Namespace of the target object. Required; must match the scaler namespace
    // (Phase 8 single-tenant scope; cross-namespace target is Phase 9 forward).
    Namespace string `json:"namespace"`
}

type MetricSpec struct {
    // Type selects the metric backend.
    // +kubebuilder:default=NPUUtilization
    // +kubebuilder:validation:Enum=NPUUtilization
    Type MetricType `json:"type,omitempty"`
    // BusyThreshold (0-100); metric > this triggers scale-up.
    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:validation:Maximum=100
    BusyThreshold int32 `json:"busyThreshold"`
    // IdleThreshold (0-100); metric < this triggers scale-down.
    // +kubebuilder:validation:Minimum=0
    // +kubebuilder:validation:Maximum=100
    IdleThreshold int32 `json:"idleThreshold"`
    // WindowSeconds is the sliding-window length for averaging the metric.
    // +kubebuilder:default=300
    // +kubebuilder:validation:Minimum=30
    WindowSeconds int32 `json:"windowSeconds,omitempty"`
}

// +kubebuilder:validation:Enum=NPUUtilization
type MetricType string

const (
    // MetricTypeNPUUtilization reads ascend_npu_utilization_percent
    // exposed by ascend-npu-exporter-plus, scoped by namespace + model_service label.
    MetricTypeNPUUtilization MetricType = "NPUUtilization"
)

type ScaleSliceSpec struct {
    // BusyTemplateName references an NPUSliceTemplate (cluster-scoped, ADR-0011 §4)
    // used when metric is above busyThreshold.
    BusyTemplateName string `json:"busyTemplateName"`
    // IdleTemplateName references an NPUSliceTemplate used when metric is below
    // idleThreshold.
    IdleTemplateName string `json:"idleTemplateName"`
}

type NPUVerticalScalerStatus struct {
    // Conditions tracks Active / ScalingInProgress / CooldownActive lifecycle.
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // ObservedTarget records the last observed state of the target ModelService.
    ObservedTarget *ObservedTargetStatus `json:"observedTarget,omitempty"`

    // LastScaleTime is the timestamp of the most recent scaling event.
    LastScaleTime *metav1.Time `json:"lastScaleTime,omitempty"`

    // ScaleHistory is a rolling window of recent scaling events (oldest first).
    // +kubebuilder:validation:MaxItems=10
    ScaleHistory []ScaleEvent `json:"scaleHistory,omitempty"`
}

type ObservedTargetStatus struct {
    Name            string `json:"name"`
    Namespace       string `json:"namespace"`
    CurrentTemplate string `json:"currentTemplate,omitempty"`
}

type ScaleEvent struct {
    Time         metav1.Time `json:"time"`
    FromTemplate string      `json:"fromTemplate,omitempty"`
    ToTemplate   string      `json:"toTemplate"`
    Reason       string      `json:"reason"`
}

// +kubebuilder:object:root=true
type NPUVerticalScalerList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []NPUVerticalScaler `json:"items"`
}

const (
    // ConditionActive indicates the scaler is reconciling its target.
    ConditionActive = "Active"
    // ConditionScalingInProgress is True between patching the target spec
    // and observing the rolling restart complete.
    ConditionScalingInProgress = "ScalingInProgress"
    // ConditionCooldownActive is True when cooldown blocks scaling.
    ConditionCooldownActive = "CooldownActive"

    // AnnotationManagedBy is stamped on target ModelService when the scaler
    // first patches sliceTemplate. Operators must configure GitOps tools to
    // ignore .spec.template.sliceTemplate on annotated ModelServices.
    AnnotationManagedBy = "ocloud.edge.example.com/vertical-scaler-managed"

    // FinalizerName is set on NPUVerticalScaler at create; removed on delete
    // after stop-scaling cleanup completes.
    FinalizerName = "npuverticalscaler.inference.ocloud.edge.example.com/finalizer"
)
```

**Phase 8 sample compositions**(P8-T-005 落地 in `config/samples/`):

| 样例文件 | Target | scaleSlice.busy/idle | metric thresholds | cooldown |
|---|---|---|---|---|
| `inference_v1alpha1_npuverticalscaler.yaml` | ModelService `qwen-pd` (ai-edge-demo ns) | busy=`qwen-pd-busy` (1× vir08), idle=`qwen-pd-idle` (1× vir04) | busy=75, idle=20, window=300 | 300 |

---

## 5. Scaling mutation model

**目标 mutation**:NPUVerticalScaler controller **直接 patch** target `ModelService.spec.template.sliceTemplate` 字段 — mirror K8s HPA pattern(patch Deployment.spec.replicas)。**不**走 admission webhook ownership transfer / **不**接管 ModelService spec(那是 Phase 9 multi-tenancy 引入 Quota 时的复杂度)。

**Patch 顺序**(reconcile tick 内):
1. `client.Get(target ModelService)` — 取当前 spec
2. 若 `currentTemplate == decisionTemplate` → return no-op
3. `mergePatch := { "metadata": { "annotations": { "ocloud.edge.example.com/vertical-scaler-managed": "<scaler-name>" } }, "spec": { "template": { "sliceTemplate": "<decisionTemplate>" } } }`
4. `client.Patch(ctx, ms, client.RawPatch(types.MergePatchType, mergePatch))` — 原子 merge patch
5. Append `ScaleEvent{ time: now, fromTemplate: currentTemplate, toTemplate: decisionTemplate, reason: "<metric value> > busyThreshold <X>" }` to `status.scaleHistory`(FIFO,max 10)
6. `status.lastScaleTime = now`
7. `status.conditions[ScalingInProgress] = True · reason ScalingTransitioning`
8. `client.Status().Update(ctx, scaler)` — atomic status update
9. Requeue 60s 观察 rolling restart 完成

**Annotation 提示约定**(GitOps 协调):

- NPUVerticalScaler 首次 patch ModelService.spec.template.sliceTemplate 时 stamp annotation `ocloud.edge.example.com/vertical-scaler-managed=<scaler-name>`(常量 `AnnotationManagedBy`)。
- 运维必须配置 GitOps tool 对带此 annotation 的 ModelService 忽略 `.spec.template.sliceTemplate` 字段:
  - **ArgoCD**:`spec.ignoreDifferences[].jsonPointers: ["/spec/template/sliceTemplate"]`(`spec.ignoreDifferences[].name` 匹配 ModelService.metadata.name)
  - **Flux**:`spec.ignore: [{ paths: [.spec.template.sliceTemplate] }]`
- ADR-0012 不引入 admission webhook 自动 enforce — 文档化约定 + chart values.yaml example + Phase 8 demo README operator-guide 段足以,产品级 enforcement 是 Phase 9 multi-tenancy 引入 Quota 时再加。
- **未带 annotation** 的 ModelService 不被 NPUVerticalScaler 管理 — 即用户可以混部 "scaler-managed" + "manual" ModelService 在同一 cluster · 互不干扰。

**Rolling restart 协调**:

- NPUVerticalScaler **不**直接驱动 Pod 删除 / Deployment rollout。它只 patch `ModelService.spec.template.sliceTemplate` ref。
- inference-operator 现有 ModelService 控制器(Phase 5 P5-T-007 落地 + Phase 7 schedulerName auto-stamp 升级)观察 spec change → 更新 Deployment.spec.template → K8s Deployment controller 走 rolling update → Pod 删除 + 重建。
- Pod 重建后 claim_controller(P8-T-008 wiring 引入)读 Pod label `npu.huawei.com/slice-template=<new-template>` → `client.Get(NPUSliceTemplate{<new-template>})` → `template.Engine.Decompose(spec)` → `allocator.AllocateBundle(bundle)` → N allocations + N audit objects + Pod annotation `npu.huawei.com/preferred-hccs-ring=<ringID>`(T104-v2 hard-fail 用的 annotation)。
- **NPUVerticalScaler 不参与** AllocateBundle / 资源分配决策 — 它只触发 spec change,让既有 reconcile 链(inference-operator + claim_controller)消费。

**Transient outage**:rolling restart 期间(typical ~30s for single-replica)PD-pair service 短暂不可用。Phase 8 demo 接受;**多 replica** ModelService(production)走 maxUnavailable=0 / maxSurge=1 K8s rolling update 默认 → 始终有一个 replica 在 serve。

**冲突处理**(同一 ModelService 被多个 NPUVerticalScaler manage):

- annotation `ocloud.edge.example.com/vertical-scaler-managed=<scaler-name>` 值唯一 — 若新 scaler 首次 patch 时发现 annotation 已存在且值不同 → `status.conditions[Active]=False reason TargetAlreadyManaged` + emit Event "ConflictWithExistingScaler" + return(不 patch)。
- 运维责任:确保 1 ModelService = 1 NPUVerticalScaler。Phase 8 不引入 controller-side referential integrity check。

---

## 6. 后果

### 正面

- **arch §13 Phase 8 row 承诺兑现**:本 ADR 落 "重启切片" pattern 决策 + NPUVerticalScaler CRD shape,Phase 8 不再需要在 entry meeting 内现场设计。
- **NPUSliceTemplate substrate(ADR-0011)立即变现**:Phase 7 落地的 NPUSliceTemplate CRD 此前主要服务"用户手工指定切分",现在成为 NPUVerticalScaler 的 spec target — 跨 ADR 复用,无新 CRD 重复。
- **AllocateBundle 控制器路径(P7-T-105 deferred body)Phase 8 落地**:T008 wiring 把 P7-T-105 的 4 unit tests 自由函数升级为完整 envtest 流程,补 Phase 7 deferred wiring。Phase 8 demo 端到端跑通 NPUVerticalScaler → ModelService → Pod → claim_controller → AllocateBundle → N allocations 链。
- **inference-operator + npu-dra-driver 解耦保持**:NPUVerticalScaler 落在 inference-operator binary(co-located with ModelService),不需要新 operator 模块;npu-dra-driver 端只多消费 Pod label(P8-T-008),不感知 NPUVerticalScaler 存在。两端通过 spec/label 解耦。
- **HPA 共存路径开放**:NPUVerticalScaler patch 的是 `spec.template.sliceTemplate`(template ref)·HPA patch 的是 `spec.replicas`(replica count)— 字段不冲突,可同时部署。Phase 9 多租户 fair scaling 可读 NPUVerticalScaler.status + HPA 的 metrics 共同决策。
- **GitOps 协调清晰**:annotation 提示 + 运维 ignoreDifferences 指南是产品惯例(K8s HPA 早期同样模式),不引入额外 admission webhook 复杂度。Phase 9 真要 enforce 时再加。

### 负面 / 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| **重启切片** 模式期间 transient outage(~30s for single-replica) | demo 演示可能"被看到一瞬不可用" | sample ModelService 默认 replicas=2 · K8s Deployment 默认 maxUnavailable=25% maxSurge=25% → 总有一个 replica 在 serve;Phase 8 demo 录屏可在 multi-replica scenario 录,避免观感问题 |
| **cooldownSeconds 默认值** 选择(Phase 8 默认 600s) | 调小 → flapping;调大 → 响应慢 | 默认 600s 对单租户演示合理;sample 用 300s 让 demo 录屏内可见 1 次 scale;production 文档建议 ≥ 300s |
| **GitOps reconciliation 冲突**(若运维忘配 ignoreDifferences) | scaler patch 后 ArgoCD/Flux 立即 reconcile 回 → 死循环 patch + revert | annotation 提示 + chart README + Phase 8 demo README operator-guide 段明示;production 落地前 e2e smoke 必含 GitOps drift 验证(Phase 9 范围) |
| **NPUSliceTemplate ref 在 NPUVerticalScaler.spec.scaleSlice 引用的 template 不存在** | rolling restart 后 claim_controller 找不到 template → ResourceClaim 卡 unallocated | NPUVerticalScaler controller 在 Patch 前 `client.Get(NPUSliceTemplate)` validate · 不存在 → `status.conditions[Active]=False reason TemplateNotFound` + emit Event;**不**继续 patch(避免破坏 target ModelService) |
| **多 replica ModelService 跨 replica 不同时切**(rolling update 间) | scaling 期间一部分 Pod 还在 idle template · 一部分已 busy → 资源占用临时翻倍 | 接受 — single-tenant Phase 8 scope;Phase 9 multi-tenant fair scaling 引入 Quota 时再 enforce "总和 ≤ N NPU";rolling update 时间窗口典型 60-120s,翻倍占用短暂 |
| **Ingestor(T006)Prometheus 不可达** | NoData 持续 → 永远 stay 当前 template | Ingestor 返回 `IngestorResult{NoData: true}` · controller 此 tick 不决策;若 NoData 持续超过 N(默认 5)tick → `status.conditions[Active]=False reason MetricsUnreachable` + emit Event(让运维知道);Active=False 时不再 reconcile metrics 直到 NPUVerticalScaler 重新 enable |
| **scaleHistory rolling window 大小**(N=10) | 频繁 scale 时历史快速 rotate · 长时窗审计断 | Phase 8 在 status.scaleHistory 内保留 10 条 + 同时 emit K8s Event 让运维通过 `kubectl describe` / Loki 长期审计;production audit 由 Loki / Prometheus 长期保留,**不**在 CRD status 内 |
| **NPUVerticalScaler 删除 race**(controller 还在 patch · 用户 kubectl delete) | finalizer 阻塞 delete · 但若 patch 进行中可能 leave inconsistent state | finalizer logic:删除前 set `status.conditions[Active]=False reason Deleting` · 等下一 reconcile tick 退出 patch 路径后 remove finalizer;Phase 8 简单实现 — Phase 9 再 harden |
| **同一 ModelService 被多个 NPUVerticalScaler manage**(运维误配) | scaler 互相 patch · 决策抢占 | annotation `vertical-scaler-managed=<name>` 值唯一 check;冲突 scaler `status.Active=False reason TargetAlreadyManaged` + Event;运维责任 |

---

## 7. Forward notes(Phase 9-10)

> 🆕 **Phase 9 候选**:多租户 fair scaling policy(读 NPUVerticalScaler.status + Quota CRD)+ Prometheus PromQL custom metric extension(`spec.metric.type=PrometheusQuery` enum 值)。Phase 8 ships only NPUUtilization built-in metric · Phase 9 加 PrometheusQuery 时 PromQL 表达式带 namespace + model_service label 自动注入(与 Phase 8 NPUUtilization 同 scoping)。

> 🆕 **Phase 9 候选**:Karmada 多站点 federation 时,NPUVerticalScaler.status.scaleHistory 由本地控制平面 own,fair scaling decision 由 federation 控制平面统一(每站点 NPUVerticalScaler 是 propagated 副本)。Phase 8 单 cluster · 不预占 federation schema。

> 🆕 **Phase 9 候选**:Volcano gang-scheduling 整合(P8-T-106 spike)— 训练 job 场景 PodGroup CRD 需要 gang,推理 ModelService(Phase 8 demo)single-replica or multi-replica 都不需要 gang。NPUVerticalScaler 不参与 gang 决策。

> 🆕 **Phase 10 候选**:Partition-aware allocator + NPUVerticalScaler 协同 — Phase 8 T101+T102 BETA-GATED 路径若 light up,Partitionable Devices 让 NPUVerticalScaler decision 可选 partition slice(不仅 whole NPU)· allocator.AllocateBundle 内部自动 prefer sibling partition for HCCS locality(per spike §5)。NPUVerticalScaler 端无 schema 变化。

> 🆕 **Phase 10 候选**:真硬件 lab smoke(P8-T-105 LAB-CONDITIONAL)验证 rolling restart 时间 + KV cache loss 影响 + HCCL rank rejoin 时间;若 latency 严重 → Phase 10 评估 KV cache warmup / preload 优化(超出 ADR-0012 范围)。

> 🆕 **Phase 11+ 候选**:Live HCCL rank migration / KV cache transfer — CNI level + vllm engine level 需要 upstream 支持;一旦 upstream(vllm-ascend / Mooncake / Cilium)暴露 live migration API,本 ADR §1 重启切片 commitment 升级为"on-line + restart fallback" pattern。Phase 8 重启切片 backward-compat 保留。

---

## 推翻条件

| 决策项 | 推翻条件 |
|---|---|
| 重启切片 pattern 是 Phase 8 deliverable | 上游(vllm-ascend / CANN / Mooncake)在 Phase 8 内暴露 live re-partition API → follow-on ADR Phase 11+ 升级路径(重启切片 保留 backward-compat) |
| NPUVerticalScaler 命名空间作用域 | Phase 9 multi-tenancy 引入 → namespace = tenant boundary 已经满足;若多 tenant 共享同一 ModelService 路径出现(Phase 11+)→ 重新评估 cluster-scoped + TenantRef field |
| API group `inference.ocloud.edge.example.com/v1alpha1` | Phase 9+ ModelService CRD 重大 schema 演进 → bump v1beta1;`inference.ocloud.edge.example.com` group 不变 |
| `MetricType` 仅 `NPUUtilization` enum | Phase 9 引入 PromQL custom metric → 加 `PrometheusQuery` enum 值;原 NPUUtilization backward-compat |
| `ScaleSliceSpec` 仅 `busy/idle` 二态 | Phase 9 引入"mid"层 → 加 `midTemplateName` field + middleThreshold;Phase 8 二态足够 demo |
| 通过 annotation 提示 + 运维 ignoreDifferences 处理 GitOps 冲突 | 出现 ≥ 3 个 production incident 因 GitOps 反复 revert scaler patch → 引入 admission webhook ownership transfer(Phase 9 mass-multi-tenancy 时一并考虑) |
| Phase 8 不引入 admission webhook | NPUVerticalScaler 冲突或 GitOps drift 引发产品级 incident → Phase 9 一并引入 |
| Pod recreation 通过 inference-operator 现有 reconcile 链触发(不直接 delete Pod) | inference-operator ModelService 控制器迟未观察 spec change(reconcile lag > 60s)→ 评估是否在 NPUVerticalScaler 加 annotation force-restart trigger;Phase 8 不需要(reconcile lag 典型 < 5s) |

---

## 引用

- `docs/adr/0001-phase0-key-decisions.md` v3 — 双轨路径基线
- `docs/adr/0002-no-kserve.md` — 推理服务全部基于 vllm-ascend Deployment + inference-operator · 不引入 KServe(NPUVerticalScaler co-located with inference-operator)
- `docs/adr/0008-pd-router-webhook.md` — PD Router webhook 与 NPUVerticalScaler 共存于 inference-operator binary
- `docs/adr/0009-npu-dra-driver.md` §4(Partitionable Devices forward · Phase 10 partition-aware allocator 协同 NPUVerticalScaler)§6(Allocator)
- `docs/adr/0010-scheduler-plugin.md` §7(Phase 7+ forward notes · Phase 8 busy-idle reconcile loop cross-ref)§288(Binpack 默认 disabled 推翻条件 · "Phase 8 vertical scaling 需要 binpack 才能腾空闲节点 → 默认 enabled 同时上调 NPU 权重")
- `docs/adr/0011-npu-dynamic-slicing-and-source-interface.md` §1(NPU 动态切分 · NPUSliceTemplate substrate)§4(NPUSliceTemplate CRD schema · NPUVerticalScaler.spec.scaleSlice 引用)§1 後果 row "Phase 8 vertical scaling reads NPUSliceTemplate.status to determine 重启切片 trigger" cross-ref 本 ADR §"Scaling decision flow"(§3)
- `docs/checkpoint-phase7.md` §6(Phase 8 handoff brief · NPUSliceTemplate substrate carry-forward · AllocateBundle 控制器 wiring deferred to Phase 8 T008)
- `docs/phase8-plan.md` §1 scope summary / §2 task overview / §3 W1 task packages T001-T008 / §4 W2 task packages T101-T107 / §5 DoD / §6 risks
- `docs/architecture.md` §1.3(Phase 路线图 · Phase 8 "忙闲时垂直伸缩")§3.4(NPU/AI 运行时表 · NPUVerticalScaler forward ref)§13(Phase 8 row "调研,可能只支持横向,垂直走'重启切片'")
- `docs/research/k8s-partitionable-devices-spike.md`(P7-T-106 落地 · KEP-4815 1.36 Beta confirmed · GA unconfirmed · Phase 8 partition-aware allocator BETA-GATED 路径)
- `docs/cni-hccl-research.md` §5(live migration CNI-level gap · 重启切片 pattern 绕开此 gap)
- upstream:K8s HPA pattern(NPUVerticalScaler patch sliceTemplate ref · HPA patch replicas · 字段解耦)
- upstream:`sigs.k8s.io/controller-runtime`(NPUVerticalScaler controller reconcile loop · ServerSideApply / MergePatch)

---

**END of ADR-0012**
