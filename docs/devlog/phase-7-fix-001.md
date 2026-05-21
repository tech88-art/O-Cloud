# P7-fix-001 · post-tag CI fix: set-b-multi-ring schema compliance + Phase 5 fixture opt-out from T003 schedulerName auto-stamp

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: ~30 min (CI log triage + 2 root-cause fixes + local validation)

## Intent

`phase-7-complete` tag(c6dc03d)triggered 3 workflows on origin/dev: helm-lint ✅ · **CI ❌** · **e2e-kind ❌**。Triage via `curl GitHub Actions REST API`:

1. **CI > Validate Mock Data Schema · 失败**:`set-b-multi-ring` fixture(P7-T-104 落地)只 ship `meta.json + npus.json` 两个文件 · 不 conform schema 要求每个 set-* directory 含全部 11 entity 文件 + 每文件含 schema 全部 8 顶级 required 字段(meta + clusters + nodes + npus + slices + workloads + pools + presets)。NPU object 还缺 required `model` field + id pattern `^[a-z0-9-]+$` 拒绝 uppercase。
2. **e2e-kind > Phase 5 ModelService apply · 失败**:T003 (f164c6f) 加 deployment_builder 自动 stamp `spec.schedulerName="npu-scheduler"` 后 · Phase 5 step 18 在 Phase 6 scheduler-plugin install (step 22+) 前 apply ModelService → PD-pair Pods stamp 了 npu-scheduler 但调度器还没装 · Pods 永远 Pending。

## Path adaptations + fixes

### Fix 1: set-b-multi-ring schema compliance(`configs/mock-data/set-b-multi-ring/`)

- 全 11 个 JSON 文件按 `configs/CLAUDE.md` §3.2 convention 落地(clusters/events/meta/networkLinks/networkSwitches/nodes/npus/pools/presets/slices/workloads)
- 实现:`cp -f set-a-small/*.json set-b-multi-ring/`(继承 schema-compliant skeleton)→ 然后 python script 把 npus.json 的 `npus` array 替换成 set-b-specific 16-NPU multi-ring 内容(2 nodes × 4 rings × 4 NPUs)+ 加 required `model: "Ascend910B"` field + lowercase id 满足 pattern `^[a-z0-9-]+$`(nodea-npu-0 / nodeb-npu-0 etc · 注意 nodeName 仍是 nodeA/nodeB 因 nodeName 字段无 pattern 限制)
- 全 11 文件 meta.name 改为 `set-b-multi-ring` + description 描述 Phase 7 P7-T-104 用途
- 其他 entity 文件(clusters/events/networkLinks/networkSwitches/nodes/pools/presets/slices/workloads)沿用 set-a-small 内容(都是 1-entity stub · empty arrays 其他)· npu-dra-driver MockJSONSource 只读 `npus` array · 其他 entity 仅用于 schema 校验

### Fix 2: Phase 5 fixture 显式 opt out npu-scheduler

- `tests/e2e/kind/phase5/fixtures/modelservice-sample.yaml` 加 `schedulerOverride: default-scheduler`
- P7-T-003 的 effectiveSchedulerName helper 已实现 SchedulerOverride 路径(commit f164c6f):non-nil + non-empty pointer → 使用 override 值
- 注释明示原因:Phase 5 step 在 Phase 6 scheduler-plugin install 前跑 · 必须 opt out · Phase 6+ kind fixtures(`modelservice-multiring` / `modelservice-with-template`)NOT 加 override(他们故意走 npu-scheduler path 验证 T003 auto-stamp)

## Debugging trail

- 首先 GitHub Actions REST API 拉 runs · 确认 c6dc03d 3 个 workflows:helm-lint ✅ + CI ❌ + e2e-kind ❌
- 分别 hit `/jobs` 端点定位 failed steps:`Validate each set-*/ directory` + `Phase 5 — apply ModelService + assert PD pair + annotation`
- 第一次 schema validate 本地跑(`npx ajv validate`)`set-b-multi-ring/npus.json invalid` · missing `model` 字段 → 加 model:Ascend910B
- 第二次 schema validate · `id` 不 match `^[a-z0-9-]+$` pattern(大写 A B 不允许)→ lowercase ids
- 第三次 schema validate · 全 11 文件 valid ✅
- mockjson_test TestLoadSetBMultiRingFixture 仍 PASS(测试只检 hccsRing 分布 + nodeName 集 · 不依赖 id 大小写)

## Key decisions

- **set-a-small skeleton inheritance**(vs 重写整套 set-b 全数据):set-b-multi-ring 只是 npu-dra-driver test fixture · 不是 demo dataset(per configs/CLAUDE.md §3.4 differentiated sets = small / multi-site / stress)· 其他 entity 字段对功能无影响 · 沿用 set-a-small 内容是最小工作量 schema-compliant 路径
- **lowercase id**(`nodea-npu-0` vs `nodeA-npu-0`):schema NPU.id pattern `^[a-z0-9-]+$` 强制 · 这是既有约束 · 修符合;nodeName 字段无 pattern 所以 `nodeA` / `nodeB` 保留(与 fixture / mockjson test expectations 一致 · 0 churn)
- **schedulerOverride opt-out vs workflow reorder**:reorder workflow 把 Phase 6 scheduler-plugin install 前置 = bigger churn + 改变 phase semantic boundaries;opt-out 只改 Phase 5 fixture 一处 · 保留 phase 顺序 + 明示 T003 auto-stamp 反向兼容性(operators 已有方法 opt out)。同时 Phase 6+ 新 fixtures(multiring / with-template)NOT 加 override · 验证 T003 hard-fail assertion 仍 exercise

## Verification

- 存在性:
  - `configs/mock-data/set-b-multi-ring/` 11 个 JSON 文件存在(2 modified + 9 new)✅
  - `tests/e2e/kind/phase5/fixtures/modelservice-sample.yaml` 加 schedulerOverride + 8 行注释 ✅
  - `docs/devlog/phase-7-fix-001.md` (this file) ✅
- 完整性(verified by execution):
  - `npx ajv validate --spec=draft2020 --strict=false -s configs/mock-data/schema.json -d "configs/mock-data/set-b-multi-ring/*.json"` 全 11 文件 valid ✅
  - `go test ./internal/source/mockjson/... -run TestLoadSetBMultiRingFixture` PASS ✅
  - YAML lint(implicit · git tree clean)✅
- 正确性:
  - Phase 5 fixture schedulerOverride: default-scheduler · 之前 T003 测试已 cover empty-string-override fallback + 显式 override 路径(TestEffectiveSchedulerName 4 sub-cases)· 这是 production opt-out 模式 · 不是 hack
  - set-b-multi-ring/npus.json 16 NPU entries 仍含 hccsRing 0/1/2/3 分布 + 2 nodes(nodeA + nodeB)· mockjson test + Phase 7 T103 install.sh reseed 路径不变

## Carry-forward

- **后续 phase 加新 mock data sets**:必须按 11 文件 convention(configs/CLAUDE.md §3.2)· NPU id lowercase + 含 `model` 字段 · 否则 CI fail。configs/CLAUDE.md §9 prompt 模板可加 "schema-compliant skeleton inheritance" hint
- **T003 schedulerName auto-stamp 对 dev 工具链的反向影响**:任何在 inference-operator 控制的 PD-pair Pod 路径上不需要 npu-scheduler 的 fixture / 集成测试 / 生产 ModelService 需要显式 set `spec.schedulerOverride`。Phase 8/10 review 时审计所有 ModelService fixtures
- **CI workflow ordering audit candidate**(non-blocking):Phase 6 scheduler-plugin install 后置在 Phase 5 之后 · 是合 phase 编号顺序但与 T003 auto-stamp 默认有副作用。Phase 8 baseline bump task 可考虑 reorder workflow 让 scheduler-plugin 在 Phase 5 之前 install · 那 Phase 5 fixture 不再需要 opt-out
- **Tag phase-7-complete 已存在 on c6dc03d**:本 fix commit 在 tag 之 *后* · 不会重新打 tag · 但 `dev` branch HEAD 推进。Phase 6 也有类似 pattern(4b5acbe "P6 post-tag polish" 在 phase-6-complete 之后)· 一致
