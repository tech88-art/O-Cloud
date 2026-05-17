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

// Package v1alpha1 — shared sub-resource types used by multiple Pool CRDs.
// Field names align 1:1 with `docs/api-contract.yaml` components.schemas where
// the same concept is exposed via REST.
package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

// SliceStrategy identifies how an NPUSlicePool partitions its parent NPUs.
// +kubebuilder:validation:Enum=FixedTemplate;Dynamic
type SliceStrategy string

const (
	// SliceStrategyFixedTemplate selects a discrete set of slice shapes
	// (StarlingX-style hard partitioning).
	SliceStrategyFixedTemplate SliceStrategy = "FixedTemplate"
	// SliceStrategyDynamic allows runtime sizing within a [min, max] range.
	SliceStrategyDynamic SliceStrategy = "Dynamic"
)

// SyncPolicy describes how a ClusterPool federates member clusters
// (Karmada terminology).
// +kubebuilder:validation:Enum=Push;Pull
type SyncPolicy string

const (
	SyncPolicyPush SyncPolicy = "Push"
	SyncPolicyPull SyncPolicy = "Pull"
)

// NodeRole identifies how a NodePool's members are deployed in the topology.
// +kubebuilder:validation:Enum=edge;core
type NodeRole string

const (
	NodeRoleEdge NodeRole = "edge"
	NodeRoleCore NodeRole = "core"
)

// SliceTemplate is a discrete slice shape used by FixedTemplate strategy.
// Reflects the Ascend virtualization shape vocabulary (e.g. "vir04", "vir08").
type SliceTemplate struct {
	// Name is the template identifier, e.g. "vir04".
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name"`

	// AICoreCount is the number of AI Cores allocated to each slice instance
	// of this template.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	AICoreCount int32 `json:"aiCoreCount"`

	// MemoryMiB is the HBM (high-bandwidth memory) reservation per slice
	// instance, in mebibytes.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	MemoryMiB int32 `json:"memoryMiB"`
}

// DynamicSlicingSpec parameterises the Dynamic slicing strategy.
// Slices are sized at allocation time within these bounds rather than
// pre-materialised from templates.
type DynamicSlicingSpec struct {
	// MinAICore is the smallest AI Core count allowed for a dynamic slice.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	MinAICore int32 `json:"minAICore"`

	// MaxAICore is the largest AI Core count allowed for a dynamic slice.
	// Must be >= MinAICore.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	MaxAICore int32 `json:"maxAICore"`

	// MemoryGranularityMiB is the smallest HBM allocation unit, in mebibytes.
	// All dynamic memory requests are rounded up to a multiple of this value.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	MemoryGranularityMiB int32 `json:"memoryGranularityMiB"`

	// AllowAggregation enables a single slice request to span more than one
	// physical NPU device (Phase 7+ capability; gated off by default).
	// +optional
	AllowAggregation bool `json:"allowAggregation,omitempty"`
}

// HCCSTopologyInfo describes the High-speed Cache Coherent Switch fabric that
// interconnects NPUs within an NPUPool. The aggregator/scheduler uses this to
// place co-located slices on NPUs with low-latency peer links.
type HCCSTopologyInfo struct {
	// FabricID is the operator-assigned identifier for the HCCS mesh
	// (e.g. "hccs-rack-1").
	// +optional
	FabricID string `json:"fabricId,omitempty"`

	// PeerGroups lists sets of NPU device IDs that share a single HCCS hop.
	// Devices within a group communicate over HCCS; cross-group traffic falls
	// back to PCIe / RoCE.
	// +optional
	// +listType=atomic
	PeerGroups []HCCSPeerGroup `json:"peerGroups,omitempty"`
}

// HCCSPeerGroup is one HCCS peer set within an NPUPool's fabric.
type HCCSPeerGroup struct {
	// GroupID identifies the peer set within its parent fabric.
	// +kubebuilder:validation:Required
	GroupID string `json:"groupId"`

	// DeviceIDs lists the NPU device IDs that share this HCCS peer set.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +listType=set
	DeviceIDs []string `json:"deviceIds"`
}

// SliceState is the lifecycle state of one slice instance.
// +kubebuilder:validation:Enum=Idle;Allocated;Error
type SliceState string

const (
	SliceStateIdle      SliceState = "Idle"
	SliceStateAllocated SliceState = "Allocated"
	SliceStateError     SliceState = "Error"
)

// SliceStatusEntry is one row in NPUSlicePoolStatus.Slices listing the live
// state of a slice instance.
type SliceStatusEntry struct {
	// ID is the unique identifier of the slice instance within its pool.
	// +kubebuilder:validation:Required
	ID string `json:"id"`

	// TemplateName references the SliceTemplate used to materialise this
	// slice (empty when Strategy=Dynamic).
	// +optional
	TemplateName string `json:"templateName,omitempty"`

	// NPUDeviceID is the underlying physical NPU device this slice maps to.
	// +kubebuilder:validation:Required
	NPUDeviceID string `json:"npuDeviceId"`

	// AICoreCount is the actual AI Core count of this slice instance.
	// +kubebuilder:validation:Minimum=0
	AICoreCount int32 `json:"aiCoreCount"`

	// MemoryMiB is the actual HBM reservation of this slice instance.
	// +kubebuilder:validation:Minimum=0
	MemoryMiB int32 `json:"memoryMiB"`

	// State is the lifecycle state of this slice instance.
	// +kubebuilder:validation:Required
	State SliceState `json:"state"`

	// AllocatedTo is the ResourceClaim reference (DRA) when State=Allocated.
	// +optional
	AllocatedTo *corev1.ObjectReference `json:"allocatedTo,omitempty"`
}

// ClusterRef identifies a member cluster of a ClusterPool. Aligned with
// `components.schemas.ClusterPool.clusters[]` in docs/api-contract.yaml.
type ClusterRef struct {
	// ClusterID is the Karmada-managed identifier for this cluster.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClusterID string `json:"clusterId"`

	// SyncPolicy controls whether Karmada pushes manifests to the member or
	// the member pulls from the control plane.
	// +kubebuilder:validation:Required
	SyncPolicy SyncPolicy `json:"syncPolicy"`
}
