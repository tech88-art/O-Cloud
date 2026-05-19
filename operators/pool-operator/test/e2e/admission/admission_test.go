//go:build e2e
// +build e2e

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

// Package admission carries the envtest-driven e2e tests for the
// ValidatingAdmissionPolicy skeleton (P3-T-005). It is a sibling of the
// existing Kind-based suite at test/e2e/ but lives in its own Go package
// so its envtest BeforeSuite does not collide with the Kind-based one.
//
// Run:
//
//	go test -tags=e2e ./test/e2e/admission/... -run TestAdmission -v
//
// The Kind-based suite (test/e2e/) is unchanged and still requires Kind.
// Main agent will fold VAP coverage into the kind smoke later (T104).
package admission

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	imsv1alpha1 "github.com/example/ocloud-edge/operators/pool-operator/api/v1alpha1"
)

func TestAdmission(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Admission Suite")
}

var (
	testEnv   *envtest.Environment
	cfg       *rest.Config
	k8sClient client.Client
)

var _ = BeforeSuite(func() {
	utilruntime.Must(imsv1alpha1.AddToScheme(clientgoscheme.Scheme))
	utilruntime.Must(admissionregv1.AddToScheme(clientgoscheme.Scheme))

	By("bootstrapping envtest with the pool CRDs")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: clientgoscheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	By("ensuring ocloud-system namespace exists")
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "ocloud-system"},
	}
	if err := k8sClient.Create(context.Background(), ns); err != nil && !apierrors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred(), "creating ocloud-system namespace")
	}

	By("applying the ValidatingAdmissionPolicy + Binding")
	applyAdmissionManifests(k8sClient)

	By("waiting for admission policy registration to propagate")
	// Even with synchronous Create above, the apiserver needs a beat to wire
	// the policy into its admission chain. Empirically 1-2s suffices.
	Eventually(func() bool {
		return policyEnforced(k8sClient)
	}, 30*time.Second, 500*time.Millisecond).Should(BeTrue(),
		"VAP did not start enforcing within 30s")
})

var _ = AfterSuite(func() {
	By("tearing down envtest")
	err := testEnv.Stop()
	// controller-runtime#1814: envtest.Stop() signals child processes via
	// os.Process.Signal(), unsupported on Windows. The processes are still
	// reaped when the test binary exits.
	if runtime.GOOS == "windows" && err != nil {
		GinkgoWriter.Printf("warning: envtest stop returned signal error on windows (expected): %v\n", err)
		return
	}
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("ValidatingAdmissionPolicy: NPUSlicePool namespace scope", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("rejects NPUSlicePool created in default namespace (no bypass label)", func() {
		slice := newMinimalNPUSlicePool("test-rejected", "default", nil)
		err := k8sClient.Create(ctx, slice)
		Expect(err).To(HaveOccurred(), "create should be rejected by VAP")
		// VAP rejection uses StatusReasonForbidden under validationActions=Deny
		// with reason=Forbidden, but Invalid is also tolerated as some
		// envtest builds wrap CEL failures as Invalid.
		Expect(apierrors.IsForbidden(err) || apierrors.IsInvalid(err)).To(BeTrue(),
			fmt.Sprintf("expected Forbidden or Invalid, got: %v", err))
		Expect(err.Error()).To(ContainSubstring("ocloud-system"))
		Expect(err.Error()).To(ContainSubstring("npu.huawei.com/multi-tenancy-bypass"))
	})

	It("accepts NPUSlicePool created in ocloud-system namespace (no bypass label)", func() {
		slice := newMinimalNPUSlicePool("test-accepted-ocloud-ns", "ocloud-system", nil)
		Expect(k8sClient.Create(ctx, slice)).To(Succeed())
	})

	It("accepts NPUSlicePool in default namespace WITH bypass label", func() {
		slice := newMinimalNPUSlicePool("test-bypass", "default", map[string]string{
			"npu.huawei.com/multi-tenancy-bypass": "true",
		})
		Expect(k8sClient.Create(ctx, slice)).To(Succeed())
	})
})

// newMinimalNPUSlicePool builds the smallest NPUSlicePool that passes CRD
// schema validation. Spec content is intentionally trivial — the test is
// about admission policy, not slicing semantics.
func newMinimalNPUSlicePool(name, ns string, labels map[string]string) *imsv1alpha1.NPUSlicePool {
	return &imsv1alpha1.NPUSlicePool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: imsv1alpha1.NPUSlicePoolSpec{
			NPUPoolRef: corev1.LocalObjectReference{Name: "dummy-pool"},
			Strategy:   imsv1alpha1.SliceStrategyFixedTemplate,
			FixedTemplates: []imsv1alpha1.SliceTemplate{
				{Name: "vir04", AICoreCount: 4, MemoryMiB: 16384},
			},
		},
	}
}

// applyAdmissionManifests reads the VAP + Binding YAML files from
// config/admission/ and applies them through the controller-runtime client
// using strongly-typed admissionregistration.k8s.io/v1 objects (simpler
// than the unstructured/dynamic path for two known kinds).
func applyAdmissionManifests(c client.Client) {
	root := admissionConfigDir()

	policy := &admissionregv1.ValidatingAdmissionPolicy{}
	readYAMLInto(filepath.Join(root, "validating-admission-policy.yaml"), policy)
	Expect(c.Create(context.Background(), policy)).To(Succeed(),
		"applying ValidatingAdmissionPolicy")

	binding := &admissionregv1.ValidatingAdmissionPolicyBinding{}
	readYAMLInto(filepath.Join(root, "validating-admission-policy-binding.yaml"), binding)
	Expect(c.Create(context.Background(), binding)).To(Succeed(),
		"applying ValidatingAdmissionPolicyBinding")
}

// admissionConfigDir returns the absolute path to operators/pool-operator/
// config/admission/, regardless of where `go test` is invoked from.
// This is needed because Go tests run with the package directory as CWD.
func admissionConfigDir() string {
	// Test file lives at test/e2e/admission/admission_test.go;
	// config lives at config/admission/. Walk up 3 levels.
	return filepath.Join("..", "..", "..", "config", "admission")
}

func readYAMLInto(path string, into any) {
	raw, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred(), "reading %s", path)
	Expect(yaml.Unmarshal(raw, into)).To(Succeed(), "unmarshalling %s", path)
}

// policyEnforced does a single dry-run create against the default namespace
// and returns true iff the VAP is actively rejecting. The dry-run does not
// persist anything regardless of outcome.
func policyEnforced(c client.Client) bool {
	probe := newMinimalNPUSlicePool("vap-readiness-probe", "default", nil)
	err := c.Create(context.Background(), probe, client.DryRunAll)
	if err == nil {
		// Policy not enforcing yet — dry-run succeeded.
		return false
	}
	// Any rejection mentioning ocloud-system means the policy fired.
	return errorMatchesVAP(err)
}

func errorMatchesVAP(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "ocloud-system") && contains(msg, "npu.huawei.com/multi-tenancy-bypass")
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	// Tiny stdlib-free substring for the readiness probe; avoids importing
	// strings just for one call (keeps imports honest).
	n, m := len(s), len(sub)
	if m == 0 {
		return 0
	}
	for i := 0; i+m <= n; i++ {
		if s[i:i+m] == sub {
			return i
		}
	}
	return -1
}

// _ = schema.GroupVersionKind referenced for clarity even if not directly
// used: ensures the admissionregistration GVK is available via the imports.
var _ = schema.GroupVersionKind{}
