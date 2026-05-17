// Package cache wraps hashicorp/golang-lru/v2 with a typed, ergonomic façade.
//
// Phase-1 use cases: hot topology JSON, preset catalog. We never use this as
// authoritative storage — sources remain the single source of truth.
package cache

import (
	"fmt"

	lru "github.com/hashicorp/golang-lru/v2"
)

// LRU is a thin typed wrapper around hashicorp's LRU cache.
type LRU[K comparable, V any] struct {
	c *lru.Cache[K, V]
}

// NewLRU constructs an LRU with the given max entries (>0 required).
func NewLRU[K comparable, V any](size int) (*LRU[K, V], error) {
	if size <= 0 {
		return nil, fmt.Errorf("cache size must be > 0, got %d", size)
	}
	c, err := lru.New[K, V](size)
	if err != nil {
		return nil, fmt.Errorf("create lru: %w", err)
	}
	return &LRU[K, V]{c: c}, nil
}

// Get returns the cached value if present.
func (c *LRU[K, V]) Get(k K) (V, bool) {
	return c.c.Get(k)
}

// Add inserts or replaces v for k. Returns true if the insert evicted an entry.
func (c *LRU[K, V]) Add(k K, v V) (evicted bool) {
	return c.c.Add(k, v)
}

// Remove deletes the entry. Returns true if it was present.
func (c *LRU[K, V]) Remove(k K) bool {
	return c.c.Remove(k)
}

// Len is the current number of entries.
func (c *LRU[K, V]) Len() int {
	return c.c.Len()
}

// Purge empties the cache.
func (c *LRU[K, V]) Purge() {
	c.c.Purge()
}
