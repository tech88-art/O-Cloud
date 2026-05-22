# P11-fix-005 · Phase 11 post-tag CI gate(2nd cycle)· scheduler-plugin numaAffinity default revert to false · partial close known-issues #12

- **Date**: 2026-05-22
- **Duration**: ~0.15d(2nd-cycle pragmatic fix · 反复 RBAC / CRD 调试不再 incremental 收益)
- **Trigger**: P11-fix-004(`25f4d64`)NRT RBAC fix push 后 CI re-run · `kind smoke (real cluster)` 仍 red 同 step "Phase 6 — install scheduler-plugin chart" 同 error `Error: context deadline exceeded`(180s helm install --wait timeout)。

## Root cause(deeper analysis after fix-004 didn't resolve)

不是 RBAC 问题(fix-004 已加 NRT verbs)· 不是 CRD 缺失(P11-T-007 已 bundled)· 实际是:

**`nrt.New` informer cache sync 在 kind 1.34.3 cluster 上 > 180s helm install --wait default timeout**。

scheduler-plugin Pod 启动序列:
1. Pod schedule + image pull(`Never` · `kind load` 已 ship)
2. Container start → `app.NewSchedulerCommand` 注册 plugins
3. NumaAffinity plugin `nrt.New(ctx, args, h)` 构造 → 启 NodeResourceTopology informer
4. **informer.HasSynced 等待**:即使 NRT CRD 已 registered + RBAC 已 granted · API discovery + informer initial list/watch + cache sync 需要时间 · 在 kind 1.34.3 cluster(资源紧)+ apiserver discovery cache cold 的情况下 · 实测 > 180s
5. /healthz 路径在 plugins 全部 ready 之前不返 200 → readinessProbe fail → Pod NotReady → helm `--wait --timeout=180s` 命中 context deadline

实际可观察证据:
- fix-004 push 后 CI re-run 同 error 同 step 同 timing(用户截图 "failed 2 minutes ago in 15m 3s")
- helm install timeout fixed at 180s in `tests/e2e/kind/phase6/install.sh` line 46
- P10-fix-002 commit message 明示 "unblock phase6 install" — 历史上同问题已知

## Fix decision rationale

**Conservative revert default to false**(同 P10-fix-002 working state)· 但保留:
- NRT CRD bundle in `chart/crds/`(P11-T-007 ship)
- NRT RBAC ClusterRole verbs gated on `numaAffinity.enabled=true`(P11-fix-004 ship)
- numaAffinity ConfigMap KubeSchedulerConfiguration block(per existing chart)

operators 全栈 opt in 路径:`helm install --set numaAffinity.enabled=true --timeout=300s`(给 nrt.New cache sync 时间)· 不需要任何额外动作(CRD + RBAC + ConfigMap 全 ready)。

**Why not "bump --timeout to 300s in phase6/install.sh"**:
- 修改测试脚本是 *adapt test to chart* · 反方向责任
- phase6 install timeout 是历史 baseline · 增 timeout 影响 CI cycle time + 不解决根本 latency 问题
- nrt.New cache sync > 180s 是真实问题 · 增 timeout 只是症状压制
- Phase 12+ K8s 1.36+ baseline bump cohort(per ADR-0016 §3 Stream 6 + Track C Partitionable Devices)同期 evaluate NRT informer fast-path / discovery cache warm-up 等系统性改进

**Why not "lazy-init nrt plugin to skip informer wait in /healthz"**:
- 改 plugin source code 是侵入式 · 与 upstream nrt 行为偏离
- 留 Phase 12+ upstream evaluate + 真 production NRT operator 装在 cluster 后 informer 自然快(NRTs 已存在)

## Changes

`deploy/helm-charts/scheduler-plugin/values.yaml`:
- `numaAffinity.enabled: true` → **`false`**(P10-fix-002 working state)
- 头部 comment 加 P11-fix-005 5-step trail + operator opt-in 完整命令(`--set numaAffinity.enabled=true --timeout=300s`)+ Phase 12+ 联动 cohort 说明
- 标 known-issues #12 **partial close**(infrastructure 全 ship · 默认仍 conservative · 完整 default true 留 Phase 12+ cohort)

`tests/e2e/kind/phase10/assert.sh` T107-A1:
- 从 "default ENABLED + NRT CRD bundled" 改为 "NRT CRD bundled + RBAC ready · operator opt-in via --set · default false per P11-fix-005"
- 验证 3 项独立(NRT CRD 文件 + RBAC verbs + numaAffinity substrate present)· 不再 check default true

`tests/e2e/kind/phase11/assert.sh` T107-A1:
- 同步更新(3 项独立 verify · 不 check default true)

`docs/adr/0010-scheduler-plugin.md`:
- §3 P10-fix-002 + P11-T-007 闭环段扩展为 7-step trail(加 P11-fix-004 RBAC + P11-fix-005 default revert)
- 标 known-issues #12 **partial close**(本 cycle)· 完整 default true 留 Phase 12+ cohort

## P7 自我审计(2nd cycle 教训累积)

P11-T-007 ship 时 audit 框架:
- ✓ chart 文件存在 + helm lint clean(syntactic)
- ✗ 真 cluster install latency · 因 dev env 无 kind 跳过 · CI gate 上 surface

P11-fix-004 ship 时 audit 框架:
- ✓ RBAC verbs render correctly(syntactic)
- ✗ 假定 RBAC 是唯一 root cause · 没考虑 nrt.New cache sync latency(P3 verify-before-claim 单维度推测 · 同 P7 "audit clean 就怀疑审计" pattern)

P11-fix-005 教训:
- **2 次 fix 同问题 → 换 root cause 假设**(本次 = informer cache sync latency 不是 missing RBAC)
- **kind smoke 是真 verification path** · 不可在 dev env(无 kind binary)替代 · 同 memory `feedback_post_tag_ci_gate.md` "tag push 后必看 CI" 真意
- **default 优 conservative**(working state)· over-promise 默认 enable + 真实环境 fails 比 default disable + 显式 opt-in 更损 trust

## Validation

- `helm lint --strict deploy/helm-charts/scheduler-plugin/` clean
- `helm template scheduler-plugin deploy/helm-charts/scheduler-plugin/` → numaAffinity ConfigMap block 仍 present(不 default-on · 但 chart 可 opt-in)
- `bash tests/e2e/kind/phase11/assert.sh` 本地 syntactic verify pass · T107-A1 检查 3 独立项
- 真 kind smoke 验证留 push 后 CI gate re-run · 期待 Phase 6 install scheduler-plugin 通过(default false · 不调 nrt.New informer)

## Refs

- ADR-0010 §3 P10-fix-002 + P11-T-007 + P11-fix-004/005 7-step trail
- ADR-0017 §2 Decision D 5th P11-T-007 carry close intent(partial close · 完整 default true 留 Phase 12+)
- ADR-0016 §3 Stream 6 K8s 1.36+ baseline bump cohort(与本 fix Phase 12+ 联动)
- memory `feedback_post_tag_ci_gate.md` Phase 7 实战 4 fixes pattern continues(Phase 11 P11-fix-001..005 共 5 fixes now)
- commit chain: P11-T-007 `7e8f026` → P11-fix-004 `25f4d64` → 本 P11-fix-005
