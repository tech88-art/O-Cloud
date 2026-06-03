/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package metrics

import (
	"context"
	"strings"
	"testing"
)

func TestBuildP99LatencyPromQL(t *testing.T) {
	q := BuildP99LatencyPromQL("qwen-pd", 300)
	for _, want := range []string{"histogram_quantile(0.99", "vllm:e2e_request_latency_seconds_bucket", `model_service="qwen-pd"`, "[300s]", "by (le)"} {
		if !strings.Contains(q, want) {
			t.Errorf("PromQL missing %q\n  got: %s", want, q)
		}
	}
	// zero window → default 300s
	if !strings.Contains(BuildP99LatencyPromQL("m", 0), "[300s]") {
		t.Errorf("zero windowSeconds should default to 300s")
	}
}

func TestQueryP99LatencyMillis(t *testing.T) {
	// vllm histogram is seconds; 0.85s → 850ms.
	ing := NewFakeIngestor([]IngestorResult{{Value: 0.85}})
	ms, ok := QueryP99LatencyMillis(context.Background(), ing, "qwen-pd", 300)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if ms < 849.9 || ms > 850.1 {
		t.Errorf("P99 ms = %f; want ~850 (0.85s ×1000)", ms)
	}
}

func TestQueryP99LatencyMillis_NoDataAndNil(t *testing.T) {
	// NoData → ok=false (no latency signal · fail-open).
	ing := NewFakeIngestor([]IngestorResult{{NoData: true}})
	if _, ok := QueryP99LatencyMillis(context.Background(), ing, "m", 300); ok {
		t.Errorf("NoData should yield ok=false")
	}
	// nil ingestor → ok=false (latency feedback disabled).
	if _, ok := QueryP99LatencyMillis(context.Background(), nil, "m", 300); ok {
		t.Errorf("nil ingestor should yield ok=false")
	}
}
