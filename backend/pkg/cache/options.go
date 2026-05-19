// Package cache enhanced API (P3-T-008).
//
// Adds TTL eviction + capacity-based LRU eviction + per-key eviction
// callback to the Phase-1 NewLRU primitive. Existing NewLRU stays
// supported as a thin wrapper around New for back-compat.
package cache

import (
	"errors"
	"fmt"
	"time"
)

// Options configures a cache. Pass to New() to construct an LRU with
// non-default behavior.
type Options struct {
	// MaxEntries is the hard upper bound on entries; further inserts
	// evict the oldest by LRU order. Must be > 0.
	MaxEntries int

	// TTL is the soft upper bound on entry age. Get() returns a miss
	// (and triggers eviction) on entries older than TTL. Zero TTL = no
	// TTL bound.
	TTL time.Duration

	// OnEvict is invoked synchronously after each eviction (TTL,
	// capacity, or explicit). Receives the key + value + reason.
	// nil = no callback.
	OnEvict func(key any, value any, reason EvictReason)
}

// EvictReason annotates the cause of an eviction passed to OnEvict.
type EvictReason int

const (
	// EvictReasonCapacity indicates the eviction was forced by MaxEntries.
	EvictReasonCapacity EvictReason = iota
	// EvictReasonTTL indicates the eviction was triggered by TTL expiry.
	EvictReasonTTL
	// EvictReasonExplicit indicates Remove() or Purge() was called.
	EvictReasonExplicit
)

// String renders EvictReason for log lines.
func (r EvictReason) String() string {
	switch r {
	case EvictReasonCapacity:
		return "capacity"
	case EvictReasonTTL:
		return "ttl"
	case EvictReasonExplicit:
		return "explicit"
	default:
		return fmt.Sprintf("unknown(%d)", int(r))
	}
}

// Validate checks for legal options. Called by New.
func (o Options) Validate() error {
	if o.MaxEntries <= 0 {
		return errors.New("cache: MaxEntries must be > 0")
	}
	if o.TTL < 0 {
		return errors.New("cache: TTL must be >= 0 (use 0 to disable TTL)")
	}
	return nil
}
