# P11-T-002 · ADR-0018 Karmada deployment topology(起草 + Accepted)

- **Commit**: (this commit)
- **Date**: 2026-05-22
- **Duration**: 0.5d plan / ~0.3d actual(docs-only · 无 build/test · 无 spike)

## Intent

锁 Karmada propagation 第一波 production-grade 4 项 decisions · 为 T102-T104 三 task 提供 implementation contract:
- Topology = 1 host + 2 member kind cluster minimum(单机模拟 multi-site)
- Chart = upstream `karmada-charts/karmada` + 自研 `deploy/karmada/policies/` PropagationPolicy selector templates
- Bootstrap = kind create cluster × 3 + karmadactl join push mode default
- Cross-cluster informer = lifted-informer pattern via Karmada karmada-aggregated-apiserver(O2 DMS Adapter + Quota ClusterQuota informer 各自 aggregation)

ADR-0013 §6 + ADR-0014 §7 既有 forward notes 是 *what* · ADR-0018 是 *how* · 后续 T103/T104 落 PropagationPolicy YAML + lifted-informer wire · 不在每 task 重述决策。

## Path adaptations(plan literal vs codebase reality)

1 处:
1. **Plan 提到 spike 文档 `docs/research/karmada-multi-site-spike.md`**(plan §3 P11-T-002 引用 line):
   - 实际 grep:`ls docs/research/` 无 `karmada-multi-site-spike.md` · 仅有 demo-backend-cache-spike + k8s-partitionable-devices-spike + volcano-gang-scheduling-spike 等
   - 解决:ADR §1.4 "本 ADR 不涉及" 明示 "本 ADR ship 时未起 spike 文档 · 本 ADR §1.2 + §2 决策依据 ADR-0013/0014 forward notes + Karmada upstream chart docs + 本 ADR §5 引用 grep-verified · spike 文档 Phase 12+ 起若需更深入 chunk-level 评估时另立" · 不强行起 spike 占位

## Debugging trail

无 build / test fail · 纯 docs · 但 2 个小决策反复:

1. **chart 选 `karmada-charts/karmada` vs `karmada-operator`** — Karmada repo 有两 chart sub-tree:`charts/karmada`(direct chart · components 显式 list)+ `charts/karmada-operator`(Operator-managed control plane · 引入 Operator CRD 一层)。本 ADR 选 direct chart · rationale 简单 ops surface(直接 install / uninstall · 无 Operator lifecycle 抽象)· 真 production HA + lifecycle 留 Phase 12+ 时再评估 Operator-managed 升级。
2. **push mode vs pull mode** — Karmada doc 两种模式:push(CP 主动 sync)vs pull(member 装 karmada-agent 拉)。本 ADR 选 push mode default · rationale host + member 都在同一控制域(same Docker daemon · same dev box · same admin)· pull mode 适合真 enterprise multi-org · 留 Phase 12+ · ADR §2 Decision C 第二段写清两 mode 适用场景。

## Key decisions

- **Topology = 1 host + 2 member kind cluster minimum**(per ADR-0016 §3 Stream 1 part 1 单机模拟 multi-site · 真 多机房留 Stream 1 part 2 Phase 12+)
- **Chart = upstream karmada-charts/karmada · pin 版本 at T102 起手 verification stamp**(不锁具体版本 in ADR · 因 Phase 11 W2 T102 时 latest stable 可能漂 · T102 devlog 入 version stamp)
- **Push mode default**(单机模拟 demo 优 push;真 enterprise multi-org pull · 留 Phase 12+)
- **Lifted-informer pattern**(per Karmada doc · controller-runtime 调 karmada-aggregated-apiserver path · O2 DMS Adapter + Quota ClusterQuota informer 各自跨 cluster aggregation)· 拒绝 KubeFed v2 lifted informer(已 archived 2024-EOL)· 拒绝 OCM(footprint 更重 · 不适合 demo)
- **per-CRD PropagationPolicy** default(per ADR §4 (b) Open question · 简单 + 演示需求够 · per-resource override 留 Phase 12+ if per-tenant placement signal)
- **cross-cluster Secret 跨 cluster static**(install.sh 手动复制 · per ADR §4 (c) Open question · Vault/external-secrets 留 Phase 12+ 与 Stream 5 part 2 完整 OIDC IdP 一起)

## Verification

P3 三项验证维度 全过(docs-only · 无 compile/test):

- **存在性**:`ls docs/adr/0018-karmada-deployment-topology.md` ✓ ;ADR §5 引用 8 行 upstream + 6 行 commit chain 全部对应真存在 file / SHA(grep / git rev-parse 验)
- **完整性**:plan §3 P11-T-002 acceptance 8 项逐项核 — §1 Context(ADR-0016 §3 Stream 1 + checkpoint-phase10 §6 #7 cited)+ §2 Decision A(1 host + 2 member minimum · 单机模拟)+ §2 Decision B(upstream chart + PropagationPolicy selector 自研)+ §2 Decision C(kind + karmadactl join · push mode default)+ §2 Decision D(lifted-informer pattern · O2 DMS / Quota informer 各自 aggregation)+ §3 T102+T103+T104 cross-ref 表 + §4 3 open questions(Karmada HA · PropagationPolicy granularity · cross-cluster RBAC + Secret)+ §5 引用 full list — 全覆盖
- **正确性**:ADR §1 引用 cited fact 全部 grep-verified(ADR-0013 §6 + ADR-0014 §7 forward notes 内容 cross-ref · ADR-0016 §3 Stream 1 content · ADR-0017 §2 Decision A 主线 2 + §4 (a) Karmada HA · checkpoint-phase10 §6 #7 项数 verified);Karmada upstream 路径(charts/karmada · karmada-aggregated-apiserver · karmadactl join push mode · ClusterPropagationPolicy vs PropagationPolicy)与 Karmada doc convention align(本 ADR 不直接 WebFetch · 信 Karmada upstream stable doc 长期 align · T102 起手 helm install 时具体 chart version 实测 verify)

## Carry-forward

- **P11-T-102** Karmada control-plane chart deploy + 2 member kind bootstrap script:本 ADR §2 Decision A topology + §2 Decision B chart + §2 Decision C bootstrap script 即 implementation contract · install.sh + uninstall.sh + values.yaml + README.md 4 file · T102 起手 reference 本 ADR §2 三 Decision 即可 · helm chart version pin 落在 T102 devlog verification stamp
- **P11-T-103** Karmada PropagationPolicy 第一波 + cross-cluster informer aggregation:本 ADR §2 Decision B 4 PropagationPolicy template + §2 Decision D O2 DMS Adapter karmada-aggregated-apiserver path 即 implementation contract · 4 YAML file in `deploy/karmada/policies/`
- **P11-T-104** Quota ClusterQuota CRD + Karmada cross-cluster usage 累计:本 ADR §2 Decision B cluster-propagation-clusterquota.yaml + §2 Decision D ClusterQuota aggregation pattern 即 implementation contract · clusterquota_types.go + admission webhook ext + status.usage.perCluster map field
- **P11-T-201** 真 multi-cluster / multi-site demo 打磨:本 ADR §2 Decision A topology 是 demo 基础 · master-demo-multi-site.sh 跑 3 cluster 拓扑 · 反映 Karmada propagation 端到端 + cache singleton multi-instance failover + Quota cross-cluster aggregation 演示
- **P11-T-203 docs 大整理**:本 ADR §4 3 open questions + Phase 12+ HA / Vault / 真 multi-region forward notes carry · arch §13 review-table promote (与 ADR-0017 promote 同 row 增量更新)
- **Phase 12+ Karmada HA + Vault Secret + 真 multi-region** trigger:本 ADR §4 (a) (c) Open questions · 与 完整 P99 SLA(ADR-0016 §3 Stream 4)+ 完整 OIDC IdP(Stream 5 part 2)一起 land

## §0a.10 / §0a.11 compliance

- 本 task 是 T001 之后 fresh continuation per 用户 "按计划执行" autonomous mode · 不停于 T001 边界
- §0a.11 docs-only 例外:main agent 直接做 · 无 subagent · strict verify = grep cross-refs + acceptance 逐项核
- T002 与 T001 是 phase11-plan §2 parallelisation candidates 标 "standalone — no code dependency · 不同文档可 parallel if user 显式 batch" · 用户未显式 batch · serial main agent · 顺序 T001 → T002
- 不 push remote · 累积到 phase-11-complete tag(per memory `feedback_push_at_phase_tag_only.md`)· 继续 T003
