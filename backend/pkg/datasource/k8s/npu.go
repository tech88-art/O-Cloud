package k8s

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// Ascend Device Plugin resource-name convention.
//
// The plugin (`gitee.com/ascend/mindxdl`) advertises whole-card capacity
// under `huawei.com/Ascend<model>` keys — `Ascend910` / `Ascend910B`
// for the 910 family, `Ascend310` for inference parts. Fixed-template
// slicing surfaces additional `Ascend910B-2c` / `-4c` / `-8c` /
// `-16c` keys, but those are out of scope for P2-T-002 (P2-T-101 will
// extend the model to slice pools via CRDs). See
// `docs/research/ascend-device-plugin.md` §"Whole-card allocation".
//
// We accept any `huawei.com/Ascend<NNN>` capacity key whose value is a
// non-zero integer and derive the model from the suffix.
const ascendResourcePrefix = "huawei.com/Ascend"

// Topology defaults for 8-card 910B nodes (Atlas 800T A2).
//
//   - NPU 0..3 → NUMA 0, HCCS group "<node>-hccs-0"
//   - NPU 4..7 → NUMA 1, HCCS group "<node>-hccs-1"
//
// These mirror the mock generator's `builder.LayoutNode` so the demo
// renders consistently whether the backend reads from the k8s.Source
// or the mock fixture. ADP itself does NOT advertise per-NPU NUMA /
// HCCS via labels in the upstream open release — Volcano scheduler
// annotations carry the topology when fine-grained placement is
// needed (Phase 4 territory). For Phase 2 the static layout is the
// realistic placeholder; a node-level annotation override path is
// supported below for operators that wire Volcano early.
const (
	npusPerNUMA = 4
	// nodeAnnotationLayout, when present on a node's annotations, is a
	// comma-separated list of `<npu-index>:numa=<n>,hccs=<group>` tuples
	// that override the static layout. Example:
	//   huawei.com/npu-layout = "0:numa=0,hccs=a-01-hccs-0;1:numa=0,hccs=a-01-hccs-0;..."
	// Empty annotation → fall through to the static convention above.
	nodeAnnotationLayout = "huawei.com/npu-layout"
	// nodeLabelModel lets operators force a model string when the
	// resource key doesn't disambiguate (e.g. air-gapped clusters that
	// rewrite resource names). Phase 2 doesn't currently use this; the
	// hook stays as a future-proofing toggle.
	nodeLabelModel = "npu.huawei.com/model"
)

// modelVRAMMiB / modelAICoreTotal capture per-model defaults so the
// demo Workloads / Metrics pages can render the NPU detail panel with
// non-zero numbers even when the apiserver doesn't carry per-card
// usage (apiserver doesn't — that's the job of the Prometheus source).
//
// Values match the Ascend public datasheets:
//   Ascend910 / 910B : 64 GiB VRAM, 32 AI cores
//   Ascend310        : 8  GiB VRAM, 4  AI cores  (Phase 2+ inference parts)
var modelDefaults = map[string]struct {
	VRAMMiB     int
	AICoreTotal int
}{
	"Ascend910":  {VRAMMiB: 65536, AICoreTotal: 32},
	"Ascend910B": {VRAMMiB: 65536, AICoreTotal: 32},
	"Ascend310":  {VRAMMiB: 8192, AICoreTotal: 4},
}

// ListNPUs reads the named node's NPUs by walking its
// `status.capacity` for any `huawei.com/Ascend*` resource. For each
// declared NPU (i = 0 .. count-1) we synthesize a model.NPU entry
// with:
//
//   - id        : "<nodeName>-npu-<i>"
//   - model     : the model suffix (Ascend910B etc.)
//   - vram      / aiCore: per-model defaults
//   - numaNode  / hccsGroup: static 4-per-numa layout (or the layout
//                            annotation override)
//   - status    : "healthy" (no per-NPU health surface from the open
//                            ADP labels alone; the Phase 2.5
//                            ascend-npu-exporter feeds usage + health
//                            via metrics rather than node labels)
//   - sliceMode : "whole"  (Phase 2 scope; fixed-template slices land
//                           with P2-T-102 once we have NPUSlicePool
//                           CRDs)
//
// Phase 2 NPUs do NOT carry Slices / Usage — those come from the
// Prometheus source (P2-T-007) and the slice CRD reader (P2-T-102).
//
// Behaviour:
//   - nodeName empty               → ErrResourceNotFound
//   - nodeName not in apiserver    → ErrResourceNotFound (Source 404)
//   - apiserver error              → wrapped via errFromAPIServer
//   - node present but 0 NPUs      → empty slice (NOT error)
func (s *Source) ListNPUs(ctx context.Context, nodeName string) ([]*model.NPU, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if nodeName == "" {
		return nil, ErrResourceNotFound
	}
	node, err := s.client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrResourceNotFound
		}
		return nil, errFromAPIServer("get node for NPUs", err)
	}
	return projectNPUs(node), nil
}

// projectNPUs is the pure list builder. Separate from ListNPUs so
// future per-cluster aggregates (e.g. Cluster.npuCount) can reuse it
// from cluster.go without re-fetching the node.
func projectNPUs(node *corev1.Node) []*model.NPU {
	count, modelName := readAscendCapacity(node)
	if count == 0 {
		return []*model.NPU{}
	}

	// Optional operator-supplied model override.
	if v, ok := node.Labels[nodeLabelModel]; ok && v != "" {
		modelName = v
	}

	defaults := modelDefaults[modelName]
	layout := readLayoutAnnotation(node.Annotations[nodeAnnotationLayout])

	out := make([]*model.NPU, 0, count)
	for i := 0; i < count; i++ {
		numa, hccs := resolveTopology(node.Name, i, layout)
		out = append(out, &model.NPU{
			ID:          npuID(node.Name, i),
			NodeName:    node.Name,
			Model:       modelName,
			Index:       i,
			VRAMMiB:     defaults.VRAMMiB,
			AICoreTotal: defaults.AICoreTotal,
			NumaNode:    numa,
			HCCSGroup:   hccs,
			Status:      "healthy",
			SliceMode:   "whole",
		})
	}
	return out
}

// readAscendCapacity returns the total NPU count across every
// `huawei.com/Ascend<...>` resource on the node, plus the model suffix
// from the highest-count resource. Mixed-model nodes (e.g. half 910 +
// half 310) are rare in practice; we surface the dominant suffix and
// fold all NPUs under it — P2-T-101 will refine per-model split when
// the schema gets a discriminator.
func readAscendCapacity(node *corev1.Node) (int, string) {
	if node == nil || node.Status.Capacity == nil {
		return 0, ""
	}
	totalByModel := map[string]int{}
	for k, q := range node.Status.Capacity {
		key := string(k)
		if !strings.HasPrefix(key, ascendResourcePrefix) {
			continue
		}
		// Skip slice variants like Ascend910B-2c / -4c — those are
		// fixed-template virtual cards, counted by P2-T-102.
		modelSuffix := strings.TrimPrefix(key, "huawei.com/")
		if strings.ContainsRune(modelSuffix, '-') {
			continue
		}
		n, ok := q.AsInt64()
		if !ok || n <= 0 {
			continue
		}
		totalByModel[modelSuffix] += int(n)
	}
	if len(totalByModel) == 0 {
		return 0, ""
	}
	// Pick the model with the largest count; tie-broken by lexicographic
	// order so the choice is deterministic across reconcile loops.
	bestModel := ""
	bestCount := 0
	total := 0
	for m, c := range totalByModel {
		total += c
		if c > bestCount || (c == bestCount && m < bestModel) {
			bestModel = m
			bestCount = c
		}
	}
	return total, bestModel
}

// resolveTopology returns (numa, hccsGroup) for an NPU at `index` on
// `nodeName`. Annotation override (if parsed) wins; otherwise apply
// the static 4-per-NUMA convention.
func resolveTopology(nodeName string, index int, override map[int]layoutEntry) (int, string) {
	if e, ok := override[index]; ok {
		return e.numa, e.hccsGroup
	}
	numa := index / npusPerNUMA
	hccs := nodeName + "-hccs-" + intToASCIIDigit(numa)
	return numa, hccs
}

// layoutEntry / readLayoutAnnotation parse the optional operator
// override.
type layoutEntry struct {
	numa      int
	hccsGroup string
}

// readLayoutAnnotation parses
// "0:numa=0,hccs=hccs-a;1:numa=0,hccs=hccs-a;..." into a map by NPU
// index. Malformed entries are silently dropped so the static
// fallback can still kick in for the un-overridden indices.
func readLayoutAnnotation(raw string) map[int]layoutEntry {
	if raw == "" {
		return nil
	}
	out := map[int]layoutEntry{}
	for _, chunk := range strings.Split(raw, ";") {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		colon := strings.IndexByte(chunk, ':')
		if colon <= 0 {
			continue
		}
		idx, err := atoiASCII(chunk[:colon])
		if err != nil {
			continue
		}
		entry := layoutEntry{numa: -1}
		for _, kv := range strings.Split(chunk[colon+1:], ",") {
			kv = strings.TrimSpace(kv)
			eq := strings.IndexByte(kv, '=')
			if eq <= 0 {
				continue
			}
			key := kv[:eq]
			val := kv[eq+1:]
			switch key {
			case "numa":
				if n, err := atoiASCII(val); err == nil {
					entry.numa = n
				}
			case "hccs":
				entry.hccsGroup = val
			}
		}
		if entry.numa < 0 || entry.hccsGroup == "" {
			continue
		}
		out[idx] = entry
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// npuID composes the canonical NPU id used throughout the project
// (matches the mock generator and the topology aggregator).
func npuID(nodeName string, index int) string {
	return nodeName + "-npu-" + intToASCIIDigit(index)
}

// intToASCIIDigit is a 1-2 digit fast formatter for ids. Avoids
// pulling fmt for hot path; 0..99 cover the demo space (8 NPUs/node).
func intToASCIIDigit(n int) string {
	if n < 0 {
		return "0"
	}
	if n < 10 {
		return string('0' + byte(n))
	}
	return string('0'+byte(n/10)) + string('0'+byte(n%10))
}

// atoiASCII parses a non-negative integer from an ASCII byte string.
// Used by the layout-annotation parser; avoids strconv import to keep
// the package import surface tight.
func atoiASCII(s string) (int, error) {
	if s == "" {
		return 0, errInvalidInt
	}
	out := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, errInvalidInt
		}
		out = out*10 + int(c-'0')
	}
	return out, nil
}

var errInvalidInt = newStaticErr("not a non-negative integer")
