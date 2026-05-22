# P11-T-102 · Karmada control-plane chart deploy + 2 member kind bootstrap

- **Commit**: (this commit)
- **Date**: 2026-05-22
- **Duration**: 1.5-2d plan / ~0.5d actual(scripted bootstrap · 实际 helm install + karmadactl join 走真 cluster 留 T201 / T107 真 smoke)

## Intent

ADR-0018 §2 Decision A topology + Decision B chart + Decision C bootstrap script 三 decisions 实质 land · 为 T103/T104 PropagationPolicy + ClusterQuota 提供 multi-cluster 部署底座。

## Path adaptations

1. **kind 在 dev env 不存**:install.sh 真 run 需要 kind binary · 本 dev 没装 · 故本 task ship 脚本 + 文档 + values · `bash -n` syntax check 通 · 真 run smoke 留 T107 phase11/install.sh 启 KARMADA_ENABLED=1 path(在装了 kind 的 CI 上跑)+ T201 master-demo-multi-site.sh 实测。
2. **chart 版本不在 install.sh 中 hard-pin**:Karmada upstream chart latest stable 移动较快 · hard-pin 容易 stale。设计:`KARMADA_CHART_VERSION` env(可选)· empty = upstream latest · operators 显式 pin via env or 在 P11-T-204 checkpoint commit 时 stamp 具体测试版本。
3. **`karmadactl` binary 不强求**:install.sh 检测 `karmadactl` 或 `kubectl-karmada`(kubectl plugin variant)· 都没装则报 install hint(`go install github.com/karmada-io/karmada/cmd/karmadactl@latest`)。

## Debugging trail

无 build/test fail · bash 脚本 + helm values + Markdown · `bash -n` syntax check 通。

## Key decisions

- **6-step install.sh**:pre-flight tool check → kind create × 3 → kubectl create karmada-system ns → helm install karmada → 提取 karmada apiserver kubeconfig → karmadactl join × N member → verify cluster Ready timeout 120s
- **install.sh exit codes 1-5**:细分错(missing binary / kind create fail / chart install fail / join fail / verify timeout)· 便于 T107 + T201 自动化诊断
- **uninstall.sh idempotent**:全 best-effort · 单步失败 echo + continue · 避免半截状态阻塞重试
- **values.yaml conservative**:single replica 全 component(per ADR-0018 §4 (a))· in-cluster etcd · auto certs(Phase 12+ Vault per ADR-0018 §4 (c))· disabled karmada-descheduler / karmada-search(demo footprint)· keep schedulerEstimator(后续 T103 PropagationPolicy 需要)
- **支持 `KARMADA_CHART_VERSION` env**(可选)· chart 版本不在脚本 hard-pin · operators 可 override

## Verification

- 存在性:`deploy/karmada/{install,uninstall}.sh + values.yaml + README.md` 4 file ✓
- 完整性:
  - `bash -n deploy/karmada/install.sh` syntax OK
  - `bash -n deploy/karmada/uninstall.sh` syntax OK
  - install.sh exit 0/1/2/3/4/5 paths covered(6-step + pre-flight + verify timeout)
  - README.md 含 quick start + topology ascii + env override table + 3 troubleshooting items + ADR cross-refs
- 正确性:install.sh ↔ uninstall.sh env var 一致 · MEMBER_PREFIX/MEMBER_COUNT 两端对应 · KARMADA_KUBECONFIG 写位置同
- kind 真 smoke fall back:dev env 无 kind binary · syntactic verify only · T107 phase11/install.sh KARMADA_ENABLED=1 段走真 smoke · T201 master-demo-multi-site.sh 自动调 install.sh

## Carry-forward

- **P11-T-103 Karmada PropagationPolicy 第一波**:本 install.sh ship 后 ·`deploy/karmada/policies/` 4 YAML(propagation-modelservice + propagation-npuslicepool + propagation-quota + cluster-propagation-clusterquota)· T103 落地
- **P11-T-104 ClusterQuota CRD + cross-cluster aggregation**:基于本 install.sh 部署完成 · admission webhook 在 host cluster · ClusterQuota CRD propagate 到 member · status.usage.perCluster aggregation
- **P11-T-107 kind smoke** 已 ship phase11/install.sh `KARMADA_ENABLED=1` 分支调本 install.sh
- **P11-T-201 真 multi-cluster / multi-site demo**:master-demo-multi-site.sh 走 install.sh → apply policies → 真 propagation 端到端 demo
- **Phase 12+ Karmada HA**(per ADR-0018 §4 (a)):values.yaml replicaCount=3 + external etcd HA + leader-elect 全 component
- **Phase 12+ Vault Secret 路径**(per ADR-0018 §4 (c)):cross-cluster Secret 跨 cluster 静态复制 → Vault Agent inject

## §0a 续 autonomous · 继续 T103 Karmada PropagationPolicy
