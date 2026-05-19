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

package controller

import (
	resourceapi "k8s.io/api/resource/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	inferencev1alpha1 "github.com/tech88-art/O-Cloud/operators/inference-operator/api/v1alpha1"
)

// NPUDeviceClassName is the bare DeviceClass name registered by the
// npu-dra-driver helm chart (P5-T-001). The claim builder references
// it as a string constant — operators/CLAUDE.md §1 forbids importing
// the npu-dra-driver Go module, so duplication is the convention.
const NPUDeviceClassName = "npu.ocloud.edge.example.com"

// Annotation keys mirrored from
// operators/npu-dra-driver/api/v1alpha1/resourceclaim_types.go. Same
// rationale as NPUDeviceClassName — text duplication over cross-module
// Go import.
const (
	AnnotationPreferredPool   = "ocloud.edge.example.com/preferred-pool"
	AnnotationModelServiceRef = "ocloud.edge.example.com/model-service-ref"
)

// buildResourceClaimTemplate returns the desired
// resource.k8s.io/v1beta1.ResourceClaimTemplate for one side of the
// PD pair. K8s creates a per-replica ResourceClaim from this template
// on Pod scheduling — satisfying the plan's "per-replica
// ResourceClaims created" acceptance via the standard DRA pattern.
//
// Annotations on the embedded ObjectMeta propagate to the created
// claims (per upstream contract: "ObjectMeta may contain labels and
// annotations that will be copied into the ResourceClaim when
// creating it"). The npu-dra-driver claim controller (T002) reads
// these annotations during allocation.
func buildResourceClaimTemplate(ms *inferencev1alpha1.ModelService, side PDSide) *resourceapi.ResourceClaimTemplate {
	msRef := ms.Namespace + "/" + ms.Name
	roleVal := string(side)

	return &resourceapi.ResourceClaimTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      claimTemplateName(ms, side),
			Namespace: ms.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "modelservice",
				"app.kubernetes.io/instance":  ms.Name,
				"app.kubernetes.io/component": roleVal,
				LabelModelService:             msRef,
			},
		},
		Spec: resourceapi.ResourceClaimTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					AnnotationModelServiceRef: msRef,
					AnnotationPreferredPool:   ms.Spec.NPUSlicePoolRef.Name,
				},
				Labels: map[string]string{
					LabelModelService:             msRef,
					"app.kubernetes.io/component": roleVal,
				},
			},
			Spec: resourceapi.ResourceClaimSpec{
				Devices: resourceapi.DeviceClaim{
					Requests: []resourceapi.DeviceRequest{{
						Name:            "req-0",
						DeviceClassName: NPUDeviceClassName,
					}},
				},
			},
		},
	}
}
