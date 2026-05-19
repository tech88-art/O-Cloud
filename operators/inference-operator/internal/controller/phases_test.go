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
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	inferencev1alpha1 "github.com/tech88-art/O-Cloud/operators/inference-operator/api/v1alpha1"
)

// TestComputePhase_TableDriven exercises the pure phase machine across
// representative inputs. Drives the plan-acceptance cases for T008.
func TestComputePhase_TableDriven(t *testing.T) {
	now := time.Now()

	cases := []struct {
		name      string
		in        PhaseInputs
		last      *inferencev1alpha1.ModelServiceStatus
		wantPhase inferencev1alpha1.ModelServicePhase
		wantAvail metav1.ConditionStatus
		wantPD    metav1.ConditionStatus
	}{
		{
			name: "Pending → Provisioning (no replicas ready yet)",
			in: PhaseInputs{
				PrefillDesired: 1, PrefillReady: 0,
				DecodeDesired: 1, DecodeReady: 0,
				ClaimsTotal: 0, ClaimsAllocated: 0,
				Now: now,
			},
			last:      nil,
			wantPhase: inferencev1alpha1.PhaseProvisioning,
			wantAvail: metav1.ConditionFalse,
			wantPD:    metav1.ConditionFalse,
		},
		{
			name: "Provisioning → Ready (all replicas + claims ready)",
			in: PhaseInputs{
				PrefillDesired: 2, PrefillReady: 2,
				DecodeDesired: 1, DecodeReady: 1,
				ClaimsTotal: 3, ClaimsAllocated: 3,
				Now: now,
			},
			last:      &inferencev1alpha1.ModelServiceStatus{Phase: inferencev1alpha1.PhaseProvisioning},
			wantPhase: inferencev1alpha1.PhaseReady,
			wantAvail: metav1.ConditionTrue,
			wantPD:    metav1.ConditionFalse,
		},
		{
			name: "Ready → Provisioning (scale up: 2→4 prefill replicas)",
			in: PhaseInputs{
				PrefillDesired: 4, PrefillReady: 2,
				DecodeDesired: 1, DecodeReady: 1,
				ClaimsTotal: 5, ClaimsAllocated: 5,
				Now: now,
			},
			last:      &inferencev1alpha1.ModelServiceStatus{Phase: inferencev1alpha1.PhaseReady},
			wantPhase: inferencev1alpha1.PhaseProvisioning,
			wantAvail: metav1.ConditionFalse,
			wantPD:    metav1.ConditionFalse,
		},
		{
			name: "Ready → Failed (image-pull error)",
			in: PhaseInputs{
				PrefillDesired: 1, PrefillReady: 0,
				DecodeDesired: 1, DecodeReady: 1,
				PrefillProgressing: &appsv1.DeploymentCondition{
					Type:    appsv1.DeploymentProgressing,
					Status:  corev1.ConditionFalse,
					Reason:  "ImagePullBackOff",
					Message: "image pull back-off",
				},
				ClaimsTotal: 2, ClaimsAllocated: 2,
				Now: now,
			},
			last:      &inferencev1alpha1.ModelServiceStatus{Phase: inferencev1alpha1.PhaseReady},
			wantPhase: inferencev1alpha1.PhaseFailed,
			wantAvail: metav1.ConditionFalse,
			wantPD:    metav1.ConditionFalse,
		},
		{
			name: "Provisioning → Failed (progress deadline exceeded)",
			in: PhaseInputs{
				PrefillDesired: 1, PrefillReady: 0,
				DecodeDesired: 1, DecodeReady: 0,
				ClaimsTotal: 0, ClaimsAllocated: 0,
				Now: now,
			},
			last: &inferencev1alpha1.ModelServiceStatus{
				Phase: inferencev1alpha1.PhaseProvisioning,
				Conditions: []metav1.Condition{{
					Type:               ConditionAvailable,
					Status:             metav1.ConditionFalse,
					Reason:             reasonProvisioningInProgress,
					LastTransitionTime: metav1.NewTime(now.Add(-15 * time.Minute)),
				}},
			},
			wantPhase: inferencev1alpha1.PhaseFailed,
			wantAvail: metav1.ConditionFalse,
			wantPD:    metav1.ConditionTrue,
		},
		{
			name: "AllocationReady=True only when ClaimsTotal>0 AND all allocated",
			in: PhaseInputs{
				PrefillDesired: 1, PrefillReady: 1,
				DecodeDesired: 1, DecodeReady: 1,
				ClaimsTotal: 2, ClaimsAllocated: 1, // one short
				Now: now,
			},
			last:      &inferencev1alpha1.ModelServiceStatus{Phase: inferencev1alpha1.PhaseProvisioning},
			wantPhase: inferencev1alpha1.PhaseProvisioning,
			wantAvail: metav1.ConditionFalse,
			wantPD:    metav1.ConditionFalse,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := computePhase(tc.in, tc.last)
			if got.Phase != tc.wantPhase {
				t.Errorf("phase: want %s, got %s", tc.wantPhase, got.Phase)
			}
			availC, ok := FindCondition(got.Conditions, ConditionAvailable)
			if !ok {
				t.Fatalf("missing Available condition")
			}
			if availC.Status != tc.wantAvail {
				t.Errorf("Available status: want %v, got %v (%s)", tc.wantAvail, availC.Status, availC.Reason)
			}
			pdC, ok := FindCondition(got.Conditions, "ProgressDeadline")
			if !ok {
				t.Fatalf("missing ProgressDeadline condition")
			}
			if pdC.Status != tc.wantPD {
				t.Errorf("ProgressDeadline status: want %v, got %v (%s)", tc.wantPD, pdC.Status, pdC.Reason)
			}
		})
	}
}

func TestDetectImagePullError(t *testing.T) {
	if msg := detectImagePullError(nil, nil); msg != "" {
		t.Errorf("nil inputs should not detect error; got %q", msg)
	}
	good := &appsv1.DeploymentCondition{
		Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue,
		Reason: "NewReplicaSetCreated",
	}
	if msg := detectImagePullError(good, nil); msg != "" {
		t.Errorf("healthy progressing should not detect error; got %q", msg)
	}
	bad := &appsv1.DeploymentCondition{
		Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse,
		Reason: "ImagePullBackOff", Message: "image pull back-off",
	}
	if msg := detectImagePullError(nil, bad); msg == "" {
		t.Errorf("ImagePullBackOff should detect error")
	}
	errImg := &appsv1.DeploymentCondition{
		Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse,
		Reason: "ErrImagePull", Message: "registry unreachable",
	}
	if msg := detectImagePullError(errImg, nil); msg == "" {
		t.Errorf("ErrImagePull should detect error")
	}
}

func TestCountClaimsAllocated(t *testing.T) {
	ms := newModelService("ms-c", "ns-c", "pool-c")
	msRef := "ns-c/ms-c"

	// Build claim fixtures via the helper.
	claims := []resourceapi.ResourceClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "c1",
				Namespace: "ns-c",
				Labels:    map[string]string{LabelModelService: msRef},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "c2",
				Namespace: "ns-c",
				Labels:    map[string]string{LabelModelService: msRef},
			},
			Status: resourceapi.ResourceClaimStatus{
				Allocation: &resourceapi.AllocationResult{
					Devices: resourceapi.DeviceAllocationResult{
						Results: []resourceapi.DeviceRequestAllocationResult{{
							Driver: NPUDeviceClassName, Pool: "nodeA", Device: "nodeA-npu-0",
						}},
					},
				},
			},
		},
		{
			// foreign claim — different model-service
			ObjectMeta: metav1.ObjectMeta{
				Name:      "c-foreign",
				Namespace: "ns-c",
				Labels:    map[string]string{LabelModelService: "other-ms"},
			},
			Status: resourceapi.ResourceClaimStatus{
				Allocation: &resourceapi.AllocationResult{
					Devices: resourceapi.DeviceAllocationResult{
						Results: []resourceapi.DeviceRequestAllocationResult{{
							Driver: NPUDeviceClassName, Pool: "nodeB", Device: "nodeB-npu-0",
						}},
					},
				},
			},
		},
	}
	total, allocated := countClaimsAllocated(ms, claims)
	if total != 2 || allocated != 1 {
		t.Errorf("counts: want total=2 allocated=1, got total=%d allocated=%d", total, allocated)
	}
}
