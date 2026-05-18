// Package aggregator builds DTOs that span more than one datasource resource.
//
// P1-T-102 introduces the first member: BuildTopology, which assembles the
// cluster → node → npu → slice tree the frontend G6 renderer consumes. The
// function is intentionally pure (no datasource imports, no I/O) so it
// table-tests cheaply and so any Source — mock today, k8s + crd tomorrow —
// can feed it preloaded slices and reuse the same wire shape.
//
// Wire shape: see docs/api-contract.yaml components.schemas.{Topology,
// TopologyNode, TopologyEdge}. TopologyNode.Type ∈ {cluster, node, npu, slice}
// (we don't emit nodepool / network here; those join later phases).
// TopologyEdge.Type is always "contains" for the parent → child relations
// BuildTopology emits; later tasks may add "hccs" / "allocated" via separate
// helpers.
package aggregator

import (
	"strconv"
	"strings"
	"time"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// Topology depth values. Matches the `depth` query param enum in
// docs/api-contract.yaml /api/v1/clusters/{clusterId}/topology.
const (
	DepthNode  = "node"  // cluster + nodes only
	DepthNPU   = "npu"   // + npu layer
	DepthSlice = "slice" // + slice layer (default; deepest set-a-small supplies)
)

// normalizeDepth maps the wire-level depth string onto one of the three
// canonical levels. Unknown / empty defaults to DepthSlice (matches the
// OpenAPI default). Kept exported-but-unparen'd because handlers may want
// to validate strict-mode against the same set; expose if needed later.
func normalizeDepth(depth string) string {
	switch strings.ToLower(strings.TrimSpace(depth)) {
	case DepthNode:
		return DepthNode
	case DepthNPU:
		return DepthNPU
	case DepthSlice, "":
		return DepthSlice
	default:
		// Per task spec: unknown depth falls through to the slice default
		// rather than 400 — keeps the frontend's `depth=foo` typos
		// graceful, mirrors how the contract names `default: slice`.
		return DepthSlice
	}
}

// TopologyInputs is the bundle of preloaded fixtures BuildTopology consumes.
// Callers (mock.GetTopology and any future source) populate it from their own
// caches; the aggregator never reaches for disk.
//
// Cluster must be non-nil; Nodes / NPUs / Slices may be empty (an empty
// cluster — no nodes — still produces a single-node graph). NPUs / Slices
// are ignored when Depth is shallower than they belong to.
type TopologyInputs struct {
	Cluster *model.Cluster
	Nodes   []model.NodeDetail
	NPUs    []*model.NPU
	Slices  []model.NPUSlice
	Depth   string // node | npu | slice (empty → slice)

	// GeneratedAt seeds Topology.meta.generatedAt. Optional — when zero we
	// stamp time.Now().UTC() so the wire payload always carries one.
	GeneratedAt time.Time
}

// BuildTopology assembles a G6-compatible Topology DTO from preloaded sources.
//
// Pure: no I/O, no globals. The returned *model.Topology owns no references
// back into the input slices (TopologyNode values are constructed fresh,
// attributes maps allocated per-node), so callers may mutate inputs after
// the call without affecting the returned graph.
//
// Returns a non-nil Topology even when inputs are empty (zero-cluster graph
// is the empty-array / empty-array shape the contract demands). When
// in.Cluster is nil the result is `{nodes: [], edges: []}` — same shape
// the source's 404 path skips around but useful for tests.
func BuildTopology(in TopologyInputs) *model.Topology {
	depth := normalizeDepth(in.Depth)

	out := &model.Topology{
		Nodes: []model.TopologyNode{},
		Edges: []model.TopologyEdge{},
	}

	// meta.generatedAt is mandatory even when the graph is empty — the
	// frontend reads it for cache freshness. Use the caller's stamp when
	// provided so tests can pin a deterministic value.
	ts := in.GeneratedAt
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	out.Meta = &model.TopologyMeta{
		GeneratedAt: &ts,
	}

	if in.Cluster == nil {
		return out
	}
	out.Meta.ClusterID = in.Cluster.ID

	// 1) cluster node — always present.
	out.Nodes = append(out.Nodes, model.TopologyNode{
		ID:     in.Cluster.ID,
		Type:   "cluster",
		Label:  clusterLabel(in.Cluster),
		Status: in.Cluster.Status,
		Attributes: map[string]interface{}{
			"role":              in.Cluster.Role,
			"location":          in.Cluster.Location,
			"kubernetesVersion": in.Cluster.KubernetesVersion,
			"nodeCount":         in.Cluster.NodeCount,
			"npuCount":          in.Cluster.NPUCount,
		},
	})

	// 2) node nodes + edges. Filter to nodes belonging to this cluster — set-a-small
	// has only one cluster so every node matches, but multi-cluster fixtures
	// will rely on this guard.
	clusterID := in.Cluster.ID
	nodeIDs := make(map[string]struct{}, len(in.Nodes))
	for i := range in.Nodes {
		nd := in.Nodes[i].Node
		if nd.ClusterID != "" && nd.ClusterID != clusterID {
			continue
		}
		out.Nodes = append(out.Nodes, model.TopologyNode{
			ID:     nd.Name,
			Type:   "node",
			Label:  nd.Name,
			Status: nd.Status,
			Attributes: map[string]interface{}{
				"role":     nd.Role,
				"arch":     nd.Arch,
				"os":       nd.OS,
				"npuCount": nd.NPUCount,
			},
		})
		out.Edges = append(out.Edges, model.TopologyEdge{
			Source: clusterID,
			Target: nd.Name,
			Type:   "contains",
		})
		nodeIDs[nd.Name] = struct{}{}
	}

	if depth == DepthNode {
		return out
	}

	// 3) npu nodes + edges. Filter to NPUs whose nodeName appears in nodeIDs
	// (drops orphan NPUs that point at unknown nodes — a defensive guard
	// rather than a real-world fixture concern, but cheap).
	npuIDs := make(map[string]struct{}, len(in.NPUs))
	for _, npu := range in.NPUs {
		if npu == nil {
			continue
		}
		if _, ok := nodeIDs[npu.NodeName]; !ok {
			continue
		}
		out.Nodes = append(out.Nodes, model.TopologyNode{
			ID:     npu.ID,
			Type:   "npu",
			Label:  npuLabel(npu),
			Status: npu.Status,
			Attributes: map[string]interface{}{
				"model":       npu.Model,
				"index":       npu.Index,
				"vramMiB":     npu.VRAMMiB,
				"aiCoreTotal": npu.AICoreTotal,
				"hccsGroup":   npu.HCCSGroup,
				"sliceMode":   npu.SliceMode,
			},
		})
		out.Edges = append(out.Edges, model.TopologyEdge{
			Source: npu.NodeName,
			Target: npu.ID,
			Type:   "contains",
		})
		npuIDs[npu.ID] = struct{}{}
	}

	if depth == DepthNPU {
		return out
	}

	// 4) slice nodes + edges. Filter to slices whose parentNPU is in scope.
	for _, sl := range in.Slices {
		if _, ok := npuIDs[sl.ParentNPU]; !ok {
			continue
		}
		attrs := map[string]interface{}{
			"parentNPU": sl.ParentNPU,
			"template":  sl.Template,
			"aiCore":    sl.AICore,
			"vramMiB":   sl.VRAMMiB,
		}
		if sl.AllocatedTo != nil {
			attrs["allocatedTo"] = map[string]interface{}{
				"namespace":     sl.AllocatedTo.Namespace,
				"podName":       sl.AllocatedTo.PodName,
				"containerName": sl.AllocatedTo.ContainerName,
			}
		}
		out.Nodes = append(out.Nodes, model.TopologyNode{
			ID:         sl.ID,
			Type:       "slice",
			Label:      sl.ID,
			Status:     sl.Status,
			Attributes: attrs,
		})
		out.Edges = append(out.Edges, model.TopologyEdge{
			Source: sl.ParentNPU,
			Target: sl.ID,
			Type:   "contains",
		})
	}

	return out
}

// clusterLabel picks a human label for the cluster topology node. Falls back
// to the id when name is missing so the G6 render never shows a blank node.
func clusterLabel(c *model.Cluster) string {
	if c.Name != "" {
		return c.Name
	}
	return c.ID
}

// npuLabel formats an NPU label as "<model>#<index>" when both are present,
// degrading gracefully to the id.
func npuLabel(n *model.NPU) string {
	if n.Model != "" {
		// Index 0 is valid, so we always print it.
		return n.Model + "#" + strconv.Itoa(n.Index)
	}
	return n.ID
}
