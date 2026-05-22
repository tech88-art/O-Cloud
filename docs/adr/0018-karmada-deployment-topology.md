# ADR-0018: Karmada control-plane deployment topology(host + 2 member kind cluster minimum · upstream chart + 自研 PropagationPolicy · lifted-informer cross-cluster aggregation)

- **状态**:Accepted(Phase 11 Karmada propagation 第一波 子契约 · 2026-05-22 · P11-T-002)
- **日期**:2026-05-22
- **决策者**:协调者(用户)
- **相关**:ADR-0013 §6 Forward notes(O2 DMS Karmada multi-cluster federation polish · 本 ADR §2 Decision D O2 DMS Adapter lifted-informer pattern 是其 implementation contract)/ ADR-0014 §7 Forward notes(Quota cluster-scope ClusterQuota + Karmada cross-cluster propagation + aggregator polish · 本 ADR §2 Decision D ClusterQuota informer 同期 contract)/ ADR-0016 §3 Stream 1 真实 multi-site / multi-机房 deployment(M5 真生产化 foundation 候选 · 本 ADR 是 Stream 1 foundation 子契约)/ ADR-0017 §2 Decision A 主线 2 Karmada propagation 第一波 production-grade(本 ADR 起手 子契约 · §2 Decision D 7th-9th 优先级 T102/T103/T104)/ ADR-0017 §4 Open question (a) Karmada control-plane HA posture(本 ADR §4 (a) 同 carry)/ `docs/checkpoint-phase10.md` §6 Phase 11+ handoff brief #7 Karmada propagation 第一波(本 ADR 是其入口契约)/ `docs/architecture.md` §9.3 多站点(Karmada · 本 ADR 详化 topology)· §13 review-table Phase 11 row(per ADR-0017 已 promote "in flight via T001-T204")/ `docs/phase11-plan.md` §3 P11-T-002 + §4 P11-T-102/T103/T104(本 ADR 是 T002 deliverable + 后续 T102/T103/T104 spec 入口)

---

## §1 Context

### §1.1 Phase 10 ships Karmada propagation substrate · 控制 plane deploy + PropagationPolicy + cross-cluster informer aggregation deferred Phase 11+

Phase 10 P10-T-103(O2 DMS Phase 10 polish · authn substrate · `4268554`)+ P10-T-104(Quota Phase 10 polish · token-bucket rate algorithm substrate · `189fdb6`)落地 multi-tenant 与 northbound contracts substrate · 但 Karmada propagation control-plane *本身* 部署 + PropagationPolicy 第一波 + cross-cluster informer aggregation **全 defer Phase 11+**(per checkpoint-phase10.md §1 T103/T104 status "Karmada / subscription / alarmEvent deferred Phase 11+" + "ClusterQuota + Karmada deferred Phase 11+")。

`docs/adr/0013-o2-dms-adapter.md` §6 Forward notes 第二行明示 "Karmada multi-cluster federation — O2 DMS Adapter 部署在 Karmada control plane · deploymentManager 反映成员 cluster list · ModelService 跨 cluster propagation 通过 PropagationPolicy"。`docs/adr/0014-multi-tenant-quota.md` §7 Forward notes 第一行明示 "cluster-scope Quota(ClusterQuota)+ Karmada cross-cluster propagation + aggregator"。两 ADR forward notes 同期对应 ADR-0016 §3 Stream 1 multi-site 主线候选。

### §1.2 ADR-0016 §3 Stream 1 是 M5 真生产化 foundation 候选 · 本 ADR 是 Stream 1 foundation 子契约

ADR-0016 §3 Stream 1 "真实 multi-site / multi-机房 deployment" 标 "M4→M5 主线候选":Phase 10 ships single Karmada control + 1 member cluster(kind cluster 模拟 multi-site)· Karmada propagation 第一波 ships via T103/T104 *substrate only* · Phase 11+ = 真 2+ member cluster · 跨机房 / 跨 region · K8s federation 真物理硬件。

ADR-0017 §2 Decision A 主线 2 = **Karmada propagation 第一波 production-grade** · 本 ADR 是 主线 2 *入口子契约*(topology 锁 + chart 选 + bootstrap 路径 + informer 模式)。后续 T102 chart deploy + T103 PropagationPolicy + T104 ClusterQuota 依据本 ADR §2 决策 4 项落地。

### §1.3 checkpoint-phase10 §6 #7 列 "Karmada 第一波" 为 Phase 11 primary work

`docs/checkpoint-phase10.md` §6 Phase 11+ chart packaging stream primary work 第 7 项明示:**Karmada propagation 第一波** = Karmada control-plane 部署 + PropagationPolicy + cross-cluster informer aggregation(per ADR-0013 §6 + ADR-0014 §7 + ADR-0016 §3 Stream 1)。本 ADR 在 ADR-0017 §2 Decision A 主线 2 框架下进一步 lock 4 项 detail decisions。

### §1.4 本 ADR 不涉及

- Karmada control-plane HA posture(>1 replica + etcd HA + leader-elect)— 留 Phase 12+ production hardening cohort · 本 ADR §4 (a) Open question 记 ·Phase 11 ship single control plane(与 demo-backend ADR-0015 §3.3 active-standby + Lease 选主同 posture)
- 真 多机房 / 跨 region / 真物理 switch multi-site — Stream 1 part 2 · 留 Phase 12+ on real hardware · 本 ADR ship 单机模拟 multi-site(kind cluster × 3 in same Docker daemon)
- 完整 OIDC IdP 部署(Keycloak / Dex)cross-cluster federation provider — Stream 5 part 2 · 留 Phase 12+ · 本 ADR ship Karmada cross-cluster RBAC 仅 static(本 ADR §4 (c) Open question 记 cross-cluster RBAC + Secret 传递 Phase 12+ Vault/external secret 路径)
- T102/T103/T104 具体 Allowed Paths / Acceptance — 由 `docs/phase11-plan.md` §4 task package overview 承载 · 本 ADR 不复述
- 跨 Karmada 真 spike(独立 spike document `docs/research/karmada-multi-site-spike.md`)— 本 ADR ship 时未起 spike 文档 · 本 ADR §1.2 + §2 决策依据 ADR-0013/0014 forward notes + Karmada upstream chart docs + 本 ADR §5 引用 grep-verified · spike 文档 Phase 12+ 起若需更深入 chunk-level 评估时另立

---

## §2 Decision

### §2.1 Decision A:Phase 11 topology = 1 host cluster(Karmada control-plane)+ 2 member kind cluster minimum · 单机模拟 multi-site

**3 cluster minimum 拓扑**:
```
┌─────────────────────────────────────────────────────────────┐
│  Host machine (Windows / Linux dev box · or Phase 12+ 真硬件) │
│                                                              │
│  ┌──────────────────┐  ┌─────────────┐  ┌─────────────┐    │
│  │  host cluster    │  │  member1    │  │  member2    │    │
│  │  (Karmada CP)    │  │  (kind ns)  │  │  (kind ns)  │    │
│  │  ocloud-system   │  │             │  │             │    │
│  │  karmada-system  │  │  pool-A     │  │  pool-B     │    │
│  │                  │  │             │  │             │    │
│  └─────────┬────────┘  └──────┬──────┘  └──────┬──────┘    │
│            │                  │                │            │
│            └─karmadactl join──┴────────────────┘            │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

- **host cluster**:`kind create cluster --name=host`(default kindest/node v1.34.3 per Phase 10 T003 baseline lockstep)· hosts Karmada control plane(`karmada-system` namespace)+ demo-backend / inference-operator binary aggregator path · 不 hosts member-cluster workloads
- **member1 / member2**:`kind create cluster --name=member1` + `--name=member2`(同 1.34.3 baseline)· hosts NPUSlicePool + ModelService + workload Pod · Karmada propagation 接收端
- **Cluster registration**:`karmadactl join member1 --cluster-kubeconfig=...` × 2 · push mode default(per Karmada doc · push mode = Karmada CP 主动 sync to member;pull mode = member 主动 pull · push mode 更适合 demo · 出错可看 CP 日志)
- **3 cluster 在同一 Docker daemon**:单机模拟 multi-site · 跨 region / 跨真物理机房留 Phase 12+ · per ADR-0016 §3 Stream 1 part 2

**Allow per-task scale up**:某 task(如 T201 master-demo-multi-site.sh)可临时加 member3 to demo 3-member propagation · 但 Phase 11 baseline = 2 member minimum · CI smoke gated on `KARMADA_ENABLED=1` env flag(per phase11-plan §3 P11-T-102)default off keep CI fast。

### §2.2 Decision B:Karmada chart = upstream Karmada chart + 自研 PropagationPolicy selector templates · namespace `karmada-system`

**Chart 选择**:
- **upstream Karmada chart**:`github.com/karmada-io/karmada/charts/karmada` · charts/karmada-operator 是 newer pattern(Karmada Operator-managed control plane)· 选 charts/karmada(direct chart · Phase 11 不引入 Operator-managed 模式 · simpler ops surface)
- **chart version pin**:`karmada-1.13.x`(or whatever is latest stable at T102 W2 entry · T102 起手 `helm repo update` + 锁定具体 version · 入 ADR `docs/devlog/phase-11-t102.md` verification stamp)
- **upstream Karmada chart 自管 control-plane components**:karmada-apiserver + karmada-controller-manager + karmada-scheduler + karmada-webhook + karmada-aggregated-apiserver + etcd(in-cluster mode for Phase 11 · external etcd Phase 12+ HA)

**自研 PropagationPolicy selector templates**(本 repo `deploy/karmada/policies/` · T103 落地):
- `propagation-modelservice.yaml`:select ModelService(inference.ocloud.edge.example.com)→ propagate to placement.clusterAffinity.clusterNames = [member1, member2]
- `propagation-npuslicepool.yaml`:select NPUSlicePool(npu.ocloud.edge.example.com)→ same placement
- `propagation-quota.yaml`:select Quota namespace-scope(inference.ocloud.edge.example.com/v1alpha1)→ same placement
- `cluster-propagation-clusterquota.yaml`:select ClusterQuota cluster-scope(per T104 · `kind: ClusterPropagationPolicy` · cluster-scope CRD 必走 ClusterPropagationPolicy 而非 namespaced PropagationPolicy)

**namespace 部署位置**:
- **`karmada-system`**:Karmada control-plane components(upstream chart default · 不动)
- **`ocloud-system`**:demo-backend / inference-operator binary on host cluster · 跨 cluster 调用 Karmada-aggregated API path
- **应用 namespace 默认**:`default` per upstream Karmada · 各 chart 通过 `--namespace` override(per phase11-plan §3-§4 P11-T-003-T008 chart values)

### §2.3 Decision C:Member cluster bootstrap = `kind create cluster` + `karmadactl join` · push mode default

**Bootstrap script**(`deploy/karmada/install.sh` · T102 落地):
```bash
#!/usr/bin/env bash
set -euo pipefail

# 1. Create host + 2 member kind cluster
kind create cluster --name=host --image=kindest/node:v1.34.3
kind create cluster --name=member1 --image=kindest/node:v1.34.3
kind create cluster --name=member2 --image=kindest/node:v1.34.3

# 2. Install Karmada control plane in host cluster
kubectl --context=kind-host create namespace karmada-system
helm repo add karmada-charts https://raw.githubusercontent.com/karmada-io/karmada/master/charts
helm repo update
helm install karmada karmada-charts/karmada \
  --kube-context=kind-host \
  --namespace=karmada-system \
  --version=<PINNED at T102>

# 3. Wait for Karmada CP Ready
kubectl --context=kind-host -n karmada-system wait \
  --for=condition=Ready pod -l app.kubernetes.io/component=karmada-apiserver \
  --timeout=300s

# 4. Extract karmada kubeconfig for karmadactl
kubectl --context=kind-host -n karmada-system get secret karmada-kubeconfig \
  -o jsonpath='{.data.kubeconfig}' | base64 -d > /tmp/karmada.conf

# 5. Join member clusters (push mode default)
karmadactl --kubeconfig=/tmp/karmada.conf join member1 \
  --cluster-kubeconfig=<kind-member1-kubeconfig> --cluster-context=kind-member1
karmadactl --kubeconfig=/tmp/karmada.conf join member2 \
  --cluster-kubeconfig=<kind-member2-kubeconfig> --cluster-context=kind-member2

# 6. Verify
karmadactl --kubeconfig=/tmp/karmada.conf get clusters
# Expected output: member1 Ready · member2 Ready
```

**Push mode 选择 rationale**(per Karmada doc · push vs pull mode):
- **Push mode**(default)· Karmada CP 主动 sync resource to member · adopt member cluster from CP control plane · member cluster 不需要安装 karmada-agent · 适合 host + member 都在同一控 control 域(same Docker daemon · same dev box · same admin)
- **Pull mode** · Karmada CP 部 ResourceTemplate · member cluster 跑 karmada-agent pull · 适合 member cluster 跨 admin 域(真 multi-org · 真 multi-tenant cross-region)· 留 Phase 12+ if signal

**Uninstall script**(`deploy/karmada/uninstall.sh` · T102 落地):
```bash
karmadactl unjoin member1 ...
karmadactl unjoin member2 ...
helm uninstall karmada --kube-context=kind-host -n karmada-system
kind delete cluster --name=member1
kind delete cluster --name=member2
kind delete cluster --name=host
```

### §2.4 Decision D:Cross-cluster informer aggregation = lifted-informer pattern · O2 DMS Adapter / Quota ClusterQuota informer 各自跨 cluster aggregation

**Lifted-informer pattern**(per Karmada doc · controller-runtime + Karmada karmada-aggregated-apiserver 路径):
- Karmada 提供 `karmada-aggregated-apiserver`(aggregated API · lifted view across member clusters)· 通过 host kubeconfig + `--cluster-context=karmada-apiserver` 路径访问
- controller-runtime / client-go 调 aggregated apiserver · 自动 fan-out 到 member cluster · return aggregated view(by default 跨所有 joined cluster · 可 filter by clusterAffinity)
- Informer 模式 `informer.Run()` 自动 watch aggregated stream · controller 不感知 cross-cluster

**O2 DMS Adapter informer 实现路径**(T103 落地 · per ADR-0013 §6 Forward notes line 1):
- O2 DMS Adapter binary 部署在 host cluster(`ocloud-system` namespace)· 调 karmada-aggregated-apiserver path · informer ModelService(inference.ocloud.edge.example.com) + NPUSlicePool(npu.ocloud.edge.example.com)
- NB endpoint `GET /o2dms/v1/deploymentManagers` → return Karmada CP 自身 + member cluster list(per `karmadactl get clusters` 同源 · 通过 karmada-aggregated-apiserver 的 ClusterResource)· 反映 ADR-0013 §2 Decision C resource reflection 映射表 row "deploymentManagers ↔ Karmada cluster"
- NB endpoint `GET /o2dms/v1/deploymentItems` → return aggregated ModelService list 跨 member · 加 `cluster` 字段 per item · 反映各 member cluster 中的 ModelService 真实部署位置

**Quota ClusterQuota cross-cluster aggregation**(T104 落地 · per ADR-0014 §7 Forward notes line 1):
- ClusterQuota CRD cluster-scope · ClusterPropagationPolicy propagate to member cluster
- Quota controller(colocated with inference-operator binary on host cluster)调 karmada-aggregated-apiserver · informer ClusterQuota status.usage 跨 cluster aggregation
- 累计 usage view(sum across member cluster)· 与 ClusterQuota.spec.hard.npuSlices 比对 · admission webhook decision 依据 aggregated usage
- 实现细节:ClusterQuota controller 在 status.usage 加 `perCluster` map(member1 / member2 子 usage · 总和到 status.usage.npuSlices)· cross-cluster sync 走 60s tick + 5s cache TTL fallback per ADR-0014 §2 Decision D · Phase 11 不 ship event-driven sync(ADR-0014 §7 Forward notes line 5 Phase 12+)

**为什么不选 KubeFed v2 lifted informer 模式**:
- KubeFed v2 已 archived(2023-02 deprecation announce · 2024 EOL)· 不能用
- Karmada 是 KubeFed v2 实质继承者 · API 兼容性最好(per arch §3.3)· 本 ADR rationale 与 arch §3.3 align

**为什么不选 Open Cluster Management (OCM)**:
- OCM 是 hub + spoke 模式 · 部署 footprint 更重(ManifestWork + Placement + AddOnDeploymentConfig 多 CRD layer)· 适合 真 enterprise multi-cluster · 不适合 demo
- Karmada 单 control plane + PropagationPolicy 一对一映射 · ops 更轻 · 与 arch §3.3 选定 align

---

## §3 Phase 11 delivery scope

本 ADR 覆盖 Phase 11 plan §4 W2 task chain 中 Karmada propagation 第一波 3 task:

| Task | Delivery | 依据本 ADR | Estimated |
|---|---|---|---|
| **P11-T-102** Karmada control-plane chart deploy + 2 member kind bootstrap | `deploy/karmada/install.sh` + `values.yaml` + `uninstall.sh` + `README.md` + kind smoke gated on `KARMADA_ENABLED=1` | §2 Decision A topology + §2 Decision B chart + §2 Decision C bootstrap script | 1.5-2d |
| **P11-T-103** Karmada PropagationPolicy 第一波 + cross-cluster informer aggregation | `deploy/karmada/policies/*.yaml`(4 PropagationPolicy / ClusterPropagationPolicy YAML)+ O2 DMS Adapter karmada-aggregated-apiserver path | §2 Decision B PropagationPolicy templates + §2 Decision D lifted-informer pattern | 1-1.5d |
| **P11-T-104** Quota ClusterQuota CRD + Karmada cross-cluster usage 累计 | `operators/inference-operator/api/v1alpha1/clusterquota_types.go` + admission webhook ext + ClusterPropagationPolicy + status.usage.perCluster map | §2 Decision B cluster-propagation-clusterquota.yaml + §2 Decision D ClusterQuota aggregation | 1-1.5d |

**Karmada 路径 W2 末尾 land**(T102 → T103 → T104 serial · 不 parallel · per phase11-plan §2 parallelisation candidates "T102 MUST serial after T002 ADR-0018 + T007 · T103 MUST serial after T102 · T104 MUST serial after T103")。

---

## §4 Open questions

### (a) Karmada control-plane HA(Phase 11 single replica · Phase 12+ HA?)

ADR-0017 §4 (a) 已 carry:Phase 11 ships single Karmada control(1 replica · 不 HA · 与 demo-backend ADR-0015 §3.3 active-standby + Lease 选主同 posture)· Phase 12+ Karmada control HA(3 replica + 外部 etcd HA + leader-elect)留 production hardening cohort。

**当前倾向**:Phase 11 single · Phase 12+ 与完整 P99 SLA / 完整 OIDC IdP / 真多机房一起 land。

### (b) PropagationPolicy granularity(per-CRD vs per-resource selector)

每个 CRD 用一个 PropagationPolicy 还是 per-resource(每 individual ModelService / NPUSlicePool 单独一个 PropagationPolicy)?

- **选项 a:per-CRD PropagationPolicy**(default · Phase 11 ship):4 YAML 文件(ModelService / NPUSlicePool / Quota / ClusterQuota)· select labels.app.kubernetes.io/component = X · 1 PropagationPolicy match all instances of given CRD
- **选项 b:per-resource PropagationPolicy**:每 individual resource 一个 · 更细粒度 selector · 适合不同 instance 不同 placement(e.g. modelservice-A 仅 member1 · modelservice-B 仅 member2)· Phase 11 不需 · 留 Phase 12+ if signal

**当前倾向**:选项 a · 简单 + 演示需求够 · Phase 12+ 若 per-tenant placement 需求出现 → 引入 per-resource override(ClusterPropagationPolicy 加 priority + tag-based selector)。

### (c) Cross-cluster RBAC + Secret 跨 cluster 传递(Phase 11 static · Phase 12+ Vault/external secret)

Karmada 跨 cluster 部署 require:
- **RBAC**:member cluster 必须有相应 RBAC · push mode 下 Karmada CP 自动 propagate ServiceAccount + ClusterRole · 但 Secret 不自动 propagate(per Karmada doc · Secret 必须显式 PropagationPolicy 才同 sync)
- **Secret 跨 cluster**:cert-manager certificate · OIDC client secret · vendor API token 等 · Phase 11 ship static(用户手动同 deploy on each cluster 或 写 ClusterPropagationPolicy 显式 sync Secret · 与 PropagationPolicy 同位 不同 kind)

**Phase 11 ship**:static Secret 复制 · 每 cluster 手动 / 通过 install.sh 复制 OIDC client secret + cert-manager cert
**Phase 12+ 候选**:Vault / external-secrets operator · Vault Agent inject · 跨 cluster shared Secret backend · 与 Stream 5 part 2 完整 OIDC IdP 一起 land

**当前倾向**:Phase 11 static · 文档明示 install.sh 复制路径 · Phase 12+ Vault 路径与 IdP 一起。

---

## §5 引用

### 上游(本 ADR 决策依据)

- ADR-0013 §6 Forward notes(O2 DMS Karmada multi-cluster federation polish · 本 ADR §2 Decision D O2 DMS Adapter lifted-informer pattern 是其 implementation contract)
- ADR-0014 §7 Forward notes(Quota cluster-scope ClusterQuota + Karmada cross-cluster propagation + aggregator polish · 本 ADR §2 Decision D ClusterQuota informer 同期 contract)
- ADR-0016 §3 Stream 1 真实 multi-site / multi-机房 deployment(M5 真生产化 foundation 候选 · 本 ADR 是 Stream 1 foundation 子契约)
- ADR-0017 §2 Decision A 主线 2 Karmada propagation 第一波(本 ADR 是其 子契约 入口)
- ADR-0017 §4 Open question (a) Karmada HA(本 ADR §4 (a) 同 carry)
- `docs/checkpoint-phase10.md` §6 Phase 11+ handoff brief #7 Karmada propagation 第一波(本 ADR 是其入口契约)
- `docs/architecture.md` §3.3 K8s 生态 Karmada 选定 row(本 ADR §2 Decision D 不选 KubeFed/OCM rationale 与之 align)· §9.3 多站点(本 ADR §2 Decision A topology 详化)
- Karmada upstream docs · `https://karmada.io/docs/` · 本 ADR §2 Decision B push/pull mode + §2 Decision C bootstrap script + §2 Decision D lifted-informer pattern 来源

### 下游(本 ADR 后续工作 / 触发 Phase 11 execution)

- P11-T-102 Karmada control-plane chart deploy(本 ADR §2 Decision A topology + §2 Decision B chart + §2 Decision C bootstrap script policy lock)
- P11-T-103 Karmada PropagationPolicy 第一波(本 ADR §2 Decision B PropagationPolicy templates + §2 Decision D lifted-informer pattern policy lock)
- P11-T-104 Quota ClusterQuota CRD(本 ADR §2 Decision B ClusterPropagationPolicy + §2 Decision D ClusterQuota aggregation policy lock)
- P11-T-201 真 multi-cluster / multi-site demo 打磨(本 ADR §2 Decision A topology 是 demo 基础 · master-demo-multi-site.sh 跑 3 cluster 拓扑)
- P11-T-203 docs 大整理 + Phase 12+ 前瞻(本 ADR §4 3 open questions + Phase 12+ HA / Vault / 真 multi-region forward notes carry)

### 上游 commit chain(决策时 grep-verified)

- ADR-0013 @ Phase 9 P9-T-001(`docs/adr/0013-o2-dms-adapter.md` · §6 Forward notes line 1 Karmada multi-cluster polish)
- ADR-0014 @ Phase 9 P9-T-002(`docs/adr/0014-multi-tenant-quota.md` · §7 Forward notes line 1 ClusterQuota + Karmada)
- ADR-0016 @ Phase 10 P10-T-002(`docs/adr/0016-lab-onboarding-and-phase-11-outlook.md` · §3 Stream 1 multi-site)
- ADR-0017 @ Phase 11 P11-T-001 `548144d`(`docs/adr/0017-phase-11-entry-decisions.md` · §2 Decision A 主线 2 + §4 (a) Karmada HA)
- Phase 10 tag `phase-10-complete` @ `04cc009`(`docs/checkpoint-phase10.md` §6 #7 Karmada 第一波)
- Phase 11 plan @ `5a17fe3`(`docs/phase11-plan.md` · §3 P11-T-002 acceptance + §4 P11-T-102/T103/T104 spec)

---

**END of ADR-0018**
