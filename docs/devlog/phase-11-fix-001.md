# P11-fix-001 · dev-stack docker-compose path A bringup unblock(8 cross-module issues)

- **Date**: 2026-05-22
- **Duration**: ~0.4d(interactive triage + fix-verify loops)
- **Trigger**: Phase 11 plan 起草后人工"先看当前阶段效果是否符合预取"·
  `make dev-up`(路径 A)冷启从未走通 · 路径 B(host 跑 backend+frontend)能起但 Grafana iframe 空 · 演示价值打折。

## Issues + fixes(in failure order)

1. **C 盘 0 字节剩 / Docker daemon Internal Server Error** — Docker Desktop
   image-store 把 C 盘吃满 · daemon 进入降级。手动清 C 盘 + restart Docker
   Desktop(不在 commit 范围 · 操作型 fix)。

2. **grafana 11.5.0 broken on Windows Docker Desktop** —
   `exec /run.sh: exec format error` on amd64/linux host with amd64/linux image
   (架构匹配但 image binary 整体不可执行)· `docker run cat /run.sh` 同样
   失败 = image-level breakage。降级到 11.4.0 跑通。

3. **pnpm 10 严格 postinstall 阻断 esbuild** — corepack 默认拉 pnpm 11.2.2 ·
   `[ERR_PNPM_IGNORED_BUILDS] Ignored build scripts: esbuild@0.21.5` · Vite
   启动时 require esbuild 失败 boot-loop。三次 retry(`pnpm rebuild esbuild` /
   `onlyBuiltDependencies` in workspace yaml / `--allow-build`)均无效。最终
   `corepack prepare pnpm@9.15.0 --activate` 锁 pnpm 9.x(没此安全策略)·
   esbuild postinstall 自然跑。

4. **pnpm 9 看 workspace yaml 报 missing packages** — 加 `--ignore-workspace`
   flag · pnpm install 完全无视 pnpm-workspace.yaml。该 yaml host 上有
   `onlyBuiltDependencies: [esbuild]`(pnpm 10 settings 新位置)· 仍保留供
   host workflow 用。

5. **backend mock-data 相对路径解析错** —
   `backend/configs/config.dev.yaml` 用 `../configs/mock-data/set-a-small` ·
   host cwd=`backend/` 时正确(parent = repo root)· 容器 cwd=`/app` 时解析
   到 `/configs/mock-data/...` · 而 docker-compose mount 在 `/app/configs/
   mock-data` · 找不到文件 → 所有 `/api/v1/clusters` 500。修法:mount target
   改 `/configs/mock-data` 让 host 和 container 两种 cwd 都对(不动 config
   文件 · path B 不受影响)。

6. **Grafana datasource UID mismatch** — dashboard JSONs hardcode
   `"uid": "prometheus"`(13 处)· provisioning yaml 没声明 uid · Grafana
   auto-gen `PBFA97CFB590B2093` · 所有 panel 查不到 datasource。修法:
   datasource yaml 加 `uid: prometheus` + `uid: loki` · 但需删
   `ocloud-dev_grafana-data` named volume(`editable: false` + cached
   datasource UID 不会被 provisioning 覆盖更新)。

7-13. **Dashboard JSON metric name mismatch with in-tree exporter** —
   exporter(P3-T-103)实际暴露 6 个 metric · dashboards(P1-T-209)期待
   完整 prom-stack。可 rename 修的(纯命名漂移):
   - `ascend_npu_aicore_util_percent` → `ascend_npu_utilization_percent`(5+1+1 处)
   - `ascend_npu_vram_used_bytes` → `ascend_npu_memory_used_bytes`(npu-detail)
   - `ascend_npu_info` → `ascend_npu_utilization_percent`(用作 count)
   - `kube_node_status_condition{...}` → `count by (node)(ascend_npu_utilization_percent)`
   - `ascend_npu_slice_allocated` → `ascend_slice_allocated_to_pod`
   - `ascend_npu_slice_total` → `ascend_slice_aicore_count`
   共 13 处 · 跨 cluster-overview / node-detail / npu-detail 3 个 dashboard。

## 不修(诚实 No-data · 非命名 bug · P11+ 工作)

- **温度 / 功耗 / 切片级 util** — exporter 从未暴露 · 需 P3-T-103 增强(增
  `ascend_npu_temperature_celsius` / `_power_watts` / `_slice_util_percent`)
- **节点 CPU/内存/网络** — 需 node-exporter 容器(dev-stack 没启)
- **Pod 资源**(container_*)— 需 cAdvisor / 真 K8s · dev-stack 不适用
- **Workload 业务**(TTFT/ITL/tokens/requests histogram)— backend `/metrics`
  目前只 emit Go runtime · 需 T201 真业务 metric emission(Phase 11+)

Fake-fixing 这些(stub data 等)会隐藏 P11+ 真工作 · 故保留 No-data 状态。

## Validation

- 5 容器全 healthy / running(`docker ps`)
- 3 Prom targets up:prometheus / backend / ascend-npu-exporter
- 关键 metric 实际 ingest:
  - `count(ascend_npu_utilization_percent) = 24`
  - `count(count by (node)(...)) = 3`
  - `count(ascend_slice_allocated_to_pod) = 4`
  - `count(ascend_slice_aicore_count) = 12`
  - `avg(ascend_npu_utilization_percent) = 60.1%`
- Grafana Cluster Overview 全 panel 渲染真数据(用户截图确认 60.1% gauge +
  24-NPU heatmap + Top-10 + avg/p95/max 时序)
- `/api/v1/clusters` HTTP 200 + 返回 cluster-prod-a-01

## P7 自我审计

第 1-3 轮我 claim "all green" 都基于 healthz / container Up · 没真 probe
业务 endpoint / Grafana proxy query。被用户截图反复打脸。教训:
**healthz 200 ≠ functional · container healthy ≠ datasource UID 对**。下次
dev-stack bringup 必带:(a) 业务 endpoint probe(至少 1 个 GET data)·
(b) Grafana `/api/datasources/proxy/uid/<X>/api/v1/query` 端到端 probe。

## Refs

- ADR-0002(no KServe)— 不变
- README §4.4 Demo · path A 4-URL 截止此 commit 真演示就绪
- `feedback_strict_per_task_verify.md` · `feedback_push_at_phase_tag_only.md`
  — commit local · not pushing(等 phase tag)
