/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package inventory hosts the K8s client wrapper for the O2 DMS Adapter
// inventory aggregation path per ADR-0013 §2 Decision D + §3 Decision C
// table row 2. Phase 9 P9-T-008 scaffold provides constructor + interface
// stubs; P9-T-104 body landing populates informer/lister implementations
// reading NPUSlicePool / Node / NPU device / NPUSliceAllocation /
// ModelService / NPUVerticalScaler.
package inventory

import "context"

// Client is the inventory aggregation contract. Phase 9 P9-T-008 scaffold
// ships an interface + NoopClient impl returning empty inventories;
// P9-T-104 body lands a controller-runtime informer-backed impl.
type Client interface {
	// AggregateInventory builds the InfrastructureInventory snapshot per
	// ADR-0013 §3 Decision C table row 2 (aggregates NPUSlicePool + Node +
	// NPU + NPUSliceAllocation).
	AggregateInventory(ctx context.Context) (InventorySnapshot, error)
}

// InventorySnapshot is the package-internal inventory aggregation
// result. The api package wraps this into types.InfrastructureInventory
// for the NB JSON response (decouples wire shape from internal model).
type InventorySnapshot struct {
	NodeCount       int
	SlicePoolCount  int
	AllocationCount int
}

// NoopClient is the Phase 9 P9-T-008 scaffold placeholder · returns
// empty inventory on every call. P9-T-104 replaces with real impl.
type NoopClient struct{}

// AggregateInventory returns an empty snapshot. P9-T-104 body replaces.
func (NoopClient) AggregateInventory(ctx context.Context) (InventorySnapshot, error) {
	return InventorySnapshot{}, nil
}

// NewNoopClient constructs the scaffold no-op client.
func NewNoopClient() Client {
	return NoopClient{}
}
