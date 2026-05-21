# ADR-0013: O2 DMS Adapter — K8s Profile · 北向 NB shape · resource inventory + lifecycle reflection (Phase 9)

- **状态**:Accepted (design freeze; implementation P9-T-008 scaffold + P9-T-104 body)(2026-05-21 — Phase 9 P9-T-001)
- **日期**:2026-05-21
- **决策者**:协调者(用户)
- **相关**:ADR-0001 v3 §5(K8s baseline · 双轨路径 → K8s Profile only 自然匹配)+ §6(边缘 / 多站点 · Phase 9 启用 Karmada → ADR-0013 §6 forward note multi-cluster 路径)/ ADR-0002(不引入 KServe · ModelService 是被 reflect 的产品 CRD)/ ADR-0003 v2(IMS 7 服务 phasing · O2 DMS NB 与 IMS Core 关系)/ ADR-0008(PD Router webhook · inference-operator 内置 · O2 DMS 不感知 webhook · 仅 reflect ModelService spec/status)/ ADR-0009(npu-dra-driver · NPUSliceAllocation 是 inventory 一部分)/ ADR-0011(NPU 动态切分 · NPUSlicePool + NPUSliceTemplate substrate · O2 DMS reflect NPUSlicePool)/ ADR-0012(NPUVerticalScaler · O2 DMS reflect NPUVerticalScaler.status)/ ADR-0014(Multi-tenant Quota · §3 Consequences 注 Quota admission 发生在 K8s API server level · O2 NB 注入的 ModelService 同样过 Quota webhook)/ `docs/checkpoint-phase8.md` §6 Phase 9 candidate workstream #3 / `docs/phase9-plan.md` §3 P9-T-001 + §3 P9-T-008 scaffold + §4 P9-T-104 body / `docs/architecture.md` §1.3(Phase 路线图 · Phase 9 "O2 DMS 接口")§5(模块划分 · 加 `operators/o2-dms-adapter/`)§13(Phase 9 review-table O2 DMS 风险行)

---

## 上下文

Phase 8 (`phase-8-complete` @ e24f365 + P8-fix-001 series · CI gate dev HEAD 全绿)合上 M3 服务编排 milestone — 8-step Reconcile + PrometheusIngestor + AllocateBundle 控制器 wiring 全栈链通。Phase 9 开 **M4 工程化对外** milestone(arch §1.3),两大并列脊柱:(a) **O2 DMS Adapter** 作为对 O-RAN 联盟北向的生产契约 reflect 内部 ocloud 资源,(b) **Multi-tenant Quota CRD + admission webhook**(ADR-0014)落 安全模型 spine。本 ADR 锁定 O2 DMS Adapter 的 design freeze;ADR-0014 是 Quota spine 的对应 ADR。

`docs/checkpoint-phase8.md` §6 Phase 9 candidate workstream #3 列 "**O2 DMS adapter**(K8s Profile · per arch §1.3 Phase 9 row):北向 O2 IMS R1 接口 · 把 ocloud 资源(NPUSlicePool / ModelService / NPUVerticalScaler)reflect 到 O-RAN 北向 API"。`docs/architecture.md` §1.3 Phase 路线图 把 "O2 DMS 接口" 列为 Phase 9 唯一明示 deliverable(M4 工程化对外 milestone);§13 review-table Phase 9 行注 "锁定一个版本(如 O2 IMS R1)" 是早期 baseline 风险条目。`docs/architecture.md` §2.1 五层架构图 Layer 5(演示与对外层)预留 `O2 DMS Adapter` block 但无 module + 无 ADR — 本 ADR 把 placeholder 升为 design freeze。

**O-RAN spec 锁定**:O-RAN ALLIANCE WG6 O2 IMS Interface Specification 命名约定为 `O-RAN.WG6.O2IMS-INTERFACE-R<release>-v<version>.<patch>`(WG6 = Cloudification & Orchestration Working Group)。本 ADR 锁定基线 **`O-RAN.WG6.O2IMS-INTERFACE-R003-v04.00`**(以下简称 "O2 IMS R1 v04.00",per phase9-plan §3 paraphrase)。**Citation 强度**:[B · 二手源] — 直接访问 https://specifications.o-ran.org 在 T001 entry session 返回 portal load error;referenced via ATIS Open RAN Minimum Viable Profile V2(2025-02 published · 引用 `O-RAN.WG6.O2IMS-INTERFACE-R003-v04.00` + `R004-v07.00.00`)。**ATIS MVP Feb 2025 提到 R004-v07.00.00 已 released** — Phase 9 W2 T104 body landing 前 main agent 再做一次 portal re-check;若 R004 spec 与 R003 NB endpoint shape 不兼容(breaking change),则 T104 body 时评估 lock R004 vs 保持 R003 + Phase 10 polish 时升级 — 详 §5 Open question (a)。

**Phase 8 → Phase 9 boundary 视角**(checkpoint-phase8.md §6 末尾):Phase 8 = 单租户 busy-idle scaling + AllocateBundle wiring · Phase 9 = 多租户 fair scaling + Karmada + O2 DMS + Volcano + PromQL custom metric。本 ADR 落 O2 DMS 那一笔 — 是 Phase 9 中 *唯一* 引入新外部契约(O-RAN 北向) 的 workstream;Quota CRD(ADR-0014)+ Volcano(P9-T-101 decision-gated)+ PromQL(P9-T-007)+ Karmada(Phase 10 carry)都是内部 spine 或 build-in 集成,本 ADR 不展开。

**为什么 Phase 9 是 O2 DMS 的自然入口**:
1. ocloud 资源(NPUSlicePool / ModelService / NPUVerticalScaler / NPUSliceAllocation)substrate 在 Phase 3-8 已完整 ship(各 CRD types + controllers + status sub-resources)— O2 DMS 是 *read-mostly + lifecycle-thin* reflector,**不**引入新 substrate
2. O-RAN 联盟 O2 IMS R1 接口 spec 自 2022 起逐步 released(R001 → R002 → R003 v04.00 → R004 v07.00.00),Phase 9 入场点 spec 已稳定足够 lock 单一版本
3. M4 工程化对外 milestone 的另一脊柱(真硬件对接 + 演示打磨 · arch §1.3 Phase 10)与 O2 DMS 互不阻塞 — Phase 9 ship O2 DMS NB API + Phase 10 polish authn/z + Karmada + 真硬件 lab

**本 ADR 不涉及**:
- Quota CRD shape — 走 ADR-0014;O2 NB 注入的 ModelService 与用户直接 kubectl apply 的 ModelService 走同一 K8s API server 路径,Quota admission webhook 同样 enforce(本 ADR §3 Consequences 强调)
- 真硬件 lab 信号 — O2 DMS Adapter 是 K8s control-plane 组件,**不**触碰真硬件;Phase 10 lab smoke 走 ADR-0011 §3 default policy
- Multi-cluster propagation — Phase 9 single-cluster 是 acceptable demo(arch §13 Phase 9 多站点 demo 缓存重构 row 是 Phase 10 follow-on);Karmada policy-controller + PropagationPolicy + cross-cluster RBAC Phase 10 polish
- frontend Workload page 是否 surface O2 DMS endpoint indicator — Phase 10 polish per Phase 6 T102/T103 chat+ADR self-RFC pattern if needed

---

## 决策

### 1. Profile 选择(Decision A):K8s Profile only

**承诺范围**:

- Phase 9 commit **K8s Profile only** O2 DMS Adapter — 北向暴露 K8s-native primitives,**不** emit OpenStack Profile,**不**重新实现 Helm Profile
- 单 Profile 选择匹配 ADR-0001 v3 双轨路径精神(Edge KubeEdge + Standard-K8s small-cluster · 两端都是 K8s API);ADR-0002(不引入 KServe)的 ModelService Deployment 路径同样是 K8s-native
- 单 Profile keeps Phase 9 scope tractable(避免 Phase 9 scope 跨 OpenStack adapter 维护 · 那是 Phase 10+ federation 时若有 hybrid stack 需求才考虑的复杂度)

**OpenStack/Helm Profile 推迟逻辑**:
- O-RAN O2 IMS R1 spec 定义多个 Profile(K8s · OpenStack · Helm)以适配不同 O-Cloud 实现栈
- ocloud 是 K8s-native stack(Phase 1-8 全部 deliverable 都是 K8s primitives / CRDs)— K8s Profile 是 *唯一* 自然匹配
- 若未来出现 hybrid stack 客户(K8s + OpenStack)→ Phase 11+ 评估 dual Profile · 本 ADR §6 forward note 留链路

### 2. NB endpoint shape(Decision B):HTTP REST per O2 IMS R1 §3.1 NB 接口 contract

**承诺范围**:

- O2 DMS Adapter 暴露 HTTP REST API · base path `/o2dms/v1`
- 7 个 endpoint(详 §4 catalog)分两类:**inventory 反射**(read-only · 5 endpoints)+ **lifecycle**(create/delete ModelService 通过 O2 NB shape · 2 endpoints)
- 默认监听 `0.0.0.0:8088`(per P9-T-008 scaffold default)· production deployment 走 K8s Service ClusterIP 暴露 + Phase 10 Ingress / Gateway 选型
- Content-Type:`application/json`(O2 IMS R1 spec 默认)· OpenAPI 3.0 doc 生成在 P9-T-104 body landing 时 ship(`operators/o2-dms-adapter/api/openapi.yaml` · 与 `docs/api-contract.yaml` 区别 — `docs/api-contract.yaml` 是 demo-backend OpenAPI · O2 DMS Adapter 的 OpenAPI 是独立 NB 接口 doc)
- **Auth header 占位 Phase 9**:`Authorization: Bearer <static-token>`(env-var `O2DMS_BEARER_TOKEN` 注入)· Phase 10 polish 升级到完整 RBAC + OIDC(per §3 Consequences)
- **TLS**:Phase 9 ship HTTP only(K8s Service ClusterIP 内 cluster 通信)· Phase 10 polish add cert-manager TLS termination(同 inference-operator webhook · P5-T-101 cert-manager 路径)

**不**走 gRPC / WebSocket / GraphQL — O2 IMS R1 §3.1 contract 是 HTTP REST,不破契约不引入额外协议。

### 3. Resource reflection 映射(Decision C):O2 IMS R1 ↔ ocloud 映射表

下表是 O2 IMS R1 spec NB-side resource 与 ocloud 内部资源的 6 行对照(per P9-T-001 acceptance "6-8 rows"):

| # | O2 IMS R1 NB resource | ocloud 内部资源 | 映射方向 | 备注 |
|---|---|---|---|---|
| 1 | `deploymentManager` | K8s cluster | 1:1 | Phase 9 single-cluster · `deploymentManagerId` = cluster UID(读 `kube-system/cluster-info` ConfigMap or kube-apiserver `cluster` URL hash);Phase 10 Karmada multi-cluster 时 1:N |
| 2 | `infrastructureInventory` | aggregated view over **NPUSlicePool** + **Node** + **NPU device** + **NPUSliceAllocation** | N:1 (聚合) | Phase 9 single-cluster 聚合 · NPU device 通过 ResourceSlice list + NPUSlicePool.status 反查;Node 通过 kube-apiserver core/v1.Node list |
| 3 | `deploymentItem` | **ModelService** (inference.ocloud.edge.example.com/v1alpha1 · Phase 5 P5-T-007) | 1:1 | Create / Get / List / Delete 走 inference-operator 现有 controller chain;O2 NB Create 时 validate against ModelService CRD schema + apply through K8s API server(过 inference-operator webhook + Quota webhook · ADR-0014) |
| 4 | `lifecycleOperation` | async operation queue (in-memory · Phase 9 process-local) | 1:1 | O2 NB Create / Delete 触发的 async ops 通过 `lifecycleOperationId` 追踪 · status:`pending` → `running` → `completed` / `failed`;Phase 9 in-memory · Phase 10 polish 评估 persistent backing(K8s ConfigMap / Lease / Redis)per arch §13 多站点 cache 重构 row |
| 5 | `subscription` | N/A (Phase 9 不 ship) | — | O2 IMS R1 spec 定义 subscription 机制(NB callback when inventory / lifecycle change)· Phase 9 **out of scope** · Phase 10 polish if needed(走 informer event → HTTP POST callback 路径) |
| 6 | `alarmEvent` | aggregated from K8s Events + Loki query | N:1 (聚合 · 暂不 ship) | Phase 9 **out of scope** · Phase 10 polish if needed(走 K8s `events.k8s.io/v1.Event` watch + filter on ocloud namespaces) |

**注**:O2 IMS R1 spec 定义更多 resource(`oCloudInfrastructure` · `swInventory` etc.)但 Phase 9 ship 范围 = inventory(map row 2)+ lifecycle(map row 3+4)。其余在 §6 forward notes 留 Phase 10+ 路径。

**NPUVerticalScaler 是否 reflect**:ADR-0012 NPUVerticalScaler.status.scaleHistory + lastScaleTime 是 ModelService 的 dynamic 行为信号 — 在 O2 IMS R1 view 下属于 `deploymentItem` lifecycle 状态而**不是**独立 resource。Phase 9 ship 把 NPUVerticalScaler 状态 inline 到 ModelService `deploymentItem` Get response 的 `extensions` 字段(O2 IMS R1 spec 允许 vendor extension fields 携带额外信息),**不**单独暴露 `verticalScaler` resource path。

### 4. Implementation 方式(Decision D):新模块 `operators/o2-dms-adapter/`

**承诺范围**:

- **新模块 path**:`operators/o2-dms-adapter/` (Phase 9 P9-T-008 scaffold + P9-T-104 body)
- **Binary**:standalone HTTP server · Go + gin 或 chi router(P9-T-008 scaffold 决策 · 默认 chi 因 lighter 依赖 + 与 inference-operator binary 解耦)
- **K8s client**:走 informer/lister(`sigs.k8s.io/controller-runtime` client cache · 同 inference-operator pattern) — read 性能 + watch-based stay-fresh
- **Write path**:read-only inventory(map row 1+2 reflect 走 informer cache) + Create/Delete ModelService 通过 controller-runtime client `Create()` / `Delete()` API call · **不**走 admission webhook(那是 inference-operator + ADR-0014 Quota webhook 的责任 · O2 DMS 仅注入 ModelService 让 K8s API server / webhook chain 决定 admit/reject · 失败时把 webhook reject 透传到 NB 调用方)
- **Helm chart**:`deploy/helm-charts/o2-dms-adapter/`(P9-T-008 scaffold ship · Deployment + Service + ServiceAccount + ClusterRole + ClusterRoleBinding · 11 resources expected)
- **RBAC**:read 权限 on NPUSlicePool / ModelService / NPUVerticalScaler / NPUSliceAllocation + Node + ResourceSlice + write 权限 on ModelService create/update/delete(仅必要 — 不给 NPUSlicePool / NPUSliceAllocation write)
- **CI 集成**:`.github/workflows/ci.yml` matrix 加 `o2-dms-adapter` 行(P9-T-008 scaffold) · root `Makefile` 加 `o2-dms-adapter-build` / `o2-dms-adapter-test` target

**为什么独立模块而不是合并到 inference-operator**:
- 职责分离:inference-operator 是 ModelService / NPUVerticalScaler controller binary(reconciliation logic);O2 DMS Adapter 是 NB API server(HTTP REST handler) — 两者生命周期不同(controller 长跑 reconcile · NB API server 响应外部 request),合并增加 binary 复杂度
- 故障域隔离:O2 DMS Adapter crash 不影响 inference-operator 内部 reconciliation;反之亦然
- 部署灵活:O2 DMS Adapter 可独立 replica(Phase 9 default replica=1 · Phase 10 polish HA 评估)· inference-operator 是 leader-elect single-replica
- Phase 10 polish 路径开放:O2 DMS Adapter 可独立加 Karmada 多站点 propagation · 不污染 inference-operator 内部 controller 模型

**与 P9-T-001 plan §3 表述对齐**:plan §3 "Phase 9 NEW operator module `operators/o2-dms-adapter/` standalone binary · reads K8s 资源 via informer/lister(no direct write to CRD substrate — read-only NB in Phase 9 + lifecycle ops via underlying inference-operator/pool-operator CRD write through controller chain)" — 本 ADR 决策表述与 plan 完全一致;唯一明示是 "lifecycle ops via underlying inference-operator/pool-operator CRD write through controller chain" 解释为 **O2 DMS Adapter 自己 call client.Create/Delete + 让 inference-operator 内部 controller chain 接管** (不是 O2 DMS Adapter 直接 call inference-operator 的 internal API)。

---

## 3. 后果

### 正面

- **arch §1.3 Phase 9 deliverable 承诺兑现**:本 ADR + P9-T-008 scaffold + P9-T-104 body 三件套合上 Phase 9 唯一明示 deliverable(M4 工程化对外 milestone 头条)。**2026-05-21 P9-T-104 body landed**:7 stub handlers 全 replaced with real impl(inventory.DynamicClient via dynamic + core K8s clients + translator package O2 ↔ unstructured · LifecycleOperationQueue in-memory · 13 handler tests + 8 translator tests = 21 cases · degraded NoopClient fallback on K8s config 不可达)· 详 `docs/devlog/phase-9-t104.md`。
- **arch §13 review-table Phase 9 row 解锁**:Phase 9 row "锁定一个版本(如 O2 IMS R1)" 在 §1 Context 锁定 R003-v04.00 + R004 forward-note + ATIS MVP cross-ref · 风险条目 close。
- **Phase 3-8 substrate 立即变现**:NPUSlicePool(Phase 3)/ NPUSliceAllocation(Phase 5)/ ModelService(Phase 5)/ NPUVerticalScaler(Phase 8)/ NPUSliceTemplate(Phase 7) — 5 个 CRD 同时被 inventory reflection 消费,跨 Phase 复用清晰可见。
- **inference-operator 解耦保持**:O2 DMS Adapter **不**修改 inference-operator 任何代码(P9-T-008 scaffold Forbidden Paths 明示)· inference-operator + npu-dra-driver + pool-operator 三个 controller binary 内部模型不变;O2 DMS Adapter 是 *上游 facade* 不是 *内部组件*。
- **ADR-0014 协同清晰**:O2 NB Create/Delete ModelService 走 K8s API server · 同时过 inference-operator webhook(P5 webhook · ADR-0008) + Quota admission webhook(P9 · ADR-0014)· 安全 / 限额 enforcement 在 API server 层统一,O2 DMS Adapter 不做平行 enforcement。
- **测试性**:7 endpoint 都是 HTTP handler · stub-routing test(P9-T-008 scaffold 7 cases)+ inventory reflection test + lifecycle test 都可以走标准 httptest pattern · 不需要真集群(P9-T-104 body 测试用 envtest + fake informer)。
- **CI 友好**:standalone Go module · `go build` / `go test` / `helm lint` / `helm template` 同 inference-operator pattern · CI workflow 矩阵 1 行扩展。

### 负面 / 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| **O2 IMS R1 spec 版本演进**(R004-v07.00.00 已 released per ATIS MVP Feb 2025 · 与 R003-v04.00 是否 NB endpoint shape 兼容未知) | T104 body landing 前 spec lock 选错 → 客户 / 集成对接方报 NB shape 不符合 R004 | T104 body landing 前 main agent re-WebFetch 一次 portal · 评估 R003 vs R004 兼容性 · 若 breaking → T104 body 评估 lock R004 + ADR-0013 §1 minor revision;若 compatible → 保持 R003 + R004 forward note;Phase 10 polish 必 re-eval |
| **Phase 9 single-cluster scope**(deploymentManager 1:1 cluster) | 多站点 federation 演示场景客户期待 | Phase 10 polish Karmada propagation · ADR-0013 §6 forward note 留路径 · arch §13 Phase 9 多站点 cache 重构 row 是 Phase 10 follow-on(P9-T-107 cache spike 是 docs-only · 不是 Phase 9 实施) |
| **Auth header 占位静态 token**(Phase 9 静态 bearer) | production 部署不安全 · 仅 Phase 9 demo / 内部测试可用 | helm chart README + DESIGN.md §2 明示 production 必须升级到 Phase 10 OIDC + RBAC · Phase 9 sample chart values.yaml 默认开启 helm hook 提示 "DO NOT use static token in production" |
| **NPUVerticalScaler inline reflection 而非独立 resource**(O2 IMS R1 spec 无 verticalScaler concept) | 客户 NB 消费时需要解析 `deploymentItem.extensions.verticalScaler` 不熟悉的 extension field | DESIGN.md §2 NB endpoint table + `operators/o2-dms-adapter/api/openapi.yaml`(T104 body)明示 extension shape · 提供 1-2 个 sample response 在 docs/ · Phase 10 polish 评估是否升级到独立 resource(取决于客户反馈) |
| **lifecycleOperation in-memory queue**(Phase 9 process-local · O2 DMS Adapter restart 丢) | NB 调用方 polling `GET /o2dms/v1/lifecycleOperations/{id}` restart 后 404 | Phase 9 接受 — single-replica + Phase 10 polish 评估 persistent backing(K8s ConfigMap / Lease / Redis · per arch §13 Phase 9 cache row · 走 P9-T-107 spike 评估三路径) |
| **inventory reflection 路径性能**(informer cache + aggregator) | 大 cluster(>100 nodes · >1000 NPUSliceAllocation)NB `GET /o2dms/v1/inventory` 响应时间长 | Phase 9 接受 — demo scale 单 cluster 数十 nodes · 数十 allocations;Phase 10 polish 评估 cache layer(short-TTL in-memory snapshot · per Phase 10 cache spike outcome) |
| **subscription / alarmEvent 不 ship**(Phase 9 out of scope per §2 Decision C 表 row 5+6) | 客户期待 NB 自动 callback (vs polling) | Phase 9 demo 只 ship polling-mode · 文档明示;Phase 10 polish add subscription(走 informer event → HTTP POST callback)+ alarmEvent(走 K8s events watch + filter)· §6 forward note 留链路 |
| **K8s Profile only · 不 ship OpenStack/Helm Profile**(per §2 Decision A) | 若客户栈是 OpenStack-based O-Cloud → 接不上 | Phase 9 接受 — ocloud 是 K8s-native stack(ADR-0001 v3);Phase 11+ 评估 hybrid stack scenario · 若 client 需求实质化 → 新 ADR 开 dual Profile · 现阶段 OpenStack adapter scope 不进 Phase 9-10 路线 |
| **NB API OpenAPI doc 与 CRD OpenAPI 双份**(`operators/o2-dms-adapter/api/openapi.yaml` 独立 · `docs/api-contract.yaml` 是 demo-backend OpenAPI · CRD OpenAPI 是 kubebuilder 生成)| 三个 OpenAPI doc 维护成本 + drift 风险 | Phase 9 接受 — 三者是不同 API surface(NB / demo / CRD);DESIGN.md §2 明示边界 · drift 由 P9-T-104 body 内 CI lint(`swagger-cli validate`)在 W2 加 CI workflow row 时 enforce |

---

## 4. NB endpoint catalog

下表是 Phase 9 ship 的 7 个 endpoint(per P9-T-008 scaffold + P9-T-104 body · 与 plan §3 P9-T-001 acceptance 末尾表 1:1 对齐):

| # | HTTP method | Path | 描述 | Phase 9 status | 内部映射 |
|---|---|---|---|---|---|
| 1 | `POST` | `/o2dms/v1/deploymentItems` | Create ModelService via O2 NB shape · validates against ModelService CRD schema + applies | T008 scaffold = 501 stub · T104 body = full | `client.Create(ctx, ms)` · 过 inference-operator webhook + ADR-0014 Quota webhook |
| 2 | `GET` | `/o2dms/v1/deploymentItems` | List ModelServices (cluster-scoped Phase 9) | T008 scaffold = 501 stub · T104 body = full | informer list of ModelService |
| 3 | `GET` | `/o2dms/v1/deploymentItems/{id}` | Get ModelService by O2 id | T008 scaffold = 501 stub · T104 body = full | informer Get + inline NPUVerticalScaler.status to `extensions` |
| 4 | `DELETE` | `/o2dms/v1/deploymentItems/{id}` | Delete ModelService (cascade to Deployment via inference-operator) | T008 scaffold = 501 stub · T104 body = full | `client.Delete(ctx, ms)` · inference-operator reconcile chain handles cascade |
| 5 | `GET` | `/o2dms/v1/inventory` | Aggregated inventory (NPUSlicePool + Node + NPU + NPUSliceAllocation) | T008 scaffold = 501 stub · T104 body = full | aggregator reads informer caches · Phase 9 single-cluster · response 形态见 DESIGN.md §2 |
| 6 | `GET` | `/o2dms/v1/deploymentManagers` | Cluster manager metadata (single entry Phase 9) | T008 scaffold = 501 stub · T104 body = full | reads `kube-system/cluster-info` ConfigMap · Phase 9 1:1 cluster |
| 7 | `GET` | `/o2dms/v1/lifecycleOperations/{id}` | Pending / running / completed ops queue | T008 scaffold = 501 stub · T104 body = full | in-memory map · keyed by op id · TTL 1h · Phase 10 polish 评估 persistent backing |

**OpenAPI 3.0 doc**:`operators/o2-dms-adapter/api/openapi.yaml`(P9-T-104 body ship · 不在 T008 scaffold 范围)· `swagger-cli validate` 在 P9-T-104 body CI lint 中 enforce。

**Error responses**:统一 JSON envelope `{"code": <int>, "message": <string>, "details": <opt>}` · 错误码遵循 O2 IMS R1 §3.1 标准 + K8s API server reject(webhook reject)透传成 422 Unprocessable Entity · 详 DESIGN.md §4 错误处理。

---

## 5. Open questions

### (a) O2 IMS R1 v05.00 / R004-v07.00.00 release timing(基线 spec 演进)

- 现状:**R003-v04.00** 是 Phase 9 lock 基线 [B · 二手源 ATIS MVP Feb 2025 cross-ref];**R004-v07.00.00** 已 released per ATIS MVP Feb 2025 引用,但 portal 直接访问失败,具体 release 日期未确认
- 决策点:Phase 9 W2 T104 body landing 前 main agent re-WebFetch 一次 https://specifications.o-ran.org · 评估 R004 vs R003 NB endpoint shape 兼容性
- 决策分支:
  - **Compatible** (R004 minor revision · NB endpoint shape 不 break R003 v04.00) → 保持 R003 lock · R004 forward note 在 §6 留;Phase 10 polish 时 re-eval upgrade
  - **Breaking change** (R004 NB endpoint shape 与 R003 不兼容) → T104 body 评估 lock R004 · ADR-0013 §1 minor revision + change log + helm chart values 加 spec version env-var
- Default:per P3 conservative · stay R003-v04.00 unless re-WebFetch 显示 R004 已成事实 industry baseline(ATIS MVP 已引用 R004 → 现已部分 satisfy condition;需 portal re-check confirm)

### (b) Cross-cluster propagation(Phase 10 路径)

- 现状:Phase 9 single-cluster(deploymentManager 1:1 cluster · §2 Decision C table row 1)
- 决策点:Phase 10 polish 时 Karmada policy-controller / PropagationPolicy / cross-cluster RBAC 集成路径
- 决策分支:
  - **Karmada-native**:O2 DMS Adapter 部署在 Karmada control plane · deploymentManager list 反映成员 cluster · ModelService create 通过 Karmada PropagationPolicy 跨 cluster propagate
  - **Per-cluster Adapter**:每个 cluster 独立部署 O2 DMS Adapter · NB 调用方需要分别访问 N 个 endpoint · Karmada 不感知
- Default Phase 10:Karmada-native(per arch §1.3 Phase 9 路线图 Karmada forward · ADR-0001 §6 边缘+多站点 spirit)
- Phase 9 不预占 — 单 Adapter binary 支持上述任一部署模式(配置驱动)

### (c) Auth header convention(Phase 10 polish)

- 现状:Phase 9 静态 bearer `Authorization: Bearer <static-token>` · env-var `O2DMS_BEARER_TOKEN`
- 决策点:Phase 10 polish 升级到完整 RBAC + OIDC
- 决策分支:
  - **OIDC + K8s ServiceAccount token**:复用 K8s SA token + Bearer Token Webhook · O2 DMS Adapter 调用 K8s `TokenReview` API 验证;ServiceAccount + RBAC 决定 NB resource 访问粒度
  - **API key + 独立 IAM**:独立 API key store(Secret · 或外部 IAM)· 不复用 K8s SA · 简单但需独立维护
- Default Phase 10:OIDC + K8s SA token + TokenReview(per arch §13 安全模型 row · Karmada RBAC 联动)· 同 inference-operator webhook cert-manager 路径 spirit · 复用 K8s 现有 identity

### (d) NB OpenAPI vs CRD OpenAPI 重复

- 现状:**3 个 OpenAPI doc**:
  1. `operators/o2-dms-adapter/api/openapi.yaml`(P9-T-104 ship · NB API)
  2. `docs/api-contract.yaml`(Phase 1 P1-T-002 ship · demo-backend API · CLAUDE.md 共享契约)
  3. CRD OpenAPI(kubebuilder 生成 · `operators/inference-operator/config/crd/bases/*.yaml` · ModelService / NPUVerticalScaler / Quota 等)
- 决策点:3 个 doc 是否合并 / 引用 / drift control
- 决策分支:
  - **保留三份独立**(本 ADR default):三者是不同 API surface(NB / demo / CRD storage)· DESIGN.md §2 明示边界 · drift 由 CI lint enforce(`swagger-cli validate` + 跨文件 spec ref check)
  - **合并 NB + CRD**:NB endpoint POST body 直接 ref CRD OpenAPI · 减重复 · 但 NB shape 可能要 transform(O2 NB envelope vs CRD raw)· 复杂度增
- Default Phase 9:保留三份 · drift via CI lint(P9-T-104 body CI workflow 加 `swagger-cli validate` step)· Phase 10+ 若 NB shape 与 CRD shape 高度同构 → 评估 merge / ref 路径

### (e) lifecycleOperation 持久化(Phase 10 polish · cross-ref P9-T-107 cache spike)

- 现状:Phase 9 in-memory map · O2 DMS Adapter restart 丢
- 决策点:Phase 10 polish 时是否升级 persistent backing
- 决策分支:per P9-T-107 cache spike outcome(arch §13 Phase 9 row · LRU → Redis/stateless 三路径 evaluation · ADR-0015 draft outline for Phase 10 决策)
- Default Phase 9:接受 in-memory + 1h TTL · NB 调用方 polling 间隔典型 < 1h 不构成实际风险
- Cross-ref:P9-T-107 spike 输出影响 inventory cache(map row 2)+ lifecycleOperation queue(map row 4)+ demo-backend LRU cache(Phase 4 P4-T-007)三者一并

---

## 6. Forward notes(Phase 10+)

> 🆕 **Phase 10 polish**:authn/z 完整实现 — OIDC + K8s SA + TokenReview + RBAC enforcement(per Open question (c))· production-grade NB API gateway 替代 Phase 9 静态 bearer。

> 🆕 **Phase 10 polish**:Karmada multi-cluster federation — O2 DMS Adapter 部署在 Karmada control plane · deploymentManager 反映成员 cluster list · ModelService 跨 cluster propagation 通过 PropagationPolicy · cross-ref ADR-0001 §6 + arch §13 Phase 9 安全模型 row 子项 "Karmada RBAC 联动"。

> 🆕 **Phase 10 polish**:O2 IMS R1 R004-v07.00.00 upgrade evaluate — 基于 §1 Context citation 强度 [B · ATIS MVP Feb 2025 二手源] + T104 body landing 前 re-WebFetch 直接源结果(Open question (a))· 若 R004 已成 industry baseline + breaking changes manageable → 升级 lock 到 R004 · 否则 stay R003 + Phase 11+ re-eval。

> 🆕 **Phase 10 polish**:subscription + alarmEvent 引入(per §2 Decision C 表 row 5+6 Phase 9 out of scope)· subscription 走 informer event → HTTP POST callback;alarmEvent 走 K8s `events.k8s.io/v1.Event` watch + filter on ocloud namespaces · NB shape 遵循 O2 IMS R1 spec subscription/alarm sections。

> 🆕 **Phase 10 polish**:frontend Workload page extension surface O2 DMS endpoint indicator + NPUVerticalScaler scaleHistory display(per phase9-plan §1 Scope summary "Out of scope" 行 · Phase 6 T102/T103 chat+ADR self-RFC pattern if needed mid-Phase 10)· 不在 O2 DMS Adapter binary scope · 走 demo-backend / frontend module。

> 🆕 **Phase 10 polish**:`lifecycleOperation` persistent backing(per Open question (e) · cross-ref P9-T-107 cache spike outcome)· 三路径 evaluation:Redis-backed shared queue · K8s ConfigMap snapshot · stateless dispatch + per-request fetch · 与 inventory cache + demo-backend cache 三者一并 ADR-0015 draft outline。

> 🆕 **Phase 11+ 候选**:dual Profile(OpenStack + K8s)· 仅当客户实质化 hybrid stack 需求出现 · 新 ADR 开 OpenStack Profile adapter · 与现 K8s Profile adapter 并列 · 共享 NB endpoint shape contract。

> 🆕 **Phase 11+ 候选**:NB API gRPC variant — 若客户 / 集成对接方明示需求 gRPC · 在 HTTP REST baseline 之上加 gRPC service · 不替换 HTTP · O2 IMS R1 spec 当前是 HTTP-only,gRPC 是 vendor extension。

> 🆕 **Phase 11+ 候选**:Multi-tenant namespace mapping — Phase 9 ship single-namespace ModelService reflection(NB endpoint 无 namespace 字段 · 默认 namespace 通过 env-var 配置)· Phase 11 若多租户 + namespace-as-tenant 需求 → NB endpoint 加 `namespace` 字段或 path prefix `/o2dms/v1/namespaces/{ns}/deploymentItems` · 不破现有 path;cross-ref ADR-0014 namespace-scope Quota substrate。

---

## 推翻条件

| 决策项 | 推翻条件 |
|---|---|
| K8s Profile only | 客户实质化 hybrid stack 需求(K8s + OpenStack)→ Phase 11+ 评估 dual Profile · 新 ADR · K8s Profile baseline 保留 |
| HTTP REST NB shape | O-RAN spec 演进 deprecate HTTP REST → 评估 gRPC / GraphQL · 不破当前 R003 v04.00 兼容 · 走 backward-compat envelope |
| `/o2dms/v1` base path | O-RAN spec v05.00+ 重命名 base path → 评估添加 `/o2dms/v2` 并行路径 · v1 backward-compat 保留 N phases |
| 独立 binary `operators/o2-dms-adapter/` | inference-operator binary 复杂度严重失控 → re-eval merge / split · 反之 O2 DMS scope 严重 shrink 也保持独立(独立模块开销已是 sunk cost) |
| informer/lister read 路径 + client.Create/Delete write 路径 | informer cache 在大集群延迟严重(>5s lag)→ Phase 10 评估 cache layer · 不破现 informer pattern |
| `R003-v04.00` lock | T104 body landing 前 re-WebFetch 显示 R004 已成 industry baseline + R003 deprecated → 升级 lock R004 · ADR §1 minor revision |
| Auth header 静态 bearer Phase 9 | Phase 9 demo / 内部测试出现 token 泄露 incident → 提前 Phase 10 OIDC + RBAC schedule;否则 Phase 10 polish 自然 schedule |
| Phase 9 single-cluster deploymentManager 1:1 | Phase 10 Karmada federation 落地 → deploymentManager 升 1:N · §2 Decision C 表 row 1 微调 · cross-ref Karmada ADR(Phase 10 起草) |
| `subscription` / `alarmEvent` Phase 9 out of scope | 客户实质化 polling-mode 体验差反馈 → Phase 10 polish schedule 提前 · §6 forward note 路径已留 |

---

## 引用

- `docs/adr/0001-phase0-key-decisions.md` v3 §5(K8s 1.31+ DRA · 双轨路径基线 — Edge KubeEdge + Standard-K8s small-cluster · 两端都是 K8s API · O2 DMS K8s Profile 选择自然匹配)+ §6(边缘 / 多站点 KubeEdge + Karmada · Phase 9 启用 Karmada — ADR-0013 §6 forward note Karmada multi-cluster 路径基线)
- `docs/adr/0002-no-kserve.md` — 推理服务全部基于 vllm-ascend Deployment + inference-operator · ModelService 是 O2 DMS reflect 的 `deploymentItem`(§2 Decision C table row 3)
- `docs/adr/0003-ims-services-phasing.md` v2 — IMS 7 服务 Option A:3 项 Phase 9 落 P9-T-IMS-{1,2,3} scaffold(P9-T-105 W2 任务)· O2 DMS NB 与 IMS Core 关系 cross-ref(本 ADR §1 Context 提)
- `docs/adr/0008-pd-router-webhook.md` — PD Router webhook 内置 inference-operator binary · O2 DMS 不感知 webhook · 仅注入 ModelService 让 webhook chain 处理(本 ADR §2 Decision D 强调)
- `docs/adr/0009-npu-dra-driver.md` — NPUSliceAllocation 是 inventory 一部分(§2 Decision C table row 2 聚合)
- `docs/adr/0011-npu-dynamic-slicing-and-source-interface.md` — NPUSlicePool + NPUSliceTemplate substrate · O2 DMS reflect NPUSlicePool(§2 Decision C table row 2)
- `docs/adr/0012-busy-idle-vertical-scaler.md` — NPUVerticalScaler · O2 DMS reflect NPUVerticalScaler.status inline 到 `deploymentItem.extensions`(§2 Decision C 末段 + §3 Consequences `extensions` 注)
- `docs/adr/0014-multi-tenant-quota.md` — Quota CRD + admission webhook · O2 NB 注入的 ModelService 同样过 Quota webhook(§3 Consequences ADR-0014 协同段)· ADR-0014 Phase 9 W1 起草(P9-T-002 docs · 与本 ADR 并列 Phase 9 spine)
- `docs/checkpoint-phase8.md` §6 Phase 9 handoff brief — workstream #3 "O2 DMS adapter K8s Profile"
- `docs/phase9-plan.md` §1 Scope summary "O2 DMS Adapter (arch §13 Phase 9 row HEADLINE · M4 spine)" 行 · §2 Task package overview T001 + T008 + T104 · §3 P9-T-001 acceptance criteria 1:1 mapping · §5 DoD W1 Foundation T001 + T008 / W2 T104 / Out of scope rows
- `docs/architecture.md` §1.3(Phase 路线图 · Phase 9 "O2 DMS 接口")· §2.1 Layer 5 演示与对外层 `O2 DMS Adapter` block · §5 模块划分(本 ADR §3 Consequences 触发 §5.9 加 `operators/o2-dms-adapter/` 行 · P9-T-001 Allowed Paths 小编辑)· §13 Phase 9 review-table O2 DMS 风险行(锁定一个版本 R1 v04.00 · 本 ADR §1 Context 解锁)
- `docs/architecture.md` 附录 A 名词对照 — O-Cloud · O2 · DMS · IMS · NB (北向接口)
- upstream:O-RAN ALLIANCE WG6 Cloudification & Orchestration Working Group · O2 IMS Interface Specification 系列(R001 → R002 → R003-v04.00 baseline lock · R004-v07.00.00 ATIS MVP Feb 2025 referenced · portal https://specifications.o-ran.org [B · 二手源 ATIS MVP cross-ref] T001 entry portal 不可达 · T104 body landing 前 re-check)
- upstream:ATIS Open RAN Minimum Viable Profile V2 (2025-02 published · `https://mvp.atis.org/wp-content/uploads/2025/02/ATIS-Open-RAN-Minimum-Viable-Profile-Report-Version-2-v8.pdf`) — 引用 R003-v04.00 + R004-v07.00.00
- upstream:`sigs.k8s.io/controller-runtime` client cache · informer/lister(本 ADR §2 Decision D 实现路径)
- upstream:K8s API server admission chain(本 ADR §2 Decision D + §3 Consequences ADR-0014 协同 — webhook reject 透传到 NB 422)

---

**END of ADR-0013**
