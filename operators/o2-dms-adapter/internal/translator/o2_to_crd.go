/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package translator converts between O-RAN O2 IMS R1 NB-side types and
// internal ocloud CRD shapes per ADR-0013 §3 Decision C 6-row mapping
// table.
//
// P9-T-104 body landing per ADR-0013 §2 Decision D · operator-owned
// translation logic · no cross-module Go imports (uses unstructured ·
// per operators/CLAUDE.md §1).
package translator

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/tech88-art/O-Cloud/operators/o2-dms-adapter/internal/types"
)

// ModelServiceFromCreateRequest builds an unstructured ModelService
// manifest from an O2 DeploymentItemCreateRequest. The result is ready
// to be `dynamic.Resource(GVR).Namespace(ns).Create(...)`. K8s API
// server admission chain (inference-operator webhook + Quota webhook)
// validates further.
//
// O2 NB mapping per ADR-0013 §3 Decision C table row 3:
//   - O2 deploymentItem.name / namespace / modelImage → ModelService
//     .metadata.name / namespace · .spec.model.image
//   - O2 deploymentItem.slicePoolRef → ModelService.spec.npuSlicePoolRef.name
func ModelServiceFromCreateRequest(req *types.DeploymentItemCreateRequest) (*unstructured.Unstructured, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	if req.Name == "" || req.Namespace == "" {
		return nil, fmt.Errorf("name + namespace required")
	}
	if req.ModelImage == "" {
		return nil, fmt.Errorf("modelImage required")
	}

	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("inference.ocloud.edge.example.com/v1alpha1")
	obj.SetKind("ModelService")
	obj.SetName(req.Name)
	obj.SetNamespace(req.Namespace)

	spec := map[string]interface{}{
		"model": map[string]interface{}{
			"image": req.ModelImage,
		},
	}
	if req.SlicePoolRef != "" {
		spec["npuSlicePoolRef"] = map[string]interface{}{
			"name": req.SlicePoolRef,
		}
	}
	if err := unstructured.SetNestedMap(obj.Object, spec, "spec"); err != nil {
		return nil, fmt.Errorf("set spec: %w", err)
	}
	return obj, nil
}

// DeploymentItemFromModelService builds an O2 DeploymentItem NB response
// from an unstructured ModelService. NPUVerticalScaler.status (if
// available · optional input) is inlined into Extensions per ADR-0013
// §3 Decision C 末段 (scaler is dynamic behavior of deploymentItem).
func DeploymentItemFromModelService(ms *unstructured.Unstructured, scaler *unstructured.Unstructured) (*types.DeploymentItem, error) {
	if ms == nil {
		return nil, fmt.Errorf("nil ModelService")
	}
	image, _, _ := unstructured.NestedString(ms.Object, "spec", "model", "image")
	poolRef, _, _ := unstructured.NestedString(ms.Object, "spec", "npuSlicePoolRef", "name")
	status, _, _ := unstructured.NestedString(ms.Object, "status", "phase")
	item := &types.DeploymentItem{
		ID:           string(ms.GetUID()),
		Name:         ms.GetName(),
		Namespace:    ms.GetNamespace(),
		ModelImage:   image,
		SlicePoolRef: poolRef,
		Status:       status,
	}
	if item.ID == "" {
		// UID may be empty for newly-created objects before K8s stamps · use ns/name as fallback
		item.ID = ms.GetNamespace() + "/" + ms.GetName()
	}
	if scaler != nil {
		scalerExt, err := scalerExtension(scaler)
		if err == nil && scalerExt != nil {
			item.Extensions = map[string]interface{}{"verticalScaler": scalerExt}
		}
	}
	return item, nil
}

// scalerExtension extracts the relevant NPUVerticalScaler status fields.
func scalerExtension(scaler *unstructured.Unstructured) (map[string]interface{}, error) {
	ext := map[string]interface{}{
		"name":      scaler.GetName(),
		"namespace": scaler.GetNamespace(),
	}
	currentTemplate, _, _ := unstructured.NestedString(scaler.Object, "status", "observedTarget", "currentTemplate")
	if currentTemplate != "" {
		ext["currentTemplate"] = currentTemplate
	}
	lastScaleTime, _, _ := unstructured.NestedString(scaler.Object, "status", "lastScaleTime")
	if lastScaleTime != "" {
		ext["lastScaleTime"] = lastScaleTime
	}
	scaleHistory, _, _ := unstructured.NestedSlice(scaler.Object, "status", "scaleHistory")
	if len(scaleHistory) > 0 {
		ext["scaleHistoryCount"] = len(scaleHistory)
	}
	return ext, nil
}

// NewLifecycleOperation creates a new in-memory tracked lifecycle op.
func NewLifecycleOperation(opType, target string) *types.LifecycleOperation {
	now := time.Now()
	return &types.LifecycleOperation{
		ID:        fmt.Sprintf("op-%d", now.UnixNano()),
		Type:      opType,
		Target:    target,
		Status:    "pending",
		StartedAt: now,
	}
}

// MarkLifecycleOperationCompleted updates an op to completed.
func MarkLifecycleOperationCompleted(op *types.LifecycleOperation, message string) {
	now := time.Now()
	op.Status = "completed"
	op.FinishedAt = &now
	op.Message = message
}

// MarkLifecycleOperationFailed updates an op to failed.
func MarkLifecycleOperationFailed(op *types.LifecycleOperation, errMsg string) {
	now := time.Now()
	op.Status = "failed"
	op.FinishedAt = &now
	op.Message = errMsg
}

// MetaToObjectMeta clones the unstructured meta into metav1.ObjectMeta
// (helper for tests + future ext where typed access is convenient).
func MetaToObjectMeta(obj *unstructured.Unstructured) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		UID:       obj.GetUID(),
	}
}
