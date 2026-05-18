// Package aggregator — table-driven tests for BuildTopology.
//
// Each test case fixes a depth + input bundle and asserts the resulting
// nodes/edges. We assert shape (counts + ids + types), not field-by-field
// attribute payloads — the latter would tightly couple the test to formatting
// minutiae that ought to be free to drift. Attribute presence is sampled.
package aggregator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// ---- fixtures ----------------------------------------------------------
//
// fixtureInputs returns a small but realistic bundle: 1 cluster, 2 nodes,
// 3 NPUs (2 on node-1, 1 on node-2), 2 slices on npu-0. Mirrors the shape
// of configs/mock-data/set-a-small but trimmed for assertion clarity.

func fixtureInputs() TopologyInputs {
	cl := &model.Cluster{
		ID:                "cluster-a",
		Name:              "cluster-a",
		Role:              "edge-single",
		Location:          "site-a",
		Status:            "healthy",
		KubernetesVersion: "v1.31.0",
		NodeCount:         2,
		NPUCount:          3,
	}
	mkNode := func(name string) model.NodeDetail {
		return model.NodeDetail{Node: model.Node{
			Name:      name,
			ClusterID: "cluster-a",
			Role:      []string{"worker"},
			Status:    "Ready",
			Arch:      "amd64",
			OS:        "Ubuntu 22.04",
			NPUCount:  2,
		}}
	}
	nodes := []model.NodeDetail{mkNode("node-1"), mkNode("node-2")}
	npus := []*model.NPU{
		{ID: "node-1-npu-0", NodeName: "node-1", Model: "Ascend910B", Index: 0, VRAMMiB: 65536, AICoreTotal: 32, HCCSGroup: "hccs-0", Status: "healthy", SliceMode: "fixed-template"},
		{ID: "node-1-npu-1", NodeName: "node-1", Model: "Ascend910B", Index: 1, VRAMMiB: 65536, AICoreTotal: 32, HCCSGroup: "hccs-0", Status: "healthy", SliceMode: "whole"},
		{ID: "node-2-npu-0", NodeName: "node-2", Model: "Ascend910B", Index: 0, VRAMMiB: 65536, AICoreTotal: 32, HCCSGroup: "hccs-0", Status: "degraded", SliceMode: "whole"},
	}
	slices := []model.NPUSlice{
		{ID: "node-1-npu-0-slice-0", ParentNPU: "node-1-npu-0", Template: "vir02", AICore: 8, VRAMMiB: 16384, Status: "allocated",
			AllocatedTo: &model.SliceAllocation{Namespace: "ai-inference", PodName: "pod-0", ContainerName: "main"}},
		{ID: "node-1-npu-0-slice-1", ParentNPU: "node-1-npu-0", Template: "vir02", AICore: 8, VRAMMiB: 16384, Status: "available"},
	}
	return TopologyInputs{
		Cluster:     cl,
		Nodes:       nodes,
		NPUs:        npus,
		Slices:      slices,
		Depth:       DepthSlice,
		GeneratedAt: time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC),
	}
}

// indexNodes builds an id → TopologyNode lookup so assertions don't depend
// on slice order. (BuildTopology emits in a documented order, but the test
// keeps that decoupled to ease future reordering.)
func indexNodes(t *testing.T, topo *model.Topology) map[string]model.TopologyNode {
	t.Helper()
	out := make(map[string]model.TopologyNode, len(topo.Nodes))
	for _, n := range topo.Nodes {
		_, dup := out[n.ID]
		require.Falsef(t, dup, "duplicate topology node id %q", n.ID)
		out[n.ID] = n
	}
	return out
}

// edgeKey returns a stable tuple key for an edge.
func edgeKey(e model.TopologyEdge) [3]string { return [3]string{e.Source, e.Target, e.Type} }

// indexEdges builds a set keyed by (source, target, type) so assertions are
// order-insensitive.
func indexEdges(topo *model.Topology) map[[3]string]struct{} {
	out := make(map[[3]string]struct{}, len(topo.Edges))
	for _, e := range topo.Edges {
		out[edgeKey(e)] = struct{}{}
	}
	return out
}

// ---- tests --------------------------------------------------------------

func TestBuildTopology_DepthSlice_FullTree(t *testing.T) {
	in := fixtureInputs()
	in.Depth = DepthSlice

	topo := BuildTopology(in)
	require.NotNil(t, topo)

	// 1 cluster + 2 node + 3 npu + 2 slice = 8 nodes
	assert.Len(t, topo.Nodes, 8)
	// 2 cluster→node + 3 node→npu + 2 npu→slice = 7 edges
	assert.Len(t, topo.Edges, 7)

	byID := indexNodes(t, topo)
	require.Contains(t, byID, "cluster-a")
	assert.Equal(t, "cluster", byID["cluster-a"].Type)
	assert.Equal(t, "healthy", byID["cluster-a"].Status)

	require.Contains(t, byID, "node-1")
	require.Contains(t, byID, "node-2")
	assert.Equal(t, "node", byID["node-1"].Type)
	assert.Equal(t, "Ready", byID["node-1"].Status)

	require.Contains(t, byID, "node-1-npu-0")
	assert.Equal(t, "npu", byID["node-1-npu-0"].Type)
	assert.Equal(t, "Ascend910B#0", byID["node-1-npu-0"].Label)
	assert.Equal(t, "fixed-template", byID["node-1-npu-0"].Attributes["sliceMode"])

	require.Contains(t, byID, "node-1-npu-0-slice-0")
	assert.Equal(t, "slice", byID["node-1-npu-0-slice-0"].Type)
	assert.Equal(t, "allocated", byID["node-1-npu-0-slice-0"].Status)
	// allocatedTo round-trips into the attributes payload.
	at, ok := byID["node-1-npu-0-slice-0"].Attributes["allocatedTo"].(map[string]interface{})
	require.True(t, ok, "allocatedTo should be map")
	assert.Equal(t, "ai-inference", at["namespace"])

	// Edges connect cluster → node → npu → slice.
	edges := indexEdges(topo)
	assert.Contains(t, edges, [3]string{"cluster-a", "node-1", "contains"})
	assert.Contains(t, edges, [3]string{"cluster-a", "node-2", "contains"})
	assert.Contains(t, edges, [3]string{"node-1", "node-1-npu-0", "contains"})
	assert.Contains(t, edges, [3]string{"node-1", "node-1-npu-1", "contains"})
	assert.Contains(t, edges, [3]string{"node-2", "node-2-npu-0", "contains"})
	assert.Contains(t, edges, [3]string{"node-1-npu-0", "node-1-npu-0-slice-0", "contains"})
	assert.Contains(t, edges, [3]string{"node-1-npu-0", "node-1-npu-0-slice-1", "contains"})

	require.NotNil(t, topo.Meta)
	assert.Equal(t, "cluster-a", topo.Meta.ClusterID)
	require.NotNil(t, topo.Meta.GeneratedAt)
	assert.Equal(t, in.GeneratedAt, *topo.Meta.GeneratedAt)
}

func TestBuildTopology_DepthNPU_DropsSlices(t *testing.T) {
	in := fixtureInputs()
	in.Depth = DepthNPU

	topo := BuildTopology(in)

	// 1 + 2 + 3 = 6 nodes; 2 + 3 = 5 edges.
	assert.Len(t, topo.Nodes, 6)
	assert.Len(t, topo.Edges, 5)

	for _, n := range topo.Nodes {
		assert.NotEqual(t, "slice", n.Type, "slice node leaked into depth=npu graph: %v", n.ID)
	}
}

func TestBuildTopology_DepthNode_OnlyClusterAndNodes(t *testing.T) {
	in := fixtureInputs()
	in.Depth = DepthNode

	topo := BuildTopology(in)

	// 1 cluster + 2 nodes = 3 nodes; 2 cluster→node edges.
	assert.Len(t, topo.Nodes, 3)
	assert.Len(t, topo.Edges, 2)

	for _, n := range topo.Nodes {
		assert.NotContains(t, []string{"npu", "slice"}, n.Type,
			"deeper type %q leaked into depth=node graph: %v", n.Type, n.ID)
	}
}

func TestBuildTopology_UnknownDepth_DefaultsToSlice(t *testing.T) {
	in := fixtureInputs()
	in.Depth = "this-is-not-a-real-depth"

	topo := BuildTopology(in)

	// Same totals as DepthSlice.
	assert.Len(t, topo.Nodes, 8)
	assert.Len(t, topo.Edges, 7)
}

func TestBuildTopology_EmptyDepth_DefaultsToSlice(t *testing.T) {
	in := fixtureInputs()
	in.Depth = ""

	topo := BuildTopology(in)
	assert.Len(t, topo.Nodes, 8)
	assert.Len(t, topo.Edges, 7)
}

func TestBuildTopology_EmptyCluster_NoNodes_ReturnsClusterOnly(t *testing.T) {
	in := TopologyInputs{
		Cluster: &model.Cluster{ID: "empty-cluster", Name: "empty-cluster", Status: "healthy"},
		Depth:   DepthSlice,
	}
	topo := BuildTopology(in)
	require.NotNil(t, topo)

	assert.Len(t, topo.Nodes, 1)
	assert.Len(t, topo.Edges, 0)
	assert.Equal(t, "empty-cluster", topo.Nodes[0].ID)
	assert.Equal(t, "cluster", topo.Nodes[0].Type)
}

func TestBuildTopology_NilCluster_ReturnsEmptyGraph(t *testing.T) {
	topo := BuildTopology(TopologyInputs{Cluster: nil, Depth: DepthSlice})
	require.NotNil(t, topo)
	assert.Empty(t, topo.Nodes)
	assert.Empty(t, topo.Edges)
	// meta still emitted so callers always see a generatedAt.
	require.NotNil(t, topo.Meta)
}

func TestBuildTopology_FiltersOrphanNPUsAndSlices(t *testing.T) {
	// node-1 exists; node-99 doesn't. npu pointing at node-99 must be dropped.
	// Likewise slice pointing at orphan npu must drop.
	in := TopologyInputs{
		Cluster: &model.Cluster{ID: "cluster-a", Name: "cluster-a", Status: "healthy"},
		Nodes: []model.NodeDetail{
			{Node: model.Node{Name: "node-1", ClusterID: "cluster-a", Status: "Ready"}},
		},
		NPUs: []*model.NPU{
			{ID: "node-1-npu-0", NodeName: "node-1", Model: "Ascend910B", Status: "healthy"},
			{ID: "orphan-npu", NodeName: "node-99", Model: "Ascend910B", Status: "healthy"},
		},
		Slices: []model.NPUSlice{
			{ID: "slice-good", ParentNPU: "node-1-npu-0", Status: "available"},
			{ID: "slice-orphan", ParentNPU: "orphan-npu", Status: "available"},
		},
		Depth: DepthSlice,
	}

	topo := BuildTopology(in)

	byID := indexNodes(t, topo)
	require.Contains(t, byID, "node-1-npu-0")
	require.Contains(t, byID, "slice-good")
	assert.NotContains(t, byID, "orphan-npu")
	assert.NotContains(t, byID, "slice-orphan")
}

func TestBuildTopology_FiltersNodesByClusterID(t *testing.T) {
	in := TopologyInputs{
		Cluster: &model.Cluster{ID: "cluster-a", Name: "cluster-a", Status: "healthy"},
		Nodes: []model.NodeDetail{
			{Node: model.Node{Name: "node-a", ClusterID: "cluster-a", Status: "Ready"}},
			{Node: model.Node{Name: "node-b", ClusterID: "cluster-b", Status: "Ready"}},
			// Empty cluster id is accepted (no enforcement) — mirrors set-a-small's
			// loose data discipline so we don't drop nodes that lack the field.
			{Node: model.Node{Name: "node-loose", ClusterID: "", Status: "Ready"}},
		},
		Depth: DepthNode,
	}
	topo := BuildTopology(in)

	byID := indexNodes(t, topo)
	require.Contains(t, byID, "node-a")
	require.Contains(t, byID, "node-loose")
	assert.NotContains(t, byID, "node-b")
}

func TestBuildTopology_GeneratedAtAutoStampedWhenZero(t *testing.T) {
	in := fixtureInputs()
	in.GeneratedAt = time.Time{} // zero

	topo := BuildTopology(in)

	require.NotNil(t, topo.Meta)
	require.NotNil(t, topo.Meta.GeneratedAt)
	// Should be a recent UTC timestamp — within a generous 1 minute window.
	delta := time.Since(*topo.Meta.GeneratedAt)
	assert.Less(t, delta, time.Minute, "auto-stamped generatedAt should be recent")
}

// ---- P1-T-211 fabric branch tests --------------------------------------
//
// Each fabric case re-uses fixtureInputs() as the base then layers in
// switches/links + IncludeFabric. We assert against the *delta* from the
// T102 base counts (8 nodes / 7 edges at depth=slice; 3 / 2 at depth=node)
// so a future T102 fixture tweak doesn't ripple into these tests.

// fabricFixture returns the canonical 1 ToR switch + 3 node↔switch link
// fixture mirroring configs/mock-data/set-a-small/networkSwitches.json +
// networkLinks.json. node-1 and node-2 are present in fixtureInputs(); the
// link to "node-3" tests that orphan endpoints get dropped.
func fabricFixture() ([]NetworkSwitch, []NetworkLink) {
	switches := []NetworkSwitch{
		{
			ID: "switch-tor-a-01", Name: "ToR-Site-A-01", Type: "tor",
			Location: "site-a", PortsTotal: 48, PortsUsed: 6,
			BandwidthGbps: 100, VLANs: []string{"vlan-100-mgmt"}, Status: "up",
		},
	}
	links := []NetworkLink{
		{ID: "link-1", From: "switch-tor-a-01", To: "node-1", BandwidthGbps: 100, Medium: "fiber", Utilization: 18.5, RTTUs: 5.4},
		{ID: "link-2", From: "switch-tor-a-01", To: "node-2", BandwidthGbps: 100, Medium: "fiber", Utilization: 32.1, RTTUs: 6.2},
	}
	return switches, links
}

func TestBuildTopology_Fabric_IncludeFabricFalse_NoSwitchesNoLinks(t *testing.T) {
	// Zero-regression contract (AC): IncludeFabric=false → output is byte-
	// equivalent to T102. Even when Switches/Links are populated, they MUST
	// be ignored.
	switches, links := fabricFixture()
	in := fixtureInputs()
	in.IncludeFabric = false
	in.Switches = switches
	in.Links = links

	topo := BuildTopology(in)
	// Same counts as TestBuildTopology_DepthSlice_FullTree.
	assert.Len(t, topo.Nodes, 8)
	assert.Len(t, topo.Edges, 7)

	byID := indexNodes(t, topo)
	assert.NotContains(t, byID, "switch-tor-a-01",
		"switch node leaked when IncludeFabric=false")
	for _, e := range topo.Edges {
		assert.NotEqual(t, "fabric-link", e.Type,
			"fabric-link edge leaked when IncludeFabric=false")
	}
}

func TestBuildTopology_Fabric_DepthNode_AddsSwitchAndLinks(t *testing.T) {
	// AC: ?depth=node&includeFabric=true → 1 cluster + 2 nodes + 1 switch
	// + 2 cluster→node edges + 2 fabric-link edges. (fixtureInputs() has
	// 2 nodes; the canonical set-a-small uses 3 — same shape, different N.)
	switches, links := fabricFixture()
	in := fixtureInputs()
	in.Depth = DepthNode
	in.IncludeFabric = true
	in.Switches = switches
	in.Links = links

	topo := BuildTopology(in)

	// 1 cluster + 2 node + 1 switch = 4 nodes.
	assert.Len(t, topo.Nodes, 4)
	// 2 cluster→node + 2 fabric-link = 4 edges.
	assert.Len(t, topo.Edges, 4)

	byID := indexNodes(t, topo)
	require.Contains(t, byID, "switch-tor-a-01")
	sw := byID["switch-tor-a-01"]
	assert.Equal(t, "switch", sw.Type)
	assert.Equal(t, "ToR-Site-A-01", sw.Label)
	assert.Equal(t, "up", sw.Status)
	assert.Equal(t, "tor", sw.Attributes["switchType"])
	assert.EqualValues(t, 100, sw.Attributes["bandwidthGbps"])

	edges := indexEdges(topo)
	assert.Contains(t, edges, [3]string{"switch-tor-a-01", "node-1", "fabric-link"})
	assert.Contains(t, edges, [3]string{"switch-tor-a-01", "node-2", "fabric-link"})
}

func TestBuildTopology_Fabric_DepthSlice_FabricCoexistsWithNPUTree(t *testing.T) {
	// Fabric is depth-agnostic — at depth=slice we should see the full
	// cluster→node→npu→slice tree AND the switch + fabric-link layer.
	switches, links := fabricFixture()
	in := fixtureInputs()
	in.Depth = DepthSlice
	in.IncludeFabric = true
	in.Switches = switches
	in.Links = links

	topo := BuildTopology(in)

	// T102 base (8 nodes / 7 edges) + 1 switch + 2 fabric-link = 9 / 9.
	assert.Len(t, topo.Nodes, 9)
	assert.Len(t, topo.Edges, 9)

	// Spot-check the tree wiring stayed intact.
	edges := indexEdges(topo)
	assert.Contains(t, edges, [3]string{"cluster-a", "node-1", "contains"})
	assert.Contains(t, edges, [3]string{"node-1-npu-0", "node-1-npu-0-slice-0", "contains"})
	// And the new fabric layer.
	assert.Contains(t, edges, [3]string{"switch-tor-a-01", "node-1", "fabric-link"})
}

func TestBuildTopology_Fabric_OrphanLinkEndpointsDropped(t *testing.T) {
	// Links whose endpoints aren't in the graph must be dropped silently —
	// keeps the wire payload self-consistent and matches how
	// FiltersOrphanNPUsAndSlices behaves for the slice/npu layers.
	in := fixtureInputs()
	in.Depth = DepthNode
	in.IncludeFabric = true
	in.Switches = []NetworkSwitch{
		{ID: "switch-tor-a-01", Name: "ToR-Site-A-01", Type: "tor", Status: "up"},
	}
	in.Links = []NetworkLink{
		{ID: "link-good", From: "switch-tor-a-01", To: "node-1", BandwidthGbps: 100},
		{ID: "link-orphan-node", From: "switch-tor-a-01", To: "node-99", BandwidthGbps: 100},
		{ID: "link-orphan-switch", From: "switch-nonexistent", To: "node-1", BandwidthGbps: 100},
		{ID: "link-no-from", From: "", To: "node-1", BandwidthGbps: 100},
	}

	topo := BuildTopology(in)

	// 1 cluster + 2 nodes + 1 switch = 4 nodes.
	assert.Len(t, topo.Nodes, 4)
	// Only link-good materializes alongside the 2 cluster→node edges = 3.
	assert.Len(t, topo.Edges, 3)

	edges := indexEdges(topo)
	assert.Contains(t, edges, [3]string{"switch-tor-a-01", "node-1", "fabric-link"})
	assert.NotContains(t, edges, [3]string{"switch-tor-a-01", "node-99", "fabric-link"})
	assert.NotContains(t, edges, [3]string{"switch-nonexistent", "node-1", "fabric-link"})
}

func TestBuildTopology_Fabric_EmptySwitchesNoLinks_NoOp(t *testing.T) {
	// IncludeFabric=true with zero switches AND zero links → graph stays at
	// the T102 baseline. Guards against accidental shape changes when a
	// future fixture set drops fabric files (they're optional per ADR-0004).
	in := fixtureInputs()
	in.IncludeFabric = true
	// Switches / Links left zero-valued.

	topo := BuildTopology(in)

	assert.Len(t, topo.Nodes, 8)
	assert.Len(t, topo.Edges, 7)
}
