# ADR-0024: 真硬件 lab 激活 + Bucket B 架构(supersede ADR-0011 §3 lab-gating default-defer · real-Ascend Source 真体 + 真 topology/telemetry/deploy/inference · demo/real decoupling-seam invariant)

- **状态**:Accepted(Phase 13 W1 · per Phase 13 plan P13-T-002 · 2026-06-03 · 承 ADR-0023 §2 Decision C lab-gating 翻转)
- **日期**:2026-06-03
- **决策者**:协调者(用户 · 2026-06-02 lab-ready 信号)
- **相关**:ADR-0023 §2 Decision C(lab-gating 翻转 · 本 ADR 是其架构展开)+ §2 Decision E(demo 验证台 · 本 ADR §2 Decision G decoupling-seam invariant 是其 enforcement)/ ADR-0011 §2(Source 接口契约 · 本 ADR §2 Decision A/B 填真体)+ §3(lab gating policy 连续 6 phase default-defer · 本 ADR supersede 该 default · status flip stamp)/ ADR-0016 §2 Decision B(lab posture re-eval trigger "ad-hoc lab access materialise" · 本 ADR 是该 trigger 兑现)/ ADR-0010 §7 forward note row 1(替换 Source.RealAscend.queryTopology · 走 npu-smi info -t topo 解析 HCCS · 本 ADR §2 Decision B 兑现)/ ADR-0009(npu-dra-driver · ResourceSlice attr 契约 · 本 ADR §2 Decision C 真拓扑数据源)/ ADR-0020(aarch64 鲲鹏 + openEuler · 本 ADR §2 Decision B/D CGO/arm64 build-tag 隔离前提)/ ADR-0008(PD Router webhook · 本 ADR §2 Decision F 真推理 endpoint)/ `docs/build-and-production-validation.md` §4.4(真机特有验证 5 项 = 本 ADR Bucket B 逐项 DoD · §3.1 真机前提)/ `docs/cann-driver-matrix.md`(driver/CANN 版本 pin · 本 ADR §4 entry-check)/ Phase 6 P6-T-003(NPUPool.status.hccsTopology 聚合 · 本 ADR §2 Decision C crd source 数据源)/ 代码:`operators/npu-dra-driver/internal/source/realascend/realascend.go`(stub)+ `.../npusmi/{client,exec,fake,parse}.go`(P7-T-005 scaffold · parse.go 已写+测)

---

## §1 Context

### §1.1 ADR-0011 §3 lab-gating 连续 6 phase default-defer

ADR-0011 §3 设 lab gating policy:`real-ascend` Source 真体(T101)+ 真硬件 smoke "当且仅当用户 entry meeting 明确 lab access available 时执行 · 缺省 = defer"。policy 连续命中:

| Phase | Outcome | Carry # |
|---|---|---|
| P7 P7-T-101(2026-05-20)| no signal → defer | 1st |
| P8 P8-T-105(2026-05-21)| no signal → defer | 2nd |
| P9 P9-T-106(2026-05-21)| no signal → defer | 3rd |
| P10 P10-T-102(2026-05-21)| no signal → defer | 4th |
| P11 P11-T-101(2026-05-22)| no signal → defer | 5th |
| P12 P12-T-101(2026-06-01)| Phase 12 = 构建 target 反转 ≠ 真硬件验证(ADR-0019 §2 Decision C)| 6th |

6 phase(P7-P12)synthetic ring fixture(`set-b-multi-ring`)cover CI + demo flow 主干 · 真硬件 stamp 始终是 *verification* 而非 *deliverable substrate*。ADR-0016 §2 Decision B 预设 re-eval trigger:(1) Phase N+ entry meeting re-eval(2) **ad-hoc lab access materialise**(3) M5+ milestone reset。

### §1.2 P7-T-004/T005 已 ship 真体 substrate(stub + parser 已写+测)

Phase 7 已 ship Source 接口抽象 + real-Ascend stub + npusmi 解析 scaffold(grep-verified · 本 task 决策时读):

- `realascend.RealAscendSource`(实现 `source.Source`):`List(ctx) ([]source.NodeDevices, error)` + `Watch(ctx) <-chan source.Event` + `QueryTopology(ctx, nodeName) (*source.HCCSTopology, error)` —— 现 stub:List/QueryTopology 返回 `source.ErrNotImplemented` · Watch 返回 closed channel · `Config{Mode string}` · `New(cfg) *RealAscendSource`
- `npusmi.Client` 接口(3 method):`QueryTopo(ctx) ([]TopoEntry, error)` + `QueryDeviceInfo(ctx, devID) (*DeviceInfo, error)` + `QueryHealth(ctx, devID) (HealthState, error)`
- `npusmi.FakeClient`(读 testdata fixture · 测试 + dev mode 用)+ `npusmi.ExecClient`(shell out `npu-smi` binary · 现 type-only scaffold)
- **`npusmi.ParseTopoMatrix(in) ([]TopoEntry, error)` 已完整实现 + parse_test.go 5 case 测**(8/16-card fixture + empty + unhealthy + malformed · connected-components → Ring 分配 · `# numa:`/`# health:` sidecar hint)
- 类型:`TopoEntry{NodeID,DeviceID,Ring,NumaNode,Health}` · `DeviceInfo{DeviceID,ChipName,AICores,MemorySizeMiB,NumaNode,Health,DriverVersion,FirmwareVersion}` · sentinel `ErrNoCommand`/`ErrParse`

**关键**:Bucket B 不是从零起 —— 接口 + stub + parser + fake client 已就位 · T101 = 把 stub body 换成 ExecClient 接线 + `List`/`QueryTopology`/`Watch` 真体。

### §1.3 demo/real profile 分离已 land(Source 接口缝是隔离边界)

Phase 12 closer `3765f5c` 落 demo/real 两交付版本架构分离(config + deploy profile · 不 fork 代码):`backend/configs/config.real.yaml` + `deploy/profiles/{demo,real}/*.values.yaml` + `install.sh --profile`。real 版当前边界(build-doc §0.1):`topology`/`deploy` 仍走 mock · `real-ascend` Source 返回 ErrNotImplemented · OIDC/Karmada/Vault/配额/SLA 未实现。**隔离活在 `Source` 接口缝** —— Bucket B 填的真体全在缝以下(Source 实现内)· 缝以上共享层(aggregator/handler/前端)零改动。

### §1.4 2026-06-02 lab-ready 信号 → trigger 兑现

用户 2026-06-02 chat lab-ready 信号 → ADR-0016 §2 Decision B trigger 2("ad-hoc lab access materialise")**FIRED** → 翻转 light-up(ADR-0023 §2 Decision C codify 事件 · 本 ADR codify 架构)。

---

## §2 Decision

### §2.1 Decision A:lab-gating policy 翻转(supersede ADR-0011 §3 default-defer · real-Ascend stub → 真体)

- **ADR-0011 §3 default-defer policy 翻转为 light-up**:`real-ascend` Source 从 stub(`ErrNotImplemented`)→ 真体 · 6-phase(P7-P12)default-defer 期结束 · Bucket B 不再是 carry · 是 Phase 13 deliverable
- substrate = P7-T-005 已写+测的 `npusmi` package(ParseTopoMatrix + Client 接口 + Fake/Exec impl)· T101 接线而非重写
- **policy core 不删**(P2 边界):ADR-0011 §3 lab gating *机制*(default sourceType=mock-json · real-ascend opt-in · CI 不依赖 lab)保留 —— demo 版仍走 mock-json(§2 Decision G)· 翻转的是 real 版 *default outcome*(defer → light-up)· 非删除 mock 路径

### §2.2 Decision B:real-Ascend body 架构(npu-smi inventory + topo + DCMI health)

T101 把 `RealAscendSource` 三 method 接 `npusmi.ExecClient`:

- **`List()`**:`ExecClient.QueryDeviceInfo(ctx, devID)` for all devices → 组装 `[]source.NodeDevices`(model/AICores/MemorySizeMiB/Health/Driver/Firmware)· device 数 = 真 910B(8/16 card)
- **`QueryTopology()`**:`ExecClient.QueryTopo(ctx)`(内走 `npu-smi info -t topo` → `ParseTopoMatrix`)→ `[]TopoEntry`(Ring + NumaNode)→ build `*source.HCCSTopology`(PeerGroups by Ring)
- **`Watch()`**:`ExecClient.QueryHealth(ctx, devID)` poll loop(cadence 可配)→ emit `source.Event`(Added/Modified/Deleted on health change)· 若 lab 驱动不支持 health poll → 退化 closed channel(non-blocking · 记 devlog)
- **`New()`**:加 config validation(npu-smi binary 存在 / hostPath mount 正确)· 失败返回 error 而非 ErrNotImplemented
- **ExecClient 硬化**(T101):`os/exec` wrapper 加 context timeout + 非零 exit 处理 + `ErrParse` retry-once(npu-smi 输出偶与 device hot-reset race)

### §2.3 Decision C:backend 真拓扑聚合数据源(ResourceSlice attr + NPUPool.status.hccsTopology)

backend `GetTopology()` 真体(T103)的真拓扑数据源:

- **k8s source**:list ResourceSlice(driver=`npu.ocloud.edge.example.com`)→ 读 device attr `npu.huawei.com/hccs_ring` + `numa_node`(real-Ascend publisher 经 §2 Decision B 真体产出)→ build ring map → `*model.Topology`(含真 HCCS/PCIE 边)
- **crd source**:读 `NPUPool.status.hccsTopology`(Phase 6 P6-T-003 聚合 primitive · pool-operator 调 Source.QueryTopology 填充)→ 聚合 by node
- **数据流**:real-Ascend Source(§2 Decision B)→ npu-dra-driver publisher 写 ResourceSlice attr / pool-operator 写 NPUPool.status → backend k8s/crd source 读 → aggregator emit 边 · **T103 软依赖 T101**(真 attr 由真 publisher 产)· 但 T103 可对 *expected schema* 先开发(attr key 已定)

### §2.4 Decision D:真 NPU telemetry(exporter DCMI/npu-smi 真读 · simulator off)

exporter `ascend-npu-exporter-plus` 真 telemetry(T102):

- `internal/collector/sources/{dcmi,npu_smi,cgroup}.go` 真读:DCMI `libdcmi.so` binding(utilization% / memory / HBM BW / temp / power)· npu-smi `info -i <id>` fallback · cgroup per-container NPU 占用
- `sources.go` source selector:mock / simulator / **real** 分流 · real profile `simulator.enabled=false`
- collector consume 真 source · PCIE/HCCS/network 带宽 stamp(替换 simulator 正弦 fixture)

### §2.5 Decision E:真 Deploy()(k8s source apply Deployment+Service)

backend `Deploy()/DeleteDeploy()` 真体(T104):

- `Deploy(ctx, *DeployRequest)`:via client-go apply Deployment + Service(+ 必要 RBAC)· label/owner-ref 约定 · 返回 `*DeployResponse`(deployID + status)
- `DeleteDeploy(ctx, deployID)`:删除 owned 资源(by label selector)
- backend ServiceAccount 需 create deploy/svc 权限(real profile RBAC · §2 Decision G 缝下:handler 不变 · 只 Source 实现切真)

### §2.6 Decision F:真推理(vllm-ascend/MindIE image + model mount + PD Router 真 endpoint)

inference-operator 真推理(T105):

- `deployment_builder.go`:Prefill/Decode Deployment 注真 vllm-ascend/MindIE image + model weights volume mount + CANN env(`ASCEND_RT_VISIBLE_DEVICES` / `huawei.com/Ascend910` resource request)· 替换 mock/placeholder image
- `pd_router.go`(mutating webhook):PD Router 注真 PD endpoint(prefill→decode KV cache 传输路径)· slice-binding 注解真 NPU(ADR-0008 契约不变)
- config/samples:Qwen 8B PD ModelService 真样例

### §2.7 Decision G:demo/real decoupling-seam invariant(build-doc §0.1 "不 fork 代码" 的 enforcement · 收口判据)

**这是 ADR-0023 §2 Decision E(demo = 长期验证台)的 enforcement 形式** —— 开发解耦的充要条件:

- **real 专属逻辑只在 `Source` 接口缝以下**(Source 实现内:realascend / k8s source / crd source / exporter real source / deployment_builder real image)
- **缝以上共享层 profile 无关**(aggregator / handler / 前端 / webhook 形状):只消费 Source 返回的形状(ring/fabric/device/DeployResponse)· 对 mock 夹具与真硬件走**同一段代码**
- **禁 `if real {}` 分支**:profile 差异由 *Source 实现选择*(factory / config mapping / chart values overlay)表达 · 非共享层条件分支 · grep 共享层无 profile 条件
- **保证 demo/real 相互验证/工作不影响**:demo profile(mock 源)回归不破是每 Bucket B task 离线层必含点 · real 版切真源 = 换 Source 实现 · 缝上零改动
- **clean demo/real 隔离 = 项目收口判据**(ADR-0023 §2 Decision B):真带宽/utilization 由 Source 提供 · 非共享层分支

---

## §3 真机集成风险 + 单点 fallback(P3 诚实 · P5 最弱环 · 不重开 phase)

**最弱环**(Phase 13 uncertainty profile high):synthetic fixture(set-a/b/c · parse_test.go)按*预期* npu-smi/DCMI 格式建模 · 真 910B 硅片的驱动版本 / CANN 兼容性 / npu-smi 输出变体可能偏离 fixture → 估算偏移风险集中 T101-T105。

**单点 fallback**(不重开 Phase 14):若某真硬件点在 lab 驱动/CANN 版本下 block(如某 DCMI 字段缺失 · npu-smi 输出变体 · health poll 不支持)→ **交付该 body + captured-fixture test 通 + 把那一个真机验证点标 lab-driver-gated**(记 devlog + checkpoint §残留)· 若 lab npu-smi 输出偏离 parser → 扩 ParseTopoMatrix + 补 test case(parser 已设计为 narrow + T101 多 section 扩展点)。项目仍按 "**软件完整 + best-effort 真机验证**" 收口 · 单点 surface 不 gating 收口。

**验证分工**(plan §8 · 2026-06-03 用户校准):**功能正确性由 demo 持续验**(captured-fixture table-test · CI 常驻)· **真机只验「对接」**(real Source 读真 npu-smi/DCMI/CANN + 输出与 Source 接口同形)· 真机不重验整条功能栈 → lab 依赖面收窄。

---

## §4 Open questions

### (a) lab K8s 版本 → Go v1 ResourceSlice migration 是否随 T101(entry-check)

- T002 entry 确认 lab 集群 K8s 版本:**若 lab 即 K8s ≥1.36**(`resource.k8s.io/v1` GA)→ Go v1 ResourceSlice schema migration(P10-fix-001 carry · 5 module v1beta1)可随 T101 一并迁(避免真机跑 v1beta1 shim)· **否则 shim 保留**(K8s 1.34+ v1beta1 shim 仍 work)· 不为此单开 phase(ADR-0023 §2 Decision F carry 触发条件)
- 当前倾向:lab 版本未确认前按 v1beta1 shim 开发(default · T101 captured-fixture 不依赖集群版本)· lab 确认 ≥1.36 → main-agent serial 起 migration mini-cohort(5 module lockstep · 非 gating)

### (b) Ascend driver / CANN 版本 pin

- T101/T102/T105 真机前置:Ascend driver + CANN 版本 pin(对照 `docs/cann-driver-matrix.md` baseline/floor)· DCMI socket/hostPath mount 路径 · `huawei.com/Ascend910B` 容量标签
- 版本偏离 → §3 单点 fallback(扩 parser / 标 lab-driver-gated)

### (c) CGO/arm64 交叉编译红线(build-tag 隔离 · 本 phase 最易踩)

- **风险**:真硬件 body 若引 cgo(DCMI `libdcmi.so` binding)→ 默认 `CGO_ENABLED=0 GOARCH=arm64` 交叉编译会断(ADR-0020 cross-compile-arm64 CI job)
- **决策**:**必须 build-tag 隔离** —— `//go:build dcmi` 真体(cgo libdcmi)vs 默认 stub-tag · `cross-compile-arm64` CI job 编 stub-tag(`CGO_ENABLED=0`)· 真体在 lab 节点本地编(`CGO_ENABLED=1` on arm64 鲲鹏)· **T101/T102 spec 必含此隔离防 arm64 CI 断**
- 替代:若 lab DCMI 可走 exec(`npu-smi`/`dcmi` CLI shell-out · 非 cgo)→ 无 cgo · 无需 build-tag(优先 · ExecClient 已是 exec 路径)· cgo 仅当 DCMI 必须走 `.so` binding 时

---

## §5 引用

### 上游(本 ADR 决策依据)

- ADR-0023 §2 Decision C(lab-gating 翻转 · 本 ADR 架构展开)+ §2 Decision E(demo 验证台 · §2 Decision G enforcement)
- ADR-0011 §2(Source 接口契约 · §2 Decision A/B 填真体)+ §3(lab gating policy 6-phase default-defer · 本 ADR supersede default · status flip)
- ADR-0016 §2 Decision B(re-eval trigger "ad-hoc lab access materialise" · 本 ADR 兑现)
- ADR-0010 §7 forward note row 1(Source.RealAscend.queryTopology → npu-smi info -t topo · §2 Decision B 兑现)
- ADR-0009(ResourceSlice attr 契约 · §2 Decision C 数据源)· ADR-0008(PD Router · §2 Decision F)· ADR-0020(aarch64 · §2 Decision B/D + §4(c)CGO/arm64)
- `docs/build-and-production-validation.md` §4.4(真机验证 5 项 = Bucket B DoD)+ §3.1(真机前提)· `docs/cann-driver-matrix.md`(§4(b)版本 pin)
- Phase 6 P6-T-003(NPUPool.status.hccsTopology · §2 Decision C crd 数据源)
- 代码 grep-verified:`realascend.go`(stub)+ `npusmi/{client,exec,fake,parse}.go`(parse.go 已写+测 · §1.2)

### 下游(本 ADR 触发 Bucket B execution)

- P13-T-101 real-Ascend Source 真体(§2 Decision A/B)· P13-T-102 真 telemetry(§2 Decision D)· P13-T-103 真拓扑聚合(§2 Decision C)· P13-T-104 真 Deploy()(§2 Decision E)· P13-T-105 真推理(§2 Decision F)
- P13-T-301 真机端到端 E2E(build-doc §4.4 5 项真机对接 stamp · §3 fallback posture)
- 全 Bucket B task 守 §2 Decision G decoupling-seam invariant(demo profile 回归不破)

### 上游 commit chain(决策时 grep-verified)

- Phase 12 tag `phase-12-complete`(`f0ce285`)+ demo/real profile 分离 `3765f5c`
- Phase 7 P7-T-004/T005(Source 接口 + realascend stub + npusmi parser scaffold · parse.go + parse_test.go)
- 本 ADR 是 Phase 13 plan §3 P13-T-002 deliverable · 承 ADR-0023 §2 Decision C

---

**END of ADR-0024**
