/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

package ratealgo

import (
	"testing"
	"time"
)

func TestTokenBucketAllowsBurstUpToCapacity(t *testing.T) {
	b := NewTokenBucket(5, time.Minute)
	for i := 0; i < 5; i++ {
		if !b.Allow() {
			t.Fatalf("Allow #%d should succeed within capacity", i+1)
		}
	}
	if b.Allow() {
		t.Fatal("6th Allow should deny(bucket empty)")
	}
}

func TestTokenBucketRefillsOverTime(t *testing.T) {
	// capacity 10 / window 1s → refill 10 tokens/sec.
	b := NewTokenBucket(10, time.Second)
	// Drain all 10.
	for i := 0; i < 10; i++ {
		b.Allow()
	}
	if b.Allow() {
		t.Fatal("expected deny after drain")
	}
	// Wait 200ms · 2 tokens replenish.
	time.Sleep(220 * time.Millisecond)
	if !b.Allow() {
		t.Fatal("expected Allow after 200ms refill")
	}
}

func TestTokenBucketCapacityClamp(t *testing.T) {
	b := NewTokenBucket(3, 100*time.Millisecond)
	// Sleep long enough that refill would exceed capacity.
	time.Sleep(500 * time.Millisecond)
	tk := b.Tokens()
	if tk > 3.0 {
		t.Fatalf("Tokens = %.2f, want ≤ 3 (capacity clamp)", tk)
	}
}

func TestTokenBucketZeroDefaults(t *testing.T) {
	b := NewTokenBucket(0, 0)
	if b.Capacity() != 1 {
		t.Fatalf("Capacity = %.2f, want 1 (zero default)", b.Capacity())
	}
}

func TestAllowN(t *testing.T) {
	b := NewTokenBucket(10, time.Minute)
	if !b.AllowN(5) {
		t.Fatal("AllowN(5) should succeed initially")
	}
	if !b.AllowN(5) {
		t.Fatal("AllowN(5) second call should succeed(remaining 5)")
	}
	if b.AllowN(1) {
		t.Fatal("AllowN(1) third call should fail(bucket empty)")
	}
}

func TestAllowNZero(t *testing.T) {
	b := NewTokenBucket(5, time.Minute)
	if !b.AllowN(0) {
		t.Fatal("AllowN(0) should always succeed (no-op)")
	}
}
