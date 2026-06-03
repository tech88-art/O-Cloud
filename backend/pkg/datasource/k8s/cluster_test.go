package k8s

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func mkNode(name string, npuCount int64, ready bool) *corev1.Node {
	cond := corev1.ConditionTrue
	if !ready {
		cond = corev1.ConditionFalse
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{}},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("96"),
				corev1.ResourceMemory: resource.MustParse("768Gi"),
			},
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: cond},
			},
			NodeInfo: corev1.NodeSystemInfo{
				Architecture:    "amd64",
				KernelVersion:   "5.15.0-105-generic",
				OSImage:         "Ubuntu 22.04.4 LTS",
				KubeletVersion:  "v1.31.0",
			},
		},
	}
	if npuCount > 0 {
		node.Status.Capacity["huawei.com/Ascend910B"] = *resource.NewQuantity(npuCount, resource.DecimalSI)
	}
	return node
}

func TestListClusters_ReturnsSyntheticCluster(t *testing.T) {
	// Two nodes with 8 NPUs each + one no-NPU control-plane node.
	client := fake.NewSimpleClientset(
		mkNode("worker-1", 8, true),
		mkNode("worker-2", 8, true),
		mkNode("control-1", 0, true),
	)
	src := NewSourceWithClient(client, Options{ClusterIDOverride: "demo"})

	clusters, err := src.ListClusters(context.Background())
	require.NoError(t, err)
	require.Len(t, clusters, 1)
	c := clusters[0]
	assert.Equal(t, "demo", c.ID)
	assert.Equal(t, "demo", c.Name) // falls back to id when override is empty
	assert.Equal(t, "edge-single", c.Role)
	assert.Equal(t, "healthy", c.Status)
	assert.Equal(t, 3, c.NodeCount)
	assert.Equal(t, 16, c.NPUCount, "8+8 NPUs across the two worker nodes")
}

func TestGetCluster_MismatchedIDReturns404(t *testing.T) {
	client := fake.NewSimpleClientset(mkNode("worker-1", 8, true))
	src := NewSourceWithClient(client, Options{ClusterIDOverride: "demo"})

	_, err := src.GetCluster(context.Background(), "not-the-right-id")
	assert.ErrorIs(t, err, ErrResourceNotFound)
}

func TestGetCluster_EmptyIDReturnsResolved(t *testing.T) {
	client := fake.NewSimpleClientset(mkNode("worker-1", 8, true))
	src := NewSourceWithClient(client, Options{ClusterIDOverride: "demo"})

	c, err := src.GetCluster(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, "demo", c.ID)
}

func TestGetCluster_FallsBackToKubernetesLiteral_WhenNoOverrideOrCM(t *testing.T) {
	// No override, no kube-system/cluster-info ConfigMap.
	client := fake.NewSimpleClientset(mkNode("worker-1", 0, true))
	src := NewSourceWithClient(client, Options{})

	clusters, err := src.ListClusters(context.Background())
	require.NoError(t, err)
	require.Len(t, clusters, 1)
	assert.Equal(t, "kubernetes", clusters[0].ID)
	assert.Equal(t, "kubernetes", clusters[0].Name)
}

func TestGetCluster_PicksUpClusterInfoConfigMap(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "kube-system",
			Name:      "cluster-info",
		},
		Data: map[string]string{
			"cluster-id":   "ocloud-edge-a",
			"cluster-name": "Edge A Cluster",
		},
	}
	client := fake.NewSimpleClientset(cm, mkNode("worker-1", 8, true))
	src := NewSourceWithClient(client, Options{})

	c, err := src.GetCluster(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, "ocloud-edge-a", c.ID)
	assert.Equal(t, "Edge A Cluster", c.Name)
}

func TestCountAscendNPUs_AbsentCapacity(t *testing.T) {
	n := mkNode("plain", 0, true)
	assert.Equal(t, 0, countAscendNPUs(n))
}

func TestCountAscendNPUs_WithCapacity(t *testing.T) {
	n := mkNode("npu-host", 8, true)
	assert.Equal(t, 8, countAscendNPUs(n))
}
