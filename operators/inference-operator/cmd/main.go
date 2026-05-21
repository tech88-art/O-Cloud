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

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	inferencev1alpha1 "github.com/tech88-art/O-Cloud/operators/inference-operator/api/v1alpha1"
	"github.com/tech88-art/O-Cloud/operators/inference-operator/internal/controller"
	"github.com/tech88-art/O-Cloud/operators/inference-operator/internal/metrics"
	"github.com/tech88-art/O-Cloud/operators/inference-operator/internal/webhook"
	"time"

	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"
	ctrladmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(inferencev1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
	// Phase 5: register ModelService controller; Phase 4 ships types only.
}

func main() {
	var metricsAddr string
	var probeAddr string
	var enableLeaderElection bool
	var enableHTTP2 bool
	var enableModelServiceController bool
	var enablePDRouterWebhook bool

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8082",
		"The address the metrics endpoint binds to. Set to 0 to disable.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081",
		"The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers.")
	flag.BoolVar(&enableModelServiceController, "enable-modelservice-controller", true,
		"Enable the ModelService controller (P5-T-006+). Resolves the "+
			"bound NPUSlicePool, drives Status.Phase through Pending → "+
			"Provisioning → Ready / Failed. Default true.")
	flag.BoolVar(&enablePDRouterWebhook, "enable-pd-router-webhook", true,
		"Enable the PD Router mutating admission webhook (P5-T-102+). "+
			"Injects npu.huawei.com/slice-bindings annotations onto Pods "+
			"carrying the inference.ocloud.edge.example.com/model-service "+
			"label. Default true.")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	var tlsOpts []func(*tls.Config)
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, func(c *tls.Config) {
			setupLog.Info("Disabling HTTP/2")
			c.NextProtos = []string{"http/1.1"}
		})
	}

	webhookServerOpts := ctrlwebhook.Options{
		TLSOpts: tlsOpts,
		Port:    9443,
	}
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr, TLSOpts: tlsOpts},
		WebhookServer:          ctrlwebhook.NewServer(webhookServerOpts),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "inference-operator.ocloud.edge.example.com",
	})
	if err != nil {
		setupLog.Error(err, "Failed to start manager")
		os.Exit(1)
	}

	if enableModelServiceController {
		msr := &controller.ModelServiceReconciler{
			Client:   mgr.GetClient(),
			Scheme:   mgr.GetScheme(),
			Recorder: mgr.GetEventRecorderFor("modelservice-controller"),
		}
		if err := msr.SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "Failed to register ModelServiceReconciler")
			os.Exit(1)
		}
		setupLog.Info("ModelServiceReconciler registered", "task", "P5-T-006")
	}
	if enablePDRouterWebhook {
		h := &webhook.PDRouter{
			Client:         mgr.GetClient(),
			Decoder:        ctrladmission.NewDecoder(mgr.GetScheme()),
			DenyOnOrphaned: true,
		}
		mgr.GetWebhookServer().Register(webhook.PathPDRouterMutate, &ctrladmission.Webhook{Handler: h})
		setupLog.Info("PDRouter webhook registered",
			"task", "P5-T-103",
			"path", webhook.PathPDRouterMutate,
			"port", 9443,
			"deny-on-orphaned", true)
	}

	// Phase 8 P8-T-006: Construct the busy-idle metrics Ingestor.
	// The T007 NPUVerticalScaler controller will consume this. Reading
	// chart-injected env vars: NPUVERTICAL_SCALER_PROMETHEUS_URL +
	// NPUVERTICAL_SCALER_QUERY_TIMEOUT (per ADR-0012 §1 reconcile step 3
	// contract — empty URL → ingestor returns NoData=true for every
	// query; controller treats NoData as "no scaling decision this tick").
	promURL := os.Getenv("NPUVERTICAL_SCALER_PROMETHEUS_URL")
	queryTimeout := 5 * time.Second
	if v := os.Getenv("NPUVERTICAL_SCALER_QUERY_TIMEOUT"); v != "" {
		if d, perr := time.ParseDuration(v); perr == nil {
			queryTimeout = d
		}
	}
	ingestor := metrics.NewPrometheusIngestor(metrics.IngestorOpts{
		PrometheusURL: promURL,
		QueryTimeout:  queryTimeout,
	})
	setupLog.Info("Busy-idle metrics ingestor constructed",
		"task", "P8-T-006",
		"prometheusURL", promURL,
		"queryTimeout", queryTimeout.String(),
		"degraded", promURL == "")
	// Phase 8 P8-T-007: Register NPUVerticalScalerReconciler with the
	// manager. Watches NPUVerticalScaler · patches target ModelService
	// annotation per ADR-0012 §5 mutation model · drives 8-step reconcile
	// loop (Get scaler → Get target → Query Ingestor → decide → cooldown
	// → patch annotation → record ScaleEvent → update status).
	nvsr := &controller.NPUVerticalScalerReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("npuverticalscaler-controller"),
		Ingestor: ingestor,
	}
	if err := nvsr.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "Failed to register NPUVerticalScalerReconciler")
		os.Exit(1)
	}
	setupLog.Info("NPUVerticalScalerReconciler registered", "task", "P8-T-007")

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
		"latest-task", "P5-T-102",
		"modelservice-controller-enabled", enableModelServiceController,
		"pd-router-webhook-enabled", enablePDRouterWebhook)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Failed to run manager")
		os.Exit(1)
	}
}
