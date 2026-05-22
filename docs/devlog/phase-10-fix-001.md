# P10-fix-001 · kind smoke ResourceSlice schema compat for K8s 1.34 DRA GA(v1 flat attributes vs v1beta1 nested basic.attributes)

- **Commit**: `7203587`(landed 2026-05-21 17:37 UTC)
- **Date**: 2026-05-21
- **Duration**: ~0.15d(diagnostic + jq path update + push)

## Trigger

Post-tag CI gate(per `feedback_post_tag_ci_gate.md`)on `phase-10-complete` tag `04cc009` · e2e-kind workflow run `26241945426` ❌ failure at step #11 `assert ResourceSlice publication (P4-T-104)`:

```
##[error]no device carries the npu.huawei.com/index attribute
##[error]sample device: {"attributes":{"npu.huawei.com/ai_cores":{"int":0},
  "npu.huawei.com/hccs_ring":{"int":0},"npu.huawei.com/health":{"string":"Healthy"},
  "npu.huawei.com/index":{"int":0},"npu.huawei.com/numa_node":{"int":0},
  "npu.huawei.com/slice_strategy":{"string":"FixedTemplate"}},"capacity":{...}}
```

Sample 设备 DOES carry `npu.huawei.com/index` attribute(value `{"int":0}`)· 但 assert 报"no device carries" · 路径不匹配。

## Root cause

K8s 1.34 DRA GA(per P10-T-003 三件套 part 1 baseline bump 1.32 → 1.34.3)promotes ResourceSlice schema to `resource.k8s.io/v1` · flattens `BasicDevice` intermediate:
- **v1beta1**(K8s 1.32 baseline pre-Phase 10):`device.basic.attributes[$attr]`(nested under `.basic`)
- **v1 GA**(K8s 1.34 baseline post-P10-T-003):`device.attributes[$attr]`(flattened · `.basic` 不存)

kubectl prefers v1(latest)when both served · 返回 flat representation · jq path `.basic.attributes[$attr]` 找 null · assert fail。

API discovery server line: `resourceslices  resource.k8s.io/v1  false  ResourceSlice` 仅 v1 listed(v1beta1 still served via conversion for typed Go clients · 但 kubectl default to v1)。

## Fix(2 scripts · schema-compat dual path)

`tests/e2e/kind/dra_publish_test.sh`(P4-T-104 assert · root failure):
```jq
- select(.basic.attributes[$attr] != null)
+ select((.attributes // .basic.attributes // {})[$attr] != null)
```

`tests/e2e/kind/phase7/assert.sh` T104-1 multi-ring topology jsonpath:
```bash
- '.basic.attributes.hccs_ring.int'
+ '.attributes.hccs_ring.int'  # v1 first
+ fallback to '.basic.attributes.hccs_ring.int'  # v1beta1 pre-1.34
```

Both paths handle:
- v1 GA cluster(post-P10-T-003):`.attributes[$attr]` 直接 hit
- v1beta1 cluster(pre-Phase 10 · 假设回滚)·`.basic.attributes[$attr]` 走 fallback path

## Why this scope minimal

- ✅ **Go code NOT changed**:npu-dra-driver / scheduler-plugin / pool-operator 用 typed `resourceapi "k8s.io/api/resource/v1beta1"` lister · K8s 1.34 仍 serve v1beta1(deprecated · 至少 1.35/1.36 仍可)· typed client 读 v1beta1 nested basic.attributes representation · runtime 不影响
- ✅ **ResourceSlice CRD NOT changed**:publisher emits via v1beta1 schema · API server converts to v1 storage transparently · no chart bundle drift
- ✅ **Phase 11+ chart packaging stream** 时 v1 → v1beta1 client migration 统一考虑(per ADR-0016 §3 真生产化 spine + client-go bump cohort)
- ❌ Alternative reject:bump Go code to v1 schema · cross 5 modules + lister regen + envtest update · large diff · 不适合 fix-NNN 范围

## Verification

- `git diff --stat 04cc009..7203587`:2 files · +15/-3 lines
- jq syntax check 实际(`.attributes // .basic.attributes // {}` 处理 null + non-null + 缺失键)
- Push 后 e2e-kind 7203587 step #11 PASS(确认 schema 修对)· 但 step #20 Phase 6 install fails(NumaAffinity wrap 真集群 issue · P10-fix-002 闭环)

## Carry-forward

- **P10-fix-002**:scheduler-plugin chart `numaAffinity.enabled` default false · NRT CRD bundling Phase 11+
- **Phase 11+**:Go code v1 schema migration cohort · client-go bump 统一
- **K8s 1.35/1.36 forward**:v1beta1 deprecated · 1.36+ 可能 remove · 全栈 v1 client migration prerequisite

## §0a.11 + memory adherence

- Per `feedback_post_tag_ci_gate.md` fix-NNN 命名 + 独立 commit + 独立 devlog(本文件)
- Per §0a.11 docs-only main-agent direct(test scripts schema compat · 无 Go code)
- Per `feedback_push_at_phase_tag_only` fix-NNN 推 dev 直接(同 P7-fix/P8-fix/P9-fix pattern)
