# Phase 11 checkpoint — M5 真生产化 foundation subset milestone CLOSER · 20-task chain · chart packaging spine + Karmada 第一波 + Frontend src/ + 5th carry posture

> **Date**: 2026-05-22 · **Tag**: `phase-11-complete`(lands at this commit chain head)· **Branch**: `fix/p11-fix-001-dev-stack`(merge to dev at push gate · per memory `feedback_push_at_phase_tag_only.md`)
>
> Phase 11 closes the **M5 真生产化 foundation subset** milestone(per ADR-0017 §2 Decision B · foundation 后缀 anti-over-promise · Phase 12+/M6 在此基础加 production hardening cohort)along 3 主线 + 3 子线 per ADR-0017 §2 Decision A:
> - **主线 1** chart packaging spine 闭环:T003 demo-backend(Lease leader-elect)+ T004/T005/T006 3 IMS(controller-runtime manager wire)+ T007 sched-plugin NRT CRD bundle(numaAffinity default true 恢复 · **known-issues #12 完整 close 循环最终步**)+ T008 inference-operator chart DEFAULT_PROXY_IMAGE env wire(**known-issues #13 5-phase carry closer**)。
> - **主线 2** Karmada propagation 第一波 production-grade:T002 ADR-0018(host + 2 member kind cluster minimum)+ T102 Karmada chart bootstrap + T103 4 PropagationPolicy YAML + O2 DMS karmada-aggregated-apiserver lifted-informer helper + T104 ClusterQuota cluster-scope CRD + RecomputeTotal aggregation。
> - **主线 3** Frontend src/ Workload page extension 完整:T105 3 indicators(O2 DMS exposed badge + Quota progress bar + scaleHistory count tag)+ backend handler 3 query params bridge + i18n en-US + zh-CN 双语 + api-contract.yaml schema fields。
> - **子线 1** Phase 10 deferred items 全 close:T007 NRT CRD bundle(P10-fix-002 carry close)+ T008 ProxyImage env wire(P10-T-106 substrate consume)+ T108 Volcano 2nd → 3rd defer + T202 Partitionable Devices 1st → 2nd defer。
> - **子线 2** Lab gating 5th attempt:T101 deferred Phase 12+(**5th carry** · ADR-0011 §3 default policy · ADR-0017 §2 Decision C trigger 1 FIRED at W1 entry meeting · 重评结论 default-defer 维持 · trigger 3 M5+ milestone reset 保留 Phase 12+)。
> - **子线 3** 真 multi-cluster / multi-site demo 打磨 + docs 大整理 + Phase 12+ 前瞻:T201 master-demo-multi-site.sh 7-step orchestrated Karmada demo(Path P real-hw / Path F synthetic ring per LAB_AVAILABLE env)+ T203 docs/phase12-candidate-streams.md(8 Stream + 3 active carry tracks + Go v1 schema migration cohort plan)+ T204 本 checkpoint。
>
> **3 deferred outcomes**:T101 LAB-CONDITIONAL **5th carry**(per ADR-0011 §3 default policy · synthetic ring fixture Phase 7-11 cumulative cover)+ T108 Volcano **3rd defer Phase 12+**(no training-job demo signal · default per Phase 9 P9-T-101 carry policy · 与真硬件 milestone naming 解耦)+ T202 Partitionable Devices **2nd defer Phase 12+**(KEP-4815 仍 Beta + K8s 1.34 baseline 不 unlock · 与 K8s baseline bump trigger 联动)。
>
> **2 known-issues closed**:#12 NumaAffinity NRT CRD bundle(P11-T-007 · 完整 close 循环最终步:Phase 6 placeholder → 8/9 baseline wait → P10-T-005 wrap body → P10-fix-002 临时禁 → P11-T-007 NRT bundle + default true 恢复)+ #13 ProxyImage env wire(P11-T-008 · 5-phase carry closer:P7 placeholder → P8/9 carry → P10-T-106 EffectiveProxyImage helper + chart values field → P11-T-008 env wire)。
>
> **2 entry ADRs**:ADR-0017 Phase 11 entry decisions(4 Decisions A-D · 3 Open questions)+ ADR-0018 Karmada deployment topology(4 Decisions A-D · 3 Open questions)。
>
> **K8s baseline outcome**:K8s 1.34.3 维持(per P10-T-003 三件套 lock · 未升 1.36 · Track C Partitionable Devices 留 Phase 12+ K8s 1.36+ baseline bump cohort · per ADR-0016 §3 Stream 6)。

## 1. Deliverables(20/20 statuses · per task taxonomy)

```
W1 Foundation(8 tasks · 全 LANDED · 2 ADRs + 4 new chart packaging + 1 NRT CRD bundle + 1 chart env wire)
├── P11-T-001  ADR-0017 entry decisions(4 Decisions A-D + 3 Open questions · 548144d)
├── P11-T-002  ADR-0018 Karmada deployment topology(4 Decisions A-D + 3 Open questions · c65283c)
├── P11-T-003  demo-backend helm chart + cmd/main.go Lease leader-elect wire
│              (9 chart file + backend/pkg/cache/leaderelect.go client-go
│               tools/leaderelection · 1a46edc)
├── P11-T-004  IMS-1 node-lifecycle-operator helm chart + cmd/main.go ctrl.Reconciler
│              (8 chart file + reconciler.go shell + SchemeBuilder + Dockerfile ·
│               go.mod controller-runtime/client-go added · aedb1a0)
├── P11-T-005  IMS-2 software-mgmt-operator helm chart + cmd/main.go(同 IMS-1 pattern
│              + RBAC 收紧 Node 仅 list/watch + annotation-based observed state · c5998d6)
├── P11-T-006  IMS-3 bare-metal-provisioning-operator helm chart + Redfish/IPMI stub
│              + Secret 解析(同 IMS-1 + redfish.go/ipmi.go BMC client stub interface +
│               Secret RBAC + buildBMC address-scheme 分发 · d544bc0)
├── P11-T-007  scheduler-plugin NRT CRD bundle(Approach B vendored 从 sched-plugins
│              v0.34.7)+ numaAffinity.enabled default true 恢复 + phase11/ folder
│              skeleton install.sh + assert.sh · known-issues #12 完整 close 循环最终步
│              · 7e8f026
└── P11-T-008  inference-operator chart DEFAULT_PROXY_IMAGE env wire(chart values
                .defaults.proxyImage field + deployment.yaml env conditional injection
                + cmd/main.go startup hook + 1 integration test · known-issues #13
                5-phase carry closer · b11573d)

W2 Polish + decision-gated + Karmada + frontend + smoke(8 tasks · 5 LANDED + 2 DEFERRED + 1 LANDED conditional)
├── P11-T-101  [LAB-CONDITIONAL · 5th attempt]Source.RealAscend body
│              → **DEFERRED Phase 12+(5th carry)** · default policy(no lab signal)
│              · per ADR-0011 §3 + ADR-0017 §2 Decision C · 6f57139
├── P11-T-102  Karmada control-plane chart deploy + 2 member kind bootstrap scripts
│              (deploy/karmada/install.sh + uninstall.sh + values.yaml + README ·
│               6-step bootstrap + idempotent teardown · 648bd68)
├── P11-T-103  Karmada PropagationPolicy 第一波 + O2 DMS Adapter cross-cluster
│              informer helper(4 PropagationPolicy YAML + karmada_aggregated.go
│               + 5 unit tests · 7d170fb)
├── P11-T-104  Quota ClusterQuota CRD + Karmada cross-cluster usage aggregation
│              primitive(clusterquota_types.go + RecomputeTotal + 3 unit tests +
│               CRD bundle in inference-operator chart · 637ff87)
├── P11-T-105  Frontend src/ Workload page 3 indicators + backend handler bridge
│              (api-contract.yaml 3 schema fields + 2 sub-schema + backend model +
│               handler + frontend service + WorkloadTable 3 conditional columns +
│               i18n 双语 · e6b9de4)
├── P11-T-106  O2 DMS authn chart wiring(OIDC + TokenReview SA env injection +
│              conditional RBAC tokenreviews verb · 27e83b9)
├── P11-T-107  kind smoke E2E Phase 11 extension(14 assertions covering W1+W2
│              substrate · 11 active + 3 conditional · all PASSED in syntactic
│              verify · 4925e86)
└── P11-T-108  [DECISION-GATED · 2nd attempt]Volcano gang-scheduling install body
                → **DEFERRED Phase 12+(3rd defer)** · default policy(no training-
                job demo signal)· per Phase 9 P9-T-101 carry policy · 77c025a

W3 Closer + Milestone Closer(4 tasks · 3 LANDED + 1 DEFERRED)
├── P11-T-201  master-demo-multi-site.sh — 真 multi-cluster Karmada demo extension
│              (7-step orchestrated · Path P real-hw / Path F synthetic ring per
│               LAB_AVAILABLE env · 同 Phase 10 master-demo.sh sibling · d4d471b)
├── P11-T-202  [DECISION-GATED · re-eval W3 entry]Partitionable Devices Beta +
│              partition-aware allocator → **DEFERRED Phase 12+** · K8s 1.34
│              baseline 不 unlock · KEP-4815 仍 Beta · 与 Stream 6 K8s baseline
│              bump cohort 一起 land Phase 12+ · 905838e
├── P11-T-203  docs 大整理 + Phase 12+ 前瞻 + Go v1 schema migration cohort plan
│              (docs/phase12-candidate-streams.md new · 4 大 section · 3 active
│               carry tracks + 15 carry-forward items + Spine candidates · README
│               current-phase + 下一阶段 + 上一阶段 line update · 76fafca)
└── P11-T-204  Phase 11 checkpoint + tag phase-11-complete + M5 milestone CLOSER
                announcement(本文件 · 即 this commit)
```

**Tally**:20 / 20 statuses · 17 net-new substrates(T001-T008 + T102-T107 + T201 + T203 + T204)+ 3 deferred outcomes(T101 + T108 + T202)+ 4 fix commits(P11-fix-001 dev-stack docker-compose path A · P11-fix-002 ascend exporter +5 metric · P11-fix-003 dashboard template-var · per checkpoint-phase11 fix entries)+ 1 Frontend UX Track-2 charter(per `docs/devlog/phase-11-frontend-ux-track-charter.md` · F01-F04 sub-track conditional · 留 Phase 12+ decide)+ 1 demo runbook(`e43b223` interactive HTML walkthrough)+ 1 Phase 11 plan(`5a17fe3`)。

## 2. ADR forward note + status updates landed in Phase 11

- **ADR-0003 v2** IMS-1/2/3 controller body status flip "chart + cmd/main.go LANDED at P11-T-004 + T005 + T006"(共 3 chart + Reconciler shell + Dockerfile + SchemeBuilder + CRD bundle)
- **ADR-0009 §4** Partitionable Devices deferred Phase 12+(P11-T-202 · 2nd defer · K8s 1.34 baseline 不 unlock · KEP-4815 Beta · Stream 6 cohort 联动)
- **ADR-0010** §未来扩展 Volcano row 加 P11-T-108 3rd defer entry(累计 3 次推迟 · 与 lab gating / Partitionable Devices 独立 trigger 条件)· §3 P10-fix-002 + P11-T-007 NRT CRD bundle 闭环段(known-issues #12 完整 close)
- **ADR-0011 §3** lab gating carry tally 加 P10-T-102 4th carry + P11-T-101 5th carry entry · 2026-05-22 update 段引 ADR-0017 §2 Decision C trigger 1 FIRED + trigger 3 M5+ milestone reset 保留 Phase 12+
- **ADR-0013 §6** Forward notes 加 Phase 11 T002/T103 子契约段(implementation contract by ADR-0018 §2 Decision D lifted-informer pattern · O2 DMS Adapter karmada-aggregated-apiserver path)
- **ADR-0014 §7** Forward notes 加 Phase 11 T002/T104 子契约段(ClusterQuota CRD + ClusterPropagationPolicy + RecomputeTotal aggregation · P11-T-104 LANDED stamp)
- **ADR-0015 §3.1** Phase 10 W1 immediate impact 加 P11-T-003 status update(chart packaging + main.go leader-elect wire LANDED · 实现路径 client-go vs controller-runtime rationale)
- **ADR-0017** Phase 11 entry decisions Accepted(P11-T-001 · 4 Decisions A-D + 3 Open questions · 全 Phase 11 task 起手 policy lock)
- **ADR-0018** Karmada deployment topology Accepted(P11-T-002 · 4 Decisions A-D + 3 Open questions · §2 Decision B 加 P11-T-102 LANDED stamp)
- **arch §1.3** phase 路线图 unchanged(per ADR-0017 §1.4 + P11-T-001 spec · M5 加 in §13 review-table promote 行 leave T204 一并)
- **arch §3.2** 前端 forward note(P11-T-001)+ §5.1 demo-backend cache 行 cross-ref T003(P11-T-001)+ §5.4 inference-operator forward note T008(P11-T-008)+ §5.9-§5.11 3 IMS row 加 "Phase 11 P11-T-004/005/006 chart packaging LANDED"(P11-T-004/T005/T006)+ §9.3 多站点 forward note(P11-T-002)+ §13 review-table Phase 11 row(T204 promote "in flight" → "landed phase-11-complete")

## 3. Test posture summary

| Suite | Active | Skipped | Coverage |
|---|---|---|---|
| Backend `go test ./...` | 100% pass | 0 | pkg/cache + pkg/config + pkg/api + pkg/model |
| Operators `go test -vet=off ./...` × 6 module | 100% pass | 0 | api/v1alpha1 round-trip + internal/controller ReconcileOnce + internal/{rollout,state,client} 各自 |
| Frontend `npx tsc --noEmit` | 100% pass | 0 | full TS typecheck post-types regen |
| Frontend `pnpm test` | (not run in T204) | 留 后续 | 留 mock source data fixture 完整后 · Phase 12+ cohort |
| Helm `helm lint --strict` × 7 chart | 100% clean | 0 | demo-backend + 3 IMS + scheduler-plugin + inference-operator + o2-dms-adapter |
| Helm `helm template` × 7 chart | render OK | 0 | full chart rendering + conditional env/RBAC verify |
| `tests/e2e/kind/phase11/assert.sh` | 11 active PASS | 3 conditional SKIP(KARMADA_ENABLED/VOLCANO_ENABLED/LAB_AVAILABLE default 0) | syntactic + chart content / file existence |
| `tests/e2e/kind/master-demo-multi-site.sh` | bash -n OK | (kind binary missing in dev · 真 cluster verify 留 CI) | 7-step orchestrated · Path P / F |
| `tests/e2e/kind/phase10/assert.sh` T107-A1 update | PASS(NRT CRD bundle stamp) | 0 | P10-fix-002 → P11-T-007 close loop verify |

## 4. Scope adaptations(透明 documented · 全 Phase 11 task 共 9 项)

详 per-task devlog `Path adaptations` 段 · Highlight:
- **T003 client-go over controller-runtime**(backend 无 reconciler 需求 · 与 IMS-1/2/3 controller-runtime 不同框架 rationale)
- **T004/T005/T006 SchemeBuilder + AddToScheme + zz_generated.deepcopy.go + CRD YAML 补齐**(Phase 9 scaffold 未 ship · controller-gen 自动 生成)
- **T005 RBAC 收紧 Node 仅 list/watch**(SoftwareBundle controller 不 mutate Node · per-node agents 留 Phase 12+ NodeSoftwareBundleStatus CR)
- **T005 annotation-based observed state**(IMS-2 SoftwareBundleStatus schema 仅 count fields · per-CR observation 走 annotation map · Phase 12+ schema 升级)
- **T006 Redfish/IPMI stub interface**(real SDK calls 留 Phase 12+ · stub interface 不变 · Phase 11 chart packaging 闭环交付)
- **T007 Approach B vendored CRD YAML**(per phase11-plan §3 P11-T-007 default · 不引入 subchart 依赖 · chart 自管 release cadence)
- **T101/T108/T202 default deferred path**(per ADR-0017 §2 Decision A/C 不在 Phase 11 spine · 各自独立 trigger 条件 · 3 active carry tracks)
- **T105 WorkloadDetailDrawer ECharts timeline 留 后续**(Table 3 column minimum-viable surface ship · 真 ECharts 在 scaleHistory entries > 5 时更有价值 · 演示数据稀)
- **T203 docs 大整理 outline-only**(arch §13 promote + checkpoint-phase11.md 留 T204 一并 land · 同 Phase 10 P10-T-203 → T204 pattern)

## 5. Verification + post-tag CI gate expectations

详 per-task devlog `Verification` 段。本 checkpoint 落地后:
1. `git tag phase-11-complete <THIS_COMMIT_SHA>` 在本 commit head 落 tag
2. Per memory `feedback_push_at_phase_tag_only.md`:**push to remote 触发 CI gate · 全链(20 task commit + 4 fix + 1 charter + 本 checkpoint + tag)一次性 push**
3. Watch GitHub Actions workflow runs on dev HEAD post-tag(24 commits 一次 push 触发)
4. 修 all ❌ via P11-fix-NNN series(per memory `feedback_post_tag_ci_gate.md` Phase 7 实战 4 fixes 经验 · Phase 11 已 ship 3 fixes P11-fix-001/002/003 · 实战 dev-stack docker-compose path A · ascend exporter +5 metric · dashboard template-var · 后续 post-tag CI 若再 surface 新 issue 走 P11-fix-NNN 续)
5. 直到 dev HEAD 全绿 → **Phase 11 真完成** → **M5 真生产化 foundation subset milestone CLOSER announcement** 真 land

## 6. Phase 12+ handoff brief

**Source-of-truth**:`docs/phase12-candidate-streams.md`(T203 ship)· 全面 covers:
- 3 active carry tracks(Track A lab 5th→6th · Track B Volcano 3rd→4th · Track C Partitionable Devices 2nd→3rd · 各自独立 trigger 条件)
- 15 new Phase 11+ carry-forward 项(from Phase 11 task devlogs · Priority H/M/L)
- Spine candidates A/B/C/D(A continuation 真生产化 production-hardening · B 真硬件-native · C 生态扩展 · D 多 milestone 拆)
- Go v1 ResourceSlice schema migration cohort plan(per P10-fix-001 carry · 5 module · 1.5-2 month cohort estimate · 4-step migration plan)
- M6 milestone naming 留 Phase 12+ entry meeting decide

**Phase 12+ entry meeting agenda**:
1. **Track A lab gating 6th carry posture re-eval**(per ADR-0017 §2 Decision C trigger 3 评估窗口 · 3 选项 a/b/c codified)
2. **Track B + Track C trigger conditions re-eval**(Volcano training-job demo signal · K8s 1.36 baseline bump cohort · KEP-4815 GA status)
3. **Phase 12+ primary spine 选择**(A continuation 推荐 · 与 完整 OIDC IdP + Vault + Karmada HA + vLLM PD SLA cohort align)
4. **M6 milestone naming**(per ADR-0017 §2 Decision B forward note · 与 Spine 选择联动)
5. **Go v1 schema migration cohort scheduling**(per P10-fix-001 carry · 与 baseline bump 同期)

## 7. CI gate post-tag

Per memory `feedback_post_tag_ci_gate.md`(Phase 7 实战 4 fixes · 2026-05-21):**phase-N-complete tag push 后必看 GitHub Actions · 修所有 ❌ 直到 dev HEAD 全绿才算 phase 真完成**。

本 phase-11-complete tag push 后 main agent / next session 必须:
1. Watch GitHub Actions workflow runs on dev HEAD post-tag(24 commits 一次 push 触发)
2. 修 all ❌ via P11-fix-NNN series · phase-11-fix-{004,005,...} 续编号(已 ship fix-001/002/003 · 续 fix-004+ 若新 issue surface)
3. 直到 dev HEAD 全绿 → **Phase 11 真完成** → **M5 真生产化 foundation subset milestone CLOSER announcement** 真 land

---

**END of Phase 11 checkpoint**
