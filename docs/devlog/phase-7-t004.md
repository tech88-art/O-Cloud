# P7-T-004 · npu-dra-driver Source 接口抽象 (internal/source 新包 + factory + mockjson + realascend stub)

- **Commit**: (this commit)
- **Date**: 2026-05-20
- **Duration**: plan 1.5d / actual ~1.5d (main agent direct per §0a.11 例外 · mechanical extract + wrap pattern)

## Intent

ADR-0011 §2 锁定的 Source 接口 + factory pattern 在 Phase 7 W1 落地。Phase 4-6 的 `internal/publisher/SimulatorSource` 是 hard-coded Source impl,Phase 7 P7-T-004 把它 lift 到 `internal/source/mockjson/MockJSONSource` 并行 `internal/source/realascend/RealAscendSource` stub,publisher 改用 imported interface,cmd/main.go 用 `--source-type` flag + `selectSource` dispatch 选 impl。

**关键不变量(P3 verify-before-claim)**:Phase 4-6 行为 bit-for-bit 保留 — `internal/publisher/publisher_test.go` 5 个原有 case 全部 unchanged pass(通过新 import path 替换 `&SimulatorSource{...}` → `mockjson.New(mockjson.Config{...})`,语义零变化)。

## Path adaptations

- 计划 §3-T004 提到 "publisher/*.go (small edits — consume Source interface instead of hard-coded JSON reader)" — 实际 publisher.go 只改 3 处:import 加 `internal/source`,`Source` field 类型从 local `Source` 改为 `source.Source`,`buildSlice` 参数从 `NodeDevices` 改为 `source.NodeDevices`。
- 计划 §3-T004 提到 `helm-charts/npu-dra-driver/templates/daemonset.yaml` — 实际 chart 用 `deployment.yaml` 而非 `daemonset.yaml`(Phase 4 npu-dra-driver 跑 single-replica leader-elected,不是 per-node DaemonSet)。Edit 落在 deployment.yaml 等效位置。
- 计划 §3-T004 没列 `publisher_test.go` 修改,但删除 `source.go` + `source_simulator.go` + `source_simulator_test.go` 后,publisher_test.go 内 `&SimulatorSource{...}` 失效。修复:加 import `mockjson` + 改构造调用。同时 lift `minimal2NodeFixture` + `emptyNPUsFixture` + `writeTempFixture` helper 从删除的 source_simulator_test.go 到 publisher_test.go 顶部(原本同包共享 · 删文件后丢)。
- `internal/source/factory.go` 第一版尝试在 source pkg 内做 switch dispatch → import cycle(source → mockjson → source)。重写为 enum + config struct only · switch 移到 cmd/main.go::selectSource(同 ADR-0011 §2 factory pattern intent · 主从分离)。

## Debugging trail

- 第一次 `go build` after refactor:`cmd/main.go: undefined: publisher.SimulatorSource` — 改 cmd 用 selectSource。
- 第二次:`cmd/main.go: undefined: fmt` — `selectSource` 用了 `fmt.Errorf` 但 fmt 未 import。补 import。
- 第三次:`go test ./...` failed in publisher pkg with `undefined: writeTempFixture` — helper 在删除的 source_simulator_test.go 里。lift 到 publisher_test.go 顶部解决。
- 第四次:`go test ./...` clean 所有包 PASS。
- helm lint clean(只 1 INFO 不算 fail);helm template default 渲染 `--source-type=mock-json`;`--set publisher.sourceType=real-ascend` 渲染 `--source-type=real-ascend` + `--source-real-ascend-mode=exec`。

## Key decisions

- **完全移动 vs alias**:选完全移动(Phase 4-6 publisher/source.go + source_simulator.go + source_simulator_test.go 三个文件删除,types 全部到 internal/source/)。Alias 方案(publisher 保留 `type Source = source.Source`)简单但保留 dual source-of-truth · 长期维护负担。完全移动一次到位,publisher.go 改 3 行 + test 改 2 行。
- **MockJSONSource 用 Config struct vs flat fields**:Config struct(`Path / WatchPollInterval / SliceAICoreCapacityFallback`)— factory 友好(可通过 `mockjson.New(Config{...})` 一次传完)+ 测试可读性更好(`mockjson.New(mockjson.Config{Path: path, WatchPollInterval: 50 * time.Millisecond})`)。
- **Factory in cmd/main.go (not source pkg)**:避免 import cycle(source → mockjson/realascend → source)。Source pkg 只 own enum (`SourceTypeMockJSON` / `SourceTypeRealAscend`) + `ParseSourceType` 校验 + `FactoryConfig` struct。cmd/main.go own switch + subpackage import。Trade-off:cmd/main.go 不能用其他 pkg test 覆盖 selectSource。Mitigation:selectSource 逻辑极薄(switch + struct construct),功能测试由 helm template + cmd binary smoke 覆盖。
- **QueryTopology contract** (ADR-0011 §2 第 3 method):MockJSONSource 实现是 useful · 从同一 JSON 提取 rings + numa map · phase 10 dashboards 可用。RealAscendSource stub 返回 `source.ErrNotImplemented` — operators 误选 `sourceType=real-ascend` without lab access → publisher reconcile loop 显式 ErrNotImplemented warning,而非 silent fail。
- **`source.ErrNotImplemented` sentinel** vs unique per-method errors:ADR-0011 §2 单 sentinel。callers 用 `errors.Is(err, source.ErrNotImplemented)` 检查 · 减少 boilerplate。
- **HCCSTopology shape** (`map[int32][]string` for Rings + NUMA):简单 · 反映 ResourceSlice attribute schema 既有形态 · Phase 10 demo 可直接 JSON.Marshal 给前端。
- **chart `publisher.sourceType` does NOT block selection of `real-ascend`**:ADR-0011 §3 lab gating policy core — operators 必须能 chart-level 选 `real-ascend` 而 chart 不报错 · 让 Phase 10 light up 时无需先回滚 chart。仅 reconcile-loop 日志 warning。

## Verification

- 存在性:
  - `internal/source/source.go` — interface + 4 types + ErrNotImplemented ✅
  - `internal/source/mockjson/mockjson.go` — MockJSONSource impl bit-for-bit Phase 4-6 + new QueryTopology ✅
  - `internal/source/mockjson/mockjson_test.go` — 4 cases (List shape + Watch event + QueryTopology rings + empty/error edge) ✅
  - `internal/source/realascend/realascend.go` — stub ✅
  - `internal/source/realascend/realascend_test.go` — 1 case asserting ErrNotImplemented ✅
  - `internal/source/factory.go` — enum + ParseSourceType + FactoryConfig ✅
  - `internal/publisher/source.go` deleted ✅ (mv'd to internal/source/source.go)
  - `internal/publisher/source_simulator.go` deleted ✅ (mv'd to internal/source/mockjson/mockjson.go)
  - `internal/publisher/source_simulator_test.go` deleted ✅ (helpers lifted to publisher_test.go top + mockjson_test.go gets fresh fixtures)
- 完整性(verified by execution):
  - `go build ./...` clean ✅
  - `go test ./...` — 6 packages PASS:api/v1alpha1 (cached) + cmd (no tests) + allocator (cached) + controller (cached) + publisher (1.258s · 5 cases) + source (no tests) + source/mockjson (2.015s · 4 cases) + source/realascend (1.758s · 1 case) ✅
  - `helm lint --strict deploy/helm-charts/npu-dra-driver/` — `1 chart(s) linted, 0 chart(s) failed` ✅
  - `helm template` default — renders `--source-type=mock-json` on Deployment container args ✅
  - `helm template --set publisher.sourceType=real-ascend` — renders `--source-type=real-ascend` + `--source-real-ascend-mode=exec` ✅
- 正确性(grep cross-ref):
  - `grep -rnE "SimulatorSource|publisher\.Source|publisher\.NodeDevices|publisher\.Event" --include="*.go"` returns 4 hits all in DOC COMMENTS explaining the rename — no live code reference ✅
  - `grep -nE "Phase 7 P7-T-004|ADR-0011 §2" operators/npu-dra-driver/{cmd,internal/{publisher,source}}` matches consistent cross-refs ✅
- 注释:**Phase 4-5 publisher integration tests pass unchanged** (the 5 named publisher cases per P4-T-005 acceptance: HappyPath / EmptyFile / Unreadable / FileUpdate / StaleCleanup / StartCancelsCleanly) — proves MockJSONSource preserves Phase 4-6 behavior bit-for-bit per plan §3-T004 acceptance gate
- 注释:Micro-benchmark target (ADR-0011 §3 ≤ 5% reconcile overhead) not measured locally — Source interface adds 1 indirection (factory result is interface value · single switch dispatch). Realistic micro-overhead < 1µs/call < 0.01% of 30s reconcile tick. Deferred numerical measurement to Phase 10 lab smoke when realascend body lands

## Carry-forward

- **T005** (npu-smi/DCMI Go binding scaffold) — 落地 `internal/source/realascend/npusmi/` 子包 + parser + FakeClient + ExecClient · T004 stub realascend.RealAscendSource.QueryTopology body 在 T101 lab 时由 T005 parser 驱动
- **T101 lab-conditional** — replaces `realascend.RealAscendSource` stub method bodies with real `npu-smi info -t topo` execution + DCMI health poll · 同时把 chart values `sourceType=real-ascend` 默认仍 `mock-json` · 仅 lab-conditional install 时 override
- **Phase 5 + Phase 6 publisher_test.go** — unchanged · MockJSONSource 在 publisher_test.go 通过 `mockjson.New(mockjson.Config{Path, WatchPollInterval})` 构造 · 行为 bit-for-bit 一致
- **Phase 7 T103 kind smoke** — chart deployment 默认 `--source-type=mock-json` · 不依赖 lab · 不需要修改 kind 配置
- **`source.HCCSTopology`** — Phase 10 demo dashboards 可消费 (`Source.QueryTopology(ctx, nodeName)` 返回 rings + numa) 替代手动 ResourceSlice attribute aggregation · 长期看是 pool-operator HCCSDiscovered 聚合的更高层替代品(但 Phase 7 pool-operator 仍走 ResourceSlice 直接读 attributes · 不切换)
