/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

// Package rollout implements the SoftwareBundle rollout strategy per
// ADR-0003 v2 IMS-2 software-mgmt-operator (Phase 10 P10-T-008 controller
// body).
//
// 3 strategies per CRD spec:
//   - RollingUpdate · node-by-node with MaxUnavailable concurrency cap
//   - Parallel · all selected nodes concurrently
//   - Sequential · strict one-at-a-time
package rollout

import (
	"fmt"

	"github.com/tech88-art/O-Cloud/operators/software-mgmt-operator/api/v1alpha1"
)

// Plan describes the next-batch rollout decision for a SoftwareBundle.
type Plan struct {
	// NodesToTarget is the set of node names that should be patched in
	// this reconcile pass per the strategy.
	NodesToTarget []string

	// RemainingNodes is the remaining unpatched set after this batch.
	RemainingNodes []string

	// Strategy is the rollout strategy used to derive this plan.
	Strategy v1alpha1.RolloutStrategy
}

// NextBatch decides which nodes to patch in this reconcile pass per
// strategy + current rollout state. Pure function: no I/O, no controller-
// runtime imports.
//
// Inputs:
//   - strategy: RolloutStrategy from Spec.RolloutPolicy.Strategy
//   - maxUnavailable: from Spec.RolloutPolicy.MaxUnavailable (RollingUpdate only)
//   - allNodes: full target node list (post-NodeSelector filter)
//   - alreadyApplied: nodes that completed rollout (Status.AppliedNodes)
//   - inProgress: nodes mid-rollout (not yet Applied)
func NextBatch(strategy v1alpha1.RolloutStrategy, maxUnavailable int32, allNodes, alreadyApplied, inProgress []string) (Plan, error) {
	pendingSet := make(map[string]struct{}, len(allNodes))
	for _, n := range allNodes {
		pendingSet[n] = struct{}{}
	}
	for _, n := range alreadyApplied {
		delete(pendingSet, n)
	}
	for _, n := range inProgress {
		delete(pendingSet, n)
	}

	pending := make([]string, 0, len(pendingSet))
	for _, n := range allNodes {
		if _, ok := pendingSet[n]; ok {
			pending = append(pending, n)
		}
	}

	if len(pending) == 0 {
		return Plan{NodesToTarget: nil, RemainingNodes: nil, Strategy: strategy}, nil
	}

	switch strategy {
	case v1alpha1.RolloutStrategyParallel:
		// Target all pending nodes concurrently.
		return Plan{
			NodesToTarget:  append([]string{}, pending...),
			RemainingNodes: nil,
			Strategy:       strategy,
		}, nil

	case v1alpha1.RolloutStrategySequential:
		// One node at a time · only target first pending IF no in-progress.
		if len(inProgress) > 0 {
			return Plan{NodesToTarget: nil, RemainingNodes: pending, Strategy: strategy}, nil
		}
		return Plan{
			NodesToTarget:  []string{pending[0]},
			RemainingNodes: pending[1:],
			Strategy:       strategy,
		}, nil

	case v1alpha1.RolloutStrategyRollingUpdate:
		// Concurrency cap = MaxUnavailable minus current in-progress count.
		cap := int(maxUnavailable) - len(inProgress)
		if cap <= 0 {
			return Plan{NodesToTarget: nil, RemainingNodes: pending, Strategy: strategy}, nil
		}
		if cap > len(pending) {
			cap = len(pending)
		}
		return Plan{
			NodesToTarget:  append([]string{}, pending[:cap]...),
			RemainingNodes: pending[cap:],
			Strategy:       strategy,
		}, nil
	}
	return Plan{}, fmt.Errorf("unknown RolloutStrategy %q", strategy)
}

// IsComplete returns true when allNodes have been applied (no pending,
// no in-progress).
func IsComplete(allNodes, alreadyApplied, inProgress []string) bool {
	return len(alreadyApplied) >= len(allNodes) && len(inProgress) == 0
}
