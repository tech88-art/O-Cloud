# Exporters CLAUDE.md — Prometheus 指标采集模块协作指南

> `exporters/` 模块覆盖所有 O-Cloud 自建的 Prometheus exporter。Phase 3 当前
> 仅 `ascend-npu-exporter-plus` 一个子模块。模块责任 Agent 启动新会话时
> **先读这份**,再开任务包。本文件覆盖根 `CLAUDE.md` 的通用条款,模块
> 内特殊规则以本文件为准。

---

## 1. 模块定位

`exporters/` — Prometheus 指标采集 · 三级粒度(NPU / slice / PID) ·
替代 Phase 2 安装的社区 `ascend-npu-exporter v6.0.0` chart + dev stub。

**两层定位**:

| 子模块 | 范围 | 阶段 |
|---|---|---|
| `ascend-npu-exporter-plus/` | 昇腾 910B NPU + 切片 + 工作负载 PID 指标 | Phase 3 起 |
| (future) | 节点级 / 网络 / 存储等其他 exporter | Phase 4+ |

**严格约束**:
- exporter 进程**无状态** —— 重启即重建,不持久化
- 所有指标 pull 模式(`/metrics` HTTP),禁止 push gateway
- Phase 3 全程 **simulator-only**;DCMI / `npu-smi` 真硬件接入推迟到
  Phase 4+(`internal/collector/sources/dcmi.go` + `npu_smi.go`)
- 自建 chart 与社区 chart **不并存** —— T103 上线时同步下线
  `deploy/helm-charts/ascend-npu-exporter/`(社区 v6.0.0)+ dev stub

---

## 2. 模块路径所有权(OWN)

```
exporters/**
```

任何模块外的修改(`deploy/helm-charts/ascend-npu-exporter-plus/`、
`docs/api-contract.yaml`、`configs/mock-data/schema.json`、根 Makefile 等)
都不在本模块授权范围内,必须走 RFC(详见根 `CLAUDE.md` §8)。

**共享契约依赖**:
- 指标名 / label schema 一旦被 Grafana dashboard 或 backend
  prometheus datasource 引用,即视为契约,修改走 RFC

---

## 3. 工程约定

### 3.1 Go module 边界

每个子模块是**独立的顶级 Go module**,不复用 backend / operators 的
module。理由:exporter 是无 client-go 重型依赖的小二进制,独立 module
打镜像更快、依赖更少。

```
github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus
github.com/example/ocloud-edge/exporters/<future-exporter>
```

### 3.2 目录约定(`ascend-npu-exporter-plus`)

```
exporters/
└── ascend-npu-exporter-plus/
    ├── cmd/exporter-plus/main.go      唯一入口
    ├── internal/
    │   ├── registry/                  Prometheus Registry + build_info + collect_duration
    │   ├── server/                    HTTP /metrics + /healthz
    │   ├── version/                   ldflags 注入目标
    │   ├── collector/                 P3-T-007+ NPU/slice/workload(尚未存在)
    │   │   └── sources/               P4+ DCMI / npu-smi 真硬件来源(尚未存在)
    │   └── testdata/                  P3-T-007+ simulator JSON(尚未存在)
    ├── Dockerfile                     multi-stage distroless, linux/amd64 only
    ├── Makefile
    ├── go.mod / go.sum
    └── README.md
```

### 3.3 平台

- **目标二进制**:linux/amd64(对应昇腾 910B 节点)
- **本机开发**:`make build-local` 不强制目标平台,产 host-OS 二进制供
  functional smoke

### 3.4 指标命名

| 前缀 | 范围 |
|---|---|
| `exporter_*` | exporter 自身元指标(build_info / collect_duration) |
| `ascend_npu_*` | NPU 设备级(P3-T-007) |
| `ascend_slice_*` | 切片级(P3-T-101) |
| `ascend_pid_*` | 工作负载 PID 级(P3-T-102) |

不要把 NPU 指标塞到 `exporter_*` 前缀下;反之亦然。

### 3.5 collect_duration 用法

```go
import "github.com/example/ocloud-edge/exporters/ascend-npu-exporter-plus/internal/registry"

start := time.Now()
defer func() {
    registry.CollectDuration.WithLabelValues("npu").Observe(time.Since(start).Seconds())
}()
```

每个 collector 用自己的 label 值(`npu` / `slice` / `pid`)。

---

## 4. Phase 3 thesis

P3-T-006(本任务) **只做 skeleton**:
- HTTP server / Registry / version / build_info
- 不接 simulator,不接 DCMI
- collector 留给 T007/T101/T102

任何在 T006 阶段接入 `internal/collector/` 的提议 = 越界,该提议属于 T007+。

---

## 5. Phase 4+ promise

```
internal/collector/sources/
├── simulator.go    Phase 3+ 已用
├── dcmi.go         Phase 4+ TODO: 真硬件 DCMI 接入
└── npu_smi.go      Phase 4+ TODO: npu-smi CLI 兜底
```

接入策略:`--simulator <path>` 非空 → simulator;否则尝试 DCMI → 失败
fallback `npu-smi`。具体协议在 P4 RFC 落地。

---

## 6. 开发命令

```sh
cd exporters/ascend-npu-exporter-plus

make build           # linux/amd64 -> bin/exporter-plus
make build-local     # host-OS     -> bin/exporter-plus.local
make test            # go test ./... + cover.out
make docker-build    # linux/amd64 distroless image (IMG=ascend-npu-exporter-plus:dev)
make lint            # golangci-lint
make run             # build-local && ./bin/exporter-plus.local
make clean           # remove bin/ + cover.out
```

`-ldflags` 注入 `internal/version.{Version,Commit,BuildTime}`,通过
`exporter_build_info{version,commit,go_version}` 暴露。

---

## 7. 禁止行为

| 禁止项 | 替代方案 |
|---|---|
| 跨模块改文件(`exporters/` 外的任何文件) | 提 RFC,由对应模块 agent 改 |
| 在 P3-T-006 阶段接入 DCMI / `npu-smi` | 留给 Phase 4+(本模块 §5) |
| 自建 chart 与社区 chart **并存** | T103 上线时同步下线社区 chart 与 dev stub |
| 把 NPU 指标硬编码到 `exporter_*` 前缀 | 用 `ascend_*` 前缀(本模块 §3.4) |
| 把指标采集进程做成有状态(写本地 DB / 文件) | 重启即重建,所有状态来自上游(DCMI / simulator JSON) |
| 引入 client-go / controller-runtime 等重型依赖 | exporter 只读硬件 / 模拟器,不需 K8s client |
| 跨平台二进制混合发布(ARM + amd64) | 仅 linux/amd64,与目标硬件匹配 |

---

## 8. 与 PD Router(ADR-0008)的协作

P3-T-102 PID-level metrics 是 PD Router 的下游 consumer。

- 默认 **关闭**(`--enable-workload-correlation=false`)以避免空载场景下
  的 `/proc` 扫描成本
- 启用前需读 ADR-0008 与 P5 inference-operator 任务包
- correlation 字段(`workload_id` / `pd_role`)在 P5 才稳定;P3 阶段
  metric 名先占位,labels 可空

---

## 9. 测试要求

| 层 | 目标覆盖率 |
|---|---|
| `internal/registry` | ≥ 80% |
| `internal/server` | ≥ 70%(`/metrics` 实抓 + `/healthz`) |
| `internal/collector` (T007+) | ≥ 70% per collector |

CI 强制:
- `go vet ./...` 必无 warning
- `make test` 必绿
- functional gate(real bin + curl `/metrics`)在 PR 描述里实证

---

## 10. 提交规则

Conventional Commits:

```
feat(exporters): P3-T-XXX <summary>
fix(exporters): <summary>
docs(exporters): <summary>
```

PR 走 `dev`,不直接 push `main`。

---

**END of exporters/CLAUDE.md**
