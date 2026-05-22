// Package cache leader-elect runner — Phase 11 P11-T-003 chart wiring per
// ADR-0015 §2 Decision B + ADR-0017 §2 Decision D (chart packaging spine
// 1st priority).
//
// RunLeaderElection wires a Singleton into client-go's Lease leader-elect
// loop. Caller (cmd/demo-backend/main.go) launches it in a goroutine; the
// loop blocks until ctx is canceled or the Lease library exits.
//
// The Lease lives at coordination.k8s.io/v1 Lease `<LeaseName>` in
// `<Namespace>` (defaults `demo-backend-leader` / `ocloud-system`). The
// HolderIdentity defaults to POD_NAME env (downward API · per chart
// templates/deployment.yaml) with hostname fallback so unit tests work.
//
// On leader transitions the loop calls Singleton.OnLeaseAcquired /
// OnLeaseLost — Singleton state machine then drives demo-backend's
// degraded read-only mode + Prometheus metric emission per ADR-0015 §3.3
// Decision C+D.
package cache

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

// LeaderElectOptions bundles the inputs RunLeaderElection needs. Most
// fields default to ADR-0015 §2 Decision B values when zero/empty.
type LeaderElectOptions struct {
	// KubeconfigPath is the path to a kubeconfig file. Empty triggers
	// in-cluster config (rest.InClusterConfig — picks up SA mounted at
	// /var/run/secrets/kubernetes.io/serviceaccount).
	KubeconfigPath string

	// LeaseConfig drives the K8s Lease resource lock parameters. If
	// HolderIdentity is empty, POD_NAME env → os.Hostname fallback.
	LeaseConfig LeaseConfig

	// Logger receives transition events. Required (no fallback).
	Logger *zap.Logger
}

// resolveHolderIdentity prefers explicit Cfg.HolderIdentity, then the
// POD_NAME env var (chart downward API), finally os.Hostname.
func resolveHolderIdentity(cfg LeaseConfig) (string, error) {
	if cfg.HolderIdentity != "" {
		return cfg.HolderIdentity, nil
	}
	if v := os.Getenv("POD_NAME"); v != "" {
		return v, nil
	}
	h, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("resolve holder identity (no Cfg / POD_NAME / hostname): %w", err)
	}
	return h, nil
}

// buildKubeClient constructs a kubernetes Clientset from a kubeconfig
// path (or in-cluster SA if empty). Centralised here so callers don't
// re-implement the fallback ladder.
func buildKubeClient(kubeconfigPath string) (kubernetes.Interface, error) {
	var (
		cfg *rest.Config
		err error
	)
	if kubeconfigPath != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("build config from %q: %w", kubeconfigPath, err)
		}
	} else {
		cfg, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("in-cluster config: %w", err)
		}
	}
	cli, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("kubernetes.NewForConfig: %w", err)
	}
	return cli, nil
}

// RunLeaderElection blocks running the leader-elect loop bound to the
// supplied Singleton. Caller is responsible for launching this in a
// goroutine and providing a ctx that the demo-backend lifecycle owns
// (e.g. signal-cancel ctx from main.go).
//
// Returns nil on graceful ctx cancellation; non-nil if the leader-elect
// library exits with error (in which case main.go should log + propagate).
func RunLeaderElection(ctx context.Context, s *Singleton, opts LeaderElectOptions) error {
	if s == nil {
		return fmt.Errorf("RunLeaderElection: Singleton is nil")
	}
	if opts.Logger == nil {
		return fmt.Errorf("RunLeaderElection: Logger is required")
	}

	leaseCfg := s.LeaseConfig()
	holder, err := resolveHolderIdentity(leaseCfg)
	if err != nil {
		return err
	}

	cli, err := buildKubeClient(opts.KubeconfigPath)
	if err != nil {
		return fmt.Errorf("build kube client: %w", err)
	}

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Namespace: leaseCfg.Namespace,
			Name:      leaseCfg.LeaseName,
		},
		Client: cli.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: holder,
		},
	}

	cfg := leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   leaseCfg.LeaseDuration,
		RenewDeadline:   leaseCfg.RenewDeadline,
		RetryPeriod:     leaseCfg.RetryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(c context.Context) {
				prior := s.OnLeaseAcquired(c)
				opts.Logger.Info("lease acquired",
					zap.String("prior_state", prior.String()),
					zap.String("lease_namespace", leaseCfg.Namespace),
					zap.String("lease_name", leaseCfg.LeaseName),
					zap.String("holder", holder))
			},
			OnStoppedLeading: func() {
				prior := s.OnLeaseLost()
				opts.Logger.Info("lease lost",
					zap.String("prior_state", prior.String()),
					zap.String("lease_namespace", leaseCfg.Namespace),
					zap.String("lease_name", leaseCfg.LeaseName),
					zap.String("holder", holder))
			},
			OnNewLeader: func(identity string) {
				if identity == holder {
					return // self-elected — covered by OnStartedLeading
				}
				opts.Logger.Info("new leader observed",
					zap.String("leader", identity),
					zap.String("self", holder))
			},
		},
		Name: leaseCfg.LeaseName,
	}

	// leaderelection.RunOrDie panics on bad config (we've validated
	// upstream via LeaseConfig.Validate()) and blocks otherwise.
	leaderelection.RunOrDie(ctx, cfg)
	return nil
}

// SetLeaseHolderIdentity is a test-helper that lets unit tests inject a
// deterministic holder without setting POD_NAME globally. Returns the
// LeaseConfig copy with HolderIdentity set.
func SetLeaseHolderIdentity(cfg LeaseConfig, holder string) LeaseConfig {
	cfg.HolderIdentity = holder
	return cfg
}

// ConfigFromOptions builds a cache.LeaseConfig from chart-style integer
// seconds + namespace / name strings. main.go uses this to convert the
// Viper-loaded backend/pkg/config.LeaseConfig into the cache.LeaseConfig
// shape (which uses time.Duration). Returns DefaultLeaseConfig overrides
// only for non-zero fields.
func ConfigFromOptions(namespace, name string, durationSeconds, renewSeconds, retrySeconds int) LeaseConfig {
	cfg := DefaultLeaseConfig()
	if namespace != "" {
		cfg.Namespace = namespace
	}
	if name != "" {
		cfg.LeaseName = name
	}
	if durationSeconds > 0 {
		cfg.LeaseDuration = time.Duration(durationSeconds) * time.Second
	}
	if renewSeconds > 0 {
		cfg.RenewDeadline = time.Duration(renewSeconds) * time.Second
	}
	if retrySeconds > 0 {
		cfg.RetryPeriod = time.Duration(retrySeconds) * time.Second
	}
	return cfg
}
