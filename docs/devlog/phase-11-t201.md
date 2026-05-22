# P11-T-201 · master-demo-multi-site.sh — 真 multi-cluster / multi-site demo extension

- **Commit**: (this commit)
- **Date**: 2026-05-22
- **Duration**: 1-1.5d plan / ~0.3d actual

## Intent

ADR-0017 §2 Decision A 主线 2 Karmada propagation 第一波 production-grade 实质 demo · 把 Phase 11 W1+W2 substrate(chart packaging + Karmada bootstrap + PropagationPolicy + ClusterQuota + demo-backend Lease singleton)串成端到端 multi-site flow。

## Path adaptations

1. **不修改 Phase 10 `tests/e2e/kind/master-demo.sh`**(single-cluster synthetic ring 路径仍用)· 新 ship `master-demo-multi-site.sh` 作为 Phase 11 sibling · 不破 Phase 10 demo
2. **kind binary 缺时 graceful skip exit 0**:dev env 没装 kind · script 不 fail · WARN 后 exit 0 · 真 demo 在 CI / 装了 kind 的 dev box 上跑
3. **`SKIP_KARMADA_BOOTSTRAP=1` + `SKIP_CHART_INSTALL=1` env flags**:operators 重复跑 demo 时不必每次 kind create cluster + chart install · iterate-friendly

## Debugging trail

无 build/test fail · bash -n syntax check 通。

## Key decisions

- **7-step orchestrated demo flow**(per plan §4 P11-T-201 acceptance):
  1. Karmada bootstrap(调 deploy/karmada/install.sh)
  2. Chart packaging spine install on host(7 charts:demo-backend + 3 IMS + scheduler-plugin + inference-operator + o2-dms-adapter)
  3. Apply 4 PropagationPolicy templates(deploy/karmada/policies/)
  4. Verify cross-cluster propagation(karmadactl get clusters)
  5. ClusterQuota cross-cluster aggregation demo(apply ClusterQuota + verify member propagation)
  6. demo-backend Lease failover simulation(identify leader + non-destructive verification hint)
  7. Path P / Path F outcome stamp(per LAB_AVAILABLE env)
- **`LAB_AVAILABLE=1` enables Path P(real hardware)**:per ADR-0017 §2 Decision C trigger 2 ad-hoc lab signal · 不 reset default policy(per-instance only)· 与 ADR-0016 §2 Decision C 模式一致
- **Path F default**:synthetic ring fallback · per ADR-0016 §2 Decision C · 80% landed deliverable · 与 Phase 10 master-demo.sh fallback pattern align
- **Step 6 demo-backend Lease failover 仅 identify leader · 不真 delete**:non-interactive mode 安全 · operators 在 dev session 手动 `kubectl delete pod <leader>` 触发 failover · 减少 demo 副作用
- **ClusterQuota example inline in script**:不依赖外部 fixture file · script 自包含 · 易于 演示 + iterate

## Verification

- `bash -n tests/e2e/kind/master-demo-multi-site.sh` syntax OK
- script step 划分清晰 · 与 P11-T-102/T103/T104/T003/T004/T005/T006/T007/T008/T106 task chain 一一对应 cross-ref
- `--`Skipping deletion in non-interactive mode`-` Lease step 6 安全 stop · operators 手动 delete-pod 演示 failover

## Carry-forward

- **真 kind cluster 跑 demo 实测**:在装了 kind + karmadactl 的环境跑全 7 step · 验证 propagation latency + failover latency · 入 P11-T-204 checkpoint stamp
- **Phase 12+ HA + real multi-region**:本 script topology 是 1 host + 2 member kind cluster 单 Docker daemon 模拟 multi-site · 真 multi-region 留 ADR-0018 §4 (a) Phase 12+
- **Phase 12+ E2E Playwright**:本 script 是 bash-orchestrated server-side · UI 端 E2E(Workload page 3 indicator 真显示)留 Playwright cohort

## §0a 续 autonomous · 继续 T202 DECISION-GATED Partitionable Devices(default deferred)
