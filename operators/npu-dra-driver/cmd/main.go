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
	"fmt"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	resourceapi "k8s.io/api/resource/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	v1alpha1 "github.com/tech88-art/O-Cloud/operators/npu-dra-driver/api/v1alpha1"
	"github.com/tech88-art/O-Cloud/operators/npu-dra-driver/internal/controller"
	"github.com/tech88-art/O-Cloud/operators/npu-dra-driver/internal/publisher"
	"github.com/tech88-art/O-Cloud/operators/npu-dra-driver/internal/source"
	"github.com/tech88-art/O-Cloud/operators/npu-dra-driver/internal/source/mockjson"
	"github.com/tech88-art/O-Cloud/operators/npu-dra-driver/internal/source/realascend"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(resourceapi.AddToScheme(scheme))
	// Phase 5 T004: register the Ocloud npu.ocloud.edge.example.com group
	// so the manager's typed client knows about NPUSliceAllocation. T005
	// adds the controller; T002+T003 readers already use the typed client
	// directly when they consume slices (via resourceapi) so this line
	// only matters once NPUSliceAllocation lookups land.
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
	// Phase 4 T004/T005 registered upstream resource.k8s.io/v1beta1 here so
	// the publisher (T005) could read/write ResourceSlices via the manager
	// client.
}

func main() {
	var metricsAddr string
	var probeAddr string
	var enableLeaderElection bool
	var enableHTTP2 bool
	var enablePublisher bool
	var enableClaimController bool
	var enableAllocationController bool
	var mockDataPath string
	var sourceType string
	var realAscendMode string
	var enableTemplateController bool

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8082",
		"The address the metrics endpoint binds to. Set to 0 to disable.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081",
		"The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers.")

	// Phase 4 reserved flags.
	// T005 wires --enable-publisher + --mock-data-path to the simulator
	// ResourceSlice publisher. T006 wires --enable-claim-controller.
	flag.BoolVar(&enablePublisher, "enable-publisher", false,
		"Enable the simulator-first ResourceSlice publisher (P4-T-005). "+
			"Requires --mock-data-path.")
	flag.BoolVar(&enableClaimController, "enable-claim-controller", false,
		"Enable the ResourceClaim controller (P5-T-002+). Performs real "+
			"allocation against ResourceSlices and writes ResourceClaim "+
			"status.allocation. Also creates audit-log NPUSliceAllocation "+
			"objects (per ADR-0009 §5 step 3) so cluster operators have a "+
			"reverse-lookup index.")
	flag.BoolVar(&enableAllocationController, "enable-allocation-controller", true,
		"Enable the NPUSliceAllocation controller (P5-T-005). Manages the "+
			"audit-log object lifecycle: Allocated → Released → Orphaned. "+
			"Default true — disable only for development with a manual "+
			"allocation pipeline.")
	flag.StringVar(&mockDataPath, "mock-data-path", "",
		"Path to the simulator NPU JSON (e.g. /etc/npu-dra-driver/mock/npus.json "+
			"or configs/mock-data/set-a-small/npus.json on host dev). "+
			"Required when --enable-publisher is set with --source-type=mock-json (default).")

	// Phase 7 P7-T-004 (ADR-0011 §2): selectSource dispatch + source type
	// + realascend mode. Default "mock-json" preserves Phase 4-6 behavior
	// bit-for-bit; "real-ascend" Phase 7 W1 stub returns ErrNotImplemented
	// (Phase 7 T101 lab-conditional lights up the real silicon body).
	flag.StringVar(&sourceType, "source-type", "mock-json",
		"Source backend for the publisher (Phase 7 P7-T-004 / ADR-0011 §2). "+
			"One of 'mock-json' (default · reads --mock-data-path JSON) | "+
			"'real-ascend' (Phase 7 W1 stub · lab-conditional T101 body).")
	flag.StringVar(&realAscendMode, "source-real-ascend-mode", "exec",
		"Backend mode for real-ascend source (Phase 7 W1 stub honors no modes · "+
			"Phase 7 T101 lab body uses 'exec' to shell out to npu-smi).")
	flag.BoolVar(&enableTemplateController, "enable-template-controller", true,
		"Enable the NPUSliceTemplate controller (Phase 7 P7-T-007 · "+
			"ADR-0011 §1 §4). Stamps Validated + Allocatable conditions on "+
			"NPUSliceTemplate objects + populates status.fallbackAppliedReason. "+
			"Default true — disable only for diagnostic builds.")

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

	// Phase 4 T005 / Phase 7 T004: register the ResourceSlice publisher
	// when --enable-publisher is set. Phase 4 T006 wires the ResourceClaim
	// controller behind --enable-claim-controller. Phase 7 P7-T-004
	// (ADR-0011 §2) switched the inline SimulatorSource construction to
	// the source-type dispatch below — mock-json default preserves Phase
	// 4-6 behavior bit-for-bit; real-ascend stub is the Phase 7 W1
	// scaffold for the lab-conditional T101 body.
	if enablePublisher {
		src, err := selectSource(sourceType, mockDataPath, realAscendMode)
		if err != nil {
			setupLog.Error(err, "Failed to construct publisher source")
			os.Exit(1)
		}
		pub := &publisher.Publisher{
			Client: mgr.GetClient(),
			Source: src,
		}
		if err := pub.SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "Failed to register publisher with manager")
			os.Exit(1)
		}
		setupLog.Info("Publisher registered",
			"task", "P4-T-005 + P7-T-004",
			"source-type", sourceType,
			"mock-data-path", mockDataPath)
	}
	if enableClaimController {
		cr := &controller.ClaimReconciler{
			Client:   mgr.GetClient(),
			Scheme:   mgr.GetScheme(),
			Recorder: mgr.GetEventRecorderFor("npu-dra-claim-controller"),
		}
		if err := cr.SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "Failed to register ClaimReconciler with manager")
			os.Exit(1)
		}
		setupLog.Info("ClaimReconciler registered", "task", "P5-T-002")
	}
	if enableAllocationController {
		ar := &controller.AllocationReconciler{
			Client:   mgr.GetClient(),
			Scheme:   mgr.GetScheme(),
			Recorder: mgr.GetEventRecorderFor("npu-dra-allocation-controller"),
		}
		if err := ar.SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "Failed to register AllocationReconciler with manager")
			os.Exit(1)
		}
		setupLog.Info("AllocationReconciler registered", "task", "P5-T-005")
	}
	if enableTemplateController {
		tr := &controller.NPUSliceTemplateReconciler{
			Client:   mgr.GetClient(),
			Scheme:   mgr.GetScheme(),
			Recorder: mgr.GetEventRecorderFor("npu-dra-template-controller"),
		}
		if err := tr.SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "Failed to register NPUSliceTemplateReconciler with manager")
			os.Exit(1)
		}
		setupLog.Info("NPUSliceTemplateReconciler registered", "task", "P7-T-007")
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
		"phase", "5",
		"latest-task", "P5-T-005",
		"publisher-enabled", enablePublisher,
		"claim-controller-enabled", enableClaimController,
		"allocation-controller-enabled", enableAllocationController)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Failed to run manager")
		os.Exit(1)
	}
}

// selectSource dispatches CLI flag values to the matching source backend
// constructor per Phase 7 P7-T-004 (ADR-0011 §2 + factory pattern).
// Lives in cmd/main.go (not internal/source/factory.go) to avoid an
// internal/source → internal/source/{mockjson,realascend} → internal/source
// import cycle. The source pkg owns the enum + config type; cmd/main.go
// owns the switch + subpackage imports.
func selectSource(sourceType, mockDataPath, realAscendMode string) (source.Source, error) {
	t, err := source.ParseSourceType(sourceType)
	if err != nil {
		return nil, err
	}
	switch t {
	case source.SourceTypeMockJSON:
		if mockDataPath == "" {
			return nil, fmt.Errorf("source mock-json: --mock-data-path required")
		}
		return mockjson.New(mockjson.Config{Path: mockDataPath}), nil
	case source.SourceTypeRealAscend:
		// Phase 7 W1 stub: constructs but returns ErrNotImplemented at
		// first call. Operators who explicitly opt in are warned in
		// publisher reconcile logs. Phase 7 T101 lab body lights up.
		setupLog.Info("source real-ascend selected · Phase 7 W1 stub "+
			"(returns ErrNotImplemented; lab body lands in T101)",
			"mode", realAscendMode)
		return realascend.New(realascend.Config{Mode: realAscendMode}), nil
	default:
		// Defensive — ParseSourceType already rejects unknown values.
		return nil, fmt.Errorf("source: unhandled type %q", t)
	}
}
