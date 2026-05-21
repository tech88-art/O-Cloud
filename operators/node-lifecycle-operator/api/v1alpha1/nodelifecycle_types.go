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

// NodeLifecycle is the node-lifecycle-operator CRD per ADR-0003 v2
// IMS 7 services phasing (P9-T-IMS-1 · Phase 9 P9-T-105 scaffold).
// References StarlingX node lifecycle state machine model · adapted for
// O-Cloud edge platform context (multi-site Karmada + edge KubeEdge nodes).
//
// Phase 9 P9-T-105: api types only · no controller body. Phase 10
// controller body + reconcile loops + helm chart land per ADR-0003 v2.
//
// +kubebuilder:resource:scope=Cluster,shortName=nodelc
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="NodeName",type=string,JSONPath=`.spec.nodeName`
// +kubebuilder:printcolumn:name="LastTransition",type=date,JSONPath=`.status.lastTransitionTime`
type NodeLifecycle struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeLifecycleSpec   `json:"spec,omitempty"`
	Status NodeLifecycleStatus `json:"status,omitempty"`
}

// NodeLifecycleSpec describes the desired lifecycle state of a node.
type NodeLifecycleSpec struct {
	// NodeName is the K8s `core/v1.Node.metadata.name` this lifecycle
	// object tracks. Cluster-scoped: 1 NodeLifecycle per cluster Node.
	// +kubebuilder:validation:Required
	NodeName string `json:"nodeName"`

	// DesiredState is the lifecycle state the operator should drive
	// the node toward.
	// +kubebuilder:default=Unlocked
	DesiredState NodeLifecycleState `json:"desiredState,omitempty"`

	// MaintenanceWindow is an optional time window during which
	// disruptive transitions (RebootRequired / Locked) are permitted.
	// +optional
	MaintenanceWindow *MaintenanceWindow `json:"maintenanceWindow,omitempty"`
}

// NodeLifecycleState enumerates the lifecycle states per StarlingX
// adapted model (P9-T-105 scaffold spec · Phase 10 controller body
// implements transitions).
// +kubebuilder:validation:Enum=Provisioning;Bootstrap;Available;DegradedAvailable;Unavailable;Locked;Unlocked;RebootRequired
type NodeLifecycleState string

const (
	// NodeStateProvisioning · initial provisioning (OS install + K8s join).
	NodeStateProvisioning NodeLifecycleState = "Provisioning"
	// NodeStateBootstrap · K8s components bootstrapping after join.
	NodeStateBootstrap NodeLifecycleState = "Bootstrap"
	// NodeStateAvailable · ready to accept workloads.
	NodeStateAvailable NodeLifecycleState = "Available"
	// NodeStateDegradedAvailable · partial functionality · degraded resources.
	NodeStateDegradedAvailable NodeLifecycleState = "DegradedAvailable"
	// NodeStateUnavailable · not ready / unhealthy / network partition.
	NodeStateUnavailable NodeLifecycleState = "Unavailable"
	// NodeStateLocked · administratively blocked from accepting workloads.
	NodeStateLocked NodeLifecycleState = "Locked"
	// NodeStateUnlocked · administratively cleared to accept workloads.
	NodeStateUnlocked NodeLifecycleState = "Unlocked"
	// NodeStateRebootRequired · reboot pending (kernel update etc.).
	NodeStateRebootRequired NodeLifecycleState = "RebootRequired"
)

// MaintenanceWindow describes a time range during which disruptive
// transitions are permitted.
type MaintenanceWindow struct {
	// Start is the start timestamp.
	Start metav1.Time `json:"start"`
	// DurationSeconds is the window length.
	// +kubebuilder:validation:Minimum=60
	DurationSeconds int32 `json:"durationSeconds"`
}

// NodeLifecycleStatus carries the observed lifecycle state.
type NodeLifecycleStatus struct {
	// State is the currently observed state.
	State NodeLifecycleState `json:"state,omitempty"`

	// LastTransitionTime is when the state last changed.
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`

	// Conditions tracks per-state condition events. Phase 9 scaffold:
	// 5-7 condition types per ADR-0003 v2 scaffold acceptance.
	// Per-condition types defined by Phase 10 controller body when
	// reconcile loop lands. Common: Ready / DiskPressure /
	// NetworkUnavailable / RebootRequired / Maintenance / Degraded.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// NodeLifecycleList is a list of NodeLifecycle objects.
type NodeLifecycleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeLifecycle `json:"items"`
}
