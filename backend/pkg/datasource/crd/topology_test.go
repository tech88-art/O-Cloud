package crd

// crd.Source real-topology tests (P13-T-103). Drive GetTopology off a
// dynamic/fake client seeded with NPUPool objects carrying
// status.hccsTopology.peerGroups (the pool-operator P6-T-003 aggregation),
// asserting the synthesized cluster→node→npu HCCS graph.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/example/ocloud-edge/backend/pkg/datasource"
)

// mkNPUPoolWithHCCS builds an unstructured NPUPool whose status carries an HCCS
// topology with the given peer groups. peerGroups is groupId → device-id list.
func mkNPUPoolWithHCCS(name string, peerGroups map[string][]string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{Group: poolGroup, Version: poolVersion, Kind: "NPUPool"})
	u.SetName(name)
	u.Object["spec"] = map[string]any{
		"nodePoolRef":   map[string]any{"name": "nodepool-a"},
		"npuModel":      "Ascend910B",
		"sliceStrategy": "fixed-a",
	}
	groups := make([]any, 0, len(peerGroups))
	// Deterministic group order so the test fixture is stable.
	for _, gid := range sortedGroupKeys(peerGroups) {
		ids := peerGroups[gid]
		anyIDs := make([]any, 0, len(ids))
		for _, id := range ids {
			anyIDs = append(anyIDs, id)
		}
		groups = append(groups, map[string]any{
			"groupId":   gid,
			"deviceIds": anyIDs,
		})
	}
	u.Object["status"] = map[string]any{
		"hccsTopology": map[string]any{
			"fabricId":   "hccs-rack-1",
			"peerGroups": groups,
		},
	}
	return u
}

func sortedGroupKeys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// tiny insertion sort to avoid an import
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// newCRDSourceWithPools wires a crd.Source over a dynamic fake seeded with the
// given NPUPool objects (reuses pools_test.go's fakeDynamicClient helper which
// already registers the NPUPool list kind). *unstructured.Unstructured
// satisfies runtime.Object, so we pass them straight through.
func newCRDSourceWithPools(objs ...*unstructured.Unstructured) *Source {
	runtimeObjs := make([]runtime.Object, 0, len(objs))
	for _, o := range objs {
		runtimeObjs = append(runtimeObjs, o)
	}
	return NewSourceWithClient(fakeDynamicClient(runtimeObjs...), Options{})
}

func TestGetTopology_Real_NoLongerErrCapabilityUnavailable(t *testing.T) {
	pool := mkNPUPoolWithHCCS("npupool-a", map[string][]string{
		"ring-0": {"atlas-800-01-npu-0", "atlas-800-01-npu-1"},
	})
	src := newCRDSourceWithPools(pool)

	topo, err := src.GetTopology(context.Background(), "", "npu")
	require.NoError(t, err)
	require.NotNil(t, topo)
	assert.NotErrorIs(t, err, datasource.ErrCapabilityUnavailable)
	// cluster + 1 node + 2 npu = 4 nodes.
	assert.Len(t, topo.Nodes, 4)
}

func TestGetTopology_Real_HCCSPeerGroupRingsUp(t *testing.T) {
	// One node, one ring of 4 → closed 4-edge ring when IncludeFabric=true.
	pool := mkNPUPoolWithHCCS("npupool-a", map[string][]string{
		"ring-0": {
			"atlas-800-01-npu-0", "atlas-800-01-npu-1",
			"atlas-800-01-npu-2", "atlas-800-01-npu-3",
		},
	})
	src := newCRDSourceWithPools(pool)

	topo, err := src.GetTopologyWithFabric(context.Background(), "", "npu",
		datasource.TopologyOptions{IncludeFabric: true})
	require.NoError(t, err)

	hccs := 0
	edgeSet := map[[3]string]struct{}{}
	var sampleBW float64
	for _, e := range topo.Edges {
		edgeSet[[3]string{e.Source, e.Target, e.Type}] = struct{}{}
		if e.Type == "hccs" {
			hccs++
			if bw, ok := e.Attributes["bandwidthGBps"].(float64); ok {
				sampleBW = bw
			}
		}
	}
	assert.Equal(t, 4, hccs, "ring of 4 NPUs → 4 hccs edges (closed ring)")
	assert.Contains(t, edgeSet, [3]string{"atlas-800-01-npu-0", "atlas-800-01-npu-1", "hccs"})
	assert.Contains(t, edgeSet, [3]string{"atlas-800-01-npu-3", "atlas-800-01-npu-0", "hccs"})
	assert.Equal(t, 56.0, sampleBW, "datasheet HCCS bandwidth nominal stamped on the edge")
}

func TestGetTopology_Real_MultiNodePeerGroups(t *testing.T) {
	// Two nodes, each a ring of 2, in separate peer groups. Device-id node
	// parsing must split them onto two node nodes; rings stay within node.
	pool := mkNPUPoolWithHCCS("npupool-a", map[string][]string{
		"ring-0": {"node-a-npu-0", "node-a-npu-1"},
		"ring-1": {"node-b-npu-0", "node-b-npu-1"},
	})
	src := newCRDSourceWithPools(pool)

	topo, err := src.GetTopologyWithFabric(context.Background(), "", "npu",
		datasource.TopologyOptions{IncludeFabric: true})
	require.NoError(t, err)

	// cluster + 2 nodes + 4 npu = 7 nodes.
	nodeTypes := map[string]int{}
	for _, n := range topo.Nodes {
		nodeTypes[n.Type]++
	}
	assert.Equal(t, 1, nodeTypes["cluster"])
	assert.Equal(t, 2, nodeTypes["node"])
	assert.Equal(t, 4, nodeTypes["npu"])

	edgeSet := map[[3]string]struct{}{}
	for _, e := range topo.Edges {
		edgeSet[[3]string{e.Source, e.Target, e.Type}] = struct{}{}
	}
	// Each node's 2-NPU ring → 1 hccs edge; no cross-node edge.
	assert.Contains(t, edgeSet, [3]string{"node-a-npu-0", "node-a-npu-1", "hccs"})
	assert.Contains(t, edgeSet, [3]string{"node-b-npu-0", "node-b-npu-1", "hccs"})
	assert.NotContains(t, edgeSet, [3]string{"node-a-npu-0", "node-b-npu-0", "hccs"})
	// node→npu contains edges land under the right node.
	assert.Contains(t, edgeSet, [3]string{"node-a", "node-a-npu-0", "contains"})
	assert.Contains(t, edgeSet, [3]string{"node-b", "node-b-npu-0", "contains"})
}

func TestGetTopology_Real_NoPoolsEmptyGraph(t *testing.T) {
	// No NPUPools → cluster-only graph (no nodes/npus), not an error.
	src := newCRDSourceWithPools()

	topo, err := src.GetTopology(context.Background(), "", "npu")
	require.NoError(t, err)
	require.NotNil(t, topo)
	// Just the synthetic cluster node.
	assert.Len(t, topo.Nodes, 1)
	assert.Equal(t, "cluster", topo.Nodes[0].Type)
}

func TestGetTopology_Real_DepthNodeDropsNPUs(t *testing.T) {
	pool := mkNPUPoolWithHCCS("npupool-a", map[string][]string{
		"ring-0": {"node-a-npu-0", "node-a-npu-1"},
	})
	src := newCRDSourceWithPools(pool)

	topo, err := src.GetTopology(context.Background(), "", "node")
	require.NoError(t, err)
	for _, n := range topo.Nodes {
		assert.NotEqual(t, "npu", n.Type, "depth=node must not emit npu nodes")
	}
	// cluster + 1 node = 2 nodes.
	assert.Len(t, topo.Nodes, 2)
}

func TestGetTopology_Real_UnknownClusterID_404(t *testing.T) {
	pool := mkNPUPoolWithHCCS("npupool-a", map[string][]string{"ring-0": {"node-a-npu-0"}})
	src := newCRDSourceWithPools(pool)

	_, err := src.GetTopology(context.Background(), "some-other-cluster", "npu")
	assert.ErrorIs(t, err, ErrResourceNotFound)
}

func TestGetTopology_Real_MalformedDeviceIDsSkipped(t *testing.T) {
	// Device ids without the "-npu-" separator are skipped defensively; valid
	// ones still surface.
	pool := mkNPUPoolWithHCCS("npupool-a", map[string][]string{
		"ring-0": {"atlas-800-01-npu-0", "garbage-id", ""},
	})
	src := newCRDSourceWithPools(pool)

	topo, err := src.GetTopology(context.Background(), "", "npu")
	require.NoError(t, err)
	npuCount := 0
	for _, n := range topo.Nodes {
		if n.Type == "npu" {
			npuCount++
		}
	}
	assert.Equal(t, 1, npuCount, "only the well-formed device id becomes an npu")
}

func TestGetTopology_Real_DeviceIDWithHyphenatedNode(t *testing.T) {
	// Node hostname itself contains hyphens — parse on the LAST "-npu-".
	pool := mkNPUPoolWithHCCS("npupool-a", map[string][]string{
		"ring-0": {"atlas-800-01-npu-3"},
	})
	src := newCRDSourceWithPools(pool)

	topo, err := src.GetTopology(context.Background(), "", "npu")
	require.NoError(t, err)
	edgeSet := map[[3]string]struct{}{}
	for _, e := range topo.Edges {
		edgeSet[[3]string{e.Source, e.Target, e.Type}] = struct{}{}
	}
	// The node node is "atlas-800-01" (not "atlas-800") and the npu hangs off it.
	assert.Contains(t, edgeSet, [3]string{"atlas-800-01", "atlas-800-01-npu-3", "contains"})
}

func TestGetTopology_Real_ZeroRegressionGetTopologyEqualsZeroOpts(t *testing.T) {
	pool := mkNPUPoolWithHCCS("npupool-a", map[string][]string{
		"ring-0": {"node-a-npu-0", "node-a-npu-1"},
	})
	src := newCRDSourceWithPools(pool)

	plain, err := src.GetTopology(context.Background(), "", "npu")
	require.NoError(t, err)
	zero, err := src.GetTopologyWithFabric(context.Background(), "", "npu", datasource.TopologyOptions{})
	require.NoError(t, err)

	assert.Equal(t, len(plain.Nodes), len(zero.Nodes))
	assert.Equal(t, len(plain.Edges), len(zero.Edges))
	for _, e := range plain.Edges {
		assert.NotEqual(t, "hccs", e.Type, "no hccs without IncludeFabric")
	}
}
