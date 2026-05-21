# P8-T-103 · kind smoke E2E Phase 8 extension

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: plan 1d / actual ~0.5d

## Intent

Extend the kind smoke E2E to validate the Phase 8 busy-idle 垂直伸缩 substrate landed in T005-T008:
- Apply qwen-pd-busy + qwen-pd-idle NPUSliceTemplates(Phase 7 reconciler picks them up)
- Apply qwen-pd ModelService with annotation `npu.huawei.com/slice-template=qwen-pd-idle` pre-seeded
- Apply qwen-pd-scaler NPUVerticalScaler(P8-T-005 CRD · P8-T-007 controller observes + sets ConditionActive)
- Stamp the slice-template annotation onto generated ResourceClaim(s)(install.sh workaround for the carry-forward deployment_builder annotation propagation gap)
- Assert ConditionActive observed + ModelService annotation stays at qwen-pd-idle in degraded mode(no Prometheus → Ingestor NoData → no scaling decision)
- Skipped per Phase 8 W1 decisions:NumaAffinity active(T002 K8s 1.32 stay)+ partition discovery(T101 deferred Phase 10)

## Path adaptations

- Plan §3-T103 Allowed Paths covered:
  - `tests/e2e/kind/phase8/install.sh`(new · 4 cmd functions · all chain · ~140 行)✅
  - `tests/e2e/kind/phase8/assert.sh`(new · 6 assertions · 2 SKIPPED · ~100 行)✅
  - `tests/e2e/kind/phase8/fixtures/npuslicetemplate-qwen-pd-busy.yaml`(new · 1× vir08)✅
  - `tests/e2e/kind/phase8/fixtures/npuslicetemplate-qwen-pd-idle.yaml`(new · 1× vir04)✅
  - `tests/e2e/kind/phase8/fixtures/modelservice-qwen-pd.yaml`(new · pre-seeded `npu.huawei.com/slice-template=qwen-pd-idle` annotation)✅
  - `tests/e2e/kind/phase8/fixtures/npuverticalscaler-qwen.yaml`(new · busy=75 / idle=20 / window=300 / cooldown=300)✅
  - `.github/workflows/e2e-kind.yml`(small edit · add Phase 8 step + cluster-dump group)✅
  - `docs/devlog/phase-8-t103.md`(this file)✅
- Forbidden Paths 兑现:`operators/**` 0 改动(T103 install + assert + fixtures only)
- 没创建 `reseed-set-d-busy-load.json`:plan §3-T103 列了"or use existing set-b-multi-ring + script-injected high-utilization signal · TBD per T103 entry"。本 T103 走"degraded mode demo · 不实际触发 scaling"路径,不需要 high-utilization injection · set-b-multi-ring 由 Phase 7 install.sh 已经 reseed · Phase 8 不再 reseed

## Debugging trail

- 无 false start。3 个 yaml 一次写成,1 install.sh + 1 assert.sh 一次写成。
- 第一次 yaml syntax check 失败:Python `open()` 默认 GBK 编码无法处理 yaml 注释里的中文字符("重启切片" / "垂直伸缩")。改 `encoding='utf-8'` 通过。
- 第二次 yaml check 失败:相对路径 `/d/code/ai-edge/...` 在 git bash 里 cwd 解析有问题。cd 到 repo root 再相对路径通过。
- bash -n syntax check 单次通过 · install.sh + assert.sh 都没 syntax issue
- workflow yaml safe_load 单次通过

## Key decisions

- **Degraded mode demo**(no Prometheus in kind smoke):chart `metrics.prometheusURL=""`(本 task 不设)· Ingestor 默认返回 NoData=true · NPUVerticalScalerReconciler 走 reconcile step 3 NoData stay 路径 · ConditionActive=True reason=NoDataThisTick + requeue 30s · ModelService annotation 保持 pre-seeded `qwen-pd-idle` 不变 · assert.sh T103-3 hard-fail 如果 annotation 翻转到 qwen-pd-busy(说明 Reconcile 有 bug 在 NoData 下还做 patch)
- **install.sh demo workaround**(`cmd_demo_bundle_path`):deployment_builder annotation→Pod-label propagation 是 Phase 8 carry-forward task(per T007 + T008 devlog)。为了在 kind smoke 内 demo T008 wiring 不依赖 propagation chain · install.sh 直接 `kubectl annotate resourceclaim` stamp slice-template annotation · 验证 claim_controller 走 bundle path · 这样 T008 wiring 可独立测试 · 不阻塞于 carry-forward
- **assert.sh 用 warning vs hard fail 区分**:NPUSliceTemplate Validated + ConditionActive observed + ModelService annotation unchanged 三项 hard fail(T008 wiring correctness depends on this)· ResourceClaim 部分 warning(claim 可能还在 unallocated · 不阻塞主路径)· NumaAffinity / Partitionable Devices SKIPPED 显式 echo(透明 · 不冒充 pass)
- **NPUVerticalScaler ConditionActive=True OR False 都算 observed**:在 degraded mode True · 在 TargetNotFound False · 两种状态都证明 Reconciler 跑过。grep pattern `'(True|False)'` 双 match 即可
- **WAIT_NVS_SECONDS=60**:NPUVerticalScalerReconciler 30s requeue cadence · 60s 应至少能观察到 2 个 reconcile cycle · 容错。

## Verification

- **存在性**:
  - `tests/e2e/kind/phase8/install.sh` 140+ 行 · 4 cmd functions(apply-templates / apply-modelservice / apply-scaler / demo-bundle-path / all) ✅
  - `tests/e2e/kind/phase8/assert.sh` 100+ 行 · 6 assertion blocks(4 hard + 2 SKIPPED) ✅
  - `tests/e2e/kind/phase8/fixtures/` 4 个 yaml files(NPUSliceTemplate busy + idle · ModelService qwen-pd · NPUVerticalScaler qwen-pd-scaler) ✅
  - `.github/workflows/e2e-kind.yml` 加 "Phase 8 — apply busy-idle 垂直伸缩 substrate" step + cluster-dump "Phase 8" group ✅
  - `docs/devlog/phase-8-t103.md`(this file) ✅
- **完整性**(plan §3-T103 acceptance vs 实际):
  - `bash -n` clean on install.sh + assert.sh ✅
  - yaml syntax clean across all 4 fixtures(`python -c "import yaml; yaml.safe_load(...)"` PASS with UTF-8) ✅
  - workflow yaml clean(`yaml.safe_load` of e2e-kind.yml PASS) ✅
  - Phase 5/6/7 phase{5,6,7}/install.sh + assert.sh 0 改动 · 完全 preserved ✅
  - Phase 8 sub-job runs in next CI push(本 commit push 后 GHA 触发 · CI 实证)— 留作 carry-forward verify
  - Assertions per ADR-0012 §5 mutation model:T103-3 hard-fail "annotation flipped in degraded mode" 反向 verify Reconcile 正确实现 NoData stay ✅
  - soft warning(NOT hard fail)when T101 deferred:T103-6 显式 echo "SKIPPED — Phase 8 P8-T-101 deferred to Phase 10" · plan §3-T103 acceptance 兑现 ✅
- **正确性**:
  - 4 yaml fixtures cross-ref clean:NPUVerticalScaler.spec.target.name=qwen-pd matches ModelService.metadata.name · scaleSlice.{busy,idle}TemplateName matches NPUSliceTemplate names · 命名空间 ocloud-system 一致
  - install.sh chain 顺序合理:templates → ms → scaler → bundle-path workaround · 与 Phase 8 reconcile 启动顺序 align(scaler 需要 ModelService target 存在 · ms 需要 NPUSliceTemplate ref 存在)
  - assert.sh 顺序 mirrors install.sh chain · 每 assertion 验证 install 后状态

## Carry-forward

- **deployment_builder annotation→Pod label propagation**(Phase 8 polish task 或 Phase 9):取消 install.sh `cmd_demo_bundle_path` 直接 stamp annotation 的 workaround · 走自然 propagation chain。涉及 claim_builder.go 修改:read `ms.Annotations[npu.huawei.com/slice-template]` → propagate to ResourceClaimTemplate.Spec.Template.Metadata.Annotations
- **Phase 10 lab smoke validates real scaling**:Phase 8 kind smoke degraded mode demo · 只 verify Reconciler observes + 不 patch · Phase 10 lab smoke 接真 Prometheus + 真 Ascend 卡 · 验证 real metric → real scaling decision → real Pod recreation → claim_controller bundle path → AllocateBundle on real silicon
- **T104 HCCS hard-fail upgrade**(next task):升级本 assert.sh T103-4 ResourceClaim preferred-hccs-ring annotation 从 warning 到 hard fail · 用 synthetic ring fixture(Phase 7 set-b-multi-ring · 已 reseeded)验证 ring stamp accuracy
- **T107 checkpoint**:本 commit 在 status table 标 "T103 landed · 4 fixtures + install.sh + assert.sh + workflow step · NumaAffinity/Partition skipped per W1 decisions"
- **CI 实证**(本 commit push 后):workflow run 完成 · phase8 step 跑通 · 不破 Phase 5/6/7 既有 jobs
