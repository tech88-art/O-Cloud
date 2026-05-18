# Checkpoint Demo — W2 Batch-1 (2026-05-17)

> 阶段交付件。手工验证脚本 + 期望产出。后端 + 前端在本机已起,可直接打开。

---

## 已交付清单

### Phase 0
- 架构 + ADR-0001 / ADR-0001 §5 v2(2026-05-17 RFC-001 驳正 DRA GA 时间到 K8s 1.34)
- agent-coordination §0a 单 agent operative model + §0a.9 worktree 隔离协议(RFC-002,2026-05-17 W1 复盘补丁)
- OpenAPI 契约(23 REST + 4 WS,sealed-for-review)
- Mock 数据 schema(draft 2020-12,9 顶层数组,19 个 $defs)
- 5 module CLAUDE.md(backend / frontend / operators / configs / deploy)

### W1(12 / 12 任务,phase tag `w1-complete`)
- T001 LICENSE + scaffold
- T002 OpenAPI sealed-for-review
- T003 4 个 pool CRD(Kubebuilder v4.14 + controller-gen v0.21)
- T004 Mock schema sealed-for-review
- T005 Backend Go scaffold(Gin + Source interface 16 methods)
- T006 Frontend Vite/React 18/TS 5/AntD 5 scaffold
- T007 CI 实填(scripts/ci/check-allowed-paths.sh)
- T008 docker-compose dev 栈
- T009 G6 vs ReactFlow POC → **结论:ReactFlow**(6 维度对比)
- T010 Grafana iframe POC
- T011 Mock generator(Go + cobra + gofakeit seed=42)
- T012 调研(Ascend Device Plugin / K8s DRA / vLLM PD,3 文档 397 行)

### W2 Batch-1(5 / 10 任务)
- T101 Cluster API(/clusters, /clusters/:id)
- T103 Nodes API(/nodes, /nodes/:name,?role= filter)
- T106 Layout & navigation polish(brand+version, lang dropdown 持久化, icons)
- T107 5 个可复用组件(ResourceCard / StatusTag / MetricChip / EmptyState / ErrorState)
- T109 set-a-small 数据故事化(32 events, 1 degraded NPU, Qwen 8B PD pair)

### Post-batch fix-up
- `fix(backend): wire mock source via main.go injection` — T005 的 factory.go stub 没在 T101/T103 被补;现在 main.go 注入 mock.NewSource → handler 真服务数据

---

## 当前服务状态

| 服务 | URL | 状态 |
|---|---|---|
| **Backend** | http://localhost:8080 | ✓ 已启动(`./bin/demo-backend.exe -c configs/config.dev.yaml`) |
| **Frontend** | http://localhost:3000 | ✓ 已启动(`pnpm dev`, Vite HMR) |
| Grafana | http://localhost:3001 | ⚠️ 仅 docker-compose 起栈才有(本次跳过 Docker) |
| Prometheus | http://localhost:9090 | ⚠️ 同上 |

---

## 手工验证清单

### 1. Backend REST 验证(已 curl 过,你可重跑)

```bash
curl http://localhost:8080/api/v1/healthz
# 期望: {"status":"ok","datasources":{"mock":"ok"}}
# ← datasources 不再是空 map,mock source 已注册

curl http://localhost:8080/api/v1/version
# 期望: {"version":"0.1.0"}

curl http://localhost:8080/api/v1/clusters
# 期望: 1 个 cluster
# id:        cluster-prod-a-01
# role:      edge-single
# location:  site-a-shanghai
# status:    healthy
# nodeCount: 3, npuCount: 24

curl http://localhost:8080/api/v1/clusters/cluster-prod-a-01
# 期望: 同上单个 cluster 对象

curl http://localhost:8080/api/v1/clusters/nonexistent
# 期望: HTTP 404 + {"code":"NotFound", "details":{...}}

curl http://localhost:8080/api/v1/nodes
# 期望: 3 个 worker 节点
# names: worker-site-a-01, worker-site-a-02, worker-site-a-03
# 每个 96 CPU / 768 Gi mem / 8 NPU / Ascend910B atlas-800t-a2

curl "http://localhost:8080/api/v1/nodes?role=worker"
# 期望: 同上 3 个(过滤生效)

curl "http://localhost:8080/api/v1/nodes?role=control-plane"
# 期望: [] (set-a-small 无 control-plane 节点)

curl http://localhost:8080/api/v1/nodes/worker-site-a-01
# 期望: NodeDetail 含 NUMA(2 节点)+ networkInterfaces + storage

curl http://localhost:8080/api/v1/nodes/unknown
# 期望: HTTP 404
```

### 2. Frontend 浏览器验证

**打开 http://localhost:3000**

应该看到:
- **AntD Layout 主壳**:
  - 顶部 Header:左侧 brand "O-Cloud Edge" + version tag `v0.1.0`;右侧 sider 折叠按钮 + 语言下拉(🇨🇳 中文 / 🇺🇸 English)
  - 左侧 Sider:5 个菜单项(概览拓扑 / 工作负载 / 应用部署 / 性能指标 / 日志)各带 AntD 图标
  - 中央 Content:渲染当前路由

- **/overview**:AntD Title "概览拓扑" + `<Empty />` 占位("Overview 拓扑页 — W2 即将上线")
- **/workloads**:同样 placeholder
- **/deploy**:同样
- **/metrics**:同样
- **/logs**:同样

**点击 Header 语言下拉切到 English**:菜单和占位文案全部立即变为英文(无 reload)。刷新页面 → 语言保持(localStorage 持久化)。

**Header sider 折叠按钮**:Sider 收缩为图标列。

### 3. POC 页面(独立路由,不挂主菜单)

**打开 http://localhost:3000/poc/topology**

T009 POC:G6 vs ReactFlow 双 tab,各渲染 10 节点拓扑(cluster → node → NPU → slice 层级),支持点击高亮 + 缩放。Header 上有 FPS 计数器(useFpsMeter)。

PR 描述里的对比表 6 维度 → 推荐 **ReactFlow v12**(理由:React 组件自定义节点对接 AntD 友好;TS 一等公民;100-NPU 规模 DOM 成本可控)。

### 4. 单测全量(可选自验)

```bash
cd backend
go test ./pkg/api/...
# 期望: ok pkg/api 18 tests pass (cluster + node + system)

cd ../frontend
pnpm typecheck    # 期望: clean
pnpm test         # 期望: 53/53 pass across 8 files

cd ..
npx -y -p ajv-cli@5 ajv validate \
  --spec=draft2020 --strict=false \
  -s configs/mock-data/schema.json \
  -d "configs/mock-data/set-a-small/*.json"
# 期望: 9/9 valid
```

---

## 已知限制(本 checkpoint 不覆盖)

1. **前端 5 个页面只是 placeholder**。真正的 Overview 拓扑页是 T108a/T108b(Stage 4-5)。
2. **后端 /clusters/:id/topology, /workloads, /presets, /metrics, /logs 还没接**(T102 / T201-T205 / T301)。
3. **WebSocket /ws/topology 未实现**(T105 ,Stage 4)。
4. **Grafana / Prometheus 未起**(Docker daemon 未运行;若需要可启动 Docker Desktop 后 `docker compose -f deploy/dev/docker-compose.yaml up -d`)。
5. **mock.Source 不读 events.json**。T109 polish 了 events.json(32 事件)但 mock.Source 不订阅它 — 这是 T105 WS 任务的事。
6. **Backend /metrics endpoint 不存在** → Prometheus scrape 会 404(T204 才接 prometheus exporter)。
7. **真实 K8s / Ascend 硬件未接** — Phase 2 起对接。

---

## 关闭服务

```bash
# Stop backend (PID found via)
# Get-NetTCPConnection -LocalPort 8080 | %{ Stop-Process -Id $_.OwningProcess -Force }

# Stop frontend (Vite dev server)
# Get-NetTCPConnection -LocalPort 3000 | %{ Stop-Process -Id $_.OwningProcess -Force }
```

---

## 下一步(批准后继续)

W2 剩余 5 任务的关键路径:

```
Stage 2 (single):   T104 NPUs API           (deps T103 ✓)
Stage 3 (single):   T102 Topology API       (deps T101 ✓ + T103 ✓ + T104)
Stage 4 (parallel): T105 WS topology + T108a Overview 骨架   (deps T102)
Stage 5 (single):   T108b Overview 详情+WS  (deps T108a + T105)
Stage 6 (single):   T110 E2E                (deps T108b)
```

继续启动 Stage 2 → user 决定。
