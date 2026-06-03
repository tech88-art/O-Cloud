/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

package main

import (
	"math"
	"sort"
)

// Percentile returns the q-quantile (q in [0,1]) of samples using the
// nearest-rank method. Input need not be sorted (a copy is sorted). Empty
// input returns 0. q<=0 → min; q>=1 → max.
//
// This is the CLIENT-side P99 computation over latencies measured by the load
// generator — the operator side (internal/metrics/latency.go) instead queries
// the server-side Prometheus histogram_quantile, so there is no shared code to
// keep in sync across the module boundary.
func Percentile(samples []float64, q float64) float64 {
	n := len(samples)
	if n == 0 {
		return 0
	}
	s := make([]float64, n)
	copy(s, samples)
	sort.Float64s(s)
	if q <= 0 {
		return s[0]
	}
	if q >= 1 {
		return s[n-1]
	}
	rank := int(math.Ceil(q * float64(n)))
	if rank < 1 {
		rank = 1
	}
	if rank > n {
		rank = n
	}
	return s[rank-1]
}

// Summary holds the latency distribution summary the harness reports.
type Summary struct {
	Count         int
	Errors        int
	P50, P90, P99 float64 // milliseconds
	Max           float64 // milliseconds
}

// Summarize computes the reported percentiles over the success latencies (ms).
func Summarize(latenciesMs []float64, errors int) Summary {
	s := Summary{Count: len(latenciesMs), Errors: errors}
	if len(latenciesMs) == 0 {
		return s
	}
	s.P50 = Percentile(latenciesMs, 0.50)
	s.P90 = Percentile(latenciesMs, 0.90)
	s.P99 = Percentile(latenciesMs, 0.99)
	s.Max = Percentile(latenciesMs, 1.0)
	return s
}
