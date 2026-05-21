/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

// Package ratealgo implements the token-bucket rate-limiting algorithm
// option for Quota admission per ADR-0014 §7 forward note (Phase 10
// P10-T-104 polish wave 1).
//
// Phase 9 P9-T-006 ships sliding-window only(`scale_events_in_last_5min ≤ N`)。
// Phase 10 polish adds token-bucket as enum option in
// `Quota.spec.enforcement.rateAlgorithm`:
//   - sliding-window(default · Phase 9 backwards compat per ADR-0014 §3)
//   - token-bucket(NEW · Phase 10 W1 opt-in via Quota.spec)
//
// Token-bucket semantics:
//   - Bucket capacity = MaxRate(events per window)
//   - Refill rate = MaxRate / WindowDuration(events per second)
//   - Each event consumes 1 token; admission denied when bucket empty
//   - Sliding-window is calendar-based(strict per-window count);
//     token-bucket allows bursts at the cost of average rate compliance
//
// Phase 10 W1 substrate: algorithm impl + 4 unit tests. Phase 10 W2
// polish integration: webhook admission path 调 RateLimiter.Allow() →
// admit / deny + emit metric per algorithm choice.
package ratealgo

import (
	"sync"
	"time"
)

// TokenBucket implements a thread-safe token-bucket rate limiter.
type TokenBucket struct {
	mu sync.Mutex

	// capacity is the maximum tokens in the bucket(burst cap).
	capacity float64

	// refillRate is tokens added per second.
	refillRate float64

	// tokens is the current bucket contents.
	tokens float64

	// lastRefill is the wall-clock time of the most recent refill.
	lastRefill time.Time
}

// NewTokenBucket constructs a token bucket sized for a maxEvents per window.
//
//	capacity = maxEvents
//	refillRate = maxEvents / window_in_seconds
//	tokens starts full
func NewTokenBucket(maxEvents int, window time.Duration) *TokenBucket {
	if maxEvents <= 0 {
		maxEvents = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	cap := float64(maxEvents)
	return &TokenBucket{
		capacity:   cap,
		refillRate: cap / window.Seconds(),
		tokens:     cap,
		lastRefill: time.Now(),
	}
}

// Allow attempts to consume 1 token. Returns true when permitted, false
// when bucket empty. Per ADR-0014 §7 token-bucket option · suitable for
// burst-tolerant admission(ad-hoc scale events admitted even if
// average rate near cap).
func (b *TokenBucket) Allow() bool {
	return b.AllowN(1)
}

// AllowN attempts to consume n tokens. Same semantics as Allow.
func (b *TokenBucket) AllowN(n int) bool {
	if n <= 0 {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refillLocked(time.Now())
	if b.tokens >= float64(n) {
		b.tokens -= float64(n)
		return true
	}
	return false
}

// Tokens returns the current bucket contents(approximate · without
// holding the lock too long).
func (b *TokenBucket) Tokens() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refillLocked(time.Now())
	return b.tokens
}

// Capacity returns the maximum burst capacity.
func (b *TokenBucket) Capacity() float64 {
	return b.capacity
}

// RefillRate returns the tokens-per-second refill rate.
func (b *TokenBucket) RefillRate() float64 {
	return b.refillRate
}

// refillLocked applies time-elapsed token replenishment. Caller MUST
// hold the mutex.
func (b *TokenBucket) refillLocked(now time.Time) {
	elapsed := now.Sub(b.lastRefill).Seconds()
	if elapsed <= 0 {
		return
	}
	add := elapsed * b.refillRate
	b.tokens += add
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	b.lastRefill = now
}
