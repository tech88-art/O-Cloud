package cache

import (
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// ============================================================================
// Back-compat: NewLRU keeps the Phase-1 contract.
// ============================================================================

func TestNewLRU_BackCompat(t *testing.T) {
	c, err := NewLRU[string, int](4)
	if err != nil {
		t.Fatalf("NewLRU: %v", err)
	}
	c.Add("a", 1)
	c.Add("b", 2)
	if got, ok := c.Get("a"); !ok || got != 1 {
		t.Fatalf("Get(a) = %d, %v; want 1, true", got, ok)
	}
	if c.Len() != 2 {
		t.Fatalf("Len = %d; want 2", c.Len())
	}
}

func TestNewLRU_RejectsNonPositiveSize(t *testing.T) {
	if _, err := NewLRU[string, int](0); err == nil {
		t.Fatalf("NewLRU(0) returned nil error")
	}
	if _, err := NewLRU[string, int](-1); err == nil {
		t.Fatalf("NewLRU(-1) returned nil error")
	}
}

// ============================================================================
// New(Options): validation
// ============================================================================

func TestNew_OptionsValidation(t *testing.T) {
	cases := []struct {
		name string
		opts Options
		wantErr bool
	}{
		{"zero max", Options{MaxEntries: 0}, true},
		{"negative max", Options{MaxEntries: -3}, true},
		{"negative ttl", Options{MaxEntries: 4, TTL: -time.Second}, true},
		{"valid no ttl", Options{MaxEntries: 4}, false},
		{"valid ttl", Options{MaxEntries: 4, TTL: time.Minute}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New[string, int](tc.opts)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// ============================================================================
// Capacity-driven eviction
// ============================================================================

func TestLRU_CapacityEviction(t *testing.T) {
	c, err := New[string, int](Options{MaxEntries: 3})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.Add("k1", 1)
	c.Add("k2", 2)
	c.Add("k3", 3)
	// 4th insert evicts the LRU (k1).
	c.Add("k4", 4)

	if c.Len() != 3 {
		t.Fatalf("Len = %d; want 3", c.Len())
	}
	if _, ok := c.Get("k1"); ok {
		t.Fatalf("k1 should have been evicted")
	}
	if got := c.Counter().Total(); got != 1 {
		t.Fatalf("counter Total = %d; want 1", got)
	}
	if got := c.Counter().ByReason(EvictReasonCapacity); got != 1 {
		t.Fatalf("Capacity = %d; want 1", got)
	}
	if got := c.Counter().ByReason(EvictReasonTTL); got != 0 {
		t.Fatalf("TTL = %d; want 0", got)
	}
}

// ============================================================================
// TTL-driven eviction
// ============================================================================

func TestLRU_TTLEviction(t *testing.T) {
	c, err := New[string, int](Options{MaxEntries: 10, TTL: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.Add("k1", 1)
	// Confirm hit immediately.
	if got, ok := c.Get("k1"); !ok || got != 1 {
		t.Fatalf("immediate Get(k1) = %d, %v; want 1, true", got, ok)
	}
	time.Sleep(100 * time.Millisecond)
	// Now should miss + record eviction.
	if _, ok := c.Get("k1"); ok {
		t.Fatalf("k1 should have TTL-expired")
	}
	if got := c.Counter().Total(); got != 1 {
		t.Fatalf("counter Total = %d; want 1", got)
	}
	if got := c.Counter().ByReason(EvictReasonTTL); got != 1 {
		t.Fatalf("TTL = %d; want 1", got)
	}
	if got := c.Counter().ByReason(EvictReasonCapacity); got != 0 {
		t.Fatalf("Capacity = %d; want 0", got)
	}
}

func TestLRU_TTLDisabledWhenZero(t *testing.T) {
	c, err := New[string, int](Options{MaxEntries: 10, TTL: 0})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.Add("k1", 1)
	time.Sleep(20 * time.Millisecond)
	// Without TTL, the entry stays regardless of age.
	if got, ok := c.Get("k1"); !ok || got != 1 {
		t.Fatalf("Get(k1) after sleep = %d, %v; want 1, true (TTL=0 disables expiry)", got, ok)
	}
}

// ============================================================================
// OnEvict callbacks
// ============================================================================

func TestLRU_OnEvictTTL(t *testing.T) {
	var (
		mu      sync.Mutex
		seenKey any
		seenVal any
		seenWhy EvictReason
	)
	cb := func(k, v any, why EvictReason) {
		mu.Lock()
		defer mu.Unlock()
		seenKey = k
		seenVal = v
		seenWhy = why
	}
	c, err := New[string, int](Options{
		MaxEntries: 10,
		TTL:        30 * time.Millisecond,
		OnEvict:    cb,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.Add("k1", 42)
	time.Sleep(80 * time.Millisecond)
	c.Get("k1") // triggers TTL eviction

	mu.Lock()
	defer mu.Unlock()
	if seenKey != "k1" {
		t.Fatalf("OnEvict key = %v; want k1", seenKey)
	}
	if seenVal != 42 {
		t.Fatalf("OnEvict val = %v; want 42", seenVal)
	}
	if seenWhy != EvictReasonTTL {
		t.Fatalf("OnEvict reason = %v; want ttl", seenWhy)
	}
}

func TestLRU_OnEvictExplicit(t *testing.T) {
	var (
		mu       sync.Mutex
		seenWhy  EvictReason
		seenKey  any
		callCount int
	)
	cb := func(k, v any, why EvictReason) {
		mu.Lock()
		defer mu.Unlock()
		seenKey = k
		seenWhy = why
		callCount++
	}
	c, err := New[string, int](Options{MaxEntries: 10, OnEvict: cb})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.Add("ke", 7)
	if ok := c.Remove("ke"); !ok {
		t.Fatalf("Remove(ke) returned false")
	}

	mu.Lock()
	defer mu.Unlock()
	if callCount != 1 {
		t.Fatalf("callback fired %d times; want 1", callCount)
	}
	if seenKey != "ke" {
		t.Fatalf("OnEvict key = %v; want ke", seenKey)
	}
	if seenWhy != EvictReasonExplicit {
		t.Fatalf("OnEvict reason = %v; want explicit", seenWhy)
	}
	if got := c.Counter().ByReason(EvictReasonExplicit); got != 1 {
		t.Fatalf("Counter Explicit = %d; want 1", got)
	}
}

func TestLRU_OnEvictCapacity(t *testing.T) {
	var (
		mu      sync.Mutex
		events  []struct {
			key  any
			val  any
			why  EvictReason
		}
	)
	cb := func(k, v any, why EvictReason) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, struct {
			key, val any
			why      EvictReason
		}{k, v, why})
	}
	c, err := New[string, int](Options{MaxEntries: 2, OnEvict: cb})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.Add("a", 1)
	c.Add("b", 2)
	c.Add("c", 3) // evicts a

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 1 {
		t.Fatalf("got %d eviction events; want 1", len(events))
	}
	e := events[0]
	if e.key != "a" || e.val != 1 {
		t.Fatalf("OnEvict capacity = (%v, %v); want (a, 1)", e.key, e.val)
	}
	if e.why != EvictReasonCapacity {
		t.Fatalf("OnEvict reason = %v; want capacity", e.why)
	}
}

// ============================================================================
// Purge accounts each remaining entry as explicit eviction.
// ============================================================================

func TestLRU_PurgeCountsExplicit(t *testing.T) {
	c, err := New[string, int](Options{MaxEntries: 10})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.Add("a", 1)
	c.Add("b", 2)
	c.Add("c", 3)
	c.Purge()
	if c.Len() != 0 {
		t.Fatalf("Len after Purge = %d; want 0", c.Len())
	}
	if got := c.Counter().ByReason(EvictReasonExplicit); got != 3 {
		t.Fatalf("Explicit = %d; want 3", got)
	}
	if got := c.Counter().Total(); got != 3 {
		t.Fatalf("Total = %d; want 3", got)
	}
}

// ============================================================================
// Reason string mapping
// ============================================================================

func TestEvictReason_String(t *testing.T) {
	cases := map[EvictReason]string{
		EvictReasonCapacity: "capacity",
		EvictReasonTTL:      "ttl",
		EvictReasonExplicit: "explicit",
	}
	for r, want := range cases {
		if got := r.String(); got != want {
			t.Errorf("%d.String() = %q; want %q", int(r), got, want)
		}
	}
	if got := EvictReason(99).String(); got != "unknown(99)" {
		t.Errorf("unknown.String() = %q; want unknown(99)", got)
	}
}

// ============================================================================
// P4-T-008 Prometheus instrumentation: nil-safe + non-nil observable.
// ============================================================================

func newTestCounter(t *testing.T, name string) *prometheus.CounterVec {
	t.Helper()
	return prometheus.NewCounterVec(prometheus.CounterOpts{Name: name},
		[]string{"resource"})
}

func TestLRU_PrometheusCounters_NilSafe(t *testing.T) {
	// Plan acceptance: passing nil retains Phase 3 behaviour with no panic.
	c, err := New[string, int](Options{MaxEntries: 2}) // no counters
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.Add("a", 1)
	c.Add("b", 2)
	c.Add("c", 3) // evicts "a"
	if got, _ := c.Get("b"); got != 2 {
		t.Errorf("nil-safe Get hit returned %d, want 2", got)
	}
	if got := c.Counter().Total(); got != 1 {
		t.Errorf("nil-safe eviction counter Total = %d, want 1", got)
	}
}

func TestLRU_PrometheusCounters_EvictionObservable(t *testing.T) {
	evictCounter := newTestCounter(t, "ocloud_backend_cache_eviction_total")
	c, err := New[string, int](Options{
		MaxEntries:         2,
		EvictionCounterVec: evictCounter,
		Resource:           "test-topology",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.Add("a", 1)
	c.Add("b", 2)
	c.Add("c", 3) // evicts "a" → counter +1
	c.Add("d", 4) // evicts "b" → counter +1

	if got := testutil.ToFloat64(evictCounter.WithLabelValues("test-topology")); got != 2 {
		t.Errorf("eviction counter for resource=test-topology = %v, want 2", got)
	}
	// Other resources should not have been touched.
	if got := testutil.CollectAndCount(evictCounter); got != 1 {
		t.Errorf("only one labelset should exist, got %d", got)
	}
}

func TestLRU_PrometheusCounters_HitsObservable(t *testing.T) {
	hitsCounter := newTestCounter(t, "ocloud_backend_cache_hits_total")
	c, err := New[string, int](Options{
		MaxEntries:     4,
		HitsCounterVec: hitsCounter,
		Resource:       "presets",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.Add("a", 1)
	c.Add("b", 2)
	// 3 hits on "a".
	for i := 0; i < 3; i++ {
		if v, ok := c.Get("a"); !ok || v != 1 {
			t.Fatalf("hit Get iteration %d: ok=%v v=%v", i, ok, v)
		}
	}
	// 1 miss — should NOT increment the hits counter.
	if _, ok := c.Get("nonexistent"); ok {
		t.Fatal("miss should not return ok=true")
	}

	if got := testutil.ToFloat64(hitsCounter.WithLabelValues("presets")); got != 3 {
		t.Errorf("hits counter = %v, want 3 (misses must NOT increment hits)", got)
	}
}

func TestLRU_PrometheusCounters_ResourceLabelRequired(t *testing.T) {
	// Validate: when a counter is set, Resource must be too.
	evict := newTestCounter(t, "x_evict")
	_, err := New[string, int](Options{
		MaxEntries:         2,
		EvictionCounterVec: evict,
		// Resource intentionally empty
	})
	if err == nil {
		t.Fatal("expected validation error when counter set but Resource empty")
	}
}

func TestLRU_PrometheusCounters_TTLEvictionCounted(t *testing.T) {
	// Plan acceptance: counter increments for every eviction regardless of
	// cause. Exercise the TTL path explicitly.
	evictCounter := newTestCounter(t, "ocloud_backend_cache_eviction_total")
	c, err := New[string, int](Options{
		MaxEntries:         4,
		TTL:                10 * time.Millisecond,
		EvictionCounterVec: evictCounter,
		Resource:           "topology",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.Add("a", 1)
	time.Sleep(30 * time.Millisecond)
	if _, ok := c.Get("a"); ok {
		t.Fatal("TTL-expired Get must return miss")
	}
	if got := testutil.ToFloat64(evictCounter.WithLabelValues("topology")); got != 1 {
		t.Errorf("TTL eviction counter = %v, want 1", got)
	}
}
