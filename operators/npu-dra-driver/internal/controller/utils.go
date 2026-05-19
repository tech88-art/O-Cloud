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

// Package controller hosts the npu-dra-driver controllers.
//
// Phase 4 T006 ships:
//   - ClaimReconciler: a Reconcile that filters incoming ResourceClaims by
//     the driver class name prefix and records an AllocationDeferred
//     condition. Allocation logic is a Phase 5 deliverable per ADR-0001 v3
//     §7 + ADR-0009 (lands at P4-T-105).
//
// The shared helpers mirror operators/pool-operator/internal/controller/utils.go
// (verbatim Apache 2.0 boilerplate) but live in this module so the
// npu-dra-driver does not cross-import pool-operator's internal/ package.
// This keeps both projects independently buildable.
package controller

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SetCondition inserts or updates c in conds, matching by Type. Identical
// semantics to operators/pool-operator/internal/controller/utils.go SetCondition.
//
// If a condition with the same Type already exists:
//   - When the Status is unchanged, LastTransitionTime is preserved and only
//     Reason / Message / ObservedGeneration are updated.
//   - When the Status differs, LastTransitionTime is bumped to metav1.Now().
//
// If no matching Type exists, c is appended (LastTransitionTime defaulted to
// metav1.Now() when unset).
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

// RemoveCondition deletes the first condition whose Type matches condType.
// Returns true when a deletion happened.
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

// MakeOwnerRef builds a controller-style OwnerReference for obj. Controller
// and BlockOwnerDeletion are both set to true so child resources are cleaned
// up via Kubernetes garbage collection when the owner is deleted.
func MakeOwnerRef(obj metav1.Object, gvk schema.GroupVersionKind) metav1.OwnerReference {
	controller := true
	blockOwnerDeletion := true
	return metav1.OwnerReference{
		APIVersion:         gvk.GroupVersion().String(),
		Kind:               gvk.Kind,
		Name:               obj.GetName(),
		UID:                obj.GetUID(),
		Controller:         &controller,
		BlockOwnerDeletion: &blockOwnerDeletion,
	}
}

// RequeueAfter returns a ctrl.Result with RequeueAfter set to d, clamped to
// a minimum of one second to avoid hot-looping the workqueue.
func RequeueAfter(d time.Duration) ctrl.Result {
	if d < time.Second {
		d = time.Second
	}
	return ctrl.Result{RequeueAfter: d}
}
