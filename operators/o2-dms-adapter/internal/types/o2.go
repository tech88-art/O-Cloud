/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package types defines the O-RAN O2 IMS R1 NB-side resource types
// exposed by the O2 DMS Adapter per ADR-0013 §2 Decision B + §3 Decision C
// 6-row mapping table.
//
// Spec lock: O-RAN.WG6.O2IMS-INTERFACE-R003-v04.00 (ADR-0013 §1 Context ·
// re-WebFetch portal in P9-T-104 body landing to evaluate R004-v07.00.00
// upgrade per ADR-0013 §5 Open question (a)).
//
// All types live under one package to keep the NB shape contract in one
// place; phase 10 polish can split per resource if helpful.
package types

import "time"

// DeploymentManager is the O2 IMS R1 cluster-manager metadata object.
// ADR-0013 §3 Decision C table row 1: 1:1 with K8s cluster.
type DeploymentManager struct {
	ID          string            `json:"deploymentManagerId"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Capabilities []string         `json:"capabilities,omitempty"`
	Extensions  map[string]string `json:"extensions,omitempty"`
}

// InfrastructureInventory is the aggregated view over NPUSlicePool +
// Node + NPU device + NPUSliceAllocation per ADR-0013 §3 Decision C
// table row 2. Phase 9 single-cluster aggregation; Phase 10 polish
// adds Karmada multi-cluster aggregator.
type InfrastructureInventory struct {
	GeneratedAt time.Time              `json:"generatedAt"`
	Nodes       []NodeEntry            `json:"nodes,omitempty"`
	SlicePools  []SlicePoolEntry       `json:"slicePools,omitempty"`
	Allocations []SliceAllocationEntry `json:"allocations,omitempty"`
}

// NodeEntry is one node in the inventory.
type NodeEntry struct {
	Name     string            `json:"name"`
	NPUCount int               `json:"npuCount"`
	Labels   map[string]string `json:"labels,omitempty"`
}

// SlicePoolEntry is one NPUSlicePool in the inventory.
type SlicePoolEntry struct {
	Namespace        string `json:"namespace"`
	Name             string `json:"name"`
	Strategy         string `json:"strategy,omitempty"`
	TotalSlices      int32  `json:"totalSlices"`
	AvailableSlices  int32  `json:"availableSlices"`
}

// SliceAllocationEntry is one NPUSliceAllocation in the inventory.
type SliceAllocationEntry struct {
	Namespace        string `json:"namespace"`
	Name             string `json:"name"`
	ModelServiceRef  string `json:"modelServiceRef,omitempty"`
	NPUSliceTemplate string `json:"npuSliceTemplate,omitempty"`
}

// DeploymentItem is the O2 IMS R1 NB representation of an inference
// deployment. Phase 9 mapping: 1:1 with ocloud ModelService per
// ADR-0013 §3 Decision C table row 3.
type DeploymentItem struct {
	ID              string            `json:"deploymentItemId"`
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace"`
	ModelImage      string            `json:"modelImage,omitempty"`
	SlicePoolRef    string            `json:"slicePoolRef,omitempty"`
	Status          string            `json:"status,omitempty"`
	// Extensions carries vendor-specific fields (per O2 IMS R1 spec).
	// Phase 9 inlines NPUVerticalScaler.status here (per ADR-0013 §3
	// Decision C 末段 — scaler is dynamic behavior of deploymentItem
	// rather than independent NB resource).
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// DeploymentItemCreateRequest is the POST body for /o2dms/v1/deploymentItems.
type DeploymentItemCreateRequest struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	ModelImage   string `json:"modelImage"`
	SlicePoolRef string `json:"slicePoolRef,omitempty"`
}

// LifecycleOperation tracks async ops queue (Create / Delete ModelService
// async paths). ADR-0013 §3 Decision C table row 4. Phase 9 in-memory
// (process-local · 1h TTL); Phase 10 polish persistent backing via
// P9-T-107 cache spike outcome.
type LifecycleOperation struct {
	ID         string    `json:"lifecycleOperationId"`
	Type       string    `json:"type"`         // create / delete / etc.
	Target     string    `json:"target"`       // deploymentItem id or name
	Status     string    `json:"status"`       // pending / running / completed / failed
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	Message    string    `json:"message,omitempty"`
}

// ErrorEnvelope is the standard error response shape per ADR-0013 §4
// (catalog table末) — all 4xx / 5xx responses use this envelope.
type ErrorEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// SpecVersion is the locked O-RAN spec baseline string (used in /healthz
// + DESIGN.md cross-ref). ADR-0013 §1 Context lock.
const SpecVersion = "O-RAN.WG6.O2IMS-INTERFACE-R003-v04.00"
