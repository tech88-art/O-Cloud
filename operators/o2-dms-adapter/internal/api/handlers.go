/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package api hosts the O2 DMS Adapter HTTP handlers per ADR-0013 §4
// NB endpoint catalog. P9-T-008 scaffold shipped 7 stub handlers
// (501 + ErrorEnvelope). P9-T-104 body lands real implementations:
// inventory aggregation + ModelService CRUD via dynamic client + in-memory
// LifecycleOperation queue.
package api

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/tech88-art/O-Cloud/operators/o2-dms-adapter/internal/inventory"
	"github.com/tech88-art/O-Cloud/operators/o2-dms-adapter/internal/translator"
	"github.com/tech88-art/O-Cloud/operators/o2-dms-adapter/internal/types"
)

// Handler bundles the 7 NB endpoint handlers. P9-T-104 body landed:
// real impl via inventory.Client + translator + in-memory LifecycleOperation
// queue.
type Handler struct {
	SpecVersion string

	// Inventory is the K8s client wrapper (dynamic + core).
	Inventory inventory.Client

	// LifecycleOps is the in-memory queue (process-local · 1h TTL ·
	// Phase 10 polish persistent backing per ADR-0013 §5 Open question (e)).
	LifecycleOps *LifecycleOperationQueue
}

// NewHandler constructs a Handler with sane defaults.
// inv may be NoopClient for scaffold tests.
func NewHandler(inv inventory.Client) *Handler {
	if inv == nil {
		inv = inventory.NewNoopClient()
	}
	return &Handler{
		SpecVersion:  types.SpecVersion,
		Inventory:    inv,
		LifecycleOps: NewLifecycleOperationQueue(),
	}
}

// LifecycleOperationQueue is the in-memory tracker for async ops.
// Phase 9 P9-T-104 ships process-local map; Phase 10 polish persistent
// backing per ADR-0013 §5 Open question (e).
type LifecycleOperationQueue struct {
	mu  sync.Mutex
	ops map[string]*types.LifecycleOperation
}

// NewLifecycleOperationQueue creates a fresh queue.
func NewLifecycleOperationQueue() *LifecycleOperationQueue {
	return &LifecycleOperationQueue{ops: make(map[string]*types.LifecycleOperation)}
}

// Track adds an op to the queue.
func (q *LifecycleOperationQueue) Track(op *types.LifecycleOperation) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ops[op.ID] = op
}

// Get returns an op by ID.
func (q *LifecycleOperationQueue) Get(id string) (*types.LifecycleOperation, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	op, ok := q.ops[id]
	return op, ok
}

// jsonResponse writes a JSON body with status code.
func jsonResponse(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// errorResponse writes an ErrorEnvelope.
func errorResponse(w http.ResponseWriter, status int, message, details string) {
	jsonResponse(w, status, types.ErrorEnvelope{Code: status, Message: message, Details: details})
}

// CreateDeploymentItem · POST /o2dms/v1/deploymentItems (ADR-0013 §4 row 1).
func (h *Handler) CreateDeploymentItem(w http.ResponseWriter, r *http.Request) {
	var req types.DeploymentItemCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	manifest, err := translator.ModelServiceFromCreateRequest(&req)
	if err != nil {
		errorResponse(w, http.StatusBadRequest, "translation failed", err.Error())
		return
	}
	op := translator.NewLifecycleOperation("create", req.Namespace+"/"+req.Name)
	h.LifecycleOps.Track(op)

	created, err := h.Inventory.CreateModelService(r.Context(), req.Namespace, manifest)
	if err != nil {
		translator.MarkLifecycleOperationFailed(op, err.Error())
		if apierrors.IsAlreadyExists(err) {
			errorResponse(w, http.StatusConflict, "ModelService already exists", err.Error())
			return
		}
		if apierrors.IsInvalid(err) || apierrors.IsForbidden(err) {
			// K8s API server admission rejected (e.g., Quota webhook)
			errorResponse(w, http.StatusUnprocessableEntity, "admission rejected", err.Error())
			return
		}
		errorResponse(w, http.StatusInternalServerError, "create ModelService failed", err.Error())
		return
	}
	translator.MarkLifecycleOperationCompleted(op, "ModelService created")

	item, err := translator.DeploymentItemFromModelService(created, nil)
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "response translation failed", err.Error())
		return
	}
	jsonResponse(w, http.StatusCreated, item)
}

// ListDeploymentItems · GET /o2dms/v1/deploymentItems (ADR-0013 §4 row 2).
func (h *Handler) ListDeploymentItems(w http.ResponseWriter, r *http.Request) {
	mss, err := h.Inventory.ListModelServices(r.Context())
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "list ModelServices failed", err.Error())
		return
	}
	items := make([]*types.DeploymentItem, 0, len(mss))
	for i := range mss {
		item, err := translator.DeploymentItemFromModelService(&mss[i], nil)
		if err != nil {
			continue
		}
		items = append(items, item)
	}
	jsonResponse(w, http.StatusOK, items)
}

// GetDeploymentItem · GET /o2dms/v1/deploymentItems/{id} (ADR-0013 §4 row 3).
// `id` format per ADR-0013 §3 Decision C: K8s UID OR namespace/name fallback.
func (h *Handler) GetDeploymentItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		errorResponse(w, http.StatusBadRequest, "missing id", "")
		return
	}
	mss, err := h.Inventory.ListModelServices(r.Context())
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "list ModelServices failed", err.Error())
		return
	}
	for i := range mss {
		ms := &mss[i]
		if string(ms.GetUID()) == id || (ms.GetNamespace()+"/"+ms.GetName()) == id {
			item, terr := translator.DeploymentItemFromModelService(ms, nil)
			if terr != nil {
				errorResponse(w, http.StatusInternalServerError, "translate failed", terr.Error())
				return
			}
			jsonResponse(w, http.StatusOK, item)
			return
		}
	}
	errorResponse(w, http.StatusNotFound, "deploymentItem not found", "id="+id)
}

// DeleteDeploymentItem · DELETE /o2dms/v1/deploymentItems/{id} (ADR-0013 §4 row 4).
func (h *Handler) DeleteDeploymentItem(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		errorResponse(w, http.StatusBadRequest, "missing id", "")
		return
	}
	// Lookup the ModelService for namespace/name extraction
	mss, err := h.Inventory.ListModelServices(r.Context())
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "list failed", err.Error())
		return
	}
	for i := range mss {
		ms := &mss[i]
		if string(ms.GetUID()) == id || (ms.GetNamespace()+"/"+ms.GetName()) == id {
			op := translator.NewLifecycleOperation("delete", ms.GetNamespace()+"/"+ms.GetName())
			h.LifecycleOps.Track(op)
			if derr := h.Inventory.DeleteModelService(r.Context(), ms.GetNamespace(), ms.GetName()); derr != nil {
				translator.MarkLifecycleOperationFailed(op, derr.Error())
				errorResponse(w, http.StatusInternalServerError, "delete failed", derr.Error())
				return
			}
			translator.MarkLifecycleOperationCompleted(op, "ModelService deleted")
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	errorResponse(w, http.StatusNotFound, "deploymentItem not found", "id="+id)
}

// GetInventory · GET /o2dms/v1/inventory (ADR-0013 §4 row 5).
func (h *Handler) GetInventory(w http.ResponseWriter, r *http.Request) {
	snap, err := h.Inventory.AggregateInventory(r.Context())
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "inventory failed", err.Error())
		return
	}
	resp := types.InfrastructureInventory{
		Nodes:       make([]types.NodeEntry, 0, len(snap.Nodes)),
		SlicePools:  make([]types.SlicePoolEntry, 0, len(snap.SlicePools)),
		Allocations: make([]types.SliceAllocationEntry, 0, len(snap.Allocations)),
	}
	for _, n := range snap.Nodes {
		resp.Nodes = append(resp.Nodes, types.NodeEntry{Name: n.Name, NPUCount: n.NPUCount, Labels: n.Labels})
	}
	for _, p := range snap.SlicePools {
		resp.SlicePools = append(resp.SlicePools, types.SlicePoolEntry{
			Namespace: p.Namespace, Name: p.Name,
			Strategy: p.Strategy, TotalSlices: p.TotalSlices, AvailableSlices: p.AvailableSlices,
		})
	}
	for _, a := range snap.Allocations {
		resp.Allocations = append(resp.Allocations, types.SliceAllocationEntry{
			Namespace: a.Namespace, Name: a.Name,
			ModelServiceRef: a.ModelServiceRef, NPUSliceTemplate: a.NPUSliceTemplate,
		})
	}
	jsonResponse(w, http.StatusOK, resp)
}

// ListDeploymentManagers · GET /o2dms/v1/deploymentManagers (ADR-0013 §4 row 6).
func (h *Handler) ListDeploymentManagers(w http.ResponseWriter, r *http.Request) {
	info, err := h.Inventory.ClusterInfo(r.Context())
	if err != nil {
		errorResponse(w, http.StatusInternalServerError, "cluster info failed", err.Error())
		return
	}
	mgr := types.DeploymentManager{
		ID:          info.ID,
		Name:        info.Name,
		Description: info.Description,
		Capabilities: []string{
			"npu-slice-template",
			"vertical-scaler",
			"multi-tenant-quota",
		},
		Extensions: map[string]string{
			"specVersion": h.SpecVersion,
		},
	}
	jsonResponse(w, http.StatusOK, []types.DeploymentManager{mgr})
}

// GetLifecycleOperation · GET /o2dms/v1/lifecycleOperations/{id} (ADR-0013 §4 row 7).
func (h *Handler) GetLifecycleOperation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		errorResponse(w, http.StatusBadRequest, "missing id", "")
		return
	}
	op, ok := h.LifecycleOps.Get(id)
	if !ok {
		errorResponse(w, http.StatusNotFound, "lifecycleOperation not found", "id="+id)
		return
	}
	jsonResponse(w, http.StatusOK, op)
}
