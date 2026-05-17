# Deploy CLAUDE.md — 部署与 DevOps 模块协作指南

> 部署 / CI / CD / 镜像 / Grafana dashboard 全由本模块负责。

---

## 1. 模块定位

```
deploy/
├── dev/                            本地开发栈（docker-compose）
├── single-node/                    K3s + KubeEdge 单节点部署
├── small-cluster/                  标准 K8s 小集群
├── multi-site/                     Karmada 多站点（Phase 9+）
├── grafana-dashboards/             Grafana JSON dashboard 文件
├── helm-charts/                    Helm chart（本项目自有 chart）
└── README.md

.github/workflows/                  GitHub Actions CI/CD（属本模块）
scripts/                            一键脚本（install.sh 等）
hack/                               开发辅助脚本
```

---

## 2. 模块路径所有权（OWN）

```
deploy/**
.github/**
scripts/**
hack/**
```

---

## 3. 关键约定

### 3.1 CI（GitHub Actions）

`.github/workflows/ci.yml` 必须包含 jobs：

| Job | 触发条件 | 内容 |
|---|---|---|
| `lint-backend` | PR 改 backend/** | golangci-lint |
| `test-backend` | PR 改 backend/** | go test + 覆盖率 |
| `build-backend` | PR 改 backend/** | go build |
| `lint-frontend` | PR 改 frontend/** | eslint + typecheck |
| `test-frontend` | PR 改 frontend/** | vitest |
| `build-frontend` | PR 改 frontend/** | vite build |
| `validate-contract` | PR 改 docs/api-contract.yaml 或 backend/** 或 frontend/** | OpenAPI spec lint + backend handler 注解对齐校验 |
| `validate-mockdata` | PR 改 configs/mock-data/** | ajv 校验 schema |
| `check-allowed-paths` | 所有 PR | 解析 PR description 的 `Refs: P1-T-XXX`，对比 diff 路径与任务包 Allowed Paths |
| `e2e` | merge 到 dev | Playwright（W2 起） |

**强制**：任何 PR 必须所有相关 job 全绿才能 merge。

### 3.2 镜像

Phase 1：
- 推 GHCR：`ghcr.io/<org>/ocloud-edge/<service>:<tag>`
- tag 策略：`pr-<num>`, `dev-<sha>`, `v0.x.y`

Phase 2 起迁 Harbor（自建）：
- `harbor.<domain>/ocloud-edge/<service>:<tag>`
- 镜像签名（cosign）

### 3.3 Helm Chart

`deploy/helm-charts/ocloud-edge/` 包含整套：

```
ocloud-edge/
├── Chart.yaml
├── values.yaml
├── values-dev.yaml
├── values-prod.yaml
├── templates/
│   ├── backend-deployment.yaml
│   ├── backend-service.yaml
│   ├── frontend-deployment.yaml
│   ├── frontend-service.yaml
│   ├── ingress.yaml
│   ├── configmap.yaml
│   ├── grafana-datasources-configmap.yaml
│   ├── rbac.yaml
│   └── _helpers.tpl
└── README.md
```

依赖（subcharts 或外部 chart）：
- `kube-prometheus-stack`（Prometheus + Grafana）
- `loki-stack`（Loki + Promtail）

### 3.4 本地开发栈

`deploy/dev/docker-compose.yaml` 一条命令拉起完整栈：

- backend（mount 代码热加载）
- frontend（Vite dev server）
- prometheus（mock，用 OpenMetrics 静态文件或 prometheus-mock）
- grafana（预置 datasources + dashboards）
- loki + promtail（可选）

启动：
```bash
docker-compose -f deploy/dev/docker-compose.yaml up
```

访问：
- 前端：http://localhost:3000
- 后端：http://localhost:8080
- Grafana：http://localhost:3001（admin / admin）
- Prometheus：http://localhost:9090

### 3.5 单节点部署

`deploy/single-node/install.sh`：

```bash
#!/usr/bin/env bash
set -euo pipefail

# 1. 安装 K3s
# 2. 安装 KubeEdge edgecore
# 3. 安装 Ascend Device Plugin（如有真机；Phase 1 跳过）
# 4. 安装 kube-prometheus-stack
# 5. 安装本项目 Helm chart
# 6. 输出访问地址
```

### 3.6 Grafana Dashboards

`deploy/grafana-dashboards/` 下放 JSON 文件，对应 backend `/api/v1/grafana/url` 的 dashboard key：

| 文件 | Dashboard key |
|---|---|
| `cluster-overview.json` | `cluster_overview` |
| `node-detail.json` | `node_detail` |
| `npu-detail.json` | `npu_detail` |
| `workload-business.json` | `workload_business` |
| `workload-resource.json` | `workload_resource` |

文件命名 = key 加 `.json`，下划线变连字符。

通过 Grafana sidecar 或 provisioning 加载。

---

## 4. 开发命令

```bash
# CI 本地预演
act -j lint-backend                  # 用 act 跑 GitHub Action 本地（可选）

# Helm 校验
helm lint deploy/helm-charts/ocloud-edge
helm template deploy/helm-charts/ocloud-edge

# Docker compose
make dev-up                          # 等价 docker-compose -f deploy/dev/docker-compose.yaml up -d
make dev-down
make dev-logs

# 单节点安装
sudo bash deploy/single-node/install.sh
```

---

## 5. 安全约定

- **不**在仓库存任何 secret（用 GitHub Secrets / 部署时注入）
- **不**在 Helm chart 默认 values 启用任何不安全配置
- 镜像构建用多阶段，runtime 用 distroless 或 alpine
- ServiceAccount 最小权限（Phase 2+ RBAC 严格校对）

---

## 6. 禁止行为

- ❌ 直接修改 `main` 分支
- ❌ 在 CI 里跳过 lint / test（不要 `continue-on-error`）
- ❌ 推未签名镜像到 Harbor（Phase 2+）
- ❌ 把 GHCR token / kubeconfig 写进仓库
- ❌ 修改其他模块代码（即使是为了 fix CI）

---

## 7. 常用 Prompt 模板

### 新增 CI job

```
读完 deploy/CLAUDE.md 和 docs/agent-coordination.md 后，执行 P1-T-XXX。

具体：
1. 在 .github/workflows/ci.yml 新增 job
2. 触发条件、运行 step 写清楚
3. 在 PR 描述中说明这个 job 会阻塞哪些 PR
4. 本地用 act（如安装）或者推到测试分支验证

仅改 .github/、scripts/、deploy/ 下的文件。
```

### 写 Helm chart 模板

```
读完模块文档后，执行 P1-T-XXX：
1. 在 deploy/helm-charts/ocloud-edge/templates/ 新增模板
2. values.yaml 里加对应字段（带默认值 + 注释）
3. helm lint 通过
4. helm template 渲染输出 review
5. 不改其他 chart
```

### 调试 Grafana dashboard

```
读完模块文档后：
1. 在 deploy/grafana-dashboards/ 新增 / 修改 JSON
2. 本地起 grafana（docker-compose）验证渲染
3. 截图贴 PR
4. 如新增 dashboard，在 backend 那边对接 grafana_url handler 时通知协调者增白名单

仅改 deploy/grafana-dashboards/。
```
