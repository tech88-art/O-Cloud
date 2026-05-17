# Backend CLAUDE.md — 演示后端模块协作指南

> 模块责任 Agent 启动新会话时**先读这份**，再开任务包。本文件覆盖根 CLAUDE.md 的通用条款，模块内特殊规则以本文件为准。

---

## 1. 模块定位

`backend/` — Go 实现的演示后端服务，提供：
- REST API（`/api/v1/...`）
- WebSocket 通道（`/ws/...`）
- 多数据源抽象（mock / k8s / crd / prometheus / configmap）

**严格约束**：
- 后端无持久化（无数据库）
- 仅做查询、聚合、短期内存缓存
- 数据源切换只改 `configs/config.yaml`，代码零感知

---

## 2. 模块路径所有权（OWN）

```
backend/**
```

**唯一例外**：`docs/api-contract.yaml` —— 由分配到 P1-T-002 的 agent 维护；其他后端 agent **只读**，需要改走 RFC。

---

## 3. 目录约定

```
backend/
├── cmd/demo-backend/main.go        入口
├── pkg/
│   ├── api/                        Gin handler（每个资源一个文件）
│   │   ├── cluster.go
│   │   ├── node.go
│   │   ├── npu.go
│   │   ├── pool.go
│   │   ├── workload.go
│   │   ├── deploy.go
│   │   ├── preset.go
│   │   ├── metrics.go
│   │   ├── logs.go
│   │   ├── grafana.go
│   │   ├── system.go               健康检查、版本
│   │   └── ws.go                   WebSocket
│   ├── datasource/
│   │   ├── source.go               接口（核心抽象）
│   │   ├── factory.go              按配置创建
│   │   ├── mock/                   Phase 1
│   │   ├── k8s/                    Phase 2
│   │   ├── crd/                    Phase 2-3
│   │   ├── prometheus/             Phase 2
│   │   └── configmap/              Phase 2
│   ├── aggregator/                 跨源聚合（如 Topology = K8s + CRD）
│   ├── config/
│   ├── cache/                      内存 LRU
│   ├── model/                      DTO（与 OpenAPI 1:1 对应）
│   └── middleware/                 logging、tracing、cors、auth
├── configs/
│   ├── config.example.yaml
│   └── config.dev.yaml             开发用，全 mock
├── go.mod
├── go.sum
├── Dockerfile
├── Makefile
└── README.md
```

**禁止**：
- 在 `pkg/api/` 里直接写 K8s client 调用 —— 必须经 `pkg/datasource/`
- 在 `pkg/model/` 里依赖 K8s 类型（model 是纯 DTO）
- 在 handler 里写超过 50 行业务逻辑 —— 抽到 service / aggregator

---

## 4. 关键约定

### 4.1 数据源接口（核心契约）

`pkg/datasource/source.go` 定义：

```go
type Source interface {
    Name() string
    Capabilities() Capabilities // 该 source 支持哪些方法
    
    // Cluster
    ListClusters(ctx) ([]*model.Cluster, error)
    GetCluster(ctx, id) (*model.Cluster, error)
    GetTopology(ctx, clusterID string, depth string) (*model.Topology, error)
    
    // Node / NPU / Slice
    ListNodes(ctx, filter NodeFilter) ([]*model.Node, error)
    GetNodeDetail(ctx, name) (*model.NodeDetail, error)
    ListNPUs(ctx, nodeName string) ([]*model.NPU, error)
    
    // Pool（CRD source）
    ListNPUSlicePools(ctx) ([]*model.NPUSlicePool, error)
    // ... 其他 pool
    
    // Workload
    ListWorkloads(ctx, filter WorkloadFilter) ([]*model.Workload, error)
    GetWorkloadDetail(ctx, ns, name) (*model.WorkloadDetail, error)
    GetWorkloadLogs(ctx, ns, name, opts LogOptions) (*model.LogPage, error)
    
    // Deploy
    ListPresets(ctx) ([]*model.Preset, error)
    GetPreset(ctx, id) (*model.PresetDetail, error)
    Deploy(ctx, req *model.DeployRequest) (*model.DeployResponse, error)
    DeleteDeploy(ctx, deployID string) error
    
    // Metrics
    QueryMetric(ctx, templateID string, vars map[string]string, range_ TimeRange) (*model.MetricQueryResponse, error)
}
```

**任何 datasource 实现新方法 → 必须先在接口里加 → 所有实现必须跟上**（编译失败强制）。

### 4.2 配置加载

`pkg/config/` 用 Viper：

```yaml
server:
  port: 8080
  enableCORS: true
datasources:
  mock:
    enabled: true
    path: "./configs/mock-data/set-a-small"
  k8s:
    enabled: false
    kubeconfig: ""
  prometheus:
    enabled: false
    url: ""
mapping:
  topology: mock
  workloads: mock
  metrics: mock
  presets: mock
```

**约定**：`mapping.<resource>` 决定该资源走哪个 source。`factory.go` 据此构造。

### 4.3 Handler 模板

```go
// pkg/api/cluster.go
package api

func (h *Handler) ListClusters(c *gin.Context) {
    src := h.sourceFor("clusters")
    clusters, err := src.ListClusters(c.Request.Context())
    if err != nil {
        respondError(c, http.StatusInternalServerError, "InternalError", err)
        return
    }
    c.JSON(http.StatusOK, clusters)
}
```

- 不在 handler 里做业务，仅做参数解析 + 调 datasource + 返回
- 错误统一走 `respondError` helper
- 所有 handler 必须有单测（用 httptest）

### 4.4 OpenAPI 一致性

每个 handler 必须与 `docs/api-contract.yaml` 一致：
- 路径
- HTTP 方法
- 请求/响应 schema

CI 会校验。**改 API 前先改契约**。

### 4.5 错误处理

- 用 `fmt.Errorf("doing X: %w", err)` wrap
- 公共错误码定义在 `pkg/api/errors.go`
- 不要 panic（除 main 启动失败）

### 4.6 Logging

用 `zap`，结构化字段：

```go
logger.Info("listing clusters",
    zap.String("source", src.Name()),
    zap.Int("count", len(clusters)))
```

请求级别日志走 middleware，handler 内只记业务关键事件。

### 4.7 WebSocket

`pkg/api/ws.go` 维护连接池。消息格式见 `docs/api-contract.yaml` `WSMessage`。

```go
type WSMessage struct {
    Type      string      `json:"type"`
    Timestamp time.Time   `json:"timestamp"`
    Payload   interface{} `json:"payload"`
}
```

Mock 阶段事件由 `mock.Source` 按 `events.json` 定时回放。

---

## 5. 开发命令

```bash
cd backend

make tidy            # go mod tidy
make build           # 编译到 ./bin/demo-backend
make run             # 本地启动（用 configs/config.dev.yaml）
make test            # 单元测试 + 覆盖率
make lint            # golangci-lint
make fmt             # gofumpt
make docker          # 构建镜像
```

启动后：
- REST: http://localhost:8080
- WS: ws://localhost:8080/ws/topology
- pprof: http://localhost:8080/debug/pprof（dev 模式）

---

## 6. 单测要求

- 每个 handler **必须**有对应的 `_test.go`
- 用 `httptest.NewRecorder` + Gin Engine
- Mock datasource 用 `pkg/datasource/mock` 已有实现
- 覆盖率目标：核心包 ≥ 70%
- 跑 `make test` 不允许失败

---

## 7. 提交规则

PR 标题 Conventional Commits：

```
feat(backend): implement cluster topology API
fix(backend): correct datasource factory wiring
refactor(backend): extract aggregator from cluster handler
test(backend): add coverage for npu handler
```

每个 PR 只做一个任务包（`Refs: P1-T-XXX`）。

---

## 8. Phase 标记

代码里出现：
- `// MOCK` — Mock 实现，Phase 2 切换真实 source 时要审查
- `// PHASE-1` — Phase 1 范围
- `// PHASE-2` — Phase 2 才实现，先放接口骨架
- `// TODO(@<task-id>)` — 关联任务

---

## 9. 禁止行为

- ❌ 修改 `docs/api-contract.yaml` 不走 RFC
- ❌ 在 handler 里直接调 K8s clientset（必须经 datasource）
- ❌ 用 `panic` 处理可恢复错误
- ❌ 引入新依赖不更新 `go.mod` 并跑 `go mod tidy`
- ❌ 把 mock 数据 hardcode 在代码里（用 `configs/mock-data/`）
- ❌ 跨模块修改（任何 `backend/` 之外的文件，除契约外）

---

## 10. 常用 Prompt 模板

### 新增一个 API endpoint

```
读完 backend/CLAUDE.md 和 docs/agent-coordination.md 后，执行任务包 P1-T-XXX。

具体：
1. 在 backend/pkg/api/<resource>.go 新增 handler
2. 在 backend/pkg/datasource/source.go 接口加方法
3. 在 backend/pkg/datasource/mock 加 mock 实现
4. 在 backend/pkg/model 加 DTO（与 docs/api-contract.yaml 一致）
5. 加 handler 单测覆盖 happy path + 1 个错误 case
6. 跑 make test、make lint
7. 提 PR，标题 feat(backend): ..., 引用 P1-T-XXX

仅修改任务包 Allowed Paths 范围内的文件。
```

### 新增数据源实现

```
读完模块文档后，执行 P1-T-XXX：
1. 在 backend/pkg/datasource/<source>/ 实现 Source 接口
2. 在 factory.go 加 case
3. 单测覆盖每个方法的成功 + 失败路径
4. 不修改其他 source 的代码

确认 Capabilities() 准确反映该 source 支持的方法集合。
```
