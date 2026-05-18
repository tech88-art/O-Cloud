package k8s

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// ListNodes returns every Node in the apiserver, projected onto the
// model.Node DTO. NodeFilter is applied client-side (the apiserver's
// field selectors don't cover all the Phase-1 filter dimensions
// uniformly; cluster size is the demo's ≤ 100 nodes, so filtering in
// memory is fine).
//
// Filter semantics:
//
//   - ClusterID — honored; non-empty mismatch yields an empty slice.
//     The Source serves only one cluster (see ListClusters), so the
//     check is "id matches the resolved id".
//   - PoolName — not implemented yet; the NodePool CRD lookup lands in
//     P2-T-101 and the filter will then resolve pool → node members.
//     Until then a non-empty PoolName matches everything (most
//     permissive default; the alternative — match nothing — would
//     surface as an empty-page bug on the Nodes UI).
//   - Role — matches against node.metadata.labels["node-role.kubernetes.io/<role>"]
//     plus the "edge" / "worker" / "control-plane" buckets used in
//     mock.Source. Case-insensitive comparison.
func (s *Source) ListNodes(ctx context.Context, filter model.NodeFilter) ([]*model.Node, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if filter.ClusterID != "" {
		resolvedID, _ := s.resolveClusterIdentity(ctx)
		if filter.ClusterID != resolvedID {
			return []*model.Node{}, nil
		}
	}

	list, err := s.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, errFromAPIServer("list nodes", err)
	}

	clusterID, _ := s.resolveClusterIdentity(ctx)
	out := make([]*model.Node, 0, len(list.Items))
	for i := range list.Items {
		n := &list.Items[i]
		if !matchesRole(n, filter.Role) {
			continue
		}
		out = append(out, projectNodeListView(n, clusterID))
	}
	return out, nil
}

// GetNodeDetail returns the per-node detail aggregate (allOf of
// model.Node + NUMA + interfaces + storage + taints).
//
// NotFound surfaces as ErrResourceNotFound for the handler's 404 path.
func (s *Source) GetNodeDetail(ctx context.Context, name string) (*model.NodeDetail, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if name == "" {
		return nil, ErrResourceNotFound
	}
	n, err := s.client.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrResourceNotFound
		}
		return nil, errFromAPIServer("get node", err)
	}
	clusterID, _ := s.resolveClusterIdentity(ctx)
	base := projectNodeListView(n, clusterID)
	return &model.NodeDetail{
		Node:              *base,
		NetworkInterfaces: extractInterfaces(n),
		Taints:            extractTaints(n),
	}, nil
}

// projectNodeListView extracts the list-view fields shared by ListNodes
// and the embedded `Node` of NodeDetail. Pure (no API calls).
func projectNodeListView(n *corev1.Node, clusterID string) *model.Node {
	role := extractRoles(n)
	status := extractStatus(n)
	out := &model.Node{
		Name:       n.Name,
		ClusterID:  clusterID,
		Role:       role,
		Status:     status,
		Arch:       n.Status.NodeInfo.Architecture,
		KernelVersion: n.Status.NodeInfo.KernelVersion,
		OS:         n.Status.NodeInfo.OSImage,
		KubeletVer: n.Status.NodeInfo.KubeletVersion,
		NPUCount:   countAscendNPUs(n),
		Labels:     n.Labels,
	}

	// Capacity quantities — keep the raw string from the apiserver, no
	// unit conversion. The frontend's <Quantity> component renders
	// human-friendly text directly off `.raw`.
	if q, ok := n.Status.Capacity[corev1.ResourceCPU]; ok {
		out.CPU = &model.Quantity{Raw: q.String()}
	}
	if q, ok := n.Status.Capacity[corev1.ResourceMemory]; ok {
		out.Memory = &model.Quantity{Raw: q.String()}
	}
	return out
}

// extractRoles reads role labels. K8s convention is
// `node-role.kubernetes.io/<role>=""`. We collect every such label and,
// for the edge / worker / control-plane trio, also map the legacy
// labels (`kubernetes.io/role=...`).
func extractRoles(n *corev1.Node) []string {
	const prefix = "node-role.kubernetes.io/"
	seen := map[string]struct{}{}
	out := []string{}
	for k := range n.Labels {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			role := k[len(prefix):]
			if _, ok := seen[role]; ok {
				continue
			}
			seen[role] = struct{}{}
			out = append(out, role)
		}
	}
	if legacy, ok := n.Labels["kubernetes.io/role"]; ok {
		if _, dup := seen[legacy]; !dup {
			out = append(out, legacy)
		}
	}
	if len(out) == 0 {
		// Most Phase-2 demos run workers only; default to "worker" so
		// the UI's role filter still shows the node.
		out = []string{"worker"}
	}
	return out
}

// extractStatus reads the NodeReady condition and surfaces it via the
// Phase-1 Status vocabulary (Ready / NotReady / Unknown).
func extractStatus(n *corev1.Node) string {
	for _, c := range n.Status.Conditions {
		if c.Type != corev1.NodeReady {
			continue
		}
		switch c.Status {
		case corev1.ConditionTrue:
			return "Ready"
		case corev1.ConditionFalse:
			return "NotReady"
		default:
			return "Unknown"
		}
	}
	return "Unknown"
}

// extractInterfaces flattens the apiserver's NodeAddress list onto the
// model.NetworkInterface shape. K8s doesn't expose interface-level
// metadata (MAC, speed) through node status — those come from cAdvisor
// /metrics or NPU plugin annotations and are best-effort here. For
// Phase 2 W1 we surface ip addresses only; the missing fields land
// later if a downstream consumer needs them.
func extractInterfaces(n *corev1.Node) []model.NetworkInterface {
	if len(n.Status.Addresses) == 0 {
		return nil
	}
	// One synthetic "eth0" interface per node carrying all the
	// InternalIP / ExternalIP addresses. K8s nodes typically only have
	// one user-relevant address each so this matches what operators
	// expect to see in the UI.
	ips := make([]string, 0, len(n.Status.Addresses))
	for _, a := range n.Status.Addresses {
		if a.Type == corev1.NodeInternalIP || a.Type == corev1.NodeExternalIP {
			ips = append(ips, a.Address)
		}
	}
	if len(ips) == 0 {
		return nil
	}
	return []model.NetworkInterface{{Name: "eth0", IPs: ips}}
}

// extractTaints projects taints onto the schema's loose object shape.
// Taint = map[string]any in the contract; we surface key / value /
// effect literally.
func extractTaints(n *corev1.Node) []model.Taint {
	if len(n.Spec.Taints) == 0 {
		return nil
	}
	out := make([]model.Taint, 0, len(n.Spec.Taints))
	for _, t := range n.Spec.Taints {
		out = append(out, model.Taint{
			"key":    t.Key,
			"value":  t.Value,
			"effect": string(t.Effect),
		})
	}
	return out
}

// matchesRole returns true if the filter is empty or matches one of
// the node's roles (case-insensitive on the role string).
func matchesRole(n *corev1.Node, role string) bool {
	if role == "" {
		return true
	}
	for _, r := range extractRoles(n) {
		if equalFold(r, role) {
			return true
		}
	}
	return false
}

// equalFold is a tiny ASCII case-insensitive comparison. We don't
// import strings just for ToLower because the role strings are always
// ASCII and the package's other code stays lean.
func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
