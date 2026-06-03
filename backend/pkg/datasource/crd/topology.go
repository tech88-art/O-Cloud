package crd

// Package crd — real-topology aggregation (P13-T-103 · ADR-0024 §2 Decision C).
//
// GetTopology builds the cluster→node→npu HCCS graph from the pool-operator's
// NPUPool.status.hccsTopology aggregation (Phase 6 P6-T-003). The pool-operator
// calls Source.QueryTopology (the real-Ascend body, P13-T-101) and writes the
// resulting HCCS peer groups onto each NPUPool's status; this reader projects
// those peer groups back into the topology DTO.
//
//	NPUPool.status.hccsTopology.peerGroups[]{ groupId, deviceIds[] }
//	          │
//	          ▼  (deviceId convention "<node>-npu-<index>")
//	cluster ──contains──▶ node ──contains──▶ npu ──hccs──▶ npu
//
// The crd.Source is a pool-scoped source — it owns no Node / NPU objects of its
// own (ListNodes / ListNPUs return ErrCapabilityUnavailable). So the topology
// here is SYNTHESIZED from the peer-group device ids: node nodes are the set of
// hostnames parsed from the device ids, npu nodes are the device ids themselves,
// and each device's hccsGroup is its peer-group id (namespaced by node so two
// nodes' "ring-0" never collapse into one cross-node group). The aggregator then
// emits the intra-node hccs edges from those groups when IncludeFabric=true —
// the SAME BuildTopology code path the k8s + mock sources drive (ADR-0024 §2
// Decision G decoupling-seam invariant: no profile branch here or downstream).
//
// Depth handling mirrors the other sources: depth=node drops the npu layer;
// depth=npu/slice keeps it (there is no slice layer on this path — the crd
// source's slice projection is the separate ResolveSlicesForNPUs primitive that
// the aggregator-wiring task would join; the HCCS topology view is npu-deep).

import (
	"context"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/example/ocloud-edge/backend/pkg/aggregator"
	"github.com/example/ocloud-edge/backend/pkg/datasource"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// topologyClusterID is the synthetic cluster id the crd-sourced topology hangs
// its nodes under. The crd source serves one apiserver (= one cluster); the
// pool CRDs carry no cluster id of their own, so we synthesize a stable literal.
// Matches the k8s source's fallback id so a deployment that routes clusters→k8s
// + topology→crd renders both under the same node.
const topologyClusterID = "kubernetes"

// hccs910BBandwidthGBps is the datasheet-nominal per-NPU HCCS link bandwidth
// (GB/s) stamped on synthesized NPUs so the aggregator's hccs edges carry a
// bandwidthGBps hover attribute (ADR-0021: 910B HCCS ≈ 56 GB/s). Identical to
// the k8s source's nominal so demo/real/k8s/crd hovers are all consistent.
const hccs910BBandwidthGBps = 56.0

// GetTopology assembles the topology DTO for clusterID from NPUPool HCCS status
// (P13-T-103). Delegates to getTopology with zero-value TopologyOptions so the
// zero-regression contract holds: GetTopology bytes ≡ GetTopologyWithFabric(
// opts{}) bytes.
//
// Errors:
//   - ctx.Err() propagates verbatim.
//   - Unknown cluster id → ErrResourceNotFound (handler maps to 404).
//   - apiserver list failures bubble up wrapped (handler maps to 500).
func (s *Source) GetTopology(ctx context.Context, clusterID string, depth string) (*model.Topology, error) {
	return s.getTopology(ctx, clusterID, depth, datasource.TopologyOptions{})
}

// GetTopologyWithFabric is the option-bag variant. IncludeFabric=true adds the
// ADR-0021 hccs edges between same-node same-peer-group NPUs. IncludeWorkloads
// is a no-op for this source (the crd source has no workload/pod surface on the
// topology path). When both flags are false the response is byte-equivalent to
// GetTopology.
func (s *Source) GetTopologyWithFabric(ctx context.Context, clusterID string, depth string, opts datasource.TopologyOptions) (*model.Topology, error) {
	return s.getTopology(ctx, clusterID, depth, opts)
}

// getTopology is the shared implementation behind both entry points.
func (s *Source) getTopology(ctx context.Context, clusterID string, depth string, opts datasource.TopologyOptions) (*model.Topology, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if clusterID != "" && clusterID != topologyClusterID {
		return nil, ErrResourceNotFound
	}

	pools, err := s.ListNPUPools(ctx)
	if err != nil {
		return nil, err
	}

	nodeSet, npus := s.projectPoolsToNPUs(pools)

	cluster := &model.Cluster{
		ID:        topologyClusterID,
		Name:      topologyClusterID,
		Role:      "edge-single",
		Status:    "healthy",
		NodeCount: len(nodeSet),
		NPUCount:  len(npus),
	}

	nodes := make([]model.NodeDetail, 0, len(nodeSet))
	for _, name := range sortedKeys(nodeSet) {
		nodes = append(nodes, model.NodeDetail{Node: model.Node{
			Name:      name,
			ClusterID: topologyClusterID,
			Status:    "Ready",
			NPUCount:  nodeSet[name],
		}})
	}

	topo := aggregator.BuildTopology(aggregator.TopologyInputs{
		Cluster:          cluster,
		Nodes:            nodes,
		NPUs:             npus,
		Depth:            depth,
		IncludeFabric:    opts.IncludeFabric,
		IncludeWorkloads: opts.IncludeWorkloads,
	})
	return topo, nil
}

// projectPoolsToNPUs walks every NPUPool's status.hccsTopology.peerGroups and
// synthesizes the (nodeName → npu-count) set plus the flat NPU list the
// aggregator consumes. Device ids that don't match the "<node>-npu-<index>"
// convention are skipped (defensive). Each NPU's hccsGroup is its peer-group id
// namespaced by node so the aggregator rings up intra-node peers only.
//
// Returns the node set (name → npu count for the cluster/node attributes) and
// the NPUs sorted by id (stable wire output across reconciles).
func (s *Source) projectPoolsToNPUs(pools []*unstructured.Unstructured) (map[string]int, []*model.NPU) {
	nodeCount := map[string]int{}
	seen := map[string]struct{}{} // dedup device ids across pools/groups
	npus := []*model.NPU{}

	for _, u := range pools {
		for _, pg := range peerGroupsOf(u) {
			for _, devID := range pg.deviceIDs {
				if devID == "" {
					continue
				}
				node, ok := nodeOfDeviceID(devID)
				if !ok {
					continue
				}
				if _, dup := seen[devID]; dup {
					continue
				}
				seen[devID] = struct{}{}
				nodeCount[node]++
				bw := hccs910BBandwidthGBps
				npus = append(npus, &model.NPU{
					ID:       devID,
					NodeName: node,
					Model:    "Ascend910B",
					Index:    indexOfDeviceID(devID),
					// hccsGroup namespaced by node: the aggregator groups hccs
					// edges by (node, hccsGroup), but namespacing the group id
					// too keeps the value self-describing and collision-free if
					// the aggregator's grouping ever changes.
					HCCSGroup:         node + "-" + pg.groupID,
					Status:            "healthy",
					SliceMode:         "whole",
					HCCSBandwidthGBps: &bw,
				})
			}
		}
	}

	sort.SliceStable(npus, func(i, j int) bool { return npus[i].ID < npus[j].ID })
	return nodeCount, npus
}

// peerGroup is the projected shape of one NPUPool.status.hccsTopology.peerGroups
// entry.
type peerGroup struct {
	groupID   string
	deviceIDs []string
}

// peerGroupsOf extracts status.hccsTopology.peerGroups from one unstructured
// NPUPool. Returns nil when the path is absent (a pool whose status hasn't been
// populated by the pool-operator yet) — the caller treats that as "no topology
// from this pool".
func peerGroupsOf(u *unstructured.Unstructured) []peerGroup {
	if u == nil {
		return nil
	}
	raw, found, err := unstructured.NestedSlice(u.Object, "status", "hccsTopology", "peerGroups")
	if err != nil || !found {
		return nil
	}
	out := make([]peerGroup, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		gid, _, _ := unstructured.NestedString(m, "groupId")
		if gid == "" {
			continue
		}
		ids, _, _ := unstructured.NestedStringSlice(m, "deviceIds")
		out = append(out, peerGroup{groupID: gid, deviceIDs: ids})
	}
	return out
}

// nodeOfDeviceID parses the node hostname out of a "<node>-npu-<index>" device
// id. Returns ("",false) when the id doesn't carry the "-npu-" separator. The
// node segment may itself contain hyphens (e.g. "atlas-800-01-npu-3"), so we
// split on the LAST "-npu-" occurrence.
func nodeOfDeviceID(devID string) (string, bool) {
	idx := strings.LastIndex(devID, "-npu-")
	if idx <= 0 {
		return "", false
	}
	return devID[:idx], true
}

// indexOfDeviceID parses the trailing NPU index from "<node>-npu-<index>".
// Returns 0 when the suffix isn't a non-negative integer (defensive — the index
// is cosmetic for the topology node label / ring ordering, so a bad suffix
// degrades to 0 rather than dropping the NPU).
func indexOfDeviceID(devID string) int {
	idx := strings.LastIndex(devID, "-npu-")
	if idx < 0 {
		return 0
	}
	suffix := devID[idx+len("-npu-"):]
	n := 0
	for i := 0; i < len(suffix); i++ {
		c := suffix[i]
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// sortedKeys returns the map keys in lexical order for deterministic node
// emission.
func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
