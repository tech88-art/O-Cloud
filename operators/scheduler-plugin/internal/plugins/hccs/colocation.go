/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package hccs

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// Pod label whose value (`<ns>/<name>`) groups sibling Pods of the same
// ModelService for HCCS co-location scoring (ADR-0010 §2 Score table).
// Source: operators/inference-operator/DESIGN.md §4.2 (PD Router webhook
// objectSelector key). Text-copied per operators/CLAUDE.md §1.
const ModelServiceLabel = "inference.ocloud.edge.example.com/model-service"

// NPUSliceAllocation cluster-scoped CRD GVR — text-copied from
// operators/npu-dra-driver/api/v1alpha1/types.go (GroupVersion +
// npusliceallocation_types.go shortname/kind). Per operators/CLAUDE.md §1
// no Go import — schema contract frozen by ADR-0009 §5 (Phase 5 audit CRD).
var npuSliceAllocationGVR = schema.GroupVersionResource{
	Group:    "npu.ocloud.edge.example.com",
	Version:  "v1alpha1",
	Resource: "npusliceallocations",
}

// AllocationPhaseAllocated mirrors operators/npu-dra-driver/api/v1alpha1.
// NPUSliceAllocationPhaseAllocated — the only phase Score treats as "this
// device is currently held by a sibling Pod".
const AllocationPhaseAllocated = "Allocated"

// SimpleAllocation is a local typed view of the fields Score reads off
// NPUSliceAllocation. Avoids cross-module Go import; extracted from
// unstructured.Unstructured at production lookup.
type SimpleAllocation struct {
	Name            string
	ModelServiceRef string
	NodeName        string
	Device          string
	Phase           string
}

// AllocationLister abstracts NPUSliceAllocation lookup so PreScore can be
// tested without a real dynamic client. Production wires the
// dynamicAllocationLister; tests inject a fake.
type AllocationLister interface {
	// ListByModelService returns NPUSliceAllocations with
	// spec.modelServiceRef == modelService AND status.phase == "Allocated".
	// Returns empty (not nil) on "no siblings yet".
	ListByModelService(modelService string) ([]*SimpleAllocation, error)
}

// dynamicAllocationLister is the production AllocationLister.
//
// Performance note: PreScore runs once per Pod scheduling cycle, so a
// per-cycle List() is acceptable for the Phase 6 simulator scope
// (single cluster, < 100 NPUSliceAllocations expected). Phase 9 multi-
// tenancy may need to upgrade to a DynamicSharedInformerFactory with a
// cached lister; cross-namespace indexer documented as a follow-up at
// ADR-0010 §3 ("performance" risk row).
type dynamicAllocationLister struct {
	client dynamic.Interface
}

// ListByModelService implements AllocationLister via a single dynamic
// List call + post-filtering on spec.modelServiceRef + status.phase.
func (l *dynamicAllocationLister) ListByModelService(modelService string) ([]*SimpleAllocation, error) {
	list, err := l.client.Resource(npuSliceAllocationGVR).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]*SimpleAllocation, 0, len(list.Items))
	for i := range list.Items {
		sa := extractAllocation(&list.Items[i])
		if sa.ModelServiceRef != modelService {
			continue
		}
		if sa.Phase != AllocationPhaseAllocated {
			continue
		}
		out = append(out, sa)
	}
	return out, nil
}

// extractAllocation pulls the SimpleAllocation fields from an
// unstructured.Unstructured. Missing fields → zero values; the caller's
// post-filter (Phase != Allocated, ModelServiceRef mismatch) handles the
// "not yet ready" cases.
func extractAllocation(u *unstructured.Unstructured) *SimpleAllocation {
	sa := &SimpleAllocation{Name: u.GetName()}
	sa.ModelServiceRef, _, _ = unstructured.NestedString(u.Object, "spec", "modelServiceRef")
	sa.NodeName, _, _ = unstructured.NestedString(u.Object, "spec", "nodeName")
	sa.Device, _, _ = unstructured.NestedString(u.Object, "spec", "sliceRef", "device")
	sa.Phase, _, _ = unstructured.NestedString(u.Object, "status", "phase")
	return sa
}
