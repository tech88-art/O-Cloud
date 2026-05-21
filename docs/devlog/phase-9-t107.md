# P9-T-107 · demo-backend cache pattern spike (docs-only research · ADR-0015 draft outline)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 0.5d planned · ~0.3d actual

## Intent

Phase 9 P9-T-107 spike per `docs/phase9-plan.md` §4 · arch §13 Phase 9 多站点 demo backend 缓存重构 row。3 路径 evaluation(Redis-backed / stateless / singleton failover)· Karmada cross-cluster cache coherence considerations · ADR-0015 draft outline for Phase 10 implementation decision。

## Spike doc structure (7 sections)

`docs/research/demo-backend-cache-spike.md` · ~200 lines · 7 sections per plan §4 P9-T-107 acceptance(a)-(f):
- §1 Phase 4 LRU 进程内 cache 现状(eviction policy + TTL + scope per resource type)
- §2 Multi-site Phase 10 演示约束(cross-site consistency + latency tail + node failure + session affinity)
- §3 3 路径 evaluation:§3.1 Redis-backed shared cache · §3.2 stateless dispatch + per-request fetch · §3.3 in-cluster singleton with active-active failover
- §4 Cost / consistency / failure-mode matrix(6 维度对比)
- §5 Karmada cross-cluster cache coherence considerations(每路径 cross-cluster 适配)
- §6 ADR-0015 draft outline(Phase 10 starting point · Context + Decision + Consequences + Open questions + 引用)
- §7 References(arch §13 + ADR-0001 §10 + ADR-0013 §6 OQ(e)+ P7/P8 spike precedent + upstream libraries)

## Decision (recommended path for Phase 10)

**§3.3 in-cluster singleton with active-active failover** · 理由:
- 最低 latency(in-process LRU benefit 保留)· 演示 UX 不被 cache miss 影响
- 0 new external dependency(K8s Lease 已有 RBAC 路径 · 同 inference-operator leader-elect spirit)
- Karmada multi-cluster API call cost 抵消 cross-instance cache benefit
- Failover 期间 cold cache 短暂(5-30s)acceptable demo SLA
- Phase 11+ if SLA strict path 实质化 · Redis-backed 是 additive rewrite 路径

**Alternative**:§3.1 Redis-backed · production-grade HA 实质需求时
**Reject**:§3.2 stateless · K8s API server load risk demo scenario 不可接受

## Cross-ref edits

3 文件更新:
1. **docs/research/demo-backend-cache-spike.md**(NEW · 7 sections · 200+ lines)
2. **docs/adr/0001-phase0-key-decisions.md** §8 演示后端无状态 · 加 2026-05-21 P9-T-107 spike landed 段(cross-ref + Phase 10 ADR-0015 starting point)
3. **docs/architecture.md** §13 review-table Phase 9 多站点 demo backend 缓存重构 row · "Phase 9 启动前 ADR + 重构路径 · P9-T-107 docs-only spike landed" → "spike landed via P9-T-107 (2026-05-21)" + Phase 10 W1 entry pick up note
4. **docs/devlog/phase-9-t107.md**(本文件)

0 代码 · CI no-op · 同 P7-T-106 / P8-T-106 spike doc-only precedent。

## Verification

P3 三项验证维度:

| 维度 | 验证方法 | 结果 |
|---|---|---|
| **存在性** | spike doc 7 sections present · 3 cross-ref files updated · devlog | 4 文件 modified/new · 0 code modules touched |
| **完整性** | plan §4 P9-T-107 Acceptance(a-f 6 项)逐项核对 | 6/6 全覆盖:(a)Phase 4 LRU 现状 §1 ✓ ·(b)multi-site 约束 §2 ✓ ·(c)3 路径 trade-off + cost table §3+§4 ✓ ·(d)dependency comparison §4 matrix ✓ ·(e)Karmada considerations §5 ✓ ·(f)ADR-0015 draft outline §6 ✓ |
| **正确性** | bi-directional cross-ref spike ↔ ADR-0001 §8 ↔ arch §13 ↔ devlog | 4 文件互相 cross-ref · 一致 outcome |

## Carry-forward

- **Phase 10 W1 entry**:起草 `docs/adr/0015-demo-backend-cache-strategy.md` per spike §6 outline · 实施 leader-elect + Lease health probe + chart values.replicas升 2-3 + monitoring
- **ADR-0013 §6 Open question (e) lifecycleOperation 持久化** · 与本 spike outcome 协调 · 都走 singleton-with-failover 路径 一致

---

**END of P9-T-107 devlog**
