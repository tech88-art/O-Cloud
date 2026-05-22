/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterQuota is the cluster-scope NPU-aware resource quota CRD per
// ADR-0014 §7 Forward notes line 1 + ADR-0018 §2 Decision D
// (Phase 11 P11-T-104).
//
// While namespace-scope Quota CRs(per existing quota_types.go)cap
// per-namespace NPUSliceAllocation count + scale-event rate, ClusterQuota
// caps cluster-wide totals across all namespaces · enforces a single
// cluster-wide budget for NPU slice consumption · and aggregates usage
// cross-cluster via Karmada karmada-aggregated-apiserver lifted-informer
// pattern(per ADR-0018 §2 Decision D).
//
// Enforcement: admission webhooks(extended in P11-T-104)read
// Status.Usage.Total to make decisions. status.usage.perCluster map
// records member cluster contributions(populated by the Quota controller
// via Karmada aggregated client when O2DMS_KARMADA_AGGREGATED_ENABLED=true
// · per inventory/karmada_aggregated.go helper · P11-T-103 substrate).
//
// API group lives under `inference.ocloud.edge.example.com/v1alpha1`
// (same scheme as namespace-scope Quota).
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=cq
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="MaxSliceAllocations",type=integer,JSONPath=`.spec.enforcement.maxSliceAllocations`
// +kubebuilder:printcolumn:name="CurrentTotal",type=integer,JSONPath=`.status.usage.total.currentSliceAllocations`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type ClusterQuota struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterQuotaSpec   `json:"spec,omitempty"`
	Status ClusterQuotaStatus `json:"status,omitempty"`
}

// ClusterQuotaSpec mirrors namespace-scope QuotaSpec but at cluster scope.
type ClusterQuotaSpec struct {
	// Enforcement carries the cluster-wide caps.
	Enforcement QuotaEnforcement `json:"enforcement"`
}

// ClusterQuotaStatus tracks the observed cluster + per-member usage.
type ClusterQuotaStatus struct {
	// Conditions tracks Active / EnforcementOK / AggregationStale
	// (Phase 11+ added · indicates Karmada aggregated-apiserver
	// last-sync staleness)。
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Usage carries Total + per-cluster breakdowns.
	// +optional
	Usage ClusterQuotaUsage `json:"usage,omitempty"`

	// LastSyncTime is the timestamp the controller last refreshed Usage.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`
}

// ClusterQuotaUsage carries cluster-wide aggregated + per-member breakdown
// usage data per ADR-0018 §2 Decision D.
type ClusterQuotaUsage struct {
	// Total is the sum across all member clusters(host + member1 +
	// member2 + ...). Compared against Spec.Enforcement.MaxSliceAllocations
	// by admission webhook decision.
	Total QuotaUsage `json:"total"`

	// PerCluster records the per-member contribution. Keys are Karmada
	// cluster names(`member1`, `member2`, ...). Empty when single-cluster
	// mode(O2DMS_KARMADA_AGGREGATED_ENABLED=false).
	// +optional
	PerCluster map[string]QuotaUsage `json:"perCluster,omitempty"`
}

// +kubebuilder:object:root=true
// ClusterQuotaList is a list of ClusterQuota objects.
type ClusterQuotaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterQuota `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterQuota{}, &ClusterQuotaList{})
}

// RecomputeTotal sums PerCluster entries into Total. Used by the Quota
// controller after each Karmada karmada-aggregated-apiserver list refresh.
// Idempotent · zero-value PerCluster yields zero Total.
func (u *ClusterQuotaUsage) RecomputeTotal() {
	var total QuotaUsage
	for _, sub := range u.PerCluster {
		total.CurrentSliceAllocations += sub.CurrentSliceAllocations
		total.ScaleEventsInWindow += sub.ScaleEventsInWindow
	}
	u.Total = total
}
