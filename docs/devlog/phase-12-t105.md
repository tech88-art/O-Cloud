# P12-T-105 · mock-data arch/os + PCIE/HCCS/node 互联带宽 fixtures + schema.json

- **Commit**: (this commit)
- **Date**: 2026-06-01
- **Duration**: 1.5d plan / ~1d actual(schema + generator + 3 set transform · 含一次 regen 回退)

## Intent

per ADR-0020 + ADR-0021 让 mock 数据全保真 + 平台真实化(T104 aggregator 消费):
- 全 set arch arm64 + os openEuler(ADR-0020)
- NPU `pcieBandwidthGBps`(32)+ `hccsBandwidthGBps`(56)· node↔node `network` 带宽 links(ADR-0021)
- schema.json $defs 扩 + arch enum arm64
- generator(preset_small/stress + model)emit 新字段(future regen 一致)

## Path adaptations(plan literal vs codebase reality · 关键)

1. **不能 regenerate 覆盖 curated sets**(P3 honesty · 防数据丢失):set-a/c 是 generator-produced 但 **"T307 polished"**(meta.description)—— hand-tuned 远比 generator 输出丰富(events/slices/workloads 多)。实测 regenerate set-a → diff `-1546/+496`(workloads.json -777 · slices.json -427)= **大量 curated 数据丢失** → 立即 `git checkout` 回退。**结论**:generator 与 curated sets 已 pre-existing drift(preset_multi 还是 stub · set-b 纯 hand-curated)· regenerate 会破坏 demo/e2e 依赖的数据。
2. **改用 minimal-diff text transform**(保留 curated 数据 byte-for-byte):jq 不可用 → 写临时 Go transformer(stdlib-only · `t105transform.go` · **未 commit · 已删**)· 仅做 targeted 插入:① nodes arch/os/kernel string replace ② npus 每个 hccsGroup 行后 regex 插 pcie/hccs ③ networkLinks 数组尾插 node↔node ring links。**existing 数据零改动**(diff 纯 additive · set-c npus +1600=800×2 · nodes 仅 arch/os/kernel 行 · links 原 104 preserved + 100 new net-)。
3. **generator 同步 emit 但不 regen**(双轨):generator(preset_small/stress + model)加新字段 emit(future regen 含新字段 · 验证:temp regen → arm64/openEuler/pcie/hccs/net-links 全在 + ajv valid)· 但当前 curated sets 走 transform 保数据 · generator 是 future SoT(per configs/CLAUDE.md §8 · drift 是 pre-existing · 不在 T105 解决)。
4. **arch enum arm64-only**(schema · 满足 grep amd64=0):mock 数据代表真实 arm64 鲲鹏集群 · arch enum `["amd64"]`→`["arm64"]`(非保留 amd64 · 否则 grep 命中 schema)· dev amd64 是 build/dev 机非 mocked 节点。
5. **node↔node ring · transform vs generator 微差**:transform set-c 用全节点... 实为 per-leaf-group ring(transformer 用 node-name 顺序)· generator preset_stress 用 intra-leaf ring · 两者都是合法 node↔node network · curated/generator 已 drift · 不强求一致(P4 note · generator 是 future SoT)。

## Key decisions

- **PCIe 32 / HCCS 56 固定值**(非随机):910B 硬件规格固定 · transform + generator 都用固定值(generator 无新 RNG draw → 若将来 regen · seeded 序列不变)
- **node↔node = `net-<from>-<to>` id · medium roce · bandwidthGBps 25(~200Gbps RoCE)**:both 端点 node id → T104 aggregator 分类为 `network`(绿)· 区别 node↔switch fabric-link
- **临时 transformer 不 commit**:一次性数据迁移工具 · 删除 · 数据已落地 · generator 是可复现 SoT(README 注 arch arm64)

## Verification

P3 三项验证:

- **存在性**:schema 3 处扩(arch enum · NPU pcie/hccs · NetworkLink bandwidthGBps+medium enum)+ generator 3 文件 emit + 3 set × 3 file transform + README · transformer 已删
- **完整性**:plan T105 acceptance 4 项核 — ① 全 set arch arm64 + os openEuler(grep amd64=0 实证)✓ ② NPU pcieBandwidthGBps(set-a 24 / set-c 800)+ node↔node network 带宽(set-a 3 / set-c 100 net- links)+ hccsBandwidthGBps ✓ ③ ajv validate 通 ✓ ④ generator 产出与 schema 一致 ✓
- **正确性 · 实证**:
  - `ajv validate --spec=draft2020 --strict=false`(= CI validate-mockdata)→ **全 3 set 全 file VALID**(nodes/npus/networkLinks 含)· exit 0
  - `grep -rl "amd64" configs/mock-data/ --binary-files=without-match` → **空**(余 0 · schema 描述 reword 去 amd64 · generator README arch=arm64)
  - generator `go build` + temp regen `--preset small` → ajv **VALID** + 含 arm64/openEuler/pcie(24)/hccs(24)/net-links(3)→ generator-schema-data 三者一致
  - set-c links preserved:原 104(spine/leaf + node-leaf)+ 100 new net- = 204 · curated 数据零丢失(diff additive)

## Carry-forward

- **P12-T-202/203 前端**:set-a 现有 pcie/hccs/network 数据 · 前端 showFabric=true 时拓扑显绿 network 边 + hover bandwidthGBps + hccs ring + npu pcie hover(render-verify 数据 ready)
- **generator drift**(pre-existing · 非 T105 引入):curated sets 比 generator 输出丰富(T307 polish)· generator preset_multi 仍 stub(set-b)· 若将来要 generator 为唯一 SoT → 需把 curated polish 回灌 generator(大工程 · 独立 task · configs/CLAUDE.md §8 tension)
- **真带宽采集**(Phase 13+):pcie/hccs/network utilization 现 fixture 静态值 · 真采集留真硬件(ADR-0021 §4 c)

## §0a.11 compliance

- **共享契约 main-agent serial**(schema.json · root CLAUDE.md §11)· main agent 直接做 · T104→T105 同 data 链连续性 · 无 subagent
- strict verify = ajv 全 set + grep amd64=0 + generator regen schema validation 实证
- rhythm(v2):W2(Track A T101-103 + Track B T104-105)完成 · commit 后续 W3 Track C(T201 前端 layout)· 不停 · 不 push(累积到 phase-12-complete tag)
