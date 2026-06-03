# P13-T-101 · [B1] real-Ascend Source 真体

- **Commit**: (this commit · Phase 13 W2 Bucket B)
- **Date**: 2026-06-03
- **Duration**: plan 2.5d vs actual ~1d(substrate 已就位 · 接线 + 测)

## Intent

把 Phase 7 W1 的 `realascend.RealAscendSource` stub(三 method 全返回 `ErrNotImplemented`)换成真体,走 `npusmi.ExecClient`(shell out 到 npu-smi)。lab-gating 翻转后(ADR-0024 §2 Decision A)real-ascend 成为 Phase 13 deliverable。List 枚举设备 + QueryTopology 解析 HCCS 环 + Watch 健康轮询。

## Path adaptations(P3 verify-before-claim · 读真实代码再写)

- **substrate 比 plan 预期更全**:`npusmi.ParseTopoMatrix` + `Client` 接口 + `FakeClient`/`ExecClient` scaffold 已 ship(P7-T-005)。`ExecClient.QueryTopo` 已完整实现(跑 `npu-smi info -t topo` → ParseTopoMatrix)· 只有 `QueryDeviceInfo`/`QueryHealth` 是 stub(返回 "parser deferred to T101")。→ T101 真正要写的:(1) realascend.go 三 method 接线 npusmi.Client (2) 实现 `QueryDeviceInfo`/`QueryHealth` 的 board/health parser。
- **List 的设备枚举源**:用 `QueryTopo`(topo matrix 给全设备 + Ring + NumaNode + Health)枚举 · `QueryDeviceInfo` 仅补 AI-core capacity。比 plan "npu-smi info 解析 device inventory" 更精确 —— topo 已是 inventory 枚举器。
- **device name 对齐**:realascend.deviceName 精确复刻 mockjson.deviceName(`node-npu-N`)· 保证 demo/real 同 (node,index) 产同名 → backend 拓扑聚合(T103)+ ResourceSlice diff profile 无关(ADR-0024 §2 Decision G)。
- **无 cgo**(ADR-0024 §4(c)):全程 npu-smi 经 os/exec · 无 libdcmi cgo binding → `CGO_ENABLED=0 GOARCH=arm64` 交叉编译绿 · 无需 build-tag 隔离(实测通)。

## Key decisions

- **测试用 FakeClient 注入**:加 `newWithClient(cfg, client, nodeID)` 测试缝 · `New(cfg)` 生产路径构造 ExecClient(NodeID 从 `$NODE_NAME` 环境变量兜底 · DaemonSet downward-API 约定)。captured-fixture(8card/16card)table-test 驱动真体 · 无需 lab npu-smi(ADR-0024 §3 离线层)。
- **Watch 降级语义**(ADR-0024 §2 Decision B):健康轮询 · QueryHealth error → 关 channel(degrade 到 publisher tick)· ctx cancel → 关 channel。两条 degrade 路径各有 test(healthErrClient / WatchClosesOnCancel)。
- **List 软容错**:单设备 QueryDeviceInfo 失败 → 用 fallback capacity(32)继续 · 不让单个 mid-reset NPU 清空整节点 ResourceSlice(QueryTopo 失败才整体 error · 无法枚举)。
- **board/health parser 容错**(ADR-0024 §3):`ParseBoardInfo`/`ParseHealth` 走 `Key : Value` 容错解析 + key alias 集 + unit 后缀剥离 · lab 输出变体扩 alias + 补 test。
- **SliceStrategy 恒 FixedTemplate**:真硬件发现恒发 whole-NPU(mirror mockjson 默认)· dynamic 切分是 NPUSliceTemplate opt-in 路径 · 非 npu-smi 发现。

## Verification(strict · per-task · 离线层全做 · 真机层 lab-gated)

- **build**:`go build ./...` OK · **vet**:`go vet ./internal/source/...` OK
- **test**:`go test ./internal/source/realascend/...` PASS(realascend 6 test:List 8card/16card · QueryTopology known/unknown · Watch cancel/degrade · List topo-error 传播 · New env 解析;npusmi board parser 4 test)· `go test ./...`(全模块)PASS 无回归
- **arm64 交叉编译**(ADR-0020):`CGO_ENABLED=0 GOARCH=arm64 GOOS=linux go build ./...` OK
- **lint**:golangci-lint `0 issues`(我的新/改文件 gofmt-clean)
- **P4 横向扫**:`grep ErrNotImplemented internal/source/realascend/` → 余项全是注释 + test 断言(断言真体**不**返回它)· 无真体返回 ErrNotImplemented
- **bug 修**:16card test 初始 numaOf 误设 i/4 · 读 fixture 实为 2-socket(8 NPU/socket → i/8)· 修断言(非代码 bug)
- **真机 verify**(§8 · lab-gated · 本 session 无 lab):lab 节点 `npu-smi info` 真跑 → ResourceSlice device 数/属性 = 真 910B · 真 `info -t topo`/`-t board`/`-t health` 输出与 fixture 同形验证 = 用户 lab 完成或标 lab-driver-gated

## Carry-forward

- **base chart 注释 drift**(deploy 模块 · 非 T101 Allowed Paths):`deploy/helm-charts/npu-dra-driver/values.yaml:103-104` 仍写 "real-ascend Phase 7 W1 stub returns ErrNotImplemented" —— 已 stale(真体 land)· 由 deploy 模块 task(T202/T203/T301)或 fix-NNN 刷新 · 本 task 守 Allowed Paths 不跨模块改 base chart(profile overlay 已更新)。
- T103(backend 真拓扑)消费 real-ascend publisher 产的 ResourceSlice attr(`npu.huawei.com/hccs_ring`+`numa_node`)· device name 约定一致 → T103 可对 expected schema 开发。
- lab 接线:ExecClient.BinaryPath 默认 PATH 查找 · lab 典型 `/usr/local/Ascend/driver/tools/npu-smi`(hostPath mount)· board/health parser 若遇真输出变体 → 扩 ParseBoardInfo/ParseHealth alias + 补 board_test case。
