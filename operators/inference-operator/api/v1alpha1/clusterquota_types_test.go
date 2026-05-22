/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha1

import (
	"encoding/json"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClusterQuotaJSONRoundtrip(t *testing.T) {
	now := metav1.NewTime(time.Now().Truncate(time.Second))
	orig := &ClusterQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "production-budget"},
		Spec: ClusterQuotaSpec{
			Enforcement: QuotaEnforcement{
				MaxSliceAllocations: 64,
				MaxScaleEventsPerWindow: ScaleEventRateCap{
					Count:         20,
					WindowSeconds: 600,
				},
				MaxNPUSliceTemplateRefs: []string{"vir02-4card", "vir04-2card"},
			},
		},
		Status: ClusterQuotaStatus{
			Usage: ClusterQuotaUsage{
				Total: QuotaUsage{CurrentSliceAllocations: 18, ScaleEventsInWindow: 7},
				PerCluster: map[string]QuotaUsage{
					"member1": {CurrentSliceAllocations: 10, ScaleEventsInWindow: 3},
					"member2": {CurrentSliceAllocations: 8, ScaleEventsInWindow: 4},
				},
			},
			LastSyncTime: &now,
		},
	}

	buf, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	round := &ClusterQuota{}
	if err := json.Unmarshal(buf, round); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if round.Spec.Enforcement.MaxSliceAllocations != 64 {
		t.Errorf("MaxSliceAllocations = %d, want 64", round.Spec.Enforcement.MaxSliceAllocations)
	}
	if got := round.Status.Usage.Total.CurrentSliceAllocations; got != 18 {
		t.Errorf("Total.CurrentSliceAllocations = %d, want 18(sum of perCluster 10+8)", got)
	}
	if got := round.Status.Usage.PerCluster["member1"].CurrentSliceAllocations; got != 10 {
		t.Errorf("PerCluster[member1] = %d, want 10", got)
	}
	if got := len(round.Spec.Enforcement.MaxNPUSliceTemplateRefs); got != 2 {
		t.Errorf("MaxNPUSliceTemplateRefs len = %d, want 2", got)
	}
}

func TestClusterQuotaUsageAggregation(t *testing.T) {
	usage := ClusterQuotaUsage{
		PerCluster: map[string]QuotaUsage{
			"member1": {CurrentSliceAllocations: 5, ScaleEventsInWindow: 2},
			"member2": {CurrentSliceAllocations: 7, ScaleEventsInWindow: 3},
			"member3": {CurrentSliceAllocations: 3, ScaleEventsInWindow: 0},
		},
	}
	usage.RecomputeTotal()
	if got := usage.Total.CurrentSliceAllocations; got != 15 {
		t.Errorf("Total.CurrentSliceAllocations = %d, want 15(5+7+3)", got)
	}
	if got := usage.Total.ScaleEventsInWindow; got != 5 {
		t.Errorf("Total.ScaleEventsInWindow = %d, want 5(2+3+0)", got)
	}
}

func TestClusterQuotaUsageAggregationEmpty(t *testing.T) {
	usage := ClusterQuotaUsage{}
	usage.RecomputeTotal()
	if usage.Total.CurrentSliceAllocations != 0 || usage.Total.ScaleEventsInWindow != 0 {
		t.Errorf("empty usage Total = %+v, want zero", usage.Total)
	}
}
