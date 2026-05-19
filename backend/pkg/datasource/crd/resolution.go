package crd

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// ResolveSlicesForNPUs projects the loaded NPUSlicePool definitions
// onto a flat []model.NPUSlice list anchored to the supplied NPUs
// (P2-T-102). This is the alternative the topology aggregator
// reaches for when fed by the crd.Source instead of the mock
// fixture's hand-written slices.json.
//
// Projection rules:
//
//  1. Pull all NPUSlicePools via ListNPUSlicePools (uses the same
//     namespace scoping the source was constructed with).
//
//  2. For each NPU, pick a pool whose NPUPoolRef matches one of the
//     NPU's lookup keys (model name or HCCS group label). When
//     multiple pools match, the lex-smallest pool name wins (stable
//     across reconciles).
//
//  3. Generate slice ids of the form `<npu-id>-slice-<index>`:
//       - FixedTemplate pool → one slice per template entry; the
//         slice's `Template` field carries the template name (vir01
//         / vir02 / vir04 / ...).
//       - Dynamic pool → 2 slices per NPU as a placeholder count
//         (the operator hasn't materialised concrete shapes yet —
//         Phase 3 NPUSlice CRD will surface actual instances).
//       - Whole-NPU NPUs (sliceMode == "whole") get NO slices —
//         they consume the whole card, no per-slice subdivision.
//
//  4. All generated slices are marked status=available; the
//     allocation overlay (which slice is bound to which pod) lands
//     from the k8s.Source's pod bindings, joined by the aggregator.
//
// Pure once the apiserver fetch completes — same input yields the
// same output. Aggregator wiring (passing this through to the
// topology builder) lands with Phase 3.
func (s *Source) ResolveSlicesForNPUs(ctx context.Context, npus []*model.NPU) ([]model.NPUSlice, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(npus) == 0 {
		return []model.NPUSlice{}, nil
	}

	pools, err := s.ListNPUSlicePools(ctx)
	if err != nil {
		return nil, err
	}
	if len(pools) == 0 {
		return []model.NPUSlice{}, nil
	}

	// Stable per-NPU pool selection: pre-sort by name so the lex
	// tie-breaker has the right order.
	sort.SliceStable(pools, func(i, j int) bool {
		return pools[i].Name < pools[j].Name
	})

	out := []model.NPUSlice{}
	for _, npu := range npus {
		if npu == nil {
			continue
		}
		// Whole-NPU mode → no slices.
		if npu.SliceMode == "whole" {
			continue
		}
		pool := pickPoolForNPU(npu, pools)
		if pool == nil {
			continue
		}
		out = append(out, generateSlicesForNPU(npu, pool)...)
	}
	// Stable order for byte-identical responses across calls.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// pickPoolForNPU selects the first slice pool whose NPUPoolRef
// matches a plausible identifier on the NPU. Phase 2 matching
// heuristics (no real NPUPool CRD bridge yet):
//
//   - NPU.HCCSGroup ends with the pool ref string
//   - NPU.Model equals the pool ref string
//   - npu's node-name prefix equals the pool ref
//
// When nothing matches, fall back to the first (lex-smallest) pool
// so a single-tenant cluster with one slice pool Just Works
// regardless of operator label hygiene.
//
// Phase 3 pool-operator → NPUPool CRD resolution will replace this
// heuristic with concrete NPU-set membership lookups.
func pickPoolForNPU(npu *model.NPU, pools []*model.NPUSlicePool) *model.NPUSlicePool {
	if len(pools) == 0 {
		return nil
	}
	for _, p := range pools {
		ref := p.NPUPoolRef
		if ref == "" {
			continue
		}
		if strings.HasSuffix(npu.HCCSGroup, ref) {
			return p
		}
		if npu.Model == ref {
			return p
		}
		if strings.HasPrefix(npu.NodeName, ref) {
			return p
		}
	}
	// Fallback: lex-smallest pool (pools slice is pre-sorted).
	return pools[0]
}

// generateSlicesForNPU emits the per-NPU slice list per the pool's
// strategy. See ResolveSlicesForNPUs's projection rules for the
// shape contract.
func generateSlicesForNPU(npu *model.NPU, pool *model.NPUSlicePool) []model.NPUSlice {
	switch pool.Strategy {
	case "FixedTemplate":
		out := make([]model.NPUSlice, 0, len(pool.FixedTemplates))
		for i, tpl := range pool.FixedTemplates {
			out = append(out, model.NPUSlice{
				ID:        npuSliceID(npu.ID, i),
				ParentNPU: npu.ID,
				Template:  tpl.Name,
				AICore:    tpl.AICoreCount,
				VRAMMiB:   tpl.MemoryMiB,
				Status:    "available",
			})
		}
		return out
	case "Dynamic":
		// Phase 2 placeholder: 2 dynamic slots per NPU. Concrete
		// slice instances come from the Phase 3 NPUSlice CRD that
		// the operator populates per real allocation.
		out := make([]model.NPUSlice, 0, 2)
		for i := 0; i < 2; i++ {
			out = append(out, model.NPUSlice{
				ID:        npuSliceID(npu.ID, i),
				ParentNPU: npu.ID,
				Template:  "dynamic",
				Status:    "available",
			})
		}
		return out
	default:
		// Unknown strategy (forward-compat): emit nothing rather
		// than guess. Operators that ship a new strategy can add
		// a case here.
		return nil
	}
}

// npuSliceID composes a slice id of the form `<npu>-slice-<i>`.
// Matches the convention the mock fixtures + the topology
// aggregator both consume.
func npuSliceID(npuID string, index int) string {
	return npuID + "-slice-" + strconv.Itoa(index)
}
