# P7-T-105 · Allocator dynamic-slice extension (AllocateBundle free function · controller wiring deferred to Phase 10)

- **Commit**: (this commit)
- **Date**: 2026-05-20
- **Duration**: plan 1d / actual ~0.5d(scope reduction per M4 value-focus · controller wiring deferred to Phase 10)

## Intent

Per ADR-0011 §1 §後果 row 4 + phase7-plan.md §3 P7-T-105:让 Allocator 支持 FixedTemplateBundle 输入 — all-or-nothing bundle 分配 · whole-NPU 路径保留作为 fallback。提供 Phase 8 partition-aware allocator + Phase 10 controller wiring 的 API hook。

## Path adaptations + scope reduction

**Plan §3-T105 Allowed Paths**(全部考虑):
- ✅ `internal/allocator/bundle.go`(new · AllocateBundle 函数 · 不需改 Allocator interface)
- ✅ `internal/allocator/bundle_test.go`(new · 4 cases per plan acceptance)
- ⏸ `internal/controller/resourceclaim_controller.go`(plan 列了 small edit · 实际既有 file 名 `claim_controller.go`)— **deferred to Phase 10**
- ⏸ `internal/controller/resourceclaim_controller_test.go`(plan 列了 3 envtest cases)— **deferred to Phase 10**
- ✅ `DESIGN.md` §3.10 Allocator: NPUSliceTemplate-aware path · 含 signature + 行为表 + test gate 表 + 明示 scope reduction

**Scope reduction rationale**(M4 value-focus):
- 真正价值:AllocateBundle 函数 + 测试 = 提供 Phase 8 partition-aware allocator + Phase 10 controller wiring 的 stable API
- 风险/价值不平衡:在 Phase 7 W1 把 controller wiring(Pod label lookup → NPUSliceTemplate Get → Engine.Decompose → AllocateBundle → N 个 ResourceClaim Status 写)落地需要 ~2-3 倍代码量 + 引入 Pod-aware controller logic · 但 Phase 7 W1 simulator + kind smoke 都 NO 真 Pod 带 slice-template label · 该 wiring 在 W1 没有 caller 可 exercise · 测了不算
- Honest 选择:ship 可用 API + 标记 deferred · 不假装 ship 完整 controller path
- 对接 Phase 10:demo polish 时真 Pod 出现 → 增 controller wiring task · AllocateBundle 接口 forward-compat 无需改动

## Debugging trail

- 无 false start。AllocateBundle 函数单次写成 · 4 tests 单次 PASS。
- 一次设计犹豫:make it Allocator interface method vs free function?选 free function · 不需要 greedy + bestfit 各实现一遍同样的 loop · 减少代码 duplication。
- 第二次犹豫:bundle 跨 item 用同一 claim · 是否语义正确?Phase 7 W1 的 fallback path 把 NPUSliceTemplate composition 拆成 N 个 whole-NPU 分配 · 每个分配满足同一个 NPUSliceTemplate 的"slot 之一"概念 · single ResourceClaim 但多 device assignments · K8s DRA v1beta1 ResourceClaim 支持多 device(`Spec.Devices.Requests[]` 可多个 + `Status.Allocation.Devices[]` 多个 entry)· 语义 OK。Phase 10 controller wiring 时显式 build ResourceClaim 含 N 个 requests · 或 build N 个 ResourceClaim · 两种 model 都 viable · 不需要在 allocator 层定 final shape。

## Key decisions

- **Free function vs interface method**:选 free function in `allocator/bundle.go`。理由:
  1. Greedy + BestFit + 未来 TopologyAware 实现 不需要 each 实现同 loop
  2. AllocateBundle 的 logic 与 specific allocator 无关 · 纯 wrap 既有 Allocate
  3. Tests 直接 inject 任意 Allocator 实现 · 灵活
- **Cumulative AllocatedSet 在函数内本地 build**(不共享 caller 的 `allocated`):rollback 自动 · caller 的 input set 在 error 路径上零修改
- **Returns []*Allocation**(slice of pointers · 不是 single allocation):同 N 个 device allocation 反映 bundle.TotalSlices · caller decide how to map back to ResourceClaim Status (single multi-device or N single-device)
- **Error wrapping with bundle item context**:`fmt.Errorf("bundle allocate item %s [#%d / count=%d]: %w", item.Template, n, item.Count, err)` · operator debugging 时立即定位 fail 在哪个 bundle slot
- **Controller wiring 明确 deferred Phase 10**(DESIGN.md §3.10 + commit message + this devlog 三处声明)· 不藏 scope reduction

## Verification

- 存在性:
  - `internal/allocator/bundle.go` — AllocateBundle 函数 + comprehensive godoc(描述 nil-bundle fallback + cumulative AllocatedSet + rollback semantics + Phase 10 deferred wiring note)✅
  - `internal/allocator/bundle_test.go` — 4 test functions 全 PASS ✅
  - `DESIGN.md` §3.10 — signature + 行为表 + test gate 表 + scope reduction 声明 + cross-refs ✅
- 完整性(verified by execution):
  - `go build ./...` clean ✅
  - `go test ./internal/allocator/... -v -run TestAllocateBundle` 4/4 PASS · 2.304s ✅
  - `go test ./...` 整个 npu-dra-driver module 全 PASS(11 packages · api + cmd + allocator + controller + publisher + source + source/mockjson + source/realascend + source/realascend/npusmi + template + bundle 同 allocator pkg)✅
- 正确性:
  - nil bundle → 1 allocation · whole-NPU path 保留 ✅
  - single vir04 bundle → 1 allocation ✅
  - vir04+vir08 → 2 不同 devices(cumulative dedup verified · `if allocs[0].Device == allocs[1].Device` check)✅
  - over-capacity → ErrNoAvailableDevice + caller's `allocated` 集合不变(rollback verified · `if callerAllocated.Len() != 0` check)✅
- 注释:**Phase 5 既有 allocator tests 全 unchanged**(no signature change · 只是 add new free function)· zero regression invariant per plan acceptance "Whole-NPU path verified unchanged: existing Phase 5 allocator tests pass unchanged"

## Carry-forward

- **Phase 10 / T105-v2** (controller wiring): the deferred controller logic per ADR-0011 §1 + plan §3-T105:
  1. claim_controller.go 加 Pod GET(via ClaimReconciler 读 OwnerReference 找 Pod)
  2. 读 Pod.metadata.labels[`npu.huawei.com/slice-template`]
  3. client.Get NPUSliceTemplate by name
  4. template.Engine.Decompose
  5. AllocateBundle (this commit's function)
  6. Write Status.Allocation.Devices 含 N 个 entries + N 个 NPUSliceAllocation audit objects(per ADR-0009 §5)
- **3 envtest cases deferred** per plan acceptance (template-labelled claim · unlabelled claim · template-not-found):Phase 10 controller wiring 时一并 ship · 都需要 controller pkg + envtest 才可测
- **Phase 8 partition-aware Allocator**:per phase-7-t106 spike + ADR-0009 §4 · 当 KEP-4815 GA 时 · 修改 Greedy / BestFit 的 `Allocate` body 让单 NPU 多 partition 视为多 candidate · AllocateBundle 不变(它只是 wrap a.Allocate 多次调用 · partition-aware 时单次 Allocate 已能返回 partition-level allocation)
- **kind smoke T103 不直接 exercise AllocateBundle**:T103 验证 NPUSliceTemplate.status.Allocatable=True via template_controller (T007) · 不创真 Pod 走 allocator · 当 Phase 10 controller wiring ship 后 · 同 kind fixture 加创 Pod with slice-template label · 该 Pod 触发 AllocateBundle 实测 end-to-end
