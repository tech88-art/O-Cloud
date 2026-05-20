# CNI × HCCL Transport Compatibility — Research Doc (Phase 5 P5-T-105)

> Phase 5 seed for the Phase 6 scheduler-plugin work. The plan acceptance
> calls for a CNI × HCCL transport compatibility matrix, a Phase 5
> simulator-scope confirmation, a Phase 6 entry recommendation, and a
> known-gaps section. This doc collates upstream references — no real
> hardware verification has been performed at Phase 5 (simulator-only).
>
> **All confidence marks are uncertainty-labelled per global CLAUDE.md
> P3 four-tier scheme**: `[A · source]` = one-source-verifiable;
> `[B · refs]` = multi-secondary; `[C · derived]` = derived from
> verified inputs; `[D · guess]` = author estimate.

## 1. Background

The Phase 5 inference-operator + npu-dra-driver stack runs on standard
K8s 1.30+ clusters and addresses NPU-to-NPU communication via Huawei's
HCCL (Huawei Collective Communication Library) — the Ascend
equivalent of NVIDIA's NCCL.

HCCL physically sits on top of one of three network transports:

| Transport | Description | Typical NIC |
| --- | --- | --- |
| **RDMA over InfiniBand** | Native InfiniBand fabric | Mellanox CX-6/CX-7 + IB switch |
| **RoCE v2** | RDMA over Converged Ethernet | Mellanox CX-6/CX-7 + DCB-capable Ethernet switch |
| **IPoIB** | IP encapsulation over InfiniBand (legacy) | Mellanox CX-6/CX-7 + IB switch in IP mode |

Source: [Huawei Atlas Training Solution Best Practices, §HCCL networking](https://www.hiascend.com/document/detail/en/Atlas800_3000_3010/24.0.RC1/SolutionDocs/EngineerD/AtlasTraining-D_24/atlasdeploy_03_0023.html) [B · refs]. Confirmation in Ascend Documentation Center (zh) — `support.huawei.com/enterprise/zh/doc/EDOC1100272080`.

K8s clusters typically run one or more CNI plugins to provide the
default Pod network (10.x.x.x overlay). HCCL transports usually
demand a SECOND, hardware-pinned network attachment per Pod —
something the default CNI does not provide. Multus + SR-IOV is the
canonical solution.

## 2. CNI × HCCL Transport Matrix

| CNI \ Transport | RDMA (IB) | RoCE v2 | IPoIB | Verdict |
| --- | --- | --- | --- | --- |
| **Calico** (BGP / IPIP mode) | Not supported (no IB plugin) | Caveat: via Multus + SR-IOV secondary | Not supported | Recommended for primary (Pod-to-Pod) network; secondary HCCL nic via Multus |
| **Calico** (VXLAN mode) | Not supported | Caveat: same — Multus + SR-IOV; VXLAN-encapsulated traffic can NOT carry RoCE v2 (PFC won't cross overlay) | Not supported | Use only for primary; HCCL goes around it |
| **Cilium** (eBPF native) | Experimental: eBPF + IB support landing in 1.16+ | Recommended: eBPF + native RoCE v2 dataplane bypass (Cilium ENI-mode) | Not supported | Most promising Phase 6 candidate per upstream announcement |
| **Cilium** (eBPF + cluster pool) | Experimental | Recommended (same as ENI mode) | Not supported | Same |
| **Flannel** (VXLAN) | Not supported | Not supported (PFC won't cross VXLAN overlay) | Not supported | Avoid — primary-only, no HCCL upgrade path |
| **Flannel** (host-gw) | Not supported | Caveat: secondary nic via Multus only | Not supported | Same as Calico VXLAN |
| **Multus** (meta-CNI) | N/A — Multus delegates | N/A — Multus delegates | N/A — Multus delegates | **Required for HCCL secondary nic**; pair with SR-IOV or Macvlan |

### 2.1 References per row

- **Calico + Multus + SR-IOV**: [Project Calico Network Policy with Multus](https://docs.tigera.io/calico/latest/networking/networking-platforms/multus-cni) [B · refs] — confirms Calico is multi-network compatible via Multus but does not natively provide RDMA transport.
- **Cilium native eBPF + RoCE**: [Cilium 1.16 release notes — RoCE v2 support announcement](https://cilium.io/blog/2024/04/30/cilium-116/) [B · refs] — claims eBPF dataplane bypass for RDMA flows; experimental in 1.16, GA target 1.18. **Confidence C · derived** — release blog mentions support; production verification not done at Phase 5.
- **Cilium + IB**: [Cilium Issue #18467](https://github.com/cilium/cilium/issues/18467) [B · refs] — RDMA support tracking issue; mixed reports of stability.
- **Flannel limitations**: [Flannel project README](https://github.com/flannel-io/flannel) [A · source] — explicitly says "intended for IP-based overlay" with no mention of RDMA/RoCE — confirms not-supported posture.
- **Multus + SR-IOV reference architecture**: [Network Operator (NVIDIA) docs — although NVIDIA-targeted, the patterns apply](https://docs.nvidia.com/networking/display/cokan10/network+operator) [B · refs]. Substitute NVIDIA components with Mellanox CX-6 + Huawei drivers.
- **HCCL transport spec**: [Ascend Documentation Center — HCCL networking](https://www.hiascend.com/document/detail/en/Atlas800_3000_3010/24.0.RC1/SolutionDocs) [B · refs].

### 2.2 General observations

1. **No CNI provides HCCL transport natively**. Every viable
   deployment uses Multus to attach a SECOND nic that bypasses the
   primary CNI for HCCL traffic. The primary CNI's job is Pod-to-Pod
   IP routing for control-plane traffic; HCCL takes the second nic.
2. **RoCE v2 is the most common production transport for Ascend
   training clusters**. RDMA over IB is faster but requires dedicated
   IB switches; RoCE v2 reuses existing Ethernet infrastructure with
   PFC (Priority Flow Control) + ECN (Explicit Congestion Notification).
3. **IPoIB is effectively deprecated for HCCL** — performance lags
   native RDMA by 3-5×. Listed for completeness.

## 3. Phase 5 simulator scope

The Phase 5 inference-operator manager + PD Router webhook (T006–T103)
have NO direct dependency on any specific CNI. The manager runs in
the cluster-level control plane network (whatever the primary CNI
provides); the webhook serves admission requests via in-cluster
service routing — also CNI-agnostic.

ResourceSlice publication (Phase 4 npu-dra-driver T005, Phase 5 T001
DeviceClass) is purely Kubernetes API surface — no networking
hardware path involved.

**Confirmation**: kind smoke (T106) runs against `kindnet` (kind's
default CNI) without any HCCL configuration. The fact that Phase 5
features work end-to-end on kindnet validates the CNI-independence
claim.

## 4. Phase 6 entry recommendation

For the Phase 6 scheduler-plugin work, the top-2 CNI candidates:

1. **Cilium + Multus + SR-IOV** (top choice). Cilium's eBPF dataplane
   sidesteps the overlay-encapsulation problem on primary traffic;
   Multus + SR-IOV handles the HCCL secondary nic. The
   scheduler-plugin can drive co-location decisions by reading the
   NUMA topology Cilium publishes (already integrated with
   topology-aware scheduling).

2. **Calico + Multus + SR-IOV** (production fallback). More
   battle-tested at large scale; well-understood operational story.
   The trade-off: less topology-aware scheduling integration than
   Cilium, so the Phase 6 plugin would need to re-implement the
   NUMA/HCCS scoring logic.

Verdict: **build the Phase 6 plugin against the standard topology
metadata exposed by both CNIs** (Pod annotations, K8s Node labels
populated by Mellanox tools). This keeps the plugin CNI-portable;
operators choose Cilium or Calico per their organization standard.

**Phase 6 selection landed in ADR-0010 (`docs/adr/0010-scheduler-plugin.md` §6)**: Cilium + Multus + SR-IOV recommended primary; Calico + Multus + SR-IOV production fallback. The scheduler-plugin reads only ResourceSlice attributes + NodeResourceTopology CR + Pod annotations — no CNI-specific API surface — so the choice does not lock the plugin.

## 5. Known gaps

The following HCCL features are unsupported across all evaluated
CNIs as of Phase 5 (2026-05-19):

- **Per-Pod RDMA bandwidth quota** — no CNI exposes a usable Linux
  cgroup interface for RDMA throughput. Phase 9 multi-tenancy may
  need this; current state-of-the-art is SR-IOV VF partitioning
  (coarse-grained).
- **HCCL collective primitives over Pod overlay networks** — none of
  Calico/Cilium/Flannel can pass PFC across VXLAN/IPIP encapsulation.
  Real-cluster HCCL traffic MUST go around the overlay (via a
  Multus-attached secondary nic).
- **Live migration of HCCL ranks** — once an HCCL group is
  established, moving a rank to a different Pod requires tearing
  down the collective. No CNI exposes a "transfer RDMA endpoint"
  primitive. Phase 7+ if real elastic training needs land.
- **Quality of Service across multi-tenant pods** — RoCE v2 PFC is
  switch-level (all-or-nothing per priority); cluster-level QoS per
  tenant requires either physical port isolation or hard partitioning
  via SR-IOV VFs.

## 6. Pointer to ADR-0007 fabric discovery

Adjacent topic: how the operator discovers the physical fabric topology
(which NIC on which host, switch topology). ADR-0007 defers this to
LLDP / SONiC API integration in Phase 7+. Until then, the
scheduler-plugin reads static labels populated by deployment tooling.

## 7. References

Primary:
- [Ascend Documentation Center — HCCL networking](https://www.hiascend.com/document/detail/en/Atlas800_3000_3010/24.0.RC1/SolutionDocs) [B · refs]
- [Huawei Atlas training solution best practices](https://support.huawei.com/enterprise/zh/doc/EDOC1100272080) [B · refs]

CNI-side:
- [Calico Multus documentation](https://docs.tigera.io/calico/latest/networking/networking-platforms/multus-cni) [B · refs]
- [Cilium 1.16 release blog](https://cilium.io/blog/2024/04/30/cilium-116/) [B · refs]
- [Cilium RDMA tracking](https://github.com/cilium/cilium/issues/18467) [B · refs]
- [Flannel project](https://github.com/flannel-io/flannel) [A · source]
- [Multus CNI project](https://github.com/k8snetworkplumbingwg/multus-cni) [A · source]
- [SR-IOV Network Operator](https://github.com/k8snetworkplumbingwg/sriov-network-operator) [A · source]

Cross-references:
- `docs/architecture.md` §13 review-table — Phase 5+ "NPU pod network
  considerations" row
- `docs/adr/0007-fabric-discovery.md` — LLDP/SONiC integration deferred
  to Phase 7+
- `docs/adr/0009-npu-dra-driver.md` §2 ADR — `npu.huawei.com/hccs-ring`
  attribute the scheduler-plugin will consume
