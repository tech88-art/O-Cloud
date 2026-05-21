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
	"time"
)

// IngestorOpts configures a PrometheusIngestor.
//
// Per Phase 8 P8-T-006 / ADR-0012 §1 reconcile step 3 the ingestor reads
// `ascend_npu_utilization_percent` (exposed by ascend-npu-exporter-plus)
// scoped by namespace + model_service label using PromQL:
//
//	avg_over_time(ascend_npu_utilization_percent{namespace="$ns",
//	  model_service="$ms"}[$window])
type IngestorOpts struct {
	// PrometheusURL is the base URL of the Prometheus HTTP API
	// (e.g. http://prometheus.observability:9090). When empty the
	// PrometheusIngestor returns NoData=true for every query.
	PrometheusURL string

	// QueryTimeout caps each /api/v1/query roundtrip.
	// Default 5s when zero.
	QueryTimeout time.Duration
}

// IngestorQuery describes the dimensions of a metric lookup.
//
// PromQL shape (filled in by the ingestor):
//
//	avg_over_time(ascend_npu_utilization_percent{namespace="$Namespace",
//	  model_service="$ModelService"}[$WindowSeconds.s])
type IngestorQuery struct {
	// Namespace is the Kubernetes namespace label.
	Namespace string
	// ModelService is the model_service label on the metric series.
	ModelService string
	// WindowSeconds is the sliding-window length in seconds. Mirrors
	// NPUVerticalScalerSpec.Metric.WindowSeconds at controller wiring.
	WindowSeconds int32
}

// IngestorResult is the controller-facing answer per reconcile tick.
//
// Per ADR-0012 §1 reconcile step 3 the controller treats NoData=true as
// "no scaling decision this tick" (stay + requeue 30s); the controller
// does not treat NoData as an error condition unless it persists across
// N reconcile ticks (Phase 8 T007 ConditionActive=False threshold).
type IngestorResult struct {
	// Value is the windowed average of the metric series (0-100 for
	// NPUUtilization). Only meaningful when NoData=false and Err==nil.
	Value float64

	// NoData indicates Prometheus returned an empty vector OR the
	// ingestor was constructed without a Prometheus URL. The controller
	// MUST treat this as "stay" + requeue; do NOT treat as an error.
	NoData bool

	// Err is non-nil only on transport / parse failures (Prometheus
	// reachable but query returned 5xx, JSON parse failed, etc.).
	// Network unreachable should manifest as NoData=true with Err==nil
	// (PrometheusIngestor sets a short timeout and folds connection
	// errors into NoData).
	Err error
}

// Ingestor is the controller-facing contract. PrometheusIngestor is the
// production impl; FakeIngestor is the testing impl.
//
// Per Phase 8 P8-T-006 acceptance: `var _ Ingestor = (*FakeIngestor)(nil)`
// at fake_ingestor.go enforces the interface guarantee at compile time.
type Ingestor interface {
	// Query executes a single metric lookup and returns the windowed
	// average. ctx deadline takes precedence over IngestorOpts.QueryTimeout.
	Query(ctx context.Context, q IngestorQuery) IngestorResult
}

// MetricSample is a single observation returned by the underlying
// Prometheus instant-vector query. Exported for tests + future Phase 9
// PromQL custom-query extension (ADR-0012 §7 forward note).
type MetricSample struct {
	// Timestamp of the observation (Unix epoch milliseconds at the
	// Prometheus side).
	Timestamp int64
	// Value of the metric at the observation timestamp.
	Value float64
}

// WindowedAverage averages a set of MetricSample values. Exported for
// tests + future custom-query extension. Returns 0 + NoData=true for
// empty input.
func WindowedAverage(samples []MetricSample) IngestorResult {
	if len(samples) == 0 {
		return IngestorResult{NoData: true}
	}
	var sum float64
	for _, s := range samples {
		sum += s.Value
	}
	return IngestorResult{Value: sum / float64(len(samples))}
}
