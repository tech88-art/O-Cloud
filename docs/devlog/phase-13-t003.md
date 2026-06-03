# P13-T-003 · ADR-0025 生产硬化架构(Bucket A)

- **Commit**: (this commit · Phase 13 W1)
- **Date**: 2026-06-03
- **Duration**: plan 1d vs actual ~0.5d(pure docs · grounded in code read)

## Intent

承 ADR-0023 §2 Decision D 展开 Bucket A 生产硬化的架构:5 Decision(authz / secret / quota 真强制 / Karmada HA / P99 SLA)逐项 1:1 map build-doc §5 验收清单。ADR-0025 是 T201-T206 的架构 gate。核心防 P3 gold-plating —— §3 right-sizing 声明给每项划 reference-grade 边界。

## Path adaptations(P3 verify-before-claim · 读真实代码再写架构)

- **build-doc §5 有 6 项 · 本 ADR 只覆盖 5 项**:第 6 项 Go v1 ResourceSlice migration 是 carry(ADR-0023 §2 Decision F · 触发条件 lab K8s ≥1.36 · ADR-0024 §4(a))· 显式标注"非本 ADR"避免被误读为漏项(P4 横向完整性)。
- **authn substrate 比 plan 描述更全**:grep-verified `authn.go` 已有 `Validator` 接口 + 3 impl + `Middleware()` helper · OIDCValidator/K8sTokenReviewValidator 现是 fail-safe placeholder(IssuerURL 空 / ReviewClient nil → ErrInvalidToken)· T201 = 填 body + wire middleware 而非建接口。ADR §1.2 codify 实际 substrate。
- **ClusterQuota schema 已含聚合 primitive**:grep-verified `ClusterQuotaUsage{Total, PerCluster}` + `RecomputeTotal()`(idempotent sum)已落(P11-T-104)· T204 = 填 webhook B 读路径 + controller tick populate · 复用 RecomputeTotal · ADR §2 Decision C 引用实际字段名(`status.usage.Total.CurrentSliceAllocations` / `spec.enforcement.MaxSliceAllocations`)。

## Key decisions

- **§3 right-sizing 表是本 ADR 的灵魂**(防 gold-plating):每项明确"交付什么 reference-grade" vs "不做什么(留甲方/incident-driven)"。例:authz 做 Dex 参考 IdP 不做 full IAM federation;Karmada HA 做 3-replica + 外置 etcd 不做 full DR(真物理 multi-region LB lab-gated)。落 ADR-0023 §2 Decision B "M7 不 over-claim"。
- **fail-open 兜底**(§2 Decision C):ClusterQuota webhook B 缺失/读失败 → 放行 + warning · 防 admission 把集群锁死(P5 弱链:配额强制不能比"不强制"更危险)。
- **cert 分层留 incident-driven**(§4(c)):不为完整性预建 mTLS/多 issuer(M4 价值聚焦 · 真实需求触发再做)。
- **主路径选 K8sTokenReview 而非 OIDC**:集群内组件互信走 in-cluster SA token(免外部 IdP 依赖)· OIDC 留外部访问 · 降低甲方 IdP input 对收口的 block 面。

## Verification(strict · per-task · 离线层 = pure docs · 代码 grep-verified)

- **存在性**:`docs/adr/0025-production-hardening-architecture.md` 写入(§1-§5 · Decision A-E 5 项 + §3 right-sizing 表 + §4 3 open question)· `git diff --stat` = ADR-0025(新)+ architecture.md(§6.7+§6.8 2 cross-ref)+ devlog(新)· 匹配 Allowed Paths。
- **正确性 / 代码 ground**(P3):ADR 引用的 authn(Validator/Name/Validate/Middleware/3 impl)+ ClusterQuota(Usage.Total/PerCluster/RecomputeTotal/MaxSliceAllocations)+ Karmada bootstrap + backend 静态 Bearer 均 Read/grep-verified。
- **横向一致**(P4):ADR-0025 5 Decision ↔ build-doc §5 5 项 1:1 map(ADR §1.1 表 + §2 每 Decision 标 DoD 出处)· architecture.md §6.7(authz)+ §6.8(ClusterQuota 真强制)cross-ref ADR-0025 · ADR-0023 §2 Decision D ↔ ADR-0025 双向闭合。
- markdown 表格列数核(§3 right-sizing 表 3 列 · §1.1 隐式编号一致)。

## Carry-forward

- W1(T001-T003)= 3 ADR entry gate **全 land** · W2 Bucket B(T101-T105)+ W3 Bucket A(T201-T206)解锁。
- T204 软依赖 T205(cross-cluster usage populate 需 Karmada HA path · §2 Decision C)· T202 依赖 T201(IdP/RBAC 需 validator 真体先 land · consumer)· T206 依赖 T105(真推理才能压测)。
- 每 Bucket A task 守 §3 right-sizing 边界 + §4 甲方依赖 default 兜底(IdP→Dex / SLO→default · 不 block 收口)。
- 真机层:T201 真 token 过 validator · T204 真 admission 拒 over-cap · T205 真 HA 3-replica · T206 真 P99 —— 由用户 lab 集成验或标 lab-driver-gated;本 session 做离线层(envtest / helm dry-run / go test)。
