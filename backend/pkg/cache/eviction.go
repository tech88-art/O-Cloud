// eviction.go owns the eviction-counter primitives plus the per-resource
// catalog wired from configs/config.yaml `cache:` block.
package cache

import (
	"sync"
	"sync/atomic"
)

// EvictionCounter is a process-wide atomic counter snapshot. Wired by
// every TTL/capacity/explicit eviction; readable via Total() / ByReason().
// Phase 5+: a Prometheus-exposing variant will be added (T008b / T103).
type EvictionCounter struct {
	total    atomic.Int64
	capacity atomic.Int64
	ttl      atomic.Int64
	explicit atomic.Int64
}

// Inc records one eviction. Safe for concurrent callers.
func (c *EvictionCounter) Inc(reason EvictReason) {
	c.total.Add(1)
	switch reason {
	case EvictReasonCapacity:
		c.capacity.Add(1)
	case EvictReasonTTL:
		c.ttl.Add(1)
	case EvictReasonExplicit:
		c.explicit.Add(1)
	}
}

// Total returns the lifetime count of all evictions across reasons.
func (c *EvictionCounter) Total() int64 { return c.total.Load() }

// ByReason returns the count for one reason. Pass EvictReasonCapacity /
// EvictReasonTTL / EvictReasonExplicit.
func (c *EvictionCounter) ByReason(r EvictReason) int64 {
	switch r {
	case EvictReasonCapacity:
		return c.capacity.Load()
	case EvictReasonTTL:
		return c.ttl.Load()
	case EvictReasonExplicit:
		return c.explicit.Load()
	default:
		return 0
	}
}

// Catalog is the runtime registry of caches keyed by logical resource
// name. Demo-backend resolves cache instances by name (e.g. "topology",
// "workloads"); the catalog provides one EvictionCounter per resource
// so future Prometheus instrumentation can label by resource.
type Catalog struct {
	mu     sync.RWMutex
	counts map[string]*EvictionCounter
}

// NewCatalog returns an empty catalog.
func NewCatalog() *Catalog {
	return &Catalog{counts: map[string]*EvictionCounter{}}
}

// Counter returns the per-resource EvictionCounter, creating it on first
// access. Caller-allocated catalog rather than a package singleton to
// keep tests independent.
func (c *Catalog) Counter(resource string) *EvictionCounter {
	c.mu.RLock()
	if ec, ok := c.counts[resource]; ok {
		c.mu.RUnlock()
		return ec
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	if ec, ok := c.counts[resource]; ok {
		return ec
	}
	ec := &EvictionCounter{}
	c.counts[resource] = ec
	return ec
}

// Snapshot returns the current totals across all registered resources,
// keyed by resource name. Useful for /api/v1/debug or test asserts.
func (c *Catalog) Snapshot() map[string]int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]int64, len(c.counts))
	for k, v := range c.counts {
		out[k] = v.Total()
	}
	return out
}
