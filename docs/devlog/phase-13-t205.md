# P13-T-205 · [A4] Karmada control-plane HA

- **Commit**: this commit (main agent)
- **Date**: 2026-06-03
- **Duration**: plan 2d vs actual ~0.5d (deploy YAML/scripts; real multi-region LB + external etcd DR are lab-gated).

## Intent

Close build-doc §5 item "Karmada 多站点 HA" per ADR-0025 §2 Decision D: light up the
single-replica Phase 11 control-plane (P11-T-102 · ADR-0018 §4 (a) deferred HA to "Phase 12+")
into a 3-replica HA control-plane + HA etcd + propagation hardening (failover / propagateDeps /
cross-cluster RBAC). Right-sized reference HA (ADR-0025 §3) — NOT full multi-region DR.

## What landed

- **`values.yaml` → HA**: every control-plane component (apiServer, controllerManager, scheduler,
  webhook, aggregatedApiServer) `replicaCount: 1 → 3`; internal etcd `replicaCount: 1 → 3` (a 3-node
  quorum StatefulSet that tolerates 1 failure). install.sh already `-f values.yaml`, so the bootstrap
  flow is unchanged — the HA is data-driven.
- **External-etcd swap documented** (not configured): a commented `etcd.mode: external` block with
  endpoints/TLS shows the production path. Configuring it by default would BREAK the kind verify (no
  reachable external etcd), so the kind-verifiable HA default is internal 3-node etcd; external etcd is
  the real multi-region DR backend (lab).
- **`install.sh` step 3.5**: post-install verify prints the control-plane Deployment replica counts +
  the etcd StatefulSet, with an explicit note that single-node kind co-locates the 3 replicas (replica
  COUNT HA, not node-spread). Header updated to reference ADR-0025 §2 Decision D.
- **Propagation hardening** (`policies/`):
  - `propagation-modelservice.yaml`: `propagateDeps: true` (carry referenced ConfigMaps/Secrets to
    members) + application `failover` (tolerationSeconds 120 · purgeMode Graciously — reschedule
    divided replicas off an unhealthy member, draining the old placement after the new is up).
  - `propagation-rbac.yaml` (new): `ClusterPropagationPolicy` propagating only RBAC objects opted-in
    with label `ocloud.edge.example.com/propagate=true` (allow-list · never blanket) — closes the
    **P13-T-202 carry** (T202 deliberately left cross-cluster RBAC propagation to T205 to avoid
    intruding on this policy's ownership).

## Key decisions / right-sizing (ADR-0025 §3)

- **Replica COUNT HA on kind, node-spread HA on real clusters.** The upstream karmada Deployments carry
  their own soft pod anti-affinity (chart default), so replicas spread where the cluster has >1 node;
  kind's single node co-locates them. I did NOT inject a hard anti-affinity / topologySpreadConstraints
  block — the exact per-component values key varies by chart version (P1: don't fabricate an unverified
  schema key), and hard anti-affinity would leave replicas Pending on single-node kind (breaking the
  verify). Documented the node-spread + LB + external-etcd as the real-cluster/lab layer.
- **Internal 3-node etcd over external etcd as the default** — gives real quorum HA that a kind install
  can actually bring up + verify; external etcd is the documented production swap for cross-AZ DR.

## Verification (offline · plan §8 layer 1)

- `python yaml.safe_load_all` on values.yaml + all 4 policy files → parse clean.
- replica counts asserted = 3 for all 5 components + etcd internal (programmatic check, not eyeball).
- `bash -n install.sh` + `bash -n uninstall.sh` → clean.
- `propagation-modelservice.yaml` parsed → `propagateDeps: true` + `failover.application` present.

**Real-machine (LAB-GATED · T301)**: `kubectl -n karmada-system get deploy` → 3 ready replicas on a
multi-node cluster + a member-cluster failover drill (cordon a member → watch reschedule) is the T301
integration stamp. Real multi-region L4 LB fronting the apiserver + external etcd are real-network/lab
(ADR-0018 §2). The offline layer proves the values + policies are schema-shaped + HA-configured.

## Carry-forward

- **T204 cross-cluster usage** now has its substrate: with HA Karmada + the aggregated-apiserver, the
  ClusterQuota controller's PerCluster buckets populate from real member usage (no T204 code change —
  it already groups by the Karmada cached-from-cluster annotation).
- **T301**: real-cluster replica readiness + failover drill + (optional) external-etcd bring-up.
