# P7-fix-004 · post-tag CI fix #4: Phase 7 install.sh namespace + ConfigMap name corrections

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: ~15 min (CI log triage · root cause via job log + helm template · 1-file fix)

## Intent

P7-fix-003(a5d0eec)修了 chart RBAC for NPUSliceTemplate · npu-dra-driver controller manager cache sync 通 · Phase 5 step 18 终于 PASS。进度推到 Phase 7 step 22 自己 fail:

```
== Phase 7 T104: reseed npu-dra-driver mock data with set-b-multi-ring ==
configmap/npu-dra-mock created
== restart npu-dra-driver to reload mock data ==
Error from server (NotFound): deployments.apps "npu-dra-driver" not found
```

Root cause:**Phase 7 install.sh `cmd_reseed_mockdata` 我写成 `-n kube-system` + ConfigMap name `npu-dra-mock`** · 实际:
- npu-dra-driver Deployment 在 `ocloud-system`(per `tests/e2e/kind/install.sh` NS=ocloud-system)
- 该 chart's mockConfigMap 名是 `<release>-mock` = `npu-dra-driver-mock`(per `_helpers.tpl::mockConfigMapName`)

`kubectl -n kube-system rollout restart deploy/npu-dra-driver` 当然 NotFound · 因 Deployment 不在 kube-system。

附加 silent bug:即使 namespace 修了 · ConfigMap 名 `npu-dra-mock` ≠ chart 实际 `npu-dra-driver-mock` · Pod volumeMount 不会 reload 错名 ConfigMap · reseed 静默无效。

## Path adaptations + fix

Single-file change: `tests/e2e/kind/phase7/install.sh`
- 加 `NS_DRA="${NS_DRA:-ocloud-system}"` env var(默认 ocloud-system · 可 override · 与 tests/e2e/kind/install.sh NS pattern 一致)
- 加 `DRA_MOCK_CM="${DRA_MOCK_CM:-npu-dra-driver-mock}"` env var(默认匹配 mockConfigMapName helper output for default release-name=npu-dra-driver)
- `cmd_reseed_mockdata`:`kubectl -n kube-system create configmap npu-dra-mock` → `kubectl -n "${NS_DRA}" create configmap "${DRA_MOCK_CM}"`
- `cmd_reseed_mockdata`:`kubectl -n kube-system rollout restart/status deploy/npu-dra-driver` → `kubectl -n "${NS_DRA}" rollout ...`
- inline 注释解释 NS_DRA + DRA_MOCK_CM rationale + 前次 bug 现象("kube-system' 错误来源" + "ConfigMap 名错被 silently ignored")

## Debugging trail

- a5d0eec e2e-kind 失败 at Phase 7 step 22(不是之前的 Phase 5 step 18)
- Phase 5/6 都 PASS — P7-fix-001/002/003 全部生效
- Job logs 3469 行 · grep "Phase 7|reseed|::error" 立即定位错误信息
- Read Phase 7 install.sh + chart's _helpers.tpl + `helm template` 确认实际命名:configmap=`npu-dra-driver-mock` in `ocloud-system`
- 验证:`helm template` 输出含 `kind: ConfigMap`, `name: npu-dra-driver-mock`, `namespace: ocloud-system` ✅

## Key decisions

- **env var(NS_DRA + DRA_MOCK_CM)而非 hard-coded string**:让 future operator 安装到不同 namespace / 不同 release name 时不需改 install.sh · 与 tests/e2e/kind/install.sh 既有 NS / NS_INF / MON_NS env-var pattern 一致
- **保留 phase7 install.sh 现有 NS_INF 不重命名为 NS_DRA**:NS_INF 用于 inference-operator pod / fixture namespace(`apply_modelservice` 调用)· NS_DRA 是 npu-dra-driver chart 的 release namespace · 两个语义不同 · 即使都默认 ocloud-system 也应分开 env var
- **inline 注释明示 silent failure mode**(错 ConfigMap 名 → Pod 仍 mount 原 ConfigMap):防止下次有人改了 chart release name 或 mockConfigMap 命名约定 · 但忘了同步 install.sh

## Verification

- 存在性:
  - `tests/e2e/kind/phase7/install.sh` 加 NS_DRA + DRA_MOCK_CM env vars(顶部 5 行注释)+ cmd_reseed_mockdata 4 处 kubectl 改用变量(2 在 configmap create · 2 在 rollout restart/status)✅
- 完整性(verified by execution):
  - `bash -n install.sh` syntax OK ✅
  - `helm template npu-dra-driver deploy/helm-charts/npu-dra-driver/ --namespace ocloud-system | grep -E "kind: ConfigMap|name: npu-dra-driver-mock|namespace: ocloud-system"` 确认 chart 实际命名 ✅
- 正确性:
  - 与 tests/e2e/kind/install.sh NS=ocloud-system + helm install release name=`npu-dra-driver` 一致
  - default value 不依赖 caller 必须 set env var(Phase 7 CI 现状不 set)
  - env var override 路径保留(future caller 可 `NS_DRA=other-namespace bash install.sh all`)

## Carry-forward

- **CI 验证**:next workflow run on this commit · Phase 7 step 22 `cmd_reseed_mockdata` 应成功 reseed + rollout 重启 npu-dra-driver Pod · Phase 7 余下 assert steps(NPUSliceTemplate Validated + schedulerName auto-stamp + multi-ring ResourceSlice 4 rings)应跑通
- **Phase 7 install.sh / assert.sh 其他可能 stale 引用**:本 fix 只解 cmd_reseed_mockdata · cmd_apply_template + cmd_apply_modelservice + assert.sh 可能也有 namespace 假设。若 CI 再 fail · 顺序排查。但这两个使用的是 NS_INF=ocloud-system + 集群范围(`kubectl apply -f npust ...`)· 估计没有同类问题
- **Phase 8 polish 候选**:`bash -n` syntax check 已经在 install.sh 静态检查范围内 · 但 dynamic semantic(namespace mismatch / ConfigMap name mismatch)只有真 cluster 才暴露。Phase 8 可加 dry-run mode 让 install.sh 跑 `kubectl --dry-run=client` 验 manifest 行为
- **chart-as-source-of-truth audit pattern**:本 fix 是第 4 个 chart-相关 bug(P7-fix-002 = CRD bundle · P7-fix-003 = RBAC · P7-fix-004 = install.sh namespace) · 累计 strong signal that Phase 7 W2 任务 chart-side audit 不足。Phase 8 baseline-bump task 应包含一个完整 chart audit pass + CI rule 防 drift
