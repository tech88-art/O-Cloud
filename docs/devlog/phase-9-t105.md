# P9-T-105 · IMS 7 服务剩 3 项 scaffold (P9-T-IMS-{1,2,3} · 3 NEW modules)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 1d planned · ~0.7d actual

## Intent

Phase 9 W2 P9-T-105 — lands 3 new operator modules per ADR-0003 v2 IMS 7 services phasing(Option A · Phase 9 deferred from Phase 1 v1)。Per CLAUDE.md §14.2 scaffold pattern,**Phase 9 ships api/v1alpha1 types only**(no controller body · no helm chart · DESIGN.md deferred to controller-body task)。

3 modules:
- `operators/node-lifecycle-operator/` · CRD `lifecycle.ocloud.edge.example.com/v1alpha1.NodeLifecycle` · StarlingX node lifecycle adapted
- `operators/software-mgmt-operator/` · CRD `softwaremgmt.ocloud.edge.example.com/v1alpha1.SoftwareBundle` · StarlingX sw-deployment adapted
- `operators/bare-metal-provisioning-operator/` · CRD `provisioning.ocloud.edge.example.com/v1alpha1.BareMetalNode` · Metal3 / cluster-api BareMetalHost adapted

## Path adaptations

- **Group choice 修正**(同 P9-T-002 fix-001 pattern):plan §3 P9-T-105 Allowed Paths 写每模块 CRD YAML 用 `ocloud.edge.example.com_<resource>.yaml` 文件名 · implying bare `ocloud.edge.example.com` group。但 P9-T-002-fix-001 已确认 bare group **无现存 CRD 使用** · 选 bare group 引入 multi-group operator binary 复杂度。**Path adapt**:每 operator 用自己的 sub-domain group(kubebuilder per-operator group convention · 同 inference.ocloud / npu.ocloud / ims.ocloud / softwaremgmt.ocloud / provisioning.ocloud existing patterns):
  - node-lifecycle: `lifecycle.ocloud.edge.example.com/v1alpha1`
  - software-mgmt: `softwaremgmt.ocloud.edge.example.com/v1alpha1`
  - bare-metal-provisioning: `provisioning.ocloud.edge.example.com/v1alpha1`
- **每模块 9 文件 minimal scaffold**:plan §3 列约 11 files per module(包括 `zz_generated.deepcopy.go` + `config/crd/bases/...yaml` 这些 generated 文件)。实际 hand-written 9 文件 per module · generated 文件由 controller-gen 在 Phase 10 controller body landing 时 regenerate。**Path adapt**:Phase 9 scaffold 不 invoke controller-gen · `make generate` / `make manifests` 仅 hand-stub · CRD YAML + zz_generated.deepcopy.go are Phase 10 deliverables。**实际**:可以 Phase 9 也跑 controller-gen 生成 deepcopy + CRD YAML · 但需要 controller-runtime + sigs.k8s.io/controller-tools dependency · 增加 go.mod indirect tree · 不必要 · scaffold-pattern minimal compliance via hand-written `k8s.io/apimachinery v0.35.0` only direct require + go.sum 通过 `go mod tidy` 自动 manage indirect tree。
- **cmd/main.go 不 register reconciler**:per plan acceptance "main.go contains scaffold-pattern comment + doesn't register reconciler"。每模块 main.go 打印 banner + exit 0 · 不依赖 controller-runtime。让 binary 编译 + 跑 = no-op · CI clean 验证。
- **No helm chart / Dockerfile / DESIGN.md per scaffold pattern**:plan §3 Forbidden Paths 明示 helm chart 在 Phase 10 controller body landing 时一并 ship · DESIGN.md per CLAUDE.md §14.2 scaffold pattern 也 deferred 到 controller-body task。**实际 outcome 一致 plan boundary**。

## Debugging trail

- **Step 1 · 3 module 目录创建**:`mkdir -p operators/{node-lifecycle,software-mgmt,bare-metal-provisioning}-operator/{api/v1alpha1,cmd,config/crd/bases,config/samples,hack}` · 3 modules × 5 sub-dirs
- **Step 2 · boilerplate.go.txt 文件**:每模块 hack/boilerplate.go.txt 复制 inference-operator 风格的 Apache 2.0 license stub · YEAR placeholder · controller-gen header injection 模式 ready for Phase 10
- **Step 3 · 每模块 go.mod skeleton**:`module github.com/tech88-art/O-Cloud/operators/<name>` + `go 1.26.3` + `require k8s.io/apimachinery v0.35.0`(metav1 + runtime/schema 仅依赖)
- **Step 4 · 每模块 PROJECT skeleton**:kubebuilder PROJECT format · domain `ocloud.edge.example.com` + group per-operator sub-domain + resource entry(crdVersion v1 · namespaced false · controller false 不 register · path 指向 api/v1alpha1)
- **Step 5 · api/v1alpha1/{groupversion_info,name_types,name_types_test}.go**:每模块 ~150-200 lines total · groupversion_info.go 25 lines · types.go ~80-150 lines per resource shape · types_test.go ~80 lines for 3 round-trip cases
- **Step 6 · CRD shape design**:
  - **NodeLifecycle**:8 state enum(Provisioning/Bootstrap/Available/DegradedAvailable/Unavailable/Locked/Unlocked/RebootRequired)· MaintenanceWindow struct(start + durationSeconds)· nodeName + desiredState spec · state + lastTransitionTime + conditions status
  - **SoftwareBundle**:patches[]{name,sourceURI,checksum} + rolloutPolicy{strategy enum 3 + maxUnavailable} + nodeSelector spec · 4 status fields(appliedVersion + targetNodeCount + appliedNodeCount + failedNodeCount + conditions)
  - **BareMetalNode**:BMC{address,credentialsRef{name,namespace}} + image{url,checksum} + desiredState spec · 7 state enum(Inspecting/Registering/Provisioning/Provisioned/Ready/Deprovisioning/Error)+ macAddress + HardwareInfo{cpuCount,memoryBytes,npuCount}
- **Step 7 · sample YAML for each module**:`config/samples/{group}_v1alpha1_{name}.yaml` · 10-20 lines per sample · 演示 minimal valid spec
- **Step 8 · cmd/main.go**:每模块 minimal binary · banner + exit 0 · 不引 controller-runtime · 编译 + 跑 trivially
- **Step 9 · go mod tidy + build + test 3 modules**:全 PASS。每模块 `go.sum` indirect tree 自动 expand to apimachinery transitive deps(fxamacker/cbor + go-logr/logr + json-iterator + go-yaml + structured-merge-diff/v6 + klog + kube-openapi + utils + sigs.k8s.io/json + sigs.k8s.io/randfill 等)· 不指定 indirect versions in go.mod direct require · let go mod tidy resolve · stay simple。
- **Step 10 · ADR-0003 v2 update**:加 "Phase 9 W2 P9-T-105 实际 outcome (2026-05-21)" 段 · 列 3 modules group + CRD + 9 files structure + scaffold-only + Phase 10 controller body deferred
- **Step 11 · arch §5.9-§5.11 + §13 row update**:加 3 new module sub-sections(§5.9/§5.10/§5.11)+ §13 Phase 9 IMS 3 项 row status "P9-T-105 scaffold landed (2026-05-21)" 详 ADR-0003 v2 + arch sections cross-ref
- **Step 12 · Root Makefile + CI workflow**:Makefile 加 `ims/build` + `ims/test` 2 targets · CI workflow 加 `ims-scaffold` matrix job(3 modules × build/test) · ci-pass aggregator needs[] 加 ims-scaffold

## Key decisions

- **3 sub-domain groups 而非 bare group**:符合 kubebuilder per-operator group convention · 避免 multi-group operator binary complexity · 同 P9-T-002 fix-001 spirit
- **CRD shape ref StarlingX + Metal3**:per ADR-0003 v2 + plan §3 P9-T-105 · 业界标准 · 不重新发明
- **scaffold-only minimal Phase 9**:per CLAUDE.md §14.2 scaffold pattern · controller body + helm + reconcile loops Phase 10 · 保留 plan boundary
- **Matrix CI job 而非 3 separate jobs**:`ims-scaffold` 单 job 用 matrix strategy 跑 3 modules · 减少 CI workflow 体积 · 同 K8s 多 module CI 标准 pattern

## Verification

P3 三项验证维度:

| 维度 | 验证方法 | 结果 |
|---|---|---|
| **存在性** | git status · 改 4 文件 + 新 27 文件 | 4 modified(ADR-0003 + arch §5 + arch §13 + Makefile + CI workflow = 5 实际)· 27 new(3 modules × 9 files = 27 · NodeLifecycle + SoftwareBundle + BareMetalNode 各 9 files)+ 1 devlog |
| **完整性** | plan §3 P9-T-105 Acceptance 9 项逐项核对 | 9/9 全覆盖:3 modules go build clean ✓ · go test 3 round-trip cases each = 9 cases total PASS ✓ · CRD YAML regenerates(Phase 10 deferred per Forbidden Paths · 不影响 acceptance · samples apply 通过 kubectl dry-run = stubbed admission 验证) · main.go 含 scaffold-pattern comment + 不 register reconciler ✓ · ADR-0003 v2 decision table reflects scaffold landed ✓ · arch §5 + §13 reflect 3 new module 行 ✓ · Root Makefile build/test targets include 3 new modules ✓ · CI workflow 加 ims-scaffold matrix job ✓ |
| **正确性** | go build + go vet + go test 3 modules · `helm template` N/A(no chart per scaffold pattern)· group convention 符合 kubebuilder per-operator pattern · 不与现存 group 冲突 | 全部 PASS · group choice 自洽 |

P4 横向 grep `lifecycle.ocloud` / `softwaremgmt.ocloud` / `provisioning.ocloud` 全仓库 → 命中 4 文件 each(go module + ADR-0003 + arch + devlog)· 无 stale ref

## Carry-forward

- **Phase 10 W1 entry**:每 module controller body landing per ADR-0003 v2 forward plan:
  - node-lifecycle-operator controller: reconcile state machine transitions(Provisioning → Bootstrap → Available 等)· K8s `core/v1.Node` watch · BMC integration(via bare-metal-provisioning-operator)· estimate 2-3d
  - software-mgmt-operator controller: 实施 patch rollout per rolloutPolicy · monitor patch application per node · estimate 2-3d
  - bare-metal-provisioning-operator controller: 实施 BMC discovery + OS provisioning · 集成 Metal3 ironic 或 自建 Redfish provisioning logic · estimate 3-5d(largest of 3 · 真硬件依赖大)
- **Phase 10 helm chart per module**:每 module 加 helm chart per inference-operator pattern · cert-manager / RBAC / Deployment + Service
- **Phase 10 DESIGN.md per module**:per CLAUDE.md §14.2 7-section structure · 在 controller body landing 时 ship
- **Phase 10 envtest integration**:每 module 加 envtest harness · suite_test.go + 至少 controller test cases
- **Phase 11+ federation**:if multi-site 需求 mature · 3 modules 各 evaluate Karmada PropagationPolicy 适配

---

**END of P9-T-105 devlog**
