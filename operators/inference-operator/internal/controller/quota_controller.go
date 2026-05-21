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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	inferencev1alpha1 "github.com/tech88-art/O-Cloud/operators/inference-operator/api/v1alpha1"
)

// QuotaReconcilePeriod is the requeue cadence per ADR-0014 §2 Decision D.
const QuotaReconcilePeriod = 60 * time.Second

// NPUSliceAllocationGVK is the cross-module GroupVersionKind for the
// NPUSliceAllocation CRD published by npu-dra-driver. Per operators/CLAUDE.md
// §1 we cannot import the npu-dra-driver Go module — we list these objects
// via `unstructured.Unstructured` with this GVK.
var NPUSliceAllocationGVK = schema.GroupVersionKind{
	Group:   "npu.ocloud.edge.example.com",
	Version: "v1alpha1",
	Kind:    "NPUSliceAllocation",
}

// QuotaReconciler reconciles Quota.status.usage against observed
// NPUSliceAllocation count + NPUVerticalScaler.status.scaleHistory sum
// per ADR-0014 §2 Decision D + §4 sync model.
//
// Reconcile loop on 60s tick:
//  1. Get Quota → if !found, no-op
//  2. List NPUSliceAllocation in target namespace → count
//  3. List NPUVerticalScaler in target namespace → sum scaleHistory entries
//     within `spec.enforcement.maxScaleEventsPerWindow.windowSeconds`
//  4. Update Quota.status.usage atomically + LastSyncTime
//  5. Update Quota.status.conditions[Active] = True
//  6. Requeue 60s
//
// Admission webhooks (NPUSliceAllocation create / NPUVerticalScaler update)
// read Quota.status.usage via in-memory cache (5s TTL) with API Get fallback —
// see quota_admission.go. Controller only writes status.usage on the tick;
// webhook never mutates status.
type QuotaReconciler struct {
	Client   client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// now is injected for testability (override in unit tests).
	NowFn func() time.Time
}

func (r *QuotaReconciler) now() time.Time {
	if r.NowFn != nil {
		return r.NowFn()
	}
	return time.Now()
}

func (r *QuotaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := log.FromContext(ctx).WithName("quota-controller").WithValues(
		"quota", req.NamespacedName, "task", "P9-T-006",
	)

	var quota inferencev1alpha1.Quota
	if err := r.Client.Get(ctx, req.NamespacedName, &quota); err != nil {
		if client.IgnoreNotFound(err) == nil {
			lg.V(1).Info("Quota deleted, no-op")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get Quota: %w", err)
	}

	usage, err := r.computeUsage(ctx, &quota)
	if err != nil {
		lg.Error(err, "Failed to compute Quota.status.usage")
		// Requeue per period — let next tick retry.
		return ctrl.Result{RequeueAfter: QuotaReconcilePeriod}, nil
	}

	updated := quota.DeepCopy()
	updated.Status.Usage = usage
	t := metav1.NewTime(r.now())
	updated.Status.LastSyncTime = &t
	setCondition(&updated.Status.Conditions, r.activeCondition())

	if err := r.Client.Status().Update(ctx, updated); err != nil {
		lg.Error(err, "Failed to update Quota status")
		return ctrl.Result{}, fmt.Errorf("update Quota status: %w", err)
	}

	lg.V(1).Info("Quota status synced",
		"currentSliceAllocations", usage.CurrentSliceAllocations,
		"scaleEventsInWindow", usage.ScaleEventsInWindow)
	return ctrl.Result{RequeueAfter: QuotaReconcilePeriod}, nil
}

// computeUsage walks NPUSliceAllocation list + NPUVerticalScaler scaleHistory
// in the Quota's namespace to produce a fresh QuotaUsage snapshot.
func (r *QuotaReconciler) computeUsage(ctx context.Context, quota *inferencev1alpha1.Quota) (inferencev1alpha1.QuotaUsage, error) {
	var usage inferencev1alpha1.QuotaUsage

	allocCount, err := r.countNPUSliceAllocations(ctx, quota.Namespace)
	if err != nil {
		return usage, fmt.Errorf("list NPUSliceAllocation: %w", err)
	}
	usage.CurrentSliceAllocations = allocCount

	windowSecs := quota.Spec.Enforcement.MaxScaleEventsPerWindow.WindowSeconds
	if windowSecs == 0 {
		windowSecs = 3600
	}
	scaleEvents, err := r.sumScaleEventsInWindow(ctx, quota.Namespace, time.Duration(windowSecs)*time.Second)
	if err != nil {
		return usage, fmt.Errorf("sum NPUVerticalScaler scaleHistory: %w", err)
	}
	usage.ScaleEventsInWindow = scaleEvents

	return usage, nil
}

// countNPUSliceAllocations lists NPUSliceAllocation in the namespace via
// unstructured (operators/CLAUDE.md §1 cross-module import rule).
func (r *QuotaReconciler) countNPUSliceAllocations(ctx context.Context, namespace string) (int32, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   NPUSliceAllocationGVK.Group,
		Version: NPUSliceAllocationGVK.Version,
		Kind:    NPUSliceAllocationGVK.Kind + "List",
	})
	if err := r.Client.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return 0, err
	}
	return int32(len(list.Items)), nil
}

// sumScaleEventsInWindow lists NPUVerticalScaler in the namespace and sums
// ScaleHistory entries whose Time is within the last `window`.
func (r *QuotaReconciler) sumScaleEventsInWindow(ctx context.Context, namespace string, window time.Duration) (int32, error) {
	var list inferencev1alpha1.NPUVerticalScalerList
	if err := r.Client.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return 0, err
	}
	cutoff := r.now().Add(-window)
	var sum int32
	for _, scaler := range list.Items {
		for _, ev := range scaler.Status.ScaleHistory {
			if ev.Time.Time.After(cutoff) {
				sum++
			}
		}
	}
	return sum, nil
}

// activeCondition returns the Active=True condition for happy-path
// reconciliation. Errors are not surfaced as Active=False because the
// transient failure is requeued; only sustained failure across many ticks
// would justify flipping Active (not implemented Phase 9 · Phase 10 polish).
func (r *QuotaReconciler) activeCondition() metav1.Condition {
	return metav1.Condition{
		Type:               inferencev1alpha1.ConditionQuotaActive,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.NewTime(r.now()),
		Reason:             "ControllerReady",
		Message:            "Quota controller reconciling status.usage on 60s tick",
	}
}

// SetupWithManager registers QuotaReconciler with the manager.
func (r *QuotaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&inferencev1alpha1.Quota{}).
		Named("quota-controller").
		Complete(r)
}
