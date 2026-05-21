/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"

	"github.com/tech88-art/O-Cloud/operators/o2-dms-adapter/internal/inventory"
	"github.com/tech88-art/O-Cloud/operators/o2-dms-adapter/internal/types"
)

// stubClient implements inventory.Client for handler tests.
type stubClient struct {
	mss           []unstructured.Unstructured
	createErr     error
	deleteErr     error
	aggregateSnap inventory.InventorySnapshot
	aggregateErr  error
}

func (s *stubClient) AggregateInventory(ctx context.Context) (inventory.InventorySnapshot, error) {
	return s.aggregateSnap, s.aggregateErr
}

func (s *stubClient) ListModelServices(ctx context.Context) ([]unstructured.Unstructured, error) {
	return s.mss, nil
}

func (s *stubClient) GetModelService(ctx context.Context, namespace, name string) (*unstructured.Unstructured, error) {
	for i := range s.mss {
		if s.mss[i].GetNamespace() == namespace && s.mss[i].GetName() == name {
			return &s.mss[i], nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "inference.ocloud.edge.example.com", Resource: "modelservices"}, name)
}

func (s *stubClient) CreateModelService(ctx context.Context, ns string, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	obj.SetUID(k8stypes.UID("test-uid-" + obj.GetName()))
	s.mss = append(s.mss, *obj)
	return obj, nil
}

func (s *stubClient) DeleteModelService(ctx context.Context, namespace, name string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	for i := range s.mss {
		if s.mss[i].GetNamespace() == namespace && s.mss[i].GetName() == name {
			s.mss = append(s.mss[:i], s.mss[i+1:]...)
			return nil
		}
	}
	return apierrors.NewNotFound(schema.GroupResource{Group: "inference.ocloud.edge.example.com", Resource: "modelservices"}, name)
}

func (s *stubClient) ClusterInfo(ctx context.Context) (inventory.DeploymentManagerInfo, error) {
	return inventory.DeploymentManagerInfo{ID: "test-cluster", Name: "test-cluster", Description: "test"}, nil
}

func makeMS(name, namespace, image string) unstructured.Unstructured {
	obj := unstructured.Unstructured{}
	obj.SetAPIVersion("inference.ocloud.edge.example.com/v1alpha1")
	obj.SetKind("ModelService")
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetUID(k8stypes.UID("uid-" + name))
	_ = unstructured.SetNestedMap(obj.Object, map[string]interface{}{"model": map[string]interface{}{"image": image}}, "spec")
	return obj
}

// TestCreateDeploymentItem covers POST /o2dms/v1/deploymentItems body.
func TestCreateDeploymentItem(t *testing.T) {
	inv := &stubClient{}
	h := NewHandler(inv)
	router := NewRouter(h)

	body, _ := json.Marshal(types.DeploymentItemCreateRequest{
		Name:         "qwen-pd",
		Namespace:    "ai-edge-demo",
		ModelImage:   "vllm-ascend:v0.11.0",
		SlicePoolRef: "qwen-pool",
	})
	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/deploymentItems", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	var item types.DeploymentItem
	if err := json.NewDecoder(rec.Body).Decode(&item); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if item.Name != "qwen-pd" || item.Namespace != "ai-edge-demo" || item.ModelImage != "vllm-ascend:v0.11.0" {
		t.Errorf("item mismatch: %+v", item)
	}
	if len(inv.mss) != 1 {
		t.Errorf("expected 1 ModelService created, got %d", len(inv.mss))
	}
}

// TestCreateDeploymentItem_AdmissionReject covers webhook reject 422.
func TestCreateDeploymentItem_AdmissionReject(t *testing.T) {
	inv := &stubClient{createErr: apierrors.NewInvalid(schema.GroupKind{Group: "inference.ocloud.edge.example.com", Kind: "ModelService"}, "qwen-pd", nil)}
	h := NewHandler(inv)
	router := NewRouter(h)
	body, _ := json.Marshal(types.DeploymentItemCreateRequest{Name: "qwen-pd", Namespace: "ai-edge-demo", ModelImage: "vllm-ascend:v0.11.0"})
	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/deploymentItems", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 (admission reject)", rec.Code)
	}
}

// TestCreateDeploymentItem_BadRequest covers invalid body / missing fields.
func TestCreateDeploymentItem_BadRequest(t *testing.T) {
	inv := &stubClient{}
	h := NewHandler(inv)
	router := NewRouter(h)
	// Missing modelImage
	body, _ := json.Marshal(types.DeploymentItemCreateRequest{Name: "qwen-pd", Namespace: "ai-edge-demo"})
	req := httptest.NewRequest(http.MethodPost, "/o2dms/v1/deploymentItems", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

// TestListDeploymentItems covers GET /o2dms/v1/deploymentItems.
func TestListDeploymentItems(t *testing.T) {
	inv := &stubClient{
		mss: []unstructured.Unstructured{
			makeMS("qwen-pd", "ai-edge-demo", "vllm-ascend:v0.11.0"),
			makeMS("llama-3", "ai-edge-demo", "vllm-ascend:v0.11.0"),
		},
	}
	h := NewHandler(inv)
	router := NewRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/deploymentItems", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var items []types.DeploymentItem
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

// TestGetDeploymentItem_Found covers GET /o2dms/v1/deploymentItems/{id}.
// Phase 9 P9-T-104: id format = K8s UID (preferred · no URL encoding
// concerns). Tests use the UID stamped by stubClient.CreateModelService
// (test-uid-{name}) or pre-seeded uid-{name}.
func TestGetDeploymentItem_Found(t *testing.T) {
	inv := &stubClient{mss: []unstructured.Unstructured{makeMS("qwen-pd", "ai-edge-demo", "vllm-ascend:v0.11.0")}}
	h := NewHandler(inv)
	router := NewRouter(h)
	// By UID (stamped uid-{name} via makeMS helper)
	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/deploymentItems/uid-qwen-pd", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

// TestGetDeploymentItem_NotFound covers 404 path.
func TestGetDeploymentItem_NotFound(t *testing.T) {
	inv := &stubClient{}
	h := NewHandler(inv)
	router := NewRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/deploymentItems/missing", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// TestDeleteDeploymentItem covers DELETE by UID.
func TestDeleteDeploymentItem(t *testing.T) {
	inv := &stubClient{mss: []unstructured.Unstructured{makeMS("qwen-pd", "ai-edge-demo", "vllm-ascend:v0.11.0")}}
	h := NewHandler(inv)
	router := NewRouter(h)
	req := httptest.NewRequest(http.MethodDelete, "/o2dms/v1/deploymentItems/uid-qwen-pd", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204; body=%s", rec.Code, rec.Body.String())
	}
}

// TestGetInventory covers GET /o2dms/v1/inventory.
func TestGetInventory(t *testing.T) {
	inv := &stubClient{
		aggregateSnap: inventory.InventorySnapshot{
			Nodes:      []inventory.NodeSummary{{Name: "node-1", NPUCount: 8}},
			SlicePools: []inventory.SlicePoolSummary{{Name: "pool-1", Namespace: "ocloud-system", TotalSlices: 16, AvailableSlices: 12}},
		},
	}
	h := NewHandler(inv)
	router := NewRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/inventory", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp types.InfrastructureInventory
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Nodes) != 1 || resp.Nodes[0].Name != "node-1" || resp.Nodes[0].NPUCount != 8 {
		t.Errorf("nodes mismatch: %+v", resp.Nodes)
	}
	if len(resp.SlicePools) != 1 || resp.SlicePools[0].TotalSlices != 16 {
		t.Errorf("pools mismatch: %+v", resp.SlicePools)
	}
}

// TestListDeploymentManagers covers GET /o2dms/v1/deploymentManagers.
func TestListDeploymentManagers(t *testing.T) {
	inv := &stubClient{}
	h := NewHandler(inv)
	router := NewRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/deploymentManagers", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var mgrs []types.DeploymentManager
	if err := json.NewDecoder(rec.Body).Decode(&mgrs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(mgrs) != 1 {
		t.Errorf("expected 1 manager, got %d", len(mgrs))
	}
	if mgrs[0].ID == "" {
		t.Errorf("manager ID empty")
	}
}

// TestGetLifecycleOperation covers GET /o2dms/v1/lifecycleOperations/{id}.
func TestGetLifecycleOperation(t *testing.T) {
	inv := &stubClient{}
	h := NewHandler(inv)
	router := NewRouter(h)
	// First create a deployment item to populate lifecycle ops queue
	body, _ := json.Marshal(types.DeploymentItemCreateRequest{Name: "x", Namespace: "default", ModelImage: "img:1"})
	cReq := httptest.NewRequest(http.MethodPost, "/o2dms/v1/deploymentItems", bytes.NewReader(body))
	cRec := httptest.NewRecorder()
	router.ServeHTTP(cRec, cReq)
	if cRec.Code != http.StatusCreated {
		t.Fatalf("setup create failed: %d %s", cRec.Code, cRec.Body.String())
	}
	// Get the op ID from the queue
	var foundOp *types.LifecycleOperation
	h.LifecycleOps.mu.Lock()
	for _, op := range h.LifecycleOps.ops {
		if op.Type == "create" {
			foundOp = op
			break
		}
	}
	h.LifecycleOps.mu.Unlock()
	if foundOp == nil {
		t.Fatalf("no create op tracked")
	}
	// Now GET it
	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/lifecycleOperations/"+foundOp.ID, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

// TestGetLifecycleOperation_NotFound covers 404.
func TestGetLifecycleOperation_NotFound(t *testing.T) {
	inv := &stubClient{}
	h := NewHandler(inv)
	router := NewRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/lifecycleOperations/op-missing", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// TestRouter404OnUnknownPath defensive: unknown path returns 404.
func TestRouter404OnUnknownPath(t *testing.T) {
	h := NewHandler(&stubClient{})
	router := NewRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/o2dms/v1/unknown-resource", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// TestNewHandlerNilInventory ensures NewHandler(nil) falls back to NoopClient.
func TestNewHandlerNilInventory(t *testing.T) {
	h := NewHandler(nil)
	if h.Inventory == nil {
		t.Fatalf("Inventory not set with nil input")
	}
	// Sanity · NoopClient AggregateInventory returns empty
	snap, err := h.Inventory.AggregateInventory(context.Background())
	if err != nil {
		t.Errorf("noop AggregateInventory err: %v", err)
	}
	if snap.NodeCount != 0 {
		t.Errorf("expected empty snap; got %+v", snap)
	}
}

// fakeInventoryUnusedHelpers silences the "declared but not used" lint
// for now-unused fmt/strings imports above.
var (
	_ = fmt.Sprintf
	_ = strings.Contains
)
