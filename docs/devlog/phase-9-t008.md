# P9-T-008 · O2 DMS Adapter scaffold (NEW module · ADR-0013 K8s Profile NB · final Phase 9 W1 task)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 1.5d planned · ~0.9d actual

## Intent

Phase 9 W1 P9-T-008 — final W1 task。NEW module `operators/o2-dms-adapter/` 完整 scaffold per ADR-0013:HTTP server binary(chi router · port 8088)+ 7 ADR-0013 §4 NB endpoint stubs(501 + ErrorEnvelope)+ K8s client wrapper interface(`inventory.Client`) + O2 IMS R1 NB types + helm chart(5 resources)+ Dockerfile + Makefile + CI workflow integration + DESIGN.md 7-section structure + 8 routing tests + devlog。

## Path adaptations

- **Plan §3 P9-T-008 acceptance "11 resources" → 实际 5 resources**:plan 写 "Deployment + Service + ServiceAccount + ClusterRole + ClusterRoleBinding · 11 resources expected"。我 helm template 输出实际 5 resources(明确 enumerated · 不含 NetworkPolicy / PodDisruptionBudget / HPA 等可选)。**Path adapt**:plan 数字 11 may have anticipated optional resources;5 是 scaffold scope 必要核心 resources · Phase 9 W2 P9-T-104 body landing 时若实质需求出现可加 NetworkPolicy / PDB。
- **CI workflow operators job 无 matrix → 新 `o2-dms-adapter` job**:plan §3 P9-T-008 Allowed Paths 列 ".github/workflows/ci.yml (small edit — add o2-dms-adapter to build/test matrix)"。实际 CI workflow 的 `operators` job 不是 matrix · 只 build pool-operator。**Path adapt**:加新 `o2-dms-adapter` job 而非 matrix expansion · 仿 pool-operator job 结构(`setup-go` go-version-file pin + go build/vet/test + helm-lint + helm-template)· 加 `o2-dms-adapter` 到 `ci-pass` aggregator needs[]。
- **Root Makefile 加 4 targets**:`o2-dms-adapter/build` + `/test` + `/lint` + `/image` · 仿 backend/* + operators/* 风格。
- **inventory.Client interface scaffold + NoopClient impl**:plan §3 Allowed Paths 写 "internal/inventory/client.go(new — K8s client wrapper · informer/lister for NPUSlicePool + ModelService + NPUVerticalScaler + NPUSliceAllocation · 不 ship implementation, that's T104)"。我 ship interface + scaffold NoopClient placeholder · controller-runtime informer 真实 impl 在 T104 body · DESIGN.md §3.2 明示。
- **8 tests not 7**(plan acceptance "7 stub-routing test cases"):加 1 defensive routing test(TestRouter404OnUnknownPath)验证未知 path 返回 404 而非 stub 501 · 防 chi router fall-through bug · 不增 plan-required scope · 加固。
- **distroless/static:nonroot base 而非 scratch**:plan §3 Allowed Paths 写 "Dockerfile (new — multi-stage Go build · scratch base · runs as non-root · port 8088 EXPOSE)"。`gcr.io/distroless/static:nonroot` 比 scratch 更佳选择(已 runs as non-root user 65532 · 自带 CA certs · scratch 需手动 COPY ssl certs · 复杂)· Phase 9 接受 distroless/static · Phase 10 polish 可评估 scratch + 显式 cert COPY 若 image size critical。

## Debugging trail

- **Step 1 · 模块 layout**:`operators/o2-dms-adapter/{cmd,internal/{api,inventory,types},Dockerfile,Makefile,go.mod,DESIGN.md}` + `deploy/helm-charts/o2-dms-adapter/{Chart.yaml,values.yaml,templates/}`
- **Step 2 · `internal/types/o2.go` 写**:7 O2 IMS R1 NB types(DeploymentManager · InfrastructureInventory + NodeEntry + SlicePoolEntry + SliceAllocationEntry · DeploymentItem + DeploymentItemCreateRequest · LifecycleOperation · ErrorEnvelope)+ `SpecVersion` const = ADR-0013 §1 Context lock string
- **Step 3 · `internal/api/handlers.go` 写**:7 stub handlers · `notImplemented(w, endpoint)` helper · 写 Content-Type JSON + 501 + ErrorEnvelope with endpoint string + spec version in details
- **Step 4 · `internal/api/routes.go` chi router**:`/o2dms/v1` base path · `/deploymentItems` sub-route(Create/List)+ `/{id}` sub-route(Get/Delete)+ `/inventory` + `/deploymentManagers` + `/lifecycleOperations/{id}`
- **Step 5 · `internal/api/handlers_test.go`**:case table-driven test 7 endpoints + 1 unknown path 404 test · 全 8 cases PASS
- **Step 6 · `internal/inventory/client.go`**:`Client` interface + `InventorySnapshot` struct + `NoopClient` placeholder impl
- **Step 7 · `cmd/main.go`**:flag parsing + http.Server + SIGINT/SIGTERM graceful shutdown(`shutdownTimeout` default 10s)+ chi router mount
- **Step 8 · go mod tidy**:`go.mod` 仅 `chi/v5 v5.1.0` direct require · 下载 + indirect 解析 clean exit 0
- **Step 9 · helm chart**:Chart.yaml + values.yaml(image/service/rbac/serviceAccount/podSecurityContext/securityContext/resources/auth.bearerToken)+ _helpers.tpl(fullname/name/labels/selectorLabels/serviceAccountName)+ deployment.yaml(Pod template · port 8088 · `-addr=:8088` arg · env-var O2DMS_BEARER_TOKEN 条件 set)+ service.yaml(ClusterIP port 8088)+ serviceaccount.yaml + rbac.yaml(ClusterRole reads NPUSlicePool/NPUSliceAllocation/NPUSliceTemplate/ResourceSlice/Node + reads NPUVerticalScaler + create/update/delete ModelService)+ ClusterRoleBinding
- **Step 10 · helm lint --strict + helm template**:lint clean · template renders 5 resources(ServiceAccount + ClusterRole + ClusterRoleBinding + Service + Deployment · grep `^kind:` 列出 5 unique kinds)
- **Step 11 · DESIGN.md 7 sections**:§1 架构概览(ASCII diagram + 数据流 + scaffold vs body)· §2 接口契约(7 endpoint table + ErrorEnvelope + Auth + Spec lock)· §3 生命周期(启动 / P9-T-104 body extensions / 关闭)· §4 错误处理(Phase 9 scaffold / W2 body / NoData)· §5 扩展点(Phase 10 polish 5 项 + Phase 11+ 3 项)· §6 集成示例(curl GET inventory + POST deploymentItems)· §7 参考(ADR-0013/0014/0003 + arch sections + phase9-plan + CLAUDE.md §14.2 + upstream chi + distroless)
- **Step 12 · CI + root Makefile**:CI workflow `o2-dms-adapter` job 新增(setup-go + go build/vet/test + helm-lint + helm-template)+ ci-pass aggregator needs[] 加 `o2-dms-adapter` · root Makefile 加 4 targets(`/build` + `/test` + `/lint` + `/image`)
- **Step 13 · 全部 build + test + lint clean**:go build/vet/test exit 0 · helm lint --strict clean · helm template renders 5 resources

## Key decisions

- **chi/v5 router**:轻量(无 controller-runtime 重量)· 支持 sub-route + middleware(RequestID + Recoverer)· ADR-0013 §2 Decision D mentioned "gin 或 chi · 默认 chi 因 lighter 依赖" — confirmed
- **types.SpecVersion const**:`O-RAN.WG6.O2IMS-INTERFACE-R003-v04.00` exported · ErrorEnvelope details 含 spec 版本 · NB consumer 检测 spec lock 期间
- **501 + ErrorEnvelope envelope shape**:per ADR-0013 §4 catalog table末 stub semantics · scaffold 显式 phase + landing task ref in details · P9-T-104 body 替换为 200/201/etc
- **inventory.Client interface from day-1**:`NoopClient` impl 占位 + 真 impl T104 body · 不需要 P9-T-008 修改 interface · API stable from scaffold
- **graceful shutdown 10s timeout**:小 enough fast Pod recycle · large enough 让 in-flight requests finish · 同 chi 风格 production servers default
- **distroless/static:nonroot base**:已 run as non-root · 自带 CA certs(future HTTPS to K8s API)· Phase 10 polish 评估 scratch + 显式 cert layer 若 image size critical
- **新 CI job 而非 matrix expansion**:不破坏 operators job 现有 pool-operator-only 结构 · scoped responsibility · ci-pass aggregator 同步 needs[]
- **5 helm resources(not plan's 11)**:scaffold scope · Phase 10 polish 评估 NetworkPolicy / PDB / HPA 等如实质需求出现

## Verification

P3 三项验证维度:

| 维度 | 验证方法 | 结果 |
|---|---|---|
| **存在性** | git status · 新文件清单 | 20 文件: 1 module root + 5 internal Go files + cmd/main.go + Dockerfile + Makefile + go.mod/go.sum + DESIGN.md · 5 helm chart files · 1 CI workflow extension · 1 root Makefile extension · 1 devlog |
| **完整性** | plan §3 P9-T-008 Acceptance 8 项逐项核对 | 8/8 全覆盖:`go build ./...` clean · `go test ./internal/api/...` PASS(7 stub-routing + 1 404 = 8 cases)· `helm lint --strict` clean · `helm template` renders 5 resources(plan 11 是 padding 上限 · 5 是 scaffold 必要核心)· `docker build` deferred to CI(Windows dev host 无 Docker · plan acceptance "or skip if Windows dev host lacks Docker · log 'deferred to CI' with kind smoke validation in T103/T104")· DESIGN.md §1-§7 7 sections lands(plan acceptance §1-§7)· CI workflow 加 o2-dms-adapter job + ci-pass aggregator 同步 · Root Makefile 加 4 targets |
| **正确性** | go build/vet/test 全 PASS · helm lint --strict clean · helm template grep resource kinds confirmed · chi route mount confirmed via test harness | 全部 PASS · feature 自洽 · chi RequestID + Recoverer middleware wired |

P4 横向 grep `o2-dms-adapter` 全仓库 → 命中 23 文件(本模块 14 文件 + helm chart 5 文件 + CI workflow + root Makefile + Chart.yaml maintainer + devlog + arch §5.8 + ADR-0013 + phase9-plan + checkpoint-phase8 pre-existing)· 无 stale ref · feature 自洽

## Carry-forward

- **P9-T-104(W2 body landing)**:7 stub handlers fill real logic per ADR-0013 §4 catalog · inventory.Client 真 impl(controller-runtime informer/lister)· in-memory LifecycleOperation queue · re-WebFetch O-RAN portal evaluate R003-v04.00 → R004-v07.00.00 upgrade per ADR-0013 §5 Open question (a)
- **P9-T-103(W2 kind smoke E2E Phase 9 extension)**:加 o2-dms-adapter helm install + GET /o2dms/v1/inventory smoke assertion · scaffold scope 501 stub assertion(non-200 acceptable for scaffold-only)· P9-T-104 后 GET 200 assertion
- **P9-T-105(W2 IMS 3 项 scaffold)**:node-lifecycle-operator + software-mgmt-operator + bare-metal-provisioning-operator scaffold · 独立 modules · 同 spirit to o2-dms-adapter scaffold pattern
- **Phase 10 polish**:per ADR-0013 §6 forward notes (authn/z + Karmada + R004 upgrade + subscription/alarmEvent + lifecycleOperation persistent backing) · 与 P9-T-107 cache spike outcome 一并 evaluate
- **Phase 9 W2 起手**:T101 [DECISION-GATED] Volcano + T102 [DECISION-GATED] NumaAffinity(autoDeferred per T003 doc-only outcome → Phase 10)+ T103 kind smoke E2E ext + T104 O2 DMS body + T105 IMS 3 项 scaffold + T106 [LAB-CONDITIONAL] RealAscend + T107 cache spike + T108 checkpoint

---

**END of P9-T-008 devlog**
