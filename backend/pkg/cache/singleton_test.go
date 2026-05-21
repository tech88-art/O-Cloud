// Package cache singleton unit tests — Phase 10 P10-T-006.
//
// Per ADR-0015 §3.1 Phase 10 W1 acceptance: 4 sanity cases covering Lease
// acquired / lost / re-acquired transitions + LRU eviction preserved. Real
// K8s Lease acquisition is delegated to controller-runtime in main.go
// wiring (deferred to follow-up · T006 devlog scope adaptation); this test
// suite covers the Singleton state machine + LeaseConfig defaulting +
// validation, which are the pieces this commit ships.

package cache

import (
	"context"
	"testing"
	"time"
)

// TestDefaultLeaseConfig verifies the ADR-0015 §2 Decision B baseline:
//   - LeaseDuration 15s
//   - RenewDeadline 10s
//   - RetryPeriod 2s
//   - Namespace ocloud-system
//   - LeaseName demo-backend-leader
func TestDefaultLeaseConfig(t *testing.T) {
	cfg := DefaultLeaseConfig()
	if cfg.LeaseDuration != 15*time.Second {
		t.Fatalf("LeaseDuration = %v, want 15s (ADR-0015 §2 Decision B)", cfg.LeaseDuration)
	}
	if cfg.RenewDeadline != 10*time.Second {
		t.Fatalf("RenewDeadline = %v, want 10s", cfg.RenewDeadline)
	}
	if cfg.RetryPeriod != 2*time.Second {
		t.Fatalf("RetryPeriod = %v, want 2s", cfg.RetryPeriod)
	}
	if cfg.Namespace != "ocloud-system" {
		t.Fatalf("Namespace = %q, want \"ocloud-system\"", cfg.Namespace)
	}
	if cfg.LeaseName != "demo-backend-leader" {
		t.Fatalf("LeaseName = %q, want \"demo-backend-leader\"", cfg.LeaseName)
	}
}

// TestLeaseConfigValidate covers the controller-runtime invariant chain
// (LeaseDuration > RenewDeadline > RetryPeriod) + the zero-value rejection.
func TestLeaseConfigValidate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     LeaseConfig
		wantErr bool
	}{
		{"valid defaults", DefaultLeaseConfig(), false},
		{"zero LeaseDuration", LeaseConfig{}, true},
		{"LeaseDuration ≤ RenewDeadline", LeaseConfig{LeaseDuration: 10 * time.Second, RenewDeadline: 15 * time.Second, RetryPeriod: 2 * time.Second}, true},
		{"RenewDeadline ≤ RetryPeriod", LeaseConfig{LeaseDuration: 15 * time.Second, RenewDeadline: 2 * time.Second, RetryPeriod: 3 * time.Second}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.cfg.Validate()
			if (err != nil) != c.wantErr {
				t.Fatalf("Validate() error = %v, wantErr = %v", err, c.wantErr)
			}
		})
	}
}

// TestSingletonStateTransitions covers the 4 ADR-0015 §3.1 P10-T-006
// acceptance sanity cases adapted to Singleton state machine API:
//
//	(1) Lease acquired → SingletonStateLeader, IsLeader=true
//	(2) Lease renew error → SingletonStateDegraded, IsLeader=true, IsDegraded=true
//	(3) Lease renew recovered → SingletonStateLeader, IsDegraded=false
//	(4) Lease lost → SingletonStateFollower, IsLeader=false
//
// Real K8s Lease acquisition + controller-runtime wiring is covered by
// kind smoke phase5+ post-tag CI gate (per `feedback_post_tag_ci_gate`).
func TestSingletonStateTransitions(t *testing.T) {
	s, err := NewSingleton(DefaultLeaseConfig())
	if err != nil {
		t.Fatalf("NewSingleton: %v", err)
	}

	// Initial state = Follower.
	if got := s.State(); got != SingletonStateFollower {
		t.Fatalf("initial State() = %v, want Follower", got)
	}
	if s.IsLeader() {
		t.Fatal("initial IsLeader() = true, want false")
	}

	// (1) Lease acquired → Leader.
	prior := s.OnLeaseAcquired(context.Background())
	if prior != SingletonStateFollower {
		t.Fatalf("OnLeaseAcquired prior = %v, want Follower", prior)
	}
	if got := s.State(); got != SingletonStateLeader {
		t.Fatalf("post-acquired State() = %v, want Leader", got)
	}
	if !s.IsLeader() {
		t.Fatal("post-acquired IsLeader() = false, want true")
	}
	if s.IsDegraded() {
		t.Fatal("post-acquired IsDegraded() = true, want false")
	}

	// (2) Lease renew error → Degraded.
	s.OnLeaseRenewError(context.DeadlineExceeded)
	if got := s.State(); got != SingletonStateDegraded {
		t.Fatalf("post-renew-error State() = %v, want Degraded", got)
	}
	if !s.IsLeader() {
		t.Fatal("degraded IsLeader() = false, want true (still serves cached reads)")
	}
	if !s.IsDegraded() {
		t.Fatal("degraded IsDegraded() = false, want true")
	}

	// (3) Lease renew recovered → Leader.
	s.OnLeaseRenewRecovered()
	if got := s.State(); got != SingletonStateLeader {
		t.Fatalf("post-recovered State() = %v, want Leader", got)
	}
	if s.IsDegraded() {
		t.Fatal("recovered IsDegraded() = true, want false")
	}

	// (4) Lease lost → Follower.
	s.OnLeaseLost()
	if got := s.State(); got != SingletonStateFollower {
		t.Fatalf("post-lost State() = %v, want Follower", got)
	}
	if s.IsLeader() {
		t.Fatal("post-lost IsLeader() = true, want false")
	}
}

// TestSingletonLastTransitionMovesOnStateChange ensures the wall-clock
// LastTransition timestamp updates on transition + stays stable otherwise.
// Used by metrics emission + ops diagnostic.
func TestSingletonLastTransitionMovesOnStateChange(t *testing.T) {
	s, err := NewSingleton(DefaultLeaseConfig())
	if err != nil {
		t.Fatalf("NewSingleton: %v", err)
	}
	t0 := s.LastTransition()

	// Sleep ensures clock has moved by at least 1ms.
	time.Sleep(1 * time.Millisecond)
	s.OnLeaseAcquired(context.Background())
	t1 := s.LastTransition()
	if !t1.After(t0) {
		t.Fatalf("LastTransition() did not advance: t0=%v t1=%v", t0, t1)
	}

	// State() call alone does NOT bump LastTransition.
	_ = s.State()
	t2 := s.LastTransition()
	if !t2.Equal(t1) {
		t.Fatalf("LastTransition() advanced without state change: t1=%v t2=%v", t1, t2)
	}
}

// TestSingletonStateString verifies log-line + metric label rendering.
func TestSingletonStateString(t *testing.T) {
	cases := []struct {
		s    SingletonState
		want string
	}{
		{SingletonStateFollower, "follower"},
		{SingletonStateLeader, "leader"},
		{SingletonStateDegraded, "degraded"},
		{SingletonState(99), "unknown"},
	}
	for _, c := range cases {
		if got := c.s.String(); got != c.want {
			t.Errorf("SingletonState(%d).String() = %q, want %q", c.s, got, c.want)
		}
	}
}
