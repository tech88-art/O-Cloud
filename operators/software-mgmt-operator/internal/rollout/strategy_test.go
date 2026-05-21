/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

package rollout

import (
	"testing"

	"github.com/tech88-art/O-Cloud/operators/software-mgmt-operator/api/v1alpha1"
)

func TestNextBatchRollingUpdate(t *testing.T) {
	all := []string{"a", "b", "c", "d"}
	plan, err := NextBatch(v1alpha1.RolloutStrategyRollingUpdate, 2, all, nil, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(plan.NodesToTarget) != 2 {
		t.Fatalf("NodesToTarget = %d, want 2 (MaxUnavailable=2 cap)", len(plan.NodesToTarget))
	}
}

func TestNextBatchRollingUpdateWithInProgress(t *testing.T) {
	all := []string{"a", "b", "c", "d"}
	// 1 already in-progress · cap = 2 - 1 = 1
	plan, _ := NextBatch(v1alpha1.RolloutStrategyRollingUpdate, 2, all, nil, []string{"a"})
	if len(plan.NodesToTarget) != 1 {
		t.Fatalf("NodesToTarget = %d, want 1 (cap exhausted by in-progress)", len(plan.NodesToTarget))
	}
}

func TestNextBatchRollingUpdateAtCap(t *testing.T) {
	all := []string{"a", "b"}
	// All MaxUnavailable slots used · no new targets this pass
	plan, _ := NextBatch(v1alpha1.RolloutStrategyRollingUpdate, 2, all, nil, []string{"a", "b"})
	if len(plan.NodesToTarget) != 0 {
		t.Fatalf("NodesToTarget = %d, want 0 (in-progress count == MaxUnavailable)", len(plan.NodesToTarget))
	}
}

func TestNextBatchParallel(t *testing.T) {
	all := []string{"a", "b", "c"}
	plan, _ := NextBatch(v1alpha1.RolloutStrategyParallel, 0, all, nil, nil)
	if len(plan.NodesToTarget) != 3 {
		t.Fatalf("Parallel NodesToTarget = %d, want 3 (all pending)", len(plan.NodesToTarget))
	}
}

func TestNextBatchSequential(t *testing.T) {
	all := []string{"a", "b", "c"}
	plan, _ := NextBatch(v1alpha1.RolloutStrategySequential, 0, all, nil, nil)
	if len(plan.NodesToTarget) != 1 || plan.NodesToTarget[0] != "a" {
		t.Fatalf("Sequential first batch = %v, want [a]", plan.NodesToTarget)
	}
}

func TestNextBatchSequentialBlocksOnInProgress(t *testing.T) {
	all := []string{"a", "b", "c"}
	plan, _ := NextBatch(v1alpha1.RolloutStrategySequential, 0, all, nil, []string{"a"})
	if len(plan.NodesToTarget) != 0 {
		t.Fatalf("Sequential with in-progress should yield no new targets, got %v", plan.NodesToTarget)
	}
}

func TestNextBatchAllApplied(t *testing.T) {
	all := []string{"a", "b"}
	plan, _ := NextBatch(v1alpha1.RolloutStrategyRollingUpdate, 2, all, all, nil)
	if len(plan.NodesToTarget) != 0 {
		t.Fatalf("All applied → NodesToTarget = %d, want 0", len(plan.NodesToTarget))
	}
}

func TestIsComplete(t *testing.T) {
	all := []string{"a", "b"}
	if !IsComplete(all, all, nil) {
		t.Fatal("IsComplete should be true when all applied + none in-progress")
	}
	if IsComplete(all, []string{"a"}, []string{"b"}) {
		t.Fatal("IsComplete should be false while one in-progress")
	}
}

func TestNextBatchUnknownStrategy(t *testing.T) {
	all := []string{"a"}
	_, err := NextBatch("BogusStrategy", 0, all, nil, nil)
	if err == nil {
		t.Fatal("expected error for unknown strategy")
	}
}
