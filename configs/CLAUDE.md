# Configs CLAUDE.md — 配置与 Mock 数据模块协作指南

> 本模块负责所有配置示例 + Phase 1 演示用的 Mock 数据集。

---

## 1. 模块定位

```
configs/
├── config.example.yaml             演示后端配置模板
├── config.dev.yaml                 本地开发配置（全 mock）
├── mock-data/
│   ├── schema.json                 ★ JSON Schema（所有数据集必须符合）
│   ├── README.md
│   ├── generator/                  数据生成器（Go 或 Python）
│   ├── set-a-small/                小规模演示集
│   ├── set-b-multi-site/           多站点演示集
│   └── set-c-stress/               压测演示集
└── grafana-datasources/            Grafana datasource provisioning
```

---

## 2. 模块路径所有权（OWN）

```
configs/**
```

---

## 3. Mock 数据集要求

### 3.1 必须符合 schema

`configs/mock-data/schema.json` 是唯一真理。**CI 强制校验**所有 JSON 文件符合 schema。

校验命令：
```bash
ajv validate -s configs/mock-data/schema.json -d "configs/mock-data/set-*/**/*.json"
```

### 3.2 数据集结构

每个 `set-*` 目录必须包含：

```
set-a-small/
├── meta.json
├── clusters.json
├── nodes.json
├── npus.json
├── slices.json
├── workloads.json
├── pools.json
├── presets.json
└── events.json
```

或一个汇总文件 `set.json`（包含所有 section）—— 二选一，推荐分文件方便维护。

### 3.3 数据真实度（关键）

**目标**：演示时让评审者感觉"这就是真集群"。

**约束**：
- 节点命名规范：`<role>-<location>-<index>`，例如 `worker-site-a-01`
- NPU 命名：`<nodeName>-npu-<index>`
- 切片命名：`<npuId>-slice-<index>`
- NPU 容量贴近 910B：`vramMiB: 65536`（64GB），`aiCoreTotal: 32`
- HCCS 组：每 4 / 8 卡一组（按 Atlas 服务器实际配置）
- 节点 NUMA：2 个 NUMA 节点，每个挂 4 张 NPU 是常见
- 时间戳：`createdAt` 错开几天到几周（不要全是同一秒）
- 工作负载状态：混合 running / pending / succeeded（不要全 running）
- 利用率：呈现真实分布（不要所有 NPU 都是 50%）
- 事件流：5-10 秒一个事件，让演示动起来

### 3.4 三个数据集的差异化

| 数据集 | 用途 | 规模 |
|---|---|---|
| **set-a-small** | 单页能看完的小演示 | 1 集群 / 3 节点 / 24 NPU |
| **set-b-multi-site** | 多站点联邦演示 | 3 集群（站点A/B/C）/ 9 节点 / 72 NPU |
| **set-c-stress** | 压测前端拓扑渲染 | 1 集群 / 100 节点 / 800 NPU |

---

## 4. 数据生成器

### 4.1 推荐方案

Go 程序，输出 JSON，可参数化：

```bash
cd configs/mock-data/generator
go run . --preset small  --output ../set-a-small/
go run . --preset multi  --output ../set-b-multi-site/
go run . --preset stress --output ../set-c-stress/
```

生成器内部：
- 用 `gofakeit` 造假数据
- 用固定种子（保证可重现）
- 严格按 schema 字段
- 内部生成后过一遍 schema 校验

### 4.2 事件流生成

`events.json` 是模拟实时事件，按时间排序，**第一条事件相对当前时间为 0 秒**：

```json
[
  { "timestamp": "+0s",  "type": "workload.statusChanged", "payload": {...} },
  { "timestamp": "+5s",  "type": "slice.allocated", "payload": {...} },
  { "timestamp": "+12s", "type": "topology.update", "payload": {...} }
]
```

backend 的 `mock.Source` 按这个时间表回放（启动时锚定 t0）。

---

## 5. 配置文件示例

### 5.1 config.example.yaml

完整字段都注释，作为运维参考。

### 5.2 config.dev.yaml

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
  prometheus:
    enabled: false
  configmap:
    enabled: false

mapping:
  topology: mock
  nodes: mock
  npus: mock
  pools: mock
  workloads: mock
  presets: mock
  metrics: mock
  logs: mock

grafana:
  enabled: false
  baseURL: ""
```

---

## 6. Grafana datasource provisioning

`configs/grafana-datasources/datasources.yaml`：

```yaml
apiVersion: 1
datasources:
  - name: Prometheus
    type: prometheus
    access: proxy
    url: http://prometheus:9090
    isDefault: true
  - name: Loki
    type: loki
    access: proxy
    url: http://loki:3100
```

部署时挂到 Grafana 的 `/etc/grafana/provisioning/datasources/`。

---

## 7. 开发命令

```bash
cd configs/mock-data

# 生成数据集
make gen-small
make gen-multi
make gen-stress

# 校验现有数据集
make validate

# 全部重生成 + 校验
make all
```

`make validate` 必须在 CI 跑。

---

## 8. 禁止行为

- ❌ 手改 `configs/mock-data/set-*/`下的 JSON 文件（除非是临时修补 + 立刻同步到生成器）
- ❌ 数据集不符合 schema 就提交
- ❌ 修改 `schema.json` 不走 RFC
- ❌ 跨模块改文件

---

## 9. 常用 Prompt 模板

### 新增一个数据集场景

```
读完 configs/CLAUDE.md 后，执行 P1-T-XXX。

具体：
1. 在 configs/mock-data/generator/ 加新 preset
2. 输出到 configs/mock-data/set-<name>/
3. 跑 make validate 全过
4. 在 README.md 描述这个数据集的演示用途

要求：数据真实度高（命名规范、HCCS 组、NUMA、利用率分布合理）。
```

### 修改 schema（必须先 RFC）

```
本任务要修改 configs/mock-data/schema.json。

请先：
1. 开 RFC issue 描述变更与影响
2. 等协调者批准
3. 改 schema
4. 同时改生成器
5. 同时改后端 model（如有结构变更）
6. 重新生成所有数据集并校验

不要直接动 schema。
```
