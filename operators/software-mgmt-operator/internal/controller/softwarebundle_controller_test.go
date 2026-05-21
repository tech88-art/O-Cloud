/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

package controller

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/tech88-art/O-Cloud/operators/software-mgmt-operator/api/v1alpha1"
)

func newInput(strategy v1alpha1.RolloutStrategy, maxUnav int32, all, applied, inProg, failed []string) ReconcileInput {
	return ReconcileInput{
		Current: &v1alpha1.SoftwareBundle{
			Spec: v1alpha1.SoftwareBundleSpec{
				Version: "v1.2.0",
				Patches: []v1alpha1.SoftwarePatch{{Name: "p1"}},
				RolloutPolicy: v1alpha1.RolloutPolicy{
					Strategy:       strategy,
					MaxUnavailable: maxUnav,
				},
			},
		},
		AllTargetNodes:  all,
		AppliedNodes:    applied,
		InProgressNodes: inProg,
		FailedNodes:     failed,
		Now:             time.Now(),
	}
}

func findCond(conds []metav1.Condition, condType string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == condType {
			return &conds[i]
		}
	}
	return nil
}

func TestReconcileRollingUpdateProducesPlan(t *testing.T) {
	in := newInput(v1alpha1.RolloutStrategyRollingUpdate, 2, []string{"a", "b", "c"}, nil, nil, nil)
	out := ReconcileOnce(in)
	if len(out.Plan.NodesToTarget) != 2 {
		t.Fatalf("plan NodesToTarget = %d, want 2", len(out.Plan.NodesToTarget))
	}
}

func TestReconcileProgressingCondition(t *testing.T) {
	in := newInput(v1alpha1.RolloutStrategyRollingUpdate, 2, []string{"a", "b"}, nil, []string{"a"}, nil)
	out := ReconcileOnce(in)
	c := findCond(out.NextStatus.Conditions, ConditionProgressing)
	if c == nil || c.Status != metav1.ConditionTrue {
		t.Fatal("Progressing should be True when in-progress nodes exist")
	}
}

func TestReconcileCompletedCondition(t *testing.T) {
	all := []string{"a", "b"}
	in := newInput(v1alpha1.RolloutStrategyRollingUpdate, 2, all, all, nil, nil)
	out := ReconcileOnce(in)
	c := findCond(out.NextStatus.Conditions, ConditionCompleted)
	if c == nil || c.Status != metav1.ConditionTrue {
		t.Fatal("Completed should be True when all applied + none in-progress")
	}
	if out.NextStatus.AppliedVersion != "v1.2.0" {
		t.Fatalf("AppliedVersion = %q, want v1.2.0", out.NextStatus.AppliedVersion)
	}
}

func TestReconcileFailedCondition(t *testing.T) {
	in := newInput(v1alpha1.RolloutStrategyRollingUpdate, 2, []string{"a", "b"}, nil, nil, []string{"a"})
	out := ReconcileOnce(in)
	c := findCond(out.NextStatus.Conditions, ConditionFailed)
	if c == nil || c.Status != metav1.ConditionTrue {
		t.Fatal("Failed should be True when failedNodes non-empty")
	}
}

func TestReconcileDegradedCondition(t *testing.T) {
	in := newInput(v1alpha1.RolloutStrategyRollingUpdate, 2, []string{"a", "b", "c"},
		[]string{"a"}, nil, []string{"b"})
	out := ReconcileOnce(in)
	c := findCond(out.NextStatus.Conditions, ConditionDegraded)
	if c == nil || c.Status != metav1.ConditionTrue {
		t.Fatal("Degraded should be True with mix of applied + failed")
	}
}

func TestReconcileStatusCounts(t *testing.T) {
	all := []string{"a", "b", "c", "d"}
	in := newInput(v1alpha1.RolloutStrategyRollingUpdate, 2, all, []string{"a"}, []string{"b"}, []string{"c"})
	out := ReconcileOnce(in)
	if out.NextStatus.TargetNodeCount != 4 {
		t.Fatalf("TargetNodeCount = %d, want 4", out.NextStatus.TargetNodeCount)
	}
	if out.NextStatus.AppliedNodeCount != 1 {
		t.Fatalf("AppliedNodeCount = %d, want 1", out.NextStatus.AppliedNodeCount)
	}
	if out.NextStatus.FailedNodeCount != 1 {
		t.Fatalf("FailedNodeCount = %d, want 1", out.NextStatus.FailedNodeCount)
	}
}

func TestReconcileRequeueCadence(t *testing.T) {
	all := []string{"a", "b"}
	steady := newInput(v1alpha1.RolloutStrategyRollingUpdate, 2, all, all, nil, nil)
	progressing := newInput(v1alpha1.RolloutStrategyRollingUpdate, 2, all, nil, []string{"a"}, nil)
	idle := newInput(v1alpha1.RolloutStrategyRollingUpdate, 0, all, nil, nil, nil) // cap=0 → no targets, no in-progress

	outSteady := ReconcileOnce(steady)
	outProg := ReconcileOnce(progressing)
	outIdle := ReconcileOnce(idle)

	if outSteady.RequeueAfter != 0 {
		t.Fatalf("completed RequeueAfter = %v, want 0 (no further reconcile)", outSteady.RequeueAfter)
	}
	if outProg.RequeueAfter != 5*time.Second {
		t.Fatalf("progressing RequeueAfter = %v, want 5s", outProg.RequeueAfter)
	}
	if outIdle.RequeueAfter != 30*time.Second {
		t.Fatalf("idle RequeueAfter = %v, want 30s", outIdle.RequeueAfter)
	}
}
