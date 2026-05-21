/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BareMetalNode is the bare-metal-provisioning-operator CRD per
// ADR-0003 v2 (P9-T-IMS-3 · Phase 9 P9-T-105 scaffold). Models
// Metal3 / cluster-api BareMetalHost pattern adapted for O-Cloud
// edge context · bmc{address,credentials} + provisioning state machine.
//
// Phase 9 P9-T-105: api types only · controller body Phase 10.
//
// +kubebuilder:resource:scope=Cluster,shortName=bmnode
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.provisioningState`
// +kubebuilder:printcolumn:name="MAC",type=string,JSONPath=`.status.macAddress`
// +kubebuilder:printcolumn:name="BMCAddress",type=string,JSONPath=`.spec.bmc.address`
type BareMetalNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BareMetalNodeSpec   `json:"spec,omitempty"`
	Status BareMetalNodeStatus `json:"status,omitempty"`
}

// BareMetalNodeSpec describes the BMC connection + desired provisioning.
type BareMetalNodeSpec struct {
	// BMC carries the Baseboard Management Controller endpoint + creds.
	// +kubebuilder:validation:Required
	BMC BMCSpec `json:"bmc"`

	// Image is the OS image to provision (oci:// or http:// URI to
	// raw / qcow2 / iso).
	// +optional
	Image *ImageSpec `json:"image,omitempty"`

	// DesiredState is the provisioning state the operator should drive
	// the bare-metal node toward.
	// +kubebuilder:default=Inspecting
	DesiredState ProvisioningState `json:"desiredState,omitempty"`
}

// BMCSpec is the BMC connection spec.
type BMCSpec struct {
	// Address is the BMC endpoint (e.g. "ipmi://192.0.2.10" or
	// "redfish+https://bmc.example.com").
	// +kubebuilder:validation:Required
	Address string `json:"address"`

	// CredentialsRef is the name of a Secret in the same namespace as
	// the BareMetalNode (cluster-scoped CRD references a namespaced
	// Secret · namespace defaults to operator's own).
	// +kubebuilder:validation:Required
	CredentialsRef CredentialsReference `json:"credentialsRef"`
}

// CredentialsReference identifies a Secret with BMC credentials.
type CredentialsReference struct {
	// Name of the Secret containing keys "username" + "password".
	Name string `json:"name"`
	// Namespace of the Secret.
	// +kubebuilder:default=ocloud-system
	Namespace string `json:"namespace,omitempty"`
}

// ImageSpec describes the OS image to provision.
type ImageSpec struct {
	// URL is the image source URI.
	URL string `json:"url"`
	// Checksum is the SHA-256 hex digest of the image.
	Checksum string `json:"checksum,omitempty"`
}

// ProvisioningState enumerates the bare-metal provisioning state machine.
// +kubebuilder:validation:Enum=Inspecting;Registering;Provisioning;Provisioned;Ready;Deprovisioning;Error
type ProvisioningState string

const (
	// BMStateInspecting · initial BMC discovery + hardware inventory.
	BMStateInspecting ProvisioningState = "Inspecting"
	// BMStateRegistering · adding to inventory.
	BMStateRegistering ProvisioningState = "Registering"
	// BMStateProvisioning · OS image being written.
	BMStateProvisioning ProvisioningState = "Provisioning"
	// BMStateProvisioned · OS image written · boot in progress.
	BMStateProvisioned ProvisioningState = "Provisioned"
	// BMStateReady · node booted + K8s joined.
	BMStateReady ProvisioningState = "Ready"
	// BMStateDeprovisioning · OS removal in progress.
	BMStateDeprovisioning ProvisioningState = "Deprovisioning"
	// BMStateError · BMC unreachable / inspection / provisioning failed.
	BMStateError ProvisioningState = "Error"
)

// BareMetalNodeStatus carries the observed provisioning state.
type BareMetalNodeStatus struct {
	// ProvisioningState is the currently observed state.
	ProvisioningState ProvisioningState `json:"provisioningState,omitempty"`

	// MACAddress is the MAC address discovered during BMC inspection.
	MACAddress string `json:"macAddress,omitempty"`

	// HardwareInfo carries inspected hardware metadata (CPU count ·
	// memory bytes · NPU count etc.).
	HardwareInfo *HardwareInfo `json:"hardwareInfo,omitempty"`

	// LastTransitionTime is when ProvisioningState last changed.
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`

	// Conditions tracks Ready / BMCReachable / ImagePulled etc.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// HardwareInfo carries hardware inventory from BMC inspection.
type HardwareInfo struct {
	// CPUCount is the number of CPU cores.
	CPUCount int32 `json:"cpuCount,omitempty"`
	// MemoryBytes is total RAM bytes.
	MemoryBytes int64 `json:"memoryBytes,omitempty"`
	// NPUCount is the number of Ascend NPU devices.
	NPUCount int32 `json:"npuCount,omitempty"`
}

// +kubebuilder:object:root=true
type BareMetalNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BareMetalNode `json:"items"`
}
