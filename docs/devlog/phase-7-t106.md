# P7-T-106 · KEP-4815 Partitionable Devices spike (docs-only research)

- **Commit**: (this commit)
- **Date**: 2026-05-20
- **Duration**: plan 0.5d / actual ~0.3d(WebFetch 一次 + spike doc 写 + ADR-0009 §4 status refresh + arch §13 Phase 8 row 更新)

## Intent

Per phase7-plan.md §4 P7-T-106 + ADR-0009 §4 Partitionable Devices forward note · refresh upstream KEP-4815 status against the Phase 4 baseline assumption("est. K8s 1.37 GA")+ enumerate Phase 8/10 migration cost matrix。

## Path adaptations

- 计划 §4-T106 Allowed Paths 全部 covered:
  - `docs/research/k8s-partitionable-devices-spike.md`(new · 7 sections · §1 KEP status + §2 upstream PRs + §3 breaking-change risk matrix + §4 effort matrix + §5 coexistence trade-offs + §6 Phase 8 entry checklist + §7 references)✅
  - `docs/adr/0009-npu-dra-driver.md` §4 forward note 状态字段刷新(2026-05-20 P7-T-106 数据)✅
  - `docs/architecture.md` §13 Phase 8 row 加 P7-T-106 spike 引用 ✅

## Gating decision outcome

**KEP-4815 status drift confirmed**:
- ADR-0009 §4 v1(2026-05-19)估计: 1.35 Alpha / 1.36 Beta / **est. 1.37 GA**
- P7-T-106 spike(2026-05-20)发现:1.35 Alpha confirmed · 1.36 Beta confirmed · **GA timing unconfirmed**(upstream issue 不 commit 具体 K8s minor)
- 影响:Phase 8 entry 应作 "1.37 minimum · possibly 1.38+" 计划 · 不能假设 1.37 GA 一定准时

**Migration cost matrix**(单 track · 无 parallelism · 估 ~13-18 calendar days = 约 3 周):
- baseline 1.32 → 1.36+ bump 1d(同时 unblock NumaAffinity #12 + ProxyImage #T102)
- AscendDevice 扩 partition 字段 0.5d
- mockjson partition fixture 1d
- realascend lab partition emit 2-3d(lab-dependent)
- template engine `dynamic-shard` unblock 1d
- **allocator partition-aware Allocate 3-5d**(HIGH risk row)
- kind smoke + lab smoke 集成 2d
- docs + checkpoint 1d 合计

## Key decisions

- **Coexistence model**:**NPUSliceTemplate 路径不删除** when Partitionable Devices GAs · 保留为 operator-facing high-level 组合语法 + 多租户隔离首选(whole-NPU = strong isolation)· partition 路径作为单租户 performance 优化。Phase 11+ 才考虑废弃 NPUSliceTemplate(如果到时 partition allocator 完全替代)
- **Phase 8 baseline bump 三角 unblock**:同一次 K8s 1.32 → 1.36+ baseline bump 解决 3 个 deferred 项:
  - NumaAffinity wrap(known-issues #12 / phase-7-t002 devlog)
  - ProxyImage chart default flip(phase-7-t102 devlog)
  - Partitionable Devices Beta 启用 + 渐进式 partition emit
  这是 ADR-0011 §3 lab gating policy 倡导的"延后 expensive 决策到合理 phase boundary"实例化 · 三件事同时 ship 比逐次干扰小

## Verification

- 存在性:
  - `docs/research/k8s-partitionable-devices-spike.md` — 7 sections + 4 table + Phase 8 entry checklist ✅
  - `docs/adr/0009-npu-dra-driver.md` §4 first 行 "事实" 加 2026-05-20 P7-T-106 refresh + cross-ref ✅
  - `docs/architecture.md` §13 Phase 8 row 加 spike 引用 + Phase 8 baseline bump triangle ✅
- 完整性(verified by execution):
  - WebFetch https://github.com/kubernetes/enhancements/issues/4815 一次性获取 stage/beta label + 1.35/1.36 milestone 确认 + "Tracked for Docs Freeze" 完成状态 ✅
  - 无代码改动 · 无 build / test ✅
- 正确性:
  - KEP-4815 阶段标识 "Beta" + 1.35 Alpha + 1.36 Beta 与 upstream issue body 一致(WebFetch 显式提取 "stage/beta" label · "alpha in 1.35 to beta in 1.36" 字面表述)
  - GA "not yet specified" 与 ADR-0009 v1 "est. K8s 1.37 GA" 假设有出入 · 本 spike 显式记录 drift · 不沿用旧假设

## Carry-forward

- **Phase 8 entry session 必读**:本 spike + `docs/known-issues.md` #12 + `docs/devlog/phase-7-t102.md` · 三者共同明确 Phase 8 baseline bump task(单 task 解决 3 个 Phase 7 deferred)
- **Phase 8 W1 pre-flight checklist**(spike §6 列):
  - Re-WebFetch KEP-4815 拿 GA 最新 status
  - Run `gh issue list -R kubernetes/enhancements -L 50 --search "4815"` 列实际 merged PRs
  - Confirm kindest/node 1.36 release status + Partitionable Devices feature gate enable path
- **Phase 8 entry meeting 决策**:
  - (a) target K8s minor(1.36 with feature gate · 1.37 with potential GA · 1.38 if 1.37 delayed)
  - (b) NumaAffinity + ProxyImage + Partitionable Devices 是否同 phase 同 task ship · 还是 stagger
  - (c) partition-aware allocator 作 Phase 8 deliverable 还是 Phase 9-10 polish
- **NPUSliceTemplate 保留为 operator-facing API**:Phase 8/10 Partition 路径上线后 · Engine.Decompose body 内部切换 · 用户 ModelService.yaml + NPUSliceTemplate.yaml 零改动 · 这是 P7-T-007 + P7-T-106 + ADR-0011 §1 §後果 合力的 forward-compat 设计
