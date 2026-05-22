# P11-fix-004 · Phase 11 post-tag CI gate fix · Frontend test param expectation + scheduler-plugin NRT RBAC

- **Date**: 2026-05-22
- **Duration**: ~0.2d(2 fix in 1 commit)
- **Trigger**: phase-11-complete tag push + PR `fix/p11-fix-001-dev-stack` → `dev` 后 CI gate red 3 jobs:
  - **CI Pass** aggregator fail(reflects underlying)
  - **Frontend (test)** `tests/Workloads.test.tsx` "filter passthrough" 1 fail
  - **kind smoke (real cluster)** "Phase 6 — install scheduler-plugin chart" `Error: context deadline exceeded`

## Fix 1 · Frontend test param expectation

**Root cause**: P11-T-105(commit e6b9de4)在 `pages/Workloads/index.tsx` 加 3 includes default opt-in(`includeO2DMSExposed/QuotaUsage/ScaleHistory: true`)· `tests/Workloads.test.tsx` line 308 + 327 仍只 assert `{ includeSliceBindings: true }`(P6-T-103 baseline)· 测试不知道 P11-T-105 新增 3 params。

**Fix**: 更新 2 处 `toHaveBeenCalledWith` assert · 加 3 new params · cite P6-T-103 + P11-T-105 双 source。

**Verify**: `npx vitest run tests/Workloads.test.tsx` → **11 tests passed** · 含 `filter passthrough > passes the chosen status to the server as ?status=` 现 pass。

## Fix 2 · scheduler-plugin NRT RBAC ClusterRole rules

**Root cause**: P11-T-007(commit 7e8f026)flipped `numaAffinity.enabled: true` default + bundled NRT CRD in `crds/noderesourcetopologies.yaml`(Approach B vendored)· 但 chart `templates/rbac.yaml` ClusterRole 缺 `topology.node.k8s.io.noderesourcetopologies` get/list/watch verbs · scheduler-plugin Pod 启动时 `nrt.New` 调 NRT informer · SA 拿不到 NRT 读权限 → 403 forbidden → Pod 永远 NotReady → `helm install --wait --timeout=180s` 命中 `context deadline exceeded` → e2e-kind CI red。

**Fix**: 在 `deploy/helm-charts/scheduler-plugin/templates/rbac.yaml` $fullname-ocloud ClusterRole rules 末尾加 conditional NRT verbs · `{{- if .Values.numaAffinity.enabled }}` gated · 默认 true 时启 · 不破 P11-T-007 default true 决策:

```yaml
{{- if .Values.numaAffinity.enabled }}
# P11-fix-004 ...
- apiGroups: ["topology.node.k8s.io"]
  resources: ["noderesourcetopologies"]
  verbs: ["get", "list", "watch"]
{{- end }}
```

**Verify**:
- `helm lint --strict deploy/helm-charts/scheduler-plugin/` clean
- `helm template scheduler-plugin deploy/helm-charts/scheduler-plugin/` → NRT verbs render(default enabled)
- `helm template ... --set numaAffinity.enabled=false` → 0 occurrences NRT verbs(conditional gate verified)

## P7 自我审计(本 P11 post-tag CI gate 教训)

P11-T-007 本来 ship 的设计是"NRT CRD bundle + default true"· 但只 ship 了 CRD(crds/folder)· 没 ship 配套 RBAC(SA 的 NRT verb)· 这是 P3 verify-before-claim 不彻底的体现 — 我应该在 ship P11-T-007 时 grep chart RBAC + 验证 SA verb 集合是否 cover NRT。教训复用 P10-fix-002 同样的 false-claim 模式:nrt.New panic 在 helm install 时才 surface · 不在 `helm lint` 或 `helm template` 阶段 surface · CI gate 是唯一真 verification path · 同 memory `feedback_post_tag_ci_gate.md` "tag push 后必看 CI" 警示重申。

## Validation

- frontend `tests/Workloads.test.tsx` 11/11 PASS(本地)
- scheduler-plugin `helm lint --strict` clean
- scheduler-plugin `helm template` NRT RBAC render correct(default true · false 时 conditional skip)
- 真 kind smoke 验证留 push 后 CI gate

## Refs

- ADR-0017 §2 Decision D 5th 优先级 P11-T-007 carry close intent
- ADR-0010 §3 known-issues #12 完整 close 循环(本 fix 把 P11-T-007 真正 close)
- memory `feedback_post_tag_ci_gate.md` Phase 7 实战 4 fixes pattern
- commit chain: P11-T-007 `7e8f026` + P11-T-105 `e6b9de4` → 本 P11-fix-004 close gap
