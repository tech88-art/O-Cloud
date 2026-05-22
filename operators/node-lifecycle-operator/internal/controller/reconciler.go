/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

// reconciler.go is the controller-runtime ctrl.Reconciler shell that
// wraps the pure-Go ReconcileOnce state machine (P10-T-007 substrate)
// behind a real apiserver fetch / status-write loop (P11-T-004 chart
// wire). Keeping the state machine in nodelifecycle_controller.go pure
// preserves the original unit tests; this wrapper handles I/O only.
package controller

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/node-lifecycle-operator/api/v1alpha1"
)

// NodeLifecycleReconciler is the ctrl.Reconciler. Owns NodeLifecycle CR;
// watches Node objects for condition propagation.
type NodeLifecycleReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// SetupWithManager wires the controller into the manager. Watches
// NodeLifecycle (primary) + Node (secondary · maps Node name → NodeLifecycle).
func (r *NodeLifecycleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.NodeLifecycle{}).
		Owns(&corev1.Node{}).
		Complete(r)
}

// Reconcile is invoked on each NodeLifecycle event. Fetches the CR + its
// linked Node, runs ReconcileOnce, writes Status back.
func (r *NodeLifecycleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("nodelifecycle", req.NamespacedName)

	var nl v1alpha1.NodeLifecycle
	if err := r.Get(ctx, req.NamespacedName, &nl); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "failed to fetch NodeLifecycle")
		return ctrl.Result{}, err
	}

	// Observe linked Node — absent / not found surfaces as NodeReady=false.
	in := ReconcileInput{
		Current: &nl,
		Now:     time.Now(),
	}
	if nl.Spec.NodeName != "" {
		var node corev1.Node
		if err := r.Get(ctx, types.NamespacedName{Name: nl.Spec.NodeName}, &node); err == nil {
			for _, cond := range node.Status.Conditions {
				switch cond.Type {
				case corev1.NodeReady:
					in.NodeReady = cond.Status == corev1.ConditionTrue
				case corev1.NodeDiskPressure:
					in.NodeDiskPressure = cond.Status == corev1.ConditionTrue
				case corev1.NodeNetworkUnavailable:
					in.NodeNetworkUnavailable = cond.Status == corev1.ConditionTrue
				}
			}
		} else if !apierrors.IsNotFound(err) {
			logger.Error(err, "failed to fetch linked Node")
		}
	}

	out := ReconcileOnce(in)

	// Status patch when state or conditions differ.
	if out.NextState != nl.Status.State || !conditionsEqual(out.Conditions, nl.Status.Conditions) {
		nl.Status.State = out.NextState
		nl.Status.Conditions = out.Conditions
		if err := r.Status().Update(ctx, &nl); err != nil {
			logger.Error(err, "failed to update NodeLifecycle status")
			return ctrl.Result{}, err
		}
		if r.Recorder != nil {
			r.Recorder.Eventf(&nl, corev1.EventTypeNormal, "StateTransition",
				"NodeLifecycle %s/%s → %s", nl.Namespace, nl.Name, out.NextState)
		}
	}

	return ctrl.Result{RequeueAfter: out.RequeueAfter}, nil
}

// conditionsEqual is a cheap equality check on length + per-type
// status/reason — enough to skip a no-op Status update.
func conditionsEqual(a, b []metav1.Condition) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Type != b[i].Type || a[i].Status != b[i].Status || a[i].Reason != b[i].Reason {
			return false
		}
	}
	return true
}
