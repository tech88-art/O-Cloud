package crd

import (
	"context"
	"encoding/json"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/example/ocloud-edge/backend/pkg/model"
)

// ListNPUSlicePools lists NPUSlicePool CRDs and projects them onto
// model.NPUSlicePool. Honors Options.SlicePoolNamespace — empty
// namespace lists across all namespaces.
//
// Projection: marshals each unstructured object through JSON into a
// local spec/status pair (matching the CRD types). Malformed
// entries (missing required fields per the CRD validator) are
// dropped silently; the apiserver shouldn't produce them in normal
// operation. Pool ordering is lex (namespace, name) for stable diffs.
func (s *Source) ListNPUSlicePools(ctx context.Context) ([]*model.NPUSlicePool, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var list *unstructured.UnstructuredList
	var err error
	if s.sliceNS != "" {
		list, err = s.client.Resource(gvrNPUSlicePools).Namespace(s.sliceNS).List(ctx, metav1.ListOptions{})
	} else {
		list, err = s.client.Resource(gvrNPUSlicePools).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, translateAPIError(err)
	}

	out := make([]*model.NPUSlicePool, 0, len(list.Items))
	for i := range list.Items {
		pool := projectNPUSlicePool(&list.Items[i])
		if pool == nil {
			continue
		}
		out = append(out, pool)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// projectNPUSlicePool maps one unstructured CRD object onto a
// model.NPUSlicePool. Returns nil on malformed input.
func projectNPUSlicePool(u *unstructured.Unstructured) *model.NPUSlicePool {
	if u == nil {
		return nil
	}
	// Round-trip via JSON into a typed local — the operators CRD
	// types live in a separate Go module and we deliberately avoid
	// the cross-module import (see source.go pkg header).
	type fixedTpl struct {
		Name        string `json:"name"`
		AICoreCount int    `json:"aiCoreCount"`
		MemoryMiB   int    `json:"memoryMiB"`
	}
	type dynPolicy struct {
		MinAICore            int  `json:"minAICore,omitempty"`
		MaxAICore            int  `json:"maxAICore,omitempty"`
		MemoryGranularityMiB int  `json:"memoryGranularityMiB,omitempty"`
		AllowAggregation     bool `json:"allowAggregation,omitempty"`
	}
	type spec struct {
		NPUPoolRef     map[string]string `json:"npuPoolRef,omitempty"`
		Strategy       string            `json:"strategy"`
		FixedTemplates []fixedTpl        `json:"fixedTemplates,omitempty"`
		DynamicSlicing *dynPolicy        `json:"dynamicSlicing,omitempty"`
	}
	type status struct {
		TotalSlices     int `json:"totalSlices,omitempty"`
		AllocatedSlices int `json:"allocatedSlices,omitempty"`
		AvailableSlices int `json:"availableSlices,omitempty"`
	}
	type wrapper struct {
		Spec   spec   `json:"spec"`
		Status status `json:"status"`
	}

	raw, err := json.Marshal(u.Object)
	if err != nil {
		return nil
	}
	var w wrapper
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil
	}

	out := &model.NPUSlicePool{
		Name:      u.GetName(),
		Namespace: u.GetNamespace(),
		Strategy:  w.Spec.Strategy,
	}
	if ref := w.Spec.NPUPoolRef["name"]; ref != "" {
		out.NPUPoolRef = ref
	}
	for _, tpl := range w.Spec.FixedTemplates {
		out.FixedTemplates = append(out.FixedTemplates, model.FixedTemplate{
			Name:        tpl.Name,
			AICoreCount: tpl.AICoreCount,
			MemoryMiB:   tpl.MemoryMiB,
		})
	}
	if w.Spec.DynamicSlicing != nil {
		out.DynamicSlicing = &model.DynamicSlicingPolicy{
			MinAICore:            w.Spec.DynamicSlicing.MinAICore,
			MaxAICore:            w.Spec.DynamicSlicing.MaxAICore,
			MemoryGranularityMiB: w.Spec.DynamicSlicing.MemoryGranularityMiB,
			AllowAggregation:     w.Spec.DynamicSlicing.AllowAggregation,
		}
	}
	if w.Status.TotalSlices > 0 || w.Status.AllocatedSlices > 0 || w.Status.AvailableSlices > 0 {
		out.Status = &model.NPUSlicePoolStatus{
			TotalSlices:     w.Status.TotalSlices,
			AllocatedSlices: w.Status.AllocatedSlices,
			AvailableSlices: w.Status.AvailableSlices,
		}
	}
	return out
}

// ListClusterPools is a concrete method (not on the Source interface)
// for the topology aggregator's future use. Returns the raw unstructured
// objects — the model package has no ClusterPool DTO yet (lands with
// P2-T-102 / Phase 3 aggregator wiring).
func (s *Source) ListClusterPools(ctx context.Context) ([]*unstructured.Unstructured, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	list, err := s.client.Resource(gvrClusterPools).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, translateAPIError(err)
	}
	return collectUnstructured(list), nil
}

// ListNodePools is the cluster-scoped NodePool reader.
func (s *Source) ListNodePools(ctx context.Context) ([]*unstructured.Unstructured, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	list, err := s.client.Resource(gvrNodePools).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, translateAPIError(err)
	}
	return collectUnstructured(list), nil
}

// ListNPUPools is the cluster-scoped NPUPool reader.
func (s *Source) ListNPUPools(ctx context.Context) ([]*unstructured.Unstructured, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	list, err := s.client.Resource(gvrNPUPools).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, translateAPIError(err)
	}
	return collectUnstructured(list), nil
}

// collectUnstructured pulls per-item pointers out of an
// UnstructuredList for callers that want to inspect / project later.
func collectUnstructured(list *unstructured.UnstructuredList) []*unstructured.Unstructured {
	if list == nil {
		return nil
	}
	out := make([]*unstructured.Unstructured, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, &list.Items[i])
	}
	return out
}
