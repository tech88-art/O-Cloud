# P11-T-007 · scheduler-plugin NRT CRD bundle + numaAffinity default true 恢复

- **Commit**: (this commit)
- **Date**: 2026-05-22
- **Duration**: 0.8-1d plan / ~0.3d actual(Approach B vendored CRD · 模式简单)

## Intent

ADR-0017 §2 Decision D 5th 优先级 close。known-issues #12 完整 close 循环最终步:
- Phase 6 引入 NumaAffinity placeholder
- Phase 8/9 baseline bump 等待
- Phase 10 P10-T-005 wrap body land(nrt.New delegate)
- P10-fix-002 chart default `numaAffinity.enabled: false` 临时禁(因 NRT CRD 不在 cluster · nrt.New panic · unblock phase6 install)
- **P11-T-007 本 task**:NRT CRD vendored into chart `crds/` folder(Approach B per phase11-plan §3 default)+ default 恢复 true

## Path adaptations

1. **Approach B vs A 选 B**(per phase11-plan §3 P11-T-007 default recommendation):
   - **A** subchart dependency on upstream `noderesourcetopology-api` chart · 引入 Chart.yaml dependencies 块 · helm dep update / pull · chart release cadence 与 upstream 耦合
   - **B** vendored CRD YAML in chart `crds/` folder · 自管 · helm 自动 pre-install hook · 与 inference-operator chart pattern 一致(其 crds/ folder 也是 vendored)
   - 选 B:简单 + chart 自管 + 与项目其他 chart 同 pattern
2. **CRD 来源**:`~/go/pkg/mod/sigs.k8s.io/scheduler-plugins@v0.34.7/manifests/noderesourcetopology/crd.yaml`(Phase 10 T003 baseline bump 后 cached)· cp 到 `deploy/helm-charts/scheduler-plugin/crds/noderesourcetopologies.yaml` · annotations 保留 `api-approved.kubernetes.io` + `controller-gen.kubebuilder.io/version` 元数据。

## Debugging trail

无 build / test fail · vendored CRD 一次成功 · 仅修 values.yaml + 头部 comment。

## Key decisions

- **Approach B vendored** rationale 上(simpler · self-contained release cadence)
- **`numaAffinity.enabled: true` default 恢复**(回到 P10-T-005 ship 后的预期默认 · P10-fix-002 仅 临时禁)
- **operators 仍可 override**:`--set numaAffinity.enabled=false`(返回 P10-fix-002 临时状态 · 如 cluster 已有 NRT operator 装了 CRD 不想冲突)· 或删 `crds/noderesourcetopologies.yaml` 后 helm package(若 GitOps 外管 CRD)
- **phase10/ assert.sh T107-A1 同步更新**:从 "NumaAffinity wrap substrate present (default disabled per P10-fix-002)" 改为 "NumaAffinity wrap substrate present + default ENABLED + NRT CRD bundled (P11-T-007 carry close)"
- **phase11/ install.sh + assert.sh 新建**(预备 T107 task · 现已 ship 6 active assertions T107-A1..A6 + 3 conditional T107-C1..C3 · T107 自身后续可加 chart runtime smoke assertions)

## Verification

- 存在性:
  - `ls deploy/helm-charts/scheduler-plugin/crds/noderesourcetopologies.yaml` ✓(从 vendored upstream cp · file header 含 `apiVersion: apiextensions.k8s.io/v1 · CustomResourceDefinition · noderesourcetopologies.topology.node.k8s.io`)
  - `ls tests/e2e/kind/phase11/{install,assert}.sh` ✓
- 完整性:
  - `helm lint --strict deploy/helm-charts/scheduler-plugin/` → clean
  - `bash tests/e2e/kind/phase11/assert.sh` → **6 PASS + 3 SKIP + 1 T107-A5 SKIP(T008 carry)· all assertions PASSED**(注意 T107-A5 SKIP 因 T008 inference-operator chart ProxyImage env wire 还没 land · 不算 fail)
- 正确性:
  - phase10/ assert.sh T107-A1 内容 review · 与 P10-fix-002 历史 trail align + P11-T-007 close 信号
  - ADR-0010 §"P10-fix-002 + P11-T-007 NRT CRD bundle 闭环" 段加入 5-step trail

## Carry-forward

- **P11-T-008 inference-operator chart ProxyImage env wire**:6th(W1 last)优先级 · 完成后 T107-A5 即 PASS instead of SKIP
- **P11-T-107 真 kind smoke E2E** body land 后:phase11/install.sh + assert.sh 已 skeleton-ready · T107 在此基础 extend 真 cluster Pod Ready + Reconcile 触发 等 assertion(本 task ship syntactic + 6 chart-content assertions)
- **Phase 12+ K8s 1.36+ baseline bump**(per ADR-0010 §1 "下一次 baseline bump 评估时机")· NRT CRD 可能需要重 vendored from 新 sched-plugins version

## §0a 续 autonomous · 继续 T008
