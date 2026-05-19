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

// Package v1alpha1 contains Ocloud Ascend NPU semantic helpers that
// project to and from upstream resource.k8s.io/v1beta1 DRA types.
//
// Phase 4 T004 ships:
//   - AscendDevice (typed view of upstream Device.Basic.Attributes / Capacity
//     populated by the simulator-first publisher landing at P4-T-005)
//   - AscendClaimAnnotations (typed view of Ocloud-specific annotations on
//     upstream ResourceClaim objects, consumed by the claim controller skeleton
//     landing at P4-T-006)
//
// This package intentionally does not define new CRDs:
//
//   - ResourceSlice is owned upstream (resource.k8s.io/v1beta1.ResourceSlice).
//   - ResourceClaim is owned upstream (resource.k8s.io/v1beta1.ResourceClaim).
//   - NPUSlicePool / NPUPool / NodePool / ClusterPool are owned by the sibling
//     pool-operator project (operators/pool-operator/api/v1alpha1/).
//
// Cross-references:
//
//   - docs/adr/0001-phase0-key-decisions.md §5 v3 — dual-path roadmap (Edge:
//     Device Plugin v1 · Standard-K8s: DRA spike)
//   - docs/adr/0009-npu-dra-driver.md — slice ↔ ResourceClaim semantic mapping
//     (lands at P4-T-105)
//   - docs/phase4-plan.md §3 P4-T-004 — task acceptance criteria
//   - configs/mock-data/set-a-small/npus.json — simulator data source
//
// Ascend device attribute schema (set on upstream Device.Basic.Attributes):
//
//   - npu.huawei.com/index         (int)    physical NPU index 0..7
//   - npu.huawei.com/health        (string) Healthy / Unhealthy / Unknown
//     (mirrors Ascend Device Plugin label)
//   - npu.huawei.com/slice-strategy (string) FixedTemplate / Dynamic
//     (mirrors NPUSlicePool CRD pool-operator API)
//   - npu.huawei.com/ai-cores      (int)    slice AI-core count for Dynamic
//     strategy; 1..max per Ascend 910B chip topology
//   - npu.huawei.com/numa-node     (int)    host NUMA node, sourced from
//     Ascend Device Plugin labels
//   - npu.huawei.com/hccs-ring     (int)    Phase 6 placeholder, default 0
//     until scheduler-plugins NUMA+HCCS lands
//
// Ascend device capacity schema (set on upstream Device.Basic.Capacity):
//
//   - npu.huawei.com/slice-aicore  (resource.Quantity) per-device slice
//     capacity; pool-operator NPUSlicePool Reconcile reads this as the
//     upper bound (cross-controller integration in P4-T-102)
//
// Ascend ResourceClaim annotation schema (set on upstream ResourceClaim
// metadata.annotations):
//
//   - ocloud.edge.example.com/model-service-ref (string) for Phase 5
//     inference-operator binding (ADR-0008 PD Router webhook)
//   - ocloud.edge.example.com/preferred-pool    (string) soft pool affinity
//     to a NPUSlicePool by name (cluster-scoped reference in v1alpha1)
//
// +groupName=npu.ocloud.edge.example.com
// +kubebuilder:object:generate=true
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupVersion is the group/version used to register Ocloud helper types.
// Phase 4: no CRD published; the GroupVersion exists so future Ocloud CRDs
// (e.g. NPUSliceAllocation per arch §6.8, Phase 5) can land in this package
// without further plumbing.
var GroupVersion = schema.GroupVersion{
	Group:   "npu.ocloud.edge.example.com",
	Version: "v1alpha1",
}

// DriverName is the resource.k8s.io DriverName the npu-dra-driver publishes
// ResourceSlices under. Consumed by the simulator publisher (P4-T-005) and
// the pool-operator cross-watch filter (P4-T-102).
const DriverName = "npu.ocloud.edge.example.com"
