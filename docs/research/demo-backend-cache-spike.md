# demo-backend Cache Pattern Spike (P9-T-107 · 2026-05-21)

> **Phase 9 P9-T-107 spike** per `docs/phase9-plan.md` §4 · arch §13
> Phase 9 多站点 demo backend 缓存重构 row · ADR-0015 draft outline
> for Phase 10 implementation decision.
>
> **状态**:landed @ P9-T-107 commit (this spike commit · references
> below) · ADR-0015 draft outline `§6` is the Phase 10 starting point.
> 0 代码 · docs-only research per P7-T-106 / P8-T-106 spike precedent.

---

## §1. Phase 4 LRU 进程内 cache 现状

`backend/pkg/cache/` 在 P4-T-007 引入,使用 `hashicorp/golang-lru/v2`
进程内 LRU 缓存。所有缓存对象生命周期为 demo-backend 单进程内,无跨实例同步。

**当前实现细节**:

- **Eviction policy**: LRU(Least Recently Used)· 固定大小驱逐
- **TTL**: 无显式 TTL · 仅靠 eviction 驱逐(LRU pure size-bound)
- **Cache scope per resource type**:
  - Topology aggregation cache: 8 entries(单 cluster N x scale)
  - Workload list cache: 32 entries
  - Pool aggregation: 16 entries
  - Metric query cache: 64 entries(short window · TTL-like behavior 通过 LRU 自然 evict)
- **Invalidation**: 全 LRU passive eviction · 无 explicit invalidation API
- **Failure mode**: cache miss → fall through to data source(K8s informer / Prometheus)· cache hit = stale read 不超过 LRU window 内最旧 entry

**Phase 1-8 验证 outcome**:LRU 进程内 cache 已 cover single-instance demo
backend 性能需求 · 无 production incident 涉及 cache layer。

---

## §2. Multi-site Phase 10 演示约束

arch §1.3 Phase 10 "真实硬件对接 + 演示打磨" 引入多站点 demo 场景(per
ADR-0001 §6 KubeEdge + Karmada 多站点联邦)。Multi-site brings:

- **Cross-site consistency**:demo 前端可能展示多 site 聚合视图(global
  topology · cross-site workload list)· 单 site cache miss 导致其他 site
  数据不可见
- **Low latency tail**:Karmada multi-cluster API 调用有 cross-site
  network latency · cache hit 关键 path
- **Node failure semantics**:某 site demo backend 实例挂掉时 · 其他实例
  能继续 serve 还是必须重启 cache warm 链路
- **Session affinity**:demo 前端访问 LBd backend pool · 若 session 命中
  不同 backend instance · cache 见到的 view 不一致(Phase 4-8 single-instance
  不存在此问题)

---

## §3. 3 路径 evaluation

### §3.1 Redis-backed shared cache

**架构**:demo-backend 多实例共享 Redis backend(`go-redis/redis/v9`)· 替换
进程内 LRU · `redis://demo-cache.observability-system:6379` cluster
internal endpoint · Redis Sentinel for HA。

**Pros**:
- Cross-instance consistency · session affinity 问题消失
- Explicit TTL · 可配置 per resource type · stale tolerance 精确
- Production-grade · Redis 是行业 standard
- 已存在 in-cluster Redis 部署模式(Kubernetes operators 常用)

**Cons**:
- 新运维负担:Redis 部署 + monitoring + backup + failover
- 单点(Redis Sentinel HA 缓解 · 但 setup 复杂)
- Network latency cost · cache hit 走 network round-trip(vs in-process
  memory access)· 典型增加 ~1-5ms per cache op
- 增加 demo backend Pod 启动依赖 · Redis 不可达 → demo 不能 serve(or
  graceful degraded mode)

**估算 cost**:
- 新增 deploy chart: 1 Redis chart subchart OR `bitnami/redis` external chart
- 代码改动: backend/pkg/cache/redis.go(~150 lines)· 替换 LRU API · 保持
  Cache 接口 不变
- Operational: Redis monitor + backup · ~0.2d/month overhead

### §3.2 Stateless dispatch + per-request fetch

**架构**:无 cache layer · demo-backend 每个 request 直接 K8s informer
list / Prometheus query · 完全无状态 · 多实例 互相独立。

**Pros**:
- 最简单 · 无新依赖 · 无 Redis 运维
- Stale read 不存在 · 永远 fresh
- 多实例 trivially scalable · 无 session affinity 问题
- 适合 read-heavy + low-frequency 场景

**Cons**:
- Latency 翻倍 · cache hit benefit 消失 · 重复 K8s API server 压力
- K8s API server rate limit risk · large cluster scenario
- Prometheus query 重复成本 high · cache hit ratio benefit 是 Phase 4-8
  设计 default
- Phase 4-8 测试覆盖了 cache hit path · stateless rewrite 需要 retest

**估算 cost**:
- 代码改动: 简单 · delete backend/pkg/cache/ + remove cache calls
- 但需要监控 K8s API server load 显著增加 · 可能需要加 informer cache
  layer in backend(就近似回 LRU)
- 风险:演示场景多 client 并发时 K8s API server 抖动

### §3.3 In-cluster singleton with active-active failover

**架构**:demo-backend 部署单 replica · 加 K8s leader-elect via
`coordination.k8s.io/v1.Lease` · 失主实例 standby 待主挂掉接管 · 但 仅
1 主 instance serve traffic · 多实例 backup 待 failover。

**Pros**:
- 保留 in-process LRU cache benefit · 性能最佳
- 0 new dependency · K8s Lease 已有 RBAC 路径(同 inference-operator
  leader election spirit)
- Cache 不跨实例 · 但单 active instance 没问题
- Cross-site:每 site 一 active instance · 不同 site cache 独立 OK

**Cons**:
- Failover 期间 cold cache · backup instance 主导后 LRU 重新 warm · 短暂
  latency spike(typical 5-30s)
- 主实例 Pod crash 期间 service unavailable · 不是 active-active true HA
- Session affinity 通过 LB(K8s Service)路由到 active · client retry
  during failover

**估算 cost**:
- 代码改动: 加 leader-elect logic in backend/cmd/demo-backend/main.go
  (~50 lines · 复用 controller-runtime leader-elect)
- 加 backup instance Pod monitoring + Lease health check
- helm chart `replicas` value 默认从 1 升 2-3 backup

---

## §4. Cost / consistency / failure-mode matrix

| 路径 | Latency (cache hit) | Cross-instance consistency | Failure mode | New ops cost | Code change | Phase 4-8 compat |
|---|---|---|---|---|---|---|
| §3.1 Redis-backed | ~1-5ms network | Strong(单 Redis source-of-truth)| Redis down → degraded(graceful fallback to direct fetch · 需 fail-open path)| Redis 部署 + monitor + backup ~0.2d/month | ~150 lines · Cache 接口保持 | API compat · drop-in |
| §3.2 Stateless | ~10-50ms per query(K8s informer + Prometheus)| N/A(no cache)| K8s API server load risk · 可能加 informer cache(实质回 LRU)| 0 new deps · 但 API server load monitoring | 减少代码 · delete cache layer | Test 重写 |
| §3.3 Singleton failover | ~1-10us(in-process LRU)| Single active instance · trivially consistent | Cold cache 5-30s on failover · K8s Lease 健康 path | K8s Lease monitoring · 同 inference-operator pattern | ~50 lines leader-elect | LRU 保持 · API compat |

---

## §5. Karmada cross-cluster cache coherence considerations

multi-site Phase 10 演示场景的 cross-cluster cache coherence:

**场景**:Karmada control-plane 部署 demo-backend · backend 通过 Karmada
multi-cluster API list ModelService / NPUSlicePool / NPUVerticalScaler ·
缓存 per-member-cluster + aggregated view。

**§3.1 Redis path**:multiple Karmada members 共享同一 Redis cluster · key
schema `cluster:<member>:resource:<type>:<id>` · trivially cross-cluster
consistent · 推荐路径

**§3.2 Stateless path**:每 Karmada API call cross-cluster latency 高 ·
likely unworkable for demo UX · 不推荐

**§3.3 Singleton path**:单 active instance + Karmada-side multi-cluster
API · cache 单实例 LRU 是 source-of-truth · 简单。Failover 期间 cold-cache
影响可接受 demo scenario

---

## §6. ADR-0015 draft outline (Phase 10 starting point)

ADR-0015 path: `docs/adr/0015-demo-backend-cache-strategy.md` (lands
Phase 10 W1 entry · per P9-T-108 checkpoint Phase 10 seed brief)

**§1 Context** (Phase 10 W1 entry 起草时填):
- Phase 4-8 LRU 进程内 cache 现状(§1 of 本 spike)
- Phase 10 multi-site Phase 10 演示约束(§2 of 本 spike)
- 3 路径 evaluation findings(§3 of 本 spike · spike doc cross-ref)

**§2 Decision** (Phase 10 W1 entry):**Default recommendation:
§3.3 Singleton with active-active failover** · 理由:
- 最低 latency · 演示 UX 不被 cache miss 影响
- 0 new external dependency
- Karmada multi-cluster API call cost 抵消 cross-instance cache benefit
- Failover 期间 cold cache 短暂 · 5-30s acceptable demo SLA

**Alternative**:§3.1 Redis-backed path · 若 production-grade HA 实质需求
出现(SLA strict · 演示阶段不太可能 · Phase 11+ 评估)

**Reject**:§3.2 stateless · K8s API server load risk demo scenario 不
可接受

**§3 Consequences**:
- Phase 10 W1 task: leader-elect logic + Lease health probe + chart
  values.replicas升 2-3
- Phase 10 polish: monitoring (Lease health · failover frequency) +
  graceful degradation on K8s Lease error(degraded read-only mode)
- Phase 11+ if SLA strict path lights up: § 3.1 Redis-backed additive
  rewrite

**§4 Open questions**:
- Karmada control-plane 部署位置(同 Karmada control · 还是独立 site)
- Failover SLA acceptable threshold(5s? 30s? operator decides)
- Cache size tuning per resource type(已 § 1.3 default · 多 site 是否
  需调整)

**§5 引用**:
- 本 spike doc(P9-T-107)+ arch §13 Phase 9 行 + ADR-0001 §10 数据源抽象
- ADR-0013 Phase 10 polish lifecycleOperation persistent backing
  (independent decision · 与本 ADR 协调)

---

## §7 References

- arch §13 Phase 9 review-table "Phase 9 多站点 demo backend 缓存重构(LRU
  进程内 → Redis/singleton/stateless)" row · this spike landed prerequisite
- arch §1.3 Phase 10 "真实硬件对接 + 演示打磨" row · multi-site 演示场景
- ADR-0001 §10 数据源抽象 DataSource interface · cache layer 是 DataSource
  实现的 transparent layer · 不影响接口
- ADR-0013 §6 Open question (e) lifecycleOperation 持久化 · cross-ref
  spike outcome · 与本 ADR-0015 协调 Phase 10 polish
- P7-T-106 spike precedent: `docs/research/k8s-partitionable-devices-spike.md`
- P8-T-106 spike precedent: `docs/research/volcano-gang-scheduling-spike.md`
- upstream: `hashicorp/golang-lru/v2` (Phase 4 LRU impl)
- upstream: `go-redis/redis/v9` (§3.1 Redis path)
- upstream: `sigs.k8s.io/controller-runtime/pkg/leaderelection` (§3.3 singleton path)

---

**END of demo-backend cache spike**
