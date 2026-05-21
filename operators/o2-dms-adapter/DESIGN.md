# o2-dms-adapter — module detailed design

> O-RAN O2 DMS Adapter — K8s Profile NB API server per ADR-0013. Phase 9
> M4 工程化对外 spine. Standalone Go binary exposing 7 HTTP REST endpoints
> under `/o2dms/v1`. Phase 9 P9-T-008 scaffold ships stub handlers (501);
> Phase 9 W2 P9-T-104 body lands inventory aggregation + lifecycle ops.

---

## 1. 架构概览

### 1.1 模块在系统中的位置

```
                          ┌────────────────────────────────────┐
                          │  O-RAN North-bound consumer         │
                          │  (集成对接方 · operator dashboard) │
                          └─────────────┬──────────────────────┘
                                        │ HTTP REST
                                        │ /o2dms/v1/* + Bearer auth
                                        ↓
              ┌─────────────────────────────────────────────────────┐
              │ o2-dms-adapter (HTTP server · port 8088)             │
              │  ┌───────────────────────────────────────┐           │
              │  │ chi router (BasePath /o2dms/v1)        │           │
              │  └────────────┬──────────────────────────┘           │
              │               │                                       │
              │   ┌───────────┴────────────┐                          │
              │   │ api.Handler stubs (7)   │                          │
              │   │  · CreateDeploymentItem │  ┌──────────────────┐   │
              │   │  · ListDeploymentItems  │  │ inventory.Client │   │
              │   │  · GetDeploymentItem    ├──│ (informer/lister │   │
              │   │  · DeleteDeploymentItem │  │ wrappers · P9-T-104) │
              │   │  · GetInventory         │  └────────┬─────────┘   │
              │   │  · ListDeploymentManagers│           │              │
              │   │  · GetLifecycleOperation │           │              │
              │   └─────────────────────────┘           │              │
              └───────────────────────────────────────────│──────────┘
                                                          │ controller-runtime client
                                                          ↓
                          ┌───────────────────────────────────────┐
                          │ K8s API server                         │
                          │  · NPUSlicePool (ims.ocloud)           │
                          │  · NPUSliceAllocation (npu.ocloud)     │
                          │  · NPUSliceTemplate (npu.ocloud)       │
                          │  · ModelService (inference.ocloud)     │
                          │  · NPUVerticalScaler (inference.ocloud)│
                          │  · Node + ResourceSlice (core/resource)│
                          └───────────────────────────────────────┘
```

### 1.2 数据流

- **Read path**: NB consumer GET → chi router → handler → `inventory.Client`
  reads via informer/lister (P9-T-104) → aggregates `InfrastructureInventory`
  / lists DeploymentItem from ModelService informer → JSON response
- **Write path**: NB consumer POST `/deploymentItems` → handler decodes
  `DeploymentItemCreateRequest` → translates to ModelService manifest →
  `client.Create(ctx, ms)` → K8s API server admission chain (inference-
  operator webhook + ADR-0014 Quota webhook) decides admit / reject →
  response (201 / 422 with reject reason)

### 1.3 Phase 9 W1 scaffold vs W2 body

Phase 9 P9-T-008 W1 ships **only stubs** (501 Not Implemented) — proves
HTTP routing wired through chi from cmd/main.go to handler methods.
Phase 9 W2 P9-T-104 fills `internal/api/handlers.go` + `internal/
inventory/client.go` with real logic.

---

## 2. 接口契约

### 2.1 NB endpoint catalog (7 endpoints · ADR-0013 §4)

| # | Method | Path | Phase 9 scaffold | Phase 9 W2 body status |
|---|---|---|---|---|
| 1 | `POST` | `/o2dms/v1/deploymentItems` | 501 stub | `client.Create(ModelService)` |
| 2 | `GET`  | `/o2dms/v1/deploymentItems` | 501 stub | informer list |
| 3 | `GET`  | `/o2dms/v1/deploymentItems/{id}` | 501 stub | informer Get + inline NPUVerticalScaler.status to `extensions` |
| 4 | `DELETE` | `/o2dms/v1/deploymentItems/{id}` | 501 stub | `client.Delete(ModelService)` |
| 5 | `GET`  | `/o2dms/v1/inventory` | 501 stub | aggregator over NPUSlicePool + Node + NPU + NPUSliceAllocation |
| 6 | `GET`  | `/o2dms/v1/deploymentManagers` | 501 stub | reads `kube-system/cluster-info` ConfigMap |
| 7 | `GET`  | `/o2dms/v1/lifecycleOperations/{id}` | 501 stub | in-memory map keyed by op id · 1h TTL |

### 2.2 Error envelope

All 4xx / 5xx responses use the `types.ErrorEnvelope` JSON shape:

```json
{
  "code": 501,
  "message": "P9-T-008 scaffold stub · body lands in P9-T-104",
  "details": "endpoint=POST /o2dms/v1/deploymentItems · spec=O-RAN.WG6.O2IMS-INTERFACE-R003-v04.00"
}
```

### 2.3 Auth header convention (Phase 9 placeholder)

`Authorization: Bearer <static-token>` per ADR-0013 §2 Decision B Auth
placeholder rationale. Token read from env var `O2DMS_BEARER_TOKEN`
(injected via helm chart `values.auth.bearerToken`). Phase 10 polish
upgrades to OIDC + K8s SA TokenReview per ADR-0013 §5 Open question (c).

### 2.4 Spec lock

`O-RAN.WG6.O2IMS-INTERFACE-R003-v04.00` (per `types.SpecVersion` const).
ADR-0013 §1 Context · re-WebFetch portal at P9-T-104 entry to evaluate
R004-v07.00.00 upgrade per §5 Open question (a).

---

## 3. 生命周期

### 3.1 启动 (cmd/main.go)

1. Parse flags: `-addr=:8088` (default · ADR-0013 §2 Decision B)
   · `-shutdown-timeout=10s`
2. Construct `api.Handler` (sets `SpecVersion=types.SpecVersion`)
3. Construct `api.NewRouter(handler)` (chi router + RequestID +
   Recoverer middleware)
4. Start `http.Server` with conservative timeouts (Read 30s · Write 30s ·
   Idle 120s)
5. Signal handler: SIGINT/SIGTERM → graceful Shutdown within
   `shutdown-timeout` then exit

### 3.2 P9-T-104 W2 body 生命周期 extensions

- Manager startup: construct controller-runtime client + informer factory
  for NPUSlicePool / NPUSliceAllocation / ModelService / NPUVerticalScaler /
  Node / ResourceSlice · wait for caches synced before serving requests
- LifecycleOperation queue: in-memory map keyed by op id · 1h TTL
  cleanup goroutine · Phase 10 polish persistent backing (P9-T-107 cache
  spike outcome decides Redis vs K8s ConfigMap vs stateless)

### 3.3 关闭

- Graceful drain: `http.Server.Shutdown(ctx)` blocks until in-flight
  requests finish or `shutdown-timeout` expires
- Informer factory stop: P9-T-104 body adds `cancel()` on context to
  let informers exit

---

## 4. 错误处理

### 4.1 Phase 9 P9-T-008 scaffold

All 7 handlers return 501 Not Implemented with `types.ErrorEnvelope`.
Unknown paths under `/o2dms/v1` return 404 (chi default).

### 4.2 P9-T-104 body error model

- **K8s API server reject** (webhook reject · CRD validation fail):
  surfaced as 422 Unprocessable Entity · envelope.details carries the
  K8s status reason / message
- **Informer cache not synced**: 503 Service Unavailable + retry-after
  hint
- **Internal server error** (panic recovered by middleware): 500 +
  generic envelope · log + metrics emit
- **Auth failure** (missing / invalid bearer token): 401 Unauthorized
- **Resource not found** (Get DeploymentItem by ID not found): 404
  Not Found

### 4.3 NoData / degraded modes

Phase 9 informer-based read paths cannot serve until caches sync · server
returns 503 + retry-after during the bootstrap window. Phase 10 polish
adds health check endpoints (`/healthz` / `/readyz` per controller-
runtime convention).

---

## 5. 扩展点

### 5.1 Phase 10 polish layers (per ADR-0013 §6 forward notes)

- **authn/z full**: OIDC + K8s SA + TokenReview · production-grade NB
  API gateway · replaces Phase 9 static bearer
- **Karmada multi-cluster**: deployment manager list reflects Karmada
  member clusters · ModelService PropagationPolicy
- **R004-v07.00.00 spec upgrade evaluate**: re-WebFetch + compatibility
  matrix · lock R004 if NB endpoint shape compatible
- **subscription + alarmEvent**: O2 IMS R1 spec sections currently
  out of scope · Phase 10 polish HTTP POST callback
- **lifecycleOperation persistent backing**: P9-T-107 spike outcome
  decides (Redis / K8s ConfigMap / stateless)

### 5.2 Phase 11+ candidates

- dual Profile (OpenStack + K8s) when hybrid stack scenario emerges
- NB API gRPC variant alongside HTTP REST
- multi-tenant namespace mapping (NB endpoint `namespace` field or
  path prefix `/o2dms/v1/namespaces/{ns}/...`)

---

## 6. 集成示例

### 6.1 Inventory snapshot via curl (Phase 9 W2 body landed · expected)

```bash
curl -H "Authorization: Bearer ${O2DMS_BEARER_TOKEN}" \
     http://o2-dms-adapter.ocloud-system.svc:8088/o2dms/v1/inventory
```

Response: `types.InfrastructureInventory` JSON with `nodes[]` +
`slicePools[]` + `allocations[]`.

### 6.2 Create ModelService through NB (Phase 9 W2 body landed)

```bash
curl -X POST \
     -H "Authorization: Bearer ${O2DMS_BEARER_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{
       "name": "qwen-pd",
       "namespace": "ai-edge-demo",
       "modelImage": "vllm-ascend:v0.11.0",
       "slicePoolRef": "qwen-pool"
     }' \
     http://o2-dms-adapter.ocloud-system.svc:8088/o2dms/v1/deploymentItems
```

Response: 201 Created with `types.DeploymentItem` body OR 422 with
`types.ErrorEnvelope` if Quota / inference-operator webhook rejects.

---

## 7. 参考

- ADR-0013 (`docs/adr/0013-o2-dms-adapter.md`) — design freeze · spec
  lock · NB endpoint catalog · §1 Context cite chain (Phase 8 checkpoint
  §6 #3 + arch §1.3 + arch §13 + ATIS MVP V2 cross-ref)
- ADR-0014 (`docs/adr/0014-multi-tenant-quota.md`) — Quota admission chain
  · O2 NB-injected ModelService 走同一 K8s API server admission · 自然
  过 Quota webhook (Phase 10 polish 无需 special path)
- ADR-0003 v2 — IMS 3 项 scaffold P9-T-105 (Phase 9 W2) · O2 DMS NB 与
  IMS Core 关系 §"O2 DMS NB 与 IMS Core 关系" 段
- `docs/architecture.md` §1.3 Phase 路线图 · §2.1 Layer 5 演示与对外层
  · §5.8 模块 · §13 Phase 9 review-table O2 DMS 风险行
- `docs/phase9-plan.md` §3 P9-T-001 + §3 P9-T-008 + §4 P9-T-104
- CLAUDE.md §14.2 module DESIGN.md 7-section convention
- upstream: O-RAN ALLIANCE WG6 Cloudification & Orchestration WG ·
  O2 IMS Interface Specification R003-v04.00 (ATIS MVP Feb 2025 cross-ref)
- upstream: `sigs.k8s.io/controller-runtime` informer/lister · `chi`
  router · `gcr.io/distroless/static:nonroot` runtime base

---

**END of o2-dms-adapter DESIGN.md**
