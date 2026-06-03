# P13-T-001 · ADR-0023 Phase 13 entry decisions(项目收官)

- **Commit**: (this commit · Phase 13 W1 entry gate)
- **Date**: 2026-06-03
- **Duration**: plan 0.5d vs actual ~0.4d(pure docs · clean run)

## Intent

把用户 2026-06-02 chat 两决策(收官视角 "demo 已完成,剩余生产化隔离完成就可以" + 真硬件 lab 已就位)codify 成 entry-gate ADR。ADR-0023 是 Phase 13 其余 15 task 的 decision gate:锁 spine(real 版完整化 + 真硬件点亮)/ milestone(M7 真生产化收口 final · 不规划 Phase 14)/ lab-gating 翻转 / Bucket A/B scope map / demo 验证台定位 / 4 carry 移出核心交付物。

## Path adaptations

- **plan 写 "arch §7 路线图 M7 'Phase 13+' → 'Phase 13' final"** —— 实际 `docs/architecture.md` §7 是 "API 契约草案",**不是**路线图;架构路线图在 §1.3。verify-before-edit(P3):grep `^## 7|路线图|Phase 13+` 确认 architecture.md 唯一路线图是 §1.3 + §13 review-table,"§7 路线图" 实指 **README §7**(README §7 才是路线图表)。故 architecture.md 只改 §1.3(加 M7 row)+ §13(加 Phase 13 in-flight row),§7 路线图改落在 README。未盲改 architecture.md §7(那会污染 API 契约章节)。
- §13 review-table Phase 行有少量乱序(Phase 5+ / Phase 6 row 在 Phase 12 row 之后),Phase 13 row 紧接 Phase 12 row 插入(保持 Phase 12→13 相邻),不动既有乱序。

## Key decisions

- **deliverable 视角 vs workload 视角**:candidate-streams 把 production-hardening cohort 估为 "~3-4 phase"(workload 视角)· 用户以 deliverable 视角收口(填平既有 stub ≠ marathon)· ADR §2 Decision A/B codify 这个 reframe · 明确 "不规划 Phase 14"。
- **M7 不 over-claim**(P2 边界):ADR §2.2 写明 M7 *closes* real 版完整化 + best-effort 真机验证,*不* claim 甲方合同级 SLA / 多区 DR / full IAM(reference-grade right-sized · 详 ADR-0025 §3)。防 P3 gold-plating。
- **4 carry 用表格固化触发条件**(§2 Decision F)· Go v1 migration 的触发条件 = lab K8s ≥1.36(留 ADR-0024 §4 entry-check 决定是否随 T101 迁),不在本 ADR 拍死。
- ADR 编号:0023(entry)· 0024(Bucket B · T002)· 0025(Bucket A · T003)· 连号承 ADR-0019..0022(Phase 12 4 ADR)。

## Verification(strict · per-task · 离线层 = pure docs)

- **存在性**:`docs/adr/0023-phase-13-entry-decisions.md` 写入(§1-§5 全 · Decision A-F 6 项)· `git diff --stat` = 4 文件(新 ADR + 新 devlog + arch + README)匹配 Allowed Paths,无越界。
- **完整性**(P3 三项验证):ADR §2 Decision A-F 全到位(spine / M7 final / lab 翻转 / Bucket A scope / demo 验证台 / carry 移出)· §3 16-task scope cross-ref · §4 3 Open questions + fallback。
- **正确性 / 横向一致**(P4 纵向级联):"不规划 Phase 14" 在 ADR(§2.2 + §5 下游)+ arch §1.3 M7 row + arch §13 Phase 13 row + README §7 M7 row 四处同步 codify · M7 naming "真生产化收口(final)" 四处一致。
- **cross-ref 落地**(P3 verify-before-claim):ADR 引用的 build-doc §4.4/§5 · ADR-0011 §3 · ADR-0016 §2 Decision B · ADR-0019 §2 Decision B · `3765f5c`/`9165ebe` commit 均 Read-verified 存在(本 session 启动已读 build-doc 全文 + plan + kickoff)。
- markdown 表格语法人工核(M7 row 列数 = 既有 M1-M6 row · README/arch 表头一致)。

## Carry-forward

- T002(ADR-0024)承本 ADR §2 Decision C(lab-gating 翻转)展开 Bucket B 5 真体架构 + decoupling-seam invariant + 真硬件集成 fallback;T003(ADR-0025)承 §2 Decision D 展开 Bucket A 5 架构 + right-sizing 声明。
- T302 收官时:checkpoint-phase13 + build-doc §4.4/§5 🔴→🟢 stamp + 项目收官 announcement 必 cross-ref 本 ADR §2 Decision B(M7 final · 无 Phase 14)+ §2 Decision F(4 carry outcome stamp)。
- **执行 session 真机层缺口**(P3 诚实):本 phase 在 amd64 开发机执行,无 lab 访问 → Bucket B/A 各 task 的真机 stamp 由用户在 lab 完成或标 lab-driver-gated(per plan §8 + 本 ADR §4(c))。每 task 离线层(go build/vet/test + GOARCH=arm64 + helm dry-run + demo 回归)在本 session 全做。
