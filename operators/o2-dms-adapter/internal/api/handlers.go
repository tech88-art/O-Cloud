/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package api hosts the O2 DMS Adapter HTTP handlers per ADR-0013 §4 NB
// endpoint catalog. Phase 9 P9-T-008 scaffold ships 7 stub handlers
// returning 501 Not Implemented + ErrorEnvelope; P9-T-104 body landing
// fills in actual logic (inventory aggregation + ModelService Create /
// Delete via controller-runtime client + LifecycleOperation in-memory
// queue).
package api

import (
	"encoding/json"
	"net/http"

	"github.com/tech88-art/O-Cloud/operators/o2-dms-adapter/internal/types"
)

// Handler bundles the 7 stub handlers. Phase 9 scaffold receives the
// inventory.Client + lifecycle.Queue via dependency injection from
// cmd/main.go; the scaffold here exposes the constructor + handler
// methods. P9-T-104 body will populate real inventory aggregation +
// lifecycle queue logic.
type Handler struct {
	// SpecVersion is the locked O-RAN spec baseline string echoed in
	// 501 Not Implemented responses + future /healthz endpoint. Defaults
	// to types.SpecVersion when zero.
	SpecVersion string
}

// NewHandler constructs a Handler with sane defaults.
func NewHandler() *Handler {
	return &Handler{SpecVersion: types.SpecVersion}
}

// notImplemented writes a 501 Not Implemented response with the ErrorEnvelope
// shape per ADR-0013 §4 catalog table末. The body's details field carries
// the NB endpoint name so client implementations can detect which stub
// they hit during the Phase 9 scaffold window.
func (h *Handler) notImplemented(w http.ResponseWriter, endpoint string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	envelope := types.ErrorEnvelope{
		Code:    http.StatusNotImplemented,
		Message: "P9-T-008 scaffold stub · body lands in P9-T-104",
		Details: "endpoint=" + endpoint + " · spec=" + h.SpecVersion,
	}
	_ = json.NewEncoder(w).Encode(envelope)
}

// CreateDeploymentItem stubs POST /o2dms/v1/deploymentItems (ADR-0013 §4 row 1).
func (h *Handler) CreateDeploymentItem(w http.ResponseWriter, r *http.Request) {
	h.notImplemented(w, "POST /o2dms/v1/deploymentItems")
}

// ListDeploymentItems stubs GET /o2dms/v1/deploymentItems (ADR-0013 §4 row 2).
func (h *Handler) ListDeploymentItems(w http.ResponseWriter, r *http.Request) {
	h.notImplemented(w, "GET /o2dms/v1/deploymentItems")
}

// GetDeploymentItem stubs GET /o2dms/v1/deploymentItems/{id} (ADR-0013 §4 row 3).
func (h *Handler) GetDeploymentItem(w http.ResponseWriter, r *http.Request) {
	h.notImplemented(w, "GET /o2dms/v1/deploymentItems/{id}")
}

// DeleteDeploymentItem stubs DELETE /o2dms/v1/deploymentItems/{id} (ADR-0013 §4 row 4).
func (h *Handler) DeleteDeploymentItem(w http.ResponseWriter, r *http.Request) {
	h.notImplemented(w, "DELETE /o2dms/v1/deploymentItems/{id}")
}

// GetInventory stubs GET /o2dms/v1/inventory (ADR-0013 §4 row 5).
func (h *Handler) GetInventory(w http.ResponseWriter, r *http.Request) {
	h.notImplemented(w, "GET /o2dms/v1/inventory")
}

// ListDeploymentManagers stubs GET /o2dms/v1/deploymentManagers (ADR-0013 §4 row 6).
func (h *Handler) ListDeploymentManagers(w http.ResponseWriter, r *http.Request) {
	h.notImplemented(w, "GET /o2dms/v1/deploymentManagers")
}

// GetLifecycleOperation stubs GET /o2dms/v1/lifecycleOperations/{id} (ADR-0013 §4 row 7).
func (h *Handler) GetLifecycleOperation(w http.ResponseWriter, r *http.Request) {
	h.notImplemented(w, "GET /o2dms/v1/lifecycleOperations/{id}")
}
