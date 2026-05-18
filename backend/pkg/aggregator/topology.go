// Package aggregator builds DTOs that span more than one datasource resource.
//
// P1-T-102 introduces the first member: BuildTopology, which assembles the
// cluster → node → npu → slice tree the frontend G6 renderer consumes. The
// function is intentionally pure (no datasource imports, no I/O) so it
// table-tests cheaply and so any Source — mock today, k8s + crd tomorrow —
// can feed it preloaded slices and reuse the same wire shape.
//
// Wire shape: see docs/api-contract.yaml components.schemas.{Topology,
// TopologyNode, TopologyEdge}. TopologyNode.Type ∈ {cluster, node, npu, slice,
// switch} (we don't emit nodepool here; that joins later phases).
// TopologyEdge.Type is "contains" for the parent → child relations and
// "fabric-link" when ADR-0004 fabric extension is enabled.
//
// P1-T-211 extends the input bundle with Switches/Links + IncludeFabric flag.
// When IncludeFabric is false (default) the output graph is byte-equivalent
// to the T102 build — zero regression for the depth=node/npu/slice paths.
package aggregator

import (
	"strconv"
	"strings"
	"time"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// Topology node / edge type constants. Stringly-typed in the contract; we
// hoist them as constants so aggregator and any future helper share the same
// spelling.
const (
	nodeTypeCluster = "cluster"
	nodeTypeNode    = "node"
	nodeTypeNPU     = "npu"
	nodeTypeSlice   = "slice"
	nodeTypeSwitch  = "switch" // ADR-0004 fabric: a network switch in the inter-node fabric

	edgeTypeContains    = "contains"
	edgeTypeFabricLink  = "fabric-link" // ADR-0004 fabric: a node↔switch (or switch↔switch) link
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

// NetworkSwitch mirrors the on-disk shape of configs/mock-data/schema.json
// $defs.NetworkSwitch (ADR-0004). Lives in the aggregator package because (a)
// BuildTopology is its sole consumer today and (b) the backend's model/
// package — owned by the API DTO layer — does not yet promote fabric types
// to its public surface (Phase 2 will, when k8s-source fabric discovery lands
// and a real DTO is needed on the /api/v1/network endpoint).
//
// All fields except Type are optional from the schema's perspective. We keep
// JSON tags identical to the schema so the mock loader can unmarshal directly.
type NetworkSwitch struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Type          string   `json:"type"` // tor | leaf | spine | access
	Location      string   `json:"location,omitempty"`
	PortsTotal    int      `json:"portsTotal,omitempty"`
	PortsUsed     int      `json:"portsUsed,omitempty"`
	BandwidthGbps int      `json:"bandwidthGbps,omitempty"`
	VLANs         []string `json:"vlans,omitempty"`
	Status        string   `json:"status"` // up | degraded | down
}

// NetworkLink mirrors $defs.NetworkLink. From / To carry node id or switch id —
// the aggregator does not enforce which (the mock fixtures decide); both ends
// must already exist as Topology nodes for the link to materialize as a fabric
// edge.
type NetworkLink struct {
	ID            string  `json:"id"`
	From          string  `json:"from"`
	To            string  `json:"to"`
	BandwidthGbps int     `json:"bandwidthGbps"`
	Medium        string  `json:"medium,omitempty"`      // copper | fiber | dac | optical
	Utilization   float64 `json:"utilization,omitempty"` // 0-100
	RTTUs         float64 `json:"rttUs,omitempty"`
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

	// IncludeFabric, when true, asks BuildTopology to emit `type=switch`
	// nodes (from Switches) and `type=fabric-link` edges (from Links). When
	// false the output is byte-equivalent to the T102 graph; Switches / Links
	// are ignored entirely. See ADR-0004.
	IncludeFabric bool

	// Switches / Links are consumed only when IncludeFabric is true. The
	// aggregator drops any link whose endpoints don't resolve to a node id
	// or switch id already present in the graph (defensive: keeps the wire
	// payload self-consistent for the frontend renderer).
	Switches []NetworkSwitch
	Links    []NetworkLink

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
		Type:   nodeTypeCluster,
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
			Type:   nodeTypeNode,
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
			Type:   edgeTypeContains,
		})
		nodeIDs[nd.Name] = struct{}{}
	}

	if depth == DepthNode {
		// Fabric (switches + links) is depth-agnostic per ADR-0004 — when
		// requested we emit it even at depth=node so the Overview page can
		// show "cluster + nodes + switches + links" without having to ask
		// for depth=npu/slice. Same logic at the end of npu / slice depth.
		appendFabric(out, in, nodeIDs)
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
			Type:   nodeTypeNPU,
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
			Type:   edgeTypeContains,
		})
		npuIDs[npu.ID] = struct{}{}
	}

	if depth == DepthNPU {
		appendFabric(out, in, nodeIDs)
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
			Type:       nodeTypeSlice,
			Label:      sl.ID,
			Status:     sl.Status,
			Attributes: attrs,
		})
		out.Edges = append(out.Edges, model.TopologyEdge{
			Source: sl.ParentNPU,
			Target: sl.ID,
			Type:   edgeTypeContains,
		})
	}

	// 5) fabric (ADR-0004) — appended last so depth-trimmed graphs keep
	// switches grouped after the deepest tree-layer node. The order is wire-
	// stable but the frontend renderer is order-insensitive.
	appendFabric(out, in, nodeIDs)
	return out
}

// appendFabric emits `type=switch` nodes (one per NetworkSwitch) and
// `type=fabric-link` edges (one per NetworkLink whose endpoints resolve).
// No-op when IncludeFabric is false — this is the zero-regression contract
// the AC depends on.
//
// Endpoint resolution rules:
//   - link.from / link.to must point at either an in-scope node id (i.e.,
//     present in nodeIDs after cluster filtering) or a switch id we just
//     emitted (i.e., in switchIDs). Otherwise the link is dropped silently.
//   - The aggregator does not enforce direction (from/to are symmetric for
//     a fabric link), but we preserve the JSON order on the wire so the G6
//     renderer can pick whichever side it draws as source.
//
// switchIDs is built lazily here rather than carried in TopologyInputs because
// the input struct is the caller's contract — switches arrive as a slice,
// and indexing them is the aggregator's responsibility.
func appendFabric(out *model.Topology, in TopologyInputs, nodeIDs map[string]struct{}) {
	if !in.IncludeFabric {
		return
	}

	switchIDs := make(map[string]struct{}, len(in.Switches))
	for _, sw := range in.Switches {
		if sw.ID == "" {
			continue
		}
		out.Nodes = append(out.Nodes, model.TopologyNode{
			ID:     sw.ID,
			Type:   nodeTypeSwitch,
			Label:  switchLabel(sw),
			Status: sw.Status,
			Attributes: map[string]interface{}{
				"switchType":    sw.Type, // "type" is the wire-level node type already; use "switchType" for the tor/leaf/spine/access distinction
				"location":      sw.Location,
				"portsTotal":    sw.PortsTotal,
				"portsUsed":     sw.PortsUsed,
				"bandwidthGbps": sw.BandwidthGbps,
				"vlans":         sw.VLANs,
			},
		})
		switchIDs[sw.ID] = struct{}{}
	}

	for _, lk := range in.Links {
		if lk.ID == "" || lk.From == "" || lk.To == "" {
			continue
		}
		if !endpointInScope(lk.From, nodeIDs, switchIDs) {
			continue
		}
		if !endpointInScope(lk.To, nodeIDs, switchIDs) {
			continue
		}
		out.Edges = append(out.Edges, model.TopologyEdge{
			Source: lk.From,
			Target: lk.To,
			Type:   edgeTypeFabricLink,
			Attributes: map[string]interface{}{
				"id":            lk.ID,
				"bandwidthGbps": lk.BandwidthGbps,
				"medium":        lk.Medium,
				"utilization":   lk.Utilization,
				"rttUs":         lk.RTTUs,
			},
		})
	}
}

// endpointInScope reports whether id is a node we emitted or a switch we just
// emitted. Either qualifies for a fabric link endpoint.
func endpointInScope(id string, nodeIDs, switchIDs map[string]struct{}) bool {
	if _, ok := nodeIDs[id]; ok {
		return true
	}
	_, ok := switchIDs[id]
	return ok
}

// switchLabel formats a switch label as its Name, falling back to ID. Mirrors
// clusterLabel / npuLabel conventions so the G6 render never blanks a node.
func switchLabel(sw NetworkSwitch) string {
	if sw.Name != "" {
		return sw.Name
	}
	return sw.ID
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
