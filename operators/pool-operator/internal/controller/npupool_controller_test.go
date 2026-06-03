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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	imsv1alpha1 "github.com/example/ocloud-edge/operators/pool-operator/api/v1alpha1"
	"github.com/example/ocloud-edge/operators/pool-operator/internal/controller"
)

// deviceFixture is a minimal description of a ResourceSlice device entry used
// by Phase 6 T003 HCCS aggregation tests. Mirrors the shape npu-dra-driver's
// publisher.buildSlice writes (operators/npu-dra-driver/internal/publisher/
// publisher.go) but constructs it directly from typed resourceapi to avoid
// cross-module Go import.
type deviceFixture struct {
	name   string
	ring   int64
	health string // "Healthy" / "Unhealthy" / "Unknown"
}

// makeHCCSResourceSlice creates a ResourceSlice labelled managed-by=npu-dra-driver
// pinned to nodeName, with the given device entries. Each device carries the
// npu.huawei.com/hccs_ring (int) + npu.huawei.com/health (string) attributes
// the NPUPool aggregator reads.
func makeHCCSResourceSlice(ctx context.Context, nodeName string, devs []deviceFixture) *resourceapi.ResourceSlice {
	GinkgoHelper()
	var devices []resourceapi.Device
	for _, d := range devs {
		ringCopy := d.ring
		healthCopy := d.health
		devices = append(devices, resourceapi.Device{
			Name: d.name,
			Basic: &resourceapi.BasicDevice{
				Attributes: map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{
					"npu.huawei.com/hccs_ring": {IntValue: &ringCopy},
					"npu.huawei.com/health":    {StringValue: &healthCopy},
				},
			},
		})
	}
	slice := &resourceapi.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "npu-dra-" + nodeName + "-" + uuid.NewString()[:6],
			Labels: map[string]string{
				"npu.ocloud.edge.example.com/managed-by": "npu-dra-driver",
			},
		},
		Spec: resourceapi.ResourceSliceSpec{
			Driver: "npu.ocloud.edge.example.com",
			Pool: resourceapi.ResourcePool{
				Name:               nodeName,
				Generation:         1,
				ResourceSliceCount: 1,
			},
			NodeName: nodeName,
			Devices:  devices,
		},
	}
	Expect(k8sClient.Create(ctx, slice)).To(Succeed())
	return slice
}

const (
	npuResourceName     = "huawei.com/Ascend910B"
	npuHealthLabelKey   = "huawei.com/Ascend910B-Health"
	npuHealthLabelValue = "Healthy"
)

// makeNode builds a Node with the given NPU capacity and health attributes,
// then creates it + writes its Status subresource via the envtest client.
//
// Capacity / Conditions / Labels are all set so isNodeNPUHealthy and
// the capacity-summing code path can exercise them without further fixup.
func makeNode(ctx context.Context, name string, npuCount int64, ready bool, healthy bool, extraLabels map[string]string) *corev1.Node {
	GinkgoHelper()
	labels := map[string]string{}
	for k, v := range extraLabels {
		labels[k] = v
	}
	if healthy {
		labels[npuHealthLabelKey] = npuHealthLabelValue
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
	Expect(k8sClient.Create(ctx, node)).To(Succeed())

	// Status subresource must be written separately.
	readyStatus := corev1.ConditionFalse
	if ready {
		readyStatus = corev1.ConditionTrue
	}
	node.Status = corev1.NodeStatus{
		Capacity: corev1.ResourceList{
			npuResourceName: *resource.NewQuantity(npuCount, resource.DecimalSI),
		},
		Allocatable: corev1.ResourceList{
			npuResourceName: *resource.NewQuantity(npuCount, resource.DecimalSI),
		},
		Conditions: []corev1.NodeCondition{
			{Type: corev1.NodeReady, Status: readyStatus},
		},
	}
	Expect(k8sClient.Status().Update(ctx, node)).To(Succeed())
	return node
}

// makePodOnNode builds a Pod scheduled to nodeName requesting npuRequest
// NPUs. The Pod's containers are minimal — only the resource request is
// load-bearing for the reconciler.
func makePodOnNode(ctx context.Context, namespace, name, nodeName string, npuRequest int64) *corev1.Pod {
	GinkgoHelper()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
			Containers: []corev1.Container{{
				Name:  "main",
				Image: "busybox",
				Resources: corev1.ResourceRequirements{
					// Extended resources are not overcommittable: kube-apiserver
					// validation requires Limits to mirror Requests for NPUs.
					Requests: corev1.ResourceList{
						npuResourceName: *resource.NewQuantity(npuRequest, resource.DecimalSI),
					},
					Limits: corev1.ResourceList{
						npuResourceName: *resource.NewQuantity(npuRequest, resource.DecimalSI),
					},
				},
			}},
		},
	}
	Expect(k8sClient.Create(ctx, pod)).To(Succeed())
	return pod
}

// makePool creates an NPUPool with the given selector and a unique name.
// Returns the created pool so callers can extract its name for re-Get.
func makePool(ctx context.Context, selector *metav1.LabelSelector) *imsv1alpha1.NPUPool {
	GinkgoHelper()
	pool := &imsv1alpha1.NPUPool{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pool-" + uuid.NewString()[:8],
		},
		Spec: imsv1alpha1.NPUPoolSpec{
			NodePoolRef:   corev1.LocalObjectReference{Name: "test-nodepool"},
			NPUModel:      "Ascend910B",
			Selector:      selector,
			SliceStrategy: "FixedTemplate",
		},
	}
	Expect(k8sClient.Create(ctx, pool)).To(Succeed())
	return pool
}

// reconcileAndFetch invokes the reconciler once for pool then re-reads it
// so callers see the populated Status.
func reconcileAndFetch(ctx context.Context, reconciler *controller.NPUPoolReconciler, pool *imsv1alpha1.NPUPool) *imsv1alpha1.NPUPool {
	GinkgoHelper()
	_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: pool.Name}})
	Expect(err).NotTo(HaveOccurred())

	updated := &imsv1alpha1.NPUPool{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pool.Name}, updated)).To(Succeed())
	return updated
}

// findCondition is shared with npuslicepool_controller_test.go.

var _ = Describe("NPUPool Reconcile", func() {
	var (
		ctx        context.Context
		reconciler *controller.NPUPoolReconciler
		testID     string
		createdNodes  []*corev1.Node
		createdPods   []*corev1.Pod
		createdPools  []*imsv1alpha1.NPUPool
		createdSlices []*resourceapi.ResourceSlice
	)

	BeforeEach(func() {
		ctx = context.Background()
		testID = uuid.NewString()[:8]
		reconciler = &controller.NPUPoolReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
		createdNodes = nil
		createdPods = nil
		createdPools = nil
		createdSlices = nil
	})

	AfterEach(func() {
		// Best-effort cleanup so cases don't leak resources into each other.
		// Cluster-scoped Nodes especially must be removed because the
		// "match all" case lists every node. ResourceSlices are cluster-
		// scoped too — managed-by label scoping wouldn't help inter-test
		// isolation since labels apply globally.
		for _, p := range createdPods {
			_ = k8sClient.Delete(ctx, p)
		}
		for _, s := range createdSlices {
			_ = k8sClient.Delete(ctx, s)
		}
		for _, n := range createdNodes {
			_ = k8sClient.Delete(ctx, n)
		}
		for _, pl := range createdPools {
			_ = k8sClient.Delete(ctx, pl)
		}
	})

	track := func(node *corev1.Node) {
		createdNodes = append(createdNodes, node)
	}
	trackPod := func(pod *corev1.Pod) {
		createdPods = append(createdPods, pod)
	}
	trackPool := func(pool *imsv1alpha1.NPUPool) {
		createdPools = append(createdPools, pool)
	}
	trackSlice := func(slice *resourceapi.ResourceSlice) {
		createdSlices = append(createdSlices, slice)
	}

	It("happy path: 3 nodes * 8 NPUs each = 24 total, all healthy", func() {
		poolLabel := map[string]string{"test-id": testID, "role": "compute"}
		for i := 0; i < 3; i++ {
			n := makeNode(ctx,
				fmt.Sprintf("node-%s-%d", testID, i),
				8, true, true, poolLabel)
			track(n)
		}

		pool := makePool(ctx, &metav1.LabelSelector{MatchLabels: poolLabel})
		trackPool(pool)

		updated := reconcileAndFetch(ctx, reconciler, pool)
		Expect(updated.Status.TotalNPUs).To(Equal(int32(24)))
		Expect(updated.Status.HealthyNPUs).To(Equal(int32(24)))
		Expect(updated.Status.AllocatedNPUs).To(Equal(int32(0)))
		Expect(updated.Status.HCCSTopology).NotTo(BeNil())

		ready := findCondition(updated.Status.Conditions, "Ready")
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionTrue))
		Expect(ready.Reason).To(Equal("Reconciled"))

		hccs := findCondition(updated.Status.Conditions, "HCCSDiscovered")
		Expect(hccs).NotTo(BeNil())
		Expect(hccs.Status).To(Equal(metav1.ConditionFalse))
		// Phase 6 T003: no ResourceSlices in this case → NoResourceSlicesObserved
		Expect(hccs.Reason).To(Equal("NoResourceSlicesObserved"))
	})

	It("empty selector: matches all nodes in cluster", func() {
		poolLabel := map[string]string{"test-id": testID}
		n := makeNode(ctx, "node-"+testID+"-only", 4, true, true, poolLabel)
		track(n)

		// Zero-value LabelSelector means "match everything" per
		// LabelSelectorAsSelector contract.
		pool := makePool(ctx, &metav1.LabelSelector{})
		trackPool(pool)

		updated := reconcileAndFetch(ctx, reconciler, pool)
		// At least our 4 NPUs are visible; other test residue may add more,
		// but the count must include ours and be >= 4. AfterEach blocks of
		// other cases reliably clean up, so in a clean suite this should
		// equal 4 — but we assert >= to remain robust to ordering.
		Expect(updated.Status.TotalNPUs).To(BeNumerically(">=", int32(4)))
		Expect(updated.Status.HealthyNPUs).To(BeNumerically(">=", int32(4)))

		ready := findCondition(updated.Status.Conditions, "Ready")
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionTrue))
	})

	It("no matching nodes: Ready=False, Reason=NoNPUFound", func() {
		// Selector requires a label no node carries.
		pool := makePool(ctx, &metav1.LabelSelector{
			MatchLabels: map[string]string{"nonexistent-label-" + testID: "yes"},
		})
		trackPool(pool)

		updated := reconcileAndFetch(ctx, reconciler, pool)
		Expect(updated.Status.TotalNPUs).To(Equal(int32(0)))
		Expect(updated.Status.HealthyNPUs).To(Equal(int32(0)))
		Expect(updated.Status.AllocatedNPUs).To(Equal(int32(0)))

		ready := findCondition(updated.Status.Conditions, "Ready")
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal("NoNPUFound"))
	})

	It("partial health: 2 of 3 nodes healthy → healthyNPUs = 16", func() {
		poolLabel := map[string]string{"test-id": testID, "role": "partial"}
		// Two healthy nodes.
		for i := 0; i < 2; i++ {
			n := makeNode(ctx,
				fmt.Sprintf("node-%s-h%d", testID, i),
				8, true, true, poolLabel)
			track(n)
		}
		// One unhealthy node (Ready=true but missing health label).
		nUnhealthy := makeNode(ctx,
			fmt.Sprintf("node-%s-u", testID),
			8, true, false, poolLabel)
		track(nUnhealthy)

		pool := makePool(ctx, &metav1.LabelSelector{MatchLabels: poolLabel})
		trackPool(pool)

		updated := reconcileAndFetch(ctx, reconciler, pool)
		Expect(updated.Status.TotalNPUs).To(Equal(int32(24)))
		Expect(updated.Status.HealthyNPUs).To(Equal(int32(16)))

		ready := findCondition(updated.Status.Conditions, "Ready")
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionTrue))
	})

	It("pod allocation: 3 pods on matched node, each requesting 2 NPUs → allocated=6", func() {
		poolLabel := map[string]string{"test-id": testID, "role": "alloc"}
		node := makeNode(ctx,
			fmt.Sprintf("node-%s-alloc", testID),
			8, true, true, poolLabel)
		track(node)

		// Schedule 3 pods to that node, each requesting 2 NPUs.
		for i := 0; i < 3; i++ {
			p := makePodOnNode(ctx, "default",
				fmt.Sprintf("pod-%s-%d", testID, i),
				node.Name, 2)
			trackPod(p)
		}

		pool := makePool(ctx, &metav1.LabelSelector{MatchLabels: poolLabel})
		trackPool(pool)

		updated := reconcileAndFetch(ctx, reconciler, pool)
		Expect(updated.Status.TotalNPUs).To(Equal(int32(8)))
		Expect(updated.Status.HealthyNPUs).To(Equal(int32(8)))
		Expect(updated.Status.AllocatedNPUs).To(Equal(int32(6)))
	})

	// Phase 6 T003 — HCCS topology aggregation cases (ADR-0010 §5).

	It("hccs aggregation: 2 nodes × 2 rings × 2 devices → 4 PeerGroups, sorted", func() {
		poolLabel := map[string]string{"test-id": testID, "role": "hccs-multi"}
		nodeA := makeNode(ctx, fmt.Sprintf("worker-%s-a", testID), 4, true, true, poolLabel)
		track(nodeA)
		nodeB := makeNode(ctx, fmt.Sprintf("worker-%s-b", testID), 4, true, true, poolLabel)
		track(nodeB)

		// 2 ResourceSlices: one per node, each with 4 devices split across
		// rings 0 and 1. Mirrors set-a-small mock layout.
		sliceA := makeHCCSResourceSlice(ctx, nodeA.Name, []deviceFixture{
			{name: "npu-0", ring: 0, health: "Healthy"},
			{name: "npu-1", ring: 0, health: "Healthy"},
			{name: "npu-2", ring: 1, health: "Healthy"},
			{name: "npu-3", ring: 1, health: "Healthy"},
		})
		trackSlice(sliceA)
		sliceB := makeHCCSResourceSlice(ctx, nodeB.Name, []deviceFixture{
			{name: "npu-0", ring: 0, health: "Healthy"},
			{name: "npu-1", ring: 0, health: "Healthy"},
			{name: "npu-2", ring: 1, health: "Healthy"},
			{name: "npu-3", ring: 1, health: "Healthy"},
		})
		trackSlice(sliceB)

		pool := makePool(ctx, &metav1.LabelSelector{MatchLabels: poolLabel})
		trackPool(pool)

		updated := reconcileAndFetch(ctx, reconciler, pool)
		Expect(updated.Status.HCCSTopology).NotTo(BeNil())
		Expect(updated.Status.HCCSTopology.PeerGroups).To(HaveLen(4))

		// Sorted: nodeA/ring-0, nodeA/ring-1, nodeB/ring-0, nodeB/ring-1.
		groupIDs := make([]string, len(updated.Status.HCCSTopology.PeerGroups))
		for i, pg := range updated.Status.HCCSTopology.PeerGroups {
			groupIDs[i] = pg.GroupID
		}
		Expect(groupIDs[0]).To(Equal(fmt.Sprintf("%s/ring-0", nodeA.Name)))
		Expect(groupIDs[1]).To(Equal(fmt.Sprintf("%s/ring-1", nodeA.Name)))
		Expect(groupIDs[2]).To(Equal(fmt.Sprintf("%s/ring-0", nodeB.Name)))
		Expect(groupIDs[3]).To(Equal(fmt.Sprintf("%s/ring-1", nodeB.Name)))

		// Each ring carries 2 device IDs qualified by nodeName.
		Expect(updated.Status.HCCSTopology.PeerGroups[0].DeviceIDs).To(ConsistOf(
			fmt.Sprintf("%s/npu-0", nodeA.Name),
			fmt.Sprintf("%s/npu-1", nodeA.Name),
		))

		hccs := findCondition(updated.Status.Conditions, "HCCSDiscovered")
		Expect(hccs).NotTo(BeNil())
		Expect(hccs.Status).To(Equal(metav1.ConditionTrue))
		Expect(hccs.Reason).To(Equal("Aggregated"))
	})

	It("hccs aggregation: ResourceSlice on out-of-pool node ignored", func() {
		poolLabel := map[string]string{"test-id": testID, "role": "hccs-isolation"}
		nodeIn := makeNode(ctx, fmt.Sprintf("worker-%s-in", testID), 2, true, true, poolLabel)
		track(nodeIn)
		// Node WITHOUT poolLabel — must NOT be aggregated.
		nodeOut := makeNode(ctx, fmt.Sprintf("worker-%s-out", testID), 2, true, true, nil)
		track(nodeOut)

		sliceIn := makeHCCSResourceSlice(ctx, nodeIn.Name, []deviceFixture{
			{name: "npu-0", ring: 0, health: "Healthy"},
		})
		trackSlice(sliceIn)
		sliceOut := makeHCCSResourceSlice(ctx, nodeOut.Name, []deviceFixture{
			{name: "npu-0", ring: 7, health: "Healthy"},
		})
		trackSlice(sliceOut)

		pool := makePool(ctx, &metav1.LabelSelector{MatchLabels: poolLabel})
		trackPool(pool)

		updated := reconcileAndFetch(ctx, reconciler, pool)
		Expect(updated.Status.HCCSTopology).NotTo(BeNil())
		Expect(updated.Status.HCCSTopology.PeerGroups).To(HaveLen(1))
		Expect(updated.Status.HCCSTopology.PeerGroups[0].GroupID).To(Equal(
			fmt.Sprintf("%s/ring-0", nodeIn.Name)))
		// ring-7 from out-of-pool node MUST NOT appear.
		for _, pg := range updated.Status.HCCSTopology.PeerGroups {
			Expect(pg.GroupID).NotTo(Equal(fmt.Sprintf("%s/ring-7", nodeOut.Name)))
		}
	})

	It("hccs aggregation: unhealthy devices are skipped", func() {
		poolLabel := map[string]string{"test-id": testID, "role": "hccs-health"}
		node := makeNode(ctx, fmt.Sprintf("worker-%s-mixed", testID), 4, true, true, poolLabel)
		track(node)

		// 2 healthy devices on ring 0, 2 unhealthy on ring 1.
		slice := makeHCCSResourceSlice(ctx, node.Name, []deviceFixture{
			{name: "npu-0", ring: 0, health: "Healthy"},
			{name: "npu-1", ring: 0, health: "Healthy"},
			{name: "npu-2", ring: 1, health: "Unhealthy"},
			{name: "npu-3", ring: 1, health: "Unknown"},
		})
		trackSlice(slice)

		pool := makePool(ctx, &metav1.LabelSelector{MatchLabels: poolLabel})
		trackPool(pool)

		updated := reconcileAndFetch(ctx, reconciler, pool)
		Expect(updated.Status.HCCSTopology).NotTo(BeNil())
		// Only ring 0 survives — unhealthy/unknown devices on ring 1 dropped.
		Expect(updated.Status.HCCSTopology.PeerGroups).To(HaveLen(1))
		Expect(updated.Status.HCCSTopology.PeerGroups[0].GroupID).To(Equal(
			fmt.Sprintf("%s/ring-0", node.Name)))
		Expect(updated.Status.HCCSTopology.PeerGroups[0].DeviceIDs).To(ConsistOf(
			fmt.Sprintf("%s/npu-0", node.Name),
			fmt.Sprintf("%s/npu-1", node.Name),
		))
	})
})
