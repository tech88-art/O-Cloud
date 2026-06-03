package k8s

// k8s.Source real-topology tests (P13-T-103). Drive GetTopology /
// GetTopologyWithFabric off a kubernetes/fake clientset (Nodes + Ascend
// capacity) plus a dynamic/fake client seeded with Ascend ResourceSlices so the
// HCCS-ring overlay path is exercised without a real cluster.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
)

// mkResourceSlice builds an unstructured ResourceSlice for one node carrying the
// given devices. Each device entry is (index, hccsRing, numaNode). Attributes
// nest under .basic.attributes (v1beta1 shape) with the int value under `int`.
func mkResourceSlice(node, driver string, devices [][3]int) *unstructured.Unstructured {
	devs := make([]interface{}, 0, len(devices))
	for _, d := range devices {
		idx, ring, numa := d[0], d[1], d[2]
		devs = append(devs, map[string]interface{}{
			"name": npuID(node, idx),
			"basic": map[string]interface{}{
				"attributes": map[string]interface{}{
					attrNPUIndex: map[string]interface{}{"int": int64(idx)},
					attrHCCSRing: map[string]interface{}{"int": int64(ring)},
					attrNUMANode: map[string]interface{}{"int": int64(numa)},
				},
			},
		})
	}
	u := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "resource.k8s.io/v1beta1",
		"kind":       "ResourceSlice",
		"metadata":   map[string]interface{}{"name": node + "-ascend"},
		"spec": map[string]interface{}{
			"driver":   driver,
			"nodeName": node,
			"devices":  devs,
		},
	}}
	u.SetGroupVersionKind(schema.GroupVersionKind{Group: "resource.k8s.io", Version: "v1beta1", Kind: "ResourceSlice"})
	return u
}

// fakeDyn builds a dynamic fake client that knows the ResourceSlice list kind
// for EVERY candidate version loadResourceSliceOverlay probes. The fake panics
// on a List against an unregistered GVR (a fake-only quirk — a real apiserver
// returns a discovery error the overlay loop tolerates), so we register all
// four candidate versions' list kinds. Objects seeded under v1beta1 still only
// surface on the v1beta1 List; the other versions list empty, which the loop
// falls through.
func fakeDyn(objs ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{}
	for _, gvr := range resourceSliceGVRs {
		listKinds[gvr] = "ResourceSliceList"
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objs...)
}

// mkAscendNode mirrors cluster_test.go's helper but local to keep the fixtures
// self-contained: a Ready node advertising `huawei.com/Ascend910B` capacity.
func mkAscendNode(name string, npuCount int64) *corev1.Node {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{}},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("192"),
				corev1.ResourceMemory: resource.MustParse("1024Gi"),
			},
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			NodeInfo:   corev1.NodeSystemInfo{Architecture: "arm64", OSImage: "openEuler 22.03"},
		},
	}
	if npuCount > 0 {
		node.Status.Capacity["huawei.com/Ascend910B"] = *resource.NewQuantity(npuCount, resource.DecimalSI)
	}
	return node
}

func TestGetTopology_Real_NoLongerErrCapabilityUnavailable(t *testing.T) {
	// The headline of P13-T-103: GetTopology must no longer be a stub.
	client := fake.NewSimpleClientset(mkAscendNode("atlas-800-01", 8))
	src := NewSourceWithClients(client, fakeDyn(), client0Opts())

	topo, err := src.GetTopology(context.Background(), "", "npu")
	require.NoError(t, err)
	require.NotNil(t, topo)
	assert.NotErrorIs(t, err, datasource.ErrCapabilityUnavailable)

	// cluster + node + 8 npu = 10 nodes (no fabric flag → structural only).
	assert.Len(t, topo.Nodes, 10)
}

func TestGetTopology_Real_HCCSRingsFromResourceSliceAttributes(t *testing.T) {
	// 8-card node; ResourceSlice publishes ring 0 for npu 0..3, ring 1 for
	// npu 4..7 (real 4-per-ring 910B layout). With IncludeFabric=true the
	// aggregator must ring up each group from the OVERLAID hccsGroup.
	node := "atlas-800-01"
	client := fake.NewSimpleClientset(mkAscendNode(node, 8))
	rs := mkResourceSlice(node, draDriverName, [][3]int{
		{0, 0, 0}, {1, 0, 0}, {2, 0, 0}, {3, 0, 0},
		{4, 1, 1}, {5, 1, 1}, {6, 1, 1}, {7, 1, 1},
	})
	src := NewSourceWithClients(client, fakeDyn(rs), client0Opts())

	topo, err := src.GetTopologyWithFabric(context.Background(), "", "npu",
		datasource.TopologyOptions{IncludeFabric: true})
	require.NoError(t, err)

	hccs := 0
	var sample string
	edgeSet := map[[3]string]struct{}{}
	for _, e := range topo.Edges {
		edgeSet[[3]string{e.Source, e.Target, e.Type}] = struct{}{}
		if e.Type == "hccs" {
			hccs++
			if bw, ok := e.Attributes["bandwidthGBps"].(float64); ok {
				sample = formatBW(bw)
			}
		}
	}
	// Two closed rings of 4 → 8 hccs edges.
	assert.Equal(t, 8, hccs, "two real 4-card rings → 8 hccs edges")
	// Ring 0 closes within npu 0..3; ring 1 within 4..7; no cross-ring edge.
	assert.Contains(t, edgeSet, [3]string{node + "-npu-0", node + "-npu-1", "hccs"})
	assert.Contains(t, edgeSet, [3]string{node + "-npu-3", node + "-npu-0", "hccs"})
	assert.NotContains(t, edgeSet, [3]string{node + "-npu-3", node + "-npu-4", "hccs"})
	assert.Equal(t, "56", sample, "datasheet HCCS bandwidth nominal stamped")
}

func TestGetTopology_Real_FallsBackToStaticLayoutWithoutSlices(t *testing.T) {
	// No ResourceSlice (dynamic client empty) → the structural npu.go layout
	// still groups the 8 NPUs into two static rings ("<node>-hccs-0/1"), so
	// IncludeFabric=true still produces HCCS rings (proves the graph renders
	// even before the publisher has run — best-effort overlay).
	node := "atlas-800-01"
	client := fake.NewSimpleClientset(mkAscendNode(node, 8))
	src := NewSourceWithClients(client, fakeDyn(), client0Opts())

	topo, err := src.GetTopologyWithFabric(context.Background(), "", "npu",
		datasource.TopologyOptions{IncludeFabric: true})
	require.NoError(t, err)

	hccs := 0
	for _, e := range topo.Edges {
		if e.Type == "hccs" {
			hccs++
		}
	}
	// Static layout: npu 0..3 → hccs-0, npu 4..7 → hccs-1 (npusPerNUMA=4) → two
	// rings of 4 → 8 edges. Same count as the overlaid case here because the
	// static layout happens to match; the point is the graph is non-empty.
	assert.Equal(t, 8, hccs, "static 4-per-NUMA layout still rings up without ResourceSlices")
}

func TestGetTopology_Real_IgnoresForeignDriverSlices(t *testing.T) {
	// A ResourceSlice from a DIFFERENT driver must not pollute the overlay —
	// only npu.ocloud.edge.example.com slices are read. With a foreign-driver
	// slice carrying bogus rings, the overlay stays empty and the static layout
	// (two rings of 4) is used, NOT the foreign attributes.
	node := "atlas-800-01"
	client := fake.NewSimpleClientset(mkAscendNode(node, 8))
	foreign := mkResourceSlice(node, "gpu.nvidia.com", [][3]int{
		{0, 9, 9}, {1, 9, 9}, // bogus ring 9 — must be ignored
	})
	src := NewSourceWithClients(client, fakeDyn(foreign), client0Opts())

	topo, err := src.GetTopologyWithFabric(context.Background(), "", "npu",
		datasource.TopologyOptions{IncludeFabric: true})
	require.NoError(t, err)

	// No npu should land in a "hccs-9" group (foreign slice ignored). The
	// static layout's hccs-0/hccs-1 rings are present instead.
	byID := map[string]struct{}{}
	for _, n := range topo.Nodes {
		if n.Type == "npu" {
			if g, ok := n.Attributes["hccsGroup"].(string); ok {
				byID[g] = struct{}{}
			}
		}
	}
	assert.NotContains(t, byID, node+"-hccs-9", "foreign-driver ring must not overlay")
	assert.Contains(t, byID, node+"-hccs-0")
}

func TestGetTopology_Real_UnknownClusterID_404(t *testing.T) {
	client := fake.NewSimpleClientset(mkAscendNode("atlas-800-01", 8))
	src := NewSourceWithClients(client, fakeDyn(), Options{ClusterIDOverride: "demo"})

	_, err := src.GetTopology(context.Background(), "not-demo", "npu")
	assert.ErrorIs(t, err, ErrResourceNotFound)
}

func TestGetTopology_Real_DepthNode_DropsNPUs(t *testing.T) {
	client := fake.NewSimpleClientset(mkAscendNode("atlas-800-01", 8))
	src := NewSourceWithClients(client, fakeDyn(), client0Opts())

	topo, err := src.GetTopology(context.Background(), "", "node")
	require.NoError(t, err)
	for _, n := range topo.Nodes {
		assert.NotEqual(t, "npu", n.Type, "depth=node must not emit npu nodes")
	}
	// cluster + 1 node = 2 nodes.
	assert.Len(t, topo.Nodes, 2)
}

func TestGetTopology_Real_ZeroRegressionGetTopologyEqualsZeroOpts(t *testing.T) {
	// GetTopology(ctx,id,depth) must equal GetTopologyWithFabric(...,opts{}).
	node := "atlas-800-01"
	client := fake.NewSimpleClientset(mkAscendNode(node, 8))
	rs := mkResourceSlice(node, draDriverName, [][3]int{{0, 0, 0}, {1, 0, 0}})
	src := NewSourceWithClients(client, fakeDyn(rs), client0Opts())

	plain, err := src.GetTopology(context.Background(), "", "npu")
	require.NoError(t, err)
	zero, err := src.GetTopologyWithFabric(context.Background(), "", "npu", datasource.TopologyOptions{})
	require.NoError(t, err)

	assert.Equal(t, len(plain.Nodes), len(zero.Nodes))
	assert.Equal(t, len(plain.Edges), len(zero.Edges))
	// Neither emits hccs (IncludeFabric defaults false) — structural only.
	for _, e := range plain.Edges {
		assert.NotEqual(t, "hccs", e.Type)
	}
}

// client0Opts is the zero Options (uses the cluster-info / fallback id chain).
func client0Opts() Options { return Options{} }

// formatBW renders a bandwidth float without trailing zeros for assertion
// readability (56.0 → "56").
func formatBW(f float64) string {
	if f == float64(int64(f)) {
		return intToASCIIDigit(int(f))
	}
	return "" // non-integer not expected for the nominal
}
