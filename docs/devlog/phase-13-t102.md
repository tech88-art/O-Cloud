# P13-T-102 · [B2] 真 NPU telemetry(exporter)

- **Commit**: (this commit · Phase 13 W2 Bucket B · main-agent verifies + commits)
- **Date**: 2026-06-03
- **Duration**: plan 2.5d vs actual ~1d(simulator/collector 已就位 · 写真源体 + 选择器 + 测)

## Intent

把 `ascend-npu-exporter-plus` 的 DCMI / npu-smi / cgroup 三个 Phase-3 stub(全返回 `ErrSourceNotAvailable`)换成真 telemetry 体,加 source selector(mock/simulator/real 分流),real profile `simulator.enabled=false`。lab-gating 翻转后(ADR-0024 §2 Decision A/D)真 telemetry 成为 Phase 13 deliverable —— 真 utilization% / memory / HBM bandwidth / temp / power(替换 simulator 正弦 fixture)+ per-container NPU 占用(真 /proc)。

## Path adaptations(P3 verify-before-claim · 读真实代码 + T101 sibling 再写)

- **不能复用 operators 的 npusmi 包**:exporter 是独立顶级 Go module(exporters/CLAUDE.md §3.1:无 client-go / 无跨模块 import)→ 在 exporter 内自带容错 `Key : Value` parser(`npusmiparse.go`),approach 镜像 T101 的 `npusmi/board.go`(splitKV / normKey / firstField / unit 后缀剥离 / alias 集),非 import。
- **exporter 数据模型比 T101 富**:`NPUSample` 要 utilization% / HBM bandwidth / temp / power(T101 的 `DeviceInfo` 只 inventory)→ real 读路径合并 **3 个 npu-smi 子视图**:`-t common`(temp/power/health/memory)+ `-t usages`(aicore util% / HBM used / HBM bw)+ `-t board`(chip name / capacity)· `info -l` 枚举设备。`mergeNPUUsage` 折叠多视图(pointer 区分"视图缺字段" vs "字段为 0")。
- **DCMI = npu-smi exec(无 cgo)**:plan §4 字面 "DCMI `libdcmi.so` binding" · 但 ADR-0024 §4(c) 红线优先 exec —— **选 exec/no-cgo**(见 Key decisions)· `DCMISource` 是 "DCMI-preferred" wrapper · 委托 npu-smi reader(同一无 cgo 机制 · exporters/CLAUDE.md §5 fallback 链)· 真 libdcmi cgo 是预留 `//go:build dcmi` 变体。
- **cgroup 真 /proc 体**:Phase-3 空 SimRoot 返回 `ErrSourceNotAvailable`(stub)→ 改真 reader:走 `ProcRoot`(默认 `/proc`)枚举 pid · 留持有 `/dev/davinci<N>` open fd 的进程(真 NPU 占用信号)· cgroup path 解 podUID + container + QoS namespace。SimRoot 非空 = demo fake-fs(原样保留)。
- **NPUSample 无 PCIE/HCCS/network 字段**:plan §4 提 "PCIE/HCCS/network 带宽 stamp" · 但 `NPUSample` 不建模 fabric(HCCS 拓扑是 backend T103 读 ResourceSlice attr 的域 · PCIE/network 是节点级)→ 不往 NPUSample 硬塞无 dashboard 消费的字段(守 "edit minimally")· HBM bandwidth 已建模(真值已接)· fabric 带宽 carry-forward 给 T103/节点 exporter。

## Key decisions

- **exec over cgo(ADR-0024 §4(c) 红线 · 镜像 T101)**:真读全程 `npu-smi` 经 os/exec(`realexec.go` wrapper:context timeout 5s + missing-binary→ErrNoCommand + 非零 exit + stderr 捕获)· **无 libdcmi cgo binding** → `CGO_ENABLED=0 GOARCH=arm64` 交叉编译绿 · **无 build-tag**(实测通)。这是 plan §9 第 5 条 "CGO/arm64 红线" 的 exec 兜底路径(优先级最高)。
- **decoupling seam = sources.go `Select` 工厂(ADR-0024 §2 Decision G)**:source 选择是唯一 profile-aware 决策点 · collector 层(npu/slice/workload.go)消费 `Source`/`SliceSource`/`CgroupSource` 接口**零改动 · 零 `if real {}` 分支**(collector 本就接口消费 · 守恒不破)。profile 差异 = `Select(SourceType)` + chart `simulator.enabled` · 非共享层条件。
- **测试缝注入 runner**:`newNPUSMISourceWithRunner(runner, ...)` / `newDCMISourceWithDelegate(...)` / cgroup 的 `readlinkFn` 包变量 —— captured-fixture(8card npu-smi `-l`/`-t common`/`-t usages`/`-t board` 文本)+ 合成 /proc 树驱动真体 · **无需 lab**(ADR-0024 §3 离线层 · 功能正确性在此层定)。
- **real 不返 SliceSource**:`Select(real)` 返 (Source, nil) —— exporter 真 slice 序列来自 DRA driver 的 ResourceSlice status(backend T103 聚合)· 非 exporter 设备读 · real slice 留 carry-forward(见下)· demo simulator 仍双返(实现 both)。
- **per-device 软容错(ADR-0024 §3)**:单设备某视图失败 → 该设备仍发(已解字段 + model 默认)· 仅 `info -l` 枚举失败才整体 error(mis-target 节点 loud-fail · 不伪造数据)。
- **chart-drift 显式不越界(P4 横向 + Allowed Paths)**:base chart `daemonset.yaml` 未 wire `/dev/davinci*` + npu-smi hostPath mount + `--node-name` fieldRef · 真源读路径已就绪但 DaemonSet 真挂载需 base chart 模板补丁(`deploy/helm-charts/**` = deploy 模块 owner · 非 exporter Allowed Paths)→ real profile overlay 用 `extraArgs: [--source=real]` 接线 + 注释记 drift · 不跨模块改 base chart。

## Verification(strict · per-task · 离线层全做 · 真机层 lab-gated)

- **build**:`go build ./...` OK · **vet**:`go vet ./...` OK(0 warning)
- **test**:`go test ./...` PASS 无回归 · sources pkg coverage **86.8%**。新增 fixture table-test:npu_smi(8card 全读 + 枚举 error + per-device 软容错 + unhealthy + no-device + parseNPUList + normaliseChipName)· dcmi(委托 + error 传播 + 真构造 ErrNoCommand)· sources `Select`(simulator 双返 / 缺 path / real 单返 *DCMISource / unknown)· cgroup 真 /proc(happy 2-container + non-kubepods skip + empty + missing ProcRoot)。
- **arm64 交叉编译**(ADR-0020 · 本 phase 最易踩):`CGO_ENABLED=0 GOARCH=arm64 GOOS=linux go build ./...` **OK**(无 cgo → 无 build-tag → cross-compile-arm64 CI 不会断)
- **lint**:golangci-lint `0 issues`(staticcheck S1016 DCMIConfig→NPUSMIConfig 已按建议转换 + 注释 future 分歧点)· 我的新/改文件 gofmt-clean(LF/CI view · 实测 `tr -d '\r' | gofmt -d` 空)
- **helm**(离线 dry-run):`helm lint` real profile `0 failed` · `helm template` real → argv 含 `--source=real`(simulator.enabled=false → 无 --simulator)· **demo profile 回归**:`helm template` demo → 仍 simulator 路径(argv 无 --source · 不破)= decoupling-seam 守恒实证
- **P4 横向扫**:`grep ErrSourceNotAvailable` → 仅剩 sources.go sentinel 定义 + Select 的 unknown-type/缺 path 返回(合理)· 三 stub 不再返回它(真体接管)
- **真机 verify**(plan §8 · lab-gated · 本 session 无 lab):lab `curl :9100/metrics | grep ascend_npu_` → 真 utilization/HBM/temp/power 序列(非 simulator 正弦)· 真 npu-smi `-t common`/`-t usages` 输出与 fixture 同形验证 = 用户 lab 完成或标 lab-driver-gated(ADR-0024 §3 单点 fallback)

## Carry-forward

- **base chart 真挂载 drift**(deploy 模块 · 非 T102 Allowed Paths · 已记 real values 注释):`deploy/helm-charts/ascend-npu-exporter-plus/templates/daemonset.yaml` 需补 (a) `/dev/davinci*` + `/usr/local/Ascend/driver/tools`(npu-smi)hostPath volumeMount · (b) DCMI socket mount · (c) `--node-name` env fieldRef($NODE_NAME)· (d) `--npu-smi-path` · (e) real-source 时 hostPID(workload-correlation 真 /proc 需)。由 T301 真机 runbook / 后续 deploy task / fix-NNN 落 · 真机部署前置。
- **real slice telemetry 缺口**:exporter real 路径不发 `ascend_slice_*`(real slice 来自 DRA ResourceSlice status · backend T103 聚合)· 若需 exporter 侧真 slice 序列 → 后续接 npu-smi `-t vnpu`(虚拟 NPU 切分视图)· 当前 real profile slice 面板靠 backend/T103 数据源。
- **PCIE/HCCS/network fabric 带宽**:NPUSample 未建模 · HCCS 拓扑 = backend T103(读 `npu.huawei.com/hccs_ring` attr)· PCIE/network = 节点级 exporter(未来)· 本 task 只接 HBM bandwidth(已建模字段)。
- **--node-name 空退化**:base chart 未 wire downward-API → real 默认 npu_id label `-npu-N`(空 node 前缀)· T301/base-chart 补 fieldRef 后变 `<node>-npu-N`(与 T101 realascend.deviceName / mockjson 同形 · 跨 profile 一致)。
- **lab parser 漂移**:真 npu-smi 输出 key 拼写若偏离 fixture → 扩 `npusmiparse.go` 的 alias 集(如 `aicore usage rate` 变体)+ 补 npu_smi_test case(ADR-0024 §3 narrow-parser 扩展点 · 同 T101 board parser 策略)。
