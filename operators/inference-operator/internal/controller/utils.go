/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package controller hosts the inference-operator controllers.
//
// Phase 5 T006 ships ModelServiceReconciler — the first body in this
// package. The shared helpers (SetCondition / RemoveCondition / etc.)
// mirror operators/npu-dra-driver/internal/controller/utils.go and
// operators/pool-operator/internal/controller/utils.go (verbatim text
// duplication per operators/CLAUDE.md §1 — no cross-module Go imports).
package controller

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SetCondition inserts or updates c in conds, matching by Type.
//
// If a condition with the same Type already exists:
//   - When the Status is unchanged, LastTransitionTime is preserved and
//     only Reason / Message / ObservedGeneration are updated.
//   - When the Status differs, LastTransitionTime is bumped to
//     metav1.Now().
//
// If no matching Type exists, c is appended (LastTransitionTime
// defaulted to metav1.Now() when unset).
func SetCondition(conds *[]metav1.Condition, c metav1.Condition) {
	if conds == nil {
		return
	}
	for i := range *conds {
		if (*conds)[i].Type != c.Type {
			continue
		}
		existing := (*conds)[i]
		if existing.Status == c.Status {
			c.LastTransitionTime = existing.LastTransitionTime
		} else {
			c.LastTransitionTime = metav1.Now()
		}
		(*conds)[i] = c
		return
	}
	if c.LastTransitionTime.IsZero() {
		c.LastTransitionTime = metav1.Now()
	}
	*conds = append(*conds, c)
}

// RemoveCondition deletes the first condition whose Type matches
// condType. Returns true when a deletion happened.
func RemoveCondition(conds *[]metav1.Condition, condType string) bool {
	if conds == nil {
		return false
	}
	for i := range *conds {
		if (*conds)[i].Type == condType {
			*conds = append((*conds)[:i], (*conds)[i+1:]...)
			return true
		}
	}
	return false
}

// FindCondition returns the first condition whose Type matches condType.
// Returns nil + false when no match.
func FindCondition(conds []metav1.Condition, condType string) (*metav1.Condition, bool) {
	for i := range conds {
		if conds[i].Type == condType {
			return &conds[i], true
		}
	}
	return nil, false
}

// RequeueAfter returns a ctrl.Result with RequeueAfter set to d,
// clamped to a minimum of one second to avoid hot-looping the
// workqueue.
func RequeueAfter(d time.Duration) ctrl.Result {
	if d < time.Second {
		d = time.Second
	}
	return ctrl.Result{RequeueAfter: d}
}
