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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterPoolSpec is the top of the 4-level pool hierarchy. It groups one or
// more member Kubernetes clusters (federated via Karmada) into a single
// logical capacity unit for the O-Cloud platform.
//
// See `docs/architecture.md` §6.5 for design context.
type ClusterPoolSpec struct {
	// Clusters lists the member clusters that participate in this pool.
	// At least one cluster must be specified.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +listType=map
	// +listMapKey=clusterId
	Clusters []ClusterRef `json:"clusters"`
}

// ClusterPoolStatus reflects the observed health of the federated member set.
type ClusterPoolStatus struct {
	// HealthyClusters lists the cluster IDs whose control plane is reachable
	// and reporting healthy in the most recent reconcile.
	// +optional
	// +listType=set
	HealthyClusters []string `json:"healthyClusters,omitempty"`

	// UnhealthyClusters lists the cluster IDs whose control plane is
	// unreachable or reporting unhealthy.
	// +optional
	// +listType=set
	UnhealthyClusters []string `json:"unhealthyClusters,omitempty"`

	// Conditions follow the standard Kubernetes condition convention.
	// Standard types: Available, Progressing, Degraded.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=cp
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Clusters",type=string,JSONPath=`.spec.clusters[*].clusterId`,description="Member cluster IDs"
// +kubebuilder:printcolumn:name="Healthy",type=string,JSONPath=`.status.healthyClusters[*]`,description="Cluster IDs reporting healthy"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ClusterPool is the Schema for the clusterpools API. It is the apex
// (level-1) resource of the pool hierarchy: ClusterPool -> NodePool ->
// NPUPool -> NPUSlicePool.
type ClusterPool struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec defines the desired state of ClusterPool.
	// +kubebuilder:validation:Required
	Spec ClusterPoolSpec `json:"spec"`

	// status defines the observed state of ClusterPool.
	// +optional
	Status ClusterPoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterPoolList contains a list of ClusterPool.
type ClusterPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterPool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterPool{}, &ClusterPoolList{})
}
