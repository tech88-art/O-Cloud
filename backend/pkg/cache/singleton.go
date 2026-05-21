// Package cache singleton wrapper — Phase 10 P10-T-006 demo-backend cache
// impl per ADR-0015 §3.3 singleton with active-active failover.
//
// The wrapper coordinates with a K8s Lease object so multi-replica demo-backend
// deployments elect a single active leader. The leader serves traffic through
// the in-process LRU cache; followers are standbys ready to take over via
// the same Lease watch mechanism `sigs.k8s.io/controller-runtime/pkg/
// leaderelection` provides.
//
// Per ADR-0015 §2 Decision A-D:
//   - **A**: §3.3 singleton with active-active failover path (terminology:
//     active-standby in steady state · single active per site · backups
//     standby for failover).
//   - **B**: K8s Lease via controller-runtime defaults — LeaseDuration 15s,
//     RenewDeadline 10s, RetryPeriod 2s. Lease namespace `ocloud-system`,
//     resource `demo-backend-leader`.
//   - **C**: chart values.replicas: 2 default · degraded read-only mode on
//     Lease error (graceful fallback returning cached data with
//     `X-Cache-Status: stale` response header).
//   - **D**: Prometheus metrics surface `demo_backend_lease_holder` gauge,
//     `demo_backend_lease_renewals_total` counter, `demo_backend_cache_hit_ratio`
//     gauge (see backend/pkg/api/prom_metrics.go MetricLeaseHolder + co.).
//
// This singleton.go ships the **wrapper + Lease config plumbing**. Wiring into
// `cmd/demo-backend/main.go` startup orchestration is deferred to a follow-up
// task (per T006 devlog scope adaptation) — chart deployment / chart-bundled
// RBAC / replicas flip is also deferred because the demo-backend helm chart
// does not yet exist (P10-T-006 plan literal references a chart not yet
// created). Phase 10 polish or Phase 11+ chart packaging can pick this up.
package cache

import (
	"context"
	"errors"
	"sync"
	"time"
)

// LeaseConfig describes how the singleton wrapper requests + renews a K8s
// Lease for leader election. Values mirror controller-runtime defaults
// unless callers override.
type LeaseConfig struct {
	// Namespace is the K8s namespace hosting the Lease object. Defaults
	// to `ocloud-system` per ADR-0015 §2 Decision B if empty.
	Namespace string

	// LeaseName is the `coordination.k8s.io/v1.Lease` object name. Defaults
	// to `demo-backend-leader` per ADR-0015 §2 Decision B if empty.
	LeaseName string

	// HolderIdentity is the unique identity string the leader publishes
	// in Lease.Spec.HolderIdentity. Per ADR-0015 §2 Decision B the demo
	// backend uses Pod name (downward API → POD_NAME env var). Empty
	// triggers a hostname-fallback to keep unit tests viable.
	HolderIdentity string

	// LeaseDuration is the leader's claim on the Lease. Default 15s per
	// controller-runtime convention; setting it lower trims failover
	// latency at the cost of K8s API server load.
	LeaseDuration time.Duration

	// RenewDeadline is the deadline by which the leader must renew the
	// Lease or lose leadership. Default 10s per controller-runtime.
	RenewDeadline time.Duration

	// RetryPeriod is the cadence at which non-leader replicas check the
	// Lease for expiry to attempt acquisition. Default 2s per
	// controller-runtime.
	RetryPeriod time.Duration
}

// DefaultLeaseConfig returns the ADR-0015 §2 Decision B baseline
// configuration. Callers may copy + mutate for custom tuning per ADR-0015
// §4 Open question (b) failover SLA threshold revisit.
func DefaultLeaseConfig() LeaseConfig {
	return LeaseConfig{
		Namespace:     "ocloud-system",
		LeaseName:     "demo-backend-leader",
		LeaseDuration: 15 * time.Second,
		RenewDeadline: 10 * time.Second,
		RetryPeriod:   2 * time.Second,
	}
}

// Validate returns an error when LeaseConfig fields are inconsistent. Per
// controller-runtime convention: LeaseDuration > RenewDeadline > RetryPeriod.
// Empty Namespace / LeaseName trigger defaulting, NOT a validation error
// (callers can fill from chart values or hardcode in tests).
func (c LeaseConfig) Validate() error {
	if c.LeaseDuration <= 0 {
		return errors.New("cache.LeaseConfig: LeaseDuration must be > 0")
	}
	if c.RenewDeadline <= 0 {
		return errors.New("cache.LeaseConfig: RenewDeadline must be > 0")
	}
	if c.RetryPeriod <= 0 {
		return errors.New("cache.LeaseConfig: RetryPeriod must be > 0")
	}
	if c.LeaseDuration <= c.RenewDeadline {
		return errors.New("cache.LeaseConfig: LeaseDuration must be > RenewDeadline (controller-runtime invariant)")
	}
	if c.RenewDeadline <= c.RetryPeriod {
		return errors.New("cache.LeaseConfig: RenewDeadline must be > RetryPeriod (controller-runtime invariant)")
	}
	return nil
}

// SingletonState tracks the current Lease-holder status of a singleton
// cache wrapper. Read by metrics handlers + degraded-read-only logic.
type SingletonState int

const (
	// SingletonStateFollower indicates the replica is healthy but NOT
	// currently holding the Lease. Cache reads short-circuit to fallback
	// (datasource direct) or 503 depending on chart configuration.
	SingletonStateFollower SingletonState = iota

	// SingletonStateLeader indicates the replica holds the Lease. Cache
	// reads/writes operate normally; metrics `demo_backend_lease_holder`
	// gauge = 1 on this instance.
	SingletonStateLeader

	// SingletonStateDegraded indicates the replica HELD the Lease but
	// most-recent renewal attempts errored. Cache reads continue to serve
	// stale data with `X-Cache-Status: stale` response header injection;
	// mutating endpoints return 503 + Retry-After (per ADR-0015 §2
	// Decision C).
	SingletonStateDegraded
)

// String renders SingletonState for log lines + metric label values.
func (s SingletonState) String() string {
	switch s {
	case SingletonStateFollower:
		return "follower"
	case SingletonStateLeader:
		return "leader"
	case SingletonStateDegraded:
		return "degraded"
	default:
		return "unknown"
	}
}

// Singleton wraps a Cache (or Catalog) instance with Lease-election state
// tracking. The actual K8s Lease acquisition is delegated to
// `sigs.k8s.io/controller-runtime/pkg/leaderelection` at main.go wiring time
// — this wrapper holds the resulting state + drives metric emission +
// degraded read-only mode decisions.
type Singleton struct {
	mu       sync.RWMutex
	state    SingletonState
	leaseCfg LeaseConfig

	// lastTransition is the wall-clock time of the most recent state
	// change. Used by `LastLeaseTime` for ops insight + metric annotation.
	lastTransition time.Time
}

// NewSingleton constructs a Singleton in Follower state per ADR-0015
// §3.1 Phase 10 W1 impl path. Lease acquisition begins at main.go startup
// when the controller-runtime leader-elect goroutine fires.
func NewSingleton(cfg LeaseConfig) (*Singleton, error) {
	// Default fill-in for chart-friendly empty values.
	if cfg.Namespace == "" {
		cfg.Namespace = "ocloud-system"
	}
	if cfg.LeaseName == "" {
		cfg.LeaseName = "demo-backend-leader"
	}
	if cfg.LeaseDuration == 0 {
		cfg.LeaseDuration = 15 * time.Second
	}
	if cfg.RenewDeadline == 0 {
		cfg.RenewDeadline = 10 * time.Second
	}
	if cfg.RetryPeriod == 0 {
		cfg.RetryPeriod = 2 * time.Second
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Singleton{
		state:          SingletonStateFollower,
		leaseCfg:       cfg,
		lastTransition: time.Now(),
	}, nil
}

// State returns the current SingletonState snapshot.
func (s *Singleton) State() SingletonState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// IsLeader returns true when the wrapper holds the Lease (steady state +
// degraded). Per ADR-0015 §2 Decision C, degraded read-only mode is still
// "leader-held" — the leader is alive but can't renew, so cached reads
// keep flowing while mutating writes fail-fast.
func (s *Singleton) IsLeader() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state == SingletonStateLeader || s.state == SingletonStateDegraded
}

// IsDegraded returns true when the wrapper is in degraded read-only mode.
// Middleware uses this to inject `X-Cache-Status: stale` response header.
func (s *Singleton) IsDegraded() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state == SingletonStateDegraded
}

// LastTransition returns the wall-clock time of the most recent state
// change. Used by metrics + ops diagnostic logs.
func (s *Singleton) LastTransition() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastTransition
}

// LeaseConfig returns a copy of the active Lease configuration. Used by
// log lines + ops introspection endpoints.
func (s *Singleton) LeaseConfig() LeaseConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.leaseCfg
}

// transitionTo updates the singleton state + emits a transition log line.
// Caller MUST hold no other lock on this Singleton. Returns the prior
// state for caller's diff logging.
func (s *Singleton) transitionTo(next SingletonState) SingletonState {
	s.mu.Lock()
	defer s.mu.Unlock()
	prior := s.state
	s.state = next
	s.lastTransition = time.Now()
	return prior
}

// OnLeaseAcquired is the callback wired into controller-runtime's
// `OnStartedLeading` hook. Caller (main.go) invokes this from the
// leader-elect goroutine when the replica wins the Lease.
func (s *Singleton) OnLeaseAcquired(_ context.Context) SingletonState {
	return s.transitionTo(SingletonStateLeader)
}

// OnLeaseLost is the callback wired into controller-runtime's
// `OnStoppedLeading` hook. Used when the leader voluntarily releases (graceful
// shutdown) OR loses leadership to a competing replica.
func (s *Singleton) OnLeaseLost() SingletonState {
	return s.transitionTo(SingletonStateFollower)
}

// OnLeaseRenewError is invoked when a Lease renew API call fails. After
// 3 consecutive failures within RenewDeadline, the wrapper flips to
// Degraded — cached reads continue, mutating writes return 503. The
// 3-failure threshold is a controller-runtime convention (Lease library
// internally retries with backoff but the wrapper still observes errors
// for metric counting per `demo_backend_lease_renewals_total{result=error}`).
func (s *Singleton) OnLeaseRenewError(_ error) SingletonState {
	// Phase 10 W1 minimum: flip Leader → Degraded on first error.
	// Phase 10 polish can add the 3-failure threshold + backoff window per
	// ADR-0015 §4 Open question (b) revisit.
	s.mu.RLock()
	prior := s.state
	s.mu.RUnlock()
	if prior == SingletonStateLeader {
		return s.transitionTo(SingletonStateDegraded)
	}
	return prior
}

// OnLeaseRenewRecovered is invoked when a Lease renew succeeds after a
// degraded streak. Flips Degraded → Leader. No-op if not in degraded.
func (s *Singleton) OnLeaseRenewRecovered() SingletonState {
	s.mu.RLock()
	prior := s.state
	s.mu.RUnlock()
	if prior == SingletonStateDegraded {
		return s.transitionTo(SingletonStateLeader)
	}
	return prior
}
