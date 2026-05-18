# Checkpoint Demo — W3 Stage 3 (2026-05-18)

> 阶段交付件。W3 12/13 完成,仅剩 T214(workload 融合主 Topology)+ W4(7 tasks)。

---

## 已交付清单(W3 全量)

### Stage 2 (6 tasks) — 见 `checkpoint-w2-batch1.md` 之后
- T201 workloads API + filter + PD pair detail
- T203 presets API
- T204 metrics proxy(white-listed PromQL + **var-slice**)
- T205 grafana URL handler
- T209 5 Grafana dashboards JSON
- T211 Topology fabric extend(`?includeFabric=true`)

### Stage 3 (6 tasks)
- T202 deploy POST/DELETE handlers
- T206 Workloads page(table + filter + drawer + PD-pair relations)
- T207 Deploy page(preset grid + 2-step wizard auto/manual)
- T208 Metrics page(5 dashboard tabs + var selectors + iframe)
- T212 Topology fabric toggle(Overview "Include Fabric" Switch)
- T213 Topology workload extend(`?includeWorkloads=true`)

### 中途 fix-up
- factory.go wire mock source (W2 batch-1 时发现)
- main.go GrafanaBaseURL wiring (T205 follow-up)
- T013 schema v0.2.0 + fabric/bindings (RFC-003)

---

## 服务现状

| 服务 | URL | 启动方式 |
|---|---|---|
| Backend | http://localhost:8080 | `cd backend && ./bin/demo-backend.exe -c configs/config.dev.yaml` |
| Frontend | http://localhost:3000 | `cd frontend && pnpm dev --host 0.0.0.0` |
| Grafana | http://localhost:3001 | `docker compose -f deploy/dev/docker-compose.yaml up -d` |
| Prometheus | http://localhost:9090 | 同上 |

---

## 手工验证清单

### 1. Backend 完整 REST surface

```bash
# 基础
curl http://localhost:8080/api/v1/healthz                 # mock:ok
curl http://localhost:8080/api/v1/clusters                # 1 cluster
curl http://localhost:8080/api/v1/nodes                   # 3 nodes
curl http://localhost:8080/api/v1/nodes/worker-site-a-01/npus   # 8 NPUs

# 拓扑(默认 + 含 fabric + 含 workloads)
curl "http://localhost:8080/api/v1/clusters/cluster-prod-a-01/topology?depth=slice"
curl "http://localhost:8080/api/v1/clusters/cluster-prod-a-01/topology?depth=slice&includeFabric=true"
curl "http://localhost:8080/api/v1/clusters/cluster-prod-a-01/topology?depth=slice&includeWorkloads=true"
curl "http://localhost:8080/api/v1/clusters/cluster-prod-a-01/topology?depth=slice&includeFabric=true&includeWorkloads=true"
# 全开:126 nodes(1c+3n+24npu+66slice+1sw+10wl+21pod)/ 108 edges(93contains+11binds-to+3fabric-link+1pd-pair)

# Workloads / Presets
curl http://localhost:8080/api/v1/workloads                # 10 workloads
curl "http://localhost:8080/api/v1/workloads?status=running"
curl http://localhost:8080/api/v1/workloads/ai-inference/qwen-8b-pd   # PD pair detail
curl http://localhost:8080/api/v1/presets                   # 4 presets

# Deploy
curl -X POST http://localhost:8080/api/v1/deploy \
  -H "Content-Type: application/json" \
  -d '{"presetId":"pi-3b","replicas":1,"namespace":"ai-inference","scheduling":{"mode":"auto"}}'
# -> 201 {"deployId":"d-1",...}

curl -X DELETE http://localhost:8080/api/v1/deploy/d-1     # 204

# Metrics (var-slice spec F4a)
curl http://localhost:8080/api/v1/metrics/templates        # 9 templates
curl "http://localhost:8080/api/v1/metrics/query?templateId=npu_aicore_util&var-slice=worker-site-a-01-npu-2-slice-0&from=2026-05-17T00:00:00Z&to=2026-05-17T00:10:00Z"
# -> 200 points

# Grafana URL
curl "http://localhost:8080/api/v1/grafana/url?dashboard=cluster_overview&var-cluster=cluster-prod-a-01"

# WebSocket
# wscat -c ws://localhost:8080/ws/topology (replays events.json)
```

### 2. Frontend 全 5 页

打开 http://localhost:3000:

| Route | 应见 |
|---|---|
| `/overview` | 左资源树 + ReactFlow 拓扑 + 右 DetailPanel + **Include Fabric** 开关(默认 OFF) |
| `/workloads` | AntD Table 10 workloads + status/namespace/type filter + 行点击打开 Drawer 显示 pods + PD-pair Tag + container 启动命令 |
| `/deploy` | 4 preset cards(Pi 3B / Qwen 8B PD / DeepSeek 20B / Qwen 14B)→ 点 card 打开 Modal 向导 → 选 auto/manual + namespace + replicas → 提交 POST /deploy |
| `/metrics` | 5 dashboard tabs(cluster/node/npu/workload-business/workload-resource)+ var selectors + GrafanaPanel iframe(若 Grafana 未起则 iframe blank) |
| `/logs` | 占位(T302 W4 实现) |
| `/poc/topology` | T009 ReactFlow vs G6 POC(辅助参考) |

**Header**:语言切换 / Sider 折叠 / 项目品牌 + 版本。

### 3. 单测全量

```bash
cd backend && go test ./pkg/api/... ./pkg/aggregator/... ./pkg/datasource/mock/...
# ~70 tests pass
cd frontend && pnpm test
# ~80 tests pass across pages + components + hooks
cd tests/e2e && pnpm exec playwright test
# 3 passed + 1 skip (WS event timing)
```

---

## Phase 1 进度

| Week | 完成 / 总数 | Tag |
|---|---|---|
| W1 | 13 / 13 ✓ | w1-complete |
| W2 | 11 / 11 ✓ | w2-complete |
| W3 | **12 / 13**(剩 T214 workload 融合主 Topology) | — |
| W4 | 0 / 7 | — |
| **总** | **36 / 44 = 82%** | |

dev 当前 HEAD: `26cdc8e` feat(backend): P1-T-213 — topology workload extend.

---

## 已知限制

1. **T214 待做**:workload + pod 节点在 backend topology API 已可暴露,但 frontend TopologyGraph 还没渲染 workload/pod 类型(T214 完成后 Overview 加 "Include Workloads" toggle 类似 T212 fabric)。
2. **T201 silently drops Pod.bindings**:T013 加了 schema field,但 OpenAPI api-contract.yaml 没同步;backend `model.Pod` 也没 bindings field。Pod.bindings 在前端 Workload Detail Drawer 显示不出来。Follow-up: api-contract.yaml + model.Pod 加 bindings(走 §0a.5 chat+ADR)。
3. **GrafanaPanel react-refresh warning**(T010 遗留):每个 frontend subagent 都 spot 到,outside 各 task Allowed Paths。Follow-up: 单独 fix-up commit(~5 min)。
4. **T013 generator drift**:set-a-small JSON 用 node 手编 patched(fabric + bindings);Go generator 未同步。下次 `make gen-small` 不会重现 fabric / bindings。Follow-up: extend generator OR document hand-edits 为 canonical。
5. **演示数据 D6 affinity 对比**:T307 W4 task,已加 AC,未实现。
6. **WS event drives status change E2E**:T110 skipped,需 `?fastforward=` query param 让 WS replay 确定时序。

---

## 下一步

1. **W3 Stage 4**:**T214** frontend workload 融合主 Topology(deps T212 + T213,~1d)
2. **W4 全量 7 tasks**:
   - T301 backend Logs API + WS
   - T302 frontend Logs page
   - T303 配置驱动数据源切换(set-a ↔ set-b 验证零代码切换)
   - T304 install.sh 单节点一键部署
   - T305 E2E test suite(5 页主流程)
   - T306 docs 收尾(README / demo.md / known-issues.md)
   - T307 演示数据集(包括 D6 affinity comparison)
3. **DoD 验收**(phase1-plan §9):8 项 Must Have 全打勾 + tag `phase-1-complete`
4. **3 个 follow-up fix-up**(上面"已知限制"#2-#4)

---

## 切 session brief

```
项目: D:/code/ai-edge
分支: dev (HEAD 26cdc8e)
Tags: phase-0-baseline / w1-complete / w2-complete

W1 13/13 ✓ · W2 11/11 ✓ · W3 12/13(剩 T214)· W4 0/7
Phase 1 进度 36/44 = 82%

必读:
- CLAUDE.md(项目根)
- docs/agent-coordination.md §0a(operative)+ §0a.9(worktree 协议)
- docs/architecture.md §13(flag-to-phase)
- docs/demo/checkpoint-w3.md(本文件)
- docs/adr/0001-0005(DRA / KServe / IMS / Fabric / Workload-fusion)

工具链(已持久化 user PATH):
- Go 1.26.3 (D:/tools/go/bin)
- pnpm 11.1.2 (APPDATA/npm)
- controller-gen v0.21.0 / kubebuilder v4.14.0 (USERPROFILE/go/bin)

服务运行中:
- backend :8080 (pid 浮动)
- frontend :3000 (Vite dev,若没起需 cd frontend && pnpm dev --host 0.0.0.0)

下一步:T214 → W4 7 tasks → DoD → tag phase-1-complete
```
