/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

// Package inventory hosts the K8s client wrapper for the O2 DMS Adapter
// inventory aggregation path per ADR-0013 §2 Decision D + §3 Decision C
// table row 2.
//
// P9-T-008 scaffold shipped NoopClient placeholder (returns empty).
// P9-T-104 body lands DynamicClient impl reading NPUSlicePool + Node +
// NPUSliceAllocation + ModelService via unstructured.Unstructured + GVR
// (per operators/CLAUDE.md §1 no-cross-module-import rule).
package inventory

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// Cross-module GroupVersionResource constants (no Go import per
// operators/CLAUDE.md §1).
var (
	NPUSlicePoolGVR = schema.GroupVersionResource{
		Group: "ims.ocloud.edge.example.com", Version: "v1alpha1", Resource: "npuslicepools",
	}
	NPUSliceAllocationGVR = schema.GroupVersionResource{
		Group: "npu.ocloud.edge.example.com", Version: "v1alpha1", Resource: "npusliceallocations",
	}
	ModelServiceGVR = schema.GroupVersionResource{
		Group: "inference.ocloud.edge.example.com", Version: "v1alpha1", Resource: "modelservices",
	}
	NPUVerticalScalerGVR = schema.GroupVersionResource{
		Group: "inference.ocloud.edge.example.com", Version: "v1alpha1", Resource: "npuverticalscalers",
	}
)

// Client is the inventory aggregation contract. P9-T-008 scaffold ships
// NoopClient placeholder; P9-T-104 body lands DynamicClient impl.
type Client interface {
	// AggregateInventory builds the InfrastructureInventory snapshot per
	// ADR-0013 §3 Decision C table row 2.
	AggregateInventory(ctx context.Context) (InventorySnapshot, error)

	// ListModelServices returns all ModelServices across namespaces
	// (cluster-scoped Phase 9 single-cluster aggregation).
	ListModelServices(ctx context.Context) ([]unstructured.Unstructured, error)

	// GetModelService returns a single ModelService by namespace/name.
	GetModelService(ctx context.Context, namespace, name string) (*unstructured.Unstructured, error)

	// CreateModelService applies a ModelService manifest to the cluster.
	// Returns the created object (post-defaulting) or error.
	CreateModelService(ctx context.Context, ns string, obj *unstructured.Unstructured) (*unstructured.Unstructured, error)

	// DeleteModelService cascade-deletes a ModelService.
	DeleteModelService(ctx context.Context, namespace, name string) error

	// ClusterInfo returns the deployment manager metadata (Phase 9
	// single-cluster · 1 entry).
	ClusterInfo(ctx context.Context) (DeploymentManagerInfo, error)
}

// InventorySnapshot is the package-internal inventory aggregation result.
type InventorySnapshot struct {
	NodeCount       int
	SlicePoolCount  int
	AllocationCount int
	Nodes           []NodeSummary
	SlicePools      []SlicePoolSummary
	Allocations     []AllocationSummary
}

// NodeSummary is one node in the inventory snapshot.
type NodeSummary struct {
	Name     string
	Labels   map[string]string
	NPUCount int
}

// SlicePoolSummary is one NPUSlicePool in the snapshot.
type SlicePoolSummary struct {
	Namespace        string
	Name             string
	Strategy         string
	TotalSlices      int32
	AvailableSlices  int32
}

// AllocationSummary is one NPUSliceAllocation in the snapshot.
type AllocationSummary struct {
	Namespace        string
	Name             string
	ModelServiceRef  string
	NPUSliceTemplate string
}

// DeploymentManagerInfo carries the single-cluster manager metadata
// per ADR-0013 §3 Decision C table row 1.
type DeploymentManagerInfo struct {
	ID          string
	Name        string
	Description string
}

// DynamicClient is the P9-T-104 body impl. Uses dynamic + kubernetes
// clients to read CRDs via unstructured.
type DynamicClient struct {
	Dynamic   dynamic.Interface
	Core      kubernetes.Interface
	ClusterID string // injected via env-var · single-cluster Phase 9
}

// NewDynamicClient constructs a DynamicClient from dynamic + core clients.
func NewDynamicClient(dyn dynamic.Interface, core kubernetes.Interface, clusterID string) *DynamicClient {
	if clusterID == "" {
		clusterID = "ocloud-edge-default"
	}
	return &DynamicClient{Dynamic: dyn, Core: core, ClusterID: clusterID}
}

// AggregateInventory walks Node + NPUSlicePool + NPUSliceAllocation
// across the cluster.
func (c *DynamicClient) AggregateInventory(ctx context.Context) (InventorySnapshot, error) {
	var snap InventorySnapshot

	// List nodes (core/v1)
	nodes, err := c.Core.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return snap, err
	}
	snap.NodeCount = len(nodes.Items)
	for _, n := range nodes.Items {
		snap.Nodes = append(snap.Nodes, NodeSummary{
			Name:     n.Name,
			Labels:   n.Labels,
			NPUCount: countNPUs(n),
		})
	}

	// List NPUSlicePool (cluster-wide · ims.ocloud.edge.example.com group)
	pools, err := c.Dynamic.Resource(NPUSlicePoolGVR).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		// CRD not installed → empty pools (don't fail · degraded mode)
		pools = &unstructured.UnstructuredList{}
	}
	snap.SlicePoolCount = len(pools.Items)
	for _, p := range pools.Items {
		strategy, _, _ := unstructured.NestedString(p.Object, "spec", "strategy")
		total, _, _ := unstructured.NestedInt64(p.Object, "status", "totalSlices")
		avail, _, _ := unstructured.NestedInt64(p.Object, "status", "availableSlices")
		snap.SlicePools = append(snap.SlicePools, SlicePoolSummary{
			Namespace:       p.GetNamespace(),
			Name:            p.GetName(),
			Strategy:        strategy,
			TotalSlices:     int32(total),
			AvailableSlices: int32(avail),
		})
	}

	// List NPUSliceAllocation (cluster-wide · npu.ocloud.edge.example.com)
	allocs, err := c.Dynamic.Resource(NPUSliceAllocationGVR).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		allocs = &unstructured.UnstructuredList{}
	}
	snap.AllocationCount = len(allocs.Items)
	for _, a := range allocs.Items {
		msRef, _, _ := unstructured.NestedString(a.Object, "spec", "modelServiceRef")
		tmpl := a.GetAnnotations()["npu.huawei.com/slice-template"]
		snap.Allocations = append(snap.Allocations, AllocationSummary{
			Namespace:        a.GetNamespace(),
			Name:             a.GetName(),
			ModelServiceRef:  msRef,
			NPUSliceTemplate: tmpl,
		})
	}

	return snap, nil
}

// countNPUs counts the Ascend910B NPUs from node status.allocatable.
func countNPUs(n corev1.Node) int {
	if v, ok := n.Status.Allocatable["huawei.com/Ascend910B"]; ok {
		return int(v.Value())
	}
	return 0
}

// ListModelServices lists ModelServices cluster-wide.
func (c *DynamicClient) ListModelServices(ctx context.Context) ([]unstructured.Unstructured, error) {
	list, err := c.Dynamic.Resource(ModelServiceGVR).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

// GetModelService gets one ModelService by ns/name.
func (c *DynamicClient) GetModelService(ctx context.Context, namespace, name string) (*unstructured.Unstructured, error) {
	return c.Dynamic.Resource(ModelServiceGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
}

// CreateModelService creates a ModelService.
func (c *DynamicClient) CreateModelService(ctx context.Context, ns string, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	return c.Dynamic.Resource(ModelServiceGVR).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{})
}

// DeleteModelService deletes a ModelService.
func (c *DynamicClient) DeleteModelService(ctx context.Context, namespace, name string) error {
	return c.Dynamic.Resource(ModelServiceGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// ClusterInfo returns the deployment manager metadata for Phase 9 single-cluster.
func (c *DynamicClient) ClusterInfo(ctx context.Context) (DeploymentManagerInfo, error) {
	return DeploymentManagerInfo{
		ID:          c.ClusterID,
		Name:        c.ClusterID,
		Description: "O-Cloud edge cluster (Phase 9 single-cluster · Phase 10 polish Karmada multi-cluster)",
	}, nil
}

// NoopClient is the P9-T-008 scaffold placeholder · returns empty inventory.
// Kept for testing fallback when no K8s client is available.
type NoopClient struct{}

// AggregateInventory returns an empty snapshot.
func (NoopClient) AggregateInventory(ctx context.Context) (InventorySnapshot, error) {
	return InventorySnapshot{}, nil
}

// ListModelServices returns empty list.
func (NoopClient) ListModelServices(ctx context.Context) ([]unstructured.Unstructured, error) {
	return nil, nil
}

// GetModelService returns NotFound.
func (NoopClient) GetModelService(ctx context.Context, namespace, name string) (*unstructured.Unstructured, error) {
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: "inference.ocloud.edge.example.com", Resource: "modelservices"}, name)
}

// CreateModelService returns the input (no-op).
func (NoopClient) CreateModelService(ctx context.Context, ns string, obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	return obj, nil
}

// DeleteModelService no-op.
func (NoopClient) DeleteModelService(ctx context.Context, namespace, name string) error {
	return nil
}

// ClusterInfo returns placeholder.
func (NoopClient) ClusterInfo(ctx context.Context) (DeploymentManagerInfo, error) {
	return DeploymentManagerInfo{ID: "noop", Name: "noop"}, nil
}

// NewNoopClient constructs the scaffold no-op client.
func NewNoopClient() Client {
	return NoopClient{}
}
