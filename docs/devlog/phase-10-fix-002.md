# P10-fix-002 · scheduler-plugin chart `numaAffinity.enabled` default true → false · NRT CRD bundling deferred Phase 11+(unblock phase6 install in CI gate)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: 0.2d(diagnostics + chart revert + assert script update)

## Intent

Post-tag CI gate failure on `7203587` (P10-fix-001) `e2e-kind` step #20 "Phase 6 — install scheduler-plugin chart" · `Error: context deadline exceeded` after 3 min · pod `npu-scheduler-scheduler-plugin-*` in Error state with 5 restarts。

## Root cause

P10-T-005 wrap delegation `nrt.New(ctx, args, h)` calls upstream
`initNodeTopologyInformer(ctx, lh, tcfg, handle)` which builds a clientset for
`NodeResourceTopology` CRDs and starts an informer. Our scheduler-plugin chart
does NOT bundle `noderesourcetopology-api` CRDs(Phase 10 P10-T-005 scope
adaptation deferred chart-packaging items)。Without NRT CRDs installed in
cluster, `nrt.New` returns error · scheduler binary panics at plugin
registration → CrashLoopBackOff → helm install timeout。

Default `numaAffinity.enabled: true` at P10-T-005 made every Phase 6 chart
install attempt fail post-1.34-baseline。

## Fix

1. `deploy/helm-charts/scheduler-plugin/values.yaml`:revert
   `numaAffinity.enabled: true` → `false`。Comment expanded to explain
   operator-opt-in path(install NRT CRDs first · then flip)。
2. `tests/e2e/kind/phase10/assert.sh` T107-A1:assert chart `numaAffinity` 
   substrate present(not "enabled in ConfigMap"). Substrate ready in chart
   values.yaml is the verified delivery · runtime enable is operator path。
3. `tests/e2e/kind/phase10/install.sh` comments updated:reflect default
   disabled state · upgrade is no-op + idempotent。

## Why this is right path vs alternatives

- ❌ **Alternative A**:Bundle NRT CRDs into scheduler-plugin chart(substantial
  · involves upstream `noderesourcetopology-api` chart subdependency or
  vendored CRD YAML · Phase 11+ chart packaging stream proper home per
  ADR-0016 §3 真生产化 spine)。
- ❌ **Alternative B**:Replace `nrt.New` with our own fault-tolerant Reconcile
  that handles missing NRT CRDs gracefully(违 "wrap, don't fork" 原则)。
- ✅ **Chosen** :revert chart default · keep wrap substrate · document
  operator-opt-in path · 不阻塞 CI gate · Phase 11+ chart packaging 真 wire
  时 enable by default again。

## Verification

- ✅ `helm lint --strict deploy/helm-charts/scheduler-plugin/` clean
- ✅ Wrap substrate(`internal/plugins/numa/plugin.go` + `plugin_test.go`)不动
  · 4 sanity tests preserved · build/vet/test 全 clean
- ✅ ADR-0010 §1 + §3 update segments + known-issues #12 RESOLVED 全保留
  ·substrate landed 论断仍然成立
- ✅ phase10/assert.sh T107-A1 不再 require runtime enable · 验证 substrate
  present 即可

## Carry-forward

- **Phase 11+ chart packaging stream**(per ADR-0016 §3 真生产化 spine):
  bundle NRT CRDs into scheduler-plugin chart(or as subchart) · flip
  `numaAffinity.enabled: true` default back · phase10/assert.sh 可加 runtime
  enable verify
- **CI gate next**:push P10-fix-002 · 等 e2e-kind 04cc009-fix-002 next run ·
  scheduler-plugin pod should start successfully(NumaAffinity not registered)
  · helm install completes · step #20 PASS · 继续后续 phases(downstream
  failures still possible · per c25627c baseline step #18 slice-bindings
  annotation flake)
