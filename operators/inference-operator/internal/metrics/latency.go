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
	"fmt"
)

// P99 inference-latency SLA support (P13-T-206 · ADR-0025 §2 Decision E).
//
// The end-to-end request latency histogram is exposed by the vllm-ascend PD
// proxy_server (Phase 6 P6-T-105 · external data-plane · metric
// `vllm:e2e_request_latency_seconds_bucket`), scraped by Prometheus. The
// operator does NOT host the histogram — it QUERIES the server-side P99 via
// Prometheus `histogram_quantile` (reusing the Ingestor's CustomPromQL path)
// and feeds it to the NPUVerticalScaler as a latency-aware scale signal. The
// tests/sla load-test harness drives load + measures P99 client-side against
// the same SLO.

// DefaultP99SLOMillis is the documented default P99 end-to-end inference
// latency SLO (milliseconds). It is a REFERENCE default, NOT a customer SLA
// contract — the customer swaps it per ADR-0025 §4(b):
//   - NPUVerticalScalerReconciler.LatencySLOMillis (controller-side scale gate),
//   - tests/sla SLO_MS env (load-test pass/fail threshold).
//
// 2000ms is a deliberately loose reference for an 8B PD model on a single
// 910B; a real SLA is workload + token-budget specific.
const DefaultP99SLOMillis = 2000.0

// VLLMLatencyHistogramMetric is the Prometheus histogram base name the
// vllm-ascend proxy exposes (the `_bucket` suffix carries the `le` label).
// CUSTOMER SWAP: set to the customer serving stack's latency histogram if it
// differs (e.g. a MindIE / Triton metric name).
const VLLMLatencyHistogramMetric = "vllm:e2e_request_latency_seconds"

// BuildP99LatencyPromQL returns the PromQL that computes the P99 of the
// end-to-end request latency histogram for one ModelService over the window.
// histogram_quantile over the per-`le` rate of the _bucket series — the
// standard Prometheus latency-percentile shape.
func BuildP99LatencyPromQL(modelService string, windowSeconds int32) string {
	if windowSeconds <= 0 {
		windowSeconds = 300
	}
	return fmt.Sprintf(
		`histogram_quantile(0.99, sum(rate(%s_bucket{model_service="%s"}[%ds])) by (le))`,
		VLLMLatencyHistogramMetric, modelService, windowSeconds,
	)
}

// QueryP99LatencyMillis queries the server-side P99 latency for a ModelService
// via the Ingestor (CustomPromQL) and returns it in milliseconds. ok=false on
// NoData / error (caller treats as "no latency signal this tick" — fail-open,
// never blocks scaling). The vllm histogram is in SECONDS; result is ×1000.
func QueryP99LatencyMillis(ctx context.Context, ing Ingestor, modelService string, windowSeconds int32) (float64, bool) {
	if ing == nil {
		return 0, false
	}
	res := ing.Query(ctx, IngestorQuery{
		CustomPromQL: BuildP99LatencyPromQL(modelService, windowSeconds),
	})
	if res.Err != nil || res.NoData {
		return 0, false
	}
	return res.Value * 1000.0, true
}
