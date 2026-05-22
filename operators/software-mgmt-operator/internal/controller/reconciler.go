/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

// reconciler.go is the controller-runtime ctrl.Reconciler shell wrapping
// the pure-Go ReconcileOnce + rollout.NextBatch from P10-T-008 substrate
// (P11-T-005 chart wire · ADR-0017 §2 Decision D 3rd).
package controller

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/software-mgmt-operator/api/v1alpha1"
)

// SoftwareBundleReconciler is the ctrl.Reconciler for SoftwareBundle.
type SoftwareBundleReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// SetupWithManager wires the controller into the manager.
func (r *SoftwareBundleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.SoftwareBundle{}).
		Complete(r)
}

// Reconcile fetches the SoftwareBundle, resolves target nodes via
// label-selector, runs ReconcileOnce, and writes the resulting Status
// back to apiserver. Phase 11 minimum-viable: applied/inProgress/failed
// node slices are stored in annotations (`*-nodes` keys) since the
// SoftwareBundleStatus Phase 10 schema only stores counts.
func (r *SoftwareBundleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("softwarebundle", req.NamespacedName)

	var sb v1alpha1.SoftwareBundle
	if err := r.Get(ctx, req.NamespacedName, &sb); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "fetch SoftwareBundle")
		return ctrl.Result{}, err
	}

	// Resolve target nodes — Phase 11 minimum-viable: list all nodes,
	// filter by Spec.NodeSelector if non-empty. Phase 12+ polish moves
	// to client.MatchingLabels for efficiency at scale.
	var nodeList corev1.NodeList
	if err := r.List(ctx, &nodeList); err != nil {
		logger.Error(err, "list Nodes")
		return ctrl.Result{}, err
	}
	allTargets := make([]string, 0, len(nodeList.Items))
	for _, n := range nodeList.Items {
		if matchesSelector(n.Labels, sb.Spec.NodeSelector) {
			allTargets = append(allTargets, n.Name)
		}
	}

	// applied/inProgress/failed slices: Phase 11 minimum-viable derives
	// from spec.observed-state annotation map. Operators stamp rollout
	// progress via annotations on the SoftwareBundle. Phase 12+ moves
	// to per-node CR (NodeSoftwareBundleStatus) for richer observation.
	applied := nodeListFromAnnotation(sb.Annotations, "softwaremgmt.ocloud.edge.example.com/applied-nodes")
	inProgress := nodeListFromAnnotation(sb.Annotations, "softwaremgmt.ocloud.edge.example.com/in-progress-nodes")
	failed := nodeListFromAnnotation(sb.Annotations, "softwaremgmt.ocloud.edge.example.com/failed-nodes")

	in := ReconcileInput{
		Current:         &sb,
		AllTargetNodes:  allTargets,
		AppliedNodes:    applied,
		InProgressNodes: inProgress,
		FailedNodes:     failed,
		Now:             time.Now(),
	}

	out := ReconcileOnce(in)

	if !statusEqual(out.NextStatus, sb.Status) {
		sb.Status = out.NextStatus
		if err := r.Status().Update(ctx, &sb); err != nil {
			logger.Error(err, "update Status")
			return ctrl.Result{}, err
		}
		if r.Recorder != nil {
			r.Recorder.Eventf(&sb, corev1.EventTypeNormal, "RolloutProgress",
				"SoftwareBundle %s/%s targets=%d applied=%d failed=%d",
				sb.Namespace, sb.Name,
				out.NextStatus.TargetNodeCount,
				out.NextStatus.AppliedNodeCount,
				out.NextStatus.FailedNodeCount)
		}
	}

	return ctrl.Result{RequeueAfter: out.RequeueAfter}, nil
}

// matchesSelector is a minimal label-selector check.
func matchesSelector(labels, selector map[string]string) bool {
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}

// nodeListFromAnnotation parses a comma-separated annotation value into
// a node-name slice. Empty / missing annotation → empty slice.
func nodeListFromAnnotation(annotations map[string]string, key string) []string {
	v, ok := annotations[key]
	if !ok || v == "" {
		return nil
	}
	out := make([]string, 0, 4)
	start := 0
	for i := 0; i < len(v); i++ {
		if v[i] == ',' {
			if i > start {
				out = append(out, v[start:i])
			}
			start = i + 1
		}
	}
	if start < len(v) {
		out = append(out, v[start:])
	}
	return out
}

// statusEqual is a cheap equality check on Status counts + condition
// length — enough to skip no-op Status updates.
func statusEqual(a, b v1alpha1.SoftwareBundleStatus) bool {
	return a.AppliedVersion == b.AppliedVersion &&
		a.TargetNodeCount == b.TargetNodeCount &&
		a.AppliedNodeCount == b.AppliedNodeCount &&
		a.FailedNodeCount == b.FailedNodeCount &&
		len(a.Conditions) == len(b.Conditions)
}
