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
// switch, workload, pod}. TopologyEdge.Type ∈ {contains, fabric-link, network,
// hccs, binds-to, pd-pair, runs-on}. ADR-0006 promoted the runtime wire enum
// into the contract; ADR-0021 (P12-T-104) adds the full-fidelity hardware
// edges: `network` (node↔node inter-node link · classified from a NetworkLink
// whose both ends are nodes) + `hccs` (npu↔npu intra-node · same node +
// hccsGroup, ring per appendHCCS) + `runs-on` (non-NPU pod↔node), plus the
// `pcieBandwidthGBps` npu node attribute. network + hccs ride the IncludeFabric
// toggle; runs-on rides IncludeWorkloads; pcie is stamped only when present —
// so the both-flags-false graph stays byte-equivalent to the T102 build.
//
// P1-T-211 extends the input bundle with Switches/Links + IncludeFabric flag.
// P1-T-213 extends it again with Workloads + IncludeWorkloads (ADR-0005).
// When both flags are false (default) the output graph is byte-equivalent
// to the T102 build — zero regression for the depth=node/npu/slice paths.
// Fabric and workload branches coexist independently when both flags are on.
//
// P13-T-103 (ADR-0024 §2 Decision G — decoupling-seam invariant): this package
// is the seam-ABOVE shared layer and MUST stay source-agnostic. The k8s / crd
// sources feed the SAME TopologyInputs the mock source does — HCCS rings arrive
// as NPUs whose hccsGroup/HCCSBandwidthGBps the Source already populated (from
// ResourceSlice attributes / NPUPool.status.hccsTopology for the real sources,
// from JSON fixtures for the mock), and appendHCCS rings them up through one
// code path. The only mock-vs-real difference is which Source the datasource
// layer selected (config mapping.topology) — there is no source-conditional
// branch in this file, and adding one would break the invariant and let
// demo/real regressions diverge. Keep this file free of any source-name check.
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
	nodeTypeCluster  = "cluster"
	nodeTypeNode     = "node"
	nodeTypeNPU      = "npu"
	nodeTypeSlice    = "slice"
	nodeTypeSwitch   = "switch"   // ADR-0004 fabric: a network switch in the inter-node fabric
	nodeTypeWorkload = "workload" // ADR-0005 workload fusion: top-level workload aggregate
	nodeTypePod      = "pod"      // ADR-0005 workload fusion: individual pod under a workload

	edgeTypeContains   = "contains"
	edgeTypeFabricLink = "fabric-link" // ADR-0004 fabric: a node↔switch (or switch↔switch) link
	edgeTypeNetwork    = "network"     // ADR-0021: node↔node inter-node 互通 link (both endpoints are nodes)
	edgeTypeHCCS       = "hccs"        // ADR-0021: npu↔npu intra-node HCCS link (same node + hccsGroup)
	edgeTypeBindsTo    = "binds-to"    // ADR-0005 workload fusion: pod ↔ slice binding
	edgeTypePDPair     = "pd-pair"     // ADR-0005 workload fusion: prefill ↔ decode pod relation
	edgeTypeRunsOn     = "runs-on"     // ADR-0021: 非 NPU workload/pod ↔ node compute placement (no slice binding)
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
	BandwidthGbps int     `json:"bandwidthGbps"`           // ADR-0004 legacy unit (Gbps · gigabits) · fabric-link 沿用
	BandwidthGBps float64 `json:"bandwidthGBps,omitempty"` // ADR-0021 unit (GB/s · gigabytes) · network/hccs/fabric hover · 与 NPU.pcieBandwidthGBps 同尺度
	Medium        string  `json:"medium,omitempty"`        // copper | fiber | dac | optical | eth | roce | ib
	Utilization   float64 `json:"utilization,omitempty"`   // 0-100
	RTTUs         float64 `json:"rttUs,omitempty"`
}

// WorkloadInput is the aggregator-local minimal shape needed to fold workload
// + pod nodes into the topology graph (ADR-0005). We keep this type here —
// rather than rely on pkg/model.WorkloadDetail — because (a) the model
// package's Pod type does not yet carry the `bindings` array T013 added to
// the schema (the OpenAPI contract migration is a follow-up RFC) and (b) the
// aggregator already owns NetworkSwitch / NetworkLink as local shapes for the
// same boundary-cleanliness reason.
//
// JSON tags mirror configs/mock-data/schema.json so the mock loader can
// unmarshal directly.
type WorkloadInput struct {
	Name      string             `json:"name"`
	Namespace string             `json:"namespace"`
	Kind      string             `json:"kind,omitempty"`
	Status    string             `json:"status,omitempty"`
	Type      string             `json:"type,omitempty"`
	Replicas  *WorkloadReplicas  `json:"replicas,omitempty"`
	NodeNames []string           `json:"nodeNames,omitempty"`
	Pods      []WorkloadPod      `json:"pods,omitempty"`
	Relations []WorkloadRelation `json:"relations,omitempty"`
}

// WorkloadReplicas mirrors components.schemas.Workload.replicas inline. Kept
// pointer-shaped at the parent so callers can distinguish "absent" from
// "desired=0 ready=0".
type WorkloadReplicas struct {
	Desired int `json:"desired"`
	Ready   int `json:"ready"`
}

// WorkloadPod carries the subset of pod fields the aggregator needs to emit
// pod nodes + binds-to edges. Bindings is the canonical T013 schema field
// (`$defs.Pod.bindings`) — per-pod slice attachments with optional role for
// PD-disaggregated workloads.
type WorkloadPod struct {
	Name      string       `json:"name"`
	Namespace string       `json:"namespace,omitempty"`
	NodeName  string       `json:"nodeName,omitempty"`
	Status    string       `json:"status,omitempty"`
	Bindings  []PodBinding `json:"bindings,omitempty"`
}

// PodBinding mirrors $defs.Pod.bindings entries. Role is informational
// (prefill / decode / peer / primary / sidecar); the aggregator emits one
// binds-to edge per (pod, sliceId) pair regardless of role.
type PodBinding struct {
	SliceID    string `json:"sliceId"`
	Role       string `json:"role,omitempty"`
	IndexInPod int    `json:"indexInPod,omitempty"`
}

// WorkloadRelation mirrors $defs.Workload.relations entries. Phase 1 surfaces
// only `pd-pair` as a topology edge (others — sidecar / init / peer — exist
// in the contract but lack a dedicated visual; we drop them on the wire to
// keep the rendered graph readable). Source / target are pod names within
// the same workload.
type WorkloadRelation struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
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

	// IncludeWorkloads, when true (P1-T-213, ADR-0005), asks BuildTopology to
	// emit workload + pod nodes plus binds-to / pd-pair edges. When false the
	// output is byte-equivalent to the T102 / T211 graph; Workloads is ignored
	// entirely (zero-regression contract). See ADR-0005.
	IncludeWorkloads bool

	// Workloads are consumed only when IncludeWorkloads is true. Pod → slice
	// binds-to edges drop silently when the slice id isn't already present in
	// the graph (defensive: matches the orphan-NPU guard for the same
	// self-consistency reason). pd-pair relations drop silently when either
	// pod isn't represented.
	Workloads []WorkloadInput

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
		// for depth=npu/slice. Workloads are depth-agnostic for the same
		// reason (ADR-0005). Same logic at the end of npu / slice depth.
		appendFabric(out, in, nodeIDs)
		// At depth=node no slices were emitted, so the empty sliceIDs map
		// drops all binds-to edges — workload + pod nodes still surface so
		// the Overview can render "show workloads" cleanly without slices.
		// No appendHCCS at depth=node — no NPUs are emitted to connect.
		appendWorkloads(out, in, nodeIDs, map[string]struct{}{})
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
		npuAttrs := map[string]interface{}{
			"model":       npu.Model,
			"index":       npu.Index,
			"vramMiB":     npu.VRAMMiB,
			"aiCoreTotal": npu.AICoreTotal,
			"hccsGroup":   npu.HCCSGroup,
			"sliceMode":   npu.SliceMode,
		}
		// ADR-0021: host↔NPU PCIe bandwidth (GB/s). Only stamped when the
		// fixture supplies it, so pre-ADR-0021 fixtures stay byte-equivalent.
		if npu.PCIeBandwidthGBps != nil {
			npuAttrs["pcieBandwidthGBps"] = *npu.PCIeBandwidthGBps
		}
		out.Nodes = append(out.Nodes, model.TopologyNode{
			ID:         npu.ID,
			Type:       nodeTypeNPU,
			Label:      npuLabel(npu),
			Status:     npu.Status,
			Attributes: npuAttrs,
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
		appendHCCS(out, in, npuIDs)
		// At depth=npu no slices were emitted; the empty sliceIDs map drops
		// all binds-to edges, but workload + pod nodes still render.
		appendWorkloads(out, in, nodeIDs, map[string]struct{}{})
		return out
	}

	// 4) slice nodes + edges. Filter to slices whose parentNPU is in scope.
	// We track emitted slice ids so the workload branch's binds-to edges can
	// drop silently when a pod points at an out-of-scope (or non-existent)
	// slice — same self-consistency guard as fabric links.
	sliceIDs := make(map[string]struct{}, len(in.Slices))
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
		sliceIDs[sl.ID] = struct{}{}
	}

	// 5) fabric (ADR-0004) — appended after the deepest tree-layer node so
	// depth-trimmed graphs keep switches grouped at the tail. The order is
	// wire-stable but the frontend renderer is order-insensitive.
	appendFabric(out, in, nodeIDs)
	appendHCCS(out, in, npuIDs)

	// 6) workloads (ADR-0005) — appended last. Workload branch is depth-
	// agnostic: even at depth=node we still emit workload + pod nodes, since
	// the user has explicitly asked for them. binds-to edges silently drop
	// when the target slice isn't in scope at that depth (e.g. depth=node
	// emits no slices, so all binds-to fall away — consistent with how the
	// rest of the depth filter works).
	appendWorkloads(out, in, nodeIDs, sliceIDs)
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
		// ADR-0021: classify by endpoint. A link whose BOTH ends are nodes is
		// an inter-node `network` link; any switch endpoint keeps it an ADR-0004
		// `fabric-link` (node↔switch / switch↔switch). Bandwidth attr differs by
		// unit convention (ADR-0021 §4 b): network → bandwidthGBps (GB/s) ·
		// fabric → bandwidthGbps (Gbps legacy) + bandwidthGBps when supplied.
		_, fromIsNode := nodeIDs[lk.From]
		_, toIsNode := nodeIDs[lk.To]
		if fromIsNode && toIsNode {
			netAttrs := map[string]interface{}{
				"id":          lk.ID,
				"medium":      lk.Medium,
				"utilization": lk.Utilization,
			}
			if lk.BandwidthGBps != 0 {
				netAttrs["bandwidthGBps"] = lk.BandwidthGBps
			}
			out.Edges = append(out.Edges, model.TopologyEdge{
				Source:     lk.From,
				Target:     lk.To,
				Type:       edgeTypeNetwork,
				Attributes: netAttrs,
			})
			continue
		}
		fabricAttrs := map[string]interface{}{
			"id":            lk.ID,
			"bandwidthGbps": lk.BandwidthGbps,
			"medium":        lk.Medium,
			"utilization":   lk.Utilization,
			"rttUs":         lk.RTTUs,
		}
		if lk.BandwidthGBps != 0 {
			fabricAttrs["bandwidthGBps"] = lk.BandwidthGBps
		}
		out.Edges = append(out.Edges, model.TopologyEdge{
			Source:     lk.From,
			Target:     lk.To,
			Type:       edgeTypeFabricLink,
			Attributes: fabricAttrs,
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

// appendHCCS emits `type=hccs` edges between NPUs that share the same node AND
// the same hccsGroup (ADR-0021 — intra-node NPU interconnect). No-op when
// IncludeFabric is false: HCCS rides the same hardware-topology toggle as the
// fabric/network edges, so the IncludeFabric=false graph stays byte-equivalent
// to the pre-ADR-0021 build. Only NPUs already emitted at depth≥npu (present
// in npuIDs) participate; called only from the npu / slice depth branches.
//
// Topology: NPUs in a group are sorted by Index and wired into a ring
// (npu0↔npu1↔…↔npuN-1↔npu0). A 2-NPU group is a single edge; ≥3 forms a closed
// ring (matches ADR-0010's 910B ring-of-rings adjacency). The edge's
// bandwidthGBps comes from the source NPU's HCCSBandwidthGBps (symmetric within
// a group), omitted when the fixture doesn't supply it.
func appendHCCS(out *model.Topology, in TopologyInputs, npuIDs map[string]struct{}) {
	if !in.IncludeFabric {
		return
	}

	type groupKey struct{ node, group string }
	groups := make(map[groupKey][]*model.NPU)
	order := make([]groupKey, 0)
	for _, npu := range in.NPUs {
		if npu == nil || npu.HCCSGroup == "" {
			continue
		}
		if _, ok := npuIDs[npu.ID]; !ok {
			continue
		}
		k := groupKey{npu.NodeName, npu.HCCSGroup}
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], npu)
	}

	for _, k := range order {
		members := groups[k]
		if len(members) < 2 {
			continue // a lone NPU in a group has no intra-group HCCS peer
		}
		sortNPUsByIndex(members)
		n := len(members)
		limit := n // closed ring → n edges
		if n == 2 {
			limit = 1 // a single pair → one edge (0↔1); avoid the dup 1↔0
		}
		for i := 0; i < limit; i++ {
			a := members[i]
			b := members[(i+1)%n]
			attrs := map[string]interface{}{
				"hccsGroup": k.group,
			}
			if a.HCCSBandwidthGBps != nil {
				attrs["bandwidthGBps"] = *a.HCCSBandwidthGBps
			}
			out.Edges = append(out.Edges, model.TopologyEdge{
				Source:     a.ID,
				Target:     b.ID,
				Type:       edgeTypeHCCS,
				Attributes: attrs,
			})
		}
	}
}

// sortNPUsByIndex sorts in place by NPU.Index ascending (stable insertion sort
// — N is tiny per hccsGroup so this stays dependency-free and deterministic).
func sortNPUsByIndex(npus []*model.NPU) {
	for i := 1; i < len(npus); i++ {
		for j := i; j > 0 && npus[j-1].Index > npus[j].Index; j-- {
			npus[j-1], npus[j] = npus[j], npus[j-1]
		}
	}
}

// appendWorkloads emits workload + pod nodes and binds-to + pd-pair edges
// when IncludeWorkloads is true. No-op otherwise — preserving the
// T102 / T211 byte-equivalent contract.
//
// Emission rules:
//   - One node per workload (id = "workload/<namespace>/<name>"). Empty
//     namespace or name → skip silently (defensive against malformed
//     fixtures).
//   - One node per pod (id = "pod/<namespace>/<podName>") with parentId
//     stashed in Attributes (model.TopologyNode has no ParentID field).
//     Pod nodes whose workload was skipped don't materialize. Pods with
//     empty names skip silently.
//   - One binds-to edge per (pod, sliceId) pair drawn from pod.bindings. The
//     edge drops when the slice id isn't already present in the graph
//     (sliceIDs). This is the orphan-binding guard mirroring fabric/NPU
//     orphan handling — keeps the wire payload self-consistent for the
//     renderer.
//   - For workload.relations entries with type=pd-pair, one pd-pair edge
//     between the two pod node ids. Other relation types (sidecar / init /
//     peer) are skipped in Phase 1 — the contract enumerates them but the
//     frontend has no dedicated styling, so we omit rather than emit
//     unrendered noise. Pd-pair edges drop when either pod isn't in the
//     emitted set (same self-consistency reason).
//
// The id schemes intentionally include the namespace+kind prefix so a
// workload and a pod with colliding short names can never produce the same
// topology node id.
func appendWorkloads(out *model.Topology, in TopologyInputs, nodeIDs, sliceIDs map[string]struct{}) {
	if !in.IncludeWorkloads {
		return
	}

	for _, wl := range in.Workloads {
		if wl.Name == "" || wl.Namespace == "" {
			continue
		}
		wid := workloadNodeID(wl.Namespace, wl.Name)
		wattrs := map[string]interface{}{
			"namespace": wl.Namespace,
			"kind":      wl.Kind,
			"type":      wl.Type,
			"nodeNames": wl.NodeNames,
		}
		if wl.Replicas != nil {
			wattrs["replicas"] = map[string]interface{}{
				"desired": wl.Replicas.Desired,
				"ready":   wl.Replicas.Ready,
			}
		}
		out.Nodes = append(out.Nodes, model.TopologyNode{
			ID:         wid,
			Type:       nodeTypeWorkload,
			Label:      wl.Namespace + "/" + wl.Name,
			Status:     wl.Status,
			Attributes: wattrs,
		})

		// 2) pods — one node per pod, parentId points at the workload.
		// Local index of this workload's pod names → pod node id so the
		// relations loop below can resolve from/to without re-scanning.
		wlPodIDs := make(map[string]string, len(wl.Pods))
		for _, pod := range wl.Pods {
			if pod.Name == "" {
				continue
			}
			pid := podNodeID(wl.Namespace, pod.Name)
			out.Nodes = append(out.Nodes, model.TopologyNode{
				ID:     pid,
				Type:   nodeTypePod,
				Label:  pod.Name,
				Status: pod.Status,
				Attributes: map[string]interface{}{
					"namespace": pod.Namespace,
					"nodeName":  pod.NodeName,
					"parentId":  wid, // parent workload id (model.TopologyNode has no ParentID column; frontend reads attributes.parentId)
					"workload":  wid,
				},
			})
			wlPodIDs[pod.Name] = pid

			// 3) binds-to — one edge per pod binding to a slice already
			// emitted earlier. Out-of-scope slice ids drop silently. The
			// edge attributes carry role + index so the frontend can label
			// "prefill" / "decode" / "primary" / "sidecar" without a join.
			for _, b := range pod.Bindings {
				if b.SliceID == "" {
					continue
				}
				if _, ok := sliceIDs[b.SliceID]; !ok {
					continue
				}
				out.Edges = append(out.Edges, model.TopologyEdge{
					Source: pid,
					Target: b.SliceID,
					Type:   edgeTypeBindsTo,
					Attributes: map[string]interface{}{
						"role":       b.Role,
						"indexInPod": b.IndexInPod,
					},
				})
			}

			// ADR-0021: runs-on — a non-NPU pod (no slice bindings) connects to
			// the node it runs on. Only when the pod declares zero bindings AND
			// its nodeName resolves to an in-scope node (mirrors the binds-to
			// orphan guard). NPU pods carry binds-to instead, so the two edge
			// kinds never both fire for one pod.
			if len(pod.Bindings) == 0 && pod.NodeName != "" {
				if _, ok := nodeIDs[pod.NodeName]; ok {
					out.Edges = append(out.Edges, model.TopologyEdge{
						Source: pid,
						Target: pod.NodeName,
						Type:   edgeTypeRunsOn,
						Attributes: map[string]interface{}{
							"workload": wid,
						},
					})
				}
			}
		}

		// 4) pd-pair edges — phase 1 surfaces only the pd-pair relation
		// type. from/to must both resolve to pod ids we just emitted within
		// this workload (the relation field is workload-scoped per
		// schema.json $defs.PodRelation). Cross-workload relations don't
		// happen today; we keep the lookup local for clarity and to avoid
		// accidental name collisions.
		for _, rel := range wl.Relations {
			if rel.Type != "pd-pair" {
				continue
			}
			fromID, fromOK := wlPodIDs[rel.From]
			toID, toOK := wlPodIDs[rel.To]
			if !fromOK || !toOK {
				continue
			}
			out.Edges = append(out.Edges, model.TopologyEdge{
				Source: fromID,
				Target: toID,
				Type:   edgeTypePDPair,
				Attributes: map[string]interface{}{
					"workload": wid,
				},
			})
		}
	}
}

// workloadNodeID composes the topology node id for a workload. Namespaced
// to avoid collisions when the same workload short name lives in different
// namespaces (e.g. "ai-inference/qwen-8b" vs "training/qwen-8b").
func workloadNodeID(namespace, name string) string {
	return "workload/" + namespace + "/" + name
}

// podNodeID composes the topology node id for a pod. Pod names are unique
// within a namespace per kubernetes semantics, so namespace+name is enough.
// The "pod/" prefix prevents collision with workload ids that share the
// same namespace+name.
func podNodeID(namespace, podName string) string {
	return "pod/" + namespace + "/" + podName
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
