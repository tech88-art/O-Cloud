# P7-T-005 · npu-smi / DCMI Go binding scaffold (internal/source/realascend/npusmi)

- **Commit**: (this commit)
- **Date**: 2026-05-20
- **Duration**: plan 1d / actual ~1d (main agent direct · scaffold + parser + 5 tests + 2 testdata fixtures + DESIGN.md cross-ref)

## Intent

Per ADR-0011 §2 + phase7-plan.md §3 P7-T-005,落地 `internal/source/realascend/npusmi/` 子包,提供:Client interface 3 方法(QueryTopo / QueryDeviceInfo / QueryHealth)+ FakeClient(testdata fixture-driven · tests + T101 dev mode)+ ExecClient(shells out to npu-smi · scaffold only · T101 lab body 时硬化)+ parse.go(topo matrix parser · connected-components on HCCS edges)+ 5 unit test cases。**Phase 7 W1 CI 0 dep on real npu-smi binary** — 全部走 FakeClient + 内嵌 testdata。

## Path adaptations

- 计划 §3-T005 列了 7 个 Allowed Paths(client.go / fake.go / exec.go / parse.go / parse_test.go / testdata/npu-smi-topo-fixture-8card.txt / npu-smi-topo-fixture-16card.txt + DESIGN.md extend)— 全部落地按 plan。
- 计划 acceptance "5 parser cases pass" — 实际落地 5 个 named test functions(TestParseEmptyInput / TestParse8CardFixture / TestParse16CardFixture / TestParseUnhealthyHint / TestParseMalformedRow)其中 TestParseEmptyInput 含 3 个 sub-cases(empty string / whitespace / comments-only),累计 7 个 PASS。
- 计划 "TopoEntry{NodeID, DeviceID, Ring, NumaNode, Health}" — 落地匹配。NumaNode + Health 在 Phase 7 W1 默认 0 / HealthUnknown(parser 处理 topo matrix 不提供 NUMA / Health);**sidecar comments**(test-only convention · `# numa: NPU0=0 NPU4=1` / `# health: NPU0=Healthy`)让 fixture 测试 populate · 实际 npu-smi 不会 emit 这种 sidecar · T101 lab body 通过 multi-query(`-t board`)合成。

## Debugging trail

- 无 false start。parser 单次写成 · 5 test 单次通过。
- 第一次 `go test`:7 个 PASS · clean。
- 唯一犹豫点:NumaNode 怎么从 npu-smi 拿。`npu-smi info -t topo` 不报 NUMA;靠 `-t board -i <id>` per-device 查。Phase 7 W1 parser 只解 topo matrix · NumaNode 默认 0 + sidecar 测试 hint。T101 lab body 多 query 合成。
- 第二个犹豫点:8-card 910B 实际 topology 是不是 ring-of-4(NPU0-3 + NPU4-7)。多数 Ascend 910B server 文档确认这是标准两 ring 设计(每 ring 4 卡 HCCS 互联 · 跨 ring PIX 链路 = PCIe switch)· 与 ADR-0010 §256 expected shape `0↔1↔2↔3↔0` 一致(单 ring 4 卡 fully-connected)。16-card 910B-pro 是 synthetic extrapolation(我们没真硬件抽样 · 4 ring of 4)— ADR-0011 §4 + DESIGN.md 已明示 synthetic。

## Key decisions

- **Sidecar comment convention 用于 testdata fixtures** — 非真实 npu-smi 输出 · 仅 test fixture 使用。parser 设计为"sidecar 可选 · 缺省时 NumaNode=0 + Health=Unknown" · 现实数据流时 sidecar 行不存在 · parser 行为不变。Trade-off:测试 fixture 看起来不完全像真 npu-smi 输出 · 但避免了 multi-query 合成在 W1 阶段引入的复杂度。T101 lab body 替换 FakeClient → ExecClient 时 ExecClient 自己负责 multi-query 合成(独立 from parser)。
- **Connected-components BFS for Ring assignment**:简单 + 确定性 · 从 NPU0 开始 BFS first-reached component = Ring 0;next = Ring 1;etc。这保证了 ring ID 分配 deterministic across runs(避免 hash-table iteration order 影响测试)。
- **Embed testdata via `//go:embed`** instead of file-read — 测试不依赖 cwd;binary self-contained。Go 1.16+ 标准 feature。
- **ExecClient scaffold ships compile-ready but bodies deferred to T101**:Phase 7 W1 ship the os/exec call shape · 但 QueryDeviceInfo / QueryHealth body 明确返回 "parser deferred to Phase 7 T101 lab body" error。这是诚实承认 W1 范围 · 而不是假装 lab-ready。
- **`ErrNoCommand` sentinel** for missing npu-smi binary — exec.LookPath 失败时返回该 sentinel · 让 Phase 7 T101 lab body 区分 "binary missing"(fail-fast · 不要 retry)vs "binary returned error"(可能 retry)。
- **`ErrParse` sentinel** for malformed input — caller 可 retry once before failing(T101 lab body 应用此模式 · npu-smi 输出偶尔 race with device hot-reset)。

## Verification

- 存在性:
  - `internal/source/realascend/npusmi/client.go` — interface + 3 types + 2 sentinels ✅
  - `internal/source/realascend/npusmi/fake.go` — FakeClient impl + embed.FS ✅
  - `internal/source/realascend/npusmi/exec.go` — ExecClient scaffold ✅
  - `internal/source/realascend/npusmi/parse.go` — ParseTopoMatrix + helpers ✅
  - `internal/source/realascend/npusmi/parse_test.go` — 5 test functions / 7 sub-cases ✅
  - `internal/source/realascend/npusmi/testdata/npu-smi-topo-fixture-8card.txt` — 8-card with sidecar ✅
  - `internal/source/realascend/npusmi/testdata/npu-smi-topo-fixture-16card.txt` — 16-card synthetic with sidecar ✅
  - `DESIGN.md` §3.7 npu-smi parser contract — 新章节 with package layout + matrix format + 5 test cases table ✅
- 完整性(verified by execution):
  - `go build ./...` clean ✅
  - `go test ./internal/source/realascend/npusmi/... -v` — 7 sub-cases PASS · 1.130s ✅
  - `go test ./...` 整个 npu-dra-driver module 全 PASS(api / cmd no-tests / allocator / controller / publisher / source no-tests / source/mockjson / source/realascend / source/realascend/npusmi)
  - FakeClient 通过 compile-time assertion `var _ Client = &FakeClient{}` ✅
  - ExecClient 通过同样的 compile-time assertion ✅
- 正确性:
  - 8-card fixture parser output: NPU0-3 Ring=0 · NPU4-7 Ring=1 · NumaNode/Health 从 sidecar 正确读取 ✅
  - 16-card fixture parser output: 4 rings of 4 each · NPU0-3=0 · NPU4-7=1 · NPU8-11=2 · NPU12-15=3 ✅
  - empty/whitespace/comments-only → ErrParse ✅
  - 截断 row → ErrParse with descriptive error ✅
- 注释:**0 dependency on real npu-smi binary** — Phase 7 CI runners 不需要安装 Ascend driver。ExecClient 的方法测试时若运行 `exec.LookPath` 会 return ErrNoCommand(npu-smi 不在 PATH)· 这是预期的 dev/CI 环境行为。

## Carry-forward

- **T101 lab-conditional** — `realascend.RealAscendSource` 当前 stub(P7-T-004)替换为 wire-through 到 npusmi.Client:`List` → `client.QueryTopo(ctx)` map to source.NodeDevices · `Watch` → `client.QueryHealth(devID)` poll loop · `QueryTopology` → 已经基于 QueryTopo 结果 build。Switch FakeClient → ExecClient by `Config.Mode` value(W1 已留 hook)。
- **T101 必须 harden ExecClient.QueryDeviceInfo + QueryHealth bodies** — 解析 `npu-smi info -t board -i <id>` 输出(chip name / AI cores / memory / driver version / firmware version)+ `dcmi_get_device_health` C library 调用(via cgo · Phase 7 T101 决定是否 cgo vs 命令行 npu-smi --health)。
- **T101 also wires hostPath mount for npu-smi binary** — helm chart `templates/deployment.yaml` 需在 sourceType=real-ascend 时挂载 `/usr/local/Ascend/driver/tools/` 进 container(per Ascend docs)+ container 跑 `privileged: true`(DCMI 要求)。该 chart 修改是 T101 Allowed Paths · 不在 T005 范围。
- **Phase 10 lab smoke verification** — 在真机上跑 ExecClient · 对比 FakeClient 8card fixture 输出 · 校验 parser 假设(HCCS legend / matrix layout / column alignment)与真实 npu-smi 输出一致。如有 drift → parser.go 修补 · 不动 testdata(testdata 是 Phase 7 W1 baseline)。
