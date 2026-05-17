// Package model contains DTOs that mirror docs/api-contract.yaml components.schemas.
//
// IMPORTANT (per backend/CLAUDE.md §3):
//   - This package MUST NOT import any kubernetes types. It is a pure DTO layer.
//   - Field names + JSON tags MUST stay 1:1 with the OpenAPI contract. Drift breaks
//     the contract-consistency CI check.
package model

// Quantity mirrors components.schemas.Quantity.
//
// The Bytes pointer is nullable on the wire — encoding/json emits `null` for nil.
type Quantity struct {
	Raw   string `json:"raw"`
	Bytes *int64 `json:"bytes,omitempty"`
}

// Error mirrors components.schemas.Error (the canonical error envelope used by all
// /api/v1 handlers). respondError in pkg/api always produces this shape.
type Error struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// Health mirrors components.schemas.Health (returned by GET /api/v1/healthz).
//
// Datasources is a map of source-name → status. Phase 1 returns empty map; later
// phases populate one entry per registered datasource via factory.go.
type Health struct {
	Status      string            `json:"status"`
	Datasources map[string]string `json:"datasources"`
}

// Version mirrors components.schemas.Version (returned by GET /api/v1/version).
//
// All fields are populated at build time via -ldflags in the Makefile, except
// Version which is the constant from pkg/api/system.go.
type Version struct {
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	BuildTime string `json:"buildTime,omitempty"`
}

// ResourceRef mirrors components.schemas.ResourceRef.
type ResourceRef struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}
