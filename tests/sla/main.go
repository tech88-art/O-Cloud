/*
Copyright 2026.
Licensed under the Apache License, Version 2.0.

P99 SLA load-test harness (P13-T-206 · ADR-0025 §2 Decision E).

Fires a fixed number of concurrent HTTP requests at a vllm-ascend PD inference
endpoint, measures per-request latency, and reports P50/P90/P99 against a
documented default SLO. Exits non-zero when P99 breaches the SLO or any request
errors — so it doubles as a lab gate.

stdlib-only. Run against a real lab endpoint (see tests/sla/README.md):

	TARGET_URL=http://qwen-pd.ocloud-system.svc.cluster.local:8000/v1/completions \
	  go run ./tests/sla -n 500 -c 20 -slo-ms 2000

The default SLO (2000ms) is a REFERENCE value (mirrors
metrics.DefaultP99SLOMillis); the customer swaps it via -slo-ms / SLO_MS per
ADR-0025 §4(b).
*/
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func main() {
	url := flag.String("url", envOr("TARGET_URL", ""), "inference endpoint URL (or TARGET_URL env)")
	n := flag.Int("n", envInt("REQUESTS", 200), "total requests (or REQUESTS env)")
	c := flag.Int("c", envInt("CONCURRENCY", 10), "concurrent workers (or CONCURRENCY env)")
	sloMs := flag.Float64("slo-ms", envFloat("SLO_MS", 2000), "P99 SLO in ms (or SLO_MS env · reference default 2000)")
	method := flag.String("method", envOr("METHOD", "POST"), "HTTP method")
	body := flag.String("body", envOr("BODY", `{"prompt":"hello","max_tokens":16}`), "request body")
	timeoutS := flag.Float64("timeout", envFloat("TIMEOUT_S", 30), "per-request timeout (s)")
	flag.Parse()

	if *url == "" {
		fmt.Fprintln(os.Stderr, "ERROR: -url (or TARGET_URL) is required — this harness drives a real PD endpoint.")
		fmt.Fprintln(os.Stderr, "See tests/sla/README.md. For a self-check of the percentile math: go test ./tests/sla")
		os.Exit(2)
	}

	fmt.Printf("== P99 SLA load test ==\n  url=%s\n  requests=%d concurrency=%d slo=%.0fms\n\n", *url, *n, *c, *sloMs)

	client := &http.Client{Timeout: time.Duration(*timeoutS * float64(time.Second))}
	jobs := make(chan int)
	var (
		mu        sync.Mutex
		latencies []float64
		errors    int
	)
	var wg sync.WaitGroup
	for w := 0; w < *c; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				start := time.Now()
				ok := doRequest(client, *method, *url, *body)
				elapsedMs := float64(time.Since(start).Microseconds()) / 1000.0
				mu.Lock()
				if ok {
					latencies = append(latencies, elapsedMs)
				} else {
					errors++
				}
				mu.Unlock()
			}
		}()
	}
	for i := 0; i < *n; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	s := Summarize(latencies, errors)
	fmt.Printf("  ok=%d errors=%d\n", s.Count, s.Errors)
	fmt.Printf("  p50=%.1fms p90=%.1fms p99=%.1fms max=%.1fms\n\n", s.P50, s.P90, s.P99, s.Max)

	pass := s.Errors == 0 && s.Count > 0 && s.P99 <= *sloMs
	if pass {
		fmt.Printf("PASS: P99 %.1fms <= SLO %.0fms (0 errors)\n", s.P99, *sloMs)
		os.Exit(0)
	}
	fmt.Printf("FAIL: P99 %.1fms vs SLO %.0fms · errors=%d\n", s.P99, *sloMs, s.Errors)
	os.Exit(1)
}

// doRequest issues one request and returns true on a 2xx response.
func doRequest(client *http.Client, method, url, body string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), client.Timeout)
	defer cancel()
	var rdr io.Reader
	if method != http.MethodGet && body != "" {
		rdr = bytes.NewBufferString(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return false
	}
	if rdr != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}
