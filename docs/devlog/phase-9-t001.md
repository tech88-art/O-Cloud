# P9-T-001 · ADR-0013 O2 DMS Adapter design freeze

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 0.5d planned · ~0.5d actual

## Intent

ADR-0013 锁定 O2 DMS Adapter 的 Phase 9 design freeze — Profile 选择(K8s only)、NB endpoint shape(HTTP REST `/o2dms/v1` 7 endpoint)、resource reflection 映射(O2 IMS R1 NB ↔ ocloud 6-row table)、implementation 形态(独立 binary `operators/o2-dms-adapter/`)、authn 占位(Phase 9 静态 bearer · Phase 10 polish OIDC)。是 M4 工程化对外 milestone 头条 spine 与 ADR-0014 Quota 并列 Phase 9 W1 落。

## Path adaptations

- **ADR-0001 §对外接口 row 不存在**:plan §3 P9-T-001 Allowed Paths 列 ADR-0001 "small edit if needed — §对外接口 row cross-ref"。实际 ADR-0001 v3 的 13 个 decision 行(§1-§13)无 §对外接口 命名。M4 (不为完整性堆 padding) → 不强行造一行。改为 ADR-0013 §相关 + §引用 把 ADR-0001 §5 K8s baseline(双轨路径)+ §6 Karmada 多站点作为更准确的 cross-ref anchor;ADR-0001 文件本身**不修改**。
- **O-RAN spec 命名实际是 R003-v04.00 而非 "R1 v04.00"**:plan §3 表述 "O2 IMS R1 v04.00 lock" 是 paraphrase。WebSearch 返回 ATIS MVP V2 (2025-02) 引用的实际 spec name 是 `O-RAN.WG6.O2IMS-INTERFACE-R003-v04.00`(R003 = Release 003;"R1" 在 plan 中是 paraphrase · 不是 spec 实际命名)。ADR-0013 §1 Context 用实际 spec name 标注 + plan paraphrase 在括号注释 keep traceability。
- **ADR-0014 forward-ref**:ADR-0013 §3 Consequences 引用 ADR-0014 Quota 协同(P9-T-002 起草)· ADR-0014 文件尚未存在 · §引用 节明示 "Phase 9 W1 起草 · 与本 ADR 并列 Phase 9 spine" · 不破 markdown 链接(用 inline text 而非 markdown link)。

## Debugging trail

- **WebFetch o-ran.org spec portal 失败**:首次 WebFetch `https://www.o-ran.org/specifications` 返回"general info + portal link"无具体 version。再 WebFetch `https://specifications.o-ran.org` 返回 "An unhandled error has occurred" 加载 fail。改用 WebSearch query "O-RAN ALLIANCE WG6 O2 IMS R1 v04.00 v05.00 release" → 10 个结果含 ATIS MVP V2 PDF + Devopedia + ETSI workshop slides + arxiv O-RAN paper · ATIS MVP 是最强二手源 · 引用 `R003-v04.00` + `R004-v07.00.00`。
- **Citation 强度决策**:portal 直接源不可达 · ATIS MVP 二手源可信。ADR-0013 §1 Context 标 `[B · 二手源 ATIS MVP cross-ref]` per P1 (诚实优先)。同时 §5 Open question (a) 明示 T104 body landing 前 main agent re-WebFetch portal 一次 · 若 R004 已成 industry baseline → 评估升级。**不**伪装为 A 级 / 不凭记忆填具体 release 日期(R004 release date 在 ATIS MVP cross-ref 也无明示)。
- **NPUVerticalScaler 是否独立 NB resource**:O2 IMS R1 spec 无 `verticalScaler` 顶层 resource concept · NPUVerticalScaler.status 是 ModelService dynamic 行为 · 决策 inline 到 `deploymentItem.extensions` 字段(O2 spec 允许 vendor extension)而非平行暴露。**理由**:NB 调用方 mental model 是 deploymentItem-centric · 把 scaling 历史挂在 deploymentItem 下符合 spec 隐含语义 + 减少 endpoint 数量。Phase 10 polish 若客户实质需求 → 评估升独立 resource。
- **subscription / alarmEvent 是否 Phase 9 ship**:O2 IMS R1 spec 定义 subscription 机制(NB callback when inventory/lifecycle change)+ alarmEvent · Phase 9 scope 决策 out of scope · 走 polling-mode demo。理由:Phase 9 W2 calendar 4-5 周内吸收 NB API server + inventory + lifecycle 三件已饱和 · subscription/alarmEvent 推 Phase 10 polish。

## Key decisions

- **K8s Profile only**(§2 Decision A · 推翻条件锁定 client 实质化 hybrid stack 需求): 匹配 ADR-0001 v3 双轨路径都是 K8s API · ADR-0002 不引入 KServe → ModelService 是 K8s-native。
- **HTTP REST · `/o2dms/v1` base path**(§2 Decision B): 不破 O2 IMS R1 §3.1 contract · 不引入 gRPC/WebSocket。
- **6-row reflection 映射**(§2 Decision C · phase9-plan 要求 6-8 rows): 5 ocloud CRD 同时被 inventory + lifecycle 消费 · subscription/alarmEvent 表内标 out of scope 留 forward 路径。
- **独立 binary `operators/o2-dms-adapter/`**(§2 Decision D): 职责分离 · 故障域隔离 · 部署灵活 · 故 Phase 10 polish Karmada 多站点路径开放;不与 inference-operator 合并。
- **Auth Phase 9 静态 bearer + Phase 10 polish OIDC**:§5 Open question (c) 路径决定 K8s SA + TokenReview;helm values 默认 helm hook 提示 "DO NOT use static token in production"。
- **lifecycleOperation in-memory queue**(Phase 9 process-local · 1h TTL):接受 single-replica + restart 丢的限制 · Phase 10 polish 走 P9-T-107 cache spike outcome 决定 persistent backing。

## Verification

P3 三项验证维度:

| 维度 | 验证方法 | 结果 |
|---|---|---|
| **存在性** | `ls docs/adr/0013-o2-dms-adapter.md` + `wc -l` 与 ADR-0012 数量级对比 | ADR file written ~280 lines · 与 ADR-0012 (~430 lines) 同量级 · 结构 mirror(§上下文 / §决策 4 块 / §3 后果 / §4 NB endpoint catalog / §5 Open questions / §6 Forward notes / §推翻条件 / §引用) |
| **完整性** | phase9-plan §3 P9-T-001 acceptance 7 项逐项核对(§1 cites · §2 Decision A/B/C/D · §3 Consequences · §4 NB endpoint catalog · §5 Open questions · §6 Forward notes)| 7/7 全覆盖 · §1 cites Phase 8 checkpoint §6 + arch §1.3 + arch §13 + O-RAN spec ref + ATIS MVP cross-ref;§4 catalog 7 endpoint 1:1 与 plan §3 末尾表 + plan §3 P9-T-008 scaffold handler list 对齐 |
| **正确性** | 4 文件 cross-ref 双向闭合(ADR-0013 ↔ arch §5.8 ↔ arch §13 Phase 9 row ↔ ADR-0003 v2 "O2 DMS NB 与 IMS Core 关系" 段)+ 引用源都存在(grep `0001-phase0` `0012-busy-idle` 在 ADR-0013 文本内)| 双向闭合 OK · 引用源全部存在(除 ADR-0014 forward-ref · 标注 P9-T-002 起草) |

P4 横向 grep `o2-dms-adapter` / `0013-o2-dms-adapter` / `ADR-0013` 全仓库 → 命中 arch §5.8 + arch §13 + ADR-0003 v2 + ADR-0013 本身 + phase9-plan(已存在引用)· 无 stale ref / 无遗漏 cascade。

## Carry-forward

- **P9-T-008(W1 scaffold)** 起草时:ADR-0013 §4 NB endpoint catalog 7 endpoint 直接用作 handler stub list;§2 Decision D 实现路径(chi router · informer/lister · helm chart)是 scaffold blueprint
- **P9-T-104(W2 body)** 起草时:T104 body landing 前 main agent 必须 re-WebFetch https://specifications.o-ran.org 一次,评估 R003 vs R004 兼容(§5 Open question (a))· 若 R004 不兼容 → ADR-0013 §1 minor revision
- **P9-T-002(ADR-0014)** 起草时:Quota CRD admission webhook 必须考虑 O2 NB 注入的 ModelService 走同一 K8s API server 路径 — ADR-0014 §5 enforcement contract 不需要为 O2 NB 加 special path · 自然 enforce
- **Phase 10 polish** workstreams:authn/z 完整 OIDC + Karmada multi-cluster + subscription/alarmEvent + R004 upgrade evaluate + lifecycleOperation persistent backing · 五项独立 · 走 Phase 10 plan 起草时拆任务

---

**END of P9-T-001 devlog**
