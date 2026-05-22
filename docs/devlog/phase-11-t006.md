# P11-T-006 · IMS-3 bare-metal-provisioning-operator chart + Redfish/IPMI stub + Secret 解析

- **Commit**: (this commit)
- **Date**: 2026-05-22
- **Duration**: 1.5-2d plan / ~0.5d actual

## Intent

ADR-0017 §2 Decision D 4th 优先级。比 IMS-1/2 多 2 项工作:
- BMC client interface + 两 stub impl(redfish.go + ipmi.go · in-memory PowerState)
- Secret 解析 username/password(reconciler.go `resolveCredentials`)+ address-scheme 分发(redfish:// vs ipmi://)

## Path adaptations

1. **Status field 名 `ProvisioningState` 而非 `State`**(实测 grep 后改正):IMS-1 用 `Status.State`(NodeLifecycleState type)· IMS-3 用 `Status.ProvisioningState`(ProvisioningState type · 因 BareMetal 有"硬件 power state"概念 · 与 K8s "node lifecycle state" 区分)。reconciler.go 5 处 `bmn.Status.State` 改为 `bmn.Status.ProvisioningState`。教训:P3 verify-before-claim · 各 IMS module schema 不必同。
2. **Secret credentials 默认 namespace = `ocloud-system`**(per CRD CredentialsReference.Namespace kubebuilder default · empty → `ocloud-system`)· reconciler.go `resolveCredentials` 显式 default fill。
3. **address scheme 分发**:`Spec.BMC.Address` 用 `redfish://` / `redfish+https://` / `ipmi://` 前缀分发到不同 stub。`buildBMC` strings.HasPrefix 判断 · 不识别返 `ErrUnsupportedBMCScheme`。

## Debugging trail

仅 1 编译错(Status.State 字段名错)· 上方 path adaptation #1 已修。其他 sed-copy chart files 一次成功。

## Key decisions

- **Phase 11 ship stub clients only**(redfish.go + ipmi.go · in-memory PowerState)· 真 Redfish HTTPS API call + `ipmitool` shell-out 留 Phase 12+(per plan §3 P11-T-006 acceptance "stubbed client interface · in-memory mock impl")
- **Secret 解析路径**:`r.resolveCredentials(ctx, ref)` 直读 Secret · 不缓存(每 Reconcile 一次 K8s API call · acceptable for 低频 BareMetalNode reconcile)· Phase 12+ 加 informer cache 减 K8s API load
- **BMC interface 4 方法**(`PowerStatus / PowerOn / PowerOff / Reset`)· 足够当前 ProvisioningState 7-state machine 转换 · Phase 12+ 加 BootDevice 切换 + Firmware update API
- **RBAC 替换 Node patch → Secret read**(IMS-1 控制器 patch Node;IMS-3 不动 Node · 只读 Secret)

## Verification

- `helm lint --strict` clean
- `helm template` 7 base kind render OK · Secret get/list/watch in ClusterRole rules
- `go build ./...` exit 0
- `go test -vet=off ./...` ok 5 packages(api/v1alpha1 + cmd noop + internal/client noop + internal/controller + internal/state · 含 Phase 10 T101 已有 17 unit tests)
- kind smoke deferred(T107 phase11/ folder)

## Carry-forward

- **P11-T-201** real BMC fixture in master-demo-multi-site.sh: stub 客户端的 PowerState transitions可被 master demo 模拟成 "BMC reach"+"hardware reboot cycle" sequence · 真 BMC integration 留 Phase 12+
- **Phase 12+** 真 Redfish HTTPS client:Stoian/gofish library or net/http + Bearer token · 替换 redfish.go stub · interface 不变
- **Phase 12+** 真 IPMI client:`exec.Command("ipmitool", ...)` 或 vmware/goipmi 库 · 替换 ipmi.go stub
- **Phase 12+** Vault Secret 路径(per ADR-0018 §4 (c)):BMC credentials 从 Vault 动态拉取 · 替代 K8s Secret

## §0a 续 autonomous mode · 继续 T007(scheduler-plugin NRT bundle)
