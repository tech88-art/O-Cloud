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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	imsv1alpha1 "github.com/example/ocloud-edge/operators/pool-operator/api/v1alpha1"
	"github.com/example/ocloud-edge/operators/pool-operator/internal/controller"
)

// makeClusterPoolFixture creates a minimal ClusterPool with one member cluster
// and returns it after the apiserver assigns metadata. Cluster-scoped so no
// namespace is needed. Names are unique per spec via uniqueName() to avoid
// envtest collisions across cases.
func makeClusterPoolFixture(ctx context.Context, name string) *imsv1alpha1.ClusterPool {
	GinkgoHelper()
	cp := &imsv1alpha1.ClusterPool{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: imsv1alpha1.ClusterPoolSpec{
			Clusters: []imsv1alpha1.ClusterRef{
				{
					ClusterID:  "test-cluster-a",
					SyncPolicy: imsv1alpha1.SyncPolicyPush,
				},
			},
		},
	}
	Expect(k8sClient.Create(ctx, cp)).To(Succeed())
	// Re-fetch so callers see the apiserver-assigned ResourceVersion / UID.
	Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, cp)).To(Succeed())
	return cp
}

var _ = Describe("ClusterPool Phase 3 placeholder Reconcile", func() {
	var (
		ctx        context.Context
		reconciler *controller.ClusterPoolReconciler
		created    []*imsv1alpha1.ClusterPool
	)

	BeforeEach(func() {
		ctx = context.Background()
		reconciler = &controller.ClusterPoolReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
		created = nil
	})

	AfterEach(func() {
		// Cluster-scoped ClusterPools must be cleaned up between specs so
		// they do not leak into other tests that List() the type.
		for _, cp := range created {
			_ = k8sClient.Delete(ctx, cp)
		}
	})

	track := func(cp *imsv1alpha1.ClusterPool) {
		created = append(created, cp)
	}

	It("sets PhaseDeferred=True with Reason=WaitingForKarmada on first reconcile", func() {
		name := uniqueName("t105-cp-first")
		cp := makeClusterPoolFixture(ctx, name)
		track(cp)

		_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: name}})
		Expect(err).NotTo(HaveOccurred())

		var got imsv1alpha1.ClusterPool
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, &got)).To(Succeed())

		Expect(got.Status.Conditions).To(HaveLen(1),
			"placeholder Reconcile should emit exactly one condition")

		deferred := findCondition(got.Status.Conditions, "PhaseDeferred")
		Expect(deferred).NotTo(BeNil())
		Expect(deferred.Status).To(Equal(metav1.ConditionTrue))
		Expect(deferred.Reason).To(Equal("WaitingForKarmada"))
		// Message must reference both the Phase 9 milestone and the architecture
		// doc so dashboard / log readers can find the forward note quickly.
		Expect(deferred.Message).To(ContainSubstring("Phase 9"))
		Expect(strings.ToLower(deferred.Message)).To(ContainSubstring("architecture.md"))
	})

	It("idempotent: 2nd Reconcile leaves Status unchanged (no LastTransitionTime churn)", func() {
		name := uniqueName("t105-cp-idemp")
		cp := makeClusterPoolFixture(ctx, name)
		track(cp)

		key := types.NamespacedName{Name: name}

		// First pass — populates the PhaseDeferred condition.
		_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())

		var afterFirst imsv1alpha1.ClusterPool
		Expect(k8sClient.Get(ctx, key, &afterFirst)).To(Succeed())
		firstCond := findCondition(afterFirst.Status.Conditions, "PhaseDeferred")
		Expect(firstCond).NotTo(BeNil())
		capturedLTT := firstCond.LastTransitionTime
		capturedRV := afterFirst.ResourceVersion

		// Second pass — must skip the Status().Update() entirely.
		_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: key})
		Expect(err).NotTo(HaveOccurred())

		var afterSecond imsv1alpha1.ClusterPool
		Expect(k8sClient.Get(ctx, key, &afterSecond)).To(Succeed())

		// (a) Condition count unchanged.
		Expect(afterSecond.Status.Conditions).To(HaveLen(len(afterFirst.Status.Conditions)))

		// (b) LastTransitionTime preserved verbatim — no churn from
		// metav1.Now() being called on the second pass.
		secondCond := findCondition(afterSecond.Status.Conditions, "PhaseDeferred")
		Expect(secondCond).NotTo(BeNil())
		Expect(secondCond.LastTransitionTime.Equal(&capturedLTT)).To(BeTrue(),
			"LastTransitionTime should not change between idempotent reconciles "+
				"(captured=%v, observed=%v)", capturedLTT, secondCond.LastTransitionTime)

		// (c) Reason / Status / Message stable.
		Expect(secondCond.Status).To(Equal(metav1.ConditionTrue))
		Expect(secondCond.Reason).To(Equal("WaitingForKarmada"))
		Expect(secondCond.Message).To(Equal(firstCond.Message))

		// (d) ResourceVersion unchanged — strongest signal that the apiserver
		// saw no write on the second pass.
		Expect(afterSecond.ResourceVersion).To(Equal(capturedRV),
			"a no-op reconcile must not bump ResourceVersion")
	})
})
