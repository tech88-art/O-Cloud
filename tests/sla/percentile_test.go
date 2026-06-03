/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.
*/

package main

import "testing"

func TestPercentileNearestRank(t *testing.T) {
	// samples 1..100 (unsorted on purpose).
	xs := make([]float64, 0, 100)
	for i := 100; i >= 1; i-- {
		xs = append(xs, float64(i))
	}
	cases := []struct {
		q    float64
		want float64
	}{
		{0.50, 50}, // ceil(0.50*100)=50 → s[49]=50
		{0.90, 90}, // ceil(0.90*100)=90 → s[89]=90
		{0.99, 99}, // ceil(0.99*100)=99 → s[98]=99
		{1.0, 100}, // max
		{0.0, 1},   // min
	}
	for _, c := range cases {
		if got := Percentile(xs, c.q); got != c.want {
			t.Errorf("Percentile(q=%.2f) = %v; want %v", c.q, got, c.want)
		}
	}
}

func TestPercentileEmpty(t *testing.T) {
	if got := Percentile(nil, 0.99); got != 0 {
		t.Errorf("Percentile(empty) = %v; want 0", got)
	}
}

func TestSummarize(t *testing.T) {
	xs := []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	s := Summarize(xs, 3)
	if s.Count != 10 || s.Errors != 3 {
		t.Errorf("Count/Errors = %d/%d; want 10/3", s.Count, s.Errors)
	}
	if s.Max != 100 {
		t.Errorf("Max = %v; want 100", s.Max)
	}
	if s.P99 != 100 { // ceil(0.99*10)=10 → s[9]=100
		t.Errorf("P99 = %v; want 100", s.P99)
	}
}
