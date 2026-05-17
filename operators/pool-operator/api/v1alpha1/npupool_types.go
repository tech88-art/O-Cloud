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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NPUPoolSpec is the level-3 pool: NPU devices on Nodes that belong to a
// parent NodePool, narrowed by Selector and pinned to a single NPU model
// (Ascend 910B by default in Phase 1).
//
// See `docs/architecture.md` §6.3.
type NPUPoolSpec struct {
	// NodePoolRef references the parent NodePool (Cluster-scoped). Required
	// so the operator can constrain candidate Nodes when materialising NPUs.
	// +kubebuilder:validation:Required
	NodePoolRef corev1.LocalObjectReference `json:"nodePoolRef"`

	// NPUModel is the device model this pool collects. Pools must be
	// homogeneous; multi-model deployments use one NPUPool per model.
	// Example values: "Ascend910B", "Ascend910C".
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	NPUModel string `json:"npuModel"`

	// Selector further narrows which NPU devices in the NodePool belong to
	// this pool (e.g. by firmware revision or rack label).
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty"`

	// SliceStrategy names a NPUSlicePool resource (by name; same group) that
	// defines how this pool's NPUs are partitioned. The pool-operator
	// materialises slices according to that strategy.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	SliceStrategy string `json:"sliceStrategy"`
}

// NPUPoolStatus reports per-device availability for the pool.
type NPUPoolStatus struct {
	// TotalNPUs is the number of physical NPU devices currently in the pool.
	// +optional
	// +kubebuilder:validation:Minimum=0
	TotalNPUs int32 `json:"totalNPUs,omitempty"`

	// HealthyNPUs is the count of devices reporting healthy in the last
	// reconcile.
	// +optional
	// +kubebuilder:validation:Minimum=0
	HealthyNPUs int32 `json:"healthyNPUs,omitempty"`

	// AllocatedNPUs is the count of devices currently bound to one or more
	// active slice allocations.
	// +optional
	// +kubebuilder:validation:Minimum=0
	AllocatedNPUs int32 `json:"allocatedNPUs,omitempty"`

	// HCCSTopology describes the high-speed peer-link fabric across the
	// devices in this pool, if any.
	// +optional
	HCCSTopology *HCCSTopologyInfo `json:"hccsTopology,omitempty"`

	// Conditions follow the standard Kubernetes condition convention.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=npup
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.spec.npuModel`
// +kubebuilder:printcolumn:name="Total",type=integer,JSONPath=`.status.totalNPUs`
// +kubebuilder:printcolumn:name="Healthy",type=integer,JSONPath=`.status.healthyNPUs`
// +kubebuilder:printcolumn:name="Allocated",type=integer,JSONPath=`.status.allocatedNPUs`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// NPUPool is the level-3 pool resource: NPU devices grouped by NodePool +
// model. Hierarchy: ClusterPool -> NodePool -> NPUPool -> NPUSlicePool.
type NPUPool struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the desired state of NPUPool.
	// +kubebuilder:validation:Required
	Spec NPUPoolSpec `json:"spec"`

	// status defines the observed state of NPUPool.
	// +optional
	Status NPUPoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NPUPoolList contains a list of NPUPool.
type NPUPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NPUPool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NPUPool{}, &NPUPoolList{})
}
