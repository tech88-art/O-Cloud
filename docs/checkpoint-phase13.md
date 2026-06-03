# Checkpoint — Phase 13 complete (M7 真生产化收口 · 项目收官)

> **Status**: Phase 13 = **M7 真生产化收口** · **final milestone** · 项目 deliverable 完整交付
> (软件层)。`phase-13-complete` tag · 2026-06-03 · 16/16 task。承 Phase 12 (M6 平台真实化)。
>
> **一句话**:Phase 12 交付了 demo / real 两版架构分离(config + deploy profile · demo 版完整)。
> Phase 13 把 **real(生产)版剩余的所有 stub / mock / ErrNotImplemented 缺口全部填成真体** ——
> Bucket B 真硬件 body(5)+ Bucket A 生产硬化(6)+ 3 entry ADR + closer(2)。demo / real 隔离闭合,
> 项目核心交付物完整。**不规划 Phase 14**(per ADR-0023 §2 Decision B)。
>
> **诚实边界(务必读)**:本收口 = **软件层完整 + 真机验证 harness 就位 · 真机实测 lab-gated**。
> 执行环境无真 910B 硅片,故 build-doc §4.4 真硬件 5 项**对接 stamp 未实测**(harness `tests/e2e/
> real/` 已 land · 随 lab 接入跑出 🟢)。这正是 plan §8 + ADR-0024 §3 预设的 "软件完整 + best-effort
> 真机验证" 收口 posture —— 真机单点不 gating 收口。详 §4 真机 verify posture。

---

## 1. 16-task deliverable table

| Task | 桶 | 交付 | 状态 | commit |
|---|---|---|---|---|
| P13-T-001 | W1 | ADR-0023 Phase 13 entry decisions(项目收官) | ✅ | 17db60a |
| P13-T-002 | W1 | ADR-0024 真硬件 lab 激活 + Bucket B 架构(supersede ADR-0011 §3) | ✅ | 79a972c |
| P13-T-003 | W1 | ADR-0025 生产硬化架构(Bucket A) | ✅ | 0bbf336 |
| P13-T-101 | B1 | real-Ascend Source 真体(npu-smi/DCMI · 复用 parse.go) | ✅ | e461c0d |
| P13-T-102 | B2 | 真 NPU telemetry(exporter DCMI/npu-smi · simulator off) | ✅ | 098c467 |
| P13-T-103 | B3 | backend 真拓扑聚合(k8s/crd GetTopology) | ✅ | 9e834b2 |
| P13-T-104 | B4 | backend 真 Deploy()/DeleteDeploy()(k8s apply) | ✅ | 4283aca |
| P13-T-105 | B5 | 真 CANN + vllm-ascend PD 分离推理 | ✅ | 5d30865 |
| P13-T-201 | A1 | OIDC/TokenReview validator 真体 + middleware wiring | ✅ | 491d899 |
| P13-T-202 | A1 | Dex 参考 IdP + 最小权限 RBAC + authz wiring | ✅ | 37eaa28 |
| P13-T-203 | A2 | Vault/external-secrets — 替换静态 secret | ✅ | 97c8b1f |
| P13-T-204 | A3 | ClusterQuota webhook B 真强制 + 跨集群 usage | ✅ | 09096a6 |
| P13-T-205 | A4 | Karmada control-plane HA + propagation 加固 | ✅ | 104043c |
| P13-T-206 | A5 | vLLM PD P99 SLA harness + latency-aware scaling | ✅ | a06a0bb |
| P13-T-301 | Closer | 真机 E2E 连接 stamp harness + buildx multi-arch push | ✅ | 3176f7f |
| P13-T-302 | Closer | docs 收官 + checkpoint + tag(本提交) | ✅ | (this) |

附 1 found-fix:`f90c4bb` demo-backend kind smoke RBAC resourceslices read parity(T103 carry · 上一 session 遗留 uncommitted · main agent 验证+committed)。

---

## 2. 两桶摘要

### Bucket B · 真硬件 body(real 版运行时缺口 → 真体 · lab-gating 翻转 ADR-0024)

- **T101 real-Ascend Source**:`realascend.go` List/QueryTopology/Watch 不再 `ErrNotImplemented` ——
  `npu-smi info`(inventory)+ `npu-smi info -t topo`(HCCS ring · 复用 `npusmi/parse.go`)+ DCMI health。
  captured-fixture table-test PASS · **CGO/arm64 build-tag 隔离**(真 DCMI cgo 走 build-tag · 默认 stub-tag
  交叉编译 · 防 `cross-compile-arm64` CI 断 · plan §9 红线)。
- **T102 真 telemetry**:exporter DCMI/npu-smi source 真读 · `simulator.enabled=false` path · PCIE/HCCS/
  network/utilization 不再来自 simulator 正弦。
- **T103 真拓扑聚合**:k8s/crd `GetTopology` 真体(ResourceSlice `hccs_ring`/`numa_node` attr → ring map)·
  替换 `ErrCapabilityUnavailable` · **缝上共享层 profile 无关守恒**(ADR-0024 §2 Decision G · aggregator 无 `if real {}`)。
- **T104 真 Deploy()**:k8s source `Deploy()/DeleteDeploy()` client-go apply Deployment+Service(label
  `managed-by=demo-backend`+`deploy.ocloud.io/id`)· config.real.yaml `deploy: k8s`。
- **T105 真推理**:deployment_builder 注真 vllm-ascend/MindIE image + model mount + CANN env · PD Router 真 endpoint。

### Bucket A · 生产硬化(build-doc §5 验收清单逐项 close · ADR-0025)

- **T201 authz 真体**:`OIDCValidator`(JWKS+JWT)+`K8sTokenReviewValidator`(in-cluster 主路径)· o2-dms
  main.go wire middleware(替 PlaceholderBearer)· backend prometheus 静态 Bearer → SA 投影 token。
- **T202 Dex 参考 IdP + RBAC**:`deploy/idp/` Dex(in-memory · 零 apiserver RBAC · 甲方 swap)· 最小权限 RBAC
  跨 3 chart(gated · demo OFF / real ON · 1:1 map 真源 body)· 零 wildcard。
- **T203 Secret 管理**:`deploy/secrets/` ESO + Vault ClusterSecretStore + 4 ExternalSecret(dex/grafana/
  bmc)· install.sh `--with-secrets` · config.real.yaml secret-free(SA token)。
- **T204 ClusterQuota 真强制**:两 webhook 读 `ClusterQuota.status.usage.Total` 强制(cluster cap)· 新
  clusterquota_controller PerCluster 分桶 + RecomputeTotal · scaler cluster-rate gate · fail-open。
- **T205 Karmada HA**:control-plane 5 组件 3-replica + 3-node internal etcd quorum · propagation 加固
  (failover + propagateDeps + cross-cluster RBAC ClusterPropagationPolicy)· external etcd swap 文档化。
- **T206 P99 SLA**:server-side P99 collector(histogram_quantile via Ingestor)+ scaler latency-aware
  scale-up(opt-in · fail-open)+ `tests/sla/` 独立 load-test harness + default SLO 文档化(甲方 swap)。

---

## 3. ADR forward notes

- **ADR-0023**(entry):spine = real 版完整化 + 真硬件点亮 → 项目收口 · M7 final · 不规划 Phase 14 ·
  Bucket A/B scope · 4 carry 移出核心交付 · demo = 长期功能验证台。
- **ADR-0024**(真硬件激活):**supersede ADR-0011 §3 lab-gating 连续 6 phase default-defer** → light-up ·
  5 真体架构 + decoupling-seam invariant(§2 Decision G)。**ADR-0011 §3 status = lab activated by ADR-0024**。
- **ADR-0025**(生产硬化架构):authz / secret / quota / HA / SLA 5 Decision · right-sizing 声明(reference-
  grade · 防 gold-plating)· 每 Decision 1:1 map build-doc §5。

---

## 4. 真机 verify posture(诚实 · lab-gated · plan §8 + ADR-0024 §3)

**验证分工**:**功能正确性由 demo + CI fixtures 持续验**(快 · CPU-only · 常驻 · 长期一等功能验证台)·
**真机只验「对接」**(real Source 读真 npu-smi/DCMI/CANN + 输出与 Source 接口同形)。

**双层 verify 落地**:
- **离线层(已全做 · 本 session)**:每 task `go build`/`go vet`/`go test`(captured-fixture / fake-client
  table-test · 功能正确性在此层定)+ `CGO_ENABLED=0 GOARCH=arm64 go build`(ADR-0020)+ YAML/helm 结构校验
  + demo profile 回归。全 ✅。
- **真机层(harness 就位 · 实测 lab-gated)**:`tests/e2e/real/connection-stamps.sh`(build-doc §4.4 五项
  对接 stamp)+ `tests/sla` P99 压测 + Release Images workflow(arm64 镜像 push)**已 land**;**真机实测随
  lab 接入跑出**(本执行环境无 910B · 未实测)。

**单点 fallback**(P3 · P5 · 不重开 phase):若某真硬件点在 lab 驱动/CANN 版本下 block → 交付该 body +
captured-fixture test 通 + 标该点 lab-driver-gated(记本 §6 残留)· 项目仍 "软件完整 + best-effort 真机" 收口。

---

## 5. Scope adaptations(透明 · P3 · 偏离 plan literal 处)

- **cmd/main.go 注册 wiring**(T204/T206):main.go 非 T204/T206 literal Allowed Paths · 但未注册的
  controller/cache/SLO env = dead code → 加必要 same-module 注册(ClusterQuota controller+cache · scaler
  ClusterScaleCap + LatencySLOMillis env)· 文档化不静默。
- **config/rbac/ 不存在**(T204):inference-operator 无 kubebuilder config/rbac/ · RBAC 在 helm chart ·
  clusterquotas 授权 T202 已 forward-provision · 无新文件(grep 验证非假设)。
- **无 Grafana JSON**(T206):`deploy/grafana-dashboards/` 无 inference dashboard(kube-prometheus-stack
  部署时 provision)· P99 PromQL 文档入 `tests/sla/README` · plan "if needed" → not needed(M4 不 pad)。
- **无 arm64 kind smoke**(T301):免费 GitHub runner 仅 amd64 · 真 arm64 = lab 路径 · 不造无法跑的 job。
- **release-images.yml 独立 workflow**(T301):trigger workflow_dispatch + `v*` tag(非 dev push / 非
  phase-* tag)→ 与 phase CI gate(ci.yml on dev push)解耦 · 重量级 image push 不 redden gate。

---

## 6. 残留(swap point · 非缺口 · 文档化兜底)+ 真机 pending

**Swap point(甲方接入即用)**:
- **OIDC IdP**:Dex 参考 IdP land(T202)· 甲方 swap issuerURL/audience(ADR-0025 §4(a))。
- **P99 SLO 阈值**:default SLO 2000ms land(T206)· 甲方 swap `NPUVERTICAL_SCALER_P99_SLO_MS` + `tests/sla -slo-ms`(ADR-0025 §4(b))。
- **Vault backend**:ESO ClusterSecretStore 指 reference Vault(T203)· 甲方 swap server/auth 或换 AWS/GCP/Azure provider。
- **真物理 multi-region LB + external etcd DR**:Karmada HA 主体 land(T205)· 真机房 L4 LB + external etcd 真网络 lab-gated(ADR-0018 §2)。

**真机 pending(lab 接入后跑 + 记录)**:
- build-doc §4.4 五项对接 stamp 实测(`tests/e2e/real/connection-stamps.sh` · 跑出 🟢 + 数据回填 build-doc §4.4)。
- `tests/sla` 真 PD P99 实测 vs default SLO。
- 真集群 Karmada HA 3-replica ready + failover drill。

**已知 carry(T201 follow-up · 非收口 gating)**:o2-dms chart env key `O2DMS_OIDC_ISSUER_URL → O2DMS_OIDC_ISSUER`
重命名(deployment.yaml ∉ T202 paths · README+profile 文档化 · 修前 active validator = in-cluster TokenReview · auth 仍 work)。

---

## 7. 项目收官声明(per ADR-0023 §2 · 用户 2026-06-02 收官决策)

**M7 真生产化收口 = final milestone**。`phase-13-complete` = 项目 deliverable 完整交付:demo 版(Phase 12)
+ real 版(Phase 13)两版软件层完整 · demo/real 隔离闭合(隔离活在 `Source` 接口缝 · ADR-0024 §2 Decision G)·
build-doc §5 生产硬化清单 land · §4.4 真机 harness 就位(实测 lab-gated)。**不规划 Phase 14**。

**4 active carry 显式移出项目核心交付物**(信号触发可选扩展 · 不 gating 收口 · **不构成后续 phase**):

| Carry | 触发条件(若未来做) |
|---|---|
| Volcano gang-scheduling(ADR-0010) | 训练 demo signal materialise |
| Partitionable Devices KEP-4815(ADR-0009) | KEP GA + K8s 1.36+ baseline |
| Go v1 ResourceSlice migration(P10-fix-001 · 5 module) | lab 集群 K8s ≥1.36(v1beta1 shim 仍 work · 兼容维护 ≠ 功能缺口) |
| Frontend G6 5.x 重写(F01) | ReactFlow set-c-stress 大基数渲染瓶颈 |

**demo 版 = 长期一等功能验证台**(Phase 12 deliverable · 本 phase 不动 · 持续承担功能/CI/演示快速验证)。

详 `docs/phase13-plan.md` · `docs/build-and-production-validation.md` §4.4/§5 · ADR-0023/0024/0025 ·
各 `docs/devlog/phase-13-t*.md`。
