# P13-T-002 · ADR-0024 真硬件 lab 激活 + Bucket B 架构

- **Commit**: (this commit · Phase 13 W1)
- **Date**: 2026-06-03
- **Duration**: plan 1d vs actual ~0.6d(pure docs · grounded in code read)

## Intent

承 ADR-0023 §2 Decision C(lab-gating 翻转)展开 Bucket B 真硬件 body 的架构:5 真体(real-Ascend Source / 真 telemetry / 真拓扑聚合 / 真 Deploy / 真推理)+ decoupling-seam invariant + 真硬件集成 fallback。ADR-0024 是 T101-T105 的架构 gate · supersede ADR-0011 §3 连续 6 phase 的 lab-gating default-defer。

## Path adaptations(P3 verify-before-claim · 读真实代码再写架构)

- **ADR-0011 §2 文档契约 vs 实际 stub 签名不一致**:ADR-0011 §2 写 `List() ([]Device, error)` / `Watch() (<-chan SourceEvent, error)`;实际 `realascend.go` 是 `List(ctx) ([]source.NodeDevices, error)` / `Watch(ctx) <-chan source.Event`(无 error)。**ADR-0024 §1.2/§2.2 引用实际签名**(NodeDevices / Event)· 不照抄 ADR-0011 §2 旧契约文字。
- **plan 说 "复用 npusmi/parse.go(`ParseTopoMatrix`)"** —— grep-verified:`parse.go` 的 `ParseTopoMatrix(in) ([]TopoEntry, error)` 已**完整实现 + parse_test.go 5 case 测**(connected-components → Ring · sidecar `# numa:`/`# health:` hint)· `npusmi.Client` 接口(QueryTopo/QueryDeviceInfo/QueryHealth)+ FakeClient/ExecClient scaffold 也已 ship。→ ADR codify "T101 = 接线 ExecClient 而非重写 parser",修正 "从零起" 的误判。
- **CGO/arm64 红线**(plan §9):DCMI cgo `libdcmi.so` binding 会断 `CGO_ENABLED=0 GOARCH=arm64` 交叉编译。ADR §4(c)codify build-tag 隔离(`//go:build dcmi` 真体 vs stub-tag)· 且标注**优先 exec 路径**(ExecClient shell-out npu-smi/dcmi CLI · 无 cgo · 无需 build-tag)· cgo 仅当 DCMI 必须走 .so 时。这是 T101/T102 spec 的硬约束。

## Key decisions

- **§2 Decision G decoupling-seam invariant** 是本 ADR 的架构灵魂:把 build-doc §0.1 "不 fork 代码" + ADR-0023 §2 Decision E(demo 验证台)落成可 grep 的 enforcement —— real 逻辑只在 Source 缝下 · 缝上禁 `if real {}` · demo profile 回归不破 = 每 task 离线层必含点。
- **policy 翻转 ≠ 删 policy**(P2 边界):ADR-0011 §3 lab gating *机制*(default mock-json + opt-in + CI 不依赖 lab)保留 · 翻转的只是 real 版 default outcome(defer → light-up)。ADR-0011 §3 加 🟢 LAB ACTIVATED note 而非删表。
- carry tally 表加 6th carry(Phase 12)+ Phase 13 ACTIVATED 终行 · 让 6-phase defer → flip 的轨迹在 ADR-0011 内完整可读。

## Verification(strict · per-task · 离线层 = pure docs · 代码 grep-verified)

- **存在性**:`docs/adr/0024-real-hardware-activation.md` 写入(§1-§5 · Decision A-G 7 项)· `git diff --stat` 范围 = ADR-0024(新)+ ADR-0011(§3 flip)+ architecture.md(2 cross-ref)+ devlog(新)· 匹配 Allowed Paths 无越界。
- **正确性 / 代码 ground**(P3):ADR 引用的实际签名(NodeDevices/Event/Config.Mode)+ npusmi package(Client/ParseTopoMatrix/FakeClient/ExecClient/ErrNoCommand/ErrParse)+ TopoEntry/DeviceInfo 字段 均 Read-verified(本 session 读 realascend.go + npusmi/client.go + parse.go)。
- **横向一致**(P4 纵向级联):"lab 翻转 / 6-phase default-defer 期结束 / ADR-0024" 在 ADR-0024 + ADR-0011 §3(table + flip note)+ architecture.md(Phase 7 row + 昇腾真机 risk row)四处同步;ADR-0023 §2 Decision C ↔ ADR-0024 双向 cross-ref 闭合。
- markdown 表格列数核(ADR-0011 carry tally 加 2 row 列数 = 既有 4 列)。

## Carry-forward

- T101 spec 必含:ExecClient 接线 + build-tag 隔离(§4(c))+ config validation(npu-smi binary)+ captured-fixture table-test(parser narrow → lab 输出偏离则扩 + 补 test)+ demo profile 回归点(§2 Decision G)。
- T103 软依赖 T101(真 ResourceSlice attr)· 但可对 expected attr schema(`npu.huawei.com/hccs_ring`+`numa_node`)先开发。
- **T002 entry-check 未决留 T101 起手**(§4(a)):lab K8s 版本决定 Go v1 ResourceSlice migration 是否随 T101 —— **执行 session 无 lab 访问 → 默认按 v1beta1 shim 开发**(captured-fixture 不依赖集群版本)· lab 版本由用户在真机环节确认。
- 真机层(§3):所有 Bucket B task 真机 stamp 由用户 lab 完成或标 lab-driver-gated;本 session 只做离线层。
