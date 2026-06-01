# P12-T-003 · ADR-0021 + api-contract.yaml 拓扑数据全保真扩展(共享契约 RFC)

- **Commit**: (this commit)
- **Date**: 2026-06-01
- **Duration**: 1-1.5d plan / ~0.7d actual(契约扩展 + gen:types + swagger validate · 无后端代码)

## Intent

Phase 12 唯一 api-contract RFC(共享契约 · main-agent serial)· codify Track B 拓扑全保真契约扩展:
- NPU `pcieBandwidthGBps`(host↔NPU PCIe 带宽 · GB/s)
- emit `network` 边(node↔node 互通 · bandwidthGBps/medium/utilization)
- emit `hccs` 边(npu↔npu intra-node · bandwidthGBps)
- 新 `runs-on` 边(非 NPU workload→node · 区别于 binds-to pod↔slice)
- edge attributes documented(供前端 hover tooltip · 单位统一 GB/s)

为 T104(aggregator emit)+ T105(mock fixtures)+ T202/T203(前端渲染)提供契约 substrate · gen:types 通后前端可消费新 union。

## Path adaptations

1. **hccs/network enum 已存在**(ADR-0006 regen)· 本 task **不加 enum 值**(仅加 `runs-on`)· 主要工作是 codify 其语义 + 补 emit 文档 + edge attributes doc。契约现状:enum 8 值但 aggregator 仅 emit 4(contains/fabric-link/binds-to/pd-pair)· hccs/network 是 "枚举存在但不 emit"(T104 补 emit)。
2. **arch.md 无独立 "拓扑数据模型" 章节** · 落点 = §13 RFC-003 追加表(ADR-0004/0005 topology schema 演进行所在)· 加 Phase 12 ADR-0021 行(承 ADR-0004+0005+0006 · 与既有 topology 行同组)。
3. **swagger-cli 非本地装** · 走 `npx -y -p @apidevtools/swagger-cli@4`(与 CI ci.yml:103 validate-contract job 同命令)· 网络可达 · 跑通。

## Key decisions

- **workload→node = 新 `runs-on` 而非复用 `binds-to`**(ADR-0021 §2 Decision D):binds-to = pod↔NPU slice 资源绑定(占切片)· runs-on = 非 NPU 负载落节点(无切片)· 两语义不同 · 复用会让前端无法区分 "占 NPU 的负载" vs "普通负载"(渲染样式 + focus 过滤都需区分)· 单独 type 保语义清晰(P4 不 conflate)
- **带宽单位统一 GB/s**(`bandwidthGBps` · §2 Decision E):PCIe/HCCS/network/fabric 边统一 GB/s · 与 NPU.pcieBandwidthGBps 同尺度 → hover 同轴比较(PCIe ~32 / HCCS ~56 / network ~50=400Gbps NIC)。**P1 单位 divergence 显式记 ADR-0021 §4 (b)**:legacy `switch` 节点属性 `bandwidthGbps`(ADR-0004 · Gbps/gigabits · 1 GB/s = 8 Gbps)保留原单位 · 不在 Phase 12 统一(switch 是节点属性非 edge 带宽 · cosmetic · 留后续)· ADR 显式记避免未来 P4 横向扫误判 "GBps vs Gbps 不一致 = bug"
- **edge attributes = documented free-form**(非 named schema · §2 Decision E):description 列各边类型属性 + `additionalProperties: true` · 与 TopologyNode.attributes 同 pattern(P4 单一 pattern)· 保 gen:types 输出最小(不生成新 named type · TopologyEdge.attributes 仍 `{ [key]: unknown }`)
- **本 task 只 regen types.ts 不手改**(frontend CLAUDE §4.1 + plan)· `pnpm run gen:types` 输出直接提交 · CI drift check gate

## Verification

P3 三项验证(契约 task · 跑 2 个 CI gate 实证):

- **存在性**:ADR-0021 + api-contract.yaml 4 处 diff + types.ts regen + arch 1 row + 本 devlog 落地
- **完整性**:plan T003 acceptance 5 项逐项核 — ① ADR §2 Decision A-E codified(PCIE 字段 / network 边 / hccs 边 / runs-on workload→node / 统一带宽属性)✓ ② api-contract.yaml PCIE 字段 + 带宽属性 doc + 边语义更新 ✓ ③ `swagger-cli validate` clean ✓ ④ gen:types regen + drift ✓ ⑤ arch 拓扑 row cross-ref ✓
- **正确性 · 2 CI gate 实证**:
  - `npx -p @apidevtools/swagger-cli@4 swagger-cli validate docs/api-contract.yaml` → **`docs/api-contract.yaml is valid`**(exit 0 · = CI validate-contract job)
  - `pnpm run gen:types` → **`openapi-typescript 7.13.0 · 147.9ms` 成功**(契约 parseable)· `git diff --stat types.ts` = 17 insert / 2 delete · grep 确认 `type: ... | "runs-on"`(line 1031)+ `pcieBandwidthGBps?: number \| null`(line 1109)· 提交 regen'd types.ts → CI gen:types drift check 将通(committed = regen output)

## Carry-forward

- **P12-T-104 backend aggregator**:消费契约 · emit network(node↔node)/ hccs(npu↔npu · 同 hccsGroup)/ runs-on(非 NPU workload→node)· npu attributes 加 pcieBandwidthGBps · edge attributes stamp bandwidthGBps/medium/utilization · **IncludeWorkloads=false 时 network/hccs 仍 emit**(硬件拓扑)· runs-on 仅 IncludeWorkloads=true · emit 的 type 必 ∈ 契约枚举(本 task landed)
- **P12-T-105 mock**:set-a/b/c 给 pcieBandwidthGBps + node↔node network 带宽 + npu↔npu hccs 带宽 fixtures · utilization 静态 fixture(真实采集留 Phase 13+ · ADR-0021 §4 (c))· schema.json 加对应 $defs
- **P12-T-202 前端**:gen:types 新 union(runs-on)+ NPU.pcieBandwidthGBps 可用 · network 绿色边 + hover 带宽 tooltip + PCIE/HCCS 渲染 + runs-on 连线
- **单位 note**:ADR-0021 §4 (b) GBps vs Gbps divergence · 后续若做单位统一 cleanup 需 cascade switch 节点属性 + mock + 前端 switch hover

## §0a.11 compliance

- **共享契约 main-agent serial**(root CLAUDE.md §11 + §6 + §0a.3):api-contract.yaml + schema 永不由 subagent 改 · 本 task main agent 直接做
- §0a.5 chat+ADR self-RFC:用户决策 2(拓扑全保真)= entry meeting chat 批准 · ADR-0021 + 契约 = codification(非新 RFC ceremony)
- rhythm(v2):commit 后续 T004 · 不停 · 不 push(累积到 phase-12-complete tag)
