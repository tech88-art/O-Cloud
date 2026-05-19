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

package main

import (
	"crypto/tls"
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
	// Phase 4 T004 registers npu.ocloud.edge.example.com/v1alpha1 here.
	// Phase 4 scaffold (T003) intentionally registers no Ocloud groups.
}

func main() {
	var metricsAddr string
	var probeAddr string
	var enableLeaderElection bool
	var enableHTTP2 bool
	var enablePublisher bool
	var enableClaimController bool
	var mockDataPath string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8082",
		"The address the metrics endpoint binds to. Set to 0 to disable.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081",
		"The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers.")

	// Phase 4 reserved flags — declared in T003 so dependent task land cleanly.
	// T005 (simulator publisher) wires --enable-publisher + --mock-data-path.
	// T006 (claim controller) wires --enable-claim-controller.
	flag.BoolVar(&enablePublisher, "enable-publisher", false,
		"Reserved (P4-T-005): enable the simulator-first ResourceSlice publisher. "+
			"Phase 4 scaffold (T003): flag declared but no-op.")
	flag.BoolVar(&enableClaimController, "enable-claim-controller", false,
		"Reserved (P4-T-006): enable the ResourceClaim controller skeleton. "+
			"Phase 4 scaffold (T003): flag declared but no-op.")
	flag.StringVar(&mockDataPath, "mock-data-path", "",
		"Reserved (P4-T-005): path to simulator NPU JSON (configs/mock-data/set-a-small/npus.json). "+
			"Phase 4 scaffold (T003): flag declared but unused.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Disable HTTP/2 by default (GHSA-qppj-fm5r-hxr3 / GHSA-4374-p667-p6c8).
	var tlsOpts []func(*tls.Config)
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, func(c *tls.Config) {
			setupLog.Info("Disabling HTTP/2")
			c.NextProtos = []string{"http/1.1"}
		})
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr, TLSOpts: tlsOpts},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "npu-dra-driver.ocloud.edge.example.com",
	})
	if err != nil {
		setupLog.Error(err, "Failed to start manager")
		os.Exit(1)
	}

	// Phase 4 T003 scaffold: no controllers registered yet.
	// T005 wires the ResourceSlice publisher behind --enable-publisher.
	// T006 wires the ResourceClaim controller behind --enable-claim-controller.
	if enablePublisher {
		setupLog.Info("--enable-publisher set but T005 ResourceSlice publisher not yet wired",
			"phase", "4-scaffold", "task", "P4-T-003", "mock-data-path", mockDataPath)
	}
	if enableClaimController {
		setupLog.Info("--enable-claim-controller set but T006 ResourceClaim controller not yet wired",
			"phase", "4-scaffold", "task", "P4-T-003")
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("Starting manager",
		"phase", "4-scaffold",
		"task", "P4-T-003",
		"controllers", "none (T003 scaffold; T005/T006 register publisher + claim controller)")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Failed to run manager")
		os.Exit(1)
	}
}
