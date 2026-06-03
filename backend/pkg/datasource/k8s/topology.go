package k8s

// Package k8s — real-topology aggregation (P13-T-103 · ADR-0024 §2 Decision C).
//
// GetTopology / GetTopologyWithFabric build the cluster→node→npu(→slice via
// fabric edges) graph the frontend G6 renderer consumes, sourced entirely from
// live apiserver objects:
//
//   - cluster node  : the synthetic single cluster (cluster.go resolution chain)
//   - node nodes    : every Node, projected via node.go's projectNodeListView
//   - npu nodes     : per-Node Ascend capacity, projected via npu.go's
//                     projectNPUs, then ENRICHED from ResourceSlice device
//                     attributes (npu.huawei.com/hccs_ring + numa_node + index)
//                     published by the real-Ascend DRA publisher (P13-T-101)
//   - hccs / network edges: emitted by aggregator.BuildTopology when
//                     IncludeFabric=true — the SAME code path the mock source
//                     drives. The only real-vs-mock difference is the data the
//                     Source loads into TopologyInputs (ADR-0024 §2 Decision G
//                     decoupling-seam invariant: no profile branch here or in
//                     the aggregator).
//
// Why the ResourceSlice enrichment matters: npu.go's static 4-per-NUMA layout
// is a structural placeholder (it groups NPUs by "<node>-hccs-<numa>"). The
// real-Ascend publisher writes the ACTUAL hccs ring + NUMA node it parsed from
// `npu-smi info -t topo` onto each ResourceSlice device. When those attributes
// are present we overlay them so the rendered HCCS rings reflect real silicon
// adjacency rather than the static guess. When the publisher hasn't run yet (or
// the dynamic client is unset in a unit test) we fall back to the structural
// layout — the graph still renders, just with the placeholder grouping.
//
// ResourceSlice GVR is resolved version-agnostically (v1beta1 today, v1 after
// the ADR-0024 §4(a) migration) by trying the known versions in order: the
// dynamic client doesn't care which the cluster serves, and the backend never
// pins a typed resource.k8s.io import (same module-boundary rationale as
// crd.Source's unstructured pool reads).

import (
	"context"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/example/ocloud-edge/backend/pkg/aggregator"
	"github.com/example/ocloud-edge/backend/pkg/datasource"
	"github.com/example/ocloud-edge/backend/pkg/model"
)

// draDriverName is the DRA driver that publishes Ascend ResourceSlices. Mirrors
// operators/npu-dra-driver/api/v1alpha1.DriverName (ADR-0024 §2 Decision C). We
// hard-code the literal rather than import the operators module — the backend
// deliberately avoids the cross-module typed dependency (see crd/source.go pkg
// header). ResourceSlice.spec.driver carrying any OTHER value is ignored.
const draDriverName = "npu.ocloud.edge.example.com"

// ResourceSlice device-attribute QualifiedNames the real-Ascend publisher sets.
// Mirror operators/npu-dra-driver/api/v1alpha1 Attr* constants (P5-T-120 fix:
// C-identifier names, underscores not hyphens). Read as the fully-qualified
// map keys on device.basic.attributes.
const (
	attrHCCSRing = "npu.huawei.com/hccs_ring"
	attrNUMANode = "npu.huawei.com/numa_node"
	attrNPUIndex = "npu.huawei.com/index"
)

// resourceSliceGVRs lists the resource.k8s.io ResourceSlice GVRs we attempt, in
// preference order. v1beta1 is the current publisher target (K8s 1.31 baseline);
// v1 is the GA shape the ADR-0024 §4(a) migration moves to. The dynamic client
// list against an unserved version returns a discovery error, so we fall through
// to the next candidate. Order = newest-GA-first so a migrated cluster is read
// via v1 without a v1beta1 round-trip.
var resourceSliceGVRs = []schema.GroupVersionResource{
	{Group: "resource.k8s.io", Version: "v1", Resource: "resourceslices"},
	{Group: "resource.k8s.io", Version: "v1beta1", Resource: "resourceslices"},
	{Group: "resource.k8s.io", Version: "v1beta2", Resource: "resourceslices"},
	{Group: "resource.k8s.io", Version: "v1alpha3", Resource: "resourceslices"},
}

// hccs910BBandwidthGBps is the per-NPU HCCS link bandwidth stamped on enriched
// NPUs so the aggregator's hccs edges carry a bandwidthGBps attribute matching
// the mock fixtures (ADR-0021: 910B HCCS ≈ 56 GB/s). The apiserver does not
// surface a measured value; this is the datasheet nominal, identical across
// demo/real for the same chip so the frontend hover is profile-consistent.
const hccs910BBandwidthGBps = 56.0

// GetTopology assembles the topology DTO for clusterID from live apiserver
// objects (P13-T-103). Delegates to getTopology with zero-value TopologyOptions
// so the zero-regression contract holds by construction: GetTopology bytes ≡
// GetTopologyWithFabric(opts{}) bytes (matches the mock source's delegation).
//
// Errors:
//   - ctx.Err() propagates verbatim (request cancellation).
//   - Unknown cluster id → ErrResourceNotFound (handler maps to 404).
//   - apiserver list failures bubble up wrapped (handler maps to 500).
func (s *Source) GetTopology(ctx context.Context, clusterID string, depth string) (*model.Topology, error) {
	return s.getTopology(ctx, clusterID, depth, datasource.TopologyOptions{})
}

// GetTopologyWithFabric is the option-bag variant. IncludeFabric=true adds the
// ADR-0021 hccs (npu↔npu intra-node) + network (node↔node) edges built from the
// ResourceSlice-enriched NPUs; IncludeWorkloads folds in workload/pod nodes
// (not yet sourced from k8s here — the k8s source has no pod-binding join on the
// topology path in Phase 13, so workloads stay empty and the flag is a no-op for
// this source). When both flags are false the response is byte-equivalent to
// GetTopology (zero-regression contract).
func (s *Source) GetTopologyWithFabric(ctx context.Context, clusterID string, depth string, opts datasource.TopologyOptions) (*model.Topology, error) {
	return s.getTopology(ctx, clusterID, depth, opts)
}

// getTopology is the shared implementation. Keeps apiserver loading + aggregator
// delegation in one place so the two public entry points can never drift on the
// depth=… path (the zero-regression contract requires byte-equivalence when
// opts.IncludeFabric is false).
func (s *Source) getTopology(ctx context.Context, clusterID string, depth string, opts datasource.TopologyOptions) (*model.Topology, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// 1) Cluster — 404 fast-path if the requested id doesn't match the
	//    Source's single resolved cluster. GetCluster("") returns the
	//    resolved cluster; we then re-check the id so an explicit mismatch is
	//    a 404 rather than silently returning the wrong cluster.
	resolvedID, _ := s.resolveClusterIdentity(ctx)
	if clusterID != "" && clusterID != resolvedID {
		return nil, ErrResourceNotFound
	}
	cluster, err := s.GetCluster(ctx, "")
	if err != nil {
		return nil, err
	}

	// 2) Nodes (always — the shallowest depth still emits them). Reuse the
	//    same projection ListNodes uses so the topology node attributes match
	//    the /nodes endpoint exactly.
	nodeList, err := s.ListNodes(ctx, model.NodeFilter{})
	if err != nil {
		return nil, err
	}
	nodes := make([]model.NodeDetail, 0, len(nodeList))
	for _, n := range nodeList {
		if n == nil {
			continue
		}
		nodes = append(nodes, model.NodeDetail{Node: *n})
	}

	// 3) NPUs (depth=node skips them, mirroring the mock source). Project each
	//    node's Ascend capacity, then enrich from ResourceSlice attributes so
	//    the HCCS rings reflect real silicon adjacency where the publisher has
	//    written them.
	var npus []*model.NPU
	if depth != aggregator.DepthNode {
		npus, err = s.collectNPUs(ctx, nodeList)
		if err != nil {
			return nil, err
		}
	}

	// 4) Slices: the k8s source does not own slice CRDs (crd.Source does), so
	//    the structural k8s topology has no slice layer. depth=slice therefore
	//    renders cluster→node→npu (+fabric); the slice layer arrives only when
	//    mapping.topology routes through a source that resolves slices.
	//    (Leaving Slices empty keeps depth=slice a superset of depth=npu here
	//    rather than erroring.)

	topo := aggregator.BuildTopology(aggregator.TopologyInputs{
		Cluster:       cluster,
		Nodes:         nodes,
		NPUs:          npus,
		Depth:         depth,
		IncludeFabric: opts.IncludeFabric,
		// Switches/Links: the apiserver carries no inter-node fabric inventory
		// (that's the ascend-npu-exporter / network-telemetry surface). HCCS
		// edges are derived by the aggregator from the NPUs' hccsGroup, so the
		// node-internal interconnect still renders without switch/link fixtures.
		IncludeWorkloads: opts.IncludeWorkloads,
	})
	return topo, nil
}

// collectNPUs projects every node's Ascend capacity into model.NPU and overlays
// the real HCCS ring / NUMA node parsed from ResourceSlice device attributes.
//
// The static npu.go layout is the baseline; ResourceSlice attributes (when the
// real-Ascend publisher has written them) override hccsGroup + numaNode so the
// aggregator emits HCCS edges along real silicon adjacency. NPUs the publisher
// hasn't covered keep the structural layout. PCIe + HCCS bandwidth datasheet
// nominals are stamped on every enriched NPU so the aggregator's edge/hover
// attributes are populated (the apiserver exposes no measured bandwidth).
func (s *Source) collectNPUs(ctx context.Context, nodes []*model.Node) ([]*model.NPU, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// ResourceSlice attribute overlay, keyed by device name (= NPU id, the
	// "<node>-npu-<index>" convention shared by the publisher, npu.go and the
	// mock generator). Best-effort: a list failure (CRD absent / RBAC gap) is
	// non-fatal — we log via the wrapped-error pattern and fall back to the
	// structural layout so the demo still renders. This is the seam-below
	// "real source loads real data" half of ADR-0024 §2 Decision G.
	overlay := s.loadResourceSliceOverlay(ctx)

	out := make([]*model.NPU, 0)
	for _, n := range nodes {
		if n == nil {
			continue
		}
		node, err := s.client.CoreV1().Nodes().Get(ctx, n.Name, metav1.GetOptions{})
		if err != nil {
			// A node that vanished between ListNodes and here is dropped, not
			// fatal — the topology is a best-effort snapshot.
			continue
		}
		for _, npu := range projectNPUs(node) {
			if ov, ok := overlay[npu.ID]; ok {
				applyResourceSliceOverlay(npu, ov)
			}
			out = append(out, npu)
		}
	}
	return out, nil
}

// sliceOverlay carries the per-device topology attributes read off a
// ResourceSlice device. hccsRing maps to the model.NPU.HCCSGroup string (the
// aggregator groups HCCS edges by that string); numaNode overrides the static
// NUMA placement.
type sliceOverlay struct {
	hccsRing int64
	numaNode int64
	hasRing  bool
	hasNUMA  bool
}

// loadResourceSliceOverlay lists ResourceSlices published by the Ascend DRA
// driver and indexes their device topology attributes by device name. Returns
// an empty (non-nil) map when the dynamic client is unset, no version is served,
// or no Ascend slices exist — callers treat "absent" as "fall back to the
// structural layout", never as an error.
func (s *Source) loadResourceSliceOverlay(ctx context.Context) map[string]sliceOverlay {
	out := map[string]sliceOverlay{}
	if s.dyn == nil {
		return out
	}

	var items []unstructured.Unstructured
	for _, gvr := range resourceSliceGVRs {
		list, err := s.dyn.Resource(gvr).List(ctx, metav1.ListOptions{})
		if err != nil {
			// Unserved version / discovery error → try the next candidate.
			continue
		}
		if list != nil && len(list.Items) > 0 {
			// First version with actual slices wins. We fall through EMPTY
			// results (don't break on them) because a cluster can serve both
			// resource.k8s.io/v1 and /v1beta1 while the publisher only WROTE
			// one of them — breaking on the empty newer version would miss the
			// slices stored under the older one. A genuinely slice-less cluster
			// probes all candidates and returns empty (a few cheap list calls),
			// then falls back to the structural layout.
			items = list.Items
			break
		}
	}

	for i := range items {
		indexResourceSliceDevices(&items[i], out)
	}
	return out
}

// indexResourceSliceDevices walks one ResourceSlice's spec.devices, filters to
// the Ascend DRA driver, and records each device's hccs_ring / numa_node onto
// the overlay map keyed by device name. Malformed / non-Ascend slices are
// skipped silently (defensive — the apiserver shouldn't produce them).
func indexResourceSliceDevices(u *unstructured.Unstructured, out map[string]sliceOverlay) {
	if u == nil {
		return
	}
	driver, _, _ := unstructured.NestedString(u.Object, "spec", "driver")
	if driver != draDriverName {
		return
	}
	devices, found, err := unstructured.NestedSlice(u.Object, "spec", "devices")
	if err != nil || !found {
		return
	}
	for _, d := range devices {
		dm, ok := d.(map[string]interface{})
		if !ok {
			continue
		}
		name, _, _ := unstructured.NestedString(dm, "name")
		if name == "" {
			continue
		}
		// Upstream v1beta1 nests attributes under .basic.attributes; v1 GA
		// flattens them to .attributes. Try basic first, then the flat form so
		// the reader spans the ADR-0024 §4(a) migration.
		attrs, ok := nestedAttributes(dm)
		if !ok {
			continue
		}
		ov := sliceOverlay{}
		if v, has := attrIntValue(attrs, attrHCCSRing); has {
			ov.hccsRing = v
			ov.hasRing = true
		}
		if v, has := attrIntValue(attrs, attrNUMANode); has {
			ov.numaNode = v
			ov.hasNUMA = true
		}
		if ov.hasRing || ov.hasNUMA {
			out[name] = ov
		}
	}
}

// nestedAttributes returns the device attribute map, spanning the v1beta1
// (.basic.attributes) and v1 (.attributes) shapes.
func nestedAttributes(dm map[string]interface{}) (map[string]interface{}, bool) {
	if attrs, found, err := unstructured.NestedMap(dm, "basic", "attributes"); err == nil && found {
		return attrs, true
	}
	if attrs, found, err := unstructured.NestedMap(dm, "attributes"); err == nil && found {
		return attrs, true
	}
	return nil, false
}

// attrIntValue extracts the integer value of one device attribute. Upstream
// DeviceAttribute carries the int under the `int` JSON field (IntValue). Returns
// (0,false) when the key is absent or not an integer.
func attrIntValue(attrs map[string]interface{}, key string) (int64, bool) {
	raw, ok := attrs[key]
	if !ok {
		return 0, false
	}
	am, ok := raw.(map[string]interface{})
	if !ok {
		return 0, false
	}
	// unstructured stores JSON numbers as int64 (decoded) or float64 (from a
	// JSON document); handle both so fake-client fixtures and live decodes both
	// resolve.
	if v, found, err := unstructured.NestedInt64(am, "int"); err == nil && found {
		return v, true
	}
	if f, ok := am["int"].(float64); ok {
		return int64(f), true
	}
	return 0, false
}

// applyResourceSliceOverlay overlays real ResourceSlice topology onto a
// structurally-projected NPU: the parsed hccs ring becomes the hccsGroup the
// aggregator groups edges by (namespaced to the node so two nodes' ring-0 never
// collapse into one cross-node group), the parsed NUMA node replaces the static
// placement, and datasheet bandwidth nominals are stamped so the aggregator's
// hccs/pcie attributes are populated.
func applyResourceSliceOverlay(npu *model.NPU, ov sliceOverlay) {
	if npu == nil {
		return
	}
	if ov.hasRing {
		npu.HCCSGroup = npu.NodeName + "-hccs-" + strconv.FormatInt(ov.hccsRing, 10)
		bw := hccs910BBandwidthGBps
		npu.HCCSBandwidthGBps = &bw
	}
	if ov.hasNUMA {
		npu.NumaNode = int(ov.numaNode)
	}
}
