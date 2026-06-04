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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	imsv1alpha1 "github.com/example/ocloud-edge/operators/pool-operator/api/v1alpha1"
	"github.com/example/ocloud-edge/operators/pool-operator/internal/controller"
)

// makeNodeWithCPUMem builds a Node with the given CPU / Memory capacity and
// labels. It is T004-specific (suffix avoids colliding with `makeNode` in
// npupool_controller_test.go which sets NPU capacity + Ready conditions
// instead). Status is updated via the status subresource so envtest persists
// the capacity values.
func makeNodeWithCPUMem(ctx context.Context, name string, cpu, mem string, labels map[string]string) *corev1.Node {
	GinkgoHelper()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
	Expect(k8sClient.Create(ctx, node)).To(Succeed())
	Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, node)).To(Succeed())
	node.Status.Capacity = corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse(cpu),
		corev1.ResourceMemory: resource.MustParse(mem),
	}
	Expect(k8sClient.Status().Update(ctx, node)).To(Succeed())
	return node
}

var _ = Describe("NodePool Reconcile", func() {
	var (
		ctx          context.Context
		reconciler   *controller.NodePoolReconciler
		testTag      string
		createdNodes []*corev1.Node
		createdPools []*imsv1alpha1.NodePool
	)

	BeforeEach(func() {
		ctx = context.Background()
		reconciler = &controller.NodePoolReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
		// testTag scopes each case's label set so the Selector cannot match
		// nodes left behind by sibling specs (envtest shares the cluster).
		testTag = uniqueName("t004")
		createdNodes = nil
		createdPools = nil
	})

	AfterEach(func() {
		// Cluster-scoped resources must be cleaned up explicitly; envtest
		// retains them across specs otherwise. Best-effort delete since some
		// cases create the NodePool implicitly through reconciliation.
		for _, n := range createdNodes {
			_ = k8sClient.Delete(ctx, n)
		}
		for _, p := range createdPools {
			_ = k8sClient.Delete(ctx, p)
		}
	})

	trackNode := func(n *corev1.Node) {
		createdNodes = append(createdNodes, n)
	}
	trackPool := func(p *imsv1alpha1.NodePool) {
		createdPools = append(createdPools, p)
	}

	reconcileOnce := func(pool *imsv1alpha1.NodePool) *imsv1alpha1.NodePool {
		GinkgoHelper()
		_, err := reconciler.Reconcile(ctx, ctrl.Request{
			NamespacedName: client.ObjectKey{Name: pool.Name},
		})
		Expect(err).NotTo(HaveOccurred())
		updated := &imsv1alpha1.NodePool{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: pool.Name}, updated)).To(Succeed())
		return updated
	}

	It("happy path: 3 matched amd64 edge nodes -> TotalCPU=24, TotalMemory=96Gi, Ready=True", func() {
		nodeLabel := map[string]string{
			"site":                  testTag,
			nodeRoleLabelForTest:    "edge",
			"kubernetes.io/arch":    "amd64",
		}
		for i := 0; i < 3; i++ {
			n := makeNodeWithCPUMem(ctx,
				fmt.Sprintf("%s-happy-%d", testTag, i),
				"8", "32Gi", nodeLabel)
			trackNode(n)
		}

		pool := &imsv1alpha1.NodePool{
			ObjectMeta: metav1.ObjectMeta{Name: uniqueName("nodepool-happy")},
			Spec: imsv1alpha1.NodePoolSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"site": testTag}},
				Role:     imsv1alpha1.NodeRole("edge"),
			},
		}
		Expect(k8sClient.Create(ctx, pool)).To(Succeed())
		trackPool(pool)

		updated := reconcileOnce(pool)
		Expect(updated.Status.Nodes).To(HaveLen(3))
		// Names are sorted in status for determinism; we just check membership.
		for i := 0; i < 3; i++ {
			Expect(updated.Status.Nodes).To(ContainElement(
				fmt.Sprintf("%s-happy-%d", testTag, i)))
		}

		// 3 * 8 = 24 CPUs (DecimalSI), 3 * 32Gi = 96Gi memory (BinarySI).
		expectedCPU := resource.MustParse("24")
		expectedMem := resource.MustParse("96Gi")
		Expect(updated.Status.TotalCPU.Cmp(expectedCPU)).To(Equal(0),
			"TotalCPU got=%s want=%s", updated.Status.TotalCPU.String(), expectedCPU.String())
		Expect(updated.Status.TotalMemory.Cmp(expectedMem)).To(Equal(0),
			"TotalMemory got=%s want=%s", updated.Status.TotalMemory.String(), expectedMem.String())

		ready := findCondition(updated.Status.Conditions, "Ready")
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionTrue))
		Expect(ready.Reason).To(Equal("Reconciled"))
		Expect(ready.Message).To(ContainSubstring("3 nodes matched"))
	})

	It("empty selector: Ready=False with Reason=MissingSelector", func() {
		// The CRD enforces `Selector: Required` at the schema layer, so we
		// cannot POST a nil pointer. The reconciler treats an empty (but
		// non-nil) LabelSelector{} the same way — both nil and empty match
		// no nodes intentionally rather than implicitly "match everything",
		// per the demo-deterministic design call. This case exercises the
		// empty-struct path because it's the one a user can still hit.
		pool := &imsv1alpha1.NodePool{
			ObjectMeta: metav1.ObjectMeta{Name: uniqueName("nodepool-nosel")},
			Spec: imsv1alpha1.NodePoolSpec{
				Selector: &metav1.LabelSelector{},
				Role:     imsv1alpha1.NodeRole("edge"),
			},
		}
		Expect(k8sClient.Create(ctx, pool)).To(Succeed())
		trackPool(pool)

		updated := reconcileOnce(pool)
		ready := findCondition(updated.Status.Conditions, "Ready")
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal("MissingSelector"))
		Expect(updated.Status.Nodes).To(BeEmpty())
	})

	It("role filter mismatch: nodes labeled edge but pool requests core -> NoNodeMatched", func() {
		nodeLabel := map[string]string{
			"site":               testTag,
			nodeRoleLabelForTest: "edge",
			"kubernetes.io/arch": "amd64",
		}
		for i := 0; i < 3; i++ {
			n := makeNodeWithCPUMem(ctx,
				fmt.Sprintf("%s-mismatch-%d", testTag, i),
				"8", "32Gi", nodeLabel)
			trackNode(n)
		}

		pool := &imsv1alpha1.NodePool{
			ObjectMeta: metav1.ObjectMeta{Name: uniqueName("nodepool-mismatch")},
			Spec: imsv1alpha1.NodePoolSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"site": testTag}},
				Role:     imsv1alpha1.NodeRole("core"),
			},
		}
		Expect(k8sClient.Create(ctx, pool)).To(Succeed())
		trackPool(pool)

		updated := reconcileOnce(pool)
		Expect(updated.Status.Nodes).To(BeEmpty())
		Expect(updated.Status.TotalCPU.IsZero()).To(BeTrue())
		Expect(updated.Status.TotalMemory.IsZero()).To(BeTrue())

		ready := findCondition(updated.Status.Conditions, "Ready")
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionFalse))
		Expect(ready.Reason).To(Equal("NoNodeMatched"))
	})

	It("arm64 included: 3 amd64 + 2 arm64 Kunpeng -> all 5 counted (ADR-0020)", func() {
		// Pre-ADR-0020 this spec asserted the 2 arm64 nodes were DROPPED by an
		// amd64-only arch guard. ADR-0020 (Phase 12) flipped the deployment
		// target to aarch64 Kunpeng 920, so arm64 nodes are now first-class
		// members; amd64 stays valid for dev/CI. P13-fix-005 removed the guard.
		// 3 amd64 dev nodes (8 CPU / 32Gi each).
		amd64Label := map[string]string{
			"site":               testTag,
			nodeRoleLabelForTest: "edge",
			"kubernetes.io/arch": "amd64",
		}
		for i := 0; i < 3; i++ {
			n := makeNodeWithCPUMem(ctx,
				fmt.Sprintf("%s-amd-%d", testTag, i),
				"8", "32Gi", amd64Label)
			trackNode(n)
		}
		// 2 arm64 Kunpeng nodes (16 CPU / 64Gi each) with the same site + role
		// labels — these must now be counted, not dropped.
		armLabel := map[string]string{
			"site":               testTag,
			nodeRoleLabelForTest: "edge",
			"kubernetes.io/arch": "arm64",
		}
		for i := 0; i < 2; i++ {
			n := makeNodeWithCPUMem(ctx,
				fmt.Sprintf("%s-arm-%d", testTag, i),
				"16", "64Gi", armLabel)
			trackNode(n)
		}

		pool := &imsv1alpha1.NodePool{
			ObjectMeta: metav1.ObjectMeta{Name: uniqueName("nodepool-arch")},
			Spec: imsv1alpha1.NodePoolSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"site": testTag}},
				Role:     imsv1alpha1.NodeRole("edge"),
			},
		}
		Expect(k8sClient.Create(ctx, pool)).To(Succeed())
		trackPool(pool)

		updated := reconcileOnce(pool)
		// All 5 nodes — both amd64 and arm64 — are present.
		Expect(updated.Status.Nodes).To(HaveLen(5))
		for i := 0; i < 3; i++ {
			Expect(updated.Status.Nodes).To(ContainElement(
				fmt.Sprintf("%s-amd-%d", testTag, i)))
		}
		for i := 0; i < 2; i++ {
			Expect(updated.Status.Nodes).To(ContainElement(
				fmt.Sprintf("%s-arm-%d", testTag, i)))
		}

		// Totals now include the arm64 capacity:
		// CPU = 3*8 + 2*16 = 56 ; Mem = 3*32Gi + 2*64Gi = 224Gi.
		expectedCPU := resource.MustParse("56")
		expectedMem := resource.MustParse("224Gi")
		Expect(updated.Status.TotalCPU.Cmp(expectedCPU)).To(Equal(0),
			"TotalCPU got=%s want=%s", updated.Status.TotalCPU.String(), expectedCPU.String())
		Expect(updated.Status.TotalMemory.Cmp(expectedMem)).To(Equal(0),
			"TotalMemory got=%s want=%s", updated.Status.TotalMemory.String(), expectedMem.String())

		ready := findCondition(updated.Status.Conditions, "Ready")
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionTrue))
		Expect(ready.Reason).To(Equal("Reconciled"))
		Expect(ready.Message).To(ContainSubstring("5 nodes matched"))
	})

	It("pure arm64 Kunpeng cluster: all nodes counted, status not empty (ADR-0020 real-target regression)", func() {
		// Regression guard for the P13-fix-005 forward-fix: on a real Atlas 800
		// cluster every node is arm64 (Kunpeng 920). The pre-fix amd64-only guard
		// would have dropped ALL of them -> status.nodes empty, totalCPU/Mem zero.
		armLabel := map[string]string{
			"site":               testTag,
			nodeRoleLabelForTest: "edge",
			"kubernetes.io/arch": "arm64",
		}
		for i := 0; i < 3; i++ {
			n := makeNodeWithCPUMem(ctx,
				fmt.Sprintf("%s-kunpeng-%d", testTag, i),
				"16", "64Gi", armLabel)
			trackNode(n)
		}

		pool := &imsv1alpha1.NodePool{
			ObjectMeta: metav1.ObjectMeta{Name: uniqueName("nodepool-kunpeng")},
			Spec: imsv1alpha1.NodePoolSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"site": testTag}},
				Role:     imsv1alpha1.NodeRole("edge"),
			},
		}
		Expect(k8sClient.Create(ctx, pool)).To(Succeed())
		trackPool(pool)

		updated := reconcileOnce(pool)
		// The real-target regression: status must NOT be empty — all 3 arm64
		// Kunpeng nodes are present.
		Expect(updated.Status.Nodes).To(HaveLen(3))
		for i := 0; i < 3; i++ {
			Expect(updated.Status.Nodes).To(ContainElement(
				fmt.Sprintf("%s-kunpeng-%d", testTag, i)))
		}

		// 3 * 16 = 48 CPU ; 3 * 64Gi = 192Gi.
		expectedCPU := resource.MustParse("48")
		expectedMem := resource.MustParse("192Gi")
		Expect(updated.Status.TotalCPU.Cmp(expectedCPU)).To(Equal(0),
			"TotalCPU got=%s want=%s", updated.Status.TotalCPU.String(), expectedCPU.String())
		Expect(updated.Status.TotalMemory.Cmp(expectedMem)).To(Equal(0),
			"TotalMemory got=%s want=%s", updated.Status.TotalMemory.String(), expectedMem.String())

		ready := findCondition(updated.Status.Conditions, "Ready")
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionTrue))
		Expect(ready.Reason).To(Equal("Reconciled"))
	})
})

// nodeRoleLabelForTest mirrors the unexported `nodeRoleLabel` from
// nodepool_controller.go. We cannot import it from the external test package,
// so we duplicate the literal here — if the production constant ever changes,
// this string must change with it. Keeping the literal in one obvious place
// (this comment + this var) makes that linkage explicit.
const nodeRoleLabelForTest = "node-role.ocloud.edge.example.com"
