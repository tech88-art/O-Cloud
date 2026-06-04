# ascend-npu-exporter-plus — module detailed design

> Self-built Prometheus exporter for 昇腾 910B telemetry at three granularities
> (NPU / slice / workload-PID). Stateless per-node DaemonSet · pull `/metrics` ·
> replaces the community `ascend-npu-exporter v6.0.0` chart. Skeleton Phase 3
> (P3-T-006); NPU collector T007; slice T101; PID T102. **Phase 13 P13-T-102**
> landed the real DCMI / npu-smi telemetry body (simulator off on the real
> profile). This DESIGN.md is a closer backfill (CLAUDE.md §14.2) documenting
> the shipped code.

---

## 1. 架构概览

### 1.1 模块在系统中的位置

```
   per-NPU-node (DaemonSet · 昇腾 910B host)
   ┌─────────────────────────────────────────────────────────────┐
   │ ascend-npu-exporter-plus (stateless · no client-go)           │
   │                                                               │
   │  cmd/exporter-plus/main.go                                    │
   │    └─ sources.Select(cfg) ── THE DECOUPLING SEAM (ADR-0024 G) │
   │         ├─ simulator/mock → replay JSON snapshot (demo)       │
   │         └─ real → DCMI(no-cgo)→npu-smi exec (real · T102)     │
   │                          │ Source / SliceSource interface      │
   │              ┌───────────┴────────────┐                        │
   │      collector.{NPU,Slice,Workload}Collector                  │
   │              │ prometheus.Collector (scrape-time poll)         │
   │      internal/registry (Registry + exporter_build_info +      │
   │              │     exporter_collect_duration_seconds)          │
   │      internal/server  /metrics + /healthz (HTTP)              │
   └──────────────────────────┬────────────────────────────────────┘
                              │ pull scrape
                              ↓
              kube-prometheus-stack Prometheus → Grafana dashboards
              + backend prometheus datasource (/api/v1/metrics)
```

### 1.2 数据流

- **Scrape-time poll** (no background loop · stateless): Prometheus GETs
  `/metrics` → each `*Collector.Collect()` calls `source.ReadNPUs/ReadSlices`
  → emits `prometheus.MustNewConstMetric` per device/slice → records scrape
  cost into `exporter_collect_duration_seconds{collector=npu|slice|pid}` (histogram).
  Restart =
  full rebuild; all state lives upstream (silicon / simulator JSON).
- **Decoupling seam** (ADR-0024 §2 Decision G): `sources.Select()` is the
  single profile-aware decision point. The collector layer consumes the
  `Source` interface uniformly — **no `if real {}` branch**. demo/real
  difference = source selection here + chart `simulator.enabled`, nothing in
  the metric-emit code. This is why切真源 (T102) does not perturb demo.

### 1.3 Phase 演进

| Phase | Delivery |
|---|---|
| Phase 3 T006 | skeleton: registry / server / version / build_info（no collector） |
| Phase 3 T007 | `NPUCollector`（7 device descriptors） |
| Phase 3 T101 | `SliceCollector`（`ascend_slice_*`） |
| Phase 3 T102 | `WorkloadCollector`（PID 级 · `ascend_pid_*` · default off） |
| Phase 11 fix-002 | +temperature/power/vram_total/vram_used_percent descriptors（dashboard No-data 修复） |
| **Phase 13 T102** | **real DCMI/npu-smi body**（live telemetry · simulator off on real profile） |

---

## 2. 接口契约

### 2.1 Source 读侧接口（`internal/collector/sources`）

```go
type Source     interface { ReadNPUs(ctx)   ([]NPUSample, error) }
type SliceSource interface { ReadSlices(ctx) ([]SliceSample, error) }
```

`Select(SelectConfig) (Source, SliceSource, error)` maps `SourceType`:
- `simulator` / `mock` / `""` → `SimulatorSource`（实现 both · replay JSON）
- `real` → `DCMISource`（实现 Source；slice 侧返回 nil → real slice 序列暂
  仍 simulator-fed，真 slice 走 backend ResourceSlice 聚合 T103 · carry）

`NPUSample`: ID · NodeName · Model · AICore% · MemoryUsed/Total · HBMBandwidth ·
Temperature · Power · Healthy。`SliceSample`: ID · NPUID · Template · AICoreCount ·
MemoryUsed · AICoreUtilization · AllocatedTo{ns,pod}。

### 2.2 指标契约（label schema 一经 dashboard/backend 引用即契约 · 改走 RFC）

| 前缀 | 范围 | 例 |
|---|---|---|
| `exporter_*` | exporter 自身元指标 | `exporter_build_info` · `exporter_collect_duration_seconds` |
| `ascend_npu_*` | NPU 设备级（7 desc · T007+fix-002） | `ascend_npu_utilization_percent{npu_id,node,model}` · `_memory_used_bytes` · `_hbm_bandwidth_bytes_per_second` · `_temperature_celsius` · `_power_watts` · `_vram_total_bytes` · `_vram_used_percent` |
| `ascend_slice_*` | 切片级（T101） | `ascend_slice_aicore_count` · `ascend_slice_memory_used_bytes` · `ascend_slice_allocated_to_pod` |
| `ascend_pid_*` | 工作负载 PID 级（T102 · default off） | correlation labels `workload_id`/`pd_role`（ADR-0008 下游） |

### 2.3 real telemetry 读机制（P13-T-102 · ADR-0024 §4(c)）

**exec-over-cgo 红线**: real body 经 `os/exec` 读 `npu-smi` stdout 取
utilization%/memory/HBM-bw/temp/power — **不绑 `libdcmi.so`**。npu-smi 是同一
DCMI 库的 userspace face，数据一致，但留在 exec 让 default `CGO_ENABLED=0
GOARCH=arm64` 交叉编译（ADR-0020）**无需 build tag 即绿**。`DCMISource` 当前是
no-cgo wrapper，delegate → `NPUSMISource`。fallback 链（exporters/CLAUDE.md §5）:
DCMI → npu-smi → cgroup（PID 级）。

---

## 3. 生命周期

### 3.1 启动（cmd/exporter-plus/main.go）

parse flags（`--simulator <path>` / source type / `--enable-workload-
correlation` / addr）→ `sources.Select(cfg)` 构建 Source（empty type → simulator
default · 匹配 chart `simulator.enabled=true`）→ 注册 NPU/Slice/Workload
collector 到 `internal/registry` Registry → 起 `internal/server` HTTP
`/metrics`+`/healthz`。构建从不失败:缺 npu-smi binary 在首次 `ReadNPUs` 才以
`ErrNoCommand` 暴露（real 源懒失败，不阻塞启动）。

### 3.2 运行（scrape-driven · 无后台 goroutine）

每次 Prometheus scrape → Collector.Collect 同步 poll Source → emit。无定时器、
无缓存、无持久化（exporter 无状态约束 · exporters/CLAUDE.md §1）。

### 3.3 关闭

HTTP server graceful shutdown；无 informer / 无 client-go / 无外部连接需 drain。

---

## 4. 错误处理

- **Source 读失败**（npu-smi 缺失 / 解析失败 / 设备不可达）: collector `Collect`
  `log.Printf` + `return` — 该 scrape 不 emit 该 collector 的序列（degraded ≠
  crash · Prometheus 记 stale）。exporter 进程存活。
- **real 源懒构造**: `NewDCMISource` 永不失败；binary 缺失在首个 `ReadNPUs`
  以 `ErrNoCommand` 出现 → 该次 scrape degraded，下次 scrape 重试（自愈）。
- **simulator 缺 JSON**: `Select` 返回 `ErrSourceNotAvailable`（启动期硬失败 ·
  demo 必须有 snapshot）。
- **vram_used_percent 除零保护**: `MemoryTotalBytes==0` 时跳过该 derived 序列。

---

## 5. 扩展点

- **cgo libdcmi 直绑（reserved · `//go:build dcmi`）**: 高密度 host 上更低
  scrape 延迟的直接 DCMI binding。约定（ADR-0024 §4(c)）: 真体在 `dcmi` build
  tag 后，default 为 no-cgo stub，使 `cross-compile-arm64`（ci.yml）编 stub 永不
  断；`DCMIConfig` 届时加 `LibraryPath`/device-socket 字段，当前 `DCMIConfig(NPUSMIConfig)`
  直转会停止编译 → 强制 revisit seam。**Phase 13 §9 红线已由 no-cgo 规避**。
- **real slice 序列**: 现 real profile slice 侧仍 simulator-fed；真 slice 利用率
  走 backend ResourceSlice 聚合（T103）后可接 DRA 状态（carry · devlog）。
- **PID correlation（ADR-0008）**: `ascend_pid_*` + `workload_id`/`pd_role`
  labels 是 PD Router 下游 consumer；default off（`--enable-workload-
  correlation=false`）避免空载 `/proc` 扫描成本。
- **其他 exporter**: `exporters/` 预留节点级/网络/存储 exporter（Phase 4+ · 各自
  独立 Go module · exporters/CLAUDE.md §1）。

---

## 6. 集成示例

### 6.1 抓真 NPU 序列（real profile · lab）

```bash
# real profile: helm -f deploy/profiles/real/ascend-npu-exporter-plus.values.yaml
#   simulator.enabled=false · DCMI/npu-smi hostPath mount
curl -fsS http://<exporter-pod>:PORT/metrics | grep '^ascend_npu_'
#   → 真 utilization/HBM/temp/power 序列（非 simulator 正弦 · build-doc §4.4(3)）
```

### 6.2 demo profile（CI/演示常驻功能验证台）

```bash
# simulator.enabled=true → replay configs/.../sim.json · 任意节点可调度
make run    # build-local + ./bin/exporter-plus.local --simulator <snapshot>
```

### 6.3 下游消费

Prometheus（kube-prometheus-stack）scrape → Grafana `npu-detail`/`node-detail`/
`workload-resource` dashboards + backend prometheus datasource（`/api/v1/metrics`
聚合）。指标名/label = 跨模块契约（改走 RFC）。

---

## 7. 参考

- **ADR-0024**（`docs/adr/0024-real-hardware-activation.md`）§2 Decision D（真
  telemetry）· §2 Decision G（decoupling-seam invariant）· §4(c)（exec-over-cgo
  红线 + `//go:build dcmi` reserved variant）
- **ADR-0020**（aarch64 鲲鹏 target · multi-arch buildx）
- **ADR-0008**（PD Router）— PID 级 correlation 下游
- `exporters/CLAUDE.md` §1 无状态/pull 约束 · §3.4 指标前缀 · §5 DCMI→npu-smi
  fallback 链 · §8 PD Router 协作
- `docs/architecture.md` §1.3 Phase 路线图 · §2 监控层
- `docs/phase3-plan.md`（T006/T007/T101/T102）· `docs/phase13-plan.md` §4 P13-T-102
  + §9 CGO/arm64 红线
- 代码: `internal/collector/sources/{sources,simulator,dcmi,npu_smi,cgroup,realexec,
  npusmiparse}.go` · `internal/collector/{npu,slice,workload}.go` ·
  `internal/{registry,server,version}` · `cmd/exporter-plus/main.go`
- CLAUDE.md §14.2 module DESIGN.md 7-section convention

---

**END of ascend-npu-exporter-plus DESIGN.md**
