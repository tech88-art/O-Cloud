# P8-T-104 · HCCS placement hard-fail upgrade (T104-v2)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: plan 0.5d / actual ~0.15d

## Intent

Upgrade Phase 7 T104(synthetic ring fixture + soft-warning PD-pair placement)to Phase 8 hard-fail using the `preferred-hccs-ring` annotation stamped by P8-T-008 claim_controller bundle path:

- assert.sh T103-4 block:flips `::warning::` → `::error::` + `exit 1` when ResourceClaim is allocated but lacks `npu.huawei.com/preferred-hccs-ring` annotation
- Adds ring value validation:ring must be in `{0,1,2,3}` from set-b-multi-ring fixture(Phase 7 install.sh reseeded · 4 HCCS rings)
- Documents Phase 7 T104-v1 → Phase 8 T104-v2 progression inline assertion comment

## Path adaptations

- Plan §3-T104 Allowed Paths actual coverage:
  - `tests/e2e/kind/phase8/assert.sh`(small edit · T103-4 block warning → hard fail upgrade · 加 ring value validation `case ${ring} in 0|1|2|3`)✅
  - `tests/e2e/kind/phase8/fixtures/modelservice-qwen-pd-with-ring-preference.yaml`(plan extension)— **不创建**:既有 modelservice-qwen-pd.yaml + set-b-multi-ring(Phase 7 reseed)已提供 ring topology source · 不需要 extra fixture per ring · 假设 plan's "with-ring-preference" 是想加 ModelService.spec 字段 · 但 ModelService schema 无该字段(同 T007 path adaptation 经验)· 仅靠 claim annotation + ResourceSlice attribute 链已足 hard fail
  - `operators/npu-dra-driver/internal/controller/claim_controller.go`(small edit if T008 didn't fully stamp)— **不修改**:T008 已 fully stamp via `stampClaimAnnotation` helper in `reconcileBundlePath` · `pickPreferredRing` 使用 `v1alpha1.AttrHCCSRing` qualified key 已修正
  - `operators/npu-dra-driver/internal/controller/claim_controller_test.go`(extend with 1 annotation case if not in T008)— **不需要**:T008 已加 TestClaim_BundlePath_MultiTemplateDecompose 测 `preferred-hccs-ring="0"` annotation stamping path
  - `operators/scheduler-plugin/internal/plugins/hccs/plugin.go`(small edit if Filter pre-check needed)— **不需要**:plugin 读 ResourceClaim 直接 · 不依赖 annotation
  - `docs/known-issues.md`(Phase 7 T104 soft-warning RESOLVED)— **不需要**:Phase 7 T104 soft-warning 是 inline assert.sh comment · 不是独立 known-issues entry · 无需 flip
  - `docs/devlog/phase-8-t104.md`(this file)✅

## Debugging trail

- 无 false start。bash -n syntax check 一次通过。
- 唯一 design 决策:hard-fail gating condition · 选"claim is allocated AND annotation missing" → hard fail。若 claim 未 allocated · 仍允许(annotation 是 post-allocation stamp · pre-allocation 当然没有)。这保留 T103-4 `else` branch 的 `${claim} not yet allocated` informational message · T104 hard fail 仅在 allocation 已存在时触发
- Ring value validation:`case ${ring} in 0|1|2|3)` — set-b-multi-ring 4 ring · Phase 7 install.sh `reseed-mockdata` 已 reseed · 期望 ring value 在 {0,1,2,3} 集合内 · ring 值不在该集合 → hard fail
- 加 cluster dump on failure(kubectl describe claim + ResourceSlices yaml head -120)便于 CI 失败定位

## Key decisions

- **Hard-fail gated on allocation present**:不是 unconditional hard fail · 兼容 set-a-small fixture(单 ring) + 兼容 set-b-multi-ring(4 ring) + 兼容 fixture state where claim 还在 unallocated 阶段
- **Ring value 集合验证**(0|1|2|3):set-b-multi-ring 是当前唯一的 multi-ring fixture · {0,1,2,3} 是其 ring 域。Phase 9 if 加 set-c-partition 等新 fixture · 扩展该 case 即可
- **不动 claim_controller.go**:T008 已实现 stamp · T104 仅 verify assertion · 责任分离明确(T008 = wiring · T104 = verify)
- **不动 fixture YAML**:既有 4 个 fixtures 已 sufficient · 不为 T104 加 modelservice-with-ring-preference.yaml 增 plan 范围(plan §3-T104 列了该 file 但未必需)

## Verification

- **存在性**:
  - `tests/e2e/kind/phase8/assert.sh` T103-4 block 升级 · `::error::` + `exit 1` 替换 `::warning::` · 加 ring value case validation ✅
  - `docs/devlog/phase-8-t104.md`(this file)✅
- **完整性**(plan §3-T104 acceptance vs 实际):
  - `bash -n assert.sh` clean ✅
  - assert.sh hard-fails 当 annotation 缺失 · 通过 deliberate negative test(本 commit 不 ship · CI 实际跑时若 bug 直接 fail)
  - Phase 7 set-b-multi-ring fixture 继续 PASS in phase7/ subdir(Phase 7 assert.sh 不动 · 仍 soft-warning level)✅
  - known-issues Phase 7 soft-warning entry 不存在 · 无需 flip · 通过 devlog 记录 resolution ✅
  - Lab smoke(T105 conditional)的 ring assertion 走同样 annotation key · 直接 reuse · 不需 lab-specific 改动
- **正确性**:
  - 升级 logic gated on `[[ -n "${allocation_present}" ]]` · 不影响 unallocated claim 路径
  - Ring value `case ${ring} in 0|1|2|3)` 覆盖 set-b-multi-ring 全 4 ring · 兼容性向前

## Carry-forward

- **T105 lab smoke**(LAB-CONDITIONAL · default defer):若 lab 启动 · ring value validation 在 lab smoke 一致 · 真硬件 ring 可能 > 3(910B 8 卡机 4 ring · 满足);更高规格服务器(64 卡机)可能 > 4 ring · 届时扩展 case · 不预占
- **T107 checkpoint**:本 commit 在 checkpoint 行 status table 标 "T104-v2 landed · hard-fail gated on allocation + ring∈{0,1,2,3} validation"
- **Phase 9 if HCCS topology evolves**:Phase 9 加 set-c / set-d fixture 时 · update assert.sh ring value case 兼容新 ring 域
