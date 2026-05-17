// Package builder contains helpers for shaping realistic mock topology:
// HCCS group assignment (4 cards per group on a 8-card 910B node), NUMA
// layout, and time skew across `createdAt` fields.
//
// Constants below capture the Atlas 800T A2 reference: 8x 910B per node,
// 2 NUMA domains, each NUMA hosts 4 NPUs that form one HCCS full-mesh group.
// See configs/CLAUDE.md §3.3 ("HCCS 组：每 4/8 卡一组").
package builder

import (
	"fmt"
	"time"
)

const (
	// NPUsPerNode is the number of 910B cards on a single Atlas 800T A2.
	NPUsPerNode = 8
	// HCCSCardsPerGroup is the HCCS full-mesh group size on 910B (4 cards).
	HCCSCardsPerGroup = 4
	// NumaPerNode is the number of NUMA domains exposed by an Atlas 800T A2.
	NumaPerNode = 2
	// NPUVRAMMiB is the on-card HBM size for Ascend 910B (64 GiB).
	NPUVRAMMiB = 65536
	// NPUAICoreTotal is the total AI-core count on one 910B die.
	NPUAICoreTotal = 32
)

// NPULayout describes where one NPU sits inside a node: NUMA domain, HCCS
// group id, and physical index.
type NPULayout struct {
	NodeName  string
	Index     int
	NumaNode  int
	HCCSGroup string
}

// LayoutNode returns the eight-card layout for nodeName.
//
// NPU 0..3 land on NUMA 0 and form HCCS group "<nodeName>-hccs-0"
// NPU 4..7 land on NUMA 1 and form HCCS group "<nodeName>-hccs-1"
func LayoutNode(nodeName string) []NPULayout {
	out := make([]NPULayout, NPUsPerNode)
	for i := 0; i < NPUsPerNode; i++ {
		numa := i / HCCSCardsPerGroup
		out[i] = NPULayout{
			NodeName:  nodeName,
			Index:     i,
			NumaNode:  numa,
			HCCSGroup: fmt.Sprintf("%s-hccs-%d", nodeName, numa),
		}
	}
	return out
}

// NPUsForNUMA returns the NPU id strings that belong to the given NUMA domain.
//
// Mirrors LayoutNode: NUMA 0 = NPU 0..3, NUMA 1 = NPU 4..7.
func NPUsForNUMA(nodeName string, numa int) []string {
	out := make([]string, 0, HCCSCardsPerGroup)
	start := numa * HCCSCardsPerGroup
	end := start + HCCSCardsPerGroup
	for i := start; i < end; i++ {
		out = append(out, fmt.Sprintf("%s-npu-%d", nodeName, i))
	}
	return out
}

// HCCSGroupsForNode lists the HCCS group ids on a single node, plus the NPU
// ids inside each group.
func HCCSGroupsForNode(nodeName string) map[string][]string {
	groups := map[string][]string{}
	for _, l := range LayoutNode(nodeName) {
		id := fmt.Sprintf("%s-npu-%d", nodeName, l.Index)
		groups[l.HCCSGroup] = append(groups[l.HCCSGroup], id)
	}
	return groups
}

// SkewCreatedAt returns a deterministic `createdAt` timestamp anchored at
// t0 minus (daysAgo days + hoursAgo hours). Used to spread cluster / node /
// workload creation times over the last ~30 days.
func SkewCreatedAt(t0 time.Time, daysAgo, hoursAgo int) string {
	t := t0.Add(-time.Duration(daysAgo)*24*time.Hour - time.Duration(hoursAgo)*time.Hour)
	return t.UTC().Format(time.RFC3339)
}

// EventTimestamp returns `t0 + offsetSeconds` formatted as RFC3339.
func EventTimestamp(t0 time.Time, offsetSeconds int) string {
	return t0.Add(time.Duration(offsetSeconds) * time.Second).UTC().Format(time.RFC3339)
}
