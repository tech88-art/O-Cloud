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
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	imsv1alpha1 "github.com/example/ocloud-edge/operators/pool-operator/api/v1alpha1"
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	Expect(imsv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	// controller-runtime's envtest.Stop() uses os.Process.Signal() to terminate
	// kube-apiserver/etcd; on Windows that returns "not supported by windows".
	// The processes still get killed when the test binary exits, so we tolerate
	// the signal error on Windows.
	if runtime.GOOS == "windows" && err != nil {
		GinkgoWriter.Printf("warning: envtest stop returned signal error on windows (expected): %v\n", err)
		return
	}
	Expect(err).NotTo(HaveOccurred())
})

// Placeholder spec so the envtest suite has at least one It() — silences
// ginkgo's "no specs" warning. Real Reconciler tests land in P3-T-002..T005.
var _ = Describe("controller scaffolding", func() {
	It("brings up envtest with the pool CRDs registered", func() {
		Expect(cfg).NotTo(BeNil())
		Expect(k8sClient).NotTo(BeNil())
	})
})
