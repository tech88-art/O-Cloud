# ADR-0021: 拓扑数据全保真扩展(NPU PCIE 带宽 + network/hccs/workload→node 边 + edge 带宽属性)

- **状态**:Accepted(Phase 12 entry · per ADR-0019 §2 Decision A Track B · Phase 12 plan P12-T-003 · 2026-06-01 · §0a.5 chat+ADR self-RFC · 用户决策 2 拓扑全保真 = chat 批准 · 本 ADR + api-contract.yaml = ADR-side codification · 共享契约 main-agent serial)
- **日期**:2026-06-01
- **决策者**:协调者(用户 · 2026-06-01 chat entry meeting 决策 2 拓扑数据全保真)
- **相关**:ADR-0019 §2 Decision A Track B(本 ADR 是 Track B contract codification)/ **ADR-0004 inter-node fabric topology**(NetworkLink BandwidthGbps/Medium/Utilization/RTT 已有 · 仅 fabric-link 用 · 本 ADR network/fabric 边带宽属性复用源)/ **ADR-0005 pod-in-topology fusion**(workload/pod 节点 + binds-to/pd-pair 边 · 本 ADR workload→node runs-on 边补充点)/ ADR-0006 wire-type enum regen(TopologyNode/Edge 联合类型 native · hccs/network enum 此 regen 已存在但 aggregator 不 emit · 本 ADR 补 emit)/ `docs/api-contract.yaml`(本 ADR codify 的契约 diff 落点)/ `docs/phase12-plan.md` §1 Track B + §4 P12-T-104/T105

---

## §1 Context

### §1.1 用户 2026-06-01 拓扑数据全保真决策

用户 entry meeting 决策 2(拓扑数据全保真):当前拓扑数据是**结构保真但属性贫瘠** —— 节点/边的拓扑结构在,但 NPU 的 host 互联带宽、节点间互通带宽、节点内 NPU 互联(HCCS)带宽、非 NPU 负载落在哪个节点,这些**真实部署里运维关心的属性**缺失。要求补全使拓扑贴近真实数据中心视图(供前端 one-page workspace 的 edge hover + focus 过滤渲染)。

### §1.2 当前 aggregator emit 现状(grep-verified)

`docs/api-contract.yaml` TopologyEdge.type enum(ADR-0006 regen)已含 8 值:`contains / hccs / network / allocated / fabric-link / binds-to / pd-pair`。但 **aggregator 实际只 emit `contains / fabric-link / binds-to / pd-pair`**:
- `hccs` 枚举存在 · 但 aggregator 不 emit npu↔npu intra-node HCCS 边
- `network` 枚举存在 · 但 aggregator 不 emit node↔node inter-node 互通边(只有 node↔switch 的 fabric-link)
- **NPU host 互联 PCIe 带宽**:契约无字段(NPU schema 有 hccsGroup / vramMiB / aiCoreTotal · 无 pcieBandwidthGBps)
- **非 NPU workload→node 边**:完全缺失(ADR-0005 只有 NPU workload 的 binds-to pod↔slice · 非 NPU 负载无 node 落点边)
- **edge 带宽属性**:`TopologyEdge.attributes` 是 free-form `additionalProperties: true` · 无文档化的 bandwidthGBps/medium/utilization · 仅 fabric-link 内部用 NetworkLink 的 BandwidthGbps

### §1.3 本 ADR 范围 = 契约扩展(T003)· emit + fixtures 是下游

本 ADR + api-contract.yaml 是**契约 codification**(字段 + 边语义 + 属性 doc)。实际 aggregator emit 逻辑(T104)+ mock fixtures(T105)是下游消费本契约。本 ADR 是 Phase 12 **唯一 api-contract RFC**(共享契约 · main-agent serial per root CLAUDE.md §11)。

---

## §2 Decision

### §2.1 Decision A:NPU 加 `pcieBandwidthGBps` 字段(host↔NPU PCIe 带宽)

- `NPU` schema 加 `pcieBandwidthGBps`(number/float/nullable · GB/s · gigabytes/second)
- 语义:host CPU ↔ NPU 的 PCIe 链路带宽。PCIe Gen4 x16 ≈ 32 GB/s · Gen5 x16 ≈ 64 GB/s
- 同步:`TopologyNode.attributes` npu 行文档加 `pcieBandwidthGBps`(npu 节点 attributes free-form · 供拓扑 contains 边 / NPU 节点 hover)
- 渲染:前端在 NPU 节点 hover 或 host↔NPU contains 边上显 PCIe 带宽

### §2.2 Decision B:emit `network` 边(node↔node inter-node 互通)

- enum `network` 已存在(ADR-0006)· 本 ADR codify 其语义 = **node↔node 节点间互通**(区别于 fabric-link 的 node↔switch)
- attributes:`bandwidthGBps`(GB/s)+ `medium`(eth/roce/ib)+ `utilization`(0-100)
- 源:ADR-0004 NetworkLink(node-to-node 项)或 mock 新字段(T105)· aggregator emit(T104 · depth=npu+ 或独立 · IncludeWorkloads 无关 — 硬件拓扑)

### §2.3 Decision C:emit `hccs` 边(npu↔npu intra-node)

- enum `hccs` 已存在(ADR-0006)· 本 ADR codify 其语义 = **同 node 内 npu↔npu HCCS 高速互联**(同 `hccsGroup` 的 NPU 之间)
- attributes:`bandwidthGBps`(GB/s · HCCS ≈ 56 GB/s 量级)+ `utilization`(0-100 · 可选)
- 源:同 node 内 NPU 的 hccsGroup 聚合(T104 aggregator · IncludeWorkloads 无关 — 节点内硬件拓扑)

### §2.4 Decision D:emit workload→node 边 = 新 `runs-on` type(不复用 binds-to)

**workload→node edge type 命名决策**:**新增 `runs-on`**(不复用 `binds-to`):
- `binds-to`(ADR-0005)语义 = **pod ↔ NPU slice 资源绑定**(NPU workload · 占用切片)
- `runs-on`(本 ADR)语义 = **非 NPU workload/pod ↔ node compute placement**(无 slice binding · 仅落在某节点)
- **为什么不复用 binds-to**:两者语义不同(资源绑定 vs 计算落点)· 复用会让前端无法区分 "占了 NPU 切片的负载" 与 "只是跑在节点上的普通负载" · 渲染样式 + focus 过滤都需区分 · 单独 `runs-on` type 保持语义清晰(P4 单一语义 · 不 conflate)
- emit 条件:`IncludeWorkloads=true` 且 pod 无 slice binding 时(T104 · 与现有 binds-to/pd-pair workload 分支独立)

### §2.5 Decision E:fabric-link/network/hccs 边统一带宽属性供前端 hover

- `fabric-link`(ADR-0004 · node↔switch)· `network`(node↔node)· `hccs`(npu↔npu)三类硬件拓扑边统一带 `bandwidthGBps`(+ `medium`/`utilization` 适用时)· 供前端 edge hover tooltip 同轴比较
- **单位统一 GB/s**(`bandwidthGBps` · gigabytes)· 与 `NPU.pcieBandwidthGBps` 同尺度 → hover 中 PCIe(~32)/ HCCS(~56)/ network(~50 = 400 Gbps NIC)可同轴比较(见 §4 单位 note)
- `TopologyEdge.attributes` 从纯 free-form 升级为 **documented free-form**(description 列各边类型属性 + additionalProperties: true · 与 TopologyNode.attributes 同 pattern · 不引入 named schema 保持 gen:types 输出最小)

---

## §3 契约 diff 清单(docs/api-contract.yaml · gen:types regen)

| 位置 | 改动 |
|---|---|
| `NPU` schema | 加 `pcieBandwidthGBps`(number/float/nullable/example 32.0)· 在 hccsGroup 后 |
| `TopologyNode.attributes` description | npu 行加 `pcieBandwidthGBps (ADR-0021)` |
| `TopologyEdge.type` enum | 加 `runs-on`(§2 Decision D)· hccs/network/fabric-link 注释补 ADR-0021 emit + attributes 语义 |
| `TopologyEdge.attributes` | 纯 free-form → documented free-form(description 列 network/hccs/fabric-link/runs-on 各边属性 + 单位 note + additionalProperties: true)|
| `frontend/src/services/types.ts` | `pnpm run gen:types` regen(本 task 只 regen 不手改)· TopologyEdge union 加 `"runs-on"` · NPU 加 `pcieBandwidthGBps?: number \| null` |

**验证(CI gate 对齐)**:
- `swagger-cli validate docs/api-contract.yaml` → `is valid`(validate-contract job)
- `pnpm run gen:types` → types.ts regen + `git diff --quiet` 提交后 CI drift check 通(gen:types drift job)

---

## §4 Open questions

### (a) 带宽属性是否 depth-gated

network/hccs 边带宽属性在 depth=cluster/nodepool 粗粒度视图是否需要(可能信息过载):
- 选项 a:始终带(前端按 depth 决定是否渲染 hover · 契约不 gate)
- 选项 b:仅 depth≥node 时 aggregator emit(减 payload)

**当前倾向**:选项 a(契约始终带 · 前端 depth 控制渲染 · T104 emit 不按 depth gate 带宽属性 · 保契约简单)· T104/T202 实施时再评估 payload。

### (b) 带宽单位 GB/s vs Gbps 统一

本 ADR 新增带宽属性统一 **GB/s**(`bandwidthGBps` · gigabytes)· 但 legacy `switch` 节点属性 `bandwidthGbps`(ADR-0004 · Gbps/gigabits)保留原单位:
- **divergence 已知**:switch 节点属性是 Gbps(网络设备惯例)· 新 edge/PCIe 属性是 GB/s(本地互联惯例 + 同轴比较)· 二者单位不同(1 GB/s = 8 Gbps)
- **不在 Phase 12 统一**:switch `bandwidthGbps` 是**节点属性**(非 edge 带宽)· 改它需 ADR-0004 cascade + mock + 前端 switch hover · cosmetic 价值低 · 留后续 cleanup
- **当前倾向**:新属性 GB/s 不变(PCIe/HCCS 厂商惯例 GB/s · 同轴比较价值)· switch Gbps 留原 · ADR 显式记 divergence 避免未来 P4 横向扫误判

### (c) network/hccs 边 utilization 数据来源

- mock(T105)给静态 fixture utilization · 真实(Phase 13+)需 NPU-Exporter / 网络 telemetry 采集
- Phase 12 mock 阶段:utilization 是 fixture 值(供前端渲染验证)· 真实采集留真硬件验证

---

## §5 引用

### 上游(本 ADR 决策依据)

- ADR-0019 §2 Decision A Track B 拓扑数据全保真(本 ADR 是 contract codification)
- ADR-0004 inter-node fabric topology(NetworkLink BandwidthGbps/Medium/Utilization/RTT · network/fabric 边带宽属性复用源)
- ADR-0005 pod-in-topology fusion(workload/pod 节点 + binds-to/pd-pair · runs-on 边补充点)
- ADR-0006 wire-type enum regen(hccs/network enum 已存在 native · 本 ADR 补 aggregator emit)

### 下游(本 ADR 触发 Track B + Track C execution)

- P12-T-104 backend aggregator emit network/hccs/runs-on 边 + PCIE + 带宽属性(消费本契约 · §2 Decision A-E)
- P12-T-105 mock-data set-a/b/c PCIE/HCCS/node 互联带宽 fixtures + schema.json(消费本契约 · 带宽 fixture 值)
- P12-T-202 前端拓扑增强:绿色 network 连线 + edge hover 带宽 + PCIE/HCCS 渲染 + runs-on 连线(消费 gen:types 新 union)
- P12-T-203 右栏 workload 信息(runs-on 边 → 负载节点落点)

### 契约改动 commit chain(本 task)

- api-contract.yaml diff(NPU pcieBandwidthGBps + TopologyEdge runs-on + edge attributes doc)+ frontend/src/services/types.ts gen:types regen · swagger-cli validate `is valid`

---

**END of ADR-0021**
