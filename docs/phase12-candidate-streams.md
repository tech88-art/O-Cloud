# Phase 12+ candidate streams enumeration

> Per ADR-0017 §2 Decision B "Phase 12+/M6 在此基础加 production hardening
> cohort" + ADR-0016 §3 Stream 1-8 carry-forward。Phase 11 W3 P11-T-203
> 起草。Phase 12 entry meeting 起草 plan 时 codify primary spine + 优先级。

## Source-of-truth recap

| Source | What it carries |
|---|---|
| ADR-0016 §3 Stream 1-8 | Initial enumeration from Phase 10 W3 entry |
| ADR-0017 §2 Decision A 不在 Phase 11 spine 列表 | Phase 11 explicitly defers what |
| ADR-0017 §4 Open questions | 3 Phase 12+ candidate(Karmada HA · Frontend i18n · Lease Pod数) |
| ADR-0018 §4 Open questions | 3 Phase 12+ candidate(Karmada control HA · PropagationPolicy granularity · cross-cluster Secret) |
| Phase 11 devlog Carry-forward 段(20 docs · 含 4 Frontend UX charter) | Per-task identified follow-up work |
| ADR-0011 §3 carry tally + ADR-0010 §未来扩展 Volcano row + ADR-0009 §4 Partitionable Devices | 3 active carry tracks |

## Active carry tracks(Phase 12+ entry W1 必评估)

### Track A · Lab gating 5th → 6th carry posture(ADR-0011 §3 + ADR-0017 §2 Decision C trigger 3)

- **Current**: 5 phase consecutive defer(P7+P8+P9+P10+P11)· default policy 维持
- **Phase 12+ entry options**(per ADR-0017 §2 Decision C codified):
  - 选项 a:仍 default-defer 维持 6th attempt(若 Phase 12+ scope 与真硬件无 direct dependency)
  - 选项 b:flip default-light-up(若 Phase 12+ scope 强依赖真硬件)
  - 选项 c:M5+ milestone naming reset(若 Phase 12+ scope 重定向 production / multi-site / SLA hardening · 真硬件 stamp 成为 entry 前提)
- **Decision triggers**:用户 chat signal at Phase 12 entry meeting · ad-hoc lab access materialise

### Track B · Volcano gang-scheduling 3rd → 4th carry(ADR-0010 §未来扩展)

- **Current**: 3 phase consecutive defer(P9+P10+P11)· `helm install` + chart smoke 路径已 ship 备用
- **Phase 12+ entry**:if training-job demo signal materialise → light up(`tests/e2e/kind/phase11/install.sh VOLCANO_ENABLED=1` 路径已 ready)· 否则 4th carry
- **Independent track from Track A**(Volcano 与真硬件 milestone naming 解耦)

### Track C · Partitionable Devices 2nd → 3rd carry(ADR-0009 §4)

- **Current**: 2 phase defer(P10+P11)· K8s baseline 1.34 + KEP-4815 Beta(GA timing unconfirmed)
- **Phase 12+ entry**:re-WebFetch KEP-4815 status + 评估 K8s 1.36 baseline bump cohort(per ADR-0016 §3 Stream 6)· 若双 unlock → light up + partition-aware allocator · 否则 3rd carry

## New Phase 11+ carry-forward(from Phase 11 task devlogs)

| Carry-forward | Source(devlog) | Priority | Description |
|---|---|---|---|
| Real Redfish/IPMI BMC SDK integration | phase-11-t006.md | M | replace stub clients(redfish.go / ipmi.go)with stoian/gofish + vmware/goipmi(or `ipmitool` shell-out)· stub interface 不变 |
| Karmada control HA(3+ replica + external etcd) | phase-11-t102.md + ADR-0018 §4 (a) | H | production hardening cohort · 与 完整 P99 SLA + OIDC IdP 一起 land |
| Vault Secret cross-cluster propagation | phase-11-t103.md + ADR-0018 §4 (c) | H | external-secrets operator / Vault Agent inject · 替代 install.sh static copy · 与 OIDC IdP 同期 |
| ClusterQuota admission webhook B + Quota controller tick reconcile | phase-11-t104.md | H | NPUVerticalScaler scale rate check 路径加 ClusterQuota.status.usage.Total 读路径 + Karmada cross-cluster usage tick |
| Source impl 真数据 fill Workload 3 indicators | phase-11-t105.md | M | mock source(fixture data)+ k8s source(annotation read · Quota CR join · NPUVerticalScaler.scaleHistory read) |
| cmd/main.go OIDC + TokenReview Validator wiring | phase-11-t106.md | H | P10-T-103 Validator interface 已 ship · main.go consumer 留 后续 · 与 完整 OIDC IdP deploy 一起 |
| Real Pod Ready + Reconcile triggered runtime kind smoke | phase-11-t107.md | M | CI 装 kind 后 · 在 phase11/assert.sh 加 runtime assertions |
| 真 multi-region multi-cluster | phase-11-t201.md + ADR-0018 §2 Decision A part 2 | H | 真物理 multi-region / multi-机房 deployment · per ADR-0016 §3 Stream 1 part 2 |
| Per-resource PropagationPolicy override | ADR-0018 §4 (b) | L | if per-tenant placement signal materialises |
| Demo-backend replicaCount 3+ HA default | ADR-0017 §4 (c) | M | 与 Karmada control HA 同 production hardening cohort |
| Frontend WorkloadDetailDrawer ECharts scaleHistory timeline | phase-11-t105.md | L | T201 / Phase 12+ richer surface · 当 scaleHistory entries > 5 时更有价值 |
| Frontend Vitest tests for 3 indicator columns | phase-11-t105.md | M | 留 mock source data fixture 完整后 · 与 Source impl 同期 |
| Event-driven Quota status.usage sync | ADR-0014 §7 line 5 | L | 替代 60s tick · informer event → quota controller 增量更新 |
| Webhook cert split | ADR-0014 §7 line 6 | L | per Quota A / Quota B / PD Router / NPUVerticalScaler 独立 cert · cert reuse 出 incident 时触发 |
| HCCL live migration kernel/driver support | ADR-0016 §3 Stream 2 | L | depends on Huawei vendor roadmap |
| Real fabric switch integration | ADR-0016 §3 Stream 3 | L | 真物理 switching gear 依赖 |
| vLLM PD 分离 production-grade SLA(P99) | ADR-0016 §3 Stream 4 | H | 甲方 SLO 输入 + 真业务 traffic 依赖 · M6 spine 候选 |
| OIDC IdP 完整集成(Keycloak / Dex) | ADR-0016 §3 Stream 5 part 2 | H | 甲方 IdP 选定 dependent · M6 spine 候选 |
| O2 IMS R1 v05.00+ spec migration | ADR-0016 §3 Stream 8 | L | O-RAN WG6 release cadence dependent |

## Spine candidates(Phase 12+ entry decide)

Per ADR-0016 §4 (b) + Phase 11 outcome experience:

- **Spine A continuation 真生产化 production-hardening cohort**(高优先级):Karmada HA + 完整 OIDC IdP + ClusterQuota webhook B 完整 + Vault Secret + vLLM PD 分离 production-grade SLA
- **Spine B 真硬件-native cohort**(per Track A 6th carry trigger 3 路径):lab onboarding + Source.RealAscend + cann-driver-matrix stamp + Partitionable Devices(if K8s 1.36+ GA · Track C 联动)
- **Spine C 生态扩展 cohort**(if signal):Volcano gang training-job(Track B 4th carry trigger)+ O2 IMS R1 v05.00+ migration(Stream 8)+ MSChat / model registry adapters
- **Spine D 多 milestone 拆**:若 scope > 1 phase calendar(typical · Spine A 完整 ~3-4 phase)→ Phase 12 = part 1(Karmada HA + OIDC IdP)· Phase 13 = part 2(SLA + Vault)· etc

**M6 milestone naming**:留 Phase 12+ entry meeting decide(per ADR-0017 §2 Decision B "M6 在此基础加 production hardening cohort" forward note · 与 Spine 选择联动)

## Go v1 ResourceSlice schema migration cohort(per P10-fix-001 carry)

P10-fix-001 commit `7203587`(post-tag CI gate)发现 K8s 1.34 DRA GA 改 schema · `tests/e2e/kind/*/assert.sh` 加 v1 flat attributes(vs v1beta1 nested basic.attributes)兼容。**substrate ship**(tests pass)· 但 5 module Go code 仍用 v1beta1:

| Module | v1beta1 use | v1 migration effort |
|---|---|---|
| operators/pool-operator | NPUPool.status.hccsTopology 聚合 read ResourceSlice | M(client lister regen + status surface 改字段路径) |
| operators/npu-dra-driver | ResourceSlice emit(claim_controller AllocateBundle path)| H(controller logic + scheme registration + envtest) |
| operators/inference-operator | ResourceClaim 创建 path | M |
| operators/scheduler-plugin | DRA framework lookup | M(framework v0.32+ 已支持 v1 · plug-in 端可能需要 import path 更新) |
| backend/pkg/datasource/k8s + crd | ResourceSlice / ResourceClaim list | M |

**Migration cohort plan**(Phase 12+/M6):
1. **Lockstep K8s baseline bump**(1.34.3 → 1.36+ · per ADR-0016 §3 Stream 6)
2. **Per-module v1 client lister regen**(controller-gen + go mod tidy · 5 module 各自 PR)
3. **envtest update**(5 module 各自)
4. **真集群 verify**(kind cluster + helm install + Reconcile loop 真触发 · 与 Partitionable Devices Track C 同期 if Track C land)
5. **Phase 12+ tag** 后 v1beta1 backward compat shim 保留 N phase · 然后 deprecate

**Cohort estimate**:1.5-2 month(同 baseline bump + multi-module concurrent migration · 跨 5 module + envtest + 真集群 verify)。

## docs/devlog index update outline

Phase 11 共 18 task devlog + 3 fix devlog + 1 charter devlog = **22 entries**(per `ls docs/devlog/ | grep "^phase-11"`)。`docs/devlog/README.md` §1 enforcement scope 已 cover · Phase 11 commit footer 100% 含 `Devlog:` line(P11-T-001 起严格执行)。

`docs/checkpoint-phase11.md`(T204 起草)__在 §1 Deliverables 表中 cross-ref 22 devlog__:

```
W1 Foundation(8 task)
├── T001 ADR-0017 · 548144d
├── T002 ADR-0018 · c65283c
├── T003 demo-backend chart · 1a46edc
├── T004 IMS-1 chart · aedb1a0
├── T005 IMS-2 chart · c5998d6
├── T006 IMS-3 chart · d544bc0
├── T007 sched-plugin NRT bundle · 7e8f026
└── T008 inference-operator env wire · b11573d

W2 Polish + decision-gated + frontend + chart wiring(8 task)
├── T101 LAB 5th carry · 6f57139
├── T102 Karmada bootstrap · 648bd68
├── T103 Karmada PropagationPolicy · 7d170fb
├── T104 ClusterQuota CRD · 637ff87
├── T105 Frontend 3 indicators · e6b9de4
├── T106 O2 DMS authn chart · 27e83b9
├── T107 kind smoke ext · 4925e86
└── T108 Volcano 3rd defer · 77c025a

W3 Closer(4 task)
├── T201 master-demo-multi-site.sh · d4d471b
├── T202 Partitionable Devices 2nd defer · 905838e
├── T203 docs 大整理 + Phase 12+ 前瞻(本 commit)
└── T204 checkpoint + tag phase-11-complete · 下一 commit
```

## arch §13 review-table promote outline(留 T204 一并 update)

Phase 11 row 当前 "in flight via P11-T-001..T204(2026-05-22 起)" · T204 起草 checkpoint 时 update 为 "**landed phase-11-complete (2026-05-22)** — 20 task chain T001-T204 · M5 真生产化 foundation milestone CLOSER · ..." 详尽 deliverable 摘要(同 Phase 10 row pattern · 含 6 chart packaging + Karmada 3-cluster + Frontend 3 indicators + 3 deferred outcomes:T101 5th carry + T108 3rd carry + T202 2nd carry)。
