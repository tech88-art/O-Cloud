package k8s

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// mkNodeWithRoles is mkNode + role labels + addresses + taints, used
// by the node-detail tests below. Built on top of cluster_test.go's
// mkNode helper rather than duplicating its capacity setup.
func mkNodeWithExtras(name, role string, ready bool, ips []string) *corev1.Node {
	n := mkNode(name, 8, ready)
	if role != "" {
		n.Labels["node-role.kubernetes.io/"+role] = ""
	}
	for _, ip := range ips {
		n.Status.Addresses = append(n.Status.Addresses, corev1.NodeAddress{
			Type:    corev1.NodeInternalIP,
			Address: ip,
		})
	}
	return n
}

func TestListNodes_NoFilter_ReturnsAll(t *testing.T) {
	client := fake.NewSimpleClientset(
		mkNodeWithExtras("worker-1", "worker", true, []string{"10.0.0.11"}),
		mkNodeWithExtras("worker-2", "worker", true, []string{"10.0.0.12"}),
		mkNodeWithExtras("control-1", "control-plane", true, nil),
	)
	src := NewSourceWithClient(client, Options{ClusterIDOverride: "demo"})

	nodes, err := src.ListNodes(context.Background(), model.NodeFilter{})
	require.NoError(t, err)
	require.Len(t, nodes, 3)

	names := map[string]bool{}
	for _, n := range nodes {
		names[n.Name] = true
		assert.Equal(t, "demo", n.ClusterID)
		assert.Equal(t, "Ready", n.Status)
		assert.Equal(t, 8, n.NPUCount)
	}
	assert.True(t, names["worker-1"])
	assert.True(t, names["worker-2"])
	assert.True(t, names["control-1"])
}

func TestListNodes_FilterByRole_MatchesCaseInsensitive(t *testing.T) {
	client := fake.NewSimpleClientset(
		mkNodeWithExtras("worker-1", "worker", true, nil),
		mkNodeWithExtras("control-1", "control-plane", true, nil),
	)
	src := NewSourceWithClient(client, Options{ClusterIDOverride: "demo"})

	nodes, err := src.ListNodes(context.Background(), model.NodeFilter{Role: "WORKER"})
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, "worker-1", nodes[0].Name)
}

func TestListNodes_FilterByClusterID_MismatchReturnsEmpty(t *testing.T) {
	client := fake.NewSimpleClientset(mkNodeWithExtras("worker-1", "worker", true, nil))
	src := NewSourceWithClient(client, Options{ClusterIDOverride: "demo"})

	nodes, err := src.ListNodes(context.Background(), model.NodeFilter{ClusterID: "other-cluster"})
	require.NoError(t, err)
	assert.Empty(t, nodes)
}

func TestListNodes_NotReadyStatus(t *testing.T) {
	client := fake.NewSimpleClientset(mkNodeWithExtras("flaky-1", "worker", false, nil))
	src := NewSourceWithClient(client, Options{ClusterIDOverride: "demo"})

	nodes, err := src.ListNodes(context.Background(), model.NodeFilter{})
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, "NotReady", nodes[0].Status)
}

func TestGetNodeDetail_HappyPath_WithInterfacesAndTaints(t *testing.T) {
	n := mkNodeWithExtras("worker-1", "worker", true, []string{"10.0.0.11", "10.0.0.12"})
	n.Spec.Taints = []corev1.Taint{
		{Key: "huawei.com/npu-broken", Effect: corev1.TaintEffectNoSchedule},
	}
	client := fake.NewSimpleClientset(n)
	src := NewSourceWithClient(client, Options{ClusterIDOverride: "demo"})

	detail, err := src.GetNodeDetail(context.Background(), "worker-1")
	require.NoError(t, err)
	assert.Equal(t, "worker-1", detail.Name)
	assert.Equal(t, "demo", detail.ClusterID)
	require.Len(t, detail.NetworkInterfaces, 1)
	assert.Equal(t, "eth0", detail.NetworkInterfaces[0].Name)
	assert.Equal(t, []string{"10.0.0.11", "10.0.0.12"}, detail.NetworkInterfaces[0].IPs)
	require.Len(t, detail.Taints, 1)
	assert.Equal(t, "huawei.com/npu-broken", detail.Taints[0]["key"])
}

func TestGetNodeDetail_NotFound_ReturnsErrResourceNotFound(t *testing.T) {
	client := fake.NewSimpleClientset()
	src := NewSourceWithClient(client, Options{ClusterIDOverride: "demo"})

	_, err := src.GetNodeDetail(context.Background(), "no-such-node")
	assert.ErrorIs(t, err, ErrResourceNotFound)
}

func TestGetNodeDetail_EmptyName_ReturnsErrResourceNotFound(t *testing.T) {
	client := fake.NewSimpleClientset()
	src := NewSourceWithClient(client, Options{ClusterIDOverride: "demo"})

	_, err := src.GetNodeDetail(context.Background(), "")
	assert.ErrorIs(t, err, ErrResourceNotFound)
}

func TestExtractRoles_DefaultsToWorker(t *testing.T) {
	n := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "x", Labels: map[string]string{}},
	}
	roles := extractRoles(n)
	assert.Equal(t, []string{"worker"}, roles)
}

func TestExtractRoles_PicksUpRoleLabels(t *testing.T) {
	n := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "x",
			Labels: map[string]string{
				"node-role.kubernetes.io/control-plane": "",
				"node-role.kubernetes.io/etcd":          "",
			},
		},
	}
	roles := extractRoles(n)
	assert.ElementsMatch(t, []string{"control-plane", "etcd"}, roles)
}
