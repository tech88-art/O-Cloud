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

package controller_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	imsv1alpha1 "github.com/example/ocloud-edge/operators/pool-operator/api/v1alpha1"
	"github.com/example/ocloud-edge/operators/pool-operator/internal/controller"
)

// makeResourceSlice creates a cluster-scoped resource.k8s.io/v1beta1.ResourceSlice
// owned by the given driver, attached to a node. Used by the P4-T-102
// cross-controller-awareness specs below.
func makeResourceSlice(ctx context.Context, k8sClient client.Client, name, nodeName, driverName string, deviceCount int) *resourceapi.ResourceSlice {
	devices := make([]resourceapi.Device, deviceCount)
	for i := range devices {
		devices[i] = resourceapi.Device{
			Name: fmt.Sprintf("%s-dev-%d", name, i),
			Basic: &resourceapi.BasicDevice{
				Attributes: map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{},
			},
		}
	}
	sl := &resourceapi.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: resourceapi.ResourceSliceSpec{
			Driver: driverName,
			Pool: resourceapi.ResourcePool{
				Name:               nodeName,
				Generation:         1,
				ResourceSliceCount: 1,
			},
			NodeName: nodeName,
			Devices:  devices,
		},
	}
	Expect(k8sClient.Create(ctx, sl)).To(Succeed())
	return sl
}

// nsCounter gives each spec a unique namespace name without bringing in a
// uuid dependency. time.Now().UnixNano() is enough resolution because Ginkgo
// runs specs serially within one process.
var nsCounter int64

func uniqueNS() string {
	nsCounter++
	return fmt.Sprintf("ts-%d-%d", time.Now().UnixNano(), nsCounter)
}

// uniqueName builds a unique-per-spec object name (cluster-scoped NPUPool /
// Node names need to avoid collisions across specs since envtest is shared).
func uniqueName(prefix string) string {
	nsCounter++
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), nsCounter)
}

// makeNPUNode creates a Node with `huawei.com/Ascend910` capacity set to
// npuCount and applies the supplied labels. Status is updated via the status
// subresource so envtest persists the capacity (envtest does not run kubelet
// and would otherwise drop Status on Create).
func makeNPUNode(ctx context.Context, k8sClient client.Client, name string, npuCount int, labels map[string]string) *corev1.Node {
	GinkgoHelper()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
	Expect(k8sClient.Create(ctx, node)).To(Succeed())
	// Re-fetch so the .Status() Update has the correct ResourceVersion.
	Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, node)).To(Succeed())
	node.Status.Capacity = corev1.ResourceList{
		corev1.ResourceName("huawei.com/Ascend910"): resource.MustParse(fmt.Sprintf("%d", npuCount)),
	}
	Expect(k8sClient.Status().Update(ctx, node)).To(Succeed())
	return node
}

// findCondition returns a pointer to the condition matching condType, or nil.
func findCondition(conds []metav1.Condition, condType string) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == condType {
			return &conds[i]
		}
	}
	return nil
}

// reconcileUntilStable runs the reconciler in a tight loop until it returns
// Result{Requeue: false}; bounded to maxIter passes to avoid infinite spin
// in case of a controller bug. Returns the last Result for inspection.
func reconcileUntilStable(ctx context.Context, r *controller.NPUSlicePoolReconciler, key types.NamespacedName, maxIter int) ctrl.Result {
	GinkgoHelper()
	var last ctrl.Result
	for i := 0; i < maxIter; i++ {
		var err error
		last, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		if !last.Requeue && last.RequeueAfter == 0 {
			return last
		}
	}
	Fail(fmt.Sprintf("reconcile did not stabilise within %d passes", maxIter))
	return last
}

var _ = Describe("NPUSlicePool Reconcile", func() {
	var (
		ctx        context.Context
		reconciler *controller.NPUSlicePoolReconciler
		ns         string
	)

	BeforeEach(func() {
		ctx = context.Background()
		reconciler = &controller.NPUSlicePoolReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
		ns = uniqueNS()
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns},
		})).To(Succeed())
	})

	It("FixedTemplate: computes totalSlices from template * npuCount", func() {
		// Setup: 4 nodes each with 1 NPU -> npuCount=4
		nodeLabel := map[string]string{"pool": "happy-fixed"}
		for i := 0; i < 4; i++ {
			makeNPUNode(ctx, k8sClient, uniqueName("node-fixed"), 1, nodeLabel)
		}

		// Cluster-scoped NPUPool selecting those nodes
		npuPoolName := uniqueName("npupool-fixed")
		npuPool := &imsv1alpha1.NPUPool{
			ObjectMeta: metav1.ObjectMeta{Name: npuPoolName},
			Spec: imsv1alpha1.NPUPoolSpec{
				NodePoolRef:   corev1.LocalObjectReference{Name: "test-nodepool"},
				NPUModel:      "Ascend910B",
				Selector:      &metav1.LabelSelector{MatchLabels: nodeLabel},
				SliceStrategy: "happy-fixed",
			},
		}
		Expect(k8sClient.Create(ctx, npuPool)).To(Succeed())

		// NPUSlicePool with FixedTemplate vir04 (4 cores/slice => 32/4 = 8 slices per NPU)
		slicePool := &imsv1alpha1.NPUSlicePool{
			ObjectMeta: metav1.ObjectMeta{Name: "happy-fixed", Namespace: ns},
			Spec: imsv1alpha1.NPUSlicePoolSpec{
				NPUPoolRef: corev1.LocalObjectReference{Name: npuPoolName},
				Strategy:   imsv1alpha1.SliceStrategyFixedTemplate,
				FixedTemplates: []imsv1alpha1.SliceTemplate{
					{Name: "vir04", AICoreCount: 4, MemoryMiB: 16384},
				},
			},
		}
		Expect(k8sClient.Create(ctx, slicePool)).To(Succeed())

		key := types.NamespacedName{Name: slicePool.Name, Namespace: ns}
		reconcileUntilStable(ctx, reconciler, key, 5)

		var got imsv1alpha1.NPUSlicePool
		Expect(k8sClient.Get(ctx, key, &got)).To(Succeed())
		Expect(got.Status.TotalSlices).To(Equal(int32(32)),
			"4 NPUs * 8 slices/NPU (32 cores / vir04 4-core) = 32 slices")
		Expect(got.Status.AvailableSlices).To(Equal(int32(32)),
			"AllocatedSlices is 0 in Phase 3 so Available == Total")

		readyCond := findCondition(got.Status.Conditions, "Ready")
		Expect(readyCond).NotTo(BeNil())
		Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		Expect(readyCond.Reason).To(Equal("Reconciled"))
		Expect(got.Finalizers).To(ContainElement("pool.ocloud.edge.example.com/npuslicepool-cleanup"))
	})

	It("Dynamic: computes totalSlices from minAICore * npuCount", func() {
		nodeLabel := map[string]string{"pool": "happy-dynamic"}
		for i := 0; i < 4; i++ {
			makeNPUNode(ctx, k8sClient, uniqueName("node-dyn"), 1, nodeLabel)
		}

		npuPoolName := uniqueName("npupool-dyn")
		Expect(k8sClient.Create(ctx, &imsv1alpha1.NPUPool{
			ObjectMeta: metav1.ObjectMeta{Name: npuPoolName},
			Spec: imsv1alpha1.NPUPoolSpec{
				NodePoolRef:   corev1.LocalObjectReference{Name: "test-nodepool"},
				NPUModel:      "Ascend910B",
				Selector:      &metav1.LabelSelector{MatchLabels: nodeLabel},
				SliceStrategy: "happy-dynamic",
			},
		})).To(Succeed())

		slicePool := &imsv1alpha1.NPUSlicePool{
			ObjectMeta: metav1.ObjectMeta{Name: "happy-dyn", Namespace: ns},
			Spec: imsv1alpha1.NPUSlicePoolSpec{
				NPUPoolRef: corev1.LocalObjectReference{Name: npuPoolName},
				Strategy:   imsv1alpha1.SliceStrategyDynamic,
				DynamicSlicing: &imsv1alpha1.DynamicSlicingSpec{
					MinAICore:            8,
					MaxAICore:            32,
					MemoryGranularityMiB: 1024,
				},
			},
		}
		Expect(k8sClient.Create(ctx, slicePool)).To(Succeed())

		key := types.NamespacedName{Name: slicePool.Name, Namespace: ns}
		reconcileUntilStable(ctx, reconciler, key, 5)

		var got imsv1alpha1.NPUSlicePool
		Expect(k8sClient.Get(ctx, key, &got)).To(Succeed())
		Expect(got.Status.TotalSlices).To(Equal(int32(16)),
			"floor(32/8) = 4 slices/NPU * 4 NPUs = 16")
		readyCond := findCondition(got.Status.Conditions, "Ready")
		Expect(readyCond).NotTo(BeNil())
		Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
	})

	It("missing NPUPool: sets Ready=False with Reason=NPUPoolNotFound", func() {
		slicePool := &imsv1alpha1.NPUSlicePool{
			ObjectMeta: metav1.ObjectMeta{Name: "orphan", Namespace: ns},
			Spec: imsv1alpha1.NPUSlicePoolSpec{
				NPUPoolRef: corev1.LocalObjectReference{Name: "does-not-exist"},
				Strategy:   imsv1alpha1.SliceStrategyFixedTemplate,
				FixedTemplates: []imsv1alpha1.SliceTemplate{
					{Name: "vir04", AICoreCount: 4, MemoryMiB: 16384},
				},
			},
		}
		Expect(k8sClient.Create(ctx, slicePool)).To(Succeed())

		key := types.NamespacedName{Name: slicePool.Name, Namespace: ns}
		reconcileUntilStable(ctx, reconciler, key, 5)

		var got imsv1alpha1.NPUSlicePool
		Expect(k8sClient.Get(ctx, key, &got)).To(Succeed())
		readyCond := findCondition(got.Status.Conditions, "Ready")
		Expect(readyCond).NotTo(BeNil())
		Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
		Expect(readyCond.Reason).To(Equal("NPUPoolNotFound"))
		Expect(got.Status.TotalSlices).To(Equal(int32(0)))
	})

	It("malformed: FixedTemplate with empty templates -> Reason=InvalidStrategy", func() {
		// We need a real parent NPUPool plus at least one NPU so that the
		// strategy validation in computeTotalSlices is actually exercised
		// (a 0-NPU shortcut would mask the error).
		nodeLabel := map[string]string{"pool": "malformed-fixed"}
		makeNPUNode(ctx, k8sClient, uniqueName("node-malformed"), 1, nodeLabel)

		npuPoolName := uniqueName("npupool-malformed")
		Expect(k8sClient.Create(ctx, &imsv1alpha1.NPUPool{
			ObjectMeta: metav1.ObjectMeta{Name: npuPoolName},
			Spec: imsv1alpha1.NPUPoolSpec{
				NodePoolRef:   corev1.LocalObjectReference{Name: "test-nodepool"},
				NPUModel:      "Ascend910B",
				Selector:      &metav1.LabelSelector{MatchLabels: nodeLabel},
				SliceStrategy: "malformed-fixed",
			},
		})).To(Succeed())

		slicePool := &imsv1alpha1.NPUSlicePool{
			ObjectMeta: metav1.ObjectMeta{Name: "malformed", Namespace: ns},
			Spec: imsv1alpha1.NPUSlicePoolSpec{
				NPUPoolRef:     corev1.LocalObjectReference{Name: npuPoolName},
				Strategy:       imsv1alpha1.SliceStrategyFixedTemplate,
				FixedTemplates: nil, // <-- the bug under test
			},
		}
		Expect(k8sClient.Create(ctx, slicePool)).To(Succeed())

		key := types.NamespacedName{Name: slicePool.Name, Namespace: ns}
		reconcileUntilStable(ctx, reconciler, key, 5)

		var got imsv1alpha1.NPUSlicePool
		Expect(k8sClient.Get(ctx, key, &got)).To(Succeed())
		readyCond := findCondition(got.Status.Conditions, "Ready")
		Expect(readyCond).NotTo(BeNil())
		Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
		Expect(readyCond.Reason).To(Equal("InvalidStrategy"))
	})

	It("finalizer lifecycle: added on create, removed on delete", func() {
		// Minimal happy fixture so the reconciler reaches the finalizer-add
		// path on the first pass and the status-update path on the second.
		nodeLabel := map[string]string{"pool": "lifecycle"}
		makeNPUNode(ctx, k8sClient, uniqueName("node-lc"), 1, nodeLabel)

		npuPoolName := uniqueName("npupool-lc")
		Expect(k8sClient.Create(ctx, &imsv1alpha1.NPUPool{
			ObjectMeta: metav1.ObjectMeta{Name: npuPoolName},
			Spec: imsv1alpha1.NPUPoolSpec{
				NodePoolRef:   corev1.LocalObjectReference{Name: "test-nodepool"},
				NPUModel:      "Ascend910B",
				Selector:      &metav1.LabelSelector{MatchLabels: nodeLabel},
				SliceStrategy: "lifecycle",
			},
		})).To(Succeed())

		slicePool := &imsv1alpha1.NPUSlicePool{
			ObjectMeta: metav1.ObjectMeta{Name: "lifecycle", Namespace: ns},
			Spec: imsv1alpha1.NPUSlicePoolSpec{
				NPUPoolRef: corev1.LocalObjectReference{Name: npuPoolName},
				Strategy:   imsv1alpha1.SliceStrategyDynamic,
				DynamicSlicing: &imsv1alpha1.DynamicSlicingSpec{
					MinAICore:            4,
					MaxAICore:            32,
					MemoryGranularityMiB: 1024,
				},
			},
		}
		Expect(k8sClient.Create(ctx, slicePool)).To(Succeed())

		key := types.NamespacedName{Name: slicePool.Name, Namespace: ns}
		// First reconcile pass: should add the finalizer and signal Requeue.
		res, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Requeue).To(BeTrue(), "first pass should requeue after adding finalizer")

		var afterAdd imsv1alpha1.NPUSlicePool
		Expect(k8sClient.Get(ctx, key, &afterAdd)).To(Succeed())
		Expect(afterAdd.Finalizers).To(ContainElement("pool.ocloud.edge.example.com/npuslicepool-cleanup"))

		// Drive a few more passes to settle status (and confirm idempotency).
		reconcileUntilStable(ctx, reconciler, key, 5)

		// Delete -> finalizer must be drained, object eventually disappears.
		Expect(k8sClient.Delete(ctx, &afterAdd)).To(Succeed())

		// A pending deletion shows up with a DeletionTimestamp; reconcile
		// removes the finalizer and the object is GCed.
		var pending imsv1alpha1.NPUSlicePool
		Expect(k8sClient.Get(ctx, key, &pending)).To(Succeed())
		Expect(pending.DeletionTimestamp).NotTo(BeNil())
		Expect(pending.Finalizers).To(ContainElement("pool.ocloud.edge.example.com/npuslicepool-cleanup"),
			"finalizer must still be present right after Delete since reconciler has not run yet")

		_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())

		var afterDelete imsv1alpha1.NPUSlicePool
		err = k8sClient.Get(ctx, key, &afterDelete)
		Expect(apierrors.IsNotFound(err)).To(BeTrue(),
			"object should be GCed by the apiserver once the last finalizer is removed; got err=%v", err)
	})
})

// =============================================================================
// P4-T-102: NPUSlicePool ↔ ResourceSlice cross-controller observability smoke
// =============================================================================

var _ = Describe("NPUSlicePool ResourceSlice cross-observation (P4-T-102)", func() {
	var (
		ctx        context.Context
		reconciler *controller.NPUSlicePoolReconciler
		ns         string
	)

	BeforeEach(func() {
		ctx = context.Background()
		reconciler = &controller.NPUSlicePoolReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
		ns = uniqueNS()
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns},
		})).To(Succeed())
	})

	makeSlicePoolWithOneNode := func(slicePoolName, nodeLabelValue string) types.NamespacedName {
		nodeLabel := map[string]string{"pool": nodeLabelValue}
		makeNPUNode(ctx, k8sClient, uniqueName("node-csw"), 1, nodeLabel)
		npuPoolName := uniqueName("npupool-csw")
		Expect(k8sClient.Create(ctx, &imsv1alpha1.NPUPool{
			ObjectMeta: metav1.ObjectMeta{Name: npuPoolName},
			Spec: imsv1alpha1.NPUPoolSpec{
				NodePoolRef:   corev1.LocalObjectReference{Name: "test-nodepool"},
				NPUModel:      "Ascend910B",
				Selector:      &metav1.LabelSelector{MatchLabels: nodeLabel},
				SliceStrategy: nodeLabelValue,
			},
		})).To(Succeed())
		slicePool := &imsv1alpha1.NPUSlicePool{
			ObjectMeta: metav1.ObjectMeta{Name: slicePoolName, Namespace: ns},
			Spec: imsv1alpha1.NPUSlicePoolSpec{
				NPUPoolRef: corev1.LocalObjectReference{Name: npuPoolName},
				Strategy:   imsv1alpha1.SliceStrategyFixedTemplate,
				FixedTemplates: []imsv1alpha1.SliceTemplate{
					{Name: "vir04", AICoreCount: 4, MemoryMiB: 16384},
				},
			},
		}
		Expect(k8sClient.Create(ctx, slicePool)).To(Succeed())
		return types.NamespacedName{Name: slicePool.Name, Namespace: ns}
	}

	It("Happy: 1 pool + 2 matching ResourceSlices -> status.resourceSlicesObserved == 2", func() {
		key := makeSlicePoolWithOneNode("happy-csw", "happy-csw")

		// Publish 2 ResourceSlices owned by npu-dra-driver
		makeResourceSlice(ctx, k8sClient, uniqueName("npu-dra-slice"), "node-a", "npu.ocloud.edge.example.com", 8)
		makeResourceSlice(ctx, k8sClient, uniqueName("npu-dra-slice"), "node-b", "npu.ocloud.edge.example.com", 8)

		reconcileUntilStable(ctx, reconciler, key, 5)
		var got imsv1alpha1.NPUSlicePool
		Expect(k8sClient.Get(ctx, key, &got)).To(Succeed())
		Expect(got.Status.ResourceSlicesObserved).To(Equal(int32(2)),
			"both ResourceSlices owned by npu-dra-driver should be counted")
	})

	It("No slices: status.resourceSlicesObserved == 0 (no panic)", func() {
		key := makeSlicePoolWithOneNode("empty-csw", "empty-csw")
		// No ResourceSlices created.

		reconcileUntilStable(ctx, reconciler, key, 5)
		var got imsv1alpha1.NPUSlicePool
		Expect(k8sClient.Get(ctx, key, &got)).To(Succeed())
		Expect(got.Status.ResourceSlicesObserved).To(Equal(int32(0)),
			"with zero ResourceSlices, the observed count must be 0 (not nil, not panic)")
	})

	It("Multiple drivers: only npu.ocloud.edge.example.com slices counted", func() {
		key := makeSlicePoolWithOneNode("multidriver-csw", "multidriver-csw")

		// 1 slice from npu-dra-driver — should count
		makeResourceSlice(ctx, k8sClient, uniqueName("npu-dra-slice"), "node-our", "npu.ocloud.edge.example.com", 8)
		// 2 slices from foreign drivers — must NOT count
		makeResourceSlice(ctx, k8sClient, uniqueName("nv-gpu-slice"), "node-gpu", "gpu.nvidia.com", 4)
		makeResourceSlice(ctx, k8sClient, uniqueName("amd-gpu-slice"), "node-amd", "gpu.amd.com", 2)

		reconcileUntilStable(ctx, reconciler, key, 5)
		var got imsv1alpha1.NPUSlicePool
		Expect(k8sClient.Get(ctx, key, &got)).To(Succeed())
		Expect(got.Status.ResourceSlicesObserved).To(Equal(int32(1)),
			"foreign-driver ResourceSlices must NOT inflate the npu-dra-driver count")
	})
})
