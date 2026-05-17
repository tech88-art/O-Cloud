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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodePoolSpec is the level-2 pool: a set of Kubernetes Nodes selected by
// label inside a parent ClusterPool. The selector is evaluated by the
// pool-operator on each reconcile; matching nodes form Status.Nodes.
//
// See `docs/architecture.md` §6.4.
type NodePoolSpec struct {
	// ClusterPoolRef references the parent ClusterPool (Cluster-scoped). The
	// reference is by name only; both objects are Cluster-scoped so no
	// namespace is required.
	// +optional
	ClusterPoolRef *corev1.LocalObjectReference `json:"clusterPoolRef,omitempty"`

	// Selector restricts the Nodes that belong to this pool.
	// +kubebuilder:validation:Required
	Selector *metav1.LabelSelector `json:"selector"`

	// Role tags the pool as edge (close-to-data) or core (central).
	// The aggregator and scheduler use this to enforce affinity policies.
	// +kubebuilder:validation:Required
	Role NodeRole `json:"role"`

	// Location is an opaque physical-placement label
	// (e.g. "dc-shanghai-rack-7"). Used for topology-aware display only.
	// +optional
	Location string `json:"location,omitempty"`
}

// NodePoolStatus aggregates per-Node capacity for the pool.
type NodePoolStatus struct {
	// Nodes lists the resolved Node names matching Spec.Selector.
	// +optional
	// +listType=set
	Nodes []string `json:"nodes,omitempty"`

	// TotalCPU is the sum of CPU capacity across all Nodes in the pool.
	// +optional
	TotalCPU resource.Quantity `json:"totalCPU,omitempty"`

	// TotalMemory is the sum of memory capacity across all Nodes in the pool.
	// +optional
	TotalMemory resource.Quantity `json:"totalMemory,omitempty"`

	// Conditions follow the standard Kubernetes condition convention.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=np
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.spec.role`
// +kubebuilder:printcolumn:name="Location",type=string,JSONPath=`.spec.location`
// +kubebuilder:printcolumn:name="Nodes",type=string,JSONPath=`.status.nodes[*]`,description="Selected node names"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// NodePool is the level-2 pool resource: groups Nodes by selector inside a
// ClusterPool. Hierarchy: ClusterPool -> NodePool -> NPUPool ->
// NPUSlicePool.
type NodePool struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the desired state of NodePool.
	// +kubebuilder:validation:Required
	Spec NodePoolSpec `json:"spec"`

	// status defines the observed state of NodePool.
	// +optional
	Status NodePoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodePoolList contains a list of NodePool.
type NodePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodePool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodePool{}, &NodePoolList{})
}
