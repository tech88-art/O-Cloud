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

package controller

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	imsv1alpha1 "github.com/example/ocloud-edge/operators/pool-operator/api/v1alpha1"
)

const (
	// nodeRoleLabel identifies the role label written on Nodes by the
	// O-Cloud bootstrap process (edge / core). NodePool.Spec.Role filters
	// by this label. Reserved as a NodePool-local constant: the other pool
	// controllers do not need it today.
	nodeRoleLabel = "node-role.ocloud.edge.example.com"

	// nodePoolReadyConditionType is the canonical Ready condition type for
	// NodePool status, matching the convention used by the other pool
	// reconcilers (literal "Ready"). Declared here so refactors stay local.
	nodePoolReadyConditionType = "Ready"
)

// NodePoolReconciler reconciles NodePool resources.
//
// Reconcile contract (P3-T-004; arch filter removed in P13-fix-005 per ADR-0020):
//   - Reads spec.selector + spec.role -> List matching Nodes.
//   - Aggregates matched node names + sums CPU / Memory capacity into
//     status.nodes / totalCPU / totalMemory. Aggregation is arch-agnostic:
//     both arm64 (Kunpeng 920 — the real deployment target per ADR-0020) and
//     amd64 (retained for dev/CI/render-verify) nodes are counted. The earlier
//     amd64-only filter encoded the superseded ADR-0001 §13 assumption and would
//     have emptied status on a real all-arm64 cluster; NPUPool/NPUSlicePool never
//     filtered by arch, so this brings NodePool in line with them.
//   - Emits Ready=True (Reason=Reconciled) when at least one node matches,
//     or Ready=False with one of MissingSelector / InvalidSelector /
//     NoNodeMatched otherwise.
type NodePoolReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=ims.ocloud.edge.example.com,resources=nodepools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ims.ocloud.edge.example.com,resources=nodepools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ims.ocloud.edge.example.com,resources=nodepools/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

// Reconcile materialises NodePool status from the cluster Node list.
// See type doc on NodePoolReconciler for the contract.
func (r *NodePoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var pool imsv1alpha1.NodePool
	if err := r.Get(ctx, req.NamespacedName, &pool); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 1. Selector is required for T004 — both a nil pointer AND an empty
	//    LabelSelector{} (no matchLabels, no matchExpressions) are reported
	//    as Ready=False rather than treated as "match everything". This
	//    mirrors the +kubebuilder:validation:Required marker on
	//    NodePoolSpec.Selector and keeps the demo dataset deterministic —
	//    a NodePool that "matches every Node in the cluster" is almost
	//    always a misconfiguration, and the CRD layer already rejects nil
	//    pointers, so the empty-struct case is the one users can still hit.
	if pool.Spec.Selector == nil ||
		(len(pool.Spec.Selector.MatchLabels) == 0 &&
			len(pool.Spec.Selector.MatchExpressions) == 0) {
		return r.markNotReady(ctx, &pool, "MissingSelector", "spec.selector is required")
	}
	sel, err := metav1.LabelSelectorAsSelector(pool.Spec.Selector)
	if err != nil {
		return r.markNotReady(ctx, &pool, "InvalidSelector", err.Error())
	}

	var nodes corev1.NodeList
	if err := r.List(ctx, &nodes, &client.ListOptions{LabelSelector: sel}); err != nil {
		return ctrl.Result{}, err
	}

	// 2. Filter by Role (spec.role = "edge" | "core") if set. No architecture
	//    filter is applied: arm64 Kunpeng 920 is the real deployment target
	//    (ADR-0020, superseding the ADR-0001 §13 amd64-only assumption) and
	//    amd64 dev/CI nodes stay valid, so the CPU/Mem rollup is arch-agnostic
	//    — consistent with NPUPool/NPUSlicePool, which aggregate by capacity
	//    regardless of node arch.
	var matched []corev1.Node
	for _, n := range nodes.Items {
		if string(pool.Spec.Role) != "" {
			if n.Labels[nodeRoleLabel] != string(pool.Spec.Role) {
				continue
			}
		}
		matched = append(matched, n)
	}

	// 3. Aggregate node names + total CPU + total Memory across matched nodes.
	//    Sort names for deterministic status output (envtest list order is
	//    not guaranteed across reconcile passes).
	nodeNames := make([]string, 0, len(matched))
	totalCPU := resource.NewQuantity(0, resource.DecimalSI)
	totalMem := resource.NewQuantity(0, resource.BinarySI)
	for _, n := range matched {
		nodeNames = append(nodeNames, n.Name)
		if q, ok := n.Status.Capacity[corev1.ResourceCPU]; ok {
			totalCPU.Add(q)
		}
		if q, ok := n.Status.Capacity[corev1.ResourceMemory]; ok {
			totalMem.Add(q)
		}
	}
	sort.Strings(nodeNames)

	// 4. Write status (capacity always reflects current match set; empty
	//    when no node matched).
	pool.Status.Nodes = nodeNames
	pool.Status.TotalCPU = *totalCPU
	pool.Status.TotalMemory = *totalMem

	if len(matched) == 0 {
		SetCondition(&pool.Status.Conditions, metav1.Condition{
			Type:               nodePoolReadyConditionType,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: pool.Generation,
			Reason:             "NoNodeMatched",
			Message:            "Selector + Role filter matched no Nodes",
		})
	} else {
		SetCondition(&pool.Status.Conditions, metav1.Condition{
			Type:               nodePoolReadyConditionType,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: pool.Generation,
			Reason:             "Reconciled",
			Message: fmt.Sprintf("%d nodes matched, total CPU=%s, total Mem=%s",
				len(matched), totalCPU.String(), totalMem.String()),
		})
	}
	if err := r.Status().Update(ctx, &pool); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// markNotReady stamps Ready=False with the given reason/message and returns
// (Result{}, nil) so the workqueue does not retry — the user must edit the
// spec to clear the condition, and the watch will re-enqueue on that edit.
func (r *NodePoolReconciler) markNotReady(ctx context.Context, pool *imsv1alpha1.NodePool, reason, message string) (ctrl.Result, error) {
	// Status capacity is reset alongside Ready=False so a stale total from
	// a prior successful reconcile cannot mislead consumers.
	pool.Status.Nodes = nil
	pool.Status.TotalCPU = *resource.NewQuantity(0, resource.DecimalSI)
	pool.Status.TotalMemory = *resource.NewQuantity(0, resource.BinarySI)
	SetCondition(&pool.Status.Conditions, metav1.Condition{
		Type:               nodePoolReadyConditionType,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: pool.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := r.Status().Update(ctx, pool); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager wires the reconciler into mgr, watching the NodePool
// primary resource only. Node label changes flow through the periodic resync;
// explicit Node Watches are deferred to a later task — the Phase 3 demo
// dataset (4 nodes) is small enough that resync latency is acceptable.
func (r *NodePoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&imsv1alpha1.NodePool{}).
		Named("nodepool").
		Complete(r)
}
