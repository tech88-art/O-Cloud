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

// SoftwareBundle is the software-mgmt-operator CRD per ADR-0003 v2
// (P9-T-IMS-2 · Phase 9 P9-T-105 scaffold). Models node-level software
// version inventory + rollout policy per StarlingX sw-deployment pattern.
//
// Phase 9 P9-T-105: api types only · controller body Phase 10.
//
// +kubebuilder:resource:scope=Cluster,shortName=swbundle
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="RolloutPolicy",type=string,JSONPath=`.spec.rolloutPolicy.strategy`
// +kubebuilder:printcolumn:name="AppliedNodes",type=integer,JSONPath=`.status.appliedNodeCount`
// +kubebuilder:printcolumn:name="TargetNodes",type=integer,JSONPath=`.status.targetNodeCount`
type SoftwareBundle struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SoftwareBundleSpec   `json:"spec,omitempty"`
	Status SoftwareBundleStatus `json:"status,omitempty"`
}

// SoftwareBundleSpec describes the desired software state.
type SoftwareBundleSpec struct {
	// Version is the human-readable bundle version (e.g. "v1.2.0").
	// +kubebuilder:validation:Required
	Version string `json:"version"`

	// Patches is the ordered list of software patches in this bundle.
	// Each patch carries a name + source URI + checksum for verification.
	// +kubebuilder:validation:Required
	Patches []SoftwarePatch `json:"patches"`

	// RolloutPolicy controls how patches are applied across nodes.
	// +kubebuilder:validation:Required
	RolloutPolicy RolloutPolicy `json:"rolloutPolicy"`

	// NodeSelector limits the bundle to a subset of nodes (optional ·
	// empty means all nodes).
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

// SoftwarePatch describes one patch in the bundle.
type SoftwarePatch struct {
	// Name identifies the patch within the bundle (must be unique).
	Name string `json:"name"`
	// SourceURI is the patch artifact location (oci:// · http:// · file://).
	SourceURI string `json:"sourceURI"`
	// Checksum is the SHA-256 hex digest of the patch artifact.
	Checksum string `json:"checksum"`
}

// RolloutPolicy controls the rollout strategy.
type RolloutPolicy struct {
	// Strategy enum: RollingUpdate / Parallel / Sequential.
	// +kubebuilder:default=RollingUpdate
	// +kubebuilder:validation:Enum=RollingUpdate;Parallel;Sequential
	Strategy RolloutStrategy `json:"strategy,omitempty"`

	// MaxUnavailable is the maximum number of nodes that can be in the
	// middle of applying patches simultaneously (RollingUpdate only).
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	MaxUnavailable int32 `json:"maxUnavailable,omitempty"`
}

// RolloutStrategy enumerates rollout strategies.
// +kubebuilder:validation:Enum=RollingUpdate;Parallel;Sequential
type RolloutStrategy string

const (
	// RolloutStrategyRollingUpdate applies patches node-by-node with
	// MaxUnavailable concurrency cap.
	RolloutStrategyRollingUpdate RolloutStrategy = "RollingUpdate"
	// RolloutStrategyParallel applies to all selected nodes concurrently.
	RolloutStrategyParallel RolloutStrategy = "Parallel"
	// RolloutStrategySequential applies one node at a time strictly.
	RolloutStrategySequential RolloutStrategy = "Sequential"
)

// SoftwareBundleStatus carries the observed rollout state.
type SoftwareBundleStatus struct {
	// AppliedVersion is the version successfully applied across nodes.
	// Empty when no nodes have completed rollout yet.
	AppliedVersion string `json:"appliedVersion,omitempty"`

	// TargetNodeCount is the number of nodes matching spec.nodeSelector
	// (or all nodes if no selector).
	TargetNodeCount int32 `json:"targetNodeCount,omitempty"`

	// AppliedNodeCount is the number of nodes that have completed rollout.
	AppliedNodeCount int32 `json:"appliedNodeCount,omitempty"`

	// FailedNodeCount is the number of nodes that failed rollout.
	FailedNodeCount int32 `json:"failedNodeCount,omitempty"`

	// Conditions tracks Active / Progressing / Completed / Failed lifecycle.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
type SoftwareBundleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SoftwareBundle `json:"items"`
}
