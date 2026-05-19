package cache

import (
	"sync"
	"testing"
)

// ============================================================================
// EvictionCounter primitives
// ============================================================================

func TestEvictionCounter_Inc(t *testing.T) {
	var c EvictionCounter
	c.Inc(EvictReasonCapacity)
	c.Inc(EvictReasonTTL)
	c.Inc(EvictReasonExplicit)

	if got := c.Total(); got != 3 {
		t.Fatalf("Total = %d; want 3", got)
	}
	if got := c.ByReason(EvictReasonCapacity); got != 1 {
		t.Fatalf("Capacity = %d; want 1", got)
	}
	if got := c.ByReason(EvictReasonTTL); got != 1 {
		t.Fatalf("TTL = %d; want 1", got)
	}
	if got := c.ByReason(EvictReasonExplicit); got != 1 {
		t.Fatalf("Explicit = %d; want 1", got)
	}
	if got := c.ByReason(EvictReason(99)); got != 0 {
		t.Fatalf("unknown reason = %d; want 0", got)
	}
}

func TestEvictionCounter_Concurrent(t *testing.T) {
	var c EvictionCounter
	const N = 1000
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Spread across all three reasons; total still == N.
			switch i % 3 {
			case 0:
				c.Inc(EvictReasonCapacity)
			case 1:
				c.Inc(EvictReasonTTL)
			case 2:
				c.Inc(EvictReasonExplicit)
			}
		}(i)
	}
	wg.Wait()
	if got := c.Total(); got != N {
		t.Fatalf("Total = %d; want %d (atomic semantics)", got, N)
	}
	sum := c.ByReason(EvictReasonCapacity) +
		c.ByReason(EvictReasonTTL) +
		c.ByReason(EvictReasonExplicit)
	if sum != N {
		t.Fatalf("sum across reasons = %d; want %d", sum, N)
	}
}

// ============================================================================
// Catalog: per-resource lookup
// ============================================================================

func TestCatalog_LazyCreate(t *testing.T) {
	cat := NewCatalog()
	a := cat.Counter("topology")
	b := cat.Counter("topology")
	if a != b {
		t.Fatalf("Counter(topology) returned different instances on repeat calls")
	}
	c := cat.Counter("workloads")
	if a == c {
		t.Fatalf("Counter(workloads) reused topology counter — must be distinct")
	}
}

func TestCatalog_Snapshot(t *testing.T) {
	cat := NewCatalog()
	cat.Counter("topology").Inc(EvictReasonCapacity)
	cat.Counter("topology").Inc(EvictReasonTTL)
	cat.Counter("workloads").Inc(EvictReasonExplicit)

	snap := cat.Snapshot()
	if got := snap["topology"]; got != 2 {
		t.Fatalf("snapshot[topology] = %d; want 2", got)
	}
	if got := snap["workloads"]; got != 1 {
		t.Fatalf("snapshot[workloads] = %d; want 1", got)
	}
	if _, ok := snap["nodes"]; ok {
		t.Fatalf("snapshot included nodes which was never registered")
	}
}

func TestCatalog_ConcurrentLookup(t *testing.T) {
	cat := NewCatalog()
	const N = 200
	var wg sync.WaitGroup
	counters := make([]*EvictionCounter, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			counters[i] = cat.Counter("shared")
		}(i)
	}
	wg.Wait()
	first := counters[0]
	for i := 1; i < N; i++ {
		if counters[i] != first {
			t.Fatalf("Catalog produced %d distinct counters for the same key", i)
		}
	}
}
