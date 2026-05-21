# ADR-0015: demo-backend cache 策略 — 单 active 实例 + K8s Lease 选主 failover(Phase 10 multi-site 演示 substrate)

- **状态**:Accepted(design + implementation handoff;impl P10-T-006)(2026-05-21 — Phase 10 P10-T-001)
- **日期**:2026-05-21
- **决策者**:协调者(用户)
- **相关**:ADR-0001 §10 数据源抽象 DataSource 接口(cache 层是 DataSource 实现的 transparent layer · 接口不变)/ ADR-0013 §6 forward notes(O2 DMS Adapter Phase 10 polish — Karmada propagation + lifecycleOperation 持久化 · cache singleton 保证 cross-instance ModelService aggregated view 一致)/ ADR-0014 §7 forward notes(Quota cluster-scope + Karmada cross-cluster propagation · cache singleton 保证 frontend Quota usage visualisation 一致)/ `docs/research/demo-backend-cache-spike.md`(P9-T-107 spike landed · 3 路径 evaluation + §6 draft outline 本 ADR 直接 pick up)/ `docs/architecture.md` §1.3 Phase 10 "真实硬件对接 + 演示打磨" multi-site 演示约束 · §5.1 演示后端 `backend/pkg/cache/` LRU 实现 · §13 review-table Phase 9 "多站点 demo backend 缓存重构" row(本 ADR 起草后 promote "in flight via ADR-0015 + T006")/ `docs/phase10-plan.md` §3 P10-T-001 起草 + P10-T-006 impl

---

## §1 Context

### §1.1 Phase 4 LRU 进程内 cache 基线

`backend/pkg/cache/`(P3-T-008 引入,P4-T-007/T008 Prometheus instrumentation)是 demo-backend 的内存 LRU 缓存层 · `hashicorp/golang-lru/v2` 后端 · 通过 `cache.Catalog` 注册 per-resource 实例(`pkg/config.CacheConfig.EntryFor`)。**实际默认值**(per `backend/pkg/config/config.go:148-149` + `backend/configs/config.example.yaml:129-139`):

- `cache.defaults.max_entries = 1024` / `cache.defaults.ttl = 5m`(全 resource 默认,viper `SetDefault`)
- `cache.per_resource.topology = { max_entries: 64, ttl: 30s }`(显式 override)
- `cache.per_resource.workloads = { max_entries: 256, ttl: 1m }`(显式 override)
- 其他 resource(pools / metrics / scaler / ms · etc)未列入 `per_resource`,继承 defaults(1024 / 5m)

> **校正**:Phase 9 spike doc §1 给的 per-resource 示例数字(Topology=8 / Workload=32 / Pool=16 / Metric=64)与 Phase 10 plan §4 (c) 表述 `pool=100/ms=200/scaler=50` 均与实际配置不符 — 本 ADR 以 viper SetDefault + config.example.yaml 实际为准(verified at T001 entry via grep)。

**Phase 1-8 验证结果**:LRU 进程内 cache 已 cover single-instance demo backend 性能需求 · 无 production incident 涉及 cache layer · `ocloud_backend_cache_eviction_total{resource}` + `ocloud_backend_cache_hits_total{resource}` Prometheus 指标 P4-T-008 起暴露(arch §3 line 238)。

### §1.2 Phase 10 multi-site 演示约束

`docs/architecture.md` §1.3 Phase 10 row "真实硬件对接 + 演示打磨" 引入多站点 demo 场景(per ADR-0001 §6 KubeEdge + Karmada 多站点联邦)· `docs/phase10-plan.md` §3 P10-T-201 master demo script 要求 multi-pool / multi-tenant / multi-modelservice 端到端 flow。Multi-site brings:

- **Cross-instance consistency**:demo backend 若部署 ≥ 2 instance(behind K8s Service LB),不同 instance 持有不同 LRU view → frontend 看到的 topology / workload list / Quota usage 因 session affinity 漂移而抖动
- **Karmada cross-cluster API call cost**:Karmada multi-cluster API list 带 cross-site network latency · cache hit 是 demo UX 关键 path
- **Failover semantics**:demo backend 单 instance crash 时,backup instance 主导后 cache cold-warm period 是 demo SLA 重要变量

### §1.3 3 路径 evaluation findings(spike doc cross-ref)

`docs/research/demo-backend-cache-spike.md` §3 评估 3 路径(详 §4 cost / consistency / failure-mode matrix):

| 路径 | Latency | Cross-instance consistency | New ops cost | Code change | API compat |
|---|---|---|---|---|---|
| §3.1 Redis-backed | ~1-5ms network | Strong | Redis 部署 + monitor + backup ~0.2d/month | ~150 lines + Cache 接口 drop-in | drop-in |
| §3.2 Stateless dispatch | ~10-50ms per query | N/A(no cache)| 0 new deps · API server load risk | delete cache layer | test 重写 |
| §3.3 Singleton + Lease failover | ~1-10us(in-process LRU)| Single active instance · trivially consistent | K8s Lease monitoring · 同 inference-operator pattern | ~50 lines leader-elect | LRU 保持 |

**关键 trade-off**:
- Redis path 上 ops cost(新 deployment + HA + backup chain)与 Phase 10 演示 grade SLA 不匹配 — 演示场景 0 new external dependency 是 first-class 约束(arch §3 设计原则 "演示后端无状态 + 短期内存缓存")
- Stateless path 把 cache miss → K8s API server load 显著放大,large cluster + Karmada cross-cluster API 场景下抖动风险高,且 Phase 4-8 cache-hit-path 测试覆盖丢失
- Singleton path 保留 in-process LRU 性能 + 0 new dependency,代价是 failover 期间 5-30s cold cache window(对 demo SLA 可接受 · 详 §2 Decision C)

### §1.4 本 ADR 不涉及

- Redis backend 完整落地(§3.1 Alternative · Phase 11+ if production-grade HA signal — 详 §3 Consequences)
- frontend cache hint header 处理 / stale state 视觉提示(Phase 10 polish if T201 演示打磨观察到 UX issue)
- Karmada control-plane 部署位置(§4 Open question (a)· Phase 10 W2/W3 由 T103 + T104 + T201 cross-task 协调)
- demo backend 与 KubeEdge cloud core / mapper 的 cache 协调(不冲突 · KubeEdge mapper 走独立路径,cache 只 cover demo backend → K8s/Prometheus 出方向 read path)

---

## §2 Decision

### §2.1 Decision A:路径选择 = §3.3 Singleton with active-active failover

**承诺范围**:Phase 10 demo backend 部署模式落 **§3.3 singleton with active-active failover** 路径 · 保留 `backend/pkg/cache/` 进程内 LRU + 加 K8s Lease 选主 · 多 replica 中 1 active serve traffic · backup standby 等 failover。

**术语澄清**:spike doc / plan 标签为 "active-active failover" · 实际语义是 **active-standby in steady state**(K8s Lease 选举 single leader 持有 traffic · backup replicas standby + Lease watch · leader crash 时 backup 抢 Lease 接管)· 不是真 active-active(那需要 cross-instance state sync · §3.1 Redis path 才能给)。沿用 spike 标签便于跨文档检索,本节澄清 enforce 真实语义。

**理由**:
- **最低 latency**:in-process LRU ~1-10us per cache op · 不被 cache miss latency 影响 demo UX
- **0 new external dependency**:无 Redis / 无 Sentinel / 无 backup chain · 完全 K8s-native(Lease 是 `coordination.k8s.io/v1` 内置资源)
- **Karmada multi-cluster API call cost 抵消 cross-instance cache benefit**:即使 Redis 共享 cache,Karmada cross-site API 仍 dominate latency · singleton 内单 instance LRU 已是 source-of-truth
- **failover 期间 cold cache 短暂**:typical 5-30s · 对 demo SLA 可接受(non-production demo · client retry tolerant)
- **重用既有 leader election infrastructure**:inference-operator(`operators/inference-operator/cmd/main.go:60-70, 106-107`)已 `controller-runtime` 内建 `--leader-elect` flag · 同 spirit 移植到 demo-backend 路径成熟

**拒绝 §3.2 Stateless**:K8s API server load risk 在 multi-tenant Karmada scenario 不可接受 · 且 Phase 4-8 测试覆盖 cache-hit-path · 重写代价 + retest 代价过大。

**保留 §3.1 Redis-backed 为 Phase 11+ 备选**:若 production-grade HA SLA(failover < 1s · zero stale window)signal 出现 · §3.1 additive rewrite(Cache 接口保持) · 0 architecture lock-in。

### §2.2 Decision B:K8s Lease 选主参数

**实现路径**:`sigs.k8s.io/controller-runtime/pkg/leaderelection` · 同 inference-operator pattern(operators/inference-operator/cmd/main.go:69-70 + 106-107)。

**参数**(controller-runtime defaults 不 override):

| 参数 | 值 | 说明 |
|---|---|---|
| Lease namespace | `ocloud-system` | demo backend Pod / 其他 operator 共用 namespace · chart values 可 override 但 default 是 `ocloud-system` |
| Lease 资源名 | `demo-backend-leader` | `coordination.k8s.io/v1.Lease` 单一资源 · 不与 inference-operator 的 `inference-operator.ocloud.edge.example.com` 冲突 |
| `LeaseDuration` | 15s | controller-runtime default · leader Lease 有效时长 |
| `RenewDeadline` | 10s | controller-runtime default · leader 需在此时间内续约 · 超时则放弃 leadership |
| `RetryPeriod` | 2s | controller-runtime default · 非 leader 实例的 Lease watch 轮询间隔 |
| `ReleaseOnCancel` | true | demo backend graceful shutdown 时主动 release Lease · 加快 failover(default behavior of controller-runtime manager) |

**关于参数 tuning**:controller-runtime defaults 与 K8s `leaderelection` library 推荐值一致(`client-go/tools/leaderelection`) · 已被多个生产级 controller 验证。Phase 10 不 override;若 §4 Open question (b) failover SLA 调研发现 5-30s window 偏长 → Phase 10 polish 调小 LeaseDuration(代价:Pod 切换更敏感)。

**RBAC**:demo backend ServiceAccount 需 `coordination.k8s.io/v1.Lease` `get/list/watch/create/update/patch` 权限 · namespace-scope = `ocloud-system` · T006 chart RBAC 落 ClusterRole + RoleBinding 而非 ClusterRoleBinding(min-priv per ADR-0014 §3 spirit)。

### §2.3 Decision C:Replica 数 + degraded read-only failover 行为

**Replica 数**:`deploy/helm-charts/demo-backend/values.yaml` 中 `replicaCount` 默认从 1 升 **2**(可选 3) · 单 active + 1-2 standby · acceptable cold-cache failover 5-30s demo SLA。`replicaCount: 2` 是 Phase 10 W1 default · 若 T201 master demo script 发现 failover window 影响 UX → 升 3 + 调 LeaseDuration。

**Failover 行为分层**:

| 状态 | Active leader 行为 | Backup follower 行为 |
|---|---|---|
| 稳态 | serve all traffic · LRU 缓存所有 read paths · Prometheus metrics 全量 emit | follower process 启 + 监听 health probe · cache 为空(冷)· **不**接 traffic(K8s Service `readinessProbe` 在 follower 上返 false · 由 demo-backend `/healthz/ready` 路径分发到 leader-elect 状态机) |
| Leader crash | Pod 失健 · LB 自动移除 endpoint | backup 通过 Lease watch 检测到 expire · 抢 Lease 成功 → become leader · readinessProbe 翻 true · serve traffic · LRU 冷启 |
| Leader graceful shutdown | `ReleaseOnCancel=true` · 主动放 Lease | backup 立即抢 · failover window 比 crash 短(< LeaseDuration) |
| Lease API call error / network partition | leader 检测 `RenewDeadline` 超时 · 切 **degraded read-only mode** · stale cache 继续 serve · response header 加 `X-Cache-Status: stale` · 暂不接 mutating write(deploy/scale endpoints 返 503 + Retry-After) | backup 看到 Lease 失效但抢不到(API 错)· 同样 standby |

**Degraded read-only mode 触发**:
- demo backend leader 调 Lease API renew 失败连续 ≥ 3 次(每 RenewDeadline 一次 retry)
- 进入 mode 后,LRU read path 保留 + write path(deploy POST / scaler patch)返 503 + `Retry-After: 15` · response 加 `X-Cache-Status: stale`
- 退出条件:Lease renew 恢复 OR backup 抢到 Lease → leader voluntarily 降级

**Graceful degradation 原因**:demo 场景下"返 stale 数据"比"完全 down"对 UX 友好 · 但 mutating ops 必须 fail-fast 防止 split-brain 写。

### §2.4 Decision D:Monitoring 指标

`backend/pkg/api/prom_metrics.go` 新增 3 个 Prometheus 指标(T006 实现,本 ADR 锁 schema):

| 指标名 | 类型 | Labels | 含义 |
|---|---|---|---|
| `demo_backend_lease_holder` | Gauge | `{instance}` | 1 if this instance is current Lease holder · 0 otherwise(per-instance · 全集 sum = 1 in steady state · sum = 0 during failover window) |
| `demo_backend_lease_renewals_total` | Counter | `{instance, result}` | Lease renewal 总次数 · `result ∈ {success, error}` · error rate spike 是 degraded mode trigger 信号 |
| `demo_backend_cache_hit_ratio` | Gauge | `{resource}` | 计算自既有 `ocloud_backend_cache_hits_total{resource}` / (`ocloud_backend_cache_hits_total{resource}` + `ocloud_backend_cache_eviction_total{resource}`) · 当 cold cache failover 后该 gauge dip 明显 · failover monitoring 信号 |

**与既有 metrics 关系**:
- 既有 `ocloud_backend_cache_eviction_total` / `ocloud_backend_cache_hits_total`(P4-T-008)继续 emit per resource · 不变
- 新增 3 个指标命名 prefix `demo_backend_`(非 `ocloud_backend_`) · 因为 Lease 行为是 demo-backend specific behavior 而非通用 cache 行为 · 命名 prefix 分割
- Grafana dashboard(Phase 10 T105 if 加 panel · 否则 manual Grafana query 用作 ops insight) · 不阻塞 T006 land

**Alert 阈值建议**(non-binding · Phase 10 polish if `prometheus-rules.yaml` 加 rules):
- `sum(demo_backend_lease_holder) != 1` for > 30s → page · split-brain 或 prolonged failover
- `rate(demo_backend_lease_renewals_total{result="error"}[5m]) > 0.1` → warn · API server / network 不稳

---

## §3 Consequences

### §3.1 Phase 10 W1 immediate impact

- **T006 demo-backend cache impl** 按本 ADR §2.1-§2.4 落地 · 估 1.5-2d:
  - `backend/cmd/demo-backend/main.go` 加 leader-elect 启动 path(~50 lines · 同 inference-operator pattern)
  - `backend/pkg/api/health.go`(new or extend)`/healthz/ready` 与 leader-elect 状态机分发
  - `backend/pkg/api/prom_metrics.go` 加 §2.4 3 个新指标 + 注册到既有 Prometheus registry
  - `backend/pkg/cache/` degraded read-only mode wrapper(包装 `cache.LRU` Get/Set · Set on degraded → no-op + log)
  - `deploy/helm-charts/demo-backend/values.yaml` `replicaCount: 2` default + Lease RBAC + readinessProbe wiring
  - `deploy/helm-charts/demo-backend/templates/{role,rolebinding,deployment}.yaml` 落 RBAC + replicaCount
  - 1-2 unit test:leader-elect smoke(envtest 起 Lease + 模拟 leader change · 验证 backup 接管 + readinessProbe 翻 true)+ degraded mode wrapper unit
- **0 new external dependency**:`controller-runtime` 是 backend 已有间接依赖(`pkg/datasource/crd` 走 controller-runtime client) · 无需 `go.mod` 加新模块。Lease 是 K8s built-in resource。

### §3.2 Phase 10 W2 polish observation window

T201 master demo script 跑 multi-pool / multi-tenant / multi-modelservice 端到端 flow 时 · 同步观察:
- failover 实际 latency(典型 5-30s · 若 > 30s → 调 LeaseDuration 下沿)
- cold-cache 期间 frontend perceived latency 增量(若 > 200ms 持续 > 30s → 考虑 cache warm-up hook)
- `demo_backend_cache_hit_ratio` 在 failover 后回升曲线(若 > 60s 才回到稳态 → 考虑 cache size 调大,§4 Open question (c))

### §3.3 Phase 11+ Redis-backed additive rewrite path retained

`backend/pkg/cache/` 接口保持 · 若 Phase 11+ production-grade HA signal 出现(SLA strict · failover < 1s · zero stale window) → §3.1 Redis-backed path 是 additive rewrite:
- 加 `backend/pkg/cache/redis.go` impl Cache interface
- Factory 走 `cache.NewFromConfig(cfg)`(已有 P3-T-008 模式 · 0 调用方代码改动)
- `deploy/helm-charts/demo-backend/` 加 Redis subchart dep(`bitnami/redis` 或 in-cluster Redis Sentinel)
- Singleton path Lease 选主可保留(冷热分离:Redis 共享 cache 解决 cross-instance state · Lease 仍可用作 deploy/scale write 序列化 · 但 Phase 11+ 重评估)

**Lock-in 评估**:本 ADR ship 后 · backend 不引入 Redis 模块 · 不动 Cache 接口 · 也不 hardcode Lease 调用到 datasource layer(Lease 调用集中在 `backend/cmd/demo-backend/main.go` setup + `backend/pkg/api/health.go` readiness 分发) · Phase 11+ 退出成本低。

### §3.4 与 ADR-0013 / ADR-0014 / ADR-0001 协调

- **ADR-0013 §6** O2 DMS Adapter Phase 10 polish — Karmada propagation 第一波 + lifecycleOperation 持久化 · cache singleton 保证 O2 DMS NB endpoint 返回的 ModelService aggregated view 在 demo backend 各 instance 之间一致(因为只 1 active 实例 serve)。本 ADR 不阻塞 ADR-0013 forward notes,反过来 cache singleton 简化 O2 DMS state consistency 推理。
- **ADR-0014 §7** Quota Phase 10 polish — cluster-scope ClusterQuota + Karmada cross-cluster Quota propagation · frontend Quota usage visualisation(T105)读 demo backend `/api/v1/quota/usage`(若加 endpoint) · cache singleton 保证 visualisation 在 multi-instance LB 后不抖动。
- **ADR-0001 §10 数据源抽象** DataSource 接口不变 · cache 是 DataSource 实现的 transparent decorator layer · 接口表面零感知。本 ADR 在 ADR-0001 §10 加 cross-ref(P10-T-001 Allowed Path)。
- **ADR-0011 §3** 真硬件对接 lab gating · 本 ADR 与 lab availability 无关 · 同期推进。

### §3.5 已知 trade-off

- **不是真 active-active**:failover 期间 5-30s service degraded(read-only) · 演示场景可接受 · 生产场景 Phase 11+ Redis path
- **多 replica 资源代价**:`replicaCount: 2-3` 比单 instance 多消耗 backend CPU/mem · 但 backend 是 lightweight Go process(< 200MB RSS · < 0.1 CPU steady state · per Phase 8 baseline)· cost 可接受
- **kind smoke E2E 需扩展**:Phase 10 T107 加 cache singleton K8s Lease leader-elect assertion(per phase10-plan §2.94)· 但 envtest unit + smoke 已 cover · 不额外阻塞

---

## §4 Open questions

### (a) Karmada control-plane 部署位置

`docs/architecture.md` §1.3 Phase 10 row 提及 Karmada propagation 第一波(via T103 + T104) · demo backend 部署位置有 3 候选:

- (A) 同 Karmada control · 与 Karmada API server 共 namespace · 调 Karmada API 走 in-cluster Service · latency 最低
- (B) 独立 site(member cluster 之外)· 走 LB 调 Karmada API · 网络成本高 · 但部署独立性强
- (C) 每 member cluster 一 demo backend 实例 + 各自 Lease · cross-site aggregation 由前端 fan-out 而非 backend

**当前倾向**:(A) per Phase 10 T201 simpler topology · Phase 11+ multi-site time 评估 (B)/(C)。**待 T201 master demo script land 时确认实测 latency 与 UX**。

### (b) Failover SLA acceptable threshold

5-30s window 是 controller-runtime default LeaseDuration=15s + RenewDeadline=10s 直接推出 · demo SLA 是否需要更紧?

- 5s window:LeaseDuration ≤ 5s · 但 K8s API server pressure 上升(每 instance 续约更频繁) · 不推荐 default
- 15s window(current default):balanced · 适合 demo
- 30s window:LeaseDuration 30s · failover 期间 readability 长 · 但 Lease churn 少 · 适合 stable production demo

**当前倾向**:保持 controller-runtime default(15s)· Phase 10 T201 observation 后调。

### (c) Cache size tuning per resource type

Phase 9 P9-T-107 spike 与 Phase 10 plan §4(c)给的 per-resource 数字均与实际配置不符 · 实际 default(verified at T001 entry):
- `cache.defaults`:`max_entries=1024 / ttl=5m`(全 resource 默认)
- `cache.per_resource.topology`:`max_entries=64 / ttl=30s`
- `cache.per_resource.workloads`:`max_entries=256 / ttl=1m`
- 其他 resource(pools / metrics / scaler / ms · O2 DMS NB / Quota usage 等)继承 defaults

**Phase 10 multi-site 影响**:Karmada 聚合后 list 大小可能涨 · `topology` 64 entries 在 multi-cluster 下是否够?`workloads` 256 在 multi-tenant 下是否够?**待 T201 observation 决** · Phase 11+ revisit。**本 ADR 不调** · 沿用既有 defaults。

### (d) Cross-cluster cache coherence Karmada path

Phase 10 ships Karmada propagation 第一波(via T103 + T104) · demo backend 通过 Karmada multi-cluster API list 各 member cluster 资源 · cache key 应否包含 member cluster 标识?

- 选项 1:`cache.LRU` key 加 `<cluster>:<resource>:<id>` prefix · per-member 独立缓存 · aggregated list 走 fan-out + per-member cache hit
- 选项 2:统一 aggregated view cache · key 是 `<resource>:<id>`(aggregated) · cache miss 触发 fan-out · 较粗粒度但简单
- 选项 3:Phase 10 single-member(kind cluster 模拟 multi-site) · 暂不区分 · Phase 11+ multi-member 再设计

**当前倾向**:选项 3 — Phase 10 single-member kind cluster 模拟 · cache key 不带 cluster 维度 · Phase 11+ 真 multi-member 时(arch §13 Phase 11+ row)落 ADR · T201 演示打磨 不卡这个。

---

## §5 引用

### 上游(本 ADR 决策依据)
- `docs/research/demo-backend-cache-spike.md`(P9-T-107 @ 4809659 · 3 路径 evaluation + §6 draft outline 本 ADR 直接 pick up)
- `docs/architecture.md` §1.3 Phase 10 "真实硬件对接 + 演示打磨" row · §3 设计原则 "演示后端无状态 + 短期内存缓存" · §5.1 演示后端 `backend/pkg/cache/` LRU · §13 review-table Phase 9 "多站点 demo backend 缓存重构" row(本 ADR 起草 promote to "in flight via ADR-0015 + T006")
- ADR-0001 §10 数据源抽象 · cache 是 DataSource transparent layer
- ADR-0013 §6 forward notes(O2 DMS Karmada propagation · cache singleton 给 cross-instance consistency 简化推理)
- ADR-0014 §7 forward notes(Quota cluster-scope + Karmada cross-cluster propagation · cache singleton 简化 usage visualisation 一致)
- ADR-0011 §3 lab gating(无 dependency · 同期推进)

### 下游(本 ADR 后续工作)
- P10-T-006 demo-backend cache impl(本 ADR §2.1-§2.4 落地 · 估 1.5-2d)
- P10-T-107 kind smoke E2E extension(cache singleton Lease leader-elect assertion)
- P10-T-201 master demo script(failover observation · §4 Open question (a)/(b) 实测)
- Phase 11+ Redis-backed additive rewrite(若 production-grade HA signal · §3.3)

### 上游代码(决策时 grep-verified)
- `backend/pkg/config/config.go:39-69, 145-149`(CacheConfig + applyDefaults · defaults max_entries=1024 ttl=5m)
- `backend/configs/config.example.yaml:129-139`(per_resource overrides topology=64/30s · workloads=256/1m)
- `backend/pkg/cache/{lru,eviction,options}.go`(P3-T-008 Cache interface · P4-T-008 Prometheus instrumentation)
- `operators/inference-operator/cmd/main.go:69-70, 106-107`(controller-runtime `LeaderElection` + `LeaderElectionID` pattern · 本 ADR §2.2 重用)
- `backend/pkg/api/prom_metrics.go:27-39, 98-102`(MetricCacheEvictionTotal + MetricCacheHitsTotal · 既有指标 · §2.4 新 3 个指标 prefix 分割 `demo_backend_`)

### 上游 external
- `sigs.k8s.io/controller-runtime/pkg/leaderelection`(K8s Lease + ctrl.Options.LeaderElection)
- `k8s.io/api/coordination/v1.Lease`(K8s 内置 Lease 资源)
- `k8s.io/client-go/tools/leaderelection`(底层 algorithm · controller-runtime 包装)
- `hashicorp/golang-lru/v2`(既有 in-process LRU backend · Phase 4 起 · 不变)

---

**END of ADR-0015**
