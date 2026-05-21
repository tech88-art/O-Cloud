# P10-T-001 · ADR-0015 demo-backend cache strategy(起草 + Accepted)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 0.5d plan / ~0.5d actual

## Intent

为 Phase 10 demo backend multi-site 演示打磨锁 cache 部署模式。Phase 9 P9-T-107 spike 已评估 3 路径 + 给 §6 draft outline · 本 task 起草 ADR-0015 + Accepted · 选择 §3.3 singleton with active-active failover · 实现交 P10-T-006 W1 落地。

## Path adaptations(plan literal vs codebase reality)

3 处文字偏离 plan / spike · 全部以 grep 实测代码为准:

1. **Cache size defaults 数字 plan 与 spike 都不对**:
   - Phase 10 plan §3 P10-T-001 acceptance §4(c)说"Phase 4 default is `pool=100/ms=200/scaler=50`"
   - Phase 9 spike `docs/research/demo-backend-cache-spike.md` §1 说 Topology=8/Workload=32/Pool=16/Metric=64
   - 实际 grep `backend/pkg/config/config.go:145-149`+`backend/configs/config.example.yaml:129-139`:`defaults.max_entries=1024 ttl=5m`+`per_resource.topology={64, 30s}`+`per_resource.workloads={256, 1m}`;其余 resource(pools/metrics/scaler/ms)继承 defaults
   - 解决:ADR §1.1 + §4(c)用实测值 + 加"校正"注脚 · 不沿用任一错数

2. **Spike 标签 "active-active failover" 是 misnomer**:
   - Spike §3.3 描述"仅 1 主 instance serve traffic · 多实例 backup 待 failover"— 这是 active-standby + Lease failover · 不是真 active-active(后者需 cross-instance state sync)
   - 解决:ADR §2.1 保留 spike 标签便于跨文档检索 · 同节"术语澄清"明确真实语义 · 防止读者误以为 cross-instance load balance

3. **Plan 说 cross-ref 到 arch.md "§5.1 演示后端 §3.2 数据源抽象"**:
   - arch.md §3.2 实际是"前端"(line 176)· 不是"数据源抽象"
   - "数据源抽象"在 ADR-0001 §10(line 162)而非 arch.md
   - 解决:arch.md cross-ref 落 §5.1 演示后端 cache 行 line 313;§10 数据源抽象 cross-ref 落 ADR-0001 §10(plan Allowed Paths 已含)· "§3.2"语义并入 ADR-0001 §10 cross-ref

## Debugging trail

无 build / test fail · 纯 docs · 但有 3 次"先信文档 → grep 实测发现错"的小循环(见 Path adaptations §1-3)。教训记忆已沉淀:

- **P1 数字必有源 + P3 verify-before-claim**:plan / spike 的具体数字 verify 前不信 · 全部 grep 源码取真值
- **P3 诚实优先**:spike 标签错就明说"misnomer" · 不为兼容传播延续 · 同时保留索引便利(双重标签:plan 标签 + 澄清行)

## Key decisions

- **路径选择 = §3.3 Singleton with Lease failover**(per spike §3 cost matrix):0 new dependency · 重用 inference-operator controller-runtime leader-elect pattern · 最低 latency · Failover 5-30s cold-cache 对 demo SLA 可接受
- **拒绝 §3.1 Redis-backed**:新运维负担(Redis Sentinel + backup + monitor)与"演示后端无状态 + 短期缓存"设计原则不匹配 · 但 Phase 11+ additive rewrite path 保留(`backend/pkg/cache/` 接口不变,Redis impl 可 drop-in)
- **拒绝 §3.2 Stateless**:K8s API server load risk + Phase 4-8 cache-hit-path 测试覆盖丢失
- **Lease 参数沿用 controller-runtime defaults(15s/10s/2s)**:不 override · 与 inference-operator 一致 · §4 Open question (b) 留 T201 实测后调
- **Replica 默认 2**(可选 3):demo SLA · cost 可接受
- **Degraded read-only mode + `X-Cache-Status: stale` header**:Lease API 错时 graceful 返 stale read · mutating write 返 503 + Retry-After 防 split-brain
- **3 个新 Prometheus 指标 prefix `demo_backend_`**(holder gauge + renewals counter + cache_hit_ratio):与既有 `ocloud_backend_cache_{eviction,hits}_total` 命名分割 — 因为 Lease 行为是 demo-backend specific 而非通用 cache 行为

## Verification

P3 三项验证维度 全过(docs-only task · 无 compile/test):

- **存在性**:`ls docs/adr/0015-demo-backend-cache-strategy.md` ✓ ;`grep -n ADR-0015 docs/architecture.md docs/adr/0001-phase0-key-decisions.md` 双 cross-ref 命中
- **完整性**:plan §3 P10-T-001 acceptance 8 项逐项核(§1 Context + §2 Decision A-D + §3 Consequences + §4 Open questions + §5 引用 + arch §13 row promote + ADR-0001 §10 cross-ref + arch §5.1 cache 行 cross-ref);plan 文字偏离 3 处均在 Path adaptations 解释
- **正确性**:所有数字 grep 源码取真值 · controller-runtime defaults 来源 inference-operator/cmd/main.go:60-107 · 不凭记忆

## Carry-forward

- **P10-T-006 demo-backend cache impl**:本 ADR §2.1-§2.4 是合同 · ~50 lines leader-elect + chart replicaCount 2 + RBAC Lease + 3 新指标 + degraded mode wrapper · 估 1.5-2d
- **P10-T-107 kind smoke E2E ext**:加 cache singleton Lease leader-elect assertion(per phase10-plan §2.94)
- **P10-T-201 master demo script**:跑端到端时同步观察 failover latency · cold-cache 期 frontend perceived latency · cache_hit_ratio 回升曲线;若 > 30s window 影响 UX → Phase 10 polish 调 LeaseDuration 下沿
- **Open questions 4 项不阻塞 T006**:都是 Phase 10 polish observation window 内决 · Karmada control 部署位置 / failover SLA / cache size / cross-cluster coherence 都不影响 ADR 实现 substrate

## §0a.10 / §0a.11 compliance

- 本 task 在 fresh execute session 起 · plan session 已 commit + 停在 c25627c
- §0a.11 docs-only 例外:main agent 直接做 · 无 subagent · strict verify = grep cross-refs + acceptance 逐项核
- commit 后停 · 不主动续 T002 / T003 · 等用户指明下一 task
