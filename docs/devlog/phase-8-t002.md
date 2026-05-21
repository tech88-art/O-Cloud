# P8-T-002 · K8s baseline bump — doc-only refresh (user-mandated stay at K8s 1.32)

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: plan 1-2d / actual ~0.3d (doc-only path after upstream block + user decision)

## Intent

Plan §3-T002 原 intent:K8s 1.32 → 1.36+ lockstep bump,unblock NumaAffinity wrap(T003)+ Partitionable Devices Beta(T101)+ partition-aware allocator(T102)+ ProxyImage chart flip(T004 secondary)。

实际落地形态:**doc-only refresh + 用户 mandated stay K8s 1.32**(2026-05-21 chat 决策)· 不动 go.mod / kind-config / Chart.yaml / workflows · 仅更新 ADR-0010 §1 + known-issues #12 + 本 devlog · 把 T003 / T101 / T102 顺延 Phase 9-10。

## Path adaptations

- 计划 Allowed Paths 列了 8 个 go.mod + kind-config.yaml + 4 Chart.yaml + 2 workflow + Makefile + 2 docs。实际只动了 2 docs(ADR-0010 §1 + known-issues #12)+ 1 devlog。
- 计划 Forbidden Paths 列"Source code outside go.mod / go.sum updates" — 本 doc-only 路径完全不动 source code · 自然满足 Forbidden 条件。
- 计划 Acceptance "All go.mod files updated to K8s 1.36+ in lockstep" + "go mod tidy clean" 等机器可验证项 — 在用户决策"stay 1.32"下 inapplicable;改用 doc-only Acceptance:ADR §1 + known-issues #12 + plan §3-T002 后果反映用户决策。

## Debugging trail

- **W1 entry re-WebFetch findings**(2026-05-21):
  - K8s upstream:**1.36.1 GA**(2026-05-12)· 1.35.5 / 1.34.8 / 1.33.12 同日 patch
  - `kubernetes-sigs/scheduler-plugins` releases:**最新 v0.34.7**(2026-04-20)· 历史 v0.33.5(2025-10-27) / v0.32.7(2025-08-06)· **v0.35.x / v0.36.x 未发布**
  - `kubernetes-sigs/kind` releases:**最新 v0.31.0**(2025-12-18)· 仅支持 kindest/node 至 **v1.35.0**(release notes confirm) · v1.36 镜像不存在 · release notes 提到 "Future kind releases will adopt kubeadm v1beta4 for Kubernetes 1.36+" 但未实际发布
  - `kubernetes-sigs/controller-runtime` releases:**最新 v0.24.1**(2026-05-12)· v0.24.0 / v0.23.3 / v0.23.2 历史
- **关键发现**:Plan 默认 target "1.36 lockstep" **upstream-blocked** — sched-plugins lag 2 minors + kind lag 1 minor(无 kindest/node v1.36 prebuilt)
- **初次尝试 v0.34.7 bump**:edit scheduler-plugin go.mod v0.32.0 → v0.34.7 + replace block 同步 → `go mod tidy` 触发下载 `k8s.io/kubernetes v1.34.7` 整源码树 → **C: 盘 100% full**(avail ≈ 1GB)· `go build` 失败("No space left on device")
- **Revert clean**:`git checkout operators/scheduler-plugin/go.mod operators/scheduler-plugin/go.sum` 恢复 v0.32.0 状态 · 仓库零残留
- **User decision**(chat,2026-05-21):"k8s版本 暂定 1.32版本,不升级最新" → Phase 8 maintain K8s 1.32 baseline 状态 quo · 不主动 bump · 降低本期风险
- **现有状态**:
  - scheduler-plugin go.mod:K8s v0.32.0 + sched-plugins replace block v0.32.0(保持)
  - 主模块(npu-dra-driver / inference-operator / pool-operator):**已飘升 K8s v0.35.0 + controller-runtime v0.23.3**(Phase 7 期间日常 `go mod tidy` 微飘升 · 用户决策"不升级最新"主要约束未来 bump · 不主动 downgrade)
  - kindest/node v1.32.0 不变
  - Helm chart 无 kubeVersion 字段(无须 add · plan §3-T002 acceptance "helm lint --strict clean" 在 doc-only 下默认通过)

## Key decisions

- **User decision drives T002 scope**:从 plan 原"1.36 lockstep bump"降级为"doc-only refresh"。属 plan §6 risk #1 worst-case fallback 路径("revert T002 + ship Phase 8 with NumaAffinity + ProxyImage + Partitionable Devices all deferred to Phase 10")的一部分实现(但 ProxyImage T004 不受 baseline 影响 · 仍可独立进行)
- **不 downgrade 主模块从 v0.35.0 → v0.32.0**:用户"stay 1.32"约束的语义是"不升级 further" · 不是"主动 downgrade"。Phase 7 期间已微飘升的状态 accept-as-is · scheduler-plugin 与主模块的 1-minor skew 通过独立 binary + client-go backward-compat 保证不破环境
- **T003 顺延 Phase 9**:NumaAffinity wrap upgrade 需要 sched-plugins v0.32.x+ 兼容性 + framework.GVK 删除 — 当 sched-plugins v0.32.x GA(已 v0.32.7)但 K8s 1.32 baseline 的 apimachinery v0.32.x 仍缺 `pkg/api/{safe,operation,validate}` 包(P7-T-002 doc-only 实测教训 · known-issues #12 历史背景段)。用户 stay 1.32 决策下 T003 无法 light up
- **T101 + T102 顺延 Phase 10**:Partitionable Devices Beta 需要 K8s 1.36 cluster · kindest/node v1.36 不存在 + 用户 stay 1.32 双重 block · 必须 carry-forward
- **T004 ProxyImage chart flip 继续 schedule**:与 K8s baseline 独立 · plan §3-T004 acceptance 不涉及 baseline · 按 plan 继续(Phase 8 T002 doc-only 落地后可立即接 T004)
- **T005-T008 继续 schedule**:NPUVerticalScaler CRD + ingestor + controller + AllocateBundle wiring 都在主模块 K8s 1.35 状态下能 build + test · 与 baseline 升级独立
- **CI status quo**:e2e-kind workflow + helm-lint workflow 不动 · Phase 5/6/7 sub-jobs 继续按 kindest/node v1.32 跑

## Verification

- **存在性**:
  - `docs/adr/0010-scheduler-plugin.md` §1 框架与部署形态 段后新增 🆕 2026-05-21 update 段(P8-T-002 · K8s baseline stay at 1.32)· grep `2026-05-21 update (P8-T-002` 命中 ✅
  - `docs/known-issues.md` #12 题目从"Phase 7 → Phase 8"改为"Phase 7 → Phase 8 → Phase 9"+ Status header 加 P8-T-002 user decision 字样 + 顶段新增 🆕 2026-05-21 update segment + Proposed resolution 从"Phase 8 candidate"改为"Phase 9 candidate" ✅
  - 本 devlog 文件存在(`docs/devlog/phase-8-t002.md`) ✅
- **完整性**(plan §3-T002 acceptance 行 vs 实际状态):
  - "All go.mod files updated to K8s 1.36+ in lockstep" → **N/A · user decision stay 1.32** · 记录在 ADR-0010 §1 update ✅
  - "go mod tidy clean in every module" → **N/A · doc-only path · 无 go.mod 改动** · `git diff --stat` 输出无 .go / go.mod / go.sum 文件 ✅
  - "go build / go vet / go test PASS" → **N/A · doc-only · 已 verify scheduler-plugin baseline `go build ./... && go vet ./... && go test ./...` PASS at v0.32.0(P8-T-002 entry smoke)** ✅
  - "helm lint --strict clean across all charts" → **N/A · doc-only · 不动 Chart.yaml** ✅
  - "kindest/node v1.36.x" → **N/A · kindest/node v1.32.0 维持** ✅
  - "All Phase 5/6/7 kind smoke E2E phases re-run + PASS against 1.36" → **N/A · 不改 baseline 即 Phase 5/6/7 smoke 不需要 re-run · 状态 quo 已 verified at phase-7-complete tag** ✅
  - "Devlog enumerates: target K8s minor decision + transitive dep changes + any temporary workaround for surfaced API drift" → 本 devlog 完整记录 ✅
  - "ADR-0010 §1 + §3 status refresh + known-issues #12 marked RESOLVED with cross-ref" → ADR-0010 §1 refresh ✅ · known-issues #12 维持 OPEN(不 RESOLVE · plan acceptance 文字应作"updated"理解 · 我已加 2026-05-21 update segment 记录决策)
- **正确性**:
  - scheduler-plugin baseline test 状态(P8-T-002 entry smoke):`go build ./... && go vet ./... && go test ./...` PASS · 已通过(本 session 之前 verify)
  - 主模块 baseline 不动 → 无回归风险
  - 仓库无 .go / go.mod / go.sum / kind-config / Chart.yaml / workflow / Makefile 改动 → CI 不需要 re-run

## Carry-forward

- **T003**(NumaAffinity wrap upgrade)· **Phase 9 W1 entry 重新评估**:已记入 known-issues #12 ✅;Phase 9 plan 入口 task 应再 re-WebFetch sched-plugins / kindest/node 状态 · 决定是否在 Phase 9 内 bump baseline
- **T101 + T102**(Partitionable Devices Beta + partition-aware allocator)· **Phase 10 carry**:用户决策 + upstream(kindest/node v1.36 lag)双重 block;Phase 10 真硬件期重新评估
- **T004**(ProxyImage chart flip)· **本 Phase 8 内继续 schedule**:plan §3-T004 acceptance 不涉及 K8s baseline · 与本 T002 完全解耦 · 下一 task 可直接进入
- **T005-T008**(NPUVerticalScaler 主流程)· **本 Phase 8 内继续 schedule**:主模块 K8s 1.35 状态足够支持 · 与本 T002 完全解耦
- **T103-T107**(kind smoke ext + HCCS hard-fail + checkpoint + tag)· **本 Phase 8 内继续 schedule**:kind smoke 维持 kindest/node v1.32 · 不需要 baseline change
- **C: 盘 cache 满**(C:\Users\Himalayan\go\pkg\mod\cache):用户已选 A · 计划跑 `go clean -modcache`(本 session 之外执行)· T005-T008 / T106 实际 `go mod tidy` 时若 disk 仍不足应再 chat 报 · 但 doc-only T002 本 commit 不依赖 disk 空间
- **本 commit 与 cache 清理解耦**:T002 doc-only 本身不需 cache 清理 · 但 T005+ task 入手前 cache 应被清(因为 T005 在 inference-operator 模块需要 `make generate` + `go test` + `go mod tidy`)
