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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// PrometheusIngestor reads windowed NPU utilization from a Prometheus
// HTTP API endpoint. Per ADR-0012 §1 reconcile step 3 + Phase 8 P8-T-006
// the production NPUVerticalScaler controller uses this implementation;
// tests use FakeIngestor.
//
// PromQL contract (Phase 8 ships only NPUUtilization built-in metric;
// Phase 9 PrometheusQuery custom enum is forward note per ADR-0012 §7):
//
//	avg_over_time(ascend_npu_utilization_percent{namespace="$ns",
//	  model_service="$ms"}[$window])
//
// Transport behaviour:
//   - Connection errors / 5xx / empty result vector → IngestorResult{NoData: true}
//   - JSON parse errors / 4xx → IngestorResult{Err: ...}
//   - Empty PrometheusURL → IngestorResult{NoData: true} unconditional (test
//     mode + degraded mode when chart values.yaml `metrics.prometheusURL`
//     unset; controller treats NoData as "no scaling decision this tick")
type PrometheusIngestor struct {
	opts   IngestorOpts
	client *http.Client
}

// NewPrometheusIngestor constructs a PrometheusIngestor with the given
// options. Pass IngestorOpts{} to get a degraded ingestor that always
// returns NoData=true (useful when chart `metrics.prometheusURL` value
// is left empty).
func NewPrometheusIngestor(opts IngestorOpts) *PrometheusIngestor {
	timeout := opts.QueryTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &PrometheusIngestor{
		opts: opts,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Query executes a single PromQL lookup and returns the windowed average.
// Per ADR-0012 §1 the controller treats NoData=true as "no scaling
// decision this tick"; transport errors fold into NoData=true so that
// transient Prometheus outages do not flip the scaler ConditionActive=False.
func (p *PrometheusIngestor) Query(ctx context.Context, q IngestorQuery) IngestorResult {
	if strings.TrimSpace(p.opts.PrometheusURL) == "" {
		return IngestorResult{NoData: true}
	}

	// P9-T-007: when CustomPromQL is provided (ADR-0012 §7 forward note
	// PrometheusQuery metric.Type path), use the expression verbatim and
	// skip the NPUUtilization built-in PromQL shape. Namespace + model_service
	// label scoping is the operator's responsibility inside the expression.
	var promQL string
	if strings.TrimSpace(q.CustomPromQL) != "" {
		promQL = q.CustomPromQL
	} else {
		if q.Namespace == "" || q.ModelService == "" || q.WindowSeconds <= 0 {
			return IngestorResult{Err: fmt.Errorf("ingestor: query missing namespace / modelService / windowSeconds")}
		}
		promQL = fmt.Sprintf(
			`avg_over_time(ascend_npu_utilization_percent{namespace="%s", model_service="%s"}[%ds])`,
			q.Namespace, q.ModelService, q.WindowSeconds,
		)
	}

	endpoint := strings.TrimRight(p.opts.PrometheusURL, "/") + "/api/v1/query"
	params := url.Values{}
	params.Set("query", promQL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return IngestorResult{Err: fmt.Errorf("ingestor: build request: %w", err)}
	}
	resp, err := p.client.Do(req)
	if err != nil {
		// Network / DNS / timeout → fold into NoData=true. ADR-0012 §1
		// reconcile step 3 contract.
		return IngestorResult{NoData: true}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		// 5xx → degraded; treat as NoData (don't flip scaler Active=False
		// on transient Prometheus 5xx).
		return IngestorResult{NoData: true}
	}
	if resp.StatusCode >= 400 {
		// 4xx (incl. 400 query syntax error) → genuine bug; surface.
		body, _ := io.ReadAll(resp.Body)
		return IngestorResult{Err: fmt.Errorf(
			"ingestor: prometheus %d: %s", resp.StatusCode, string(body))}
	}

	var pr prometheusQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return IngestorResult{Err: fmt.Errorf("ingestor: decode response: %w", err)}
	}
	if pr.Status != "success" {
		return IngestorResult{Err: fmt.Errorf(
			"ingestor: prometheus status=%q error=%q", pr.Status, pr.Error)}
	}
	if pr.Data.ResultType != "vector" {
		return IngestorResult{Err: fmt.Errorf(
			"ingestor: unexpected resultType=%q (want vector)", pr.Data.ResultType)}
	}
	if len(pr.Data.Result) == 0 {
		// Empty vector → series missing entirely from Prometheus.
		// ADR-0012 §1 contract: NoData=true.
		return IngestorResult{NoData: true}
	}

	// avg_over_time returns one series per label combination. We expect
	// exactly one (namespace + model_service are unique selectors); if
	// more than one is returned, average them — defensive against label
	// drift in the exporter.
	samples := make([]MetricSample, 0, len(pr.Data.Result))
	for _, r := range pr.Data.Result {
		if len(r.Value) != 2 {
			continue
		}
		// r.Value[0] is timestamp (float seconds), r.Value[1] is value (string).
		tsFloat, ok := r.Value[0].(float64)
		if !ok {
			continue
		}
		valStr, ok := r.Value[1].(string)
		if !ok {
			continue
		}
		val, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			continue
		}
		samples = append(samples, MetricSample{
			Timestamp: int64(tsFloat * 1000),
			Value:     val,
		})
	}
	if len(samples) == 0 {
		return IngestorResult{NoData: true}
	}
	return WindowedAverage(samples)
}

// prometheusQueryResponse maps the /api/v1/query response shape per
// https://prometheus.io/docs/prometheus/latest/querying/api/#instant-queries.
// Only the fields we need; tolerant of extra fields.
type prometheusQueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			// Value is [timestamp float, value string]. Use []interface{}
			// because the value is a string in JSON even for numeric data
			// (Prometheus convention).
			Value []interface{} `json:"value"`
		} `json:"result"`
	} `json:"data"`
	Error string `json:"error,omitempty"`
}

// Compile-time interface assertion.
var _ Ingestor = (*PrometheusIngestor)(nil)
