/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package translator

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8stypes "k8s.io/apimachinery/pkg/types"

	"github.com/tech88-art/O-Cloud/operators/o2-dms-adapter/internal/types"
)

func TestModelServiceFromCreateRequest_OK(t *testing.T) {
	req := &types.DeploymentItemCreateRequest{
		Name:         "qwen-pd",
		Namespace:    "ai-edge-demo",
		ModelImage:   "vllm-ascend:v0.11.0",
		SlicePoolRef: "qwen-pool",
	}
	obj, err := ModelServiceFromCreateRequest(req)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if obj.GetName() != "qwen-pd" || obj.GetNamespace() != "ai-edge-demo" {
		t.Errorf("meta mismatch: %+v", obj.Object)
	}
	if obj.GetAPIVersion() != "inference.ocloud.edge.example.com/v1alpha1" {
		t.Errorf("apiVersion = %q", obj.GetAPIVersion())
	}
	image, _, _ := unstructured.NestedString(obj.Object, "spec", "model", "image")
	if image != "vllm-ascend:v0.11.0" {
		t.Errorf("spec.model.image = %q", image)
	}
	pool, _, _ := unstructured.NestedString(obj.Object, "spec", "npuSlicePoolRef", "name")
	if pool != "qwen-pool" {
		t.Errorf("spec.npuSlicePoolRef.name = %q", pool)
	}
}

func TestModelServiceFromCreateRequest_NilReq(t *testing.T) {
	_, err := ModelServiceFromCreateRequest(nil)
	if err == nil {
		t.Errorf("expected error for nil req")
	}
}

func TestModelServiceFromCreateRequest_MissingFields(t *testing.T) {
	cases := []*types.DeploymentItemCreateRequest{
		{Namespace: "ns", ModelImage: "img"}, // missing name
		{Name: "n", ModelImage: "img"},       // missing ns
		{Name: "n", Namespace: "ns"},         // missing image
	}
	for i, c := range cases {
		if _, err := ModelServiceFromCreateRequest(c); err == nil {
			t.Errorf("case %d: expected error for incomplete req %+v", i, c)
		}
	}
}

func TestDeploymentItemFromModelService_Basic(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("inference.ocloud.edge.example.com/v1alpha1")
	obj.SetKind("ModelService")
	obj.SetName("qwen-pd")
	obj.SetNamespace("ai-edge-demo")
	obj.SetUID(k8stypes.UID("uid-123"))
	_ = unstructured.SetNestedField(obj.Object, "vllm-ascend:v0.11.0", "spec", "model", "image")
	_ = unstructured.SetNestedField(obj.Object, "qwen-pool", "spec", "npuSlicePoolRef", "name")
	_ = unstructured.SetNestedField(obj.Object, "Ready", "status", "phase")

	item, err := DeploymentItemFromModelService(obj, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if item.ID != "uid-123" {
		t.Errorf("ID = %q, want uid-123", item.ID)
	}
	if item.Name != "qwen-pd" || item.Namespace != "ai-edge-demo" {
		t.Errorf("meta mismatch: %+v", item)
	}
	if item.ModelImage != "vllm-ascend:v0.11.0" {
		t.Errorf("image = %q", item.ModelImage)
	}
	if item.SlicePoolRef != "qwen-pool" {
		t.Errorf("poolRef = %q", item.SlicePoolRef)
	}
	if item.Status != "Ready" {
		t.Errorf("status = %q", item.Status)
	}
}

func TestDeploymentItemFromModelService_FallbackID(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetName("qwen-pd")
	obj.SetNamespace("ai-edge-demo")
	// UID empty
	item, err := DeploymentItemFromModelService(obj, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if item.ID != "ai-edge-demo/qwen-pd" {
		t.Errorf("ID fallback = %q, want ai-edge-demo/qwen-pd", item.ID)
	}
}

func TestDeploymentItemFromModelService_WithScaler(t *testing.T) {
	ms := &unstructured.Unstructured{}
	ms.SetName("qwen-pd")
	ms.SetNamespace("ai-edge-demo")
	ms.SetUID(k8stypes.UID("uid-1"))

	scaler := &unstructured.Unstructured{}
	scaler.SetName("qwen-scaler")
	scaler.SetNamespace("ai-edge-demo")
	_ = unstructured.SetNestedField(scaler.Object, "qwen-pd-busy", "status", "observedTarget", "currentTemplate")
	_ = unstructured.SetNestedField(scaler.Object, "2026-05-21T12:00:00Z", "status", "lastScaleTime")

	item, err := DeploymentItemFromModelService(ms, scaler)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if item.Extensions == nil {
		t.Fatalf("Extensions nil")
	}
	ext, ok := item.Extensions["verticalScaler"].(map[string]interface{})
	if !ok {
		t.Fatalf("verticalScaler ext missing")
	}
	if ext["currentTemplate"] != "qwen-pd-busy" {
		t.Errorf("currentTemplate ext = %v", ext["currentTemplate"])
	}
	if ext["lastScaleTime"] != "2026-05-21T12:00:00Z" {
		t.Errorf("lastScaleTime ext = %v", ext["lastScaleTime"])
	}
}

func TestLifecycleOperationLifecycle(t *testing.T) {
	op := NewLifecycleOperation("create", "ai-edge-demo/qwen-pd")
	if op.Status != "pending" {
		t.Errorf("initial status = %q", op.Status)
	}
	if op.ID == "" {
		t.Errorf("ID empty")
	}
	MarkLifecycleOperationCompleted(op, "ok")
	if op.Status != "completed" {
		t.Errorf("completed status = %q", op.Status)
	}
	if op.FinishedAt == nil {
		t.Errorf("FinishedAt nil after completion")
	}
}

func TestLifecycleOperationFailed(t *testing.T) {
	op := NewLifecycleOperation("delete", "ns/n")
	MarkLifecycleOperationFailed(op, "test-error")
	if op.Status != "failed" {
		t.Errorf("failed status = %q", op.Status)
	}
	if op.Message != "test-error" {
		t.Errorf("message = %q", op.Message)
	}
}
