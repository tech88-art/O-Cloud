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
	"os"
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

// ClusterQuotaReconcilePeriod is the requeue cadence (mirrors the namespace
// QuotaReconcilePeriod) per ADR-0014 §2 Decision D + ADR-0018 §2 Decision D.
const ClusterQuotaReconcilePeriod = 60 * time.Second

// KarmadaCachedFromClusterAnnotation is the annotation Karmada's
// karmada-aggregated-apiserver stamps on objects lifted from member clusters,
// recording the source member cluster name. When the inference-operator runs
// against the aggregated apiserver (ADR-0018 §2 Decision D · O2DMS_KARMADA_
// AGGREGATED_ENABLED), listed NPUSliceAllocation / NPUVerticalScaler objects
// carry this so we can bucket usage PerCluster. Objects WITHOUT it (plain
// single-cluster mode) bucket under the local cluster name.
const KarmadaCachedFromClusterAnnotation = "resource.karmada.io/cached-from-cluster-name"

// LocalClusterNameEnv overrides the bucket name for objects with no Karmada
// source annotation (single-cluster / host objects). Defaults to "host".
const LocalClusterNameEnv = "KARMADA_CLUSTER_NAME"

// DefaultLocalClusterName is the PerCluster key for host/single-cluster usage.
const DefaultLocalClusterName = "host"

// ClusterQuotaReconciler reconciles the cluster-scoped ClusterQuota.status.usage
// against observed cluster-wide NPUSliceAllocation count + NPUVerticalScaler
// scale-event sum, bucketed per source cluster (ADR-0018 §2 Decision D
// lifted-informer pattern), then RecomputeTotal() sums PerCluster → Total.
//
// Reconcile loop on 60s tick:
//  1. Get ClusterQuota → if !found, no-op
//  2. List NPUSliceAllocation cluster-wide (all namespaces) → bucket count by
//     source cluster (Karmada annotation · else local)
//  3. List NPUVerticalScaler cluster-wide → bucket scaleHistory-in-window by
//     source cluster
//  4. usage.PerCluster = buckets · usage.RecomputeTotal()
//  5. Update ClusterQuota.status.usage + LastSyncTime + Active condition
//  6. Requeue 60s
//
// The two admission webhooks read ClusterQuota.status.usage.Total via the
// ClusterQuotaCache (webhook package, 5s TTL); the controller is the sole
// writer of status.usage on the tick.
type ClusterQuotaReconciler struct {
	Client   client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// LocalClusterName buckets objects without a Karmada source annotation.
	// Empty → env LocalClusterNameEnv → DefaultLocalClusterName.
	LocalClusterName string

	// NowFn is injected for testability (override in unit tests).
	NowFn func() time.Time
}

func (r *ClusterQuotaReconciler) now() time.Time {
	if r.NowFn != nil {
		return r.NowFn()
	}
	return time.Now()
}

func (r *ClusterQuotaReconciler) localName() string {
	if r.LocalClusterName != "" {
		return r.LocalClusterName
	}
	if v := os.Getenv(LocalClusterNameEnv); v != "" {
		return v
	}
	return DefaultLocalClusterName
}

func (r *ClusterQuotaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := log.FromContext(ctx).WithName("clusterquota-controller").WithValues(
		"clusterquota", req.Name, "task", "P13-T-204",
	)

	var cq inferencev1alpha1.ClusterQuota
	if err := r.Client.Get(ctx, req.NamespacedName, &cq); err != nil {
		if client.IgnoreNotFound(err) == nil {
			lg.V(1).Info("ClusterQuota deleted, no-op")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get ClusterQuota: %w", err)
	}

	usage, err := r.computeUsage(ctx, &cq)
	if err != nil {
		lg.Error(err, "Failed to compute ClusterQuota.status.usage")
		return ctrl.Result{RequeueAfter: ClusterQuotaReconcilePeriod}, nil
	}

	updated := cq.DeepCopy()
	updated.Status.Usage = usage
	t := metav1.NewTime(r.now())
	updated.Status.LastSyncTime = &t
	setCondition(&updated.Status.Conditions, r.activeCondition())

	if err := r.Client.Status().Update(ctx, updated); err != nil {
		lg.Error(err, "Failed to update ClusterQuota status")
		return ctrl.Result{}, fmt.Errorf("update ClusterQuota status: %w", err)
	}

	lg.V(1).Info("ClusterQuota status synced",
		"clusters", len(usage.PerCluster),
		"totalSliceAllocations", usage.Total.CurrentSliceAllocations,
		"totalScaleEventsInWindow", usage.Total.ScaleEventsInWindow)
	return ctrl.Result{RequeueAfter: ClusterQuotaReconcilePeriod}, nil
}

// computeUsage walks cluster-wide NPUSliceAllocation + NPUVerticalScaler
// scaleHistory, buckets by source cluster, and sums PerCluster → Total.
func (r *ClusterQuotaReconciler) computeUsage(ctx context.Context, cq *inferencev1alpha1.ClusterQuota) (inferencev1alpha1.ClusterQuotaUsage, error) {
	perCluster := map[string]inferencev1alpha1.QuotaUsage{}

	// (2) NPUSliceAllocation cluster-wide, bucketed by source cluster.
	allocList := &unstructured.UnstructuredList{}
	allocList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   NPUSliceAllocationGVK.Group,
		Version: NPUSliceAllocationGVK.Version,
		Kind:    NPUSliceAllocationGVK.Kind + "List",
	})
	if err := r.Client.List(ctx, allocList); err != nil {
		return inferencev1alpha1.ClusterQuotaUsage{}, fmt.Errorf("list NPUSliceAllocation: %w", err)
	}
	for i := range allocList.Items {
		cluster := r.clusterOf(allocList.Items[i].GetAnnotations())
		u := perCluster[cluster]
		u.CurrentSliceAllocations++
		perCluster[cluster] = u
	}

	// (3) NPUVerticalScaler scaleHistory-in-window cluster-wide, bucketed.
	windowSecs := cq.Spec.Enforcement.MaxScaleEventsPerWindow.WindowSeconds
	if windowSecs == 0 {
		windowSecs = 3600
	}
	cutoff := r.now().Add(-time.Duration(windowSecs) * time.Second)
	var scalerList inferencev1alpha1.NPUVerticalScalerList
	if err := r.Client.List(ctx, &scalerList); err != nil {
		return inferencev1alpha1.ClusterQuotaUsage{}, fmt.Errorf("list NPUVerticalScaler: %w", err)
	}
	for i := range scalerList.Items {
		cluster := r.clusterOf(scalerList.Items[i].GetAnnotations())
		var inWindow int32
		for _, ev := range scalerList.Items[i].Status.ScaleHistory {
			if ev.Time.Time.After(cutoff) {
				inWindow++
			}
		}
		if inWindow > 0 {
			u := perCluster[cluster]
			u.ScaleEventsInWindow += inWindow
			perCluster[cluster] = u
		}
	}

	usage := inferencev1alpha1.ClusterQuotaUsage{PerCluster: perCluster}
	usage.RecomputeTotal()
	return usage, nil
}

// clusterOf returns the source cluster for an object: the Karmada
// cached-from-cluster annotation when present (aggregated mode), else the
// local cluster name (single-cluster / host objects).
func (r *ClusterQuotaReconciler) clusterOf(annotations map[string]string) string {
	if c := annotations[KarmadaCachedFromClusterAnnotation]; c != "" {
		return c
	}
	return r.localName()
}

func (r *ClusterQuotaReconciler) activeCondition() metav1.Condition {
	return metav1.Condition{
		Type:               inferencev1alpha1.ConditionQuotaActive,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.NewTime(r.now()),
		Reason:             "ControllerReady",
		Message:            "ClusterQuota controller reconciling status.usage on 60s tick",
	}
}

// SetupWithManager registers ClusterQuotaReconciler with the manager.
func (r *ClusterQuotaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&inferencev1alpha1.ClusterQuota{}).
		Named("clusterquota-controller").
		Complete(r)
}
