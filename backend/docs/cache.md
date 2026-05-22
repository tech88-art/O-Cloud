# Demo-backend cache + Lease leader-elect (P10-T-006 substrate + P11-T-003 chart wire)

> Module docs for `backend/pkg/cache/`. ADR-0015 owns the design rationale;
> this file documents the **implementation**: what's wired where, env vars
> the chart sets, and the Pod lifecycle from chart install to leader
> handoff to graceful shutdown.

## §1 Components

- **`backend/pkg/cache/lru.go`** — in-process LRU per resource (P3-T-008)
- **`backend/pkg/cache/singleton.go`** — Lease singleton state machine
  (P10-T-006 substrate · ADR-0015 §3.3 Decision B · `Singleton` struct with
  `Follower / Leader / Degraded` SingletonState + callbacks
  `OnLeaseAcquired / OnLeaseLost / OnLeaseRenewError / OnLeaseRenewRecovered`)
- **`backend/pkg/cache/leaderelect.go`** — client-go `tools/leaderelection`
  wiring (P11-T-003 · `RunLeaderElection` blocks on the lease loop, drives
  Singleton state transitions, logs each handoff)
- **`backend/cmd/demo-backend/main.go`** — startup orchestrator that loads
  `cfg.Lease` from Viper, constructs the Singleton + launches the lease
  goroutine, exposes Singleton to handlers via `Handler.CacheSingleton`

## §2 Lease config flow

The chart at `deploy/helm-charts/demo-backend/values.yaml` ships defaults
that mirror ADR-0015 §2 Decision B (LeaseDuration 15s · RenewDeadline 10s
· RetryPeriod 2s). Override paths:

```
values.yaml leaseElection.* fields
  ↓ rendered by templates/deployment.yaml as env vars
OCEDGE_LEASE_ENABLED=true
OCEDGE_LEASE_NAMESPACE=ocloud-system
OCEDGE_LEASE_NAME=demo-backend-leader
OCEDGE_LEASE_DURATION_SECONDS=15
OCEDGE_LEASE_RENEW_DEADLINE_SECONDS=10
OCEDGE_LEASE_RETRY_PERIOD_SECONDS=2
  ↓ Viper picks up via envPrefix=OCEDGE + AutomaticEnv
cfg.Lease (backend/pkg/config.LeaseConfig)
  ↓ cmd/demo-backend/main.go converts to cache.LeaseConfig via cache.ConfigFromOptions
  ↓ cache.NewSingleton(leaseCfg) constructs the Singleton (default-applies + validates)
  ↓ go cache.RunLeaderElection(ctx, singleton, opts) launches the lease loop
```

## §3 Chart wiring + Lease RBAC + leader-elect Pod lifecycle (P11-T-003)

### §3.1 Chart files (`deploy/helm-charts/demo-backend/`)

| File | Purpose |
|---|---|
| `Chart.yaml` | apiVersion v2 · name demo-backend · version 0.1.0 · appVersion 0.1.0 |
| `values.yaml` | replicaCount 2 default · leaseElection.* tuning · service / probes / resources / ServiceMonitor toggles |
| `templates/_helpers.tpl` | name + fullname + labels + selectorLabels + serviceAccountName |
| `templates/deployment.yaml` | 2-replica Deployment · downward API POD_NAME + POD_NAMESPACE · OCEDGE_LEASE_* env injection · readinessProbe + livenessProbe on /healthz · configmap volume + emptyDir /tmp |
| `templates/service.yaml` | ClusterIP service on port 8080 (REST + /metrics same port) |
| `templates/configmap.yaml` | Viper-format YAML config with mock datasource baseline |
| `templates/serviceaccount.yaml` | SA the Deployment runs as · scoped per `serviceAccount.create` flag |
| `templates/rbac.yaml` | namespaced Role+RoleBinding · coordination.k8s.io/v1 Lease verbs `get/list/watch/create/update/patch/delete` in leaseElection.namespace |
| `templates/servicemonitor.yaml` | Optional Prometheus Operator ServiceMonitor scraping /metrics (off by default · `serviceMonitor.enabled=true` to opt in) |
| `.helmignore` | standard ignore patterns |

### §3.2 RBAC scope (Lease only · ADR-0015 §2 Decision B)

The chart's RBAC is intentionally minimal:
- **Role + RoleBinding** (NOT ClusterRole) — Lease access is scoped to
  `leaseElection.namespace` (default `ocloud-system`)
- **Resources**: `coordination.k8s.io/v1.Lease` only — demo-backend does
  NOT need to read/write any other K8s resource for the singleton loop
- **Verbs**: `get list watch` (lease state observation) + `create update
  patch` (lease renewal) + `delete` (graceful release on shutdown)
- **No ClusterRole**: keeps blast radius bounded · demo-backend cannot
  read Secrets / Pods / Nodes via this SA

### §3.3 Pod lifecycle

```
T+0  Pod startup (replica N of replicaCount=2)
T+~1s  Viper loads /etc/demo-backend/config.yaml + env overrides
T+~1s  main.go constructs Singleton (state=Follower) + launches RunLeaderElection goroutine
T+~1s  HTTP server listening on :8080 (readinessProbe serves /healthz → 200 immediately)
T+~3-15s  client-go lease library tries to acquire `demo-backend-leader` Lease
   ├── If no existing holder OR existing holder expired → acquire → OnStartedLeading → Singleton state=Leader
   └── If active holder seen → stay Follower · retry every RetryPeriod (2s)
T+steady  Leader renews lease every (LeaseDuration - RenewDeadline) ~= 5s
   ├── Renew OK → metric demo_backend_lease_renewals_total{result=ok} ++ · stay Leader
   └── Renew error → OnLeaseRenewError callback fires → state=Degraded · cache reads return X-Cache-Status: stale
T+steady  Follower watches lease via informer · ready to acquire on RenewDeadline expiry
T+shutdown  SIGTERM → main.go cancels rootCtx → RunOrDie returns (ReleaseOnCancel=true)
   ├── Leader: lease.Delete API call (cooperative release) → next watcher acquires within RetryPeriod
   └── Follower: goroutine returns immediately
T+shutdown+10s  HTTP server graceful shutdown (shutdownGrace) → Pod terminates
```

### §3.4 Failover SLA observation (per ADR-0015 §3.2)

When `T201 master-demo-multi-site.sh` (P11-T-201) exercises 2-member
multi-cluster propagation, watch the 3 metrics from ADR-0015 §3.3
Decision D:

- `demo_backend_lease_holder{pod="<podname>"}` gauge — 1 on leader · 0
  elsewhere · expect step-function transition during failover
- `demo_backend_lease_renewals_total{result=ok|error}` counter — error
  count climbs during induced disruption · 0 in steady state
- `demo_backend_cache_hit_ratio` gauge — drops then recovers during
  failover (cold cache on new leader)

Target failover latency: 5-30s (per ADR-0015 §3.2). If observation
exceeds 30s sustained → trim LeaseDuration to 10s + RenewDeadline 7s
(see ADR-0015 §4 Open question (b)).

### §3.5 Deferred to Phase 12+ (per ADR-0015 §3.5 trade-offs)

- **HA Pod count default 3+**: current default 2 is demo-grade. Per
  ADR-0017 §4 Open question (c), Phase 12+ production polish flips
  default to 3 with weighted quorum + Karmada control HA cohort.
- **Redis-backed shared cache**: ADR-0015 §3.3 keeps the interface ready
  for additive rewrite. Trigger = production SLO signal (failover < 1s
  · zero stale window).
- **Event-driven OnLeaseRenewError 3-failure threshold**: current
  implementation flips Leader → Degraded on first error (Phase 10 W1
  minimum). 3-failure threshold + backoff window per ADR-0015 §4 Open
  question (b) revisit.

## §4 Local development without K8s

Set `OCEDGE_LEASE_ENABLED=false` (or omit env entirely) → cache.Singleton
is not constructed, RunLeaderElection is not invoked, Handler.CacheSingleton
stays nil. The /healthz handler short-circuits gracefully when nil. This
is the path used by `backend/configs/config.dev.yaml` + the local
docker-compose dev stack (Phase 11 P11-fix-001 path A).

## §5 References

- ADR-0015 §1-§5 — cache strategy design rationale
- ADR-0017 §2 Decision D — chart packaging spine priority (T003 1st)
- ADR-0017 §4 Open question (c) — replicaCount Phase 12+ trajectory
- `docs/checkpoint-phase10.md` §6 #1 — Phase 11+ handoff brief for chart packaging
- `docs/phase11-plan.md` §3 P11-T-003 — task package spec
- `docs/devlog/phase-10-t006.md` — singleton substrate landing
- `docs/devlog/phase-11-t003.md` — chart packaging + main.go wiring trail
