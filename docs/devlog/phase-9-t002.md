# P9-T-002 · ADR-0014 Multi-tenant Quota design freeze

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 0.5d planned · ~0.4d actual

## Intent

ADR-0014 锁定 multi-tenant Quota + fair scaling 的 Phase 9 design freeze — Scope unit(namespace-scope only · cluster-scope Phase 10)· Quota CRD shape(`ocloud.edge.example.com/v1alpha1, kind: Quota` + maxSliceAllocations + maxScaleEventsPerWindow + maxNPUSliceTemplateRefs whitelist)· Enforcement points(TWO ValidatingAdmissionWebhooks colocated with inference-operator binary · Webhook A on NPUSliceAllocation create · Webhook B on NPUVerticalScaler.spec patch)· Status sync(60s tick + 5s cache TTL fallback)。是 Phase 9 安全模型 spine 与 ADR-0013 O2 DMS 并列 W1 双脊柱。

## Path adaptations

- **ADR-0009 §6.4 不存在**:plan §3 P9-T-002 Allowed Paths 列 "§6.4 migration table 加 'Phase 9 Quota CRD admission validates NPUSliceAllocation create' 行"。实际 ADR-0009 §6 是 "Allocator 算法详细"(line 102)无 §6.4 nested。**Path adapt**:加段到 ADR-0009 §2(slice ↔ ResourceClaim operative 表)末尾(line 51 之后)作 "Phase 9 Quota admission cross-ref" 段落 — §2 才是真正的 operative migration table · 与 plan 意图最契合。
- **ADR-0011 §1 後果 row 不存在**:plan 列 "§1 後果 row 加 Quota CRD substrate interaction cross-ref"。实际 ADR-0011 §1 "NPU 动态切分"是 decision section · 无 nested 後果 subsection · 总 §後果(consequences)在 §225-249。**Path adapt**:在 §1 末尾(line 39 "关键不变量"后)加 "Phase 9 Quota substrate interaction" 段 — NPUSliceTemplate name 是 Quota CRD `maxNPUSliceTemplateRefs` whitelist 的引用 unit · 与 plan 意图一致。
- **ADR-0012 §3 Consequences 不是 ADR-0012 的 Consequences section**:plan 列 "§3 Consequences 加 ..."。实际 ADR-0012 §3 是 "Scaling decision flow" decision section · 实际 Consequences 在 §6 "后果"(line 352)。**Path adapt**:plan 意图是更新 §3 内 "**多租户 Phase 9 forward note**" 段(line 105) status 的 ADR-0014 cross-ref · 比 §6 Consequences 更精准(那是 forward note location)。
- **ADR-0012 §7 forward notes 多租户 row status flip**:plan acceptance "§7 forward notes update flips 'Phase 9 fair scaling policy' 行 status" — 直接 inline status update 到 §7 第一行 "🆕 **Phase 9 候选**:多租户 fair scaling policy ..." 末尾(加 "Status update 2026-05-21 · ADR-0014 落" 后缀)· 不改原有 forward 表达。

## Debugging trail

- **NPUSliceAllocation create 路径与 claim_controller 内部 emit 的兼容性**:Phase 5 P5-T-005 audit lifecycle 是 claim_controller 内部 emit NPUSliceAllocation create · Webhook A `failurePolicy=Fail` Reject 会阻塞 controller reconcile。**决策**:Webhook A 设计 = 拦截所有 create source(用户 / O2 NB / claim_controller)— 这是预期 security boundary 行为 · 但 claim_controller 必须能 graceful handle reject(走 Pod scheduling backoff 而非 reconcile 死锁)。P9-T-006 acceptance §3 risk row 显式记录此约束 · Phase 11+ 候选 claim_controller pre-check Quota 避免 admission round-trip(§7 forward note)。
- **Webhook B 拦截点选择 — NPUVerticalScaler.spec patch vs ModelService.spec.template.sliceTemplate patch**:ADR-0012 §5 mutation model 显示 NPUVerticalScaler controller patch 的是 ModelService(per Phase 8 mutation adaptation note in arch §13 — "annotation vs spec.template.sliceTemplate")。**决策**:Webhook B 拦 **NPUVerticalScaler.spec patch**(user / O2 NB-触发 scale rule change · 是 scaler-driven scale event 的上游入口)而非 ModelService patch(下游 controller-internal 动作)· 避免与 PD Router webhook chain 顺序冲突 + 避免误拒 kubectl edit ModelService 路径。详 §2 Decision C "为什么 Webhook B 拦 NPUVerticalScaler.spec patch 而非 ModelService.spec.template.sliceTemplate patch" 段。
- **Colocated vs new binary 决策**:plan §3 P9-T-002 Decision C 显示 "default = colocated with inference-operator binary for Phase 9 simplicity OR new module `operators/quota-controller/`"。**决策**:colocated · 3 个理由(cert-manager 重用 P5-T-101 / single leader-elect / 故障域隔离成本 < demo scale 收益) · Phase 10 polish split 路径开放 · §6 Open question (a) 留判据(webhook reject 性能 > 100ms · controller reconcile race)。
- **scaleHistory sum vs event-driven counter**:ADR-0012 §4 status.scaleHistory 是 rolling 10-entry FIFO · oldest evicted。**风险**:windowSeconds > 1h + maxScaleEventsPerWindow > 10 production scenario 内 quota usage 计数低估。**决策**:Phase 9 接受 scaleHistory-based sum(demo scale 内 ≤5/hour 业务 reasonable) · §3 risk row + §7 forward note 路径替换 event-driven counter at Phase 10 polish。
- **K8s ResourceQuota orthogonal not replacing**:Quota CRD 与 K8s 原生 ResourceQuota(generic CPU/mem/pod quota)语义独立(NPU-aware vs generic)· 同 namespace 可同时设两端(K8s ResourceQuota + 本 Quota)互不替代。§1 Context 明示 + §引用 upstream K8s ResourceQuota 标注 orthogonal。

## Key decisions

- **namespace-scope only Phase 9**(§2 Decision A · 推翻条件锁 Karmada federation Phase 10 落):匹配 K8s ResourceQuota 心智模型 · namespace = tenant boundary 与 ADR-0012 §6 推翻条件 spirit 一致。
- **2 ValidatingAdmissionWebhooks**(§2 Decision C · A on NPUSliceAllocation create · B on NPUVerticalScaler.spec patch):覆盖 multi-tenancy 安全模型两侧入口 — 分配数限额 + scale event rate cap。
- **colocated with inference-operator binary**(§2 Decision C · cert-manager 重用 P5-T-101 · §6 Open question (a) Phase 10 split 路径开放):Phase 9 W1 scope tractable;不引入新 binary 复杂度。
- **5s webhook cache TTL + 60s controller tick**(§2 Decision D · §3 risk row + §6 Open question (d) strong-consistency 模式 Phase 10):burst create 内允许 +N 误差 · demo scope acceptable trade-off。
- **fail-open on Quota NotFound**(§2 Decision C step 2):admission webhook 通行 default · 避免 webhook 故障锁死 namespace · production gap 走 Phase 10 polish ClusterQuota default fallback。
- **sliding-window count not token-bucket**(§3 Consequences · §6 Open question (e) Phase 10):简单 demo 友好 · token-bucket additive Phase 10 polish 路径开放。

## Verification

P3 三项验证维度:

| 维度 | 验证方法 | 结果 |
|---|---|---|
| **存在性** | `wc -l docs/adr/0014-multi-tenant-quota.md docs/devlog/phase-9-t002.md` · grep 5 cross-ref 文件命中 | ADR-0014 ~310 lines · devlog ~60 lines · 5 cross-ref edits 全部 land(arch §6.7 + §6.8 + §13 安全模型 row + ADR-0009 §2 + ADR-0011 §1 + ADR-0012 §3 + §7) |
| **完整性** | phase9-plan §3 P9-T-002 acceptance 7 项(§1 Context · §2 ABCD · §3 Consequences · §4 CRD schema · §5 Enforcement contract · §6 Open questions · §7 Forward notes)逐项核对 | 7/7 全覆盖 · §1 cites 5 sources(Phase 8 checkpoint §6 #2 + arch §6.7 + arch §13 安全模型 + Phase 5 NPUSliceAllocation 8173e83/c283e94 + Phase 8 NPUVerticalScaler 45775bc)· §6 Open questions 6 项(a-f · 加 (e) token-bucket · (f) frontend visualisation 超出 plan list 4 项 — 主动加 2 项 forward visibility) |
| **正确性** | grep `ADR-0014` / `0014-multi-tenant-quota` 全 docs 仓库 + ADR-0013 + ADR-0014 双向 cross-ref 闭合 + Webhook B 拦截点解释自洽(NPUVerticalScaler.spec patch ≠ ModelService.annotation patch) | grep 命中 7 文件(本身 + arch + ADR-0009 + ADR-0011 + ADR-0012 + devlog + phase9-plan pre-existing)· bi-directional 闭合 · Webhook 拦截点 §2 Decision C 明示理由段 |

P4 横向 grep `Quota` 全仓库 → 命中 ADR-0001/0009/0011/0012/0013/0014 + arch.md + phase9-plan + checkpoint-phase8 等 · 无 stale ref / 无遗漏 cascade。

## Carry-forward

- **P9-T-005(W1 CRD types + scheme + samples)** 起草时:ADR-0014 §2 Decision B + §4 CRD schema Go types skeleton 直接用作 quota_types.go 蓝本 · `operators/inference-operator/api/v1alpha1/quota_types.go` 落地 · sample `config/samples/ocloud_v1alpha1_quota.yaml` 用 §2 Decision B YAML form;`maxSliceAllocations=8` + `maxScaleEventsPerWindow{count=5, windowSeconds=3600}` + `maxNPUSliceTemplateRefs=[qwen-pd-busy, qwen-pd-idle]` Phase 8 sample 风格匹配
- **P9-T-006(W1 controller + admission webhook body)** 起草时:§2 Decision C Webhook A/B logic + §5 Enforcement contract Webhook ManifestEvent YAML 直接用 · 6 webhook test cases(under-quota allow + at-cap reject A + at-cap reject B + stale cache fallback Get + template whitelist allow + template whitelist deny)对齐 §2 Decision C decision logic
- **P9-T-007(W1 PromQL custom metric extension)** 起草时:ADR-0014 §7 forward note 未触及 PromQL — P9-T-007 是 ADR-0012 §7 PromQL forward 独立 task · 但 Quota CRD `maxScaleEventsPerWindow` 不依赖 metric type · PrometheusQuery metric 与 NPUUtilization metric 共享同 Webhook B 拦截路径
- **P9-T-008(W1 O2 DMS scaffold)** 验证 ADR-0014 §1 Context "O2 NB 注入 ModelService 走同一 K8s API server admission chain · 自然过 Quota webhook" 在 T104 body landing 时通过 envtest 验证(O2 NB injected ModelService 触发 NPUVerticalScaler.spec patch 时 Webhook B 拦截)
- **Phase 10 polish** workstreams:cluster-scope ClusterQuota + Karmada cross-cluster propagation + strong-consistency webhook mode + token-bucket algorithm + frontend visualisation + event-driven status.usage sync · 6 项独立 · 走 Phase 10 plan 起草拆任务

---

**END of P9-T-002 devlog**
