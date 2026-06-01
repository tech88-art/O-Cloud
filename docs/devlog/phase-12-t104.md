# P12-T-104 · backend aggregator — network/hccs/runs-on 边 + PCIE + 带宽属性

- **Commit**: (this commit)
- **Date**: 2026-06-01
- **Duration**: 2d plan / ~1d actual(aggregator emit + model 2 字段 + 8 新测 + 3 测更新)

## Intent

per ADR-0021 §2 让 backend aggregator emit Track B 全保真拓扑:
- NPU `pcieBandwidthGBps` 节点属性(host↔NPU PCIe)
- `network` 边(node↔node inter-node · bandwidthGBps/medium/utilization)
- `hccs` 边(npu↔npu intra-node · 同 node+hccsGroup ring)
- `runs-on` 边(非 NPU pod→node)
为 T202/T203 前端渲染提供数据。消费 T003 契约(edge type 枚举已含 runs-on)。

## Path adaptations(plan literal vs codebase reality)

1. **mock/*.go 无需改**(plan Allowed Paths 列了 mock · 实际不需要):新字段加在 `model.NPU`(PCIeBandwidthGBps/HCCSBandwidthGBps)+ `aggregator.NetworkLink`(BandwidthGBps)· mock source 已用 `json.Unmarshal` 把 npus.json → []*model.NPU + networkLinks.json → []NetworkLink → **新字段经 JSON tag 自动流过**(loadNPUs/loadLinks 不变)。T105 mock 数据填值即可。
2. **network 边 = 复用 NetworkLink 按端点分类**(非新 input 字段):appendFabric 里 link 两端都是 node → `network`(ADR-0021)· 含 switch 端 → `fabric-link`(ADR-0004 不变)· 现有 set-a-small links 是 node↔switch → 仍 fabric-link(零回归)· T105 加 node↔node links → network。
3. **hccs 边 = 从 NPU hccsGroup 派生**(非显式 HCCS input):同 (node, hccsGroup) 的 NPU 按 Index 排序成 ring(n=2→1 边 · n≥3→闭环 n 边)· bandwidthGBps 取 source NPU 的 HCCSBandwidthGBps(symmetric)。
4. **hccs/network gate = IncludeFabric**(非新 flag):二者是硬件互联拓扑 · 复用现有 fabric toggle(避免新增 contract query param + 前端 plumbing)· "IncludeWorkloads 无关"(plan)= 二者在 IncludeFabric 下 · 非 IncludeWorkloads。runs-on gate = IncludeWorkloads(它是 workload 拓扑)。

## Key decisions

- **零回归靠 3 重 conditional**(P4 + plan zero-regression):① pcie 属性仅 npu.PCIeBandwidthGBps != nil 才 stamp(旧 fixture 无字段 → byte-equivalent)② network/hccs gate IncludeFabric=true(默认 off → 无)③ runs-on gate IncludeWorkloads + pod 无 binding + nodeName 解析。**验证**:所有 IncludeFabric=false/IncludeWorkloads=false 既有测试不变(仅 3 个 IncludeFabric=true 测试 +1 hccs · 因 set-a fixture node-1 有 2 NPU 同 hccs-0)
- **hccs ring 而非 full mesh**:n NPU → n 边(ring)非 n² (mesh)· 对齐 ADR-0010 "910B ring-of-rings adjacency" · n=2 特判 1 边(避免 0↔1 + 1↔0 dup)
- **runs-on 仅 len(Bindings)==0 pod**:有 binding 的 NPU pod 走 binds-to · 无 binding 的非 NPU pod 走 runs-on · 二者互斥(一个 pod 不会同时 emit)· binding 越界 drop 的 NPU pod 不 fallback 到 runs-on(len(Bindings)>0 → 不 runs-on)
- **单位**:network/hccs/fabric 边 `bandwidthGBps`(GB/s · ADR-0021)· fabric-link 保留 legacy `bandwidthGbps`(Gbps)+ 新 `bandwidthGBps`(supplied 时 additive)· NetworkLink 双字段(BandwidthGbps int legacy + BandwidthGBps float64 new)

## Verification

P3 三项验证维度:

- **存在性**:model/npu.go 2 字段 + aggregator/topology.go(3 边 emit + appendHCCS)+ topology_test.go(8 新测 + 3 更新)落地
- **完整性**:plan T104 acceptance 4 项核 — ① `go test ./pkg/aggregator/...` 通 · network/hccs/runs-on/pcie 断言 PASS(8 新测:每边 1 happy + 1 edge)✓ ② emit 边 type ∈ 契约枚举(network/hccs T003 已有 · runs-on T003 加)· attributes 含 bandwidthGBps ✓ ③ make test/lint 等价(go test + vet + gofmt · 下方)✓ ④ IncludeFabric/IncludeWorkloads=false zero-regression(既有非 fabric 测试不变)✓
- **正确性 · 实证**:
  - `go build ./pkg/aggregator/... ./pkg/model/...` + `go vet ./...` → **clean**
  - `go test ./...`(全 backend)→ **ALL PASS**(mock source 用 aggregator · 无破)
  - `go test ./pkg/aggregator/... -cover` → **93.5%**(核心包 ≥70% ✓)
  - `gofmt -l` → 无 diff(3 文件 formatted)
  - 3 既有 IncludeFabric=true 测试更新 +1 hccs(node-1 hccs-0 pair · grep-verified fixture)· 其余 byte-equivalent

## Carry-forward

- **P12-T-105 mock**:set-a/b/c 填 ① npu `pcieBandwidthGBps` + `hccsBandwidthGBps` ② networkLinks.json 加 node↔node links 带 `bandwidthGBps`/`medium`(eth/roce/ib)/`utilization` ③ schema.json 加对应 $defs。fixtures 填值后 aggregator 自动 emit(本 task 逻辑已 ready)
- **P12-T-202 前端**:gen:types 新 union(runs-on)+ NPU.pcieBandwidthGBps · network 绿边 + hover bandwidthGBps + hccs 渲染 + runs-on 连线 · hccs/network 在 showFabric toggle 下
- **真带宽 telemetry**(Phase 13+ · ADR-0021 §4 c):utilization 现 mock fixture · 真采集留真硬件
- **hccs ring vs mesh**:现 ring · 若前端需 full HCCS 连接可视化 → 后续评估 mesh option

## §0a.11 compliance

- §0a.11:main agent 直接做(backend 单模块 · 我持 ADR-0021 fresh context + T104→T105 耦合连续性 · 无 subagent)· strict verify = go test 全 backend + 93.5% cover + vet + gofmt + 8 新测断言
- rhythm(v2):commit 后续 T105(Track B mock · 同模块连续性)· 不停 · 不 push(累积到 phase-12-complete tag)
