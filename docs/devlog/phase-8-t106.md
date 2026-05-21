# P8-T-106 · Volcano gang-scheduling integration spike

- **Commit**: (this commit)
- **Date**: 2026-05-21
- **Duration**: plan 0.5d / actual ~0.3d

## Intent

Docs-only research spike per ADR-0010 §7 forward note "Volcano gang-scheduling 整合"and arch §13 Phase 9 row。落地的内容:
- 6-section spike doc `docs/research/volcano-gang-scheduling-spike.md`:
  §1 PodGroup CRD shape · §2 inference(Phase 8 demo)vs training(Phase 9 candidate)scope · §3 npu-scheduler + Volcano coexistence model(multi-scheduler · 3 scheduler 同时运行)· §4 Phase 9 migration cost matrix(3 paths · 推荐 A · Volcano binary)· §5 cross-node HCCL training-job scenario topology constraints · §6 references
- ADR-0010 §7 Volcano 行 forward note refresh:加 "2026-05-21 P8-T-106 spike landed · 推荐 Phase 9 W1 entry 路径 A"
- arch §13 Phase 9 row refresh:加 "训练大批量 job 场景 P8-T-106 spike landed"+cross-ref 到 spike doc

## Path adaptations

- Plan §3-T106 Allowed Paths covered:
  - `docs/research/volcano-gang-scheduling-spike.md`(new · 6 sections · ~230 行) ✅
  - `docs/adr/0010-scheduler-plugin.md` §7 Volcano 行 forward note refresh(同列加 P8-T-106 spike 引用) ✅
  - `docs/architecture.md` §13 Phase 9 row 加 Volcano spike cross-ref ✅
  - `docs/devlog/phase-8-t106.md`(this file) ✅

No code changes per plan §3-T106 NO code change 约束。

## Debugging trail

- 无 false start。Volcano upstream knowledge 直接写 · spike doc 一次成稿
- 第一次 architecture.md §13 Phase 9 row Edit 失败 — 我 search `Phase 9 (O2 DMS)` 行 verbatim · 通过

## Key decisions

- **推荐 Phase 9 路径 A**(引入 Volcano binary)而非 B(npu-scheduler internal PodGroup)或 C(defer Phase 10+):
  - A:1-2d 工作量 · 训练 job 立即 work · 行业 standard · 不重新发明轮子
  - B:4-6d 工作量 · sched-plugins framework 整合 PodGroup + ResourceClaim 兼容性高 · 不值得
  - C:0 风险 · 但 Phase 9 训练场景 cluster 上线时无 gang 能力 → 部分 collective 超时 · 用户体验差
- **3 scheduler 同时运行 architecture**(default-scheduler + npu-scheduler + volcano):K8s 原生支持 · Pod 显式 opt-in via `spec.schedulerName` · 3 scheduler 独立 watch · 互不冲突。这是 npu-scheduler 设计的延续(ADR-0010 §1 独立 binary)· 不需 redesign
- **Volcano 与 npu-scheduler 解耦 ownership**:Volcano = workload-level atomicity / queue / quota / preemption · npu-scheduler = device-level topology(HCCS Filter+Score · NumaAffinity · Binpack)· 训练 Pod 走 Volcano(放弃 npu-scheduler 的 NPU 级 score)· Phase 9 may 写 Volcano 自定义 plugin 让 Volcano 也消费同样 HCCS adjacency map · 但 Phase 8 spike 不细化
- **Phase 9 W1 entry decision matrix**(同 ADR-0011 §3 lab gating spirit):用户显式 signal "需要训练 gang" → A;只 inference → C defer;无信号 → C default。让 Phase 9 W1 user 选 · 不预判
- **Training topology constraints 详细写**(§5):同 Pod 同 NPU → 同节点不同 NPU → 跨节点 RDMA → 跨 leaf switch 4 层 cost。Phase 9 if 引入 leaf-switch 拓扑(ADR-0007 fabric discovery)· Volcano plugin 自定义 score 可消费

## Verification

- **存在性**:
  - `docs/research/volcano-gang-scheduling-spike.md` 6 sections ~230 行 · `grep -n '^## §' docs/research/volcano-gang-scheduling-spike.md` 6 命中 ✅
  - `docs/adr/0010-scheduler-plugin.md` §7 Volcano 行 加 P8-T-106 spike 引用 ✅
  - `docs/architecture.md` §13 Phase 9 row 加 Volcano spike cross-ref ✅
  - `docs/devlog/phase-8-t106.md`(this file) ✅
- **完整性**(plan §3-T106 acceptance vs 实际):
  - Spike doc enumerates:(a) Volcano PodGroup CRD shape + status conditions ✅ · (b) inference workload Phase 8 不需 gang · training workload Phase 9 需 gang ✅ · (c) coexistence model multi-scheduler 3-binary · Pod opt-in via spec.schedulerName ✅ · (d) Phase 9 cost matrix native PodGroup vs Volcano binary 3 路径 ✅ · (e) HCCS rank co-placement requirements for cross-node HCCL ✅
  - ADR-0010 §7 + architecture §13 cross-references updated ✅
  - NO code change · spike is doc-only research per Phase 7 T106 precedent ✅
- **正确性**:
  - Volcano upstream knowledge cross-ref'd via 6 references list(spike doc §6)· upstream URL + GitHub repo URL 写明
  - Coexistence model 与 ADR-0010 §1 独立 binary 设计 align · 不引入 architecture 冲突
  - Phase 9 cost matrix 3 路径 各自工作量评估 1-2d / 4-6d / 0d · plan §3-T106 acceptance "Phase 9 cost matrix" 实质兑现

## Carry-forward

- **Phase 9 W1 entry decision**:用户在 Phase 9 入口 meeting / chat 显式选 path A vs B vs C
- **Phase 9 if A 路径**:
  - `deploy/helm-charts/` 加 volcano subchart 或 native install volcano via `kubectl apply` from upstream YAML
  - inference-operator deployment_builder 加 schedulerName "volcano" 路径(训练 ModelService 类 CRD if 引入 · 或扩展 ModelService spec 加 trainingJob 字段)
  - kind smoke phase9/install.sh 加 volcano install + PodGroup fixture + gang assertion
- **Phase 9 if A + 自定义 plugin**:写 Volcano scheduler plugin 消费 ADR-0010 §5 HCCSTopologyArgs 同样 adjacency map · npu-scheduler binary 不必动
- **Phase 9 if C 路径**:本 spike doc 仍是 Phase 10+ 的 reference · 不丢失
