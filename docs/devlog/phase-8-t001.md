# P8-T-001 · ADR-0012 Busy-idle 垂直伸缩 + NPUVerticalScaler CRD shape + 重启切片 pattern

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: plan 0.5d / actual ~0.5d (main agent direct per §0a.11 例外 "写 ADR")

## Intent

锁定 Phase 8 的 5 个设计基石,让后续 14 个 task subagent 不需要在 task entry 时再就以下决策做澄清:

1. 垂直伸缩落地形态:**重启切片** pattern 为 Phase 8 deliverable(arch §13 Phase 8 row 调研结论 — NPU vendor stack 不暴露 live re-partition);横向 HPA 走 K8s 标准 off-the-shelf,不是 Phase 8 deliverable
2. NPUVerticalScaler CRD shape:`inference.ocloud.edge.example.com/v1alpha1` · namespace-scoped · 与 ModelService 共 operator binary · finalizer + 4 printer columns
3. Scaling decision flow:reconcile loop 8 步、metric > busy / < idle / hysteresis 三态、cooldown 防 flapping、NoData 不决策
4. Mutation model:patch ModelService.spec.template.sliceTemplate + annotation `vertical-scaler-managed` 提示 + 让 inference-operator 现有 reconcile 链触发 rolling restart(NPUVerticalScaler 不直接 delete Pod)
5. GitOps 协调:annotation 提示 + 运维 ignoreDifferences 指南,Phase 8 不引入 admission webhook(那是 Phase 9 multi-tenancy 时一并考虑)

## Path adaptations

- Plan §3-T001 Allowed Paths 列了 ADR-0009 "§6.4 migration table" cross-reference,但 ADR-0009 实际 §6 内**无 §6.4 子节**(沿用 P7-T-001 devlog 路径适配同样问题)。按 plan intent(让 ADR-0012 在 ADR-0009 内被发现),在 §4 末 Partitionable Devices forward note 增 ADR-0012 cross-ref 段 — 同位置 P7-T-001 已加 ADR-0011 cross-ref,本 commit 紧随其后,layout 一致。
- arch §13 Phase 8 row 状态从"调研,可能只支持横向..."升级为"**in flight via ADR-0012**(2026-05-21 · P8-T-001)— 调研结论 commit ..." — 符合 P3 verify-before-claim(ADR 真的 landed 才升级 row 状态;本 commit 同时落 ADR 主体 + arch 状态更新,无"claim before exist"漏洞)
- arch §3.4 NPU/AI 运行时表后加 🆕 forward note 段(Phase 8 行)— 与现存 v2/v3/🎯/Phase 7 forward note 同列样式
- Edit 工具第一次失败原因 = 我打 ASCII `,` 但 architecture.md 原文用全宽 `，`(中文标点);第二次 Read 原文 verbatim 即过

## Debugging trail

- 无 false start。ADR 体量约 430 行 · 一次成稿。
- 第一次 architecture.md §13 Edit 失败 — String not found · 原因:`,` (ASCII) vs `，` (全宽)。Read 原文 verbatim 后通过。
- 唯一有犹豫的设计点:**NPUVerticalScaler scope**(cluster 还是 namespace)。选 namespace-scoped(Phase 9 multi-tenancy 时 namespace = tenancy boundary 已足够);推翻条件留给 Phase 11+ 共享路径(本 ADR §推翻条件第 2 行明示)。
- 第二犹豫:**FinalizerName** 是否必要(Phase 8 简单 no-op)。落 finalizer + 简单 no-op 让 Phase 9 quota counter dec 走此 hook(平滑升级);Phase 8 spend 少量复杂度换 Phase 9 平滑(本 ADR §2 末说明)。
- 第三犹豫:**多 NPUVerticalScaler manage 同一 ModelService 的冲突处理**。决:annotation 值唯一 check + status.Active=False + Event(非 admission webhook enforce)— Phase 8 文档化运维责任,Phase 9 加 enforcement。
- 第四犹豫:**`MetricType` enum** Phase 8 是否预占 `PrometheusQuery`。决:不预占,Phase 8 enum 仅 `NPUUtilization` · Phase 9 时扩 enum(K8s CRD backward-compat) — 避免 Phase 8 ship 一个无实现的 enum 值。

## Key decisions

- **重启切片 pattern 是 Phase 8 deliverable**(本 ADR §1):明确把 arch §13 Phase 8 row 的"调研结论"固化为 deliverable 形态,不再"调研可能只支持横向"含糊。NPU vendor stack 不暴露 live re-partition 是 P1 数字必有源(CANN ≤ 8.1 / driver ≤ 24.x);workload state(KV cache · model weights · HCCL ranks)的 live transfer 是 P2 边界(CNI-level + vllm engine level 双 gap,详 `docs/cni-hccl-research.md` §5)。
- **NPUVerticalScaler co-located with inference-operator binary**:避免新 operator 模块 proliferation(本 ADR §2 + ADR-0008 PD Router 共存模式)。inference-operator binary 现已有 ModelService controller + PD Router webhook + 现增 NPUVerticalScaler controller — 三 controller 同 binary 同 chart 同 helm release。
- **不直接 delete Pod / 不接管 ModelService spec ownership**(本 ADR §5):mirror K8s HPA pattern(HPA 只 patch spec.replicas · 不接管 Deployment ownership)。NPUVerticalScaler 只 patch spec.template.sliceTemplate · 让 inference-operator ModelService 控制器 + K8s Deployment controller 处理 rolling restart · 两端解耦最大化。
- **annotation 提示 + 运维 ignoreDifferences 指南**(本 ADR §5):产品级 enforcement 是 Phase 9 引入 Quota 时再一并加 admission webhook。Phase 8 文档化约定足够 — K8s HPA 早期也是同样模式(annotation + 运维约束),后续才加 webhook。
- **NoData 不决策**:Ingestor 返回 IngestorResult{NoData: true} 时 stay 当前 template — 避免因 Prometheus 故障引发 false scaling event(本 ADR §1 reconcile loop 第 3 步)。

## Verification

- **存在性**:`docs/adr/0012-busy-idle-vertical-scaler.md` 428 行(含 §上下文 / §决策 §1-3 Decision A/B/C / §4 CRD schema YAML + Go types / §5 mutation model / §6 后果 / §7 Forward notes / §推翻条件 / §引用 9 段)
- **完整性**:`grep ^##\|^### docs/adr/0012-busy-idle-vertical-scaler.md` 输出全部 plan §3 T001 acceptance 段落均存在(§上下文 / §1 Decision A / §2 Decision B / §3 Decision C / §4 CRD schema / §5 Mutation model / §6 后果 / §7 Forward notes / §推翻条件 / §引用)
- **正确性 cross-refs**:`grep -c ADR-0012 docs/**` 结果:
  - `docs/architecture.md`:2 处(§13 Phase 8 row 升级 ✅ · §3.4 forward note 新段 ✅)
  - `docs/adr/0009-npu-dra-driver.md`:1 处(§4 forward note 末加 P8-T-001 update 段 ✅)
  - `docs/adr/0010-scheduler-plugin.md`:1 处(§7 forward notes 表前加 P8-T-001 update 段 ✅)
  - `docs/adr/0011-npu-dynamic-slicing-and-source-interface.md`:1 处(§後果 第 5 行 NPUSliceTemplate substrate hook 后加 P8-T-001 update 段 ✅)
  - `docs/adr/0012-busy-idle-vertical-scaler.md`:6 处 self-references
- **diff 范围**:`git diff --stat` = 4 文件 9 insertions(新增 ADR-0012 主文件 untracked · 4 文件 small edit)— 与 plan §3 T001 Allowed Paths 完全匹配,无越界

## Carry-forward

- **T002**(K8s baseline bump 1.32→1.36)入口需 re-WebFetch KEP-4815 + check kindest/node 1.36 release tracker + check sched-plugins v0.36.x GA · 本 ADR 不影响 T002 决策(T002 主轴是 baseline bump · ADR-0012 无 baseline 依赖);decision 表见 phase8-plan §3-T002 entry decision
- **T005**(NPUVerticalScaler CRD types)按本 ADR §4 Go types 区块**逐字落地**:CRD scope=Namespaced + kubebuilder markers(shortName=npuvs / printcolumns / validation / default)+ FinalizerName 常量 + AnnotationManagedBy 常量都已写明
- **T006**(metrics ingestor)按本 ADR §1 reconcile loop 第 3 步 IngestorResult contract 落地:`{Value: float64, NoData: bool, Err: error}`;Prometheus 不可达 → NoData=true · 上游不决策
- **T007**(NPUVerticalScaler controller)按本 ADR §1 reconcile loop 8 步 + §5 mutation 顺序落地;PatchType=MergePatchType + 同时 stamp annotation + status.scaleHistory FIFO max=10
- **T008**(AllocateBundle controller wiring)按本 ADR §5 Pod label 解析路径落地;NPUVerticalScaler 不参与 wiring 决策 · 只触发 spec change · wiring 是 Phase 7 deferred body 的延续
- **T103**(kind smoke E2E)按本 ADR §1 reconcile loop 在 phase8/assert.sh 写 "metric 触发 → 1 reconcile cycle 内 patch + scaleHistory append" 断言
- **Phase 9 forward note** 已 commit 在本 ADR §7 + §推翻条件 — 多租户 fair scaling / PromQL custom metric / Karmada federation 都已在 schema 留空间(不需 v1alpha1 → v1alpha2 bump · v1alpha1 backward-compat 加 enum 值即可)
