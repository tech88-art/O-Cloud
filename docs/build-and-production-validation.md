# 构建指南 + 生产环境验证步骤

> O-Cloud 边缘云平台 —— 从源码构建多架构镜像 → 部署到 K8s → 生产环境逐层验证。
>
> **依据**(全部来自仓库真实配方,非杜撰):根 `Makefile` · `scripts/install.sh` · `.github/workflows/e2e-kind.yml` · `tests/e2e/kind/install.sh` · 各模块 `Makefile` · ADR-0008/0009/0013/0014/0020 · `docs/checkpoint-phase12.md` §6。
>
> **现状边界(务必先读 · 更新至 `phase-13-complete`)**:**real(生产)版软件层完整** —— Phase 13
> 把 real 版所有 stub/mock/ErrNotImplemented 填成真体(Bucket B 真硬件 body T101-105 + Bucket A
> 生产硬化 T201-206 · §5 清单全 land)。功能正确性由 **demo 版 + CI fixtures** 持续验证(Mock + 合成
> fixture + amd64 kind · CI 绿)。**真鲲鹏 920 + 昇腾 910B + openEuler 真机「对接」验证 harness 已 land**
> (`tests/e2e/real/connection-stamps.sh` + `tests/sla` + Release Images arm64 push)· **真机实测
> lab-gated**(随 lab 接入跑出 🟢 · 见 §4.4)。本文 **🔴 真机** = 软件体已 land · 真机实测 pending lab;
> **🟢 已验证** = 当前 CI/demo 已覆盖。

---

## 0. 三种部署形态

| 形态 | 用途 | 入口 | 架构 |
|---|---|---|---|
| **A. 单机 docker-compose** | 演示 / 快速起栈 | `scripts/install.sh` | amd64 dev / arm64 target |
| **B. K8s + Helm/Kustomize** | **生产形态**(本文重点) | 见 §3 | arm64(Kunpeng)+ amd64(dev/CI) |
| **C. kind smoke** | 开发 / CI 验证 | `tests/e2e/kind/install.sh` | amd64 |

**目标平台**(ADR-0020 · 翻转 ADR-0001 §13 amd64-only):华为 **Atlas 800 原生配置** = 鲲鹏 920(aarch64)host + 昇腾 910B + **openEuler 22.03 LTS**。镜像 multi-arch buildx(`linux/amd64,linux/arm64`)· arm64=部署 target · amd64 保留本机 dev/CI/render-verify。

### 0.1 两个交付版本(demo / real · 同一代码库,由架构分离)

由运行时数据源抽象(`mapping.<resource>` + chart values overlay)分离,**不 fork 代码**;用户按需选哪个验证:

| | **Demo 版(验证数据)** | **Real 版(物理数据)** |
|---|---|---|
| 数据 | 全 mock / simulator | in-cluster K8s / Pool CRD / Prometheus / ConfigMap |
| 后端 config | `backend/configs/config.dev.yaml` | `backend/configs/config.real.yaml` |
| 部署 overlay | `deploy/profiles/demo/*.values.yaml` | `deploy/profiles/real/*.values.yaml` |
| 主要形态 | docker-compose / `make run`(localhost:3000) | K8s + helm(真集群) |
| NPU 源 | `mock-json` | `real-ascend`(npu-smi/DCMI 真体 · **P13-T-101** land · 见下) |
| **选择** | `scripts/install.sh`(默认)或 `--profile demo` | `scripts/install.sh --profile real` 或 `helm -f deploy/profiles/real/<chart>.values.yaml` |

**Real 版当前边界(如实 · `phase-13-complete`)**:Phase-13 已把 real 版**全部 stub/mock/`ErrNotImplemented` 填成真体** —— `topology` 真聚合(`GetTopology` 读 ResourceSlice `hccs_ring`/`numa_node` · **P13-T-103**)+ 真 `Deploy()/DeleteDeploy()`(client-go apply Deployment+Service · **P13-T-104**)+ `real-ascend` NPU 源(npu-smi/DCMI 真体 · **P13-T-101**)+ 真 telemetry(simulator off · **P13-T-102**)+ 真推理(CANN/vllm-ascend PD · **P13-T-105**)+ OIDC/RBAC + Vault + 配额真强制 + Karmada HA + P99 SLA(**P13-T-201..206** · §5 全 `[x]`)。即 **Real 版软件层完整 · 可部署 · 全资源接真源**;剩余仅 **真机运行对接验证**(lab-gated · `tests/e2e/real/` · §4.4 五项 stamp 🔴 随 lab 实测回填)。详见 `deploy/profiles/real/README.md`。

---

## 1. 工具链前提

| 工具 | 版本 | 用途 |
|---|---|---|
| Go | ≥ 1.22(`backend/go.mod` 当前 pin ~1.26) | backend + operators 编译 |
| Node.js | ≥ 22(install.sh min 20) | frontend |
| pnpm | 9 | frontend 包管理 |
| Docker + **buildx** | BuildKit 启用 | 多架构镜像 |
| Helm | ≥ v3.16 | chart 部署 |
| kubectl | 匹配集群 | 部署 + 验证 |
| kustomize | v5.8+(operators 自动下载) | pool-operator 部署 |
| kind | v0.31+(仅形态 C) | 本地 K8s smoke |

**🔴 真机额外前提**:openEuler 22.03 LTS(node OS)· **CANN**(昇腾计算架构)· **Ascend Device Plugin + 驱动**(暴露 `huawei.com/Ascend910B`)· K8s **1.34+**(DRA `resource.k8s.io/v1` GA;1.31–1.33 用 `v1beta1`)。

---

## 2. 构建指南

### 2.1 一键(本仓库根)

```bash
make ci            # 全仓库 lint + test + build(等价 make lint && make test && make build)
make build         # backend/build + frontend/build + operators generate+manifests
make test          # backend + frontend + operators 单测
make lint          # backend + frontend lint/typecheck + configs 校验
```

### 2.2 后端(Go · 静态)

```bash
cd backend
make build         # CGO_ENABLED=0 go build → bin/demo-backend(注入 version/commit ldflags)
make test          # 单测 + 覆盖率
make run           # 用 configs/config.dev.yaml 本地起(:8080)
```

### 2.3 前端(React/Vite)

```bash
cd frontend
pnpm install
pnpm run gen:types # 由 docs/api-contract.yaml 生成 TS 类型(契约同步)
pnpm run build     # → frontend/dist/
```

### 2.4 Operators(CRD + 控制器)

```bash
# CRD 类型 + YAML(pool-operator 为 4 级池化 CRD 源)
make -C operators/pool-operator generate manifests
# 产物:operators/pool-operator/config/crd/bases/*.yaml

# 单组件编译
make -C operators/<op> build      # → bin/manager
```

### 2.5 多架构镜像(ADR-0020 · 生产关键)

共 **10 个镜像**:`demo-backend` · `ascend-npu-exporter-plus` · 8 个 operator(`pool-operator` `npu-dra-driver` `inference-operator` `scheduler-plugin` `o2-dms-adapter` `node-lifecycle-operator` `software-mgmt-operator` `bare-metal-provisioning-operator`)。

**Kubebuilder operator** —— 用自带 buildx 目标(自动 `linux/arm64,linux/amd64` + `--push`):

```bash
make -C operators/pool-operator docker-buildx \
  IMG=<registry>/ocloud/pool-operator:<tag>
# PLATFORMS 默认 linux/arm64,linux/amd64,...(见各 operator Makefile)
# 内部:GOARCH=${TARGETARCH} 静态跨编译 · CGO off · 无需换 base
```

**backend / exporter**(通用 buildx · Dockerfile 已支持 `TARGETARCH`):

```bash
cd backend   # 或 exporters/ascend-npu-exporter-plus
docker buildx build --platform linux/amd64,linux/arm64 \
  -t <registry>/ocloud/demo-backend:<tag> --push .
```

> **单架构本机** 可用 `make -C backend docker` / `make -C operators/<op> docker-build IMG=...`(形态 C kind 即用此 + `kind load`,不 push registry)。
> **生产**:必须 `--push` 到镜像仓库,集群侧 `image.pullPolicy=IfNotPresent` 或按 tag 拉取。

---

## 3. 部署(形态 B · 生产)

### 3.1 集群前提

- K8s **1.34+**,**arm64 节点**(Kunpeng 920),openEuler node OS。
- **🔴 真机**:Ascend Device Plugin 已装(`kubectl get nodes -o json | jq '.items[].status.allocatable["huawei.com/Ascend910B"]'` 非空)。
  *(形态 C kind 用 `kubectl patch node ... huawei.com/Ascend910B=8` 打 **fake** 容量替代 —— 仅 dev/CI。)*
- helm `nodeAffinity kubernetes.io/arch=arm64`(**soft** preferredDuringScheduling · ADR-0020):arm64 优先、不硬性排他。

### 3.2 部署顺序(依赖序 · 从 e2e-kind 配方提炼)

> 命名空间:`cert-manager` · `pool-operator-system` · `ocloud-system`(backend/dra/inference/o2)· `monitoring` · `kube-system`(scheduler)。

```bash
# 1) cert-manager —— inference-operator webhook TLS 硬前提
helm repo add jetstack https://charts.jetstack.io --force-update
helm upgrade --install cert-manager jetstack/cert-manager \
  --namespace cert-manager --create-namespace \
  --version v1.16.0 --set crds.enabled=true --wait --timeout 10m

# 2) pool-operator(4 级池化 CRD + 控制器)—— kustomize,非 helm
make -C operators/pool-operator deploy IMG=<registry>/ocloud/pool-operator:<tag>
#    (内部:kustomize edit set image + kustomize build config/default | kubectl apply)

# 3) npu-dra-driver(ResourceSlice 发布者)
helm upgrade --install npu-dra-driver deploy/helm-charts/npu-dra-driver/ \
  --namespace ocloud-system --create-namespace \
  --set image.repository=<registry>/ocloud/npu-dra-driver --set image.tag=<tag>
#    🔴 真机:开 real allocator(--enable-publisher / --enable-claim-controller),
#            非 simulator;simulator 模式读 mock npus.json(dev/CI)。

# 4) ascend-npu-exporter-plus(监控 exporter · DaemonSet)
helm upgrade --install ascend-npu-exporter-plus deploy/helm-charts/ascend-npu-exporter-plus/ \
  --namespace monitoring --create-namespace \
  --set image.repository=<registry>/ocloud/ascend-npu-exporter-plus --set image.tag=<tag>
#    🔴 真机:simulator.enabled=false(读真 NPU);dev/CI 用 simulator.enabled=true。

# 5) demo-backend(聚合后端 · 无状态)
helm upgrade --install demo-backend deploy/helm-charts/demo-backend/ \
  --namespace ocloud-system
#    🔴 真机:config mapping 由 mock → k8s/crd/prometheus(configs/config.yaml);
#            dev/CI 挂 mock-data ConfigMap。

# 6) inference-operator(ModelService CRD + PD Router webhook)
helm upgrade --install inference-operator deploy/helm-charts/inference-operator/ \
  --namespace ocloud-system

# 7) scheduler-plugin(自研 kube-scheduler · NUMA+HCCS 亲和)
helm upgrade --install scheduler-plugin deploy/helm-charts/scheduler-plugin/ \
  --namespace kube-system

# 8) o2-dms-adapter(O-RAN O2 DMS 北向)
helm upgrade --install o2-dms-adapter deploy/helm-charts/o2-dms-adapter/ \
  --namespace ocloud-system

# 9) 监控栈(可选 · Prometheus + Grafana)
helm upgrade --install kube-prometheus-stack \
  prometheus-community/kube-prometheus-stack \
  --namespace monitoring --values deploy/single-node/values-kps.yaml
```

> 镜像替换:每个 chart 用 `--set image.repository/tag` 指向 §2.5 推送的镜像。
> 所有 chart `helm upgrade --install` 幂等 · 生产建议加 `--wait --timeout`。
>
> **版本 overlay(§0.1)**:上面给的是基础命令;选 demo/real 版时给有差异的 chart
> 追加 `-f deploy/profiles/<demo|real>/<chart>.values.yaml`(demo-backend / exporter /
> npu-dra-driver / inference-operator 四个有 overlay,其余用 chart 默认)。或一条龙:
> `scripts/install.sh --profile real --all-phase-4`。Real 版前置见
> `deploy/profiles/real/README.md`(含 mock-data 缺口 + presets ConfigMap + KPS 等)。

### 3.3 单机快速形态(A · 非生产)

```bash
./scripts/install.sh                 # 源码构建 + docker-compose 起栈(:3000 前端 / :8080 后端)
./scripts/install.sh --all-phase-4   # 附加 kube-prometheus-stack + npu-dra-driver(当前 kubeconfig)
./scripts/install.sh --uninstall     # 拆栈
```
openEuler(dnf)/ Ubuntu(apt)自动识别 · arm64/amd64 自动检测(ADR-0020)。

---

## 4. 生产环境验证步骤(逐层)

> 每条给 **验证命令** + **通过判据**。🟢 = 当前 CI/demo 已实证;🔴 真机 = 需真硬件。

### 4.1 基础设施层(IMS · 算力发现/池化)

```bash
# (a) ResourceSlice 发布(npu-dra-driver)🟢(真机=真 NPU 数)
kubectl get resourceslices -o json \
  | jq '[.items[] | select(.spec.driver=="npu.ocloud.edge.example.com")] | length'
#   判据:≥1 slice · 每节点 ≥8 device · device 带 npu.huawei.com/index 属性
#   (参考 tests/e2e/kind/dra_publish_test.sh)

# (b) 池化 reconcile 上卷(pool-operator)🟢
kubectl get npuslicepool -n ocloud-system <pool> -o jsonpath='{.status.totalSlices}'
#   判据:totalSlices > 0(证明 Reconcile 读到 CR + 走 NPUPool ref + 物化 slice)
kubectl get clusterpool,nodepool,npupool,npuslicepool -A   # 4 级层级 status 上卷

# (c) 后端聚合 + 指标 🟢
curl -fsS http://<backend>/api/v1/healthz                  # 200
curl -fsS http://<backend>/metrics | grep ocloud_backend_  # ≥1 ocloud_backend_* 序列
```

### 4.2 服务编排层(推理部署 / 调度)

```bash
# (a) cert-manager + inference-operator 🟢
kubectl -n cert-manager get pods                            # Running
kubectl -n ocloud-system get deploy inference-operator      # Available
kubectl -n ocloud-system get certificate                    # Ready

# (b) ModelService + PD 分离 + webhook 注入 🟢
kubectl -n ocloud-system apply -f <modelservice.yaml>       # 如 Qwen 8B PD
kubectl -n ocloud-system get ms <name> -o jsonpath='{.status.phase}'   # 推进(非空/Ready)
kubectl -n ocloud-system get deploy -l inference.ocloud.edge.example.com/model-service=<...>
#   判据:Prefill + Decode 两 Deployment 物化 · Pod 带 npu.huawei.com/slice-bindings 注解(PD Router webhook 注入)

# (c) scheduler-plugin(NUMA+HCCS)🟢(放置 best-effort;🔴 真 HCCS 亲和需真硅片)
kubectl -n kube-system get cm -l app.kubernetes.io/name=scheduler-plugin -o yaml | grep npu-scheduler
kubectl -n kube-system get lease -l app.kubernetes.io/name=scheduler-plugin   # leader lease 存在
#   判据:KubeSchedulerConfiguration profile=npu-scheduler 存在 · leader lease 持有
#   另:inference-operator /metrics 暴露 3 个 Phase 6 collector(P6-T-104)

# (d) 动态切分模板 / 垂直伸缩 / 配额 🟢
kubectl get npuslicetemplate <t> -o jsonpath='{.status}'    # Validated / Allocatable
kubectl -n ocloud-system get npuvs <scaler> -o jsonpath='{.status.conditions}'  # ConditionActive
kubectl -n ocloud-system get quota,clusterquota             # admission 生效(over-cap 被拒)

# (e) O2 DMS 北向 🟢
curl -fsS http://<o2-dms>/o2dms/v1/deploymentManagers       # 返回 deploymentManager 列表
```

### 4.3 前端 / 端到端

```bash
# 操作台一体化页(ADR-0022 one-page workspace)🟢
#   浏览器开 http://<frontend> → 概览拓扑(节点→NPU→切片)/ 工作负载 / 部署 / 指标(Grafana iframe)/ 日志
# E2E 回归(Playwright)
cd tests/e2e && pnpm exec playwright test --config=playwright.config.kind.ts   # 🟢 kind smoke
#   🔴 真机:把 baseURL 指向真集群入口,重跑同一 spec
```

### 4.4 真硬件特有验证(Phase 13 软件体 land · 真机实测 lab-gated)

**软件体 Phase 13 全 land**(T101-105);**真机实测 harness 就位**(`tests/e2e/real/connection-stamps.sh`
+ `tests/sla`)· **真机跑出 🟢 + 数据回填本节随 lab 接入**(执行环境无 910B · 未实测)。验证分工:demo 验
功能 / 真机验「对接」(real Source 读真源 + 输出同形 · plan §8 · ADR-0024 §3)。

1. **真 NPU 发现** 🟡 body land(T101 real-Ascend `npu-smi info` · 复用 `npusmi/parse.go`)· 真机 stamp:
   ResourceSlice 数量/属性 = 真实 910B(`connection-stamps.sh` stamp 1 · 🔴 lab-gated 实测)。
2. **真切片** 🟡 body land(T101 claim-controller)· 真机 stamp:910B 动态切分真实下发(stamp 2 · 🔴 lab)。
3. **真拓扑 telemetry 替换 fixture** 🟡 body land(T102 DCMI/npu-smi · simulator off · T103 真 GetTopology)·
   真机 stamp:真 PCIE / 真 HCCS ring / 真 node↔node network / 真 utilization(stamp 3 · 🔴 lab)。
4. **真推理** 🟡 body land(T105 CANN + vllm-ascend PD)· 真机 stamp:真 910B 跑 Qwen PD 分离 + `tests/sla`
   P99 vs default SLO(T206 · stamp 4 · 🔴 lab)。
5. **HCCS 亲和放置** 🟡 body land(T103 + scheduler-plugin)· 真机 stamp:真 HCCS 拓扑上 PD 共置(stamp 5 · 🔴 lab)。

> 🟡 = 软件体 + captured-fixture/fake test land + harness 就位;🔴 = 真机实测 pending lab。单点 block →
> 标 lab-driver-gated · 不 gating 收口(plan §8 fallback · checkpoint-phase13 §6 残留)。

---

## 5. 生产硬化验收清单(Phase 13 软件层 land · ADR-0025)

> 出处 `docs/checkpoint-phase12.md` §6 + ADR-0025 Bucket A。**软件层全部 land(P13-T-201..206)**;
> 真机实测 / 真多区 DR 仍 lab-gated(各项注明)。

- [x] **认证授权**:`OIDCValidator`(JWKS+JWT)+ `K8sTokenReviewValidator`(in-cluster 主路径)真体 +
  o2-dms middleware wire + backend prometheus SA token + Dex 参考 IdP + 各 chart 最小权限 RBAC
  (**P13-T-201/T-202** · ADR-0025 §2 Decision A · 甲方 IdP swap = §4(a))。
- [x] **多租户配额强制**:`ClusterQuota` 两 webhook 读 `status.usage.Total` 真强制(cluster cap)+
  scale-rate + clusterquota_controller PerCluster 分桶/RecomputeTotal + scaler cluster gate + fail-open
  (**P13-T-204** · ADR-0025 §2 Decision C · envtest/fake-client 验)。
- [x] **Karmada 多站点 HA**:control-plane 3-replica + 3-node etcd quorum + propagation failover/
  propagateDeps + cross-cluster RBAC ClusterPropagationPolicy(**P13-T-205** · ADR-0025 §2 Decision D)。
  真物理 multi-region LB + external etcd DR 留 lab(ADR-0018 §2 · external-etcd swap 文档化)。
- [x] **Secret 管理**:静态 token/明文 → ESO + Vault(ClusterSecretStore + 4 ExternalSecret · install.sh
  `--with-secrets` · config.real.yaml secret-free)(**P13-T-203** · ADR-0025 §2 Decision B · Vault swap = §4)。
- [x] **推理 SLA**:server-side P99 collector(histogram_quantile)+ scaler latency-aware scale-up +
  `tests/sla` load-test harness + default SLO 文档化(**P13-T-206** · ADR-0025 §2 Decision E · 真 P99 实测
  lab-gated · SLO 阈值甲方 swap = §4(b))。
- [ ] **Go v1 ResourceSlice schema 迁移**(随 K8s 1.36 baseline · 5 模块 lockstep)—— **carry · 移出项目
  核心交付物**(checkpoint-phase13 §7 · v1beta1 shim 仍 work · 兼容维护 ≠ 功能缺口 · lab K8s ≥1.36 触发 ·
  不 gating 收口 · 不构成 Phase 14)。

---

## 6. 故障排查 / 回滚

```bash
# 全量状态快照(参考 e2e-kind "dump cluster on failure")
kubectl get all -A
kubectl -n ocloud-system describe npuslicepool; kubectl describe nodepool npupool
kubectl -n pool-operator-system logs deploy/pool-operator-controller-manager --tail=300
kubectl -n ocloud-system logs deploy/inference-operator --tail=300
kubectl -n ocloud-system logs deploy/npu-dra-driver --tail=300

# 回滚 / 拆除
helm uninstall <release> -n <namespace>                      # 各 helm 组件
make -C operators/pool-operator undeploy                     # pool-operator(kustomize)
./scripts/install.sh --uninstall                             # 单机形态 A
```

常见点:① DRA API 未服务(`kubectl get --raw /apis/resource.k8s.io/v1` 应 200)② 镜像未推到 registry 或 arch 不匹配(arm64 节点拉到 amd64 镜像)③ cert-manager 未就绪导致 inference-operator webhook 起不来。

---

## 7. 参考

- 可执行配方:`.github/workflows/e2e-kind.yml`(构建→部署→断言全链)· `tests/e2e/kind/install.sh` · `scripts/install.sh` · 根 `Makefile` + 各模块 `Makefile`。
- 决策:ADR-0020(aarch64 鲲鹏 + openEuler target)· ADR-0008/0009(PD Router + DRA 分配)· ADR-0013/0014(O2 DMS + 多租户配额)· ADR-0018(Karmada 拓扑)· ADR-0022(one-page workspace)。
- 路线:`docs/architecture.md` §1.3(Phase 路线图)· `docs/checkpoint-phase12.md` §6(Phase 13+ handoff brief)。

---

**END** · 本文随真机 lab 接入(Phase 13+)更新 🔴 段落实测结果。
