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

package integration_test

import (
	v1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hccspkg "github.com/tech88-art/O-Cloud/operators/scheduler-plugin/internal/plugins/hccs"
)

// fakeSliceLister satisfies hccspkg.SliceLister for the integration suite.
// Mirrors the same-named test helper in hccs package, but lives here so
// integration_test can construct it without exporting the in-package one.
type fakeSliceLister struct {
	byNode map[string][]*resourceapi.ResourceSlice
}

func newFakeSliceLister() *fakeSliceLister {
	return &fakeSliceLister{byNode: map[string][]*resourceapi.ResourceSlice{}}
}

// ListForNode implements hccspkg.SliceLister.
func (f *fakeSliceLister) ListForNode(nodeName string) ([]*resourceapi.ResourceSlice, error) {
	return f.byNode[nodeName], nil
}

func (f *fakeSliceLister) addSlice(nodeName string, devices ...resourceapi.Device) {
	slice := &resourceapi.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "integration-slice-" + nodeName,
			Labels: map[string]string{
				"npu.ocloud.edge.example.com/managed-by": "npu-dra-driver",
			},
		},
		Spec: resourceapi.ResourceSliceSpec{
			Driver:   "npu.ocloud.edge.example.com",
			NodeName: nodeName,
			Devices:  devices,
		},
	}
	f.byNode[nodeName] = append(f.byNode[nodeName], slice)
}

// fakeAllocationLister satisfies hccspkg.AllocationLister.
type fakeAllocationLister struct {
	byMS map[string][]*hccspkg.SimpleAllocation
}

func newFakeAllocationLister() *fakeAllocationLister {
	return &fakeAllocationLister{byMS: map[string][]*hccspkg.SimpleAllocation{}}
}

// ListByModelService implements hccspkg.AllocationLister.
func (f *fakeAllocationLister) ListByModelService(ms string) ([]*hccspkg.SimpleAllocation, error) {
	return f.byMS[ms], nil
}

func (f *fakeAllocationLister) addAllocation(ms, nodeName, device string) {
	f.byMS[ms] = append(f.byMS[ms], &hccspkg.SimpleAllocation{
		Name:            "integ-alloc-" + device,
		ModelServiceRef: ms,
		NodeName:        nodeName,
		Device:          device,
		Phase:           hccspkg.AllocationPhaseAllocated,
	})
}

func (f *fakeAllocationLister) reset() {
	f.byMS = map[string][]*hccspkg.SimpleAllocation{}
}

// makeDevice builds a ResourceSlice device with hccs_ring + health
// attributes, mirroring the npu-dra-driver publisher output.
func makeDevice(name string, ring int64, health string) resourceapi.Device {
	ringCopy := ring
	healthCopy := health
	return resourceapi.Device{
		Name: name,
		Basic: &resourceapi.BasicDevice{
			Attributes: map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{
				"npu.huawei.com/hccs_ring": {IntValue: &ringCopy},
				"npu.huawei.com/health":    {StringValue: &healthCopy},
			},
		},
	}
}

// podBuilder is a small fluent builder for v1.Pod fixtures.
type podBuilderT struct {
	pod *v1.Pod
}

func podBuilder() *podBuilderT {
	return &podBuilderT{
		pod: &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "integ-pod",
				Namespace: "default",
			},
		},
	}
}

func (b *podBuilderT) withMS(ms string) *podBuilderT {
	if b.pod.Labels == nil {
		b.pod.Labels = map[string]string{}
	}
	b.pod.Labels[hccspkg.ModelServiceLabel] = ms
	return b
}

func (b *podBuilderT) withRingAnnotation(rings string) *podBuilderT {
	if b.pod.Annotations == nil {
		b.pod.Annotations = map[string]string{}
	}
	b.pod.Annotations[hccspkg.DefaultPreferAnnotation] = rings
	return b
}

func (b *podBuilderT) Build() *v1.Pod {
	return b.pod
}
