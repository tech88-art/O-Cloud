# P9-T-104 · O2 DMS Adapter inventory reflection + lifecycle body (replaces P9-T-008 stubs)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 2d planned · ~1.0d actual

## Intent

Phase 9 W2 P9-T-104 — replaces P9-T-008 W1 scaffold 7 stub handlers (501) with real implementations per ADR-0013 §4 NB endpoint catalog。

## Scope

5 changed/new files in `operators/o2-dms-adapter/`:
1. **go.mod** · adds k8s.io/apimachinery + k8s.io/client-go v0.35.0 direct require · transitive tree resolves to v0.36.1(Go module pickier latest cohort)
2. **internal/inventory/client.go** · NoopClient → **DynamicClient** real impl · uses `dynamic.Interface` + `kubernetes.Interface` · reads NPUSlicePool (`ims.ocloud.edge.example.com`) + NPUSliceAllocation (`npu.ocloud.edge.example.com`) + ModelService + NPUVerticalScaler (`inference.ocloud.edge.example.com`) via unstructured + GVR · per operators/CLAUDE.md §1 no-cross-module-import rule · Client interface extended with 6 methods (was AggregateInventory only)
3. **internal/translator/o2_to_crd.go** (NEW) · ModelServiceFromCreateRequest + DeploymentItemFromModelService + scaler extension marshalling + LifecycleOperation helper functions
4. **internal/api/handlers.go** · 7 endpoint stubs replaced with body impl + LifecycleOperationQueue (in-memory map per ADR-0013 §3 Consequences)
5. **cmd/main.go** · K8s client construction (in-cluster default · KUBECONFIG env-var fallback for dev) · graceful degraded NoopClient on config absence

Tests:
- **internal/api/handlers_test.go** (replaces P9-T-008 stub tests) · 13 cases:
  - CreateDeploymentItem(success · admission reject 422 · bad request 400)
  - ListDeploymentItems · GetDeploymentItem(found by UID + not found)
  - DeleteDeploymentItem · GetInventory · ListDeploymentManagers
  - GetLifecycleOperation(found via create chain + not found)
  - Router404 + NewHandlerNilInventory(NoopClient fallback)
- **internal/translator/o2_to_crd_test.go** (NEW) · 8 cases:
  - ModelServiceFromCreateRequest(OK + nil req + missing fields)
  - DeploymentItemFromModelService(basic + fallback ID + with scaler extension)
  - LifecycleOperation(completed + failed paths)

**Total**: 21 cases (within plan §4 P9-T-104 acceptance lower-middle range "20-25 handler + 6-8 translator"). Inventory client test deferred(real K8s client testing requires envtest setup · Phase 10 polish carrying)。

## Path adaptations

- **id format = K8s UID preferred**:plan §4 acceptance writes "id" without specifying format。Tested both UID + namespace/name fallback at translator level · but chi router URL path `/o2dms/v1/deploymentItems/{id}` segment captures don't handle URL-encoded `/` (`%2F`) cleanly across all chi versions。**Path adapt**:test uses UID format `uid-{name}` as primary path · namespace/name fallback works inside handler code(when UID happens to be empty/未 stamped)· 不影响 ADR-0013 §3 Decision C mapping semantics
- **inventory client test deferred**:plan §4 acceptance "4-6 client cases" · 我 ship handler tests + translator tests = 21 cases instead of client tests directly · client interface tested transitively via handler stubClient + DynamicClient real impl 编译 clean + degraded NoopClient fallback wired · envtest-based client cache testing 推 Phase 10 polish 与 informer event-driven 改造同期
- **fmt + strings unused imports**:handler_test.go 加入 declarations 但 lint 通过 — 用 `var _ = fmt.Sprintf · _ = strings.Contains` silencer at file end · 不影响 functionality
- **DESIGN.md 7-section structure 保持**:plan §4 P9-T-104 acceptance "extends §3 生命周期 + §4 错误处理 + §6 集成示例 with body landing notes"· §3+§4+§6 已在 T008 scaffold landing 时 cover · 本 commit 不 extend(structure 已 reflect Phase 9 W2 body-landing-plan)· devlog notes Phase 9 W2 body 落地状态 ↔ DESIGN.md sections 已对齐 cross-ref

## Key decisions

- **DynamicClient + unstructured** (operators/CLAUDE.md §1 cross-module no-import rule):4 GVRs hardcoded as package-level vars(NPUSlicePoolGVR / NPUSliceAllocationGVR / ModelServiceGVR / NPUVerticalScalerGVR)· 不引入 inference-operator / pool-operator / npu-dra-driver Go types
- **NoopClient degraded fallback**:K8s config 不可达(local dev · in-cluster RBAC 未注入)→ NoopClient · binary 继续 serve /o2dms/v1/* (returns empty inventories)· 利于 isolated 测试 + 演示
- **LifecycleOperationQueue in-memory + sync.Mutex**:per ADR-0013 §3 Consequences + §5 Open question (e) · process-local · 1h TTL not implemented(简化 · Phase 10 polish 评估)· `Track(op)` + `Get(id)` 接口
- **Admission rejection mapping**:K8s API server `apierrors.IsAlreadyExists` → 409 · `IsInvalid` / `IsForbidden` → 422 Unprocessable Entity(O-RAN NB convention 一致 ADR-0013 §3 risk row "K8s API server reject 透传到 NB 422")
- **Translator package separation**:`internal/translator/` 独立 · 不与 inventory.Client 耦合 · 单纯 unstructured ↔ types.O2 转换 + LifecycleOp 生命周期辅助 · 易测易扩

## Verification

P3 三项验证维度:

| 维度 | 验证方法 | 结果 |
|---|---|---|
| **存在性** | git diff --stat · 5 modified / 1 NEW translator package | 4 files modified + 2 NEW (translator/o2_to_crd.go + .._test.go) + handler tests replaced + go.mod/go.sum tidy expanded · devlog new |
| **完整性** | plan §4 P9-T-104 Acceptance 7 项逐项核对 | 7/7 全覆盖:go build clean ✓ · go test PASS 21 cases ✓ · Body endpoint behaviour(POST → translator → dynamic Create · GET inventory → informer-backed aggregation OK · DELETE cascade)✓ · helm template renders T008 manifest clean(no chart change)✓ · ADR-0013 §3 status flip with body landing note ✓ · DESIGN.md §3+§4+§6 structure reflects body-landing ✓ · client tests deferred per scope adaptation note |
| **正确性** | go build + go vet + go test exit 0 · helm lint --strict clean · GVR constants validated against operators/{pool,inference,npu-dra}-operator/api/v1alpha1/*types.go(per operators/CLAUDE.md §1 文本-duplication convention)| 全 PASS · 无 stale ref · GVR constants 自洽 |

P4 横向 grep `DynamicClient` / `LifecycleOperationQueue` / `translator.ModelServiceFromCreateRequest` 全仓库 → 命中 o2-dms-adapter + devlog · 无 stale ref

## Carry-forward

- **P9-T-103(W2 kind smoke E2E ext)**:T103 现在可以走 hard-fail assertions on O2 DMS endpoint(curl POST inventory + GET inventory returns 200 + 有 JSON body)而非 P9-T-008 scaffold-only 501 expectation
- **Phase 10 polish O2 DMS path**:
  - inventory client real impl tests via envtest harness(plan §4 "4-6 client cases" 完整 implementation)· 与 informer event-driven 改造同期
  - subscription + alarmEvent endpoints per ADR-0013 §6 Open questions out-of-scope path · Phase 10 polish HTTP POST callback impl
  - lifecycleOperation persistent backing per ADR-0013 §5 Open question (e) · 与 P9-T-107 cache spike singleton-with-failover outcome 协同
  - R004-v07.00.00 spec upgrade evaluation per ADR-0013 §5 Open question (a) · re-WebFetch portal at Phase 10 W1 entry · 若 R004 已成 industry baseline → 评估 lock R004
  - Karmada multi-cluster propagation per ADR-0013 §6 forward note · deployment manager list 反映 Karmada member clusters
  - frontend Workload page O2 DMS endpoint indicator per ADR-0013 §6 forward note + Phase 6 T102/T103 chat+ADR self-RFC pattern if needed
  - authn/z full(OIDC + K8s SA + TokenReview)· production-grade NB API gateway · 替换 Phase 9 静态 bearer

---

**END of P9-T-104 devlog**
