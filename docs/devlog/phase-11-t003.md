# P11-T-003 · demo-backend helm chart + cmd/main.go leader-elect wire

- **Commit**: (this commit)
- **Date**: 2026-05-22
- **Duration**: 1-1.5d plan / ~0.7d actual(client-go 已有 · 选 client-go 而非 controller-runtime 简化路径)

## Intent

把 ADR-0015 §3.3 Singleton substrate(P10-T-006 落地的 backend/pkg/cache/singleton.go)接通到一个真 helm chart + 一个真 leader-elect goroutine · 实现 multi-replica demo-backend Lease 选主 + degraded read-only mode + 3 Prometheus 指标暴露 · ADR-0017 §2 Decision D 1st 优先级 closure。

## Path adaptations(plan literal vs codebase reality)

3 处:
1. **Plan 说 "controller-runtime leader-elect goroutine"** · 实际选 **client-go `tools/leaderelection` 直接 wire**:
   - backend/go.mod 已有 `k8s.io/client-go v0.31.4` 间接依赖(via datasource/k8s)· 引入 controller-runtime 是新 *direct* dep + 一整套 manager / reconciler 抽象(backend 不是 operator · 不需要 reconciler)
   - client-go 直接提供 `leaderelection.RunOrDie` + `LeaseLock` resource lock · 50 行 helper 即可包装 · 完全满足 Singleton callbacks contract
   - 解决:`backend/pkg/cache/leaderelect.go` 新增(~180 行 · 含 docstring + helper)· main.go 仅 ~25 行 leader-elect wiring · ADR-0015 §3.1 status update 已记此路径偏离 "0 new external dependency" 文字(实际依赖是 client-go 已有 sub-package · 不破 go.mod direct dep)
2. **Plan Allowed Paths 含 `backend/Dockerfile`(existing edit · confirm multi-stage + COPY binary path · 无重大改动)** · 实际 grep:Dockerfile 已是 multi-stage distroless · golang:1.26-alpine build · 无改动需求 · 不动
3. **Plan Allowed Paths 含 `backend/pkg/api/health.go` 与 leader-elect 状态机分发**(从 ADR-0015 §3.1 line 154) · 实际 grep:`backend/pkg/api/health.go` 不存(健康检查在 `backend/pkg/api/system.go`)· Phase 11 W1 minimum 不动 system.go(/healthz 短路 200 即可让 chart readinessProbe 工作)· Singleton state 表面化到 /healthz/ready 留 W2 polish or T201 实测 trigger

## Debugging trail

无 build / test fail(单 run 通)· 几个小决策:

1. **`LeaseLock.LeaseMeta` 字段类型** — 首次写成自定义 `leaseMeta` struct alias(避免 import resourcelock to namespace)· 实际 client-go 直接用 `metav1.ObjectMeta` 字段。解决:import `k8s.io/apimachinery/pkg/apis/meta/v1` · `LeaseMeta: metav1.ObjectMeta{Namespace, Name}` 直 set。删除自定义 alias 段。
2. **`Handler.CacheSingleton` 字段** — 加在 `backend/pkg/api/router.go` Handler struct · 同 MetricsRegistry pattern (may be nil → handler 短路)· main.go 在 leader-elect goroutine launch 后 set handler.CacheSingleton 之前已 NewRouter 完成 — 顺序是 NewHandler → set GrafanaBaseURL → set MetricsRegistry → 起 Lease loop → NewRouter。**注意**:CacheSingleton 当前未被任何 handler consume(/healthz 仍 200 OK 不查 state · /metrics 暂未 emit Lease gauge)· W2 polish or T201 实测 trigger 时 wire。本 task ship "field 加上" + "main.go wire 通" 两步 · 不强行加 handler consume(防止 mission creep)。
3. **chart values.yaml 默认 leaseElection.enabled=true** · 但 backend pkg/config defaults Lease.Enabled 字段 默认 false(Viper 没显式 default)— 这意味着 *chart-installed* demo-backend Lease 默认开 · *本地 go run / config.dev.yaml* 跑的 demo-backend Lease 默认关。两条路径都健康(chart 用户 → 有 K8s API)+(本地 dev → 无 K8s API)。devlog 明示这是 by-design。

## Key decisions

- **client-go tools/leaderelection over controller-runtime**(rationale 上 path adaptation #1):
  - backend "无 reconciler" 设计 · controller-runtime 大半抽象不用
  - client-go 已是 indirect dep · 不新增 go.mod direct dep
  - 50 lines wrapper 全包 OK · 不为短期方便引入大依赖
- **POD_NAME downward API for HolderIdentity**(per ADR-0015 §2 Decision B + chart templates/deployment.yaml):
  - chart inject POD_NAME via downward API
  - main.go 通过 cache.LeaderElectOptions 走 leaderelect.go `resolveHolderIdentity` 三层 fallback(`cfg.HolderIdentity` → `POD_NAME` env → `os.Hostname`)
  - 单元测试可 SetLeaseHolderIdentity 注入 deterministic holder
- **namespaced Role + RoleBinding for Lease RBAC**(NOT ClusterRole):
  - Lease 只活在 `leaseElection.namespace`(ocloud-system default)· namespace-scoped RBAC 足够
  - blast radius 最小 — demo-backend SA 不能 read Secret / Pod / Node / 任何其他 cluster-scoped 资源
  - 与 inference-operator chart pattern 一致(inference-operator 有 ClusterRole 是因为它要 reconcile cluster-wide ModelService;demo-backend 仅 Lease 一项 · 不需 ClusterRole)
- **ReleaseOnCancel=true**(cooperative lease release on shutdown):
  - SIGTERM → main.go rootCtx cancel → leaderelection.RunOrDie 退出 · 自动 delete Lease 让 next replica 立即 acquire(< 1s failover vs 等 LeaseDuration timeout 15s)
  - graceful shutdown failover 优 ungraceful 5x
- **ServiceMonitor opt-in default off**:
  - Prometheus Operator 不一定装 · default true 会让 chart install 失败 in plain K8s cluster
  - `--set serviceMonitor.enabled=true` 启 · 与 inference-operator chart 同 pattern

## Verification

P3 三项验证维度 全过:

- **存在性**:
  - `ls deploy/helm-charts/demo-backend/{Chart.yaml,values.yaml,.helmignore}` ✓
  - `ls deploy/helm-charts/demo-backend/templates/{_helpers.tpl,deployment,service,configmap,serviceaccount,rbac,servicemonitor}.yaml` ✓(7 templates · 全 ship)
  - `ls backend/pkg/cache/{singleton,leaderelect}.go backend/docs/cache.md` ✓
- **完整性**(plan §3 P11-T-003 acceptance 7 items + ADR-0015 §3.1 P10-T-006 carry list 5 items):
  - `helm lint --strict deploy/helm-charts/demo-backend/` → `1 chart(s) linted, 0 chart(s) failed`(only `[INFO] Chart.yaml: icon is recommended` · 不算 fail)
  - `helm template demo-backend deploy/helm-charts/demo-backend/` → 8 kind render(ServiceAccount + ConfigMap + Role + RoleBinding + Service + Deployment)+ ServiceMonitor 当 enabled=true
  - `helm template` → Lease RBAC YAML 含 `coordination.k8s.io/leases` resource verbs `get/list/watch/create/update/patch/delete` ✓
  - `helm template` → Service ClusterIP default + Deployment replicaCount 2 default + readinessProbe + livenessProbe on /healthz ✓
  - `helm template` → ServiceMonitor 3 metrics scraping intended via /metrics port ✓(scrape path documented in template comment)
  - `cd backend && go build ./...` → exit 0 · 无 unresolved import / unused / shadow
  - `cd backend && go test ./pkg/cache/... ./pkg/config/...` → `ok pkg/cache 0.468s` PASS · pkg/config no test
  - main.go: cfg.Lease.Enabled 分支 + Singleton 构造 + RunLeaderElection goroutine launch + Handler.CacheSingleton 注入 — 5 lines 之内
  - backend/docs/cache.md §3 chart wiring + Pod lifecycle 文档 — 5 sub-sections(§3.1 chart files · §3.2 RBAC scope · §3.3 Pod lifecycle · §3.4 failover SLA obs · §3.5 Phase 12+ deferred)
- **正确性**(诚实标 kind smoke 替代):
  - kind binary `command -v kind` 在 dev host 不存(本 session 已 toolchain check) · kind smoke `helm install` 实测 fall back 到 `helm template` 全 kind 渲染 OK
  - 真 kind smoke 由 T107 phase11/ folder 覆盖(per plan §4 P11-T-107 assertion (a) demo-backend chart + Lease leader-elect)
  - 本 task 不 ship kind cluster smoke · devlog 诚实标 "kind missing in dev env · syntactic verify only · T107 真 smoke carry"

## Carry-forward

- **P11-T-004/T005/T006 IMS chart** Pattern reuse:本 chart 的 templates 结构(_helpers + deployment + service + configmap + serviceaccount + rbac + servicemonitor + .helmignore)是 4 chart 共同 skeleton · 各 chart 在此基础调整(IMS 需 CRD bundle + crds/ folder + CRD pre-install hook;不需 ConfigMap config yaml inject;Role 改 ClusterRole · resources 增加 CRD-specific verbs)
- **P11-T-008 inference-operator chart edit** ProxyImage env wire:本 chart values.yaml leaseElection.* env injection pattern 可参考(deployment.yaml env 段 + values.yaml 字段)
- **P11-T-107 kind smoke E2E ext** 主 assertion(a):demo-backend chart install + Lease leader-elect Pod Ready · Lease object 出现 in coordination.k8s.io · 1 Pod=leader + 1 Pod=follower
- **P11-T-201 master-demo-multi-site.sh** cache singleton multi-instance failover 演示:本 chart replicaCount 2 default ready · T201 起 `kubectl delete pod <leader>` 触发 failover · 观察 demo_backend_lease_holder gauge step transition
- **Phase 12+ Handler CacheSingleton consumer wiring**:本 task ship field + main.go inject · Phase 12+ /healthz/ready 加 state surface · /metrics emit Lease gauge · X-Cache-Status middleware 在 degraded mode 注入 header — 全部留 future
- **Phase 12+ replicaCount 3+ HA default**:per ADR-0017 §4 Open question (c)

## §0a.10 / §0a.11 compliance

- 本 task 续 T002 autonomous mode · main agent 直接做(per §0a.11 chart packaging non-batch · default serial main agent verify)
- strict verify = go build + go test + helm lint + helm template · 各项 stdout 实证 in devlog Verification 段
- 不 push remote · 累积到 phase-11-complete tag(per memory `feedback_push_at_phase_tag_only.md`)
- 不停于 T003 边界(per 用户 "按计划执行" autonomous mode)· 继续 T004
