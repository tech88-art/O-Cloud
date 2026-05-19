#!/usr/bin/env bash
# Phase 3 P3-T-104 — seed the pool hierarchy + a smoke workload.
#
# Order matters: CRDs must be Established before we apply CRs, and the
# pool-operator's ValidatingAdmissionPolicy (operators/pool-operator/
# config/admission/) forces NPUSlicePool into the ocloud-system
# namespace, so we apply the leaf there.
#
# Hierarchy reproduced (architecture.md §6):
#   ClusterPool (skipped — Phase 3 stub controller per P3-T-105 is
#               separately tested; not load-bearing for this smoke)
#   NodePool      -> selects 2 worker nodes by labels {site, role}
#   NPUPool       -> ref NodePool, npuModel=Ascend910B,
#                    sliceStrategy=smoke-pool
#   NPUSlicePool  -> ref NPUPool, strategy=FixedTemplate, vir04 shape
#
# The workflow's `wait for NPUSlicePool reconcile` step polls
# .status.totalSlices > 0 against the smoke-pool to prove the operator
# Reconcile loops (T002-T004) actually fired against real-cluster API
# server traffic, not just envtest.
set -euo pipefail

NS="${NS:-ocloud-system}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "== wait for CRDs to be Established =="
for crd in \
    nodepools.ims.ocloud.edge.example.com \
    npupools.ims.ocloud.edge.example.com \
    npuslicepools.ims.ocloud.edge.example.com \
    clusterpools.ims.ocloud.edge.example.com; do
  kubectl wait --for=condition=Established "crd/${crd}" --timeout=60s
done

echo "== apply NodePool (Cluster-scoped) =="
cat <<EOF | kubectl apply -f -
apiVersion: ims.ocloud.edge.example.com/v1alpha1
kind: NodePool
metadata:
  name: smoke-nodepool
spec:
  selector:
    matchLabels:
      site: site-a
      role: edge
  role: edge
  location: site-a-shanghai
EOF

echo "== apply NPUPool (Cluster-scoped) =="
cat <<EOF | kubectl apply -f -
apiVersion: ims.ocloud.edge.example.com/v1alpha1
kind: NPUPool
metadata:
  name: smoke-npupool
spec:
  nodePoolRef:
    name: smoke-nodepool
  npuModel: Ascend910B
  selector:
    matchLabels:
      site: site-a
  sliceStrategy: smoke-pool
EOF

echo "== apply NPUSlicePool (Namespaced — VAP forces ocloud-system) =="
cat <<EOF | kubectl apply -f -
apiVersion: ims.ocloud.edge.example.com/v1alpha1
kind: NPUSlicePool
metadata:
  name: smoke-pool
  namespace: ${NS}
spec:
  npuPoolRef:
    name: smoke-npupool
  strategy: FixedTemplate
  fixedTemplates:
    - name: vir04
      aiCoreCount: 4
      memoryMiB: 16384
EOF

echo "== apply mock workload Pod =="
kubectl apply -f "${SCRIPT_DIR}/manifests/mock-workload.yaml"

echo "== seed complete =="
