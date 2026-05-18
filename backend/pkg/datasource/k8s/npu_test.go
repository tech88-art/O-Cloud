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

// mkNPUNode builds a node with N Ascend NPUs of the named model.
// model = "Ascend910B" puts capacity under "huawei.com/Ascend910B".
func mkNPUNode(name, model string, count int64) *corev1.Node {
	n := mkNode(name, 0, true) // start from cluster_test's base node, no NPUs
	n.Status.Capacity[corev1.ResourceName("huawei.com/"+model)] = *resource.NewQuantity(count, resource.DecimalSI)
	return n
}

func TestListNPUs_HappyPath_910B_DefaultsAndStaticTopology(t *testing.T) {
	client := fake.NewSimpleClientset(mkNPUNode("worker-1", "Ascend910B", 8))
	src := NewSourceWithClient(client, Options{ClusterIDOverride: "demo"})

	npus, err := src.ListNPUs(context.Background(), "worker-1")
	require.NoError(t, err)
	require.Len(t, npus, 8, "8 NPUs requested via huawei.com/Ascend910B=8")

	// First NPU on NUMA 0 / hccs-0; per-model defaults applied.
	first := npus[0]
	assert.Equal(t, "worker-1-npu-0", first.ID)
	assert.Equal(t, "worker-1", first.NodeName)
	assert.Equal(t, "Ascend910B", first.Model)
	assert.Equal(t, 0, first.Index)
	assert.Equal(t, 65536, first.VRAMMiB)
	assert.Equal(t, 32, first.AICoreTotal)
	assert.Equal(t, 0, first.NumaNode)
	assert.Equal(t, "worker-1-hccs-0", first.HCCSGroup)
	assert.Equal(t, "healthy", first.Status)
	assert.Equal(t, "whole", first.SliceMode)

	// NPU 4 crosses the static-layout NUMA boundary.
	fifth := npus[4]
	assert.Equal(t, 1, fifth.NumaNode)
	assert.Equal(t, "worker-1-hccs-1", fifth.HCCSGroup)
}

func TestListNPUs_NoNPUResource_ReturnsEmpty(t *testing.T) {
	client := fake.NewSimpleClientset(mkNode("plain-1", 0, true))
	src := NewSourceWithClient(client, Options{ClusterIDOverride: "demo"})

	npus, err := src.ListNPUs(context.Background(), "plain-1")
	require.NoError(t, err)
	assert.Empty(t, npus)
}

func TestListNPUs_NotFound(t *testing.T) {
	client := fake.NewSimpleClientset()
	src := NewSourceWithClient(client, Options{ClusterIDOverride: "demo"})

	_, err := src.ListNPUs(context.Background(), "no-such-node")
	assert.ErrorIs(t, err, ErrResourceNotFound)
}

func TestListNPUs_EmptyNodeName(t *testing.T) {
	client := fake.NewSimpleClientset()
	src := NewSourceWithClient(client, Options{})

	_, err := src.ListNPUs(context.Background(), "")
	assert.ErrorIs(t, err, ErrResourceNotFound)
}

func TestListNPUs_Ascend310_UsesPerModelDefaults(t *testing.T) {
	client := fake.NewSimpleClientset(mkNPUNode("infer-1", "Ascend310", 4))
	src := NewSourceWithClient(client, Options{})

	npus, err := src.ListNPUs(context.Background(), "infer-1")
	require.NoError(t, err)
	require.Len(t, npus, 4)
	assert.Equal(t, "Ascend310", npus[0].Model)
	assert.Equal(t, 8192, npus[0].VRAMMiB)
	assert.Equal(t, 4, npus[0].AICoreTotal)
}

func TestListNPUs_SkipsVirtualCardResources(t *testing.T) {
	// `Ascend910B-2c` etc. are fixed-template slices, NOT counted as
	// whole-card NPUs by ListNPUs (P2-T-102 takes those via NPUSlicePool).
	n := mkNode("worker-1", 0, true)
	n.Status.Capacity["huawei.com/Ascend910B"] = *resource.NewQuantity(8, resource.DecimalSI)
	n.Status.Capacity["huawei.com/Ascend910B-2c"] = *resource.NewQuantity(16, resource.DecimalSI)
	n.Status.Capacity["huawei.com/Ascend910B-4c"] = *resource.NewQuantity(8, resource.DecimalSI)
	client := fake.NewSimpleClientset(n)
	src := NewSourceWithClient(client, Options{})

	npus, err := src.ListNPUs(context.Background(), "worker-1")
	require.NoError(t, err)
	assert.Len(t, npus, 8, "only the whole-card resource counts; -2c / -4c are slice variants")
}

func TestListNPUs_ModelLabelOverride(t *testing.T) {
	// Node advertises Ascend910 but operator forces "Ascend910B" via
	// the optional model label.
	n := mkNPUNode("worker-1", "Ascend910", 8)
	n.Labels[nodeLabelModel] = "Ascend910B"
	client := fake.NewSimpleClientset(n)
	src := NewSourceWithClient(client, Options{})

	npus, err := src.ListNPUs(context.Background(), "worker-1")
	require.NoError(t, err)
	require.Len(t, npus, 8)
	assert.Equal(t, "Ascend910B", npus[0].Model)
}

func TestListNPUs_LayoutAnnotationOverride(t *testing.T) {
	n := mkNPUNode("worker-1", "Ascend910B", 8)
	// Override NPU 0 + 1 only; the rest fall back to the static layout.
	n.Annotations = map[string]string{
		nodeAnnotationLayout: "0:numa=1,hccs=custom-group-a;1:numa=1,hccs=custom-group-a",
	}
	client := fake.NewSimpleClientset(n)
	src := NewSourceWithClient(client, Options{})

	npus, err := src.ListNPUs(context.Background(), "worker-1")
	require.NoError(t, err)
	require.Len(t, npus, 8)

	assert.Equal(t, 1, npus[0].NumaNode, "override flipped npu-0 to numa-1")
	assert.Equal(t, "custom-group-a", npus[0].HCCSGroup)
	assert.Equal(t, 1, npus[1].NumaNode)
	assert.Equal(t, "custom-group-a", npus[1].HCCSGroup)

	// NPU 2 falls back to static: numa 0 (since 2/4 = 0), hccs "worker-1-hccs-0".
	assert.Equal(t, 0, npus[2].NumaNode)
	assert.Equal(t, "worker-1-hccs-0", npus[2].HCCSGroup)
}

func TestReadAscendCapacity_TieBreakerDeterministic(t *testing.T) {
	// Mixed model: both Ascend910 and Ascend910B with equal counts.
	// Tie-break is lexicographic on model suffix.
	n := mkNode("worker-1", 0, true)
	n.Status.Capacity["huawei.com/Ascend910"] = *resource.NewQuantity(4, resource.DecimalSI)
	n.Status.Capacity["huawei.com/Ascend910B"] = *resource.NewQuantity(4, resource.DecimalSI)

	count, modelName := readAscendCapacity(n)
	assert.Equal(t, 8, count, "total counts across both keys")
	assert.Equal(t, "Ascend910", modelName, "lex tie-break picks 'Ascend910' < 'Ascend910B'")
}

func TestReadLayoutAnnotation_Malformed(t *testing.T) {
	// Missing colon, missing key, malformed numa value — all skipped
	// silently; map empty.
	cases := []string{
		"",
		"no-colon",
		"0:foo=1",                       // missing numa+hccs
		"0:numa=abc,hccs=g",             // malformed numa
		"0:numa=0",                      // missing hccs
		"0:numa=0,hccs=g;;;",            // trailing empty chunks ignored
	}
	for _, c := range cases {
		got := readLayoutAnnotation(c)
		switch c {
		case "0:numa=0,hccs=g;;;":
			require.Len(t, got, 1)
			assert.Equal(t, 0, got[0].numa)
			assert.Equal(t, "g", got[0].hccsGroup)
		default:
			assert.Empty(t, got, "input %q should yield empty override map", c)
		}
	}
}

func TestNPUID_Format(t *testing.T) {
	assert.Equal(t, "worker-1-npu-0", npuID("worker-1", 0))
	assert.Equal(t, "worker-1-npu-7", npuID("worker-1", 7))
	assert.Equal(t, "worker-1-npu-12", npuID("worker-1", 12))
}

func TestAtoiASCII(t *testing.T) {
	good := []string{"0", "7", "42", "100"}
	for _, s := range good {
		_, err := atoiASCII(s)
		assert.NoError(t, err, "input %q should parse", s)
	}
	bad := []string{"", "-1", "1.5", "abc", "0x10"}
	for _, s := range bad {
		_, err := atoiASCII(s)
		assert.Error(t, err, "input %q should fail", s)
	}
}

// Sanity: ensure the var-only fake-client setup compiles + the
// metav1 import isn't dropped.
func TestMetav1Sanity(t *testing.T) {
	_ = metav1.ObjectMeta{}
}
