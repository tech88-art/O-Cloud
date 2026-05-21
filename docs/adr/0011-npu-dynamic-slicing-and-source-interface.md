# ADR-0011: NPU 动态切分 (多模板组合 fallback) + Source 接口抽象 + Lab gating 政策 (Phase 7)

- **状态**:Accepted (design freeze; implementation P7-T-002..T008 + P7-T-101 lab-conditional)(2026-05-20 — Phase 7 P7-T-001)
- **日期**:2026-05-20
- **决策者**:协调者(用户)
- **相关**:ADR-0001 v3(双轨路径)/ ADR-0009(npu-dra-driver · slice ↔ ResourceClaim 映射 + Partitionable Devices forward)/ ADR-0010(scheduler-plugin · HCCS Score adjacency forward)/ `docs/checkpoint-phase6.md` §7(Phase 7 handoff brief — 7 candidate workstreams)/ `docs/phase7-plan.md`(15 tasks W1+W2)/ `docs/architecture.md` §13(Phase 7 review row "动态切分若 fallback 多模板组合 削弱设计目标 → ADR 明确触发条件")/ §3.4(NPU/AI 运行时:动态切分 = 自研 DRA Driver)/ `docs/cann-driver-matrix.md`(lab-conditional entry gate)

---

## 上下文

Phase 6 (`phase-6-complete` @ 4b5acbe + T102/T103 post-tag polish) 落地了 HCCSTopologyPlugin Filter+Score、NumaAffinity placeholder、Binpack opt-in、scheduler-plugin Helm chart、inference-operator Prometheus metrics、vllm-ascend PD proxy_server schema substrate、kind smoke E2E 扩展。Phase 6 checkpoint §7 列出 7 项 carry-forward,Phase 7 plan 选定 5 项做为 W1+W2 落点(剩 2 项 — Karmada 多站点 / Volcano gang — 推到 Phase 8+)。本 ADR 锁定 3 个 Phase 7 关键设计点:

1. **NPU 动态切分**:arch §13 Phase 7 row 标"驱动层突破不在控制范围",同 row 缓解策略写"准备 fallback:多模板组合"。同 §13 评审追加段第 8 行明确"Phase 7 动态切分若 fallback '多模板组合' 削弱设计目标 → Phase 7 启动前 ADR,明确触发 fallback 的条件"。**本 ADR 即该 ADR**。
2. **Source 接口抽象**:ADR-0009 §4 Partitionable Devices forward 与 ADR-0010 §7 第 1 行 "替换 npu-dra-driver `Source.RealAscend.queryTopology()` · 走 `npu-smi info -t topo` 解析 HCCS group 拓扑" 均把 RealAscend 标为 forward · Phase 6 npu-dra-driver 实际硬编码 `mockJSONSource` 读 `configs/mock-data/set-a-small/`。Phase 7 引入 `Source` Go 接口 + factory 让 mock 与 real 两条路径并存,unblock lab-conditional T101。
3. **Lab gating 政策**:Phase 7 是迄今 calendar uncertainty 最大的 phase(per phase7-plan §6 "Top 3 risks" 第 1 项 "Lab access timing dominates Phase 7 calendar uncertainty")。本 ADR 明确 T101 + T104-real-cluster 的 ship vs defer 决策政策,让 main agent 在 W2 entry 时不必再就政策本身澄清。

---

## 决策

### 1. NPU 动态切分:多模板组合 fallback 是 Phase 7 deliverable

**承诺范围**:

- Phase 7 **commit** 落多模板组合 fallback 路径作为 deliverable(对应 arch §13 缓解策略)。
- 驱动层突破(昇腾团队真切分能力暴露)是 **opportunistic upside**,不是 Phase 7 blocker。期间若 NPU SDK / DCMI / npu-smi 暴露真切分 API → 在 Phase 7 W2 加 follow-on ADR 升级路径;若否 → fallback 路径承担演示。
- **Fallback 触发条件**(本 ADR 落 arch §13 的 "明确触发条件"):
  - 驱动层 ≤ 24.x(对照 `docs/cann-driver-matrix.md` baseline)不暴露 partition / dynamic shard API → fallback 强制启用
  - K8s 集群 < 1.37 (Partitionable Devices not GA) → fallback 强制启用
  - 用户在 Phase 7 W1 entry 显式标"先走 fallback / 暂不 chase 真切分" → fallback 启用
  - 上述 3 条全反 → 触发 follow-on ADR,Phase 7 W2 评估升级路径

**Fallback 机制语义**:

- 新增 `NPUSliceTemplate` CRD 表达"用户想要的复合切分"(如 `1× vir04 (prefill) + 1× vir08 (decode)`)。CRD schema 见 §4。
- 新增 template engine(`operators/npu-dra-driver/internal/template/`)接受 NPUSliceTemplate,经 `Validate()` + `Decompose()` 输出 `FixedTemplateBundle{Items []{Template string; Count int}}` — 即把 composition 拆解为 ResourceClaim 已知能消费的 *现存固定模板* 组合(vir04 / vir08 / vir16 / whole)。
- Allocator 扩展(P7-T-105)读 Bundle 后按既有 NPU 级别 allocate 多次,每次对应 Bundle 中的一个 item;保持 Phase 5 greedy first-fit + Phase 6 topology-aware hint 不变;仅"per claim 多 device 分配"被拆成"per bundle item 多 claim";状态写回 NPUSliceTemplate.status.conditions[Allocatable]。
- **关键不变量**:不带 `npu.huawei.com/slice-template=<name>` label 的 Pod 走 Phase 5 既有 whole-NPU 路径,不感知 NPUSliceTemplate 存在。NPUSliceTemplate 是 opt-in,不是替换。

**Phase 9 Quota substrate interaction(2026-05-21 · ADR-0014 / P9-T-002)**:NPUSliceTemplate cluster-scoped name 是 Quota CRD `spec.enforcement.maxNPUSliceTemplateRefs []string` whitelist 的引用 unit · cluster admin 可显式限定某 namespace 仅允许引用列表内 NPUSliceTemplate(NPUVerticalScaler.spec.scaleSlice.{busy,idle}TemplateName 必须 IN whitelist 否则被 Quota Webhook B Reject reason=TemplateRefNotAllowed)· 防止跨 namespace 越权引用 · NPUSliceTemplate substrate 端无 schema 变化 · 仅作为 Quota whitelist 引用对象。详 ADR-0014 §2 Decision B `maxNPUSliceTemplateRefs` 字段 + §2 Decision C Webhook B logic。

### 2. Source 接口抽象

**Go 接口契约**(P7-T-004 落地):

```go
package source // operators/npu-dra-driver/internal/source

// Source abstracts where the npu-dra-driver gets its inventory and topology.
// Phase 4-6 uses MockJSONSource (reads JSON from configs/mock-data/set-a-small/).
// Phase 7 introduces RealAscendSource (calls npu-smi / DCMI on real silicon),
// lit up by lab-conditional P7-T-101.
type Source interface {
    // List returns the full device inventory observed by this source.
    // Caller is publisher.reconcileLoop — emits ResourceSlices.
    List(ctx context.Context) ([]Device, error)

    // Watch returns a channel of SourceEvent (Added / Modified / Deleted).
    // Caller is publisher.reconcileLoop — triggers per-event reconcile.
    Watch(ctx context.Context) (<-chan SourceEvent, error)

    // QueryTopology returns the HCCS ring + NUMA topology for a single node.
    // Caller is pool-operator NPUPool.status.hccsTopology aggregator
    // (Phase 6 P6-T-003) — populates PeerGroups[].
    QueryTopology(ctx context.Context, nodeName string) (*HCCSTopology, error)
}

// SourceEvent + Device + HCCSTopology types live in this package;
// MockJSONSource (internal/source/mockjson/) and RealAscendSource
// (internal/source/realascend/) implement Source. Factory in
// internal/source/factory.go selects via --source-type flag (default mock-json).

var ErrNotImplemented = errors.New("source: not implemented")
```

**MockJSONSource**(P7-T-004 落地):

- 包名:`internal/source/mockjson/`
- 行为 **bit-for-bit 保留** Phase 4-6 既有 `mockJSONSource` 逻辑(读 `configs/mock-data/set-a-small/{nodes,npupools,slices}.json`,emit cadence 不变,ResourceSlice attribute 写法不变)。
- 现有 Phase 4-5 publisher integration tests 必须 pass unchanged → 这是 P7-T-004 验证 gate。

**RealAscendSource**(P7-T-004 stub + P7-T-101 lab-conditional 真实现):

- 包名:`internal/source/realascend/`
- W1 stub:所有方法返回 `ErrNotImplemented`;通过 factory 注册可选,但若 chart values `sourceType=real-ascend` 误选 → DaemonSet 启动 emit warning + reconcile loop noop(non-blocking)。
- W2 lab-conditional T101 真实现:`List` 经 npu-smi 查 device inventory;`Watch` 通过 DCMI health poll;`QueryTopology` 跑 `npu-smi info -t topo` 文本解析(P7-T-005 提供 parser)。

**Factory + flag**:

```go
// internal/source/factory.go
func NewSource(sourceType string, cfg Config) (Source, error) {
    switch sourceType {
    case "mock-json":
        return mockjson.New(cfg)
    case "real-ascend":
        return realascend.New(cfg)
    default:
        return nil, fmt.Errorf("source: unknown type %q (want mock-json | real-ascend)", sourceType)
    }
}
```

- `cmd/main.go` 加 `--source-type` flag 默认 `mock-json` → 现 Phase 4-6 helm chart 升级零行为变化。
- Helm chart values `sourceType: "mock-json"` 默认;lab 时改 `real-ascend` 并同时打开 hostPath mount(P7-T-101)。

**Phase 7 不动**:ResourceSlice attribute schema 不变(ADR-0010 §5 表);DeviceClass 名不变(`npu.ocloud.edge.example.com`);ResourceClaim 消费契约不变。Source 抽象仅在 publisher 内部生效,对上游(inference-operator)透明。

### 3. Lab gating 政策

**政策核心**:

- T101(Source.RealAscend impl on real silicon)与 T104 内的 real-cluster smoke(synthetic ring fixture 之外的真硬件 smoke)**当且仅当** 用户在 Phase 7 W2 entry meeting 中明确"lab access available in window"时执行。
- **缺省 = 推迟到 Phase 10**(arch §1.3 "真实硬件对接 + 演示打磨" 那一行)。
- T104 内的 synthetic ring fixture (kind smoke 不依赖真硬件那部分) **无论 lab 与否都落地** — 提供 CI hard-assertion baseline,不依赖 lab access。

**Subagent brief 约定**:

- Lab-gated 任务的 subagent brief 必须以 `[LAB-GATED]` 开头。Main agent 派发前根据用户 W2 entry signal 决定是否实际派,不派也不算 skip — checkpoint 单独记录 deferral 与理由。
- Phase 7 tag (`phase-7-complete`) 落地与 T101 状态无关 — `docs/checkpoint-phase7.md` §"DoD reconciliation" 记录 lab-gating actual outcome(landed at YYYY-MM-DD 或 deferred to Phase 10 / 理由)。

**Phase 7 入口决策矩阵**(W2 entry meeting agenda 第 1 项 per phase7-plan §6):

| 用户 W2 entry signal | T101 | T104 real-cluster smoke | Checkpoint 记录 |
|---|---|---|---|
| "lab access available · 在 W2-DX 起 N 天" | 派 | 派 | "T101 landed on real silicon at YYYY-MM-DD; CANN X / driver Y verified" |
| "lab access deferred / Phase 7 窗口内不可得" | skip | skip(synthetic ring fixture 仍落地) | "T101 deferred to Phase 10 per W2 entry meeting YYYY-MM-DD"|
| 无明确信号 (default) | skip | skip(synthetic ring fixture 仍落地) | "T101 deferred to Phase 10 (no lab signal received)" |

**CI 行为**:

- helm chart 的 `sourceType=real-ascend` value 即使被运维误选,DaemonSet 启动也只 emit warning,**不 block** — 让 Phase 10 session 可以直接 light up 而无需先回滚 chart。
- e2e-kind workflow(T103+T104)不依赖 lab,跑 synthetic ring fixture(set-b-multi-ring),hard-assert 兼容 lab 缺席。
- `tests/lab/phase7/` 目录仅在 T101 actually landed 时存在;deferral 路径不落该目录。

---

## NPUSliceTemplate CRD schema(P7-T-006 落地)

```yaml
apiVersion: npu.ocloud.edge.example.com/v1alpha1
kind: NPUSliceTemplate
metadata:
  name: qwen-8b-pd-pair       # 集群范围唯一
spec:
  composition:
    - type: vir04             # enum: whole | vir04 | vir08 | vir16 | dynamic-shard
      count: 1
      aiCoreRequest: 4        # int32; only used when type=dynamic-shard, else informational
    - type: vir08
      count: 1
      aiCoreRequest: 8
  fallbackStrategy: fixed-template-combination
                              # enum: fixed-template-combination (default) | refuse
                              # fixed-template-combination → engine decomposes composition into
                              #   existing vir04/vir08/whole template requests; allocator handles
                              #   per-item allocation
                              # refuse → engine emits Validated=False if composition cannot be
                              #   directly served by existing templates (no decomposition)
status:
  conditions:
    - type: Validated
      status: "True"
      reason: CompositionValid
    - type: Allocatable
      status: "True"          # set to True only after allocator confirms all items can be served
      reason: AllAvailable
  fallbackAppliedReason: ""   # populated when fallbackStrategy=fixed-template-combination
                              # actually fired; format "decomposed into 1×vir04 + 1×vir08"
```

**Go types**(P7-T-006 落地 in `operators/npu-dra-driver/api/v1alpha1/npuslicetemplate_types.go`):

```go
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type NPUSliceTemplate struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec              NPUSliceTemplateSpec   `json:"spec,omitempty"`
    Status            NPUSliceTemplateStatus `json:"status,omitempty"`
}

type NPUSliceTemplateSpec struct {
    // Composition lists the slice parts that together make up this template.
    Composition []TemplatePart `json:"composition"`
    // FallbackStrategy controls what happens when the composition cannot be
    // directly served by existing fixed templates (vir04/vir08/vir16/whole).
    // +kubebuilder:default=fixed-template-combination
    FallbackStrategy FallbackStrategy `json:"fallbackStrategy,omitempty"`
}

type TemplatePart struct {
    // +kubebuilder:validation:Enum=whole;vir04;vir08;vir16;dynamic-shard
    Type           PartType `json:"type"`
    // +kubebuilder:validation:Minimum=1
    Count          int32    `json:"count"`
    // AICoreRequest is only consulted for Type=dynamic-shard; ignored otherwise.
    AICoreRequest  int32    `json:"aiCoreRequest,omitempty"`
}

// +kubebuilder:validation:Enum=fixed-template-combination;refuse
type FallbackStrategy string

type NPUSliceTemplateStatus struct {
    Conditions             []metav1.Condition `json:"conditions,omitempty"`
    FallbackAppliedReason  string             `json:"fallbackAppliedReason,omitempty"`
}
```

**Pod opt-in 契约**:

- Pod 携带 label `npu.huawei.com/slice-template=<NPUSliceTemplate.metadata.name>` → resourceclaim_controller 查 NPUSliceTemplate → Engine.Decompose → 把 Bundle 传给 Allocator;**absent** → 走 Phase 5 既有 whole-NPU 路径(无回归)。
- ResourceClaim 多 device request 沿用 K8s DRA v1beta1 既有 schema;allocator 内部按 Bundle 顺序分配。

**Phase 7 sample compositions**(P7-T-006 落地 in `config/samples/`):

| 样例文件 | Composition | FallbackStrategy | 用例 |
|---|---|---|---|
| `npuslicetemplate_qwen_pd.yaml` | 1× vir04 (prefill) + 1× vir08 (decode) | fixed-template-combination | Qwen 8B PD-pair on a single NPU |
| `npuslicetemplate_deepseek_20b.yaml` | 1× whole | refuse | DeepSeek 20B per-replica,严格模式演示 |

---

## 后果

### 正面

- **arch §13 Phase 7 row "明确触发条件"承诺兑现**:本 ADR 落 fallback 触发条件 + fallback 机制 + opt-in 契约,Phase 7 不再需要在 entry meeting 内现场设计。
- **Source 接口让 Phase 7 → Phase 10 无破坏性升级**:Phase 7 W1 ship interface + stub + factory,Phase 10 / 真硬件期 light up RealAscend body,publisher / pool-operator / scheduler-plugin / inference-operator 全部 0 改动。
- **NPUSliceTemplate 是 opt-in**:Phase 5/6 既有 ModelService 不带 slice-template label → 走 whole-NPU 路径 → Phase 5 + Phase 6 tests 全部 pass unchanged。
- **Lab gating 政策 unblocks main agent 派发决策**:政策在 ADR 内固化 → main agent 不必为 "T101 派还是不派" 反复澄清。
- **NPUSliceTemplate substrate 给 Phase 8 vertical scaling 留出 hook**:Phase 8 busy-idle vertical scaling controller 读 `NPUSliceTemplate.status` 判断"重启切片"触发(arch §13 row Phase 8 "调研,可能只支持横向,垂直走重启切片")。

> 🆕 **2026-05-21 update (P8-T-001 / ADR-0012)**:Phase 8 vertical scaling reads NPUSliceTemplate.status to determine 重启切片 trigger — 详 **ADR-0012 §"Scaling decision flow"**(§3 Decision C reconcile loop)+ ADR-0012 §5 mutation model。NPUVerticalScaler 通过 patch `ModelService.spec.template.sliceTemplate` 字段(指向 NPUSliceTemplate ref)触发 rolling restart;Pod recreation 后 claim_controller(P8-T-008 wiring)按 label `npu.huawei.com/slice-template=<new-template>` 解析 NPUSliceTemplate → Engine.Decompose → AllocateBundle → N allocations。本 ADR §4 NPUSliceTemplate CRD schema 在 Phase 8 期间被 NPUVerticalScaler.spec.scaleSlice.{busyTemplateName, idleTemplateName} 引用,**无 schema 变化**。Pod opt-in label 契约(本 ADR §"Pod opt-in 契约")保留;NPUVerticalScaler 不感知 label 路径,只 patch spec。

### 负面 / 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| Source 接口引入 1 层 indirection(factory + interface 调用) | 可能在 publisher reconcile 热路径上加 overhead | Acceptance gate:微基准 reconcile loop overhead ≤ 5% vs Phase 6 baseline(P7-T-004 验证;numbers recorded in npu-dra-driver/DESIGN.md);Go interface 方法 dispatch + 单 switch,实际 overhead 应远小于 1µs/call |
| Lab access 在 W2 期间临时材料化 | T101 突然 unblocked 但 main agent 已 schedule 其他任务 | 政策定 default = defer,所以"临时来"的情况不影响 Phase 7 tag;若 lab 真来 → main agent 把 T101 插入 W2 余下时段,T101 estimated 2d 单 subagent 可吸收 |
| NPUSliceTemplate fallback 引入额外 controller(template_controller.go) | 增加 npu-dra-driver binary 复杂度 | 复用 controller-runtime · watch NPUSliceTemplate · Reconcile 只跑 Engine.Validate + Engine.Decompose + status update;无 envtest 依赖外资源 |
| 多 NPU device 跨 ResourceClaim 分配的原子性 | Bundle 中部分 item allocate 成功 / 部分失败 → 残留 partial allocation | Allocator(P7-T-105)对 Bundle 走"all-or-nothing":先 dry-allocate 全 bundle items,success 后才 commit;任一 fail → rollback + 返回 Unallocatable;NPUSliceTemplate.status.conditions[Allocatable]=False |
| 用户给 composition 含 `dynamic-shard` type | Phase 7 fixed-template-combination 无法 decompose dynamic-shard(没有对应固定模板) | Validate 阶段 reject `dynamic-shard` + fallbackStrategy=fixed-template-combination 组合;明确 emit Validated=False + reason="dynamic-shard not supported in Phase 7 fallback path; gated on driver-layer breakthrough or KEP-4815" |
| Phase 7 W2 期间 sched-plugins v0.32.x / vllm-ascend v0.12+ 未 GA | T002 / T102 走 doc-only fallback,影响 Phase 7 完成度 | phase7-plan §3-T002 + §4-T102 均预定义 doc-only fallback 路径;Phase 7 tag 不被 block;Phase 8 重新评估 |

---

## 推翻条件

| 决策项 | 推翻条件 |
|---|---|
| 多模板组合 fallback 是 Phase 7 deliverable | 昇腾团队在 Phase 7 W1 / W2 真把 driver-layer 切分能力暴露 → follow-on ADR 升级路径(fallback 保留为 backward-compat) |
| Source 接口契约 (`List` / `Watch` / `QueryTopology`) | K8s DRA v1 GA(K8s 1.34 已发布)或 Partitionable Devices GA(KEP-4815)导致 publisher 必须改成 partition-emitter — 接口可能需要加 `ListPartitions` 方法 → ADR 增订 v2 |
| Lab gating default = defer | Phase 7 W1 末 / W2 头时,lab access 进入项目周期可见的 calendar(运维确认 ≥ 5d 可用) → 同 chat 立即把 T101 + T104-real-cluster 提到 W2 前段,deferral 政策只对"无信号"场景生效 |
| NPUSliceTemplate cluster-scoped | Phase 9 multi-tenancy 引入 → 改 namespaced,加 TenantRef field;Phase 7 不预占 schema |
| FallbackStrategy `refuse` enum | Phase 8 vertical scaling 需要"refuse 但保留 partial allocation 用于 next-cycle replay" → 加新 enum 值;Phase 7 二值足够 |
| Pod opt-in via `npu.huawei.com/slice-template` label | 未来 ResourceClaim spec 原生支持 template reference(K8s DRA v1 演进)→ 切官方字段,label 标 deprecated |

---

## 引用

- `docs/adr/0001-phase0-key-decisions.md` v3 §5 / §7 — 双轨路径 + 不引入 MindCluster 的设计空间
- `docs/adr/0009-npu-dra-driver.md` §2(slice ↔ ResourceClaim 表)§3(KubeEdge gap)§4(Partitionable Devices forward note · 本 ADR §1 cross-ref)§5(Phase 5 实施)§6(Allocator 算法)
- `docs/adr/0010-scheduler-plugin.md` §5(Args schema + ResourceSlice attribute contract)§7(Phase 7+ forward notes · 第 1 行 Source.RealAscend.queryTopology — 本 ADR §2 cross-ref · §256 risk row HCCS Adjacency map empty default — Phase 7 T008 fixes)
- `docs/checkpoint-phase6.md` §7(Phase 7 handoff brief · 7 candidate workstreams · 本 ADR 选定 5)
- `docs/phase7-plan.md` §1 scope summary / §2 task overview / §3 W1 task packages / §4 W2 task packages / §5 DoD / §6 risks / §6 entry meeting agenda
- `docs/architecture.md` §1.3(phase roadmap · Phase 7 row "NPU 动态切分(突破硬模板)" · Phase 10 row "真实硬件对接 + 演示打磨")§3.4(NPU/AI 运行时表 · 动态切分 = 自研 DRA Driver 行)§13(Phase 7 review row "动态切分若 fallback '多模板组合' 削弱设计目标 · 触发条件本 ADR 明确"行)
- `docs/cann-driver-matrix.md`(P4-T-002 落地 · lab gating entry gate for T101)
- `docs/known-issues.md` #11(scheduler-plugin schedulerName opt-in · P7-T-003 closes)
- upstream:KEP-4815 Partitionable Devices(K8s 1.37 est. GA · 本 ADR §3 forward note + P7-T-106 spike)
- upstream:`sigs.k8s.io/scheduler-plugins` v0.32.x(P7-T-002 gating)
- upstream:`vllm-project/vllm-ascend` v0.12+(P7-T-102 gating)

---

**END of ADR-0011**
