// Package cache wraps hashicorp/golang-lru/v2 with a typed, ergonomic façade.
//
// Phase-1 use cases: hot topology JSON, preset catalog. We never use this as
// authoritative storage — sources remain the single source of truth.
//
// P3-T-008 extension: TTL eviction + per-key OnEvict callback + atomic
// eviction counter. NewLRU stays as a back-compat alias to New.
package cache

import (
	"fmt"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
)

// LRU is a thin typed wrapper around hashicorp's LRU cache, with optional
// TTL eviction and an OnEvict callback layered on top.
//
// All mutating operations (Add / Get-with-TTL / Remove / Purge) serialize
// through mu so the hashicorp-driven onEvicted callback can read a stable
// pendingReason. Concurrent reads via Len() / Counter() are safe without
// holding mu (Len delegates to hashicorp's internal lock; Counter returns
// a pointer with its own atomic semantics).
type LRU[K comparable, V any] struct {
	c    *lru.Cache[K, V]
	opts Options

	mu sync.Mutex
	// addedAt tracks insertion timestamps for TTL bookkeeping; nil if
	// opts.TTL == 0. Guarded by mu.
	addedAt map[K]time.Time
	// pendingReason tells the hashicorp-driven onEvicted callback what
	// reason to record. Default is EvictReasonCapacity (the natural
	// reason when hashicorp's Add() bumps the oldest entry). Remove,
	// Purge, and the TTL path set this before triggering hashicorp's
	// eviction. Guarded by mu, but the callback reads it within the
	// same locked region.
	pendingReason EvictReason

	counter *EvictionCounter
}

// NewLRU constructs an LRU with the given max entries (>0 required).
// Retained for back-compat; equivalent to New(Options{MaxEntries: size}).
func NewLRU[K comparable, V any](size int) (*LRU[K, V], error) {
	if size <= 0 {
		return nil, fmt.Errorf("cache size must be > 0, got %d", size)
	}
	return New[K, V](Options{MaxEntries: size})
}

// New constructs an LRU with full Options control.
func New[K comparable, V any](opts Options) (*LRU[K, V], error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	out := &LRU[K, V]{
		opts:          opts,
		counter:       &EvictionCounter{},
		pendingReason: EvictReasonCapacity,
	}
	if opts.TTL > 0 {
		out.addedAt = make(map[K]time.Time)
	}

	// Wire hashicorp's per-eviction callback into our counter + OnEvict
	// + optional Prometheus instrumentation (P4-T-008).
	// hashicorp invokes this synchronously inside Add/Remove/Purge while
	// the caller still holds our mu (we serialize all mutating paths).
	onEvicted := func(k K, v V) {
		reason := out.pendingReason
		out.counter.Inc(reason)
		if out.addedAt != nil {
			delete(out.addedAt, k)
		}
		if opts.EvictionCounterVec != nil {
			opts.EvictionCounterVec.WithLabelValues(opts.Resource).Inc()
		}
		if opts.OnEvict != nil {
			opts.OnEvict(k, v, reason)
		}
	}

	c, err := lru.NewWithEvict[K, V](opts.MaxEntries, onEvicted)
	if err != nil {
		return nil, fmt.Errorf("create lru: %w", err)
	}
	out.c = c
	return out, nil
}

// Counter returns the eviction counter (lifetime totals).
func (c *LRU[K, V]) Counter() *EvictionCounter { return c.counter }

// Get returns the cached value if present and not TTL-expired. On TTL
// expiry, evicts the entry + records the eviction. On hit, increments
// the optional HitsCounterVec (P4-T-008).
//
// Get does NOT take mu in the common (non-expired) path; it only locks
// when a TTL check fails so a stale entry must be evicted.
func (c *LRU[K, V]) Get(k K) (V, bool) {
	var zero V
	if c.opts.TTL > 0 {
		c.mu.Lock()
		addedAt, ok := c.addedAt[k]
		if ok && time.Since(addedAt) > c.opts.TTL {
			// TTL expired — drop the entry under our lock so the
			// onEvicted callback observes pendingReason = TTL.
			c.pendingReason = EvictReasonTTL
			c.c.Remove(k)
			c.pendingReason = EvictReasonCapacity
			c.mu.Unlock()
			return zero, false
		}
		c.mu.Unlock()
	}
	v, ok := c.c.Get(k)
	if ok && c.opts.HitsCounterVec != nil {
		c.opts.HitsCounterVec.WithLabelValues(c.opts.Resource).Inc()
	}
	return v, ok
}

// Add inserts or replaces v for k. Records timestamp for TTL.
// Returns true if the insert evicted an entry by capacity.
func (c *LRU[K, V]) Add(k K, v V) (evicted bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.opts.TTL > 0 {
		c.addedAt[k] = time.Now()
	}
	// pendingReason defaults to Capacity; hashicorp's Add fires
	// onEvicted with that reason when the LRU is full.
	return c.c.Add(k, v)
}

// Remove deletes the entry. Returns true if present, and records the
// eviction reason as Explicit.
func (c *LRU[K, V]) Remove(k K) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pendingReason = EvictReasonExplicit
	present := c.c.Remove(k)
	c.pendingReason = EvictReasonCapacity
	return present
}

// Len returns the current number of entries. Delegates to hashicorp's
// internal lock; safe without our mu.
func (c *LRU[K, V]) Len() int { return c.c.Len() }

// Purge empties the cache. Each entry counts as an explicit eviction.
func (c *LRU[K, V]) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pendingReason = EvictReasonExplicit
	c.c.Purge()
	c.pendingReason = EvictReasonCapacity
}
