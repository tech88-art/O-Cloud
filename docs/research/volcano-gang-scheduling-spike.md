# Volcano gang-scheduling integration spike (Phase 8 P8-T-106)

- **Status**: Spike (docs only)
- **Date**: 2026-05-21
- **Phase**: 8 W2 · per plan §3 P8-T-106
- **Related**: ADR-0010 §7 (Phase 7+ forward notes · Volcano row) · arch §13 Phase 9 row · `docs/cni-hccl-research.md` §5 (live migration gap forward notes)

---

## §1. Volcano PodGroup CRD shape

Volcano's gang-scheduling primitive is the **PodGroup CRD**
(`scheduling.volcano.sh/v1beta1.PodGroup`). It declares "this set of N
Pods MUST be co-scheduled atomically" — if Volcano cannot place all N
simultaneously, none start.

**Spec shape**(operative · per upstream `scheduling.volcano.sh/v1beta1`):

```yaml
apiVersion: scheduling.volcano.sh/v1beta1
kind: PodGroup
metadata:
  name: gang-llama-distributed-train
  namespace: ocloud-system
spec:
  minMember: 8              # exact number of Pods required (atomicity gate)
  minResources:             # cluster-wide resource floor for scheduling decision
    "huawei.com/Ascend910": 8
    cpu: 16
    memory: 64Gi
  queue: default            # which Volcano queue this PG belongs to
  priorityClassName: ""
  minTaskMember:            # per-task type minimums (e.g. 1 master + 7 workers)
    master: 1
    worker: 7
status:
  phase: Running            # enum: Pending | Running | Inqueue | Completed | Failed
  conditions:
    - type: Scheduled       # True once all N Pods placed atomically
      status: "True"
  running: 8
  succeeded: 0
  failed: 0
```

Pods opt into a PodGroup via annotation:

```yaml
metadata:
  annotations:
    scheduling.k8s.io/group-name: gang-llama-distributed-train
```

Volcano's scheduler observes (PodGroup + member Pods) holistically;
Pods without the annotation go through Volcano's default scheduling
loop (single-Pod first-fit).

---

## §2. Inference (Phase 8 demo) vs Training (Phase 9 candidate) scope

**Inference workload**(Phase 8 demo · qwen-pd-pair single-replica):

- 1 Prefill Pod + 1 Decode Pod = 2 Pods
- HCCS communication intra-NPU (same physical card)
- Per-Pod independent ResourceClaim allocation
- **No gang needed**:Pods can start in any order; pair-of-2 fits trivially in any pool with ≥ 2 NPUs
- Phase 8 single-tenant scope · multiple ModelServices don't compete for atomicity
- arch §13 Phase 8 row 调研 conclusion("重启切片"/ vertical restart)与 gang scheduling **不冲突也不依赖** · NPUVerticalScaler 走 spec patch → rolling restart 路径 · 不需要 PodGroup atomicity

**Training workload**(Phase 9+ candidate · cross-node HCCL collectives):

- N Pods spanning multiple nodes(典型 8-32 Pods for 70B-class training)
- HCCL allreduce/allgather/broadcast 跨节点 · 任一 Pod 缺席 → 全 group 阻塞 / collective 超时
- **Gang needed**:atomicity guarantees all-or-nothing scheduling · 避免"部分 Pod schedule 但等不到 sibling Pod" deadlock
- Cross-node RDMA bandwidth + topology constraints(per ADR-0007 fabric discovery + cni-hccl-research §2)
- Volcano PodGroup + minMember + minResources 是该场景的 standard solution

---

## §3. npu-scheduler + Volcano coexistence model

Per ADR-0010 §1 npu-scheduler 是独立 kube-scheduler binary(profile name `npu-scheduler`)· 不修改 default-scheduler · Pods 显式 opt-in via `spec.schedulerName`。Volcano 同样是独立 scheduler 实例(profile name `volcano`)。

**Co-existence pattern**(multi-scheduler · K8s 原生支持):

| Pod 类型                            | spec.schedulerName | Notes                                                        |
|-------------------------------------|-------------------|--------------------------------------------------------------|
| Phase 8 inference PD-pair Pod      | `npu-scheduler`   | P7-T-003 auto-stamp · HCCS-aware Filter+Score per ADR-0010 §2 |
| Phase 8 demo workload(non-NPU)    | `default-scheduler` (默认) | 未显式 opt-in 走 default                                       |
| Phase 9 训练 job Pod                | `volcano`         | 显式 opt-in · 走 PodGroup gang 路径                            |
| Phase 9 实时推理(latency-sensitive) | `npu-scheduler`   | 同 Phase 8 · 不需要 gang                                       |

**3 schedulers 同时运行**(kube-system / 各 chart):
- default-scheduler(K8s 自带 · 总在线)
- npu-scheduler(operators/scheduler-plugin chart · Phase 6 起 ship)
- volcano-scheduler(Phase 9 引入 · 单独 chart 或 native helm install volcano)

每 scheduler binary 独立 watch ResourceClaim / Pod / Node · 不互相 conflict · K8s api-server scheduling-name dispatch 保证 Pod 只被 1 scheduler reconcile。

**为什么不在 npu-scheduler 内 native PodGroup**:

- Volcano 已有成熟 PodGroup admission + queue/quota/preemption 全栈逻辑(数年生产验证)
- npu-scheduler 关注 NPU 拓扑层调度(HCCS Filter+Score · NumaAffinity wrap · Binpack)· 范围 narrow
- 2 scheduler 解耦 ownership 清晰:Volcano = workload-level atomicity · npu-scheduler = device-level topology
- 升级路径:Phase 9 训练场景启动时 install volcano · 不动 npu-scheduler binary · 不引入 plugin framework 兼容性风险

---

## §4. Phase 9 migration cost matrix

| 路径                                      | 工作量(d) | Phase 9 风险                     | 收益                            |
|-------------------------------------------|-----------|----------------------------------|---------------------------------|
| **A · 引入 Volcano binary**(独立 helm)   | 1-2d      | Volcano CRD 与既有 K8s 1.32 兼容(确认 Volcano v1.10.x+ 支持) · 多 scheduler 运维复杂度 | 训练 job gang 即 work             |
| **B · npu-scheduler 内 native PodGroup** | 4-6d      | sched-plugins framework 整合 PodGroup + ResourceClaim 兼容性 · 4-6d 工作量大于 A · 测试成本高                | 单一 scheduler · 运维简单         |
| **C · 推迟到 Phase 10+**(只 inference)  | 0d        | 训练场景 cluster 上线时无 gang 能力 · 部分 collective 超时 | 0 风险 · 简化 Phase 9 scope       |

**Phase 9 W1 entry decision matrix**(同 ADR-0011 §3 lab gating spirit):

| 用户 W1 entry 信号                     | 路径选择             | Notes                                            |
|---------------------------------------|---------------------|--------------------------------------------------|
| "Phase 9 训练 demo 需要 gang"          | A · 引入 Volcano binary | 1-2d 工作量 · 在 Phase 9 calendar 内可吸收         |
| "Phase 9 只 inference 演进 · 不引入训练"| C · defer Phase 10+ | 0 风险路径 · Phase 8 NPUVerticalScaler 继续主轴   |
| 无明确信号(default)                  | C · defer            | 保守路径 · 同 lab gating default 政策             |

**推荐路径**:A(引入 Volcano binary)· 训练 job 是 Phase 9-10 lab smoke 自然需求(70B model training scenarios)· Volcano 是行业 standard · 不重新发明轮子。

---

## §5. Cross-node HCCL training-job scenario(scope reference)

Phase 9 训练场景的 typical workload(per `docs/cni-hccl-research.md` §5 forward notes):

- **Model parallel**(70B class):8 Pods × 1 NPU each · TP=8 sharding
- **Pipeline parallel**:N stages × M Pods each · GPipe / 1F1B 调度
- **Data parallel**:N replicas of full model · gradient allreduce across all
- **HCCL collectives**:allreduce / allgather / reducescatter / broadcast · 跨节点走 RDMA(per ADR-0007 fabric discovery)
- **Rank co-placement**:HCCL ring 通信 · rank 顺序敏感 · gang atomicity 保证所有 rank Pod 同时在线后才启动 collective(避免 rank N 等 rank N+1 超时)

**Topology constraints**:

- 同 Pod 同 NPU 优先(HCCS intra-card 通信 0 cost)
- 同节点不同 NPU 次优(HCCS intra-node · 100GB/s class)
- 跨节点 RDMA 第三选项(InfiniBand HDR/NDR · 100-400 Gb/s class)
- 跨 leaf switch 最差(经 spine layer · 增加 1 hop latency · 减半 bandwidth)

npu-scheduler 当前 Filter+Score(ADR-0010 §2)只 cover NPU device 级 + HCCS ring 级。leaf-switch 拓扑感知 deferred to Phase 9+ per ADR-0007。Volcano gang scheduling 与 topology scoring 解耦:Volcano 保证 atomicity · topology choice 仍 by NPU-scheduler Filter+Score(if Pod opts in via dual `schedulerName=npu-scheduler` + PodGroup annotation · 但实际 K8s 限制单 scheduler per Pod · 因此训练 Pod 走 Volcano 而不走 npu-scheduler)。

**实际 Phase 9 实施 architecture**:

```
Training Pod
├── schedulerName: volcano
├── annotations:
│   ├── scheduling.k8s.io/group-name: gang-llama-7b-train
│   └── ocloud.edge.example.com/topology-preference: same-leaf  # Phase 9 候选
└── PodGroup{ minMember: 8, minResources: { huawei.com/Ascend910: 8 } }
                ↓
        Volcano scheduler
            ├── enqueue
            ├── wait until 8 Pods + cluster has 8 NPUs available
            └── place 8 atomically (best-effort NUMA + HCCS within Volcano's
                pluggable scoring; Phase 9 may write a Volcano plugin that
                consumes the same HCCS adjacency map as npu-scheduler)
```

---

## §6. References

- Volcano upstream:https://volcano.sh/ · CRD docs https://volcano.sh/en/docs/podgroup/
- Volcano GitHub:https://github.com/volcano-sh/volcano(latest stable v1.10.x as of 2026-05-21)
- ADR-0010 §1(scheduler-plugin 独立 binary)+ §7(Volcano 行 forward note · "当前 Phase 6 不引入 — 推理场景 PD-pair 2-4 个 Pod 不需要 gang")
- arch §13 Phase 9 row · O2 DMS + multi-tenancy + Volcano spike candidate
- `docs/cni-hccl-research.md` §5(live migration gap · 跨节点 HCCL rank rebind forward notes)
- `docs/adr/0007-fabric-discovery.md`(leaf-switch topology · Phase 9+ extension)
- `docs/research/k8s-partitionable-devices-spike.md`(KEP-4815 · Phase 10 partition-aware allocator协同)

---

**END of Volcano gang-scheduling spike**
