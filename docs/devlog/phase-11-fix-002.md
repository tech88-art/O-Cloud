# P11-fix-002 · ascend-exporter +5 metric, node-exporter wired in, workload dashboard ⚠ markers

- **Date**: 2026-05-22
- **Duration**: ~0.3d(exporter Go edits + rebuild + node-exporter wiring + verify)
- **Trigger**: After P11-fix-001 unblocked path A bringup, user audit shows
  4 dashboards 中 node-detail / npu-detail / workload-* 各有 0-13 个 No-data
  panel。分类 root cause:metric naming(已在 fix-001 修)/ exporter feature gap
  (本 fix)/ backend metric emission gap(P11+ T201,本 fix 不动)。

## Changes(由用户选择的 A+B+E 组合)

### A — node-exporter container 接入

`deploy/dev/docker-compose.yaml` 新增 `node-exporter` service(`prom/
node-exporter:v1.8.2`)· `--collector.disable-defaults` + 显式启用
cpu / meminfo / netdev / filesystem / loadavg。`deploy/dev/prometheus/
prometheus.yml` 加 scrape job。**Synthetic `node=worker-site-a-01` label**:
单个 docker-host VM 不可能拆 3 个 node 的 metric · 用 1 个 fake node 让
dashboard `$node` selector 仍能解析。Windows Docker Desktop 上看到的是
Linux VM 的 CPU/memory/network,不是真 host — 演示足够,生产路径上
node-exporter 是真 DaemonSet 每 node 1 个。

### B — ascend-npu-exporter-plus 加 5 metric

| Metric | Type | Source field | Synth default |
|---|---|---|---|
| `ascend_npu_temperature_celsius` | gauge `{npu_id,node}` | `NPUSample.TemperatureCelsius` | 65°C base · clamp [30,95] |
| `ascend_npu_power_watts` | gauge `{npu_id,node}` | `NPUSample.PowerWatts` | 300W base · clamp [50,400] |
| `ascend_npu_vram_total_bytes` | gauge `{npu_id,node,model}` | `NPUSample.MemoryTotalBytes`(已存在 field,未暴露) | from simulator JSON |
| `ascend_npu_vram_used_percent` | gauge `{npu_id,node}` | **derived** `100*memory_used/memory_total` | from above |
| `ascend_npu_slice_util_percent` | gauge `{slice_id,npu_id,node,template,workload}` | `SliceSample.AICoreUtilization`(only for allocated) | 50% base |

Simulator 同 ±10% 正弦扰动复用现有 `factor` · 6 个 NPU 维 metric 共相位偏移
(避免 dashboard 上完全一致的曲线)· slice util 走 π/4 偏移与父 NPU 解耦
(原 slice memory 即如此)。

**Backward compat**: 所有新 field `omitempty` JSON tag · 旧 testdata
JSON 不需改;`NPUSample` / `SliceSample` 仅 additive · 外部 caller 不破。
`TestNPUCollector_Describe` 期望 3 → 7 · `TestSliceCollector_Describe`
3 → 4 · 已同步更新。

### E — workload-business / workload-resource 描述加 ⚠ 标记

两个 dashboard 的 No-data 不是 metric naming bug · 是 backend 没 emit
workload_*_total / _ms_bucket histogram(T201 工作)+ cAdvisor 没启用
(可选 path D)。**故意不修 panel** · 只在 dashboard `description` 加
"⚠ Demo state" 前缀解释 phase 11+ 工作 + 引导用户用 cluster-overview /
node-detail / npu-detail。这避免假装 fix 隐藏 P11+ 真工作。

## Validation

- `go test ./internal/collector/...` ✅(7 / 4 expected count 更新通)
- 4 Prom targets up: prometheus / backend / ascend-npu-exporter / node-exporter
- 新 metric Prom series count:
  - ascend_npu_temperature_celsius = 24
  - ascend_npu_power_watts = 24
  - ascend_npu_vram_total_bytes = 24
  - ascend_npu_vram_used_percent = 24
  - ascend_npu_slice_util_percent = 4(only allocated slices)
- node-exporter:`count(node_cpu_seconds_total)=64`(8 core × 8 modes,Linux
  VM 实际)· node_memory_MemAvailable_bytes present

## P3 P4 自我审计

P11-fix-001 后我以为 metric rename 是 deploy 模块单方面问题;实际审计
4 个 dashboard 才发现 exporter 本身有真 feature gap · `MemoryTotalBytes`
field 已存在但未暴露 metric 这种细节也走漏 — 跨 module systematic
audit 才能发现 (exporter / deploy / grafana 3 处协同设计 system)。
exporters/CLAUDE.md §2 说 "指标名一旦被 dashboard 引用即视为契约,修改
走 RFC" — 本 fix 是 **兑现** dashboard 已有期待的契约,不是修改 · 不
需要 RFC。

## 不修(P11+ 真工作)

- backend `workload_*_total` / `workload_*_ms_bucket` histogram emit — T201
- cAdvisor 接入 — 可选 path D(workload-resource 的 container_* panel)
- DCMI / npu-smi 真硬件源 — Phase 4+ exporter §5 已 promise

## Refs

- P11-fix-001(devlog phase-11-fix-001.md)— path A bringup unblock
- exporters/CLAUDE.md §3.4 指标命名规则 + §10 commit prefix
- deploy/CLAUDE.md §3.4 dev 栈定义
