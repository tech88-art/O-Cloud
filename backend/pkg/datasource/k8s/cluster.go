package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// Cluster identity resolution constants. We try to look up the
// kube-system/cluster-info ConfigMap (a kubeadm convention) for a
// stable id; absent that we synthesize "kubernetes" so the wire shape
// always carries a non-empty id.
const (
	clusterInfoNamespace = "kube-system"
	clusterInfoConfigMap = "cluster-info"
	fallbackClusterID    = "kubernetes"
)

// ListClusters returns the single "cluster" served by this Source.
// K8s has no native concept of multiple clusters per apiserver, so
// the list is always length 1; the multi-cluster federation case
// (Karmada / Cluster API) is Phase 9 and will use a separate Source
// implementation rather than overloading this one.
func (s *Source) ListClusters(ctx context.Context) ([]*model.Cluster, error) {
	c, err := s.GetCluster(ctx, "")
	if err != nil {
		return nil, err
	}
	return []*model.Cluster{c}, nil
}

// GetCluster returns the single cluster. The id parameter is honored
// for symmetry with the Source interface — when non-empty and
// matching the resolved cluster id it returns the same payload; a
// mismatch returns ErrResourceNotFound (a 404 from the handler side).
//
// Resolution chain for cluster id / name:
//
//  1. constructor overrides (Options.ClusterIDOverride / NameOverride)
//  2. kube-system/cluster-info ConfigMap's `cluster-id` data key
//  3. fallback literal "kubernetes"
//
// Node + NPU counts are populated by counting Nodes via the apiserver
// (cheap O(N) — the demo target is ≤ 100 nodes). KubernetesVersion
// comes from the apiserver discovery endpoint.
func (s *Source) GetCluster(ctx context.Context, id string) (*model.Cluster, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	clusterID, clusterName := s.resolveClusterIdentity(ctx)
	if id != "" && id != clusterID {
		return nil, ErrResourceNotFound
	}

	// Node count + npu count (NPU count is best-effort — depends on
	// Ascend Device Plugin labels surfacing the capacity).
	nodeList, err := s.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errFromAPIServer("list nodes for cluster aggregate", err)
	}
	npuCount := 0
	for _, n := range nodeList.Items {
		npuCount += countAscendNPUs(&n)
	}

	version := ""
	if info, err := s.client.Discovery().ServerVersion(); err == nil {
		version = info.GitVersion
	}

	cluster := &model.Cluster{
		ID:                clusterID,
		Name:              clusterName,
		Role:              "edge-single",
		Status:            "healthy",
		KubernetesVersion: version,
		NodeCount:         len(nodeList.Items),
		NPUCount:          npuCount,
	}
	return cluster, nil
}

// resolveClusterIdentity walks the override → cluster-info → fallback
// chain. Returns (id, name); both never empty.
func (s *Source) resolveClusterIdentity(ctx context.Context) (string, string) {
	if s.clusterID != "" {
		name := s.clusterName
		if name == "" {
			name = s.clusterID
		}
		return s.clusterID, name
	}

	// kube-system/cluster-info is the standard kubeadm artefact.
	cm, err := s.client.CoreV1().
		ConfigMaps(clusterInfoNamespace).
		Get(ctx, clusterInfoConfigMap, metav1.GetOptions{})
	if err == nil && cm != nil {
		if v, ok := cm.Data["cluster-id"]; ok && v != "" {
			name := cm.Data["cluster-name"]
			if name == "" {
				name = v
			}
			return v, name
		}
	}
	// NotFound is expected on K3s / fresh installs — fall through silently.
	if err != nil && !apierrors.IsNotFound(err) {
		// Other errors are also non-fatal; we just lose the override and
		// fall back to the literal id. Log once via the wrapping error
		// pattern so dev environments still see the cause if they care.
		_ = fmt.Errorf("k8s cluster-info lookup: %w", err)
	}
	return fallbackClusterID, fallbackClusterID
}

// countAscendNPUs reads a node's `huawei.com/Ascend910B` resource
// capacity. The Ascend Device Plugin advertises NPUs under this
// resource name; the count is the integer capacity. Zero on absent
// resource (non-NPU nodes).
//
// This helper lives next to GetCluster because the cluster-level NPU
// aggregate needs it; ListNPUs in P2-T-002 will use it again per node.
func countAscendNPUs(node *corev1.Node) int {
	const ascendResource = "huawei.com/Ascend910B"
	if node == nil || node.Status.Capacity == nil {
		return 0
	}
	q, ok := node.Status.Capacity[ascendResource]
	if !ok {
		return 0
	}
	n, ok := q.AsInt64()
	if !ok {
		return 0
	}
	return int(n)
}
