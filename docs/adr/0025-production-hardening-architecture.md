# ADR-0025: 生产硬化架构(Bucket A · authz / secret / quota 真强制 / Karmada HA / P99 SLA · → build-doc §5 清单逐项 close · right-sized reference-grade)

- **状态**:Accepted(Phase 13 W1 · per Phase 13 plan P13-T-003 · 2026-06-03 · 承 ADR-0023 §2 Decision D Bucket A scope)
- **日期**:2026-06-03
- **决策者**:协调者(用户 · 2026-06-02 收官决策)
- **相关**:ADR-0023 §2 Decision D(Bucket A scope = build-doc §5 清单逐项 · 本 ADR 是其架构展开)+ §2 Decision B(M7 不 over-claim · 本 ADR §3 right-sizing 是其 enforcement)/ ADR-0013(O2 DMS adapter · §6 forward note authn full = 本 ADR §2 Decision A 上游)/ ADR-0014(多租户 Quota · §6 Open question (b)+(c) cluster-scope ClusterQuota + cross-cluster propagation = 本 ADR §2 Decision C 上游)/ ADR-0018(Karmada deployment topology · §2 = 本 ADR §2 Decision D HA 上游 · §2 真物理 multi-region LB lab-gated)/ ADR-0008(PD Router webhook · 早标 p99 隐患 = 本 ADR §2 Decision E SLA 上游)/ `docs/build-and-production-validation.md` §5(生产硬化验收清单 6 项 = 本 ADR Bucket A 逐项 DoD · 第 6 项 Go v1 migration 是 carry 非本 ADR)/ 代码 grep-verified:`operators/o2-dms-adapter/internal/authn/authn.go`(Validator 接口 + 3 impl)+ `operators/inference-operator/api/v1alpha1/clusterquota_types.go`(ClusterQuotaUsage.Total/PerCluster + RecomputeTotal)+ `deploy/karmada/`(P11-T-102 bootstrap)+ `backend/pkg/datasource/prometheus/`(静态 Bearer)

---

## §1 Context

### §1.1 build-doc §5 生产硬化验收清单(6 项 · 5 项进 Bucket A)

`docs/build-and-production-validation.md` §5 列 6 项 "设计在/实现未做" 的生产硬化清单:

1. 认证授权(OIDC IdP + K8s SA + TokenReview + RBAC · 替换 O2 DMS 静态 Bearer)
2. 多租户配额强制(ClusterQuota webhook B scale-rate + 跨集群 usage reconcile + fail-open)
3. Karmada 多站点 HA(control-plane HA + 跨集群 PropagationPolicy + cross-cluster RBAC)
4. Secret 管理(静态 token/明文 → Vault 注入)
5. 推理 SLA(vLLM PD P99 延迟压测达标)
6. Go v1 ResourceSlice schema 迁移(随 K8s 1.36 · 5 模块 lockstep)—— **carry · 移出核心交付**(ADR-0023 §2 Decision F · 触发条件 = lab K8s ≥1.36 · ADR-0024 §4(a))

**本 ADR 覆盖第 1-5 项**(Bucket A = T201-T206)· 第 6 项不在本 ADR。

### §1.2 已 ship substrate(接口/schema 已落 · 缺 body/wiring)

grep-verified(本 task 决策时读):

- **authn**(P10-T-103):`authn.Validator` 接口(`Name()` + `Validate(r) error`)+ 3 impl —— `PlaceholderBearerValidator`(静态 token)· `OIDCValidator{IssuerURL,Audience,AllowedClaims}`(现 IssuerURL 空即 `ErrInvalidToken` fail-safe placeholder)· `K8sTokenReviewValidator{Audiences,ReviewClient interface{}}`(现 ReviewClient nil 即 ErrInvalidToken)· `Middleware(v Validator)` helper 已 ship · o2-dms `main.go` 未 wire middleware
- **ClusterQuota**(P11-T-104):`ClusterQuota`(cluster-scoped)· `ClusterQuotaSpec.Enforcement.MaxSliceAllocations` · `ClusterQuotaStatus.Usage ClusterQuotaUsage{Total QuotaUsage, PerCluster map[string]QuotaUsage}` · `RecomputeTotal()`(sum PerCluster → Total · idempotent)· webhook B 现仅 namespace 级(`Quota`)· scaler 无 ClusterQuota 读路径
- **Karmada**(P11-T-102/T103):`deploy/karmada/` bootstrap(host + 2 member kind cluster · 4 PropagationPolicy · push mode)· 单 control-plane · 无 HA topology
- **backend prometheus**:`backend/pkg/datasource/prometheus/` 静态 Bearer token(非 SA token)

### §1.3 用户收官决策 → deliverable 视角 right-size

ADR-0023 §2 Decision B/D:M7 收口 · Bucket A = **reference-grade right-sized**(非 gold-plated production program)· 每 task 1:1 关一个 build-doc §5 项 · 不为完整性堆 padding(P3 + M4 价值聚焦)。

---

## §2 Decision

### §2.1 Decision A:authz model(K8sTokenReview 主路径 + OIDC 外部 IdP · Dex 参考 · backend SA token)

- **主路径** = o2-dms `K8sTokenReviewValidator`(in-cluster ServiceAccount token · `authentication.k8s.io/v1.TokenReview` API · 集群内组件互信免外部 IdP)· **外部路径** = `OIDCValidator`(JWKS fetch IssuerURL + JWT 签名/claims aud/iss/exp/AllowedClaims 验证)
- **T201**:填 `OIDCValidator.Validate`(JWKS+JWT)+ `K8sTokenReviewValidator.Validate`(构造 TokenReview + 调 K8s API · 引 client-go)· 去未配置即短路 · wire `Middleware(validator)` 到 o2-dms chi router(config 选 validator)· 替换 `PlaceholderBearerValidator`(保留 env-driven local-dev)
- **backend prometheus**:静态 Bearer → in-cluster ServiceAccount token(`/var/run/secrets/...` mount · 自动 rotate)
- **T202**:**Dex 参考 IdP** 部署(OIDC issuer · o2-dms/backend audience)+ 各 chart 最小权限 RBAC manifest(Role/ClusterRole + Binding · backend deploy/svc create · scaler quota read · TokenReview SA)· Dex = 甲方 swap point(§4(a))

### §2.2 Decision B:secret 管理(external-secrets-operator + Vault backend)

- **T203**:external-secrets-operator + Vault backend(`ExternalSecret` CR 指向 Vault path · `SecretStore`)· 替换 `config.real.yaml` 硬编码 URL/token + install.sh 静态拷贝
- helm `*secret*.yaml`/`externalsecret.yaml` · install.sh external-secrets install + Vault approle setup 分支 · config.real.yaml env-from-Secret
- Vault = 参考 backend · 甲方 swap point(§4(a))

### §2.3 Decision C:quota 真强制(webhook B 读 ClusterQuota.status.usage.Total + cross-cluster reconcile)

- **T204**:webhook B 加 `ValidateClusterQuota()` path —— 读 cluster-scoped `ClusterQuota` → 校验 `status.usage.Total.CurrentSliceAllocations >= spec.enforcement.MaxSliceAllocations` → 拒/放 · scale-rate(`maxScaleEventsPerWindow`)真强制 · **fail-open 兜底**(ClusterQuota 缺失/读失败 → 放行 · 记 warning · 防 admission 把集群锁死)
- `NPUVerticalScaler` controller:scale action 前读 `ClusterQuota.status.usage.Total` · 校验 scale 不超 cluster cap
- `clusterquota_controller.go` tick reconcile:聚合 namespace usage → `ClusterQuotaStatus.Usage` · Karmada `PerCluster` populate(member cluster usage 累计)· 调既有 `RecomputeTotal()`
- **T204 软依赖 T205**(cross-cluster usage populate 需 Karmada HA path)

### §2.4 Decision D:Karmada control-plane HA(3-replica + 外置 etcd)

- **T205**:control-plane(apiserver/scheduler/controller-manager)`replicas: 3` + 外置 etcd 集群(或 HA etcd statefulset)+ anti-affinity · LB front 控面 · propagation 加固(replica scheduling + failover + cross-cluster RBAC propagation)
- **真物理 multi-region LB bind 真网络 = lab-gated**(ADR-0018 §2 · §3 right-sizing)· HA 主体(3-replica + 外置 etcd + dry-run 渲染)在 kind 多集群验

### §2.5 Decision E:P99 SLA(load-test harness + default SLO + scaler latency feedback)

- **T206**:`tests/sla/` load-test harness(并发请求 Qwen 8B PD · 测 P99)· **文档化 default SLO**(`tests/sla/README.md` · 甲方 swap · §4(b))· scaler P99 latency query(Prometheus/PD Router metric)→ scale feedback(busy/idle slice 调整)· P99 latency collector(`internal/metrics/`)
- 达标判据用 default SLO · 真硬件 P99 测量(T206 真机层)· 甲方阈值 swap

---

## §3 Right-sizing 声明(reference-grade · 防 P3 gold-plating · ADR-0023 §2 Decision B enforcement)

每 Bucket A 项明确 **reference-grade 边界** —— close build-doc §5 缺口,**不** over-build 成甲方合同级:

| 项 | Phase 13 交付(reference-grade) | **不**做(留甲方/incident-driven) |
|---|---|---|
| authz | Dex 参考 IdP + JWKS/JWT + TokenReview + 最小 RBAC | full IAM(Keycloak federation / AD/LDAP / SSO) · 甲方 IdP swap |
| secret | external-secrets + 单 Vault backend | 多区 Vault 集群 / HSM / auto-unseal HA |
| quota | webhook B 读 status.usage 真强制 + PerCluster populate + fail-open | 配额预测 / 弹性配额 / billing 集成 |
| Karmada HA | 3-replica + 外置 etcd + dry-run 渲染 | full DR(跨机房 etcd 复制 / 真物理 multi-region LB · lab-gated) |
| P99 SLA | load-test harness + default SLO + scaler feedback | 甲方合同 SLO / 全链 APM / chaos 压测 |

**cert split 留 incident-driven**(§4(c)):现 cert-manager 单 issuer 满足;多 issuer/mTLS 分层留真实需求触发。

---

## §4 Open questions

### (a) 甲方 IdP / Vault 选型(default 兜底 · 不 block 收口)

- IdP:Dex 参考 land(T202)· 甲方选定后 swap `issuerURL`/`audience`(`deploy/idp/README.md` swap point)· Vault:external-secrets land(T203)· 甲方 secret backend 选定后 swap SecretStore
- 不 block 收口:甲方 input 未到位时 default 兜底即可交付(ADR-0023 §4(a))

### (b) P99 SLO 阈值(default SLO 兜底)

- default SLO 文档化 + 真硬件 P99 测量(T206)· 甲方 SLO input 后改阈值(`tests/sla/README.md`)· 达标判据用 default SLO(ADR-0023 §4(b))

### (c) cert 分层 / mTLS(incident-driven · 不预建)

- 现 cert-manager 单 issuer(inference-operator webhook TLS)满足;组件间 mTLS / 多 issuer 分层留真实安全需求触发 · 不 Phase 13 预建(P3 + M4 不为完整性堆 padding)

---

## §5 引用

### 上游(本 ADR 决策依据)

- ADR-0023 §2 Decision D(Bucket A scope = build-doc §5 · 本 ADR 架构展开)+ §2 Decision B(M7 不 over-claim · §3 right-sizing enforcement)
- ADR-0013 §6 forward note(authn full · §2 Decision A 上游)· ADR-0014 §6 Open question (b)+(c)(cluster-scope ClusterQuota + cross-cluster propagation · §2 Decision C 上游)
- ADR-0018 §2(Karmada deployment topology + 真物理 multi-region LB lab-gated · §2 Decision D 上游)· ADR-0008(PD Router p99 隐患 · §2 Decision E 上游)
- `docs/build-and-production-validation.md` §5(生产硬化验收清单 6 项 · 5 项进 Bucket A · 第 6 项 carry)
- 代码 grep-verified:authn.go(Validator + 3 impl)+ clusterquota_types.go(Usage.Total/PerCluster + RecomputeTotal)+ deploy/karmada/(bootstrap)+ backend prometheus(静态 Bearer)

### 下游(本 ADR 触发 Bucket A execution)

- P13-T-201 OIDC/TokenReview validator 真体 + middleware wiring(§2 Decision A)· P13-T-202 Dex IdP + RBAC(§2 Decision A)
- P13-T-203 Vault/external-secrets(§2 Decision B)· P13-T-204 ClusterQuota webhook B 真强制(§2 Decision C)
- P13-T-205 Karmada control-plane HA(§2 Decision D)· P13-T-206 vLLM PD P99 SLA harness(§2 Decision E)
- 全 Bucket A task 守 §3 right-sizing 边界(防 gold-plating)· §4 甲方依赖 default 兜底

### 上游 commit chain(决策时 grep-verified)

- Phase 12 tag `phase-12-complete`(`f0ce285`)· authn(P10-T-103)· ClusterQuota(P11-T-104)· Karmada bootstrap(P11-T-102/T103)
- 本 ADR 是 Phase 13 plan §3 P13-T-003 deliverable · 承 ADR-0023 §2 Decision D

---

**END of ADR-0025**
